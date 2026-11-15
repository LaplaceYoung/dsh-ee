package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// dsh-ee package check：把本地 npm 包交给服务端预检（POST /api/me/packages/check）。
// 本机只读文件、不做检查；解包、自动检查和发布冲突判断都在服务端，与网页「发布」页同一份代码。

const (
	exitOK       = 0
	exitProblems = 1
	exitUsage    = 2
	exitService  = 3
)

func cliPackage(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, packageUsage)
		return exitUsage
	}
	if args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprint(stdout, packageUsage)
		return exitOK
	}
	switch args[0] {
	case "check":
		return cliPackageCheck(args[1:], stdout, stderr)
	}
	fmt.Fprintf(stderr, tr("不认识的命令 package %s\n\n", "unknown command package %s\n\n"), args[0])
	fmt.Fprint(stderr, packageUsage)
	return exitUsage
}

// packageCheck 与 server/internal/api.packageCheck 的 JSON 字段一一对应；这里只声明要展示的部分。
type packageCheck struct {
	Name           string `json:"name"`
	Version        string `json:"version"`
	IsPlugin       bool   `json:"is_plugin"`
	CanSubmit      bool   `json:"can_submit"`
	WillPublish    bool   `json:"will_publish"`
	RequiresReview bool   `json:"requires_review"`
	Scan           []struct {
		Key    string         `json:"key"`
		Level  string         `json:"level"`
		Params map[string]any `json:"params,omitempty"`
	} `json:"scan"`
	Conflicts []struct {
		Key    string         `json:"key"`
		Params map[string]any `json:"params,omitempty"`
	} `json:"conflicts"`
}

func cliPackageCheck(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 && (args[0] == "help" || args[0] == "-h" || args[0] == "--help") {
		fmt.Fprint(stdout, packageUsage)
		return exitOK
	}
	o, err := parseOpts(args, "server", "token")
	if err != nil {
		fmt.Fprintln(stderr, err)
		fmt.Fprintln(stderr, tr("用法：dsh-ee package check <文件.tgz> [--json]", "Usage: dsh-ee package check <file.tgz> [--json]"))
		return exitUsage
	}
	if len(o.pos) != 1 {
		fmt.Fprintln(stderr, tr("用法：dsh-ee package check <文件.tgz> [--json]", "Usage: dsh-ee package check <file.tgz> [--json]"))
		return exitUsage
	}
	path := o.pos[0]
	if st, err := os.Stat(path); err != nil {
		fmt.Fprintf(stderr, tr("读不了 %s：%v\n", "cannot read %s: %v\n"), path, err)
		return exitUsage
	} else if st.IsDir() {
		fmt.Fprintf(stderr, tr("%s 是一个目录；这里要 npm pack 生成的 .tgz 文件\n", "%s is a directory; this command needs a .tgz file produced by npm pack\n"), path)
		return exitUsage
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(stderr, tr("读不了 %s：%v\n", "cannot read %s: %v\n"), path, err)
		return exitUsage
	}
	if len(raw) == 0 {
		fmt.Fprintf(stderr, tr("%s 是空文件\n", "%s is an empty file\n"), path)
		return exitUsage
	}

	ep := resolveEndpoint(o)
	c, err := ep.client(tr("registry:publish（或 agent）范围", "registry:publish (or agent) scope"), 5*time.Minute)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsage
	}
	status, body, err := c.request("POST", "/api/me/packages/check", "application/octet-stream", raw)
	if err != nil {
		fmt.Fprintf(stderr, tr("连不上 %s：%v\n", "cannot reach %s: %v\n"), c.base, err)
		fmt.Fprintln(stderr, tr("检查地址、网络或 VPN；也可以先用 dsh-ee doctor 看接入状态", "check the address, network or VPN; you can also run dsh-ee doctor to see the connection status"))
		return exitService
	}
	switch {
	case status == http.StatusOK:
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		fmt.Fprintf(stderr, tr("预检接口拒绝了这个令牌（HTTP %d）：%s\n", "the precheck endpoint rejected this token (HTTP %d): %s\n"), status, serverErrText(body))
		fmt.Fprintf(stderr, tr("令牌要在 dsh-ee 的「我的令牌」里建，范围至少包含 registry:publish 或 agent：%s/tokens\n", "create the token in dsh-ee under \"My tokens\" with at least the registry:publish or agent scope: %s/tokens\n"), c.base)
		return exitService
	case status >= 400 && status < 500:
		fmt.Fprintf(stderr, tr("服务端不接受这个包（HTTP %d）：%s\n", "the server does not accept this package (HTTP %d): %s\n"), status, serverErrText(body))
		return exitProblems
	default:
		fmt.Fprintf(stderr, tr("调用预检接口失败（HTTP %d）：%s\n", "calling the precheck endpoint failed (HTTP %d): %s\n"), status, serverErrText(body))
		return exitService
	}

	var res packageCheck
	// 用 json.Number 保留数字的原始写法：scan 里 size 之类的参数直接展示，不能变成科学计数法。
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	if err := dec.Decode(&res); err != nil {
		fmt.Fprintf(stderr, tr("服务端返回了看不懂的结果：%v\n", "the server returned an unreadable result: %v\n"), err)
		return exitService
	}
	if o.asJSON {
		printRawJSON(stdout, body)
	} else {
		printPackageCheck(stdout, &res)
	}
	if !res.CanSubmit || hasScanFail(&res) {
		return exitProblems
	}
	return exitOK
}

func hasScanFail(res *packageCheck) bool {
	for _, item := range res.Scan {
		if item.Level == "fail" {
			return true
		}
	}
	return false
}

// scanLevelLabel 把自动检查项的级别翻成当前语言的词（默认中文）。
func scanLevelLabel(level string) string {
	switch level {
	case "pass":
		return tr("通过", "pass")
	case "warn":
		return tr("提醒", "warning")
	case "fail":
		return tr("失败", "failed")
	}
	return level
}

func printPackageCheck(w io.Writer, res *packageCheck) {
	kind := tr("普通 npm 包", "plain npm package")
	if res.IsPlugin {
		kind = tr("DSH 插件", "DSH plugin")
	}
	fmt.Fprintf(w, tr("预检 %s@%s（%s）\n", "precheck %s@%s (%s)\n"), res.Name, res.Version, kind)

	pass, warn, fail := 0, 0, 0
	for _, item := range res.Scan {
		switch item.Level {
		case "pass":
			pass++
		case "warn":
			warn++
		case "fail":
			fail++
		}
	}
	fmt.Fprintf(w, tr("自动检查：%d 项通过", "Automatic checks: %d passed"), pass)
	if warn > 0 {
		fmt.Fprintf(w, tr("，%d 项提醒", ", %d warnings"), warn)
	}
	if fail > 0 {
		fmt.Fprintf(w, tr("，%d 项失败", ", %d failed"), fail)
	}
	fmt.Fprintln(w)
	for _, item := range res.Scan {
		if item.Level == "pass" {
			continue
		}
		label := scanLevelLabel(item.Level)
		if label == "" {
			label = item.Level
		}
		fmt.Fprintf(w, "  · %s  %s\n", label, scanText(item.Level, item.Key, item.Params))
	}

	if len(res.Conflicts) > 0 {
		fmt.Fprintln(w, tr("\n发布冲突：", "\nPublish conflicts:"))
		for _, c := range res.Conflicts {
			fmt.Fprintf(w, "  · %s\n", conflictText(c.Key, c.Params))
		}
	}

	fmt.Fprintln(w)
	switch {
	case !res.CanSubmit:
		fmt.Fprintln(w, tr("结论：不能提交，服务端会直接拒绝；先解决上面的发布冲突。", "Result: cannot submit; the server will reject it outright. Resolve the publish conflicts above first."))
	case fail > 0:
		fmt.Fprintln(w, tr("结论：不能自动上架；提交后会进入人工审核，审核员会看这些失败项。", "Result: cannot be published automatically; after submission it goes to manual review, where the reviewer will look at these failures."))
	case res.WillPublish:
		fmt.Fprintln(w, tr("结论：可以直接上架（管理员开了开放模式，且没有阻塞项）。", "Result: can be published directly (open mode is on and there are no blockers)."))
	case res.RequiresReview:
		fmt.Fprintln(w, tr("结论：可以提交，会进入人工审核。", "Result: can be submitted; it will go to manual review."))
	default:
		fmt.Fprintln(w, tr("结论：可以提交。", "Result: can be submitted."))
	}
}

// scanText 把服务端 scan 项的 key/level/params 翻成一句人话。
// 文案与网页 i18n 是两份（CLI 不能读前端资源），但 key 和语义以服务端为准。
func scanText(level, key string, params map[string]any) string {
	str := func(k string) string {
		if v, ok := params[k]; ok && v != nil {
			return fmt.Sprint(v)
		}
		return ""
	}
	path := func() string {
		if p := str("path"); p != "" {
			return p + tr("：", ": ")
		}
		return ""
	}
	switch key {
	case "bundle":
		if level == "warn" {
			return tr("没有声明 dsh.bundle：这是普通 npm 包，不会出现在能力市场和 DSH 索引里", "no dsh.bundle declared: this is a plain npm package; it will not appear in the capability marketplace or the DSH index")
		}
		switch str("reason") {
		case "no-patch":
			return tr("声明了 dsh.bundle，但缺少 patch 字段", "dsh.bundle is declared but the patch field is missing")
		case "unsafe":
			return tr("dsh.bundle.patch 不是安全的相对路径：", "dsh.bundle.patch is not a safe relative path: ") + str("path")
		case "file-missing":
			return tr("dsh.bundle.patch 指向的 ", "dsh.bundle.patch points at ") + str("path") + tr(" 不在包里（是不是没有 build 或 files 漏了？）", ", which is not in the package (did you skip the build, or is it missing from files?)")
		}
		return tr("组合包声明有问题（", "bundle declaration problem (") + str("reason") + tr("）", ")")
	case "patch":
		detail := patchReason(str("reason"), params)
		return path() + detail
	case "skills":
		return fmt.Sprintf(tr("包内含 %s 个技能：%s。它们由插件运行时注入，不会出现在能力中心的技能 Tab，也不能按技能单独授权或下架。", "the package contains %s skills: %s. They are injected by the plugin runtime, do not appear in the Skills tab of the capability center, and cannot be authorized or delisted individually."), str("count"), str("files"))
	case "install-scripts":
		return tr("安装时会执行脚本：", "install scripts will run: ") + str("scripts") + tr("；确认它不在安装时下载或执行外部代码", "; make sure they do not download or execute external code at install time")
	case "secrets":
		return fmt.Sprintf(tr("发现 %s 处疑似密钥或敏感文件：%s", "found %s possible secrets or sensitive files: %s"), str("count"), str("files"))
	case "license":
		return tr("没有声明 license", "no license declared")
	case "engines":
		return tr("没有声明 engines.dsh，DSH 会按 peer 依赖判断兼容性", "no engines.dsh declared; DSH will judge compatibility from peer dependencies")
	case "cordis":
		return tr("包把 ", "the package puts ") + str("packages") + tr(" 放进了 dependencies：DSH 已提供 Cordis，双份会导致服务注入和生命周期错乱；请移到 peerDependencies（仅构建用可放 devDependencies）", " into dependencies: DSH already provides Cordis, and a duplicate causes service injection and lifecycle problems; move it to peerDependencies (devDependencies is fine if it is build-only)")
	case "cordis-peer":
		return tr("peerDependencies 的 Cordis 范围 ", "the peerDependencies Cordis range ") + str("range") + tr(" 不包含当前 DSH 使用的 ", " does not include ") + str("want") + tr("；如果还要兼容旧宿主，请确认范围", " which the current DSH uses; if you also need to support older hosts, check the range")
	case "size":
		return fmt.Sprintf(tr("包 %s（解包 %s，%s 个文件），偏大", "package %s (unpacked %s, %s files) is large"), humanBytes(str("size")), humanBytes(str("unpacked")), str("files"))
	}
	return key + tr("（", " (") + paramsText(params) + tr("）", ")")
}

func patchReason(reason string, params map[string]any) string {
	str := func(k string) string {
		if v, ok := params[k]; ok && v != nil {
			return fmt.Sprint(v)
		}
		return ""
	}
	row, insert := str("row"), str("insert")
	switch reason {
	case "yaml":
		return tr("不是合法 YAML（", "invalid YAML (") + str("error") + tr("）", ")")
	case "not-array":
		return tr("顶层必须是数组", "the top level must be an array")
	case "empty":
		return tr("没有任何 patch 行", "there are no patch rows")
	case "row-not-object":
		if insert != "" {
			return tr("第 ", "row ") + row + tr(" 行 insert 第 ", ", insert item ") + insert + tr(" 项不是对象", " is not an object")
		}
		return tr("第 ", "row ") + row + tr(" 行不是对象", " is not an object")
	case "insert-not-array":
		return tr("第 ", "row ") + row + tr(" 行的 insert 必须是数组", " insert must be an array")
	case "missing-id":
		if insert != "" {
			return tr("第 ", "row ") + row + tr(" 行 insert 第 ", ", insert item ") + insert + tr(" 项缺少 row id", " is missing the row id")
		}
		return tr("第 ", "row ") + row + tr(" 行缺少 row id", " is missing the row id")
	case "missing-name":
		return tr("第 ", "row ") + row + tr(" 行 insert 第 ", ", insert item ") + insert + tr(" 项缺少 name", " is missing name")
	case "duplicate-id":
		return tr("row id ", "row id ") + str("id") + tr(" 重复（第 ", " is duplicated (row ") + row + tr(" 行与第 ", " and row ") + str("first") + tr(" 行）", ")")
	case "name-mismatch":
		return tr("没有任何一行引用本包名 ", "no row references this package name ") + str("name") + tr("；DSH 会找不到插件", "; DSH will not find the plugin")
	case "config-not-map":
		return str("name") + tr(" 的 config 不是 mapping", " config is not a mapping")
	case "unreadable":
		return tr("patch 文件太大或未能读取", "the patch file is too large or could not be read")
	}
	return reason
}

func conflictText(key string, params map[string]any) string {
	str := func(k string) string {
		if v, ok := params[k]; ok && v != nil {
			return fmt.Sprint(v)
		}
		return ""
	}
	switch key {
	case "name":
		return tr("包名 ", "package name ") + str("name") + tr(" 不是合法的 npm 包名", " is not a valid npm package name")
	case "scope":
		return tr("包名不在企业作用域内；这里能发布的包以企业作用域开头（允许：", "the package name is outside the enterprise scope; packages published here must start with the enterprise scope (allowed: ") + str("scope") + tr("）", ")")
	case "version":
		return tr("版本号 ", "version ") + str("version") + tr(" 不合法（要像 1.0.0 这样）", " is invalid (it should look like 1.0.0)")
	case "upstream":
		return str("name") + tr(" 是从上游预载的包，不能发布同名包", " was preloaded from upstream; a package with the same name cannot be published")
	case "owner":
		return str("name") + tr(" 属于别人，不能发布它的新版本", " belongs to someone else; a new version of it cannot be published")
	case "exists":
		return str("name") + "@" + str("version") + tr(" 已经存在：同一个版本号不能重复发布（被驳回或下架过的也不行）", " already exists: the same version cannot be published twice (not even after a rejection or unpublish)")
	}
	return key + tr("（", " (") + paramsText(params) + tr("）", ")")
}

func paramsText(params map[string]any) string {
	if len(params) == 0 {
		return tr("无参数", "no params")
	}
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%v", k, params[k]))
	}
	return strings.Join(parts, tr("，", ", "))
}

func humanBytes(s string) string {
	if s == "" || s == "0" {
		return "0 B"
	}
	var n int64
	if _, err := fmt.Sscan(s, &n); err != nil {
		return s
	}
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGT"[exp])
}

// ---- 连接信息：地址与令牌 ----

// cliEndpoint 是 package check 和 doctor 共用的服务地址与令牌。
// 记下来源，doctor 才能说清楚「这个地址是从哪读到的」。
type cliEndpoint struct {
	base, token             string
	baseSource, tokenSource string
}

// client 校验地址与令牌后建一个带认证的客户端；tokenHint 用于缺令牌时提示需要什么范围。
func (ep cliEndpoint) client(tokenHint string, timeout time.Duration) (*eeClient, error) {
	if ep.base == "" {
		return nil, errors.New(tr("不知道 dsh-ee 的地址：加 --server https://…、设置 DSH_EE_URL，或先运行一次 setup.sh 写好 .npmrc", "dsh-ee address unknown: add --server https://…, set DSH_EE_URL, or run setup.sh once to write .npmrc"))
	}
	if u, err := url.Parse(ep.base); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf(tr("dsh-ee 的地址 %q 不对，要像 https://ee.acme.com 这样", "the dsh-ee address %q is invalid; it should look like https://ee.acme.com"), ep.base)
	}
	if ep.token == "" {
		return nil, fmt.Errorf(tr("没有令牌：用 --token 或 DSH_EE_TOKEN 传一个带 %s 的令牌（网页「我的令牌」：%s/tokens）", "no token: pass one with %s via --token or DSH_EE_TOKEN (web page \"My tokens\": %s/tokens)"), tokenHint, ep.base)
	}
	return &eeClient{base: ep.base, token: ep.token, hc: &http.Client{Timeout: timeout}}, nil
}

// resolveEndpoint 按「参数 → 环境变量 → 本机配置」找 dsh-ee 的地址与令牌：
// 地址还看 .npmrc 里的企业源，令牌还看 $DSH_HOME/dsh-ee/token 与 .npmrc 的 _authToken。
// 找不到不报错，由调用方决定算用法错误（package check）还是诊断结果（doctor）。
func resolveEndpoint(o *cliOpts) cliEndpoint {
	ep := cliEndpoint{}
	switch {
	case o.vals["server"] != "":
		ep.base, ep.baseSource = o.vals["server"], tr("参数 --server", "flag --server")
	case os.Getenv("DSH_EE_URL") != "":
		ep.base, ep.baseSource = os.Getenv("DSH_EE_URL"), tr("环境变量 DSH_EE_URL", "env var DSH_EE_URL")
	}
	switch {
	case o.vals["token"] != "":
		ep.token, ep.tokenSource = o.vals["token"], tr("参数 --token", "flag --token")
	case os.Getenv("DSH_EE_TOKEN") != "":
		ep.token, ep.tokenSource = os.Getenv("DSH_EE_TOKEN"), tr("环境变量 DSH_EE_TOKEN", "env var DSH_EE_TOKEN")
	}

	npm := readNpmrc(npmrcPath())
	if ep.base == "" {
		if v := npm.server(); v != "" {
			ep.base, ep.baseSource = v, "npmrc "+npm.path
		}
	}
	if ep.token == "" {
		if v, path := readDSHToken(); v != "" {
			ep.token, ep.tokenSource = v, tr("文件 ", "file ")+path
		} else if v := npm.anyAuthToken(); v != "" {
			ep.token, ep.tokenSource = v, "npmrc "+npm.path
		}
	}
	ep.base = strings.TrimRight(strings.TrimSpace(ep.base), "/")
	return ep
}

// request 发一次请求，把 HTTP 状态码和响应体原样交回；只有网络层失败才返回错误。
// doctor 要根据 400/401/403 区分「有权限」「令牌不对」，不能像 do 一样把非 2xx 都变成错误。
func (c *eeClient) request(method, path, contentType string, body []byte) (int, []byte, error) {
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, c.base+path, rd)
	if err != nil {
		return 0, nil, err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	req.Header.Set("X-DshEE-Client", "dsh-ee-cli/"+version)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, raw, nil
}

func serverErrText(raw []byte) string {
	var e struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(raw, &e) == nil && strings.TrimSpace(e.Error) != "" {
		return strings.TrimSpace(e.Error)
	}
	return strings.TrimSpace(string(raw))
}

// ---- 本机配置：.npmrc 与 $DSH_HOME/dsh-ee/token ----

// npmrcFile 是 .npmrc 里与本命令有关的部分：企业源地址与对应令牌。
// 只解析、不写文件；doctor 只读诊断，package check 也不会改用户的配置。
type npmrcFile struct {
	path       string
	exists     bool
	registries map[string]string // "@scope" / ""（默认 registry）→ 地址
	authTokens map[string]string // "//host/npm/:_authToken" → 令牌
}

func npmrcPath() string {
	if p := strings.TrimSpace(os.Getenv("NPM_CONFIG_USERCONFIG")); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".npmrc"
	}
	return filepath.Join(home, ".npmrc")
}

func dshHomeDir() string {
	if d := strings.TrimSpace(os.Getenv("DSH_HOME")); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".dsh")
}

// readDSHToken 读 setup / 接入插件在凭据库不可用时留下的本地令牌文件。
func readDSHToken() (string, string) {
	home := dshHomeDir()
	if home == "" {
		return "", ""
	}
	path := filepath.Join(home, "dsh-ee", "token")
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", ""
	}
	return strings.TrimSpace(string(raw)), path
}

func readNpmrc(path string) *npmrcFile {
	f := &npmrcFile{path: path, registries: map[string]string{}, authTokens: map[string]string{}}
	raw, err := os.ReadFile(path)
	if err != nil {
		return f
	}
	f.exists = true
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		switch {
		case strings.HasSuffix(key, ":registry"):
			f.registries[strings.TrimSuffix(key, ":registry")] = value
		case key == "registry":
			f.registries[""] = value
		case strings.HasSuffix(key, ":_authToken"):
			f.authTokens[key] = value
		}
	}
	return f
}

// server 从 .npmrc 的企业源里取出 dsh-ee 地址。setup.sh 写的是 <地址>/npm/。
func (f *npmrcFile) server() string {
	keys := make([]string, 0, len(f.registries))
	for k := range f.registries {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := strings.TrimRight(strings.TrimSpace(f.registries[k]), "/")
		if base, ok := strings.CutSuffix(v, "/npm"); ok && base != "" {
			return base
		}
	}
	return ""
}

// anyAuthToken 返回 .npmrc 里第一条 dsh-ee 企业源的令牌（order 稳定：按 registry 键排序）。
func (f *npmrcFile) anyAuthToken() string {
	keys := make([]string, 0, len(f.authTokens))
	for k := range f.authTokens {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if strings.Contains(k, "/npm") && strings.TrimSpace(f.authTokens[k]) != "" {
			return strings.TrimSpace(f.authTokens[k])
		}
	}
	return ""
}

// registryURL 返回与 base 对应的企业源地址，例如 https://ee.acme.com/npm/。
func registryURL(base string) string { return strings.TrimRight(base, "/") + "/npm/" }

// authTokenKey 与 setup.sh / npmrcEntries 的写法一致：//host/npm/:_authToken。
func authTokenKey(registry string) string {
	u, err := url.Parse(registry)
	if err != nil || u.Host == "" {
		return ""
	}
	return "//" + u.Host + u.Path + ":_authToken"
}
