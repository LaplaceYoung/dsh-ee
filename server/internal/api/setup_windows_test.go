//go:build windows

package api

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestSetupEnvPowerShell 在 Windows 上真正执行 setup.ps1 写 .env 的那段：
// 只断言字符串不够，PowerShell 5 的 Set-Content -Encoding utf8 会加 BOM，
// Node 的 process.loadEnvFile 会把 BOM 当成变量名的一部分。
func TestSetupEnvPowerShell(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, ".env")
	if err := os.WriteFile(envFile, []byte("KEEP=1\r\n# 中文注释\r\n"+connectTokenEnv+"=old\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	script := "$DshDir = " + psQuote(dir) + "\n" +
		"$Token = 'tok-123'\n" +
		"$DeviceLogin = $false\n" +
		"$Pending = " + psQuote(filepath.Join(dir, "token.pending")) + "\n" +
		setupEnvPowerShell(ps1TextZh)
	scriptFile := filepath.Join(dir, "setup-env-test.ps1")
	// PowerShell 5 默认按 ANSI 读无 BOM 文件；脚本里有中文，测试脚本本身要带 BOM。
	if err := os.WriteFile(scriptFile, append([]byte{0xEF, 0xBB, 0xBF}, []byte(script)...), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", scriptFile).CombinedOutput()
	if err != nil {
		t.Fatalf("powershell: %v\n%s", err, out)
	}
	data, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}) {
		t.Fatalf(".env 不应带 UTF-8 BOM：%x", data[:3])
	}
	got := strings.ReplaceAll(string(data), "\r\n", "\n")
	if !strings.Contains(got, "KEEP=1\n") || !strings.Contains(got, "# 中文注释\n") {
		t.Fatalf("原有内容（含中文注释）应逐字节保留：%q", got)
	}
	if strings.Count(got, connectTokenEnv+"=") != 1 || !strings.Contains(got, connectTokenEnv+"=tok-123\n") {
		t.Fatalf("令牌应被替换且只出现一次：%q", got)
	}
}

// TestSetupTokenArgPowerShell：`powershell -File setup.ps1 -Token x` 的参数只在脚本级
// $args 里；要真的传进 Setup-DshEE，而不是让脚本自己提示“请使用 -Token”又回到设备登录。
func TestSetupTokenArgPowerShell(t *testing.T) {
	dir := t.TempDir()
	scriptFile := filepath.Join(dir, "token-arg-test.ps1")
	script := setupTokenArgPowerShell() + "Write-Host \"TOKEN=[$tokenArg]\"\n"
	if err := os.WriteFile(scriptFile, []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) string {
		t.Helper()
		cmdArgs := append([]string{"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", scriptFile}, args...)
		out, err := exec.Command("powershell.exe", cmdArgs...).CombinedOutput()
		if err != nil {
			t.Fatalf("powershell: %v\n%s", err, out)
		}
		return string(out)
	}
	if out := run("-Token", "tok-abc"); !strings.Contains(out, "TOKEN=[tok-abc]") {
		t.Fatalf("空格形式 -Token 没有传进去：%s", out)
	}
	if out := run("-Token=tok-def"); !strings.Contains(out, "TOKEN=[tok-def]") {
		t.Fatalf("等号形式 -Token= 没有传进去：%s", out)
	}
	if out := run("-Other", "x"); !strings.Contains(out, "TOKEN=[]") {
		t.Fatalf("没有 -Token 时应为空：%s", out)
	}
}
