package config

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

// setRequiredEnv 设好 Load 必需的两项配置；其余 DSH_EE_* 由各测试按需覆盖。
func setRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DSH_EE_DSN", "root:@tcp(127.0.0.1:4000)/dsh_ee")
	t.Setenv("DSH_EE_SECRET_KEY", base64.StdEncoding.EncodeToString(make([]byte, 32)))
}

func TestLoginIPLimitDefaults(t *testing.T) {
	setRequiredEnv(t)
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.LoginIPLimit != 50 || c.LoginIPWindow != 15*time.Minute {
		t.Fatalf("默认应为 50 次 / 15m，得到 %d / %v", c.LoginIPLimit, c.LoginIPWindow)
	}
	if c.Metrics {
		t.Fatal("metrics 默认应关闭")
	}
}

func TestLoginIPLimitEnv(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("DSH_EE_LOGIN_IP_LIMIT", "7")
	t.Setenv("DSH_EE_LOGIN_IP_WINDOW", "2m")
	t.Setenv("DSH_EE_METRICS", "true")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.LoginIPLimit != 7 || c.LoginIPWindow != 2*time.Minute {
		t.Fatalf("配置没生效：%d / %v", c.LoginIPLimit, c.LoginIPWindow)
	}
	if !c.Metrics {
		t.Fatal("DSH_EE_METRICS=true 应开启 metrics")
	}
}

func TestLoginIPLimitCanBeDisabled(t *testing.T) {
	for _, v := range []string{"0", "off", "false"} {
		setRequiredEnv(t)
		t.Setenv("DSH_EE_LOGIN_IP_LIMIT", v)
		c, err := Load()
		if err != nil {
			t.Fatalf("%q: %v", v, err)
		}
		if c.LoginIPLimit != -1 {
			t.Fatalf("%q 应关闭限流（-1），得到 %d", v, c.LoginIPLimit)
		}
	}
}

func TestLoginIPLimitInvalid(t *testing.T) {
	cases := []struct{ key, value string }{
		{"DSH_EE_LOGIN_IP_LIMIT", "-1"},
		{"DSH_EE_LOGIN_IP_LIMIT", "abc"},
		{"DSH_EE_LOGIN_IP_WINDOW", "30s"},
		{"DSH_EE_LOGIN_IP_WINDOW", "25h"},
		{"DSH_EE_LOGIN_IP_WINDOW", "nope"},
		{"DSH_EE_METRICS", "maybe"},
	}
	for _, c := range cases {
		setRequiredEnv(t)
		// 先设回合法值，确保是当前这一项触发的错误
		t.Setenv("DSH_EE_LOGIN_IP_LIMIT", "50")
		t.Setenv("DSH_EE_LOGIN_IP_WINDOW", "15m")
		t.Setenv("DSH_EE_METRICS", "false")
		t.Setenv(c.key, c.value)
		if _, err := Load(); err == nil {
			t.Errorf("%s=%q 应报错", c.key, c.value)
		}
	}
}

func TestLoadEmbedConfig(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("DSH_EE_EMBED_ORIGINS", "https://harness.acme.internal,http://localhost:5173")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load 报错：%v", err)
	}
	if strings.Join(cfg.EmbedOrigins, ",") != "https://harness.acme.internal,http://localhost:5173" {
		t.Fatalf("EmbedOrigins = %v", cfg.EmbedOrigins)
	}
	if cfg.EmbedCrossSite {
		t.Fatal("跨站嵌入默认应关闭")
	}
	if cfg.EmbedSessionTTL != 2*time.Hour {
		t.Fatalf("EmbedSessionTTL 默认 2h，实际 %v", cfg.EmbedSessionTTL)
	}
	if cfg.EmbedMintPerMinute != 30 || cfg.EmbedExchangePerMinute != 60 {
		t.Fatalf("嵌入限流默认值不对：mint=%d exchange=%d", cfg.EmbedMintPerMinute, cfg.EmbedExchangePerMinute)
	}
}

// 多实例共享限流的额度可配；非法值启动失败（fail closed）。
func TestLoadEmbedRateLimits(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("DSH_EE_EMBED_MINT_PER_MINUTE", "7")
	t.Setenv("DSH_EE_EMBED_EXCHANGE_PER_MINUTE", "120")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load 报错：%v", err)
	}
	if cfg.EmbedMintPerMinute != 7 || cfg.EmbedExchangePerMinute != 120 {
		t.Fatalf("限流配置没读到：mint=%d exchange=%d", cfg.EmbedMintPerMinute, cfg.EmbedExchangePerMinute)
	}
	for _, bad := range []string{"0", "-1", "abc", "100001"} {
		t.Setenv("DSH_EE_EMBED_MINT_PER_MINUTE", bad)
		if _, err := Load(); err == nil || !strings.Contains(err.Error(), "DSH_EE_EMBED_MINT_PER_MINUTE") {
			t.Fatalf("DSH_EE_EMBED_MINT_PER_MINUTE=%q 应该让 Load 失败，实际 err=%v", bad, err)
		}
	}
}

func TestLoadEmbedOriginsFailClosed(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("DSH_EE_EMBED_ORIGINS", "https://harness.acme.internal/path")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "DSH_EE_EMBED_ORIGINS") {
		t.Fatalf("非法来源应该让 Load 失败，实际 err=%v", err)
	}
}

func TestLoadEmbedCrossSite(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("DSH_EE_EMBED_CROSS_SITE", "1")
	t.Setenv("DSH_EE_EMBED_ORIGINS", "https://harness.acme.internal:8443")
	t.Setenv("DSH_EE_EMBED_SESSION_TTL", "90m")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load 报错：%v", err)
	}
	if !cfg.EmbedCrossSite || cfg.EmbedSessionTTL != 90*time.Minute {
		t.Fatalf("cross-site=%v ttl=%v", cfg.EmbedCrossSite, cfg.EmbedSessionTTL)
	}

	// 开了跨站却没有来源：启动失败（不允许对任意来源放开）
	t.Setenv("DSH_EE_EMBED_ORIGINS", "")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "DSH_EE_EMBED_ORIGINS") {
		t.Fatalf("跨站开启但没配来源应失败，实际 err=%v", err)
	}

	// TTL 越界
	t.Setenv("DSH_EE_EMBED_ORIGINS", "https://harness.acme.internal")
	t.Setenv("DSH_EE_EMBED_SESSION_TTL", "9h")
	if _, err := Load(); err == nil {
		t.Fatal("超过 8h 的会话 TTL 应该失败")
	}
}

// loadWithRateLimit 只设置 Load 必需的变量，再按用例给 DSH_EE_RATE_LIMIT。
func loadWithRateLimit(t *testing.T, rateLimit string) (*Config, error) {
	t.Helper()
	t.Setenv("DSH_EE_DSN", "root:@tcp(127.0.0.1:4000)/dsh_ee")
	t.Setenv("DSH_EE_SECRET_KEY", base64.StdEncoding.EncodeToString(make([]byte, 32)))
	t.Setenv("DSH_EE_RATE_LIMIT", rateLimit)
	return Load()
}

func TestRateLimitConfig(t *testing.T) {
	cases := []struct {
		name    string
		env     string
		want    int
		wantErr bool
	}{
		{name: "默认每分钟 60 次", env: "", want: 60},
		{name: "环境变量调大", env: "120", want: 120},
		{name: "0 关闭", env: "0", want: 0},
		{name: "off 关闭", env: "off", want: 0},
		{name: "false 关闭", env: "false", want: 0},
		{name: "带空格", env: "  45  ", want: 45},
		{name: "负数非法", env: "-1", wantErr: true},
		{name: "非数字非法", env: "abc", wantErr: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg, err := loadWithRateLimit(t, c.env)
			if c.wantErr {
				if err == nil || !strings.Contains(err.Error(), "DSH_EE_RATE_LIMIT") {
					t.Fatalf("期望 DSH_EE_RATE_LIMIT 报错，得到 %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if cfg.RateLimitPerMinute != c.want {
				t.Fatalf("RateLimitPerMinute = %d，期望 %d", cfg.RateLimitPerMinute, c.want)
			}
		})
	}
}

// API 客户端（client_credentials）的每 IP 限流开关：默认 120，0/off 关闭，非法值启动失败。
func TestClientTokenRateLimitConfig(t *testing.T) {
	cases := []struct {
		name    string
		env     string
		want    int
		wantErr bool
	}{
		{name: "默认每分钟 120 次", env: "", want: 120},
		{name: "环境变量调整", env: "30", want: 30},
		{name: "0 关闭", env: "0", want: 0},
		{name: "off 关闭", env: "off", want: 0},
		{name: "false 关闭", env: "false", want: 0},
		{name: "负数非法", env: "-1", wantErr: true},
		{name: "非数字非法", env: "abc", wantErr: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			setRequiredEnv(t)
			t.Setenv("DSH_EE_CLIENT_TOKEN_PER_MINUTE", c.env)
			cfg, err := Load()
			if c.wantErr {
				if err == nil || !strings.Contains(err.Error(), "DSH_EE_CLIENT_TOKEN_PER_MINUTE") {
					t.Fatalf("期望 DSH_EE_CLIENT_TOKEN_PER_MINUTE 报错，得到 %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if cfg.ClientTokenPerMinute != c.want {
				t.Fatalf("ClientTokenPerMinute = %d，期望 %d", cfg.ClientTokenPerMinute, c.want)
			}
		})
	}
}

func TestBackgroundLeaseDefaults(t *testing.T) {
	setRequiredEnv(t)
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !c.BackgroundLease || c.BackgroundLeaseTTL != 90*time.Second {
		t.Fatalf("后台任务租约默认应开启、TTL 90s，得到 %v / %v", c.BackgroundLease, c.BackgroundLeaseTTL)
	}
}

func TestBackgroundLeaseEnv(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("DSH_EE_BACKGROUND_LEASE", "false")
	t.Setenv("DSH_EE_BACKGROUND_LEASE_TTL", "15s")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.BackgroundLease || c.BackgroundLeaseTTL != 15*time.Second {
		t.Fatalf("配置没生效：%v / %v", c.BackgroundLease, c.BackgroundLeaseTTL)
	}

	setRequiredEnv(t)
	t.Setenv("DSH_EE_BACKGROUND_LEASE_TTL", "500ms")
	if _, err := Load(); err == nil {
		t.Fatal("过短的 TTL 应让启动失败")
	}
	setRequiredEnv(t)
	t.Setenv("DSH_EE_BACKGROUND_LEASE", "maybe")
	if _, err := Load(); err == nil {
		t.Fatal("非法的开关值应让启动失败")
	}
}

func TestLoadRefreshConfig(t *testing.T) {
	setRequiredEnv(t)
	// 默认关闭，参数是保守的默认值
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load 报错：%v", err)
	}
	if cfg.RefreshTokens || cfg.RefreshAccessTTL != time.Hour || cfg.RefreshTokenTTL != 30*24*time.Hour ||
		cfg.RefreshFamilyTTL != 180*24*time.Hour || cfg.RefreshGrace != 0 {
		t.Fatalf("刷新令牌默认配置不对：%+v", cfg)
	}
	t.Setenv("DSH_EE_REFRESH_TOKENS", "1")
	t.Setenv("DSH_EE_REFRESH_ACCESS_TTL", "15m")
	t.Setenv("DSH_EE_REFRESH_TOKEN_TTL", "720h")
	t.Setenv("DSH_EE_REFRESH_FAMILY_TTL", "4320h")
	t.Setenv("DSH_EE_REFRESH_GRACE", "20s")
	if cfg, err = Load(); err != nil {
		t.Fatalf("Load 报错：%v", err)
	}
	if !cfg.RefreshTokens || cfg.RefreshAccessTTL != 15*time.Minute || cfg.RefreshTokenTTL != 720*time.Hour ||
		cfg.RefreshFamilyTTL != 4320*time.Hour || cfg.RefreshGrace != 20*time.Second {
		t.Fatalf("刷新令牌配置没生效：%+v", cfg)
	}
	// 族上限必须覆盖单枚 refresh 的有效期
	t.Setenv("DSH_EE_REFRESH_FAMILY_TTL", "500h")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "REFRESH_FAMILY_TTL") {
		t.Fatalf("族上限小于 refresh 有效期应失败，实际 err=%v", err)
	}
	// access TTL 越界
	t.Setenv("DSH_EE_REFRESH_FAMILY_TTL", "4320h")
	t.Setenv("DSH_EE_REFRESH_ACCESS_TTL", "25h")
	if _, err := Load(); err == nil {
		t.Fatal("超过 24h 的 access TTL 应该失败")
	}
	// 宽限期越界
	t.Setenv("DSH_EE_REFRESH_ACCESS_TTL", "15m")
	t.Setenv("DSH_EE_REFRESH_GRACE", "6m")
	if _, err := Load(); err == nil {
		t.Fatal("超过 5m 的宽限期应该失败")
	}
}

// DeepSeek Chatbot 的两个选项：默认都不注入，取值写错应让 Load 失败（fail closed）。
func TestChatbotOptionsDefaults(t *testing.T) {
	setRequiredEnv(t)
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.SafetyCheck != "" {
		t.Fatalf("DSH_EE_SAFETY_CHECK 默认应关闭，得到 %q", c.SafetyCheck)
	}
	if c.ChatTitle {
		t.Fatal("DSH_EE_CHAT_TITLE 默认应关闭")
	}
}

func TestChatbotOptionsEnv(t *testing.T) {
	cases := []struct {
		name      string
		safety    string
		title     string
		wantSafe  string
		wantTitle bool
	}{
		{name: "显式 off", safety: "off", title: "off"},
		{name: "开启 high / on", safety: "high", title: "on", wantSafe: "high", wantTitle: true},
		{name: "大小写与空白", safety: " HIGH ", title: " On ", wantSafe: "high", wantTitle: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setRequiredEnv(t)
			t.Setenv("DSH_EE_SAFETY_CHECK", tc.safety)
			t.Setenv("DSH_EE_CHAT_TITLE", tc.title)
			c, err := Load()
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if c.SafetyCheck != tc.wantSafe {
				t.Fatalf("SafetyCheck = %q，期望 %q", c.SafetyCheck, tc.wantSafe)
			}
			if c.ChatTitle != tc.wantTitle {
				t.Fatalf("ChatTitle = %v，期望 %v", c.ChatTitle, tc.wantTitle)
			}
		})
	}
}

func TestChatbotOptionsInvalid(t *testing.T) {
	cases := []struct{ key, value string }{
		{"DSH_EE_SAFETY_CHECK", "low"},
		{"DSH_EE_SAFETY_CHECK", "true"},
		{"DSH_EE_SAFETY_CHECK", "on"},
		{"DSH_EE_CHAT_TITLE", "true"},
		{"DSH_EE_CHAT_TITLE", "high"},
		{"DSH_EE_CHAT_TITLE", "maybe"},
	}
	for _, tc := range cases {
		t.Run(tc.key+"="+tc.value, func(t *testing.T) {
			setRequiredEnv(t)
			t.Setenv(tc.key, tc.value)
			_, err := Load()
			if err == nil || !strings.Contains(err.Error(), tc.key) {
				t.Fatalf("%s=%q 应报错，得到 %v", tc.key, tc.value, err)
			}
		})
	}
}
