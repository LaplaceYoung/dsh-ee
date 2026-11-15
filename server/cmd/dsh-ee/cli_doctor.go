package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

// dsh-ee doctor：只读诊断「发布一个包到 dsh-ee」这条链路。
// 不写文件、不改服务端状态；唯一的 POST 是向预检接口发一个空包探权限，
// 预检本身不落库（package_check.go），所以也算只读。

type doctorCheck struct {
	ID     string `json:"id"`
	Status string `json:"status"` // ok / warn / fail
	Title  string `json:"title"`
	Detail string `json:"detail,omitempty"`
	Fix    string `json:"fix,omitempty"`
}

type doctorReport struct {
	Summary string        `json:"summary"`
	OK      bool          `json:"ok"`
	Checks  []doctorCheck `json:"checks"`
}

// doctorCommand 跑本机的 node / dsh 取版本；测试里替换成假命令。
var doctorCommand = func(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).Output()
	return strings.TrimSpace(string(out)), err
}

const doctorTimeout = 10 * time.Second

func cliDoctor(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 && (args[0] == "help" || args[0] == "-h" || args[0] == "--help") {
		fmt.Fprint(stdout, doctorUsage)
		return exitOK
	}
	o, err := parseOpts(args, "server", "token")
	if err != nil {
		fmt.Fprintln(stderr, err)
		fmt.Fprintln(stderr, tr("用法：dsh-ee doctor [--server 地址] [--token 令牌] [--json]", "Usage: dsh-ee doctor [--server URL] [--token TOKEN] [--json]"))
		return exitUsage
	}
	if len(o.pos) > 0 {
		fmt.Fprintln(stderr, tr("用法：dsh-ee doctor [--server 地址] [--token 令牌] [--json]", "Usage: dsh-ee doctor [--server URL] [--token TOKEN] [--json]"))
		return exitUsage
	}
	rep := collectDoctor(resolveEndpoint(o))
	if o.asJSON {
		printJSON(stdout, rep)
	} else {
		printDoctor(stdout, rep)
	}
	if !rep.OK {
		return exitProblems
	}
	return exitOK
}

func collectDoctor(ep cliEndpoint) *doctorReport {
	checks := []doctorCheck{}
	add := func(id, status, title, detail, fix string) {
		checks = append(checks, doctorCheck{ID: id, Status: status, Title: title, Detail: detail, Fix: fix})
	}
	npm := readNpmrc(npmrcPath())

	// 1. 服务地址
	baseOK := false
	switch {
	case ep.base == "":
		add("endpoint", "fail", tr("dsh-ee 地址", "dsh-ee address"), tr("没有找到服务地址", "no server address found"),
			tr("加 --server https://… 或设置 DSH_EE_URL；也可以运行一次 setup.sh，让 .npmrc 指向企业源", "add --server https://… or set DSH_EE_URL; you can also run setup.sh once to point .npmrc at the enterprise registry"))
	case !validBaseURL(ep.base):
		add("endpoint", "fail", tr("dsh-ee 地址", "dsh-ee address"), fmt.Sprintf(tr("%q 不是 http(s) 地址", "%q is not an http(s) address"), ep.base),
			tr("改成浏览器能打开的地址，例如 https://ee.acme.com", "use an address a browser can open, e.g. https://ee.acme.com"))
	default:
		baseOK = true
		add("endpoint", "ok", tr("dsh-ee 地址", "dsh-ee address"), ep.base+tr("（来自 ", "(from ")+ep.baseSource+tr("）", ")"), "")
	}

	if baseOK {
		c := &eeClient{base: ep.base, token: ep.token, hc: &http.Client{Timeout: doctorTimeout}}

		// 2. 服务连通与版本
		serverOK := false
		status, raw, err := c.request("GET", "/healthz", "", nil)
		switch {
		case err != nil:
			add("server", "fail", tr("服务连通", "server reachable"), tr("连不上 ", "cannot reach ")+ep.base+tr("：", ": ")+err.Error(),
				tr("检查地址、网络或 VPN；用浏览器打开 ", "check the address, network or VPN; open ")+ep.base+tr("/healthz 确认服务在跑", "/healthz in a browser to confirm the server is running"))
		case status == http.StatusOK:
			serverOK = true
			var health struct {
				Version string `json:"version"`
			}
			_ = json.Unmarshal(raw, &health)
			v := health.Version
			if v == "" {
				v = tr("未知版本", "unknown version")
			}
			add("server", "ok", tr("服务连通", "server reachable"), tr("版本 ", "version ")+v, "")
		default:
			add("server", "fail", tr("服务连通", "server reachable"), fmt.Sprintf(tr("HTTP %d：%s", "HTTP %d: %s"), status, serverErrText(raw)),
				tr("确认这个地址是 dsh-ee 服务端；反向代理 / 证书有问题时看服务端日志", "confirm this address is a dsh-ee server; if the reverse proxy or certificate is the problem, check the server logs"))
		}

		// 3. 令牌：用 npm 源的 whoami 验有效性（不要求 hub 范围），能读 /api/dsh/whoami 时补充范围和到期时间
		tokenOK := false
		if ep.token == "" {
			add("token", "fail", tr("令牌", "token"), tr("没有找到令牌", "no token found"),
				fmt.Sprintf(tr("在 %s/tokens 建一个带 agent 或 registry:publish 范围的令牌，用 --token 或 DSH_EE_TOKEN 传进来（也可以运行 setup.sh）", "create a token with the agent or registry:publish scope at %s/tokens and pass it with --token or DSH_EE_TOKEN (or run setup.sh)"), ep.base))
		} else {
			status, raw, err = c.request("GET", "/npm/-/whoami", "", nil)
			switch {
			case err != nil:
				add("token", "fail", tr("令牌", "token"), tr("读不到令牌身份（连不上 npm 源）：", "cannot read token identity (npm registry unreachable): ")+err.Error(),
					tr("检查网络或 VPN 后重试；持续失败看服务端是否在维护", "check the network or VPN and retry; if it keeps failing, check whether the server is under maintenance"))
			case status == http.StatusOK:
				tokenOK = true
				var who struct {
					Username string `json:"username"`
				}
				_ = json.Unmarshal(raw, &who)
				detail := tr("有效，用户名 ", "valid, username ") + who.Username + tr("（来自 ", "(from ") + ep.tokenSource + tr("）", ")")
				state, fix := "ok", ""
				if scopes, expires, ok := dshTokenInfo(c); ok {
					if len(scopes) > 0 {
						detail += tr("；范围：", "; scopes: ") + joinText(scopes)
					}
					if left, ok := daysLeft(expires); ok && left < 7 {
						state = "warn"
						fix = tr("令牌快到期了；到期前重新登录，或在「我的令牌」里换一个新的", "the token expires soon; log in again before it expires, or create a new one in \"My tokens\"")
						detail += fmt.Sprintf(tr("；%d 天内到期", "; expires in %d days"), left)
					}
				}
				add("token", state, tr("令牌", "token"), detail, fix)
			case status == http.StatusUnauthorized:
				add("token", "fail", tr("令牌", "token"), tr("无效、已过期或已吊销", "invalid, expired, or revoked"),
					fmt.Sprintf(tr("在 %s/tokens 重新建一个带 agent 或 registry:publish 范围的令牌，再用 --token 或 DSH_EE_TOKEN 传进来", "create a new token with the agent or registry:publish scope at %s/tokens, then pass it with --token or DSH_EE_TOKEN"), ep.base))
			default:
				add("token", "warn", tr("令牌", "token"), fmt.Sprintf(tr("读不到令牌身份（HTTP %d）：%s", "cannot read token identity (HTTP %d): %s"), status, serverErrText(raw)),
					tr("稍后重试；也可以用 --server/--token 再跑一次 doctor 排除本机配置问题", "retry later; you can also rerun doctor with --server/--token to rule out local config problems"))
			}
		}

		// 4. 发布预检权限：空包探一下，400=有权限（空包被拒），403=范围不够，401=令牌无效
		if tokenOK && serverOK {
			status, raw, err = c.request("POST", "/api/me/packages/check", "application/octet-stream", nil)
			switch {
			case err != nil:
				add("publish-scope", "warn", tr("发布权限", "publish permission"), tr("探不到预检接口：", "cannot reach the precheck endpoint: ")+err.Error(), tr("稍后重试", "retry later"))
			case status == http.StatusBadRequest:
				add("publish-scope", "ok", tr("发布权限", "publish permission"), tr("令牌可以调用发布预检（registry.publish 或 agent 范围）", "the token can call the publish precheck (registry.publish or agent scope)"), "")
			case status == http.StatusForbidden:
				add("publish-scope", "fail", tr("发布权限", "publish permission"), tr("令牌没有 registry:publish 或 agent 范围", "the token lacks the registry:publish or agent scope"),
					fmt.Sprintf(tr("在 %s/tokens 建一个带 agent 或 registry:publish 范围的令牌，或让管理员确认账号有「发布能力」权限", "create a token with the agent or registry:publish scope at %s/tokens, or ask an admin to confirm the account has the publish capability"), ep.base))
			case status == http.StatusUnauthorized:
				add("publish-scope", "fail", tr("发布权限", "publish permission"), tr("令牌无效", "invalid token"), tr("重新建一个令牌后再跑一次 doctor", "create a new token and rerun doctor"))
			default:
				add("publish-scope", "warn", tr("发布权限", "publish permission"), fmt.Sprintf(tr("预检接口返回 HTTP %d：%s", "the precheck endpoint returned HTTP %d: %s"), status, serverErrText(raw)), tr("服务端异常；看服务端日志", "server problem; check the server logs"))
			}
		}

		// 5. 企业 npm 源
		status, raw, err = c.request("GET", "/npm/-/ping", "", nil)
		switch {
		case err != nil:
			add("npm-source", "fail", tr("企业 npm 源", "enterprise npm registry"), tr("连不上 ", "cannot reach ")+registryURL(ep.base)+tr("：", ": ")+err.Error(),
				tr("检查地址或反向代理；npm publish / 装企业插件会走这个源", "check the address or reverse proxy; npm publish and enterprise plugin installs use this registry"))
		case status == http.StatusOK:
			add("npm-source", "ok", tr("企业 npm 源", "enterprise npm registry"), registryURL(ep.base)+tr(" 可达", " is reachable"), "")
		default:
			add("npm-source", "fail", tr("企业 npm 源", "enterprise npm registry"), fmt.Sprintf(tr("HTTP %d：%s", "HTTP %d: %s"), status, serverErrText(raw)),
				tr("确认服务端 npm 源已启用、数据目录可写；看服务端日志", "confirm the server npm registry is enabled and its data directory is writable; check the server logs"))
		}
	}

	// 6. .npmrc：企业源地址和令牌是否与当前登录一致。只读，不打印令牌。
	registries := []string{}
	if ep.base != "" {
		registries = append(registries, registryURL(ep.base))
	}
	npmDetail, npmState, npmFix := doctorNpmrc(npm, ep, registries)
	add("npmrc", npmState, "npmrc", npmDetail, npmFix)

	// 7. Node
	if out, err := doctorCommand("node", "--version"); err != nil {
		add("node", "warn", "Node", tr("没有找到 node 命令", "node command not found"),
			tr("装 Node 22+（npm pack 与构建插件都要它；上游 dsh-primer 脚手架要求 Node ≥24.9）", "install Node 22+ (needed for npm pack and for building plugins; the upstream dsh-primer scaffold requires Node ≥24.9)"))
	} else {
		major := versionMajor(out)
		switch {
		case major < 0:
			add("node", "warn", "Node", tr("版本读不懂：", "cannot parse version: ")+out, tr("升级到 Node 22+", "upgrade to Node 22+"))
		case major < 22:
			add("node", "warn", "Node", out+tr(" 太旧", " is too old"),
				tr("升级到 Node 22+；上游 dsh-primer 脚手架要求 Node ≥24.9", "upgrade to Node 22+; the upstream dsh-primer scaffold requires Node ≥24.9"))
		case major < 24:
			add("node", "ok", "Node", out+tr("（dsh-ee 预检可用；用上游 dsh-primer 脚手架需要 ≥24.9）", " (works for the dsh-ee precheck; the upstream dsh-primer scaffold needs ≥24.9)"), "")
		default:
			add("node", "ok", "Node", out, "")
		}
	}

	// 8. dsh
	if out, err := doctorCommand("dsh", "--version"); err != nil {
		add("dsh", "warn", "DSH", tr("没有找到 dsh 命令", "dsh command not found"),
			tr("只发布 npm 包不强制；要在本机安装、验证企业插件时装 DSH", "not required just to publish npm packages; install DSH to install and verify enterprise plugins locally"))
	} else if line := firstLine(out); line != "" {
		add("dsh", "ok", "DSH", line, "")
	} else {
		add("dsh", "warn", "DSH", tr("dsh --version 没有输出", "dsh --version produced no output"), tr("确认 DSH 安装完整", "make sure DSH is fully installed"))
	}

	fails, warns := 0, 0
	for _, c := range checks {
		switch c.Status {
		case "fail":
			fails++
		case "warn":
			warns++
		}
	}
	rep := &doctorReport{OK: fails == 0, Checks: checks}
	switch {
	case fails > 0:
		rep.Summary = fmt.Sprintf(tr("%d 项失败，%d 项需要注意", "%d failed, %d need attention"), fails, warns)
	case warns > 0:
		rep.Summary = fmt.Sprintf(tr("%d 项需要注意", "%d need attention"), warns)
	default:
		rep.Summary = tr("全部正常", "all checks passed")
	}
	return rep
}

func validBaseURL(base string) bool {
	u, err := url.Parse(base)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// dshTokenInfo 是尽力而为：/api/dsh/whoami 要 hub 范围，只有 registry 范围的令牌读不到，
// 这时返回 ok=false，不影响 doctor 结论。
func dshTokenInfo(c *eeClient) (scopes []string, expires string, ok bool) {
	status, raw, err := c.request("GET", "/api/dsh/whoami", "", nil)
	if err != nil || status != http.StatusOK {
		return nil, "", false
	}
	var me struct {
		Scopes    []string `json:"scopes"`
		ExpiresAt string   `json:"expires_at"`
	}
	if json.Unmarshal(raw, &me) != nil {
		return nil, "", false
	}
	sort.Strings(me.Scopes)
	return me.Scopes, me.ExpiresAt, true
}

func doctorNpmrc(npm *npmrcFile, ep cliEndpoint, registries []string) (detail, state, fix string) {
	if !npm.exists {
		return tr("没有找到 "+npm.path+"（企业源装插件和 npm publish 会用到）", "not found: "+npm.path+" (needed to install plugins from the enterprise registry and to npm publish)"), "warn",
			tr("运行一次 setup.sh 会为 DSH profile 写好企业源；需要全局 npm publish 时把 registry 与 _authToken 写到 "+npm.path, "running setup.sh once writes the enterprise registry for the DSH profile; if you also need a global npm publish, write registry and _authToken to "+npm.path)
	}
	if len(registries) == 0 {
		return tr(npm.path+"：不知道 dsh-ee 地址，没法核对企业源", npm.path+": unknown dsh-ee address; cannot check the enterprise registry"), "warn", tr("先配置 dsh-ee 地址再跑一次 doctor", "configure the dsh-ee address and rerun doctor")
	}
	want := registries[0]
	found := false
	for _, v := range npm.registries {
		if strings.TrimRight(strings.TrimSpace(v), "/") == strings.TrimRight(want, "/") {
			found = true
			break
		}
	}
	if !found {
		return tr(npm.path+" 里没有指向 "+want+" 的企业源", npm.path+" has no enterprise registry pointing at "+want), "warn",
			tr("运行 setup.sh 自动同步；或手动加一行 registry="+want+"（以及各 <作用域>:registry="+want+"）", "run setup.sh to sync automatically; or add a registry="+want+" line manually (plus <scope>:registry="+want+" for each scope)")
	}
	key := authTokenKey(want)
	token, ok := npm.authTokens[key]
	if key == "" || !ok || strings.TrimSpace(token) == "" {
		return tr(npm.path+" 里有企业源，但缺少 "+key, npm.path+" has the enterprise registry but is missing "+key), "warn",
			tr("运行 setup.sh 会自动写这一行；或者重新登录让接入插件同步", "running setup.sh writes this line automatically; or log in again so the connect plugin syncs it")
	}
	if ep.token != "" && strings.TrimSpace(token) != strings.TrimSpace(ep.token) {
		return tr(npm.path+" 的企业源令牌与当前令牌不一致（npm publish 会 401）", npm.path+" has an enterprise registry token that differs from the current token (npm publish will get a 401)"), "warn",
			tr("重新登录或运行 setup.sh 同步令牌（只改 _authToken 那一行）", "log in again or run setup.sh to sync the token (only the _authToken line changes)")
	}
	return npm.path + tr("：企业源与令牌一致", ": enterprise registry and token match"), "ok", ""
}

func printDoctor(w io.Writer, rep *doctorReport) {
	fmt.Fprintln(w, tr("dsh-ee doctor（只读诊断，不修改任何东西）", "dsh-ee doctor (read-only diagnosis; nothing is modified)"))
	fmt.Fprintln(w)
	for _, c := range rep.Checks {
		fmt.Fprintf(w, tr("%s %s：%s\n", "%s %s: %s\n"), doctorMark(c.Status), c.Title, c.Detail)
	}
	fmt.Fprintf(w, tr("\n结论：%s\n", "\nResult: %s\n"), rep.Summary)
	var next []string
	seen := map[string]bool{}
	for _, c := range rep.Checks {
		if c.Status == "ok" || strings.TrimSpace(c.Fix) == "" || seen[c.Fix] {
			continue
		}
		seen[c.Fix] = true
		next = append(next, c.Fix)
	}
	if len(next) > 0 {
		fmt.Fprintln(w, tr("下一步：", "Next steps:"))
		for _, s := range next {
			fmt.Fprintln(w, "  · "+s)
		}
	}
}

func doctorMark(status string) string {
	switch status {
	case "ok":
		return "✓"
	case "warn":
		return "!"
	default:
		return "✗"
	}
}

// versionMajor 从 "v24.22.1" / "24.22.1" 这类输出里取主版本号；读不懂返回 -1。
func versionMajor(v string) int {
	v = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(v), "v"))
	if i := strings.IndexAny(v, "-+ "); i >= 0 {
		v = v[:i]
	}
	first := strings.SplitN(v, ".", 2)[0]
	n, err := strconv.Atoi(first)
	if err != nil {
		return -1
	}
	return n
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}

// daysLeft 把 RFC3339 到期时间换算成剩余天数；没有到期时间返回 ok=false。
func daysLeft(expires string) (int, bool) {
	if strings.TrimSpace(expires) == "" {
		return 0, false
	}
	t, err := time.Parse(time.RFC3339, expires)
	if err != nil {
		return 0, false
	}
	left := int(time.Until(t).Hours() / 24)
	if left < 0 {
		left = 0
	}
	return left, true
}
