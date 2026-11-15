package config

import (
	"strings"
	"testing"
)

// 未设置 DSH_EE_CSP 时默认启用内置基线策略（enforce），不是 off。
func TestCSPDefaultsEnforce(t *testing.T) {
	setRequiredEnv(t)
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.CSP != CSPEnforce {
		t.Fatalf("DSH_EE_CSP 未设置时应默认 enforce，得到 %q", c.CSP)
	}
	if c.CSP != CSPDefaultMode {
		t.Fatalf("CSPDefaultMode = %q，与默认解析结果 %q 不一致", CSPDefaultMode, c.CSP)
	}
	if c.CSPReportURI != "" {
		t.Fatalf("报告地址默认应为空，得到 %q", c.CSPReportURI)
	}
}

// 显式 off 仍然关闭完整策略：升级后要保留旧行为（只加 frame-ancestors）就设它。
func TestCSPExplicitOff(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("DSH_EE_CSP", "off")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.CSP != CSPOff {
		t.Fatalf("DSH_EE_CSP=off 应解析为 off，得到 %q", c.CSP)
	}
}

func TestCSPModes(t *testing.T) {
	for _, c := range []struct{ env, want string }{
		{"off", CSPOff},
		{"report", CSPReport},
		{"enforce", CSPEnforce},
		{"  ENFORCE  ", CSPEnforce},
		{"Report", CSPReport},
	} {
		setRequiredEnv(t)
		t.Setenv("DSH_EE_CSP", c.env)
		cfg, err := Load()
		if err != nil {
			t.Fatalf("DSH_EE_CSP=%q: %v", c.env, err)
		}
		if cfg.CSP != c.want {
			t.Fatalf("DSH_EE_CSP=%q 解析成 %q，希望 %q", c.env, cfg.CSP, c.want)
		}
	}
}

// 写错模式名时不能静默退回 off：管理员会以为 CSP 已经生效。
func TestCSPModeInvalidFailsClosed(t *testing.T) {
	for _, bad := range []string{"on", "true", "1", "block", "report-only"} {
		setRequiredEnv(t)
		t.Setenv("DSH_EE_CSP", bad)
		if _, err := Load(); err == nil || !strings.Contains(err.Error(), "DSH_EE_CSP") {
			t.Errorf("DSH_EE_CSP=%q 应该让 Load 失败，实际 err=%v", bad, err)
		}
	}
}

func TestCSPReportURI(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("DSH_EE_CSP", "report")
	t.Setenv("DSH_EE_CSP_REPORT_URI", "https://ee.acme.internal/api/csp-report")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.CSPReportURI != "https://ee.acme.internal/api/csp-report" {
		t.Fatalf("报告地址没读到：%q", c.CSPReportURI)
	}
}

func TestCSPReportURIInvalid(t *testing.T) {
	for _, bad := range []string{
		"not-a-url",
		"/api/csp-report",
		"ftp://ee.acme.internal/report",
		"https://",
		"https://ee.acme.internal/a;b",
		"https://ee.acme.internal/a b",
		"https://ee.acme.internal/a?x=1,2",
		"https://ee.acme.internal/\"></script>",
	} {
		setRequiredEnv(t)
		t.Setenv("DSH_EE_CSP", "report")
		t.Setenv("DSH_EE_CSP_REPORT_URI", bad)
		if _, err := Load(); err == nil || !strings.Contains(err.Error(), "DSH_EE_CSP_REPORT_URI") {
			t.Errorf("DSH_EE_CSP_REPORT_URI=%q 应该让 Load 失败，实际 err=%v", bad, err)
		}
	}
}

func TestParseCSPMode(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"", CSPEnforce},
		{"off", CSPOff},
		{"report", CSPReport},
		{"enforce", CSPEnforce},
	} {
		got, err := parseCSPMode(c.in)
		if err != nil || got != c.want {
			t.Fatalf("parseCSPMode(%q) = %q, %v；希望 %q", c.in, got, err, c.want)
		}
	}
	if _, err := parseCSPMode("yes"); err == nil {
		t.Fatal("parseCSPMode 应拒绝非法值")
	}
}
