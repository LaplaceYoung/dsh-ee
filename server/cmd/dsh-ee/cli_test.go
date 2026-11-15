package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"
)

const exampleDir = "../../../extensions/examples/demo-declarative"

func cli(args ...string) (handled bool, code int, out, errOut string) {
	var o, e bytes.Buffer
	handled, code = runCLI(args, &o, &e)
	return handled, code, o.String(), e.String()
}

// hasCJK 判断输出里有没有汉字（英文输出不该有）。
func hasCJK(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

// TestMain 隔离宿主机的语言环境：既有用例都按默认中文断言，新的语言用例自己用 t.Setenv / --lang 指定。
func TestMain(m *testing.M) {
	for _, key := range []string{"DSH_EE_LANG", "LC_ALL", "LANG"} {
		os.Unsetenv(key)
	}
	os.Exit(m.Run())
}

func TestCLI(t *testing.T) {
	// 没有子命令、或不是这里的子命令：交回去启动服务
	if handled, _, _, _ := cli(); handled {
		t.Fatal("没有参数时应启动服务")
	}
	if handled, _, _, _ := cli("serve"); handled {
		t.Fatal("不认识的命令不归这里管")
	}

	// 示例扩展：通过，退出码 0；--json 给出身份、摘要和工具
	_, code, out, _ := cli("extension", "check", exampleDir)
	if code != 0 || !strings.Contains(out, "dsh-ee/demo-itsm@0.1.0") || !strings.Contains(out, "通过：0 个错误，0 个提醒") || !strings.Contains(out, "create_ticket  POST /demo/itsm/tickets  （写操作，调用前要员工确认）") {
		t.Fatalf("示例应通过：%d\n%s", code, out)
	}
	_, code, out, _ = cli("extension", "check", "--json", exampleDir)
	var res struct {
		OK       bool
		Identity string
		Digest   string
		Errors   int
		Tools    []struct {
			Name     string
			ReadOnly bool `json:"readOnly"`
		}
		Results []map[string]string
	}
	if code != 0 || json.Unmarshal([]byte(out), &res) != nil || !res.OK || res.Identity != "dsh-ee/demo-itsm@0.1.0" || len(res.Digest) != 64 || len(res.Tools) != 4 || res.Results == nil {
		t.Fatalf("--json 输出不对：%d\n%s", code, out)
	}

	// extension check --help 给用法，不当成不认识的参数
	if _, code, out, errOut := cli("extension", "check", "--help"); code != 0 || !strings.Contains(out, "dsh-ee extension check") || strings.Contains(errOut, "不认识") {
		t.Fatalf("--help 应给用法：%d\n%s\n%s", code, out, errOut)
	}

	// 弄坏的扩展：退出码 1，每条结果都有位置和改法
	dir := t.TempDir()
	for _, name := range []string{"dsh-ee.extension.json", "openapi.yaml"} {
		raw, err := os.ReadFile(filepath.Join(exampleDir, name))
		if err != nil {
			t.Fatal(err)
		}
		if name == "dsh-ee.extension.json" {
			raw = bytes.Replace(raw, []byte(`"name": "create_ticket",`), []byte(`"name": "create_ticket", "readOnly": true,`), 1)
		}
		if err := os.WriteFile(filepath.Join(dir, name), raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	_, code, out, _ = cli("extension", "check", dir, "--json")
	if code != 1 || json.Unmarshal([]byte(out), &res) != nil || res.OK || res.Errors != 1 || len(res.Results) != 1 {
		t.Fatalf("把写操作标成只读应报错：%d\n%s", code, out)
	}
	if d := res.Results[0]; d["code"] != "tool.readonly_suspect" || d["severity"] != "error" || d["where"] != "dsh-ee.extension.json#/tools/3" || !strings.Contains(d["message"], "POST") || !strings.Contains(d["fix"], "tools[3]") {
		t.Fatalf("结果要说清楚是哪个工具、怎么改：%v", d)
	}
	_, code, out, _ = cli("extension", "check", dir)
	if code != 1 || !strings.Contains(out, "错误  tool.readonly_suspect") || !strings.Contains(out, "改法：") || !strings.Contains(out, "没通过：1 个错误") {
		t.Fatalf("给人看的输出：%d\n%s", code, out)
	}

	// 用法不对：退出码 2，说明怎么用
	for _, args := range [][]string{{"extension"}, {"extension", "check"}, {"extension", "check", "a", "b"}, {"extension", "check", exampleDir, "--verbose"}, {"extension", "check", filepath.Join(dir, "nope")}, {"schema"}, {"schema", "get"}, {"schema", "dump"}} {
		if _, code, _, errOut := cli(args...); code != 2 || errOut == "" {
			t.Fatalf("%v：应是退出码 2 并说明用法，实际 %d %q", args, code, errOut)
		}
	}

	// 规格
	_, code, out, _ = cli("schema", "list")
	if code != 0 || !strings.Contains(out, "manifest") || !strings.Contains(out, "errors") {
		t.Fatalf("schema list：%d\n%s", code, out)
	}
	_, code, out, _ = cli("schema", "get", "manifest")
	var schema map[string]any
	if code != 0 || json.Unmarshal([]byte(out), &schema) != nil || schema["title"] == nil {
		t.Fatalf("schema get manifest：%d\n%s", code, out)
	}
	_, code, out, _ = cli("schema", "get", "errors")
	if code != 0 || !strings.Contains(out, "tool.readonly_suspect") {
		t.Fatalf("schema get errors：%d\n%s", code, out)
	}
	if _, code, _, errOut := cli("schema", "get", "nope"); code != 1 || !strings.Contains(errOut, "schema list") {
		t.Fatalf("不存在的规格：%d %q", code, errOut)
	}
	if _, code, out, _ := cli("help"); code != 0 || !strings.Contains(out, "extension check") {
		t.Fatalf("help：%d\n%s", code, out)
	}
}

// ---- package check / doctor ----

// cleanCLIEnv 把地址、令牌与本机配置隔离到临时目录，避免读到跑测试机器上的真实 .npmrc。
func cleanCLIEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DSH_EE_URL", "")
	t.Setenv("DSH_EE_TOKEN", "")
	t.Setenv("DSH_HOME", t.TempDir())
	t.Setenv("NPM_CONFIG_USERCONFIG", filepath.Join(t.TempDir(), "npmrc"))
}

const passCheckJSON = `{"name":"@company/demo","version":"1.0.0","is_plugin":true,"can_submit":true,"will_publish":false,"requires_review":true,
"scan":[{"key":"bundle","level":"pass","params":{"path":"cordis.patch.yml"}},{"key":"size","level":"warn","params":{"size":3145728,"unpacked":10485760,"files":42}}],
"conflicts":[]}`

func writeTempFile(t *testing.T, name string, body []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCLIPackageCheck(t *testing.T) {
	cleanCLIEnv(t)
	var gotAuth, gotType string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/me/packages/check" {
			http.NotFound(w, r)
			return
		}
		gotAuth = r.Header.Get("Authorization")
		gotType = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		switch string(gotBody) {
		case "conflict":
			_, _ = io.WriteString(w, `{"name":"@company/demo","version":"1.0.0","is_plugin":true,"can_submit":false,"will_publish":false,"requires_review":false,"scan":[],"conflicts":[{"key":"exists","params":{"name":"@company/demo","version":"1.0.0"}}]}`)
		case "scanfail":
			_, _ = io.WriteString(w, `{"name":"@company/demo","version":"1.0.0","is_plugin":true,"can_submit":true,"will_publish":false,"requires_review":true,"scan":[{"key":"secrets","level":"fail","params":{"count":2,"files":"src/index.js（openai-key）、.env（env-file）"}}],"conflicts":[]}`)
		default:
			_, _ = io.WriteString(w, passCheckJSON)
		}
	}))
	defer srv.Close()

	tgz := writeTempFile(t, "demo-1.0.0.tgz", []byte("fake-tgz"))

	// 正常：上传原文件、带 Bearer，退出码 0，人类可读摘要
	_, code, out, errOut := cli("package", "check", tgz, "--server", srv.URL, "--token", "ee_at_test")
	if code != 0 {
		t.Fatalf("预检应通过：%d\n%s\n%s", code, out, errOut)
	}
	for _, want := range []string{"预检 @company/demo@1.0.0（DSH 插件）", "自动检查：1 项通过，1 项提醒", "包 3.0 MB（解包 10.0 MB，42 个文件）", "结论：可以提交，会进入人工审核"} {
		if !strings.Contains(out, want) {
			t.Fatalf("输出缺少 %q：\n%s", want, out)
		}
	}
	if gotAuth != "Bearer ee_at_test" || gotType != "application/octet-stream" || string(gotBody) != "fake-tgz" {
		t.Fatalf("请求不对：auth=%q type=%q body=%q", gotAuth, gotType, gotBody)
	}

	// --json 输出服务端原始结果
	_, code, out, _ = cli("package", "check", "--json", tgz, "--server", srv.URL, "--token", "ee_at_test")
	var res struct {
		Name      string `json:"name"`
		CanSubmit bool   `json:"can_submit"`
		Scan      []struct {
			Level string `json:"level"`
		} `json:"scan"`
	}
	if code != 0 || json.Unmarshal([]byte(out), &res) != nil || !res.CanSubmit || res.Name != "@company/demo" || len(res.Scan) != 2 {
		t.Fatalf("--json 输出不对：%d\n%s", code, out)
	}

	// 有发布冲突：退出码 1，说明不能提交
	conflict := writeTempFile(t, "conflict.tgz", []byte("conflict"))
	_, code, out, _ = cli("package", "check", conflict, "--server", srv.URL, "--token", "ee_at_test")
	if code != 1 || !strings.Contains(out, "发布冲突") || !strings.Contains(out, "不能提交") || !strings.Contains(out, "已经存在") {
		t.Fatalf("发布冲突应退出 1：%d\n%s", code, out)
	}

	// 自动检查失败：退出码 1，但仍可提交审核
	scanFail := writeTempFile(t, "scanfail.tgz", []byte("scanfail"))
	_, code, out, _ = cli("package", "check", scanFail, "--server", srv.URL, "--token", "ee_at_test")
	if code != 1 || !strings.Contains(out, "失败  发现 2 处疑似密钥") || !strings.Contains(out, "不能自动上架") {
		t.Fatalf("自动检查失败应退出 1：%d\n%s", code, out)
	}

	// 服务端不接受这个包：退出码 1（不是网络/认证问题）
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":"不是 npm pack 生成的 .tgz："}`)
	}))
	defer bad.Close()
	_, code, _, errOut = cli("package", "check", tgz, "--server", bad.URL, "--token", "ee_at_test")
	if code != 1 || !strings.Contains(errOut, "服务端不接受这个包") || !strings.Contains(errOut, "不是 npm pack") {
		t.Fatalf("坏包应退出 1：%d %q", code, errOut)
	}

	// 令牌被拒：退出码 3，并提示去哪建令牌
	deny := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":"令牌无效或已过期","code":"token-invalid"}`)
	}))
	defer deny.Close()
	_, code, _, errOut = cli("package", "check", tgz, "--server", deny.URL, "--token", "ee_at_test")
	if code != 3 || !strings.Contains(errOut, "registry:publish") || !strings.Contains(errOut, "tokens") {
		t.Fatalf("令牌被拒应退出 3：%d %q", code, errOut)
	}

	// 服务端 5xx：退出码 3
	sick := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(w, "bad gateway")
	}))
	defer sick.Close()
	_, code, _, errOut = cli("package", "check", tgz, "--server", sick.URL, "--token", "ee_at_test")
	if code != 3 || !strings.Contains(errOut, "调用预检接口失败") {
		t.Fatalf("5xx 应退出 3：%d %q", code, errOut)
	}

	// 用法错误：退出码 2
	dir := t.TempDir()
	for _, args := range [][]string{
		{"package"},
		{"package", "check"},
		{"package", "check", tgz, tgz},
		{"package", "check", tgz, "--verbose"},
		{"package", "check", filepath.Join(dir, "nope.tgz")},
		{"package", "check", dir},
	} {
		if _, code, _, errOut := cli(args...); code != 2 || errOut == "" {
			t.Fatalf("%v 应退出 2：%d %q", args, code, errOut)
		}
	}
	if _, code, out, _ := cli("package", "check", "--help"); code != 0 || !strings.Contains(out, "dsh-ee package check") {
		t.Fatalf("package check --help：%d\n%s", code, out)
	}

	// 没有地址和令牌：退出码 2，并说清怎么配
	_, code, _, errOut = cli("package", "check", tgz)
	if code != 2 || !strings.Contains(errOut, "不知道 dsh-ee 的地址") {
		t.Fatalf("缺地址应退出 2：%d %q", code, errOut)
	}

	// 地址在 .npmrc，令牌也在 .npmrc：不用参数也能用
	u, _ := url.Parse(srv.URL)
	npmrc := writeTempFile(t, "npmrc", []byte("registry="+srv.URL+"/npm/\n@company:registry="+srv.URL+"/npm/\n//"+u.Host+"/npm/:_authToken=ee_at_from_npmrc\n"))
	t.Setenv("NPM_CONFIG_USERCONFIG", npmrc)
	_, code, _, errOut = cli("package", "check", tgz)
	if code != 0 || gotAuth != "Bearer ee_at_from_npmrc" {
		t.Fatalf("应能从 .npmrc 读地址与令牌：%d %q auth=%q", code, errOut, gotAuth)
	}

	// 令牌在 $DSH_HOME/dsh-ee/token：同样兜底
	home := t.TempDir()
	t.Setenv("DSH_HOME", home)
	t.Setenv("NPM_CONFIG_USERCONFIG", filepath.Join(t.TempDir(), "none"))
	if err := os.MkdirAll(filepath.Join(home, "dsh-ee"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "dsh-ee", "token"), []byte("ee_at_from_file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, code, _, errOut = cli("package", "check", tgz, "--server", srv.URL)
	if code != 0 || gotAuth != "Bearer ee_at_from_file" {
		t.Fatalf("应从 $DSH_HOME/dsh-ee/token 读令牌：%d %q auth=%q", code, errOut, gotAuth)
	}
}

// doctorStub 是只读诊断用的假 dsh-ee；记录收到的请求，按开关返回不同状态。
type doctorStub struct {
	server       *httptest.Server
	whoamiStatus int
	methods      []string
	paths        []string
}

func newDoctorStub(t *testing.T) *doctorStub {
	t.Helper()
	d := &doctorStub{whoamiStatus: http.StatusOK}
	d.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		d.methods = append(d.methods, r.Method)
		d.paths = append(d.paths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/healthz":
			_, _ = io.WriteString(w, `{"status":"ok","version":"0.1.0-test"}`)
		case "/npm/-/ping":
			_, _ = io.WriteString(w, `{}`)
		case "/npm/-/whoami":
			if d.whoamiStatus != http.StatusOK {
				w.WriteHeader(d.whoamiStatus)
				_, _ = io.WriteString(w, `{"error":"登录已失效"}`)
				return
			}
			_, _ = io.WriteString(w, `{"username":"alice"}`)
		case "/api/dsh/whoami":
			_, _ = io.WriteString(w, `{"username":"alice","department":"研发效能部","scopes":["agent","models"],"expires_at":"2030-01-01T00:00:00Z"}`)
		case "/api/me/packages/check":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":"请求体必须是 .tgz（application/octet-stream）"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(d.server.Close)
	return d
}

func TestCLIDoctor(t *testing.T) {
	cleanCLIEnv(t)
	oldCommand := doctorCommand
	t.Cleanup(func() { doctorCommand = oldCommand })
	doctorCommand = func(name string, args ...string) (string, error) {
		switch name {
		case "node":
			return "v24.22.1", nil
		case "dsh":
			return "0.2.2", nil
		}
		return "", os.ErrNotExist
	}

	stub := newDoctorStub(t)
	u, _ := url.Parse(stub.server.URL)
	npmrcPath := writeTempFile(t, "npmrc", []byte("registry="+stub.server.URL+"/npm/\n@company:registry="+stub.server.URL+"/npm/\n//"+u.Host+"/npm/:_authToken=ee_at_doctor\n"))
	before, _ := os.ReadFile(npmrcPath)
	t.Setenv("NPM_CONFIG_USERCONFIG", npmrcPath)

	_, code, out, errOut := cli("doctor", "--server", stub.server.URL, "--token", "ee_at_doctor")
	if code != 0 {
		t.Fatalf("doctor 应通过：%d\n%s\n%s", code, out, errOut)
	}
	for _, want := range []string{"dsh-ee doctor（只读诊断", "✓ 服务连通：版本 0.1.0-test", "✓ 令牌：有效，用户名 alice", "✓ 发布权限", "✓ Node：v24.22.1", "✓ DSH：0.2.2", "结论：全部正常"} {
		if !strings.Contains(out, want) {
			t.Fatalf("doctor 输出缺少 %q：\n%s", want, out)
		}
	}
	if strings.Contains(out, "下一步") {
		t.Fatalf("全部正常时不该有下一步：\n%s", out)
	}

	// 只读：探测只用 GET，唯一的 POST 是不落库的预检
	for i, method := range stub.methods {
		path := stub.paths[i]
		if path == "/api/me/packages/check" {
			if method != "POST" {
				t.Fatalf("预检探测应是 POST：%s %s", method, path)
			}
			continue
		}
		if method != "GET" {
			t.Fatalf("doctor 只应发只读请求：%s %s", method, path)
		}
	}
	after, _ := os.ReadFile(npmrcPath)
	if string(before) != string(after) {
		t.Fatal("doctor 不应该修改 .npmrc")
	}

	// --json 给机器看
	_, code, out, _ = cli("doctor", "--json", "--server", stub.server.URL, "--token", "ee_at_doctor")
	var rep doctorReport
	if code != 0 || json.Unmarshal([]byte(out), &rep) != nil || !rep.OK || (rep.Summary != "全部正常" && rep.Summary != "all checks passed") || len(rep.Checks) < 8 {
		t.Fatalf("doctor --json 不对：%d\n%s", code, out)
	}
	ids := map[string]string{}
	for _, c := range rep.Checks {
		ids[c.ID] = c.Status
	}
	for _, id := range []string{"endpoint", "server", "token", "publish-scope", "npm-source", "npmrc", "node", "dsh"} {
		if ids[id] != "ok" {
			t.Fatalf("检查项 %s 应是 ok：%v (status=%q)", id, ids, ids[id])
		}
	}

	// --lang=en：--json 的人读字段（summary/title）跟着语言走，字段名与结构不变；人读输出不含中文
	_, code, out, _ = cli("doctor", "--lang=en", "--json", "--server", stub.server.URL, "--token", "ee_at_doctor")
	var repEN doctorReport
	if code != 0 || json.Unmarshal([]byte(out), &repEN) != nil || !repEN.OK || repEN.Summary != "all checks passed" || len(repEN.Checks) < 8 {
		t.Fatalf("doctor --lang=en --json 不对：%d\n%s", code, out)
	}
	for _, c := range repEN.Checks {
		if hasCJK(c.Title) || hasCJK(c.Detail) {
			t.Fatalf("doctor --lang=en 的检查项仍是中文：%+v", c)
		}
	}
	_, code, out, _ = cli("doctor", "--lang=en", "--server", stub.server.URL, "--token", "ee_at_doctor")
	if code != 0 || hasCJK(out) {
		t.Fatalf("doctor --lang=en 输出不应有中文：%d\n%s", code, out)
	}

	// 令牌失效：退出 1，给下一步建议
	stub.whoamiStatus = http.StatusUnauthorized
	_, code, out, _ = cli("doctor", "--server", stub.server.URL, "--token", "ee_at_bad")
	if code != 1 || !strings.Contains(out, "无效、已过期或已吊销") || !strings.Contains(out, "下一步：") {
		t.Fatalf("令牌失效应退出 1 并给建议：%d\n%s", code, out)
	}

	// 什么都没配：地址失败 + Node/DSH 提醒，退出 1
	doctorCommand = func(name string, args ...string) (string, error) { return "", os.ErrNotExist }
	cleanCLIEnv(t)
	_, code, out, _ = cli("doctor")
	if code != 1 || !strings.Contains(out, "✗ dsh-ee 地址：没有找到服务地址") || !strings.Contains(out, "没有找到 node 命令") || !strings.Contains(out, "下一步：") {
		t.Fatalf("缺配置应退出 1：%d\n%s", code, out)
	}

	// 用法错误：退出码 2
	for _, args := range [][]string{{"doctor", "extra"}, {"doctor", "--verbose"}} {
		if _, code, _, errOut := cli(args...); code != 2 || errOut == "" {
			t.Fatalf("%v 应退出 2：%d %q", args, code, errOut)
		}
	}
	if _, code, out, _ := cli("doctor", "--help"); code != 0 || !strings.Contains(out, "dsh-ee doctor") {
		t.Fatalf("doctor --help：%d\n%s", code, out)
	}

	// 主 help 里能看到两个新命令
	if _, code, out, _ := cli("help"); code != 0 || !strings.Contains(out, "package check") || !strings.Contains(out, "doctor") {
		t.Fatalf("help 缺少新命令：%d\n%s", code, out)
	}
}

func TestVersionMajor(t *testing.T) {
	cases := map[string]int{"v24.22.1": 24, "22.0.0": 22, " 18.19.1\n": 18, "v20.0.0-rc.1": 20, "dev": -1, "": -1}
	for in, want := range cases {
		if got := versionMajor(in); got != want {
			t.Errorf("versionMajor(%q)=%d，想要 %d", in, got, want)
		}
	}
}

func TestNpmrcHelpers(t *testing.T) {
	path := writeTempFile(t, "npmrc", []byte("# 注释\nregistry=https://ee.acme.com/npm/\n@acme:registry=https://ee.acme.com/npm/\n//ee.acme.com/npm/:_authToken=ee_at_x\nother=1\n"))
	f := readNpmrc(path)
	if !f.exists || f.server() != "https://ee.acme.com" || f.anyAuthToken() != "ee_at_x" {
		t.Fatalf("npmrc 解析不对：%+v", f)
	}
	if authTokenKey(registryURL("https://ee.acme.com")) != "//ee.acme.com/npm/:_authToken" {
		t.Fatalf("authTokenKey 不对：%q", authTokenKey(registryURL("https://ee.acme.com")))
	}
	if f := readNpmrc(filepath.Join(t.TempDir(), "missing")); f.exists || f.server() != "" || f.anyAuthToken() != "" {
		t.Fatalf("缺文件时应是空配置：%+v", f)
	}
}

// ---- 语言：CLI 只做中英双语（P5 五语化的 CLI 部分）----

func TestCLILang(t *testing.T) {
	// 默认（没有任何语言设置）是中文：既有脚本与测试都按这个约定
	if _, code, out, _ := cli("help"); code != 0 || !hasCJK(out) || !strings.Contains(out, "用法：") {
		t.Fatalf("默认应输出中文 help：%d\n%s", code, out)
	}

	// --lang=en：help 整段没有中文
	_, code, out, errOut := cli("--lang=en", "help")
	if code != 0 || hasCJK(out) || hasCJK(errOut) || !strings.Contains(out, "Usage:") {
		t.Fatalf("--lang=en help 应是英文：%d\n%s\n%s", code, out, errOut)
	}

	// --lang=en 也可以写在子命令参数里（不要求放在第一个参数）
	_, code, _, errOut = cli("extension", "check", filepath.Join(t.TempDir(), "nope"), "--lang=en")
	if code != 2 || hasCJK(errOut) || !strings.Contains(errOut, "is not a directory") {
		t.Fatalf("--lang=en 的目录错误应是英文：%d %q", code, errOut)
	}
	_, code, _, errOut = cli("schema", "get", "nope", "--lang=en")
	if code != 1 || hasCJK(errOut) || !strings.Contains(errOut, "no schema named") {
		t.Fatalf("--lang=en 的规格错误应是英文：%d %q", code, errOut)
	}

	// version 只打印版本号，但走 --lang=en 入口也不该有中文
	_, code, out, _ = cli("--lang=en", "version")
	if code != 0 || hasCJK(out) || strings.TrimSpace(out) != version {
		t.Fatalf("--lang=en version：%d %q", code, out)
	}

	// LANG 环境变量生效：英文 locale → 英文
	t.Setenv("LANG", "en_US.UTF-8")
	if _, code, out, _ := cli("help"); code != 0 || hasCJK(out) || !strings.Contains(out, "Usage:") {
		t.Fatalf("LANG=en_US.UTF-8 应用英文：%d\n%s", code, out)
	}
	// 显式 --lang=zh 覆盖 LANG=en
	if _, code, out, _ := cli("--lang=zh", "help"); code != 0 || !hasCJK(out) || !strings.Contains(out, "用法：") {
		t.Fatalf("--lang=zh 应覆盖 LANG：%d\n%s", code, out)
	}
	// DSH_EE_LANG 优先于 LANG
	t.Setenv("DSH_EE_LANG", "zh")
	if _, code, out, _ := cli("help"); code != 0 || !hasCJK(out) {
		t.Fatalf("DSH_EE_LANG=zh 应覆盖 LANG：%d\n%s", code, out)
	}
	// 中文 locale → 中文
	t.Setenv("DSH_EE_LANG", "")
	t.Setenv("LANG", "zh_CN.UTF-8")
	if _, code, out, _ := cli("help"); code != 0 || !hasCJK(out) {
		t.Fatalf("LANG=zh_CN.UTF-8 应用中文：%d\n%s", code, out)
	}

	// 不认识的 --lang：用法错误，退出码 2
	if _, code, _, errOut := cli("--lang=fr", "help"); code != 2 || errOut == "" {
		t.Fatalf("非法 --lang 应退出 2：%d %q", code, errOut)
	}
}
