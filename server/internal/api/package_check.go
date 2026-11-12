package api

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/hikariming/dsh-ee/server/internal/rbac"
	"github.com/hikariming/dsh-ee/server/internal/registry"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// readPackageTarball 读取并解包网页上传的 .tgz。失败时已经写好响应，ok 为 false。
// 发布和预检共用这一份解析，保证两条路径对同一个包给出同样的错误。
func (s *Server) readPackageTarball(w http.ResponseWriter, r *http.Request) (raw []byte, info *registry.TarballInfo, m *registry.Manifest, ok bool) {
	// 数据目录不可写/磁盘满时 newRegistry 返回 nil，但服务仍会启动；
	// 这里要按文档承诺降级成 503，而不是在 s.registry.MaxTarball 上 panic。
	if s.registry == nil {
		writeErr(w, http.StatusServiceUnavailable, "npm 源暂不可用：服务端数据目录未初始化或不可写")
		return nil, nil, nil, false
	}
	mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if mt != "application/octet-stream" && mt != "application/gzip" && mt != "application/x-gzip" {
		writeErr(w, http.StatusUnsupportedMediaType, "请求体必须是 .tgz（application/octet-stream）")
		return nil, nil, nil, false
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, s.registry.MaxTarball+1))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "读取上传内容失败")
		return nil, nil, nil, false
	}
	if int64(len(raw)) > s.registry.MaxTarball {
		writeErr(w, http.StatusRequestEntityTooLarge, "tarball 太大")
		return nil, nil, nil, false
	}
	info, err = registry.InspectTarball(bytes.NewReader(raw))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "不是 npm pack 生成的 .tgz："+err.Error())
		return nil, nil, nil, false
	}
	m, err = registry.ParseManifest(info.PackageJSON)
	if err != nil || m.Name == "" || m.Version == "" {
		writeErr(w, http.StatusBadRequest, "tarball 里的 package.json 缺少 name 或 version")
		return nil, nil, nil, false
	}
	return raw, info, m, true
}

// packageConflict 是预检发现的、会让 Publish 直接拒绝的问题。
// 只返回 key 和参数，文案由前端翻译；和 Check 的 ScanItem 同一套做法。
type packageConflict struct {
	Key    string         `json:"key"`
	Params map[string]any `json:"params,omitempty"`
}

type packageCheck struct {
	Name           string              `json:"name"`
	Version        string              `json:"version"`
	IsPlugin       bool                `json:"is_plugin"`
	CanSubmit      bool                `json:"can_submit"`
	WillPublish    bool                `json:"will_publish"`
	RequiresReview bool                `json:"requires_review"`
	Scan           []registry.ScanItem `json:"scan"`
	Conflicts      []packageConflict   `json:"conflicts"`
}

// POST /api/me/packages/check：发布前预检。只解包和检查，不落库，不占用版本号。
//
// 发布接口 POST /api/me/packages 会直接写库；而同一个版本号被驳回或下架后也不能重发。
// 作者在发布前需要看到 Check 的结果和发布冲突，否则一次失败就浪费一个版本号。
func (s *Server) handleCheckPackage(w http.ResponseWriter, r *http.Request) {
	raw, info, m, ok := s.readPackageTarball(w, r)
	if !ok {
		return
	}
	set, err := s.appSettings(r.Context())
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	scopes := set.CompanyScopes()
	scan := registry.Check(m, info, int64(len(raw)), scopes)
	conflicts, err := s.packageConflicts(r.Context(), m, currentUser(r), currentAccess(r).Has(rbac.PluginsManage), scopes)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	canSubmit := len(conflicts) == 0
	scanFailed := false
	for _, item := range scan {
		if item.Level == registry.LevelFail {
			scanFailed = true
			break
		}
	}
	// 三种结果对应 Publish 的行为：冲突直接拒绝；open 且无失败项直接上架；其余进审核。
	willPublish := canSubmit && !scanFailed && set.AdmissionMode == "open"
	writeJSON(w, http.StatusOK, packageCheck{
		Name:           m.Name,
		Version:        m.Version,
		IsPlugin:       m.IsPlugin,
		CanSubmit:      canSubmit,
		WillPublish:    willPublish,
		RequiresReview: canSubmit && !willPublish,
		Scan:           scan,
		Conflicts:      conflicts,
	})
}

// packageConflicts 按 Publish 的规则逐项检查：包名、版本号、企业作用域、同名包、归属和版本占用。
func (s *Server) packageConflicts(ctx context.Context, m *registry.Manifest, user *store.User, manager bool, scopes []string) ([]packageConflict, error) {
	out := []packageConflict{}
	validName := registry.ValidPublishName(m.Name)
	if !validName {
		out = append(out, packageConflict{Key: "name", Params: map[string]any{"name": m.Name}})
	} else if !registry.InScopes(m.Name, scopes) {
		out = append(out, packageConflict{Key: "scope", Params: map[string]any{"scope": scopeConflictParam(scopes)}})
	}
	if !registry.ValidVersion(m.Version) {
		out = append(out, packageConflict{Key: "version", Params: map[string]any{"version": m.Version}})
	}
	if !validName {
		return out, nil
	}

	pkg, err := s.store.GetPackage(ctx, m.Name)
	switch {
	case err == nil:
		if pkg.Origin != store.OriginHosted {
			out = append(out, packageConflict{Key: "upstream", Params: map[string]any{"name": m.Name}})
		} else if pkg.OwnerID != nil && user != nil && *pkg.OwnerID != user.ID && !manager {
			out = append(out, packageConflict{Key: "owner", Params: map[string]any{"name": m.Name}})
		}
	case errors.Is(err, store.ErrNotFound):
	default:
		return nil, err
	}

	if !registry.ValidVersion(m.Version) {
		return out, nil
	}
	if _, err := s.store.GetVersion(ctx, m.Name, m.Version); err == nil {
		out = append(out, packageConflict{Key: "exists", Params: map[string]any{"name": m.Name, "version": m.Version}})
	} else if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	return out, nil
}

// scopeConflictParam 与 registry.Publish 拒绝文案里的作用域列表一致：全部作用域用顿号拼接。
// 空列表时返回空串，避免直接取 scopes[0] 越界 panic。
func scopeConflictParam(scopes []string) string {
	return strings.Join(scopes, "、")
}
