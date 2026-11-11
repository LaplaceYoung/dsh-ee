package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/store"
)

// 公开演示站（DSH_EE_DEMO_MODE）：所有访客共用初始管理员账号，数据由 deploy/demo 的脚本定期清空重来。
// 这里只防两类事：一是有人把别人锁在外面（改密码、停用账号、改角色、接企业目录），
// 二是有人把演示站当跳板或网盘（对外请求、上传 npm 包、同步上游）。其余随便改，重置后就回来了。

// DemoSite 是演示站的配置；Server 的 opts.DemoSite 为 nil 表示正常部署。
type DemoSite struct {
	// 登录页上显示的演示账号
	Username string
	Password string
	// 给访客看的重置周期说明，如「每 6 小时」；空则不显示
	Reset string
	// 实际的重置间隔，和 crontab 一致（如 6h 对应 0 */6 * * *）；用来给访客算下次重置时间。0 表示不算
	ResetEvery time.Duration
	// 介绍视频所在目录（intro-zh.mp4、intro-en.mp4），首页播放；空则不显示
	VideoDir string
	// 访问统计追加到这个文件，每行一个 JSON；空则不记。放在数据目录和日志轮换之外，重置不清它
	StatsFile string
	// dsh-ee 自己监听的端口：对外请求只许连本机的这个端口（示例连接就是连自己）
	selfPort string
}

// demoSiteBlocked 是演示站里关掉的接口（按路由表里的 pattern）。没列出的照常可用：
// 建应用、知识库上传、数据中台上传 CSV、管理模式这些正是要演示的，数据都在自己库里，重置即清。
var demoSiteBlocked = map[string]bool{
	// 会把别人锁在外面
	"POST /api/auth/password":         true,
	"PATCH /api/admin/users/{id}":     true,
	"PUT /api/admin/users/{id}/roles": true,
	"PATCH /api/admin/roles/{id}":     true,
	"DELETE /api/admin/roles/{id}":    true,
	"PUT /api/admin/auth/ldap":        true,
	"POST /api/admin/auth/ldap/test":  true,
	"POST /api/admin/auth/ldap/sync":  true,
	"PUT /api/admin/auth/oidc":        true,
	"POST /api/admin/auth/oidc/test":  true,
	"PUT /api/admin/brand/logo":       true,
	"POST /api/admin/ee-jwt/rotate":   true,
	// 模型接入：访客填进来的 key 会被别人拿来用
	"POST /api/admin/providers":            true,
	"PATCH /api/admin/providers/{id}":      true,
	"POST /api/admin/providers/{id}/check": true,
	// 上传文件、从外网拉东西
	"POST /api/me/packages":                    true,
	"POST /api/agent/packages":                 true,
	"POST /api/me/import-requests":             true,
	"POST /api/admin/upstream/sync":            true,
	"POST /api/admin/upstream/check-updates":   true,
	"POST /api/admin/upstream/import":          true,
	"POST /api/admin/dsh-releases/check":       true,
	"POST /api/admin/dsh-releases/{id}/mirror": true,
}

const demoSiteMsg = "演示站不开放这个操作。想完整体验，可以自己部署一套：见 deploy/README.md"

func demoSiteRefuse(w http.ResponseWriter, r *http.Request) {
	writeErr(w, http.StatusForbidden, demoSiteMsg)
}

// demoSiteNpmWrite：npm 源在演示站只读（不许 npm publish / login）。
func (s *Server) demoSiteNpmWrite(w http.ResponseWriter, r *http.Request) bool {
	if s.opts.DemoSite == nil || r.Method == http.MethodGet || r.Method == http.MethodHead {
		return false
	}
	demoSiteRefuse(w, r)
	return true
}

var errDemoSiteDial = errors.New("演示站不允许访问外部地址")

// guard 让对外请求只能连 dsh-ee 自己：按真正要连的 IP 判断（DNS 解析之后），代理也一样被拦。
// 正常部署（d 为 nil）原样返回。
func (d *DemoSite) guard(t *http.Transport) *http.Transport {
	if d == nil {
		return t
	}
	dialer := &net.Dialer{
		Timeout: 10 * time.Second,
		Control: func(network, address string, _ syscall.RawConn) error {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return errDemoSiteDial
			}
			ip := net.ParseIP(host)
			if ip == nil || !ip.IsLoopback() || (d.selfPort != "" && port != d.selfPort) {
				return fmt.Errorf("%w：%s", errDemoSiteDial, address)
			}
			return nil
		},
	}
	t.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		return dialer.DialContext(ctx, network, addr)
	}
	t.Proxy = nil
	return t
}

// demoNpmHosts 是演示站 npm 源可以回源的地址：只有内置的两个公共 registry（按主机名，不认访客改的自定义上游）。
// DSH 装接入插件时 pnpm 可能要经 npm 源下载自己的指定版本（@pnpm/exe），回源被拦就装不上。
// npm 发布在演示站本来就关掉了，这里放开的只是公共包的只读转发。
var demoNpmHosts = map[string]bool{
	"registry.npmjs.org":     true,
	"registry.npmmirror.com": true,
	"cdn.npmmirror.com":      true,
}

// guardNpm 和 guard 一样只许连自己，另外放行 demoNpmHosts 的 443 端口。正常部署（d 为 nil）原样返回。
func (d *DemoSite) guardNpm(t *http.Transport) *http.Transport {
	if d == nil {
		return t
	}
	t = d.guard(t)
	guarded := t.DialContext
	open := &net.Dialer{Timeout: 10 * time.Second}
	t.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		if host, port, err := net.SplitHostPort(addr); err == nil && port == "443" && demoNpmHosts[strings.ToLower(host)] {
			return open.DialContext(ctx, network, addr)
		}
		return guarded(ctx, network, addr)
	}
	return t
}

type demoSiteInfo struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Reset    string `json:"reset,omitempty"`
	// 下次重置的时间点；没配 ResetEvery 时不给
	NextReset *time.Time `json:"next_reset,omitempty"`
	// 登录页上的场景卡片：以哪位示例员工的身份、打开哪个应用、问什么
	Personas []demoGuideStep `json:"personas"`
	// 有介绍视频的语言（zh / en），首页按界面语言挑一个播放；没有就不给
	Videos []string `json:"videos,omitempty"`
}

// nextDemoReset 按 cron「0 */N * * *」的规律算下次重置：从当地零点起每隔 every 一次，到第二天零点重新对齐。
// 用服务器本地时区，和 cron 一致。
func nextDemoReset(now time.Time, every time.Duration) time.Time {
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	tomorrow := midnight.AddDate(0, 0, 1)
	next := midnight.Add((now.Sub(midnight)/every + 1) * every)
	if next.After(tomorrow) {
		next = tomorrow
	}
	return next
}

// demoSiteInfo 给登录页和页面顶部的提示条用；正常部署返回 nil。
func (s *Server) demoSiteInfo() *demoSiteInfo {
	d := s.opts.DemoSite
	if d == nil {
		return nil
	}
	info := &demoSiteInfo{Username: d.Username, Password: d.Password, Reset: d.Reset, Personas: demoGuide(), Videos: d.videos()}
	if d.ResetEvery > 0 {
		next := nextDemoReset(time.Now(), d.ResetEvery)
		info.NextReset = &next
	}
	return info
}

// handleDemoLogin：演示站上一键以示例员工的身份登录，不用输密码。
// 只认示例公司的员工账号；访客本来就能用共享的管理员账号登录，这里没有多给权限，只是省掉找密码这一步。
// 正常部署不开放（404）。
func (s *Server) handleDemoLogin(w http.ResponseWriter, r *http.Request) {
	if s.opts.DemoSite == nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	var req struct {
		Username string `json:"username"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	name := strings.ToLower(strings.TrimSpace(req.Username))
	if !isDemoUser(name) {
		writeErr(w, http.StatusBadRequest, "只能用示例公司员工的身份体验")
		return
	}
	u, err := s.store.GetUserByUsername(r.Context(), name)
	if notFound(err) {
		writeErr(w, http.StatusNotFound, "示例数据正在重置，请过一分钟再试")
		return
	}
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	if u.Disabled || u.AccountType != store.AccountHuman {
		writeErr(w, http.StatusForbidden, "这个示例账号已停用")
		return
	}
	if err := s.store.TouchLogin(r.Context(), u.ID); err != nil {
		s.log.Warn("记录登录时间失败", "err", err)
	}
	s.auditAs(r, u, auditLogin, u.Username, map[string]any{"via": "demo"})
	s.demoStat("persona", u.Username)
	s.startWebSession(w, r, u)
}

// demoStat 记一次访客动作：点了场景卡片（persona）、打开了应用的网页版（app_open）、DSH 接入成功（dsh_connect）。
// 只记时间、动作和对象（示例员工的用户名 / 应用名），不记 IP、浏览器或别的访客信息。
// 数据库每次重置都清空、日志只留最近一份，所以单独追加到一个文件里。正常部署不记。
func (s *Server) demoStat(event, what string) {
	d := s.opts.DemoSite
	if d == nil || d.StatsFile == "" {
		return
	}
	line, _ := json.Marshal(map[string]string{"t": time.Now().UTC().Format(time.RFC3339), "event": event, "what": what})
	// O_APPEND 下一次 Write 就是一整行，并发的请求不会写串
	f, err := os.OpenFile(d.StatsFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		s.log.Warn("写演示站统计失败", "err", err)
		return
	}
	defer f.Close()
	_, _ = f.Write(append(line, '\n'))
}

func isDemoUser(name string) bool {
	for _, u := range demoUsers {
		if u.Username == name {
			return true
		}
	}
	return false
}

// demoVideoLangs 是介绍视频的语言，文件名 intro-<语言>.mp4。
var demoVideoLangs = []string{"zh", "en"}

// videos 返回目录里实际有的介绍视频语言。
func (d *DemoSite) videos() []string {
	if d.VideoDir == "" {
		return nil
	}
	var out []string
	for _, l := range demoVideoLangs {
		if st, err := os.Stat(filepath.Join(d.VideoDir, "intro-"+l+".mp4")); err == nil && st.Mode().IsRegular() {
			out = append(out, l)
		}
	}
	return out
}

// handleDemoVideo 播放演示站首页的介绍视频。只认固定的文件名，不拼访客给的路径；
// http.ServeFile 支持 Range，拖进度条、手机播放都要用到。正常部署 404。
func (s *Server) handleDemoVideo(w http.ResponseWriter, r *http.Request) {
	d := s.opts.DemoSite
	lang := r.PathValue("lang")
	if d == nil || d.VideoDir == "" || !slices.Contains(demoVideoLangs, lang) {
		http.NotFound(w, r)
		return
	}
	file := filepath.Join(d.VideoDir, "intro-"+lang+".mp4")
	if st, err := os.Stat(file); err != nil || !st.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	http.ServeFile(w, r, file)
}
