package api_test

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/hikariming/dsh-ee/server/internal/api"
	"github.com/hikariming/dsh-ee/server/internal/secret"
)

func setupScriptServer(t *testing.T, publicURL ...string) (*httptest.Server, *client) {
	t.Helper()
	st := openTestStore(t)
	if _, err := api.Bootstrap(t.Context(), st, "admin", "admin-password-123"); err != nil {
		t.Fatal(err)
	}
	box, err := secret.NewBox(bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	opts := api.Options{DataDir: t.TempDir()}
	if len(publicURL) > 0 {
		opts.PublicURL = publicURL[0]
	}
	srv := api.New(st, box, opts, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	admin := newClient(t, ts.URL)
	code, body := admin.do("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "admin-password-123"}, nil)
	expect(t, "管理员登录", code, http.StatusOK, body)
	return ts, admin
}

func getScript(t *testing.T, base, path string) string {
	t.Helper()
	return getScriptLang(t, base, path, "")
}

// getScriptLang 取脚本；acceptLanguage 非空时带上请求头，用来测语言协商。
func getScriptLang(t *testing.T, base, path, acceptLanguage string) string {
	t.Helper()
	req, err := http.NewRequest("GET", base+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if acceptLanguage != "" {
		req.Header.Set("Accept-Language", acceptLanguage)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s：%d %s", path, resp.StatusCode, body)
	}
	return string(body)
}

func TestSetupScripts(t *testing.T) {
	ts, admin := setupScriptServer(t)
	// 先发布接入插件：只有 pkg 非空时 setup.ps1 才会走“装好插件”的收尾分支，
	// $Base/hub 的提示和 CONNECT_PKG 相关断言都在这条分支上。
	code, body := admin.do("POST", "/api/admin/connect/publish", map[string]bool{"force": true}, nil)
	expect(t, "发布接入插件", code, 200, body)
	sh := getScript(t, ts.URL, "/setup.sh")
	for _, want := range []string{"node -e", "/api/device/code", "/api/device/token", "--token", "DSH_EE_SETUP_INTERACTIVE", "token.pending", `if [ "$DEVICE_LOGIN" -eq 0 ]`, `msg_saved_pending "$PENDING_FILE"`, `msg_saved_env "$ENV_FILE"`, "令牌格式不对", `PROFILES="${DSH_PROFILES:-`} {
		if !strings.Contains(sh, want) {
			t.Errorf("setup.sh 缺少 %q", want)
		}
	}
	if bad := regexp.MustCompile(`\$[A-Za-z_][A-Za-z0-9_]*[^\x00-\x7F]`).FindString(sh); bad != "" {
		t.Errorf("变量后紧跟非 ASCII 字符，必须用花括号：%q", bad)
	}
	file := filepath.Join(t.TempDir(), "setup.sh")
	if err := os.WriteFile(file, []byte(sh), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("sh", "-n", file).CombinedOutput(); err != nil {
		t.Fatalf("sh -n：%v\n%s", err, out)
	}

	ps := getScript(t, ts.URL, "/setup.ps1")
	for _, want := range []string{"param([string]$Token)", "Invoke-RestMethod", "Start-Process", "icacls $Pending", "token.pending", "Setup-DshEE -Token $tokenArg\n", "$arg -like '-Token=*'", "$Token -notmatch", "$Profiles = @(", "Get-Content $EnvFile -Encoding UTF8", "Get-Content $NpmrcFile -Encoding UTF8"} {
		if !strings.Contains(ps, want) {
			t.Errorf("setup.ps1 缺少 %q", want)
		}
	}
	// Windows PowerShell 5 的 Set-Content -Encoding utf8 会写 BOM；Node 的 process.loadEnvFile
	// 会把 BOM 算进变量名，导致 .env 里的 DSHEE_TOKEN 读不到。
	if strings.Contains(ps, "Set-Content -Path $EnvFile") {
		t.Error("setup.ps1 写 .env 不能用 Set-Content -Encoding utf8（PowerShell 5 会加 BOM）")
	}
	if !strings.Contains(ps, "[System.IO.File]::WriteAllLines($EnvFile") {
		t.Error("setup.ps1 写 .env 应该用 [System.IO.File]::WriteAllLines，避免 BOM")
	}
	if !strings.Contains(ps, "icacls $EnvFile") {
		t.Error("setup.ps1 写 .env 后应该收紧 ACL，和 .npmrc / token.pending 一致")
	}
	if !strings.Contains(ps, `Write-Host "以后在 $Base/hub`) {
		t.Error("setup.ps1 的收尾提示应该用双引号，否则 $Base 不会展开")
	}
}

// setup.sh 是多语言的：?lang= 与 Accept-Language 在生成时选定语言；都不带时脚本在员工机器上
// 按 LANG 选（zh* → 中文，其它 → 英文），命令、路径与退出码不变。
func TestSetupScriptLanguage(t *testing.T) {
	ts, admin := setupScriptServer(t)
	// 发布接入插件，让脚本走完整分支（含 CONNECT_PKG 收尾提示）。
	code, body := admin.do("POST", "/api/admin/connect/publish", map[string]bool{"force": true}, nil)
	expect(t, "发布接入插件", code, 200, body)

	han := regexp.MustCompile(`[\x{4e00}-\x{9fff}]`)
	shSyntax := func(name, script string) string {
		t.Helper()
		file := filepath.Join(t.TempDir(), name)
		if err := os.WriteFile(file, []byte(script), 0o700); err != nil {
			t.Fatal(err)
		}
		if out, err := exec.Command("sh", "-n", file).CombinedOutput(); err != nil {
			t.Fatalf("sh -n %s：%v\n%s", name, err, out)
		}
		return file
	}

	en := getScriptLang(t, ts.URL, "/setup.sh?lang=en", "")
	if m := han.FindString(en); m != "" {
		t.Fatalf("?lang=en 的 setup.sh 不应含中文提示，找到 %q", m)
	}
	for _, want := range []string{"Usage: sh -s -- --token <token>", "Invalid token format", "Node.js / npm not found", "Device sign-in timed out", "Signed in as", "Installed"} {
		if !strings.Contains(en, want) {
			t.Errorf("?lang=en 脚本缺少 %q", want)
		}
	}
	shSyntax("setup-en.sh", en)

	zh := getScriptLang(t, ts.URL, "/setup.sh?lang=zh", "")
	for _, want := range []string{"令牌格式不对", "没有找到 Node.js / npm", "请在浏览器中确认登录", "管理员还没有发布 DSH 接入插件", "已在 DSH 的"} {
		if !strings.Contains(zh, want) {
			t.Errorf("?lang=zh 脚本缺少 %q", want)
		}
	}
	shSyntax("setup-zh.sh", zh)

	// Accept-Language 与 ?lang= 等价；curl 默认不带该头。
	if han.MatchString(getScriptLang(t, ts.URL, "/setup.sh", "en-US,en;q=0.9,zh;q=0.8")) {
		t.Error("Accept-Language: en 时 setup.sh 不应含中文提示")
	}
	if !strings.Contains(getScriptLang(t, ts.URL, "/setup.sh", "zh-CN,zh;q=0.9"), "令牌格式不对") {
		t.Error("Accept-Language: zh 时 setup.sh 应输出中文提示")
	}

	// 默认（curl | sh）：脚本自带运行端 LANG 判断，两种语言的文案都在。
	auto := getScript(t, ts.URL, "/setup.sh")
	for _, want := range []string{`case "${LC_ALL:-${LANG:-}}"`, "令牌格式不对", "Invalid token format"} {
		if !strings.Contains(auto, want) {
			t.Errorf("默认脚本缺少 %q", want)
		}
	}
	shSyntax("setup-auto.sh", auto)

	// 真正执行脚本：参数校验阶段（Node 检查）就会输出提示，不需要联网或 Node。
	run := func(script, lang string) string {
		t.Helper()
		file := filepath.Join(t.TempDir(), "setup-run.sh")
		if err := os.WriteFile(file, []byte(script), 0o700); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("/bin/sh", file, "--token", "ee_test")
		// PATH 指向空目录：脚本在“找不到 Node.js / npm”处停下，只验证语言选择。
		cmd.Env = []string{"PATH=" + t.TempDir(), "HOME=" + t.TempDir(), "LANG=" + lang}
		out, _ := cmd.CombinedOutput()
		return string(out)
	}
	if out := run(auto, "zh_CN.UTF-8"); !strings.Contains(out, "没有找到 Node.js / npm") {
		t.Errorf("LANG=zh_CN 时应输出中文提示：%s", out)
	}
	if out := run(auto, "C"); !strings.Contains(out, "Node.js / npm not found") {
		t.Errorf("LANG=C 时应输出英文提示：%s", out)
	}
	// 生成时指定了语言，就不再随员工机器的 LANG 变化。
	if out := run(en, "zh_CN.UTF-8"); han.MatchString(out) || !strings.Contains(out, "Node.js / npm not found") {
		t.Errorf("?lang=en 应固定英文：%s", out)
	}
	if out := run(zh, "C"); !strings.Contains(out, "没有找到 Node.js / npm") {
		t.Errorf("?lang=zh 应固定中文：%s", out)
	}
}

func TestSetupScriptQuotes(t *testing.T) {
	ts, _ := setupScriptServer(t, "https://example.com/o'clock")
	sh := getScript(t, ts.URL, "/setup.sh")
	if !strings.Contains(sh, `'https://example.com/o'\''clock/npm/'`) {
		t.Fatal("setup.sh 没有正确转义单引号")
	}
	file := filepath.Join(t.TempDir(), "quoted.sh")
	if err := os.WriteFile(file, []byte(sh), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("sh", "-n", file).CombinedOutput(); err != nil {
		t.Fatalf("含引号脚本 sh -n：%v\n%s", err, out)
	}
	ps := getScript(t, ts.URL, "/setup.ps1")
	if !strings.Contains(ps, `'https://example.com/o''clock/npm/'`) {
		t.Fatal("setup.ps1 没有正确转义单引号")
	}
}

// 命令行路线：配置只写进 profile 的 .npmrc，不调用 npm config，也不改 ~/.npmrc。
func TestSetupCLIWritesProfileNpmrc(t *testing.T) {
	ts, admin := setupScriptServer(t)
	code, body := admin.do("POST", "/api/admin/connect/publish", map[string]bool{"force": true}, nil)
	expect(t, "发布接入插件", code, 200, body)
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	log := filepath.Join(root, "calls.log")
	record := `printf '%s %s\n' "$(basename "$0")" "$*" >> "$FAKE_LOG"` + "\n"
	for _, name := range []string{"npm", "node", "dsh", "pnpm"} {
		writeFakeExecutable(t, filepath.Join(bin, name), record)
	}
	writeFakeExecutable(t, filepath.Join(bin, "uname"), "printf 'Linux\\n'\n")
	writeFakeExecutable(t, filepath.Join(bin, "curl"), record+"cat >> \"$FAKE_LOG\"\n")
	home := filepath.Join(root, "home")
	dshHome := filepath.Join(root, "dsh-home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	scriptPath := filepath.Join(root, "setup.sh")
	if err := os.WriteFile(scriptPath, []byte(getScript(t, ts.URL, "/setup.sh")), 0o700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", scriptPath, "--token", "ee_at_cli")
	cmd.Env = []string{"HOME=" + home, "DSH_HOME=" + dshHome, "DSH_PROFILES=web default", "PATH=" + bin + ":/usr/bin:/bin", "FAKE_LOG=" + log}
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("命令行脚本执行失败：%v\n%s", err, output)
	}
	calls, _ := os.ReadFile(log)
	if strings.Contains(string(calls), "npm config") {
		t.Fatalf("默认不应写全局 npm 配置：%s", calls)
	}
	// 令牌经 stdin 交给 curl，不出现在 curl 的参数里
	for _, line := range strings.Split(string(calls), "\n") {
		if strings.HasPrefix(line, "curl ") && strings.Contains(line, "ee_at_cli") {
			t.Fatalf("令牌出现在 curl 参数里：%s", line)
		}
	}
	if !strings.Contains(string(calls), `header = "Authorization: Bearer ee_at_cli"`) {
		t.Fatalf("curl 应从 stdin 读到令牌：%s", calls)
	}
	for _, profile := range []string{"web", "default"} {
		data, err := os.ReadFile(filepath.Join(dshHome, "profiles", profile, ".npmrc"))
		if err != nil {
			t.Fatalf("%s profile 没有 .npmrc：%v", profile, err)
		}
		if !strings.Contains(string(data), "@company:registry="+ts.URL+"/npm/") || !strings.Contains(string(data), ":_authToken=ee_at_cli") {
			t.Fatalf("%s profile 的 .npmrc 不对：%s", profile, data)
		}
		if !strings.Contains(string(calls), "dsh plugin --profile "+profile+" add @company/dsh-connect@latest") {
			t.Fatalf("应在 %s profile 安装接入插件：%s", profile, calls)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".npmrc")); !os.IsNotExist(err) {
		t.Fatalf("不应创建 ~/.npmrc：%v", err)
	}
}

// 免登录的接入插件 tarball：只给当前包已发布的版本，/api/connect 返回带版本号的地址。
func TestConnectTarball(t *testing.T) {
	ts, admin := setupScriptServer(t)
	code, body := admin.do("POST", "/api/admin/connect/publish", map[string]bool{"force": true}, nil)
	expect(t, "发布接入插件", code, 200, body)
	code, body = admin.do("GET", "/api/connect", nil, nil)
	expect(t, "接入信息", code, 200, body)
	info := decodeAs[struct {
		Version    string `json:"version"`
		TarballURL string `json:"tarball_url"`
	}](t, body)
	if info.TarballURL != ts.URL+"/dl/connect/dsh-connect-"+info.Version+".tgz" {
		t.Fatalf("tarball_url 不对：%+v", info)
	}
	resp, err := http.Get(info.TarballURL)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("匿名下载接入插件：%d %s", resp.StatusCode, raw)
	}
	files := untar(t, raw)
	if !strings.Contains(files["package/cordis.patch.yml"], "scopes:\n          - '@company'") {
		t.Fatalf("接入插件应带上企业作用域：%s", files["package/cordis.patch.yml"])
	}
	for _, path := range []string{"/dl/connect/dsh-connect-9.9.9.tgz", "/dl/connect/other.tgz", "/dl/connect/dsh-connect-.tgz"} {
		resp, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("%s 应返回 404，实际 %d", path, resp.StatusCode)
		}
	}
}
