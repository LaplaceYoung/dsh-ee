package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// DSH「自定义插件索引源」format 1：员工在 DSH 插件页添加 <dsh-ee>/index/dsh-index.json，
// 就能浏览、安装企业已上架的插件；安装时 DSH 按 npm 源（.npmrc 指向 dsh-ee）下载。
//
// 口径与 dshfind 的 scripts/lib/dsh-index.mjs 一致：读端遇到任何一条违规就整份拒收，
// 所以每个字段先按读端规则过一遍，过不了的字段省略、过不了的条目整条跳过。

var categoryTitles = map[string]map[string]string{
	"skin":     {"en": "Skins & Themes", "zh": "皮肤主题"},
	"ui":       {"en": "Panels & Views", "zh": "面板增强"},
	"agent":    {"en": "Agent Boosters", "zh": "Agent 增强"},
	"memory":   {"en": "Memory & Context", "zh": "记忆上下文"},
	"client":   {"en": "Clients", "zh": "客户端"},
	"channel":  {"en": "Channels & Notify", "zh": "通道通知"},
	"tools":    {"en": "Tools & Integrations", "zh": "工具集成"},
	"fun":      {"en": "Fun", "zh": "趣味互动"},
	"resource": {"en": "Resources", "zh": "资源导航"},
}

var (
	indexLanguageID = regexp.MustCompile(`^[a-z]{2,8}(?:-[a-z0-9]{1,8})*$`)
	pathSegment     = regexp.MustCompile(`^[A-Za-z0-9._@+-]+$`)
)

const maxPublishTimestamp = 1e11

type indexRepo struct {
	URL       string `json:"url"`
	Directory string `json:"directory,omitempty"`
}

type indexVersion struct {
	Version          string            `json:"version"`
	PublishTimestamp int64             `json:"publishTimestamp"`
	Source           map[string]string `json:"source"`
	// 版本声明的 DSH 兼容范围；DSH 用它过滤不可安装的版本，缺省表示任意版本
	Engines *indexEngines `json:"engines,omitempty"`
}

type indexEngines struct {
	Dsh string `json:"dsh,omitempty"`
}

type indexMeta struct {
	Category string         `json:"category,omitempty"`
	Labels   []string       `json:"labels,omitempty"`
	SortKeys map[string]int `json:"sortKeys,omitempty"`
}

type IndexPlugin struct {
	Name        string         `json:"name"`
	Title       string         `json:"title,omitempty"`
	Description string         `json:"description,omitempty"`
	Author      string         `json:"author,omitempty"`
	Icon        string         `json:"icon,omitempty"`
	Homepage    string         `json:"homepage,omitempty"`
	Repository  *indexRepo     `json:"repository,omitempty"`
	Metadata    *indexMeta     `json:"metadata,omitempty"`
	Versions    []indexVersion `json:"versions"`
}

type Index struct {
	Format      int               `json:"format"`
	Title       map[string]string `json:"title"`
	Description map[string]string `json:"description"`
	Metadata    map[string]any    `json:"metadata"`
	Plugins     []IndexPlugin     `json:"plugins"`
}

func titled(en, zh string) map[string]any {
	return map[string]any{"title": map[string]string{"en": en, "zh": zh}}
}

// BuildIndex 用所有已上架的插件版本生成索引。portalBase 是门户地址，插件主页链回门户详情页。
func (s *Service) BuildIndex(ctx context.Context, portalBase string) (*Index, error) {
	set, err := s.settings(ctx)
	if err != nil {
		return nil, err
	}
	vs, err := s.store.PublishedPluginVersions(ctx)
	if err != nil {
		return nil, err
	}
	byName := map[string][]*store.NpmVersion{}
	var names []string
	for _, v := range vs {
		if _, ok := byName[v.Name]; !ok {
			names = append(names, v.Name)
		}
		byName[v.Name] = append(byName[v.Name], v)
	}
	pkgs, err := s.store.ListPackages(ctx, store.PackageFilter{Names: names})
	if err != nil {
		return nil, err
	}
	installs, err := s.store.InstallCounts(ctx, names)
	if err != nil {
		return nil, err
	}

	org := strings.TrimSpace(set.OrgName)
	idx := &Index{
		Format:      1,
		Title:       map[string]string{"zh": "企业能力市场", "en": "Enterprise capability market"},
		Description: map[string]string{"zh": "由 dsh-ee 审核上架的插件：企业内部发布的能力与经过审核的社区插件。", "en": "Plugins approved in dsh-ee: capabilities published inside the company and reviewed community plugins."},
		Metadata: map[string]any{
			"categories": func() map[string]any {
				out := map[string]any{}
				for id, t := range categoryTitles {
					out[id] = map[string]any{"title": t}
				}
				return out
			}(),
			"labels": map[string]any{
				"internal": titled("Internal", "企业内部"),
				"featured": titled("Recommended", "推荐"),
			},
			"sortKeys": map[string]any{
				"installs": map[string]any{"title": map[string]string{"en": "Installs in company", "zh": "企业内安装量"}, "order": "desc"},
				"stars":    map[string]any{"title": map[string]string{"en": "GitHub stars", "zh": "GitHub Star"}, "order": "desc"},
				"score":    map[string]any{"title": map[string]string{"en": "dshfind score", "zh": "dshfind 评分"}, "order": "desc"},
			},
		},
		Plugins: []IndexPlugin{},
	}
	if org != "" {
		idx.Title = map[string]string{"zh": org + " · 企业能力市场", "en": org + " · Capability market"}
	}

	// 企业内部的排前面，再按安装量
	slices.SortStableFunc(pkgs, func(a, b *store.NpmPackage) int {
		if (a.Origin == store.OriginHosted) != (b.Origin == store.OriginHosted) {
			if a.Origin == store.OriginHosted {
				return -1
			}
			return 1
		}
		return installs[b.Name] - installs[a.Name]
	})
	for _, p := range pkgs {
		if ip := indexPlugin(p, byName[p.Name], installs[p.Name], portalBase); ip != nil {
			idx.Plugins = append(idx.Plugins, *ip)
		}
	}
	if err := AssertIndex(idx); err != nil {
		return nil, err
	}
	return idx, nil
}

// manifestEngines 从版本 package.json 读 engines.dsh。索引读端要求它是合法 SemVer 范围，
// 解析不了就省略（读端把缺省当成任意版本），避免一条坏范围让整份索引被拒收。
func manifestEngines(raw json.RawMessage) *indexEngines {
	if len(raw) == 0 {
		return nil
	}
	var m struct {
		Engines map[string]string `json:"engines"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	r := strings.TrimSpace(m.Engines["dsh"])
	if r == "" || strings.HasPrefix(r, "workspace:") {
		return nil
	}
	if _, err := semver.NewConstraint(r); err != nil {
		return nil
	}
	return &indexEngines{Dsh: r}
}

func indexPlugin(p *store.NpmPackage, versions []*store.NpmVersion, installs int, portalBase string) *IndexPlugin {
	if !ValidPublishName(p.Name) || strings.HasPrefix(p.Name, "@deepseek-ai/") {
		return nil
	}
	ip := &IndexPlugin{Name: p.Name}
	if t := plainText(p.Title); t != "" {
		ip.Title = t
	}
	if d := truncateRunes(plainText(p.Description), 400); d != "" {
		ip.Description = d
	}
	if a := plainText(p.Author); a != "" {
		ip.Author = a
	}
	if icon := iconURLFor(p.Icon, portalBase); icon != "" {
		ip.Icon = icon
	}
	if portalBase != "" {
		ip.Homepage = portalBase + "/plugins/" + url.PathEscape(p.Name)
	} else if home := httpURL(p.Homepage); home != "" {
		ip.Homepage = home
	}
	if repo := httpURL(p.Repository); repo != "" {
		ip.Repository = &indexRepo{URL: repo}
	}
	meta := &indexMeta{SortKeys: map[string]int{"installs": installs}}
	if _, ok := categoryTitles[p.Category]; ok {
		meta.Category = p.Category
	}
	if p.Origin == store.OriginHosted {
		meta.Labels = append(meta.Labels, "internal")
	}
	if p.Featured {
		meta.Labels = append(meta.Labels, "featured")
	}
	if p.UpstreamStars != nil {
		meta.SortKeys["stars"] = *p.UpstreamStars
	}
	if p.UpstreamScore != nil {
		meta.SortKeys["score"] = *p.UpstreamScore
	}
	ip.Metadata = meta

	for _, v := range versions {
		if !ValidVersion(v.Version) {
			continue
		}
		t := v.CreatedAt
		if v.PublishedAt != nil {
			t = *v.PublishedAt
		}
		ts := t.Unix()
		if ts < 0 || ts >= maxPublishTimestamp {
			ts = time.Now().Unix()
		}
		src := map[string]string{"type": "npm"}
		// 写明从 dsh-ee 的 npm 源装：DSH 安装时先问它，不依赖员工本机 .npmrc 的默认源
		if portalBase != "" {
			src["registry"] = portalBase + "/npm/"
		}
		ip.Versions = append(ip.Versions, indexVersion{Version: v.Version, PublishTimestamp: ts, Source: src, Engines: manifestEngines(v.Manifest)})
	}
	if len(ip.Versions) == 0 {
		return nil
	}
	// 新版本在前
	slices.SortFunc(ip.Versions, func(a, b indexVersion) int { return compareVersions(b.Version, a.Version) })
	return ip
}

// AssertIndex 是生成后的自检，规则与读端一致；任何一条不过就不输出，免得 DSH 整份拒收。
func AssertIndex(idx *Index) error {
	fail := func(path, reason string) error { return fmt.Errorf("dsh-index %s: %s", path, reason) }
	for path, text := range map[string]map[string]string{"$.title": idx.Title, "$.description": idx.Description} {
		for lang, s := range text {
			if !indexLanguageID.MatchString(lang) || strings.TrimSpace(s) == "" {
				return fail(path+"."+lang, "bad localized text")
			}
		}
	}
	names := map[string]bool{}
	for i, p := range idx.Plugins {
		at := fmt.Sprintf("$.plugins[%d]", i)
		if !ValidPublishName(p.Name) {
			return fail(at+".name", p.Name)
		}
		if names[p.Name] {
			return fail(at+".name", "duplicate "+p.Name)
		}
		names[p.Name] = true
		if p.Metadata != nil && p.Metadata.Category != "" {
			if _, ok := categoryTitles[p.Metadata.Category]; !ok {
				return fail(at+".metadata.category", p.Metadata.Category)
			}
		}
		if p.Repository != nil && p.Repository.Directory != "" {
			for _, seg := range strings.Split(p.Repository.Directory, "/") {
				if seg == "." || seg == ".." || !pathSegment.MatchString(seg) {
					return fail(at+".repository.directory", p.Repository.Directory)
				}
			}
		}
		if len(p.Versions) == 0 {
			return fail(at+".versions", "empty")
		}
		for _, v := range p.Versions {
			if !ValidVersion(v.Version) {
				return fail(at+".versions", v.Version)
			}
			if v.PublishTimestamp < 0 || v.PublishTimestamp >= maxPublishTimestamp {
				return fail(at+".versions", fmt.Sprintf("publishTimestamp %d", v.PublishTimestamp))
			}
		}
	}
	return nil
}
