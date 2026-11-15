package api

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"path"
	"regexp"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"

	"github.com/hikariming/dsh-ee/server/internal/registry"
)

// 技能包导入（docs/expert-center.md §8 第 4 批）：员工上传一个 .zip（里面有 SKILL.md，可以带附件），
// 这里解析成技能的草稿返回给网页编辑器预填；提交仍走 POST /api/hub，审核、范围、下发规则不变。
// 附件只收文本，按「## 附件：路径」接在正文后面（DSH 的技能来源暂时不支持单独的资源文件）。

const (
	skillPkgMaxZip     = 2 << 20 // 上传的 zip 本身
	skillPkgMaxEntries = 100
	skillPkgMaxFile    = 64 << 10 // 单个附件
	skillPkgMaxBody    = 64 << 10 // 合并后的正文，和 validateSpec 一致
)

type skillPackageReq struct {
	Filename string `json:"filename"`
	// zip 内容，base64
	Data string `json:"data"`
}

type skillPackageSkip struct {
	Path   string `json:"path"`
	Reason string `json:"reason"` // binary / too_large / over_limit / unsafe_path
}

type skillFrontmatter struct {
	Name           string `yaml:"name"`
	Title          string `yaml:"title"`
	Description    string `yaml:"description"`
	WhenToUse      string `yaml:"when_to_use"`
	WhenToUseDash  string `yaml:"when-to-use"`
	DisableModel   *bool  `yaml:"disable-model-invocation"`
	ModelInvocable *bool  `yaml:"model_invocable"`
}

var frontmatterRe = regexp.MustCompile(`(?s)\A\x{FEFF}?---\r?\n(.*?)\r?\n---\r?\n?(.*)\z`)
var slugCleanRe = regexp.MustCompile(`[^a-z0-9]+`)

// parseSkillMD 拆出 SKILL.md 的 frontmatter 与正文。没有 frontmatter 时整篇当正文。
func parseSkillMD(src string) (skillFrontmatter, string, error) {
	var fm skillFrontmatter
	m := frontmatterRe.FindStringSubmatch(src)
	if m == nil {
		return fm, strings.TrimSpace(src), nil
	}
	if err := yaml.Unmarshal([]byte(m[1]), &fm); err != nil {
		return fm, "", fmt.Errorf("SKILL.md 开头的 frontmatter 不是合法的 YAML：%v", err)
	}
	return fm, strings.TrimSpace(m[2]), nil
}

// skillSlug 把技能名规范成 kebab-case 标识；规范不出来时返回空串，由员工自己填。
func skillSlug(name string) string {
	s := strings.Trim(slugCleanRe.ReplaceAllString(strings.ToLower(strings.TrimSpace(name)), "-"), "-")
	if len(s) > maxSlug {
		s = strings.Trim(s[:maxSlug], "-")
	}
	if !slugRe.MatchString(s) {
		return ""
	}
	return s
}

// POST /api/hub/skill-package：解析技能包，返回草稿。不写库。
func (s *Server) handleSkillPackage(w http.ResponseWriter, r *http.Request) {
	var req skillPackageReq
	if !readJSONLimit(w, r, &req, skillPkgMaxZip*2) {
		return
	}
	raw, err := base64.StdEncoding.DecodeString(req.Data)
	if err != nil || len(raw) == 0 {
		writeErr(w, http.StatusBadRequest, "没有收到技能包内容")
		return
	}
	if len(raw) > skillPkgMaxZip {
		writeErr(w, http.StatusBadRequest, "技能包不能超过 2 MB")
		return
	}
	draft, msg := parseSkillPackage(raw)
	if msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	writeJSON(w, http.StatusOK, draft)
}

type skillPackageDraft struct {
	Slug     string             `json:"slug"`
	Spec     assetSpec          `json:"spec"`
	Included []string           `json:"included"`
	Skipped  []skillPackageSkip `json:"skipped"`
}

// parseSkillPackage 是 handleSkillPackage 的纯函数部分，返回草稿或给员工看的错误。
func parseSkillPackage(raw []byte) (*skillPackageDraft, string) {
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return nil, "不是有效的 zip 文件"
	}
	if len(zr.File) > skillPkgMaxEntries {
		return nil, fmt.Sprintf("技能包最多 %d 个文件", skillPkgMaxEntries)
	}
	// SKILL.md 可以在根目录，也可以在唯一的一级目录里（直接压缩文件夹的常见情况）
	root := ""
	found := false
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		dir, base := path.Split(f.Name)
		if base == "SKILL.md" && strings.Count(dir, "/") <= 1 && (!found || len(dir) < len(root)) {
			root, found = dir, true
		}
	}
	if !found {
		return nil, "技能包里没有 SKILL.md（放在压缩包根目录，或唯一的一级文件夹里）"
	}
	read := func(f *zip.File) ([]byte, bool) {
		rc, err := f.Open()
		if err != nil {
			return nil, false
		}
		defer rc.Close()
		// 不信 zip 头里声明的大小：多读一个字节判断是否超限，防压缩炸弹
		b, err := io.ReadAll(io.LimitReader(rc, skillPkgMaxFile+1))
		if err != nil || len(b) > skillPkgMaxFile {
			return nil, false
		}
		return b, true
	}
	var secrets []string
	var skillMD []byte
	type attach struct {
		rel  string
		body []byte
	}
	var attachments []attach
	draft := &skillPackageDraft{Included: []string{}, Skipped: []skillPackageSkip{}}
	for _, f := range zr.File {
		if f.FileInfo().IsDir() || !strings.HasPrefix(f.Name, root) {
			continue
		}
		rel := strings.TrimPrefix(f.Name, root)
		// macOS 压缩时带的元数据
		if strings.HasPrefix(rel, "__MACOSX/") || path.Base(rel) == ".DS_Store" {
			continue
		}
		if !registry.SafeRelPath(rel) {
			draft.Skipped = append(draft.Skipped, skillPackageSkip{Path: rel, Reason: "unsafe_path"})
			continue
		}
		if kind := registry.SecretFileKind(rel); kind != "" {
			secrets = append(secrets, rel+"（"+kind+"）")
			continue
		}
		b, ok := read(f)
		if !ok {
			if rel == "SKILL.md" {
				return nil, "SKILL.md 不能超过 64 KB"
			}
			draft.Skipped = append(draft.Skipped, skillPackageSkip{Path: rel, Reason: "too_large"})
			continue
		}
		// 文本：合法 UTF-8 且没有 NUL；图片、表格文件等二进制跳过
		if bytes.IndexByte(b, 0) >= 0 || !utf8.Valid(b) {
			draft.Skipped = append(draft.Skipped, skillPackageSkip{Path: rel, Reason: "binary"})
			continue
		}
		for _, hit := range registry.ScanText(rel, b) {
			secrets = append(secrets, rel+"（"+hit.Kind+"）")
		}
		if rel == "SKILL.md" {
			skillMD = b
		} else {
			attachments = append(attachments, attach{rel, b})
		}
	}
	if len(secrets) > 0 {
		slices.Sort(secrets)
		return nil, "技能包里疑似有密钥或凭据文件，删掉后再导入：" + strings.Join(slices.Compact(secrets), "、")
	}
	fm, body, err := parseSkillMD(string(skillMD))
	if err != nil {
		return nil, err.Error()
	}
	sort.Slice(attachments, func(i, j int) bool { return attachments[i].rel < attachments[j].rel })
	var b strings.Builder
	b.WriteString(body)
	for _, a := range attachments {
		lang := strings.TrimPrefix(strings.ToLower(path.Ext(a.rel)), ".")
		text := strings.TrimRight(string(a.body), "\n")
		fence := "```"
		for strings.Contains(text, fence) {
			fence += "`"
		}
		section := fmt.Sprintf("\n\n## 附件：%s\n\n%s%s\n%s\n%s", a.rel, fence, lang, text, fence)
		if b.Len()+len(section) > skillPkgMaxBody {
			draft.Skipped = append(draft.Skipped, skillPackageSkip{Path: a.rel, Reason: "over_limit"})
			continue
		}
		b.WriteString(section)
		draft.Included = append(draft.Included, a.rel)
	}
	if strings.TrimSpace(b.String()) == "" {
		return nil, "SKILL.md 的正文是空的"
	}
	title := strings.TrimSpace(fm.Title)
	if title == "" {
		title = strings.TrimSpace(fm.Name)
	}
	when := strings.TrimSpace(fm.WhenToUse)
	if when == "" {
		when = strings.TrimSpace(fm.WhenToUseDash)
	}
	model := true
	if fm.ModelInvocable != nil {
		model = *fm.ModelInvocable
	} else if fm.DisableModel != nil {
		model = !*fm.DisableModel
	}
	draft.Slug = skillSlug(fm.Name)
	draft.Spec = assetSpec{Title: truncateStr(title, 64), Description: truncateStr(strings.TrimSpace(fm.Description), 1000), WhenToUse: truncateStr(when, 500), Body: b.String(), ModelInvocable: &model}
	return draft, ""
}
