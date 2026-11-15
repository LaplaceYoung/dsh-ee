package api

import (
	"context"
	"crypto/sha512"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/store"
	"gopkg.in/yaml.v3"
)

const dshReleaseHost = "download.deepseek.com"

// dshReleaseTimeout 是单个安装包从建连到读完全部内容的硬上限。官方安装包接近 1 GB，
// 跨境镜像可能很慢，所以留足 45 分钟；HTTP 客户端的 Timeout 与后台下载任务的 context
// 超时共用这个值，避免上游建连后挂起时永久占住 goroutine、让 DB 一直停在 downloading。
const dshReleaseTimeout = 45 * time.Minute

// maxDshReleaseBytes 是没有清单大小可依时的下载硬上限。官方 DMG 固定没有 size/sha512，
// 只能按绝对上限兜底，防止上游行为异常或清单被换时把磁盘写满；2 GiB 远高于安装包实际大小。
// 定义为变量是为了让单测能临时改小上限来覆盖超限分支。
var maxDshReleaseBytes int64 = 2 << 30

var (
	dshReleaseFeedURLs = map[string]string{
		"mac-arm64": "https://download.deepseek.com/dsh-desk/feeds/mac-arm64/nightly-mac.yml",
		"mac-x64":   "https://download.deepseek.com/dsh-desk/feeds/mac-x64/nightly-mac.yml",
		"win-x64":   "https://download.deepseek.com/dsh-desk/feeds/win-x64/nightly.yml",
	}
	// DMG 是 macOS 专有的补充安装包（官方不提供哈希），没有对应 feed 的平台不会出现在这里。
	dshReleaseDMGURLs = map[string]string{
		"mac-arm64": "https://download.deepseek.com/desktop/dsh-latest-macos-arm64.dmg",
		"mac-x64":   "https://download.deepseek.com/desktop/dsh-latest-macos-x64.dmg",
	}
)

// releaseKind 按官方安装包文件名判断类型：Windows 是 exe，macOS 是 zip。
// 认不出来的后缀按 zip 处理，免得新平台漏进来时整批同步失败。
func releaseKind(file string) string {
	switch strings.ToLower(path.Ext(file)) {
	case ".exe", ".msi":
		return "exe"
	}
	return "zip"
}

func (s *Server) initDshReleases() {
	s.releaseFeeds = map[string]string{}
	s.releaseDMGs = map[string]string{}
	for target, raw := range dshReleaseFeedURLs {
		s.releaseFeeds[target] = raw
		s.releaseDMGs[target] = dshReleaseDMGURLs[target]
	}
	s.releaseKick = make(chan struct{}, 1)
	s.releaseJobs = map[string]bool{}
	s.releaseHTTP = &http.Client{
		Transport: s.opts.DemoSite.guard(&http.Transport{Proxy: http.ProxyFromEnvironment, ResponseHeaderTimeout: time.Minute, IdleConnTimeout: 90 * time.Second}),
		// 总超时：大文件下载需要余量，但必须能结束，不能只靠 ResponseHeaderTimeout。
		Timeout: dshReleaseTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if !s.releaseURLAllowed(req.URL.String()) {
				return fmt.Errorf("拒绝重定向到非官方地址：%s", req.URL.Redacted())
			}
			if len(via) >= 10 {
				return errors.New("重定向次数过多")
			}
			return nil
		},
	}
}

func (s *Server) releaseURLAllowed(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.Host == "" {
		return false
	}
	if u.Scheme == "https" && strings.EqualFold(u.Hostname(), dshReleaseHost) {
		return true
	}
	for _, feeds := range []map[string]string{s.releaseFeeds, s.releaseDMGs} {
		for _, feed := range feeds {
			base, err := url.Parse(feed)
			if err == nil && u.Scheme == base.Scheme && u.Host == base.Host && (u.Scheme == "https" || u.Scheme == "http") {
				return true
			}
		}
	}
	return false
}

type dshManifest struct {
	Version string `yaml:"version"`
	Files   []struct {
		URL    string `yaml:"url"`
		SHA512 string `yaml:"sha512"`
		Size   int64  `yaml:"size"`
	} `yaml:"files"`
	Path        string `yaml:"path"`
	SHA512      string `yaml:"sha512"`
	ReleaseDate string `yaml:"releaseDate"`
}

func (s *Server) fetchDshManifest(ctx context.Context, target string) (*dshManifest, error) {
	feed := s.releaseFeeds[target]
	if !s.releaseURLAllowed(feed) {
		return nil, errors.New("清单地址不在官方白名单")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feed, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.releaseHTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("清单返回 HTTP %d", resp.StatusCode)
	}
	var m dshManifest
	if err := yaml.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&m); err != nil {
		return nil, fmt.Errorf("解析清单：%w", err)
	}
	return &m, nil
}

// assetFromManifest 从清单生成「官方给了哈希」的安装包：macOS 是 zip，Windows 是 exe。
// 类型按安装包文件名后缀判断，不写死，加了新平台不用再改这里。
func (s *Server) assetFromManifest(target string, m *dshManifest) (*store.DshRelease, error) {
	if m.Version == "" || m.ReleaseDate == "" {
		return nil, errors.New("清单缺少版本或发布日期")
	}
	releasedAt, err := time.Parse(time.RFC3339, m.ReleaseDate)
	if err != nil {
		return nil, fmt.Errorf("发布日期格式错误：%w", err)
	}
	r := &store.DshRelease{Target: target, Version: m.Version, ReleasedAt: releasedAt, State: store.ReleaseAvailable}
	if len(m.Files) > 0 {
		r.UpstreamURL, r.SHA512, r.Size = m.Files[0].URL, m.Files[0].SHA512, m.Files[0].Size
	}
	if r.UpstreamURL == "" {
		r.UpstreamURL = m.Path
	}
	if r.SHA512 == "" {
		r.SHA512 = m.SHA512
	}
	if !s.releaseURLAllowed(r.UpstreamURL) || r.Size <= 0 {
		return nil, errors.New("清单中的安装包地址或大小无效")
	}
	sum, err := base64.StdEncoding.DecodeString(r.SHA512)
	if err != nil || len(sum) != sha512.Size {
		return nil, errors.New("清单中的 sha512 不是有效的 base64 哈希")
	}
	u, _ := url.Parse(r.UpstreamURL)
	r.File = path.Base(u.Path)
	r.Kind = releaseKind(r.File)
	if _, err := s.releasePartPath(r); err != nil {
		return nil, err
	}
	return r, nil
}

func (s *Server) dmgFromManifest(target string, m *dshManifest) (*store.DshRelease, error) {
	if s.releaseDMGs[target] == "" {
		return nil, errors.New("该平台没有官方 DMG")
	}
	r := &store.DshRelease{Target: target, Kind: "dmg", Version: m.Version, State: store.ReleaseAvailable, UpstreamURL: s.releaseDMGs[target]}
	if !s.releaseURLAllowed(r.UpstreamURL) {
		return nil, errors.New("DMG 地址不在官方白名单")
	}
	asset, err := s.assetFromManifest(target, m)
	if err != nil {
		return nil, err
	}
	r.ReleasedAt = asset.ReleasedAt
	// 官方 DMG 地址固定不变，本地文件名带上版本号，免得新版本覆盖旧版本。
	r.File = fmt.Sprintf("deepseek-harness-%s-%s.dmg", m.Version, target)
	if _, err := s.releasePartPath(r); err != nil {
		return nil, err
	}
	return r, nil
}

func (s *Server) upsertDshRelease(ctx context.Context, r *store.DshRelease) error {
	old, err := s.store.DshReleaseByVersion(ctx, r.Target, r.Kind, r.Version)
	if errors.Is(err, store.ErrNotFound) {
		return s.store.CreateDshRelease(ctx, r)
	}
	if err != nil {
		return err
	}
	r.ID = old.ID
	return s.store.UpdateDshReleaseMetadata(ctx, r)
}

func (s *Server) syncDshReleases(ctx context.Context) error {
	set, err := s.appSettings(ctx)
	if err != nil {
		return err
	}
	if !set.Distribution.Enabled || set.Distribution.AutoSync == "off" {
		return nil
	}
	targets := make([]string, 0, len(s.releaseFeeds))
	for target := range s.releaseFeeds {
		targets = append(targets, target)
	}
	sort.Strings(targets)
	var errs []error
	for _, target := range targets {
		m, err := s.fetchDshManifest(ctx, target)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s：%w", target, err))
			continue
		}
		asset, err := s.assetFromManifest(target, m)
		if err == nil {
			err = s.upsertDshRelease(ctx, asset)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("%s 安装包：%w", target, err))
		}
		if set.Distribution.OfferDmg && s.releaseDMGs[target] != "" {
			dmg, dmgErr := s.dmgFromManifest(target, m)
			if dmgErr == nil {
				dmgErr = s.upsertDshRelease(ctx, dmg)
			}
			if dmgErr != nil {
				errs = append(errs, fmt.Errorf("%s dmg：%w", target, dmgErr))
			}
		}
	}
	if len(errs) > 0 || set.Distribution.AutoSync != "mirror" {
		return errors.Join(errs...)
	}
	for _, target := range targets {
		items, err := s.store.ListDshReleases(ctx, target)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		// 自动镜像只下载每种安装包的最新版本；被清理或删除（removed）的旧版本不会被拉回来。
		newest := map[string]time.Time{}
		for _, item := range items {
			if item.ReleasedAt.After(newest[item.Kind]) {
				newest[item.Kind] = item.ReleasedAt
			}
		}
		for _, item := range items {
			if (item.State == store.ReleaseAvailable || item.State == store.ReleaseFailed) && item.ReleasedAt.Equal(newest[item.Kind]) {
				if err := s.downloadDshRelease(ctx, item.ID); err != nil {
					errs = append(errs, err)
				}
			}
		}
	}
	return errors.Join(errs...)
}

func (s *Server) releasePartPath(r *store.DshRelease) (string, error) {
	if _, ok := s.releaseFeeds[r.Target]; !ok || r.File == "" || r.File == "." || r.File == ".." || path.Base(r.File) != r.File || strings.Contains(r.File, `\`) {
		return "", errors.New("安装包 target 或文件名无效")
	}
	return filepath.Join(s.opts.DataDir, "dsh-releases", r.Target, r.File+".part"), nil
}

func (s *Server) downloadDshRelease(ctx context.Context, id int64) error {
	r, err := s.store.DshReleaseByID(ctx, id)
	if err != nil {
		return err
	}
	s.releaseMu.Lock()
	if s.releaseJobs[r.Target] {
		s.releaseMu.Unlock()
		return nil
	}
	s.releaseJobs[r.Target] = true
	s.releaseMu.Unlock()
	defer func() {
		s.releaseMu.Lock()
		delete(s.releaseJobs, r.Target)
		s.releaseMu.Unlock()
	}()
	if r.State == store.ReleaseMirrored || r.State == store.ReleaseDownloading {
		return nil
	}
	if err := s.store.UpdateDshReleaseState(ctx, r.ID, store.ReleaseDownloading, "", ""); err != nil {
		return err
	}
	size, sum, localPath, err := s.downloadDshReleaseFile(ctx, r)
	if err != nil {
		_ = s.store.UpdateDshReleaseState(context.WithoutCancel(ctx), r.ID, store.ReleaseFailed, "", err.Error())
		s.auditSystem(ctx, auditDshReleaseMirrorFailed, r.Target+"/"+r.File, map[string]any{"version": r.Version, "error": err.Error()})
		return err
	}
	if err := s.store.FinishDshRelease(ctx, r.ID, size, sum, localPath); err != nil {
		return err
	}
	set, err := s.appSettings(ctx)
	if err != nil {
		return err
	}
	return s.cleanupDshReleases(ctx, r.Target, set.Distribution.KeepVersions)
}

// copyReleaseBody 把安装包内容从 src 拷到 dst，返回实际写入的字节数，并保证不会无限写盘。
// wantSize > 0（zip/exe，清单给了大小）时最多读 wantSize+1 字节：多一个字节就足以判定
// 上游返回的内容比清单大，小于 wantSize 同样报错。
// wantSize == 0（官方 DMG 没有清单大小）时最多读 maxDshReleaseBytes 作为绝对上限。
func copyReleaseBody(dst io.Writer, src io.Reader, wantSize int64) (int64, error) {
	limit := maxDshReleaseBytes + 1
	if wantSize > 0 {
		limit = wantSize + 1
	}
	n, err := io.Copy(dst, io.LimitReader(src, limit))
	if err != nil {
		return n, err
	}
	if wantSize > 0 {
		if n > wantSize {
			return n, fmt.Errorf("上游返回的内容比清单声明的 %d 字节多", wantSize)
		}
		if n < wantSize {
			return n, fmt.Errorf("安装包大小不一致：清单 %d，实际 %d", wantSize, n)
		}
		return n, nil
	}
	if n > maxDshReleaseBytes {
		return n, fmt.Errorf("安装包超过 %d 字节上限（清单未声明大小）", maxDshReleaseBytes)
	}
	return n, nil
}

func (s *Server) downloadDshReleaseFile(ctx context.Context, r *store.DshRelease) (size int64, sum, localPath string, retErr error) {
	if !s.releaseURLAllowed(r.UpstreamURL) {
		return 0, "", "", errors.New("安装包地址不在官方白名单")
	}
	part, err := s.releasePartPath(r)
	if err != nil {
		return 0, "", "", err
	}
	localPath = strings.TrimSuffix(part, ".part")
	if err := os.MkdirAll(filepath.Dir(part), 0o700); err != nil {
		return 0, "", "", err
	}
	if err := os.Remove(part); err != nil && !errors.Is(err, os.ErrNotExist) {
		return 0, "", "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.UpstreamURL, nil)
	if err != nil {
		return 0, "", "", err
	}
	resp, err := s.releaseHTTP.Do(req)
	if err != nil {
		return 0, "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, "", "", fmt.Errorf("安装包返回 HTTP %d", resp.StatusCode)
	}
	f, err := os.OpenFile(part, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, "", "", err
	}
	defer func() {
		_ = f.Close()
		if retErr != nil {
			_ = os.Remove(part)
		}
	}()
	h := sha512.New()
	size, err = copyReleaseBody(f, io.TeeReader(resp.Body, h), r.Size)
	if err != nil {
		return 0, "", "", err
	}
	if err := f.Sync(); err != nil {
		return 0, "", "", err
	}
	if err := f.Close(); err != nil {
		return 0, "", "", err
	}
	actual := h.Sum(nil)
	if r.SHA512 == "" {
		// 官方 DMG 没有哈希：镜像行为保持不变，但必须让管理员看见「装完无法验证内容」。
		s.log.Warn("安装包没有可校验的哈希，镜像后无法验证内容", "target", r.Target, "version", r.Version)
	} else {
		expected, err := base64.StdEncoding.DecodeString(r.SHA512)
		if err != nil || len(expected) != sha512.Size {
			return 0, "", "", errors.New("清单 sha512 无效")
		}
		if base64.StdEncoding.EncodeToString(actual) != base64.StdEncoding.EncodeToString(expected) {
			return 0, "", "", errors.New("安装包 sha512 校验不一致")
		}
	}
	if err := os.Rename(part, localPath); err != nil {
		return 0, "", "", err
	}
	return size, base64.StdEncoding.EncodeToString(actual), localPath, nil
}

func (s *Server) recoverDshReleases(ctx context.Context) error {
	items, err := s.store.ListDshReleases(ctx, "")
	if err != nil {
		return err
	}
	for _, r := range items {
		if r.State != store.ReleaseDownloading {
			continue
		}
		if part, err := s.releasePartPath(r); err == nil {
			_ = os.Remove(part)
		}
	}
	return s.store.RecoverDshReleases(ctx)
}

func (s *Server) cleanupDshReleases(ctx context.Context, target string, keep int) error {
	if keep < 1 {
		keep = 1
	}
	if keep > 10 {
		keep = 10
	}
	items, err := s.store.ListDshReleases(ctx, target)
	if err != nil {
		return err
	}
	// zip 与 dmg 各自保留 keep 个。
	byKind := map[string][]*store.DshRelease{}
	for _, r := range items {
		if r.State == store.ReleaseMirrored {
			byKind[r.Kind] = append(byKind[r.Kind], r)
		}
	}
	for _, mirrored := range byKind {
		if err := s.pruneDshReleases(ctx, mirrored, keep); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) pruneDshReleases(ctx context.Context, mirrored []*store.DshRelease, keep int) error {
	sort.Slice(mirrored, func(i, j int) bool {
		if mirrored[i].ReleasedAt.Equal(mirrored[j].ReleasedAt) {
			return mirrored[i].ID > mirrored[j].ID
		}
		return mirrored[i].ReleasedAt.After(mirrored[j].ReleasedAt)
	})
	for i, r := range mirrored {
		if i < keep || r.Recommended {
			continue
		}
		if r.LocalPath != "" {
			if err := os.Remove(r.LocalPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
		if err := s.store.ResetDshRelease(ctx, r.ID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) kickDshReleases() {
	select {
	case s.releaseKick <- struct{}{}:
	default:
	}
}

func (s *Server) runDshReleases(ctx context.Context) {
	// 启动恢复也走租约：别的实例正在下载时不要动它的 .part 文件和 downloading 状态。
	// 进程被强杀后旧租约要等 TTL 到期，所以这里重试几次再放弃（下一轮 6 小时后仍会同步）。
	if !s.runLeasedRetry(ctx, leaseDshReleases, func(ctx context.Context) {
		if err := s.recoverDshReleases(ctx); err != nil {
			s.log.Warn("恢复 Desktop 下载任务失败", "err", err)
		}
	}) && ctx.Err() == nil {
		s.log.Debug("跳过 Desktop 下载任务的启动恢复（租约由其他实例持有或数据库不可用）")
	}
	check := func(ctx context.Context) {
		s.runLeased(ctx, leaseDshReleases, func(ctx context.Context) {
			if err := s.syncDshReleases(ctx); err != nil {
				s.log.Warn("同步 Desktop 版本失败", "err", err)
			}
		})
	}
	timer := time.NewTimer(time.Minute)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.releaseKick:
			check(ctx)
		case <-timer.C:
			check(ctx)
			timer.Reset(6 * time.Hour)
		}
	}
}
