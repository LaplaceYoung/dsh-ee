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

// 网页上传的 .tgz 与 npm publish 的请求体上限都是 100 MB 的包，发布路径要把整包读进
// 内存做解包检查和入库；npm publish 的附件是 base64，解码后还会再占一份。几个大上传
// 同时进来就能把内存打满，所以两条入口共用这一个并发名额。
const maxConcurrentPackageUploads = 2

var packageUploadSlots = make(chan struct{}, maxConcurrentPackageUploads)

// beginPackageUpload 占一个上传名额。达到上限时立即失败、让调用方回 429，不在内存里
// 排队等；返回的 release 必须调用一次。
func beginPackageUpload() (release func(), ok bool) {
	select {
	case packageUploadSlots <- struct{}{}:
		return func() { <-packageUploadSlots }, true
	default:
		return nil, false
	}
}

// packageUploadBody 校验上传入口并给请求体加上 MaxTarball 上限。
//
// 只有 io.LimitReader 时只限制我们读多少，超限的另一端仍会继续发数据；http.MaxBytesReader
// 在读满上限后会让连接断开，配合 Content-Length 预检可以尽早回 413。
func (s *Server) packageUploadBody(w http.ResponseWriter, r *http.Request) (io.Reader, bool) {
	// 数据目录不可写/磁盘满时 newRegistry 返回 nil，但服务仍会启动；
	// 这里要按文档承诺降级成 503，而不是在 s.registry.MaxTarball 上 panic。
	if s.registry == nil {
		writeErr(w, http.StatusServiceUnavailable, "npm 源暂不可用：服务端数据目录未初始化或不可写")
		return nil, false
	}
	mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if mt != "application/octet-stream" && mt != "application/gzip" && mt != "application/x-gzip" {
		writeErr(w, http.StatusUnsupportedMediaType, "请求体必须是 .tgz（application/octet-stream）")
		return nil, false
	}
	// Content-Length 声明的长度已经超限时不必再读请求体，直接回 413。
	if r.ContentLength > s.registry.MaxTarball {
		writeErr(w, http.StatusRequestEntityTooLarge, "tarball 太大")
		return nil, false
	}
	return http.MaxBytesReader(w, r.Body, s.registry.MaxTarball), true
}

// writePackageReadErr 把读请求体的错误转成响应：超限 413，其余按读取失败处理。
func writePackageReadErr(w http.ResponseWriter, err error) bool {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		writeErr(w, http.StatusRequestEntityTooLarge, "tarball 太大")
		return false
	}
	writeErr(w, http.StatusBadRequest, "读取上传内容失败")
	return false
}

// readPackageBody 把上传的 tarball 读进内存。Content-Length 已知时按精确大小分配：
// io.ReadAll 扩容过程中会临时多占接近一份，100 MB 的包峰值会接近两倍。
// 分块上传（Content-Length 未知）退回有上限的 io.ReadAll。
func readPackageBody(w http.ResponseWriter, r *http.Request, body io.Reader) ([]byte, bool) {
	if r.ContentLength >= 0 {
		raw := make([]byte, r.ContentLength)
		if _, err := io.ReadFull(body, raw); err != nil {
			return nil, writePackageReadErr(w, err)
		}
		return raw, true
	}
	raw, err := io.ReadAll(body)
	if err != nil {
		return nil, writePackageReadErr(w, err)
	}
	return raw, true
}

// parsePackageTarball 解包并解析清单；失败时已经写好响应。
func parsePackageTarball(w http.ResponseWriter, tarball io.Reader) (*registry.TarballInfo, *registry.Manifest, bool) {
	info, err := registry.InspectTarball(tarball)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "不是 npm pack 生成的 .tgz："+err.Error())
		return nil, nil, false
	}
	m, err := registry.ParseManifest(info.PackageJSON)
	if err != nil || m.Name == "" || m.Version == "" {
		writeErr(w, http.StatusBadRequest, "tarball 里的 package.json 缺少 name 或 version")
		return nil, nil, false
	}
	return info, m, true
}

// readPackageTarball 读取并解包网页发布的 .tgz。失败时已经写好响应，ok 为 false。
// 发布需要原始字节（入库要算摘要），所以这条路径会把整包读进内存；预检走
// inspectPackageTarball，不保留整包副本。
func (s *Server) readPackageTarball(w http.ResponseWriter, r *http.Request) (raw []byte, info *registry.TarballInfo, m *registry.Manifest, ok bool) {
	body, ok := s.packageUploadBody(w, r)
	if !ok {
		return nil, nil, nil, false
	}
	raw, ok = readPackageBody(w, r, body)
	if !ok {
		return nil, nil, nil, false
	}
	info, m, ok = parsePackageTarball(w, bytes.NewReader(raw))
	if !ok {
		return nil, nil, nil, false
	}
	return raw, info, m, true
}

// inspectPackageTarball 读取并解包预检的 .tgz，但不把整包留在内存里：
// 边读边解包，内存只取决于解包检查的上限（单文件 ≤ 4 MB），与上传大小无关。
func (s *Server) inspectPackageTarball(w http.ResponseWriter, r *http.Request) (size int64, info *registry.TarballInfo, m *registry.Manifest, ok bool) {
	body, ok := s.packageUploadBody(w, r)
	if !ok {
		return 0, nil, nil, false
	}
	cr := &countingReader{r: body}
	info, err := registry.InspectTarball(cr)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeErr(w, http.StatusRequestEntityTooLarge, "tarball 太大")
			return 0, nil, nil, false
		}
		writeErr(w, http.StatusBadRequest, "不是 npm pack 生成的 .tgz："+err.Error())
		return 0, nil, nil, false
	}
	// gzip 读到流末尾就停，后面可能还有尾随字节；读完剩下的才能得到真实大小，
	// 也才能让 MaxBytesReader 发现超过上限的请求。
	if _, err := io.Copy(io.Discard, cr); err != nil {
		return 0, nil, nil, writePackageReadErr(w, err)
	}
	size = cr.n
	m, err = registry.ParseManifest(info.PackageJSON)
	if err != nil || m.Name == "" || m.Version == "" {
		writeErr(w, http.StatusBadRequest, "tarball 里的 package.json 缺少 name 或 version")
		return 0, nil, nil, false
	}
	return size, info, m, true
}

// countingReader 统计从请求体读了多少字节（含解包检查没消费的尾随数据）。
type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
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
	release, ok := beginPackageUpload()
	if !ok {
		writeErr(w, http.StatusTooManyRequests, "同时上传/预检的包太多，请稍后重试")
		return
	}
	defer release()
	size, info, m, ok := s.inspectPackageTarball(w, r)
	if !ok {
		return
	}
	set, err := s.appSettings(r.Context())
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	scopes := set.CompanyScopes()
	scan := registry.Check(m, info, size, scopes)
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
