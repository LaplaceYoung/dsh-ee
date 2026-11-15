// Package config 从环境变量读取服务配置。
package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/go-sql-driver/mysql"
	"github.com/hikariming/dsh-ee/server/internal/directory"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/embed"
)

type Config struct {
	// 监听地址，默认只监听本机。
	Addr string
	// MySQL 协议 DSN，OceanBase（MySQL 模式）与 TiDB 通用。
	DSN string
	// 加密上游 API key 的 32 字节主密钥。
	SecretKey []byte
	// 会话 cookie 是否加 Secure。不显式设置时跟随 DSH_EE_PUBLIC_URL：
	// 对外地址是 https 就默认开启；显式 DSH_EE_COOKIE_SECURE 覆盖这个默认。
	CookieSecure bool
	SessionTTL   time.Duration
	// 同一客户端 IP 在窗口内允许的登录失败次数；达到后返回 429。默认 50；小于 0 表示关闭。
	LoginIPLimit int
	// 登录失败按 IP 限流的窗口，默认 15m。
	LoginIPWindow time.Duration
	// 是否开放 GET /metrics（Prometheus 文本格式，只允许本机 / 内网抓取）。默认关闭。
	Metrics bool
	// 公开协议面（/v1/* 与 /mcp）的每令牌限流，单位「次/分钟」；默认 60，0 或 off 关闭。
	RateLimitPerMinute int
	// API 客户端（client_credentials）的每 IP 每分钟签发次数（DSH_EE_CLIENT_TOKEN_PER_MINUTE）；
	// 默认 120，0 或 off 关闭。只防用错误 secret 撞库 / 刷库，正常集成远低于这个量。
	ClientTokenPerMinute int
	// 旧版 npm 令牌（ee_npm_）与 API key（sk-ee-）的 scope 收敛（docs/token-lifecycle.md）。
	// true 时它们只保留原始用途的 scope：npm 令牌只剩 registry，key 只剩模型网关；
	// 默认 false 不改变旧行为，避免现有客户端突然失效。
	RestrictLegacyTokenScopes bool
	// 刷新令牌（OAuth 风格，docs/token-lifecycle.md §3）。默认关闭：关闭时
	// POST /api/auth/refresh 返回 404，设备/手工签发的响应也不带 refresh_token。
	RefreshTokens bool
	// refresh 换出来的 access token 有效期（DSH_EE_REFRESH_ACCESS_TTL）。
	RefreshAccessTTL time.Duration
	// 单枚 refresh token 的空闲有效期（DSH_EE_REFRESH_TOKEN_TTL），轮换时重新计算。
	RefreshTokenTTL time.Duration
	// 令牌族绝对有效期（DSH_EE_REFRESH_FAMILY_TTL）：无论怎么轮换都不能超过它。
	RefreshFamilyTTL time.Duration
	// 轮换宽限（DSH_EE_REFRESH_GRACE）：>0 时旧 refresh 在轮换后这段时间内重试会原样
	// 返回上次响应（客户端没收到响应时的幂等重试）；0 表示严格，再次出现即按复用吊销整族。
	RefreshGrace time.Duration
	// 首次启动、用户表为空时创建的管理员。
	BootstrapAdmin         string
	BootstrapAdminPassword string
	// 本地数据目录：npm 包 tarball、上游元数据缓存。
	DataDir string
	// 对外访问地址（如 https://dsh.acme.internal），用于生成 tarball 下载地址；
	// 为空时按请求的 Host / X-Forwarded-* 推断。
	PublicURL string
	// 浏览器内第三方 Harness 直连 /v1 与 /mcp 的跨域白名单（DSH_EE_CORS_ORIGINS，逗号分隔）。
	// 只接受完整 origin：https://host[:port]，http 只放行 localhost；为空时关闭 CORS。
	CORSOrigins []string
	// 可信反向代理的地址或网段（DSH_EE_TRUSTED_PROXIES，逗号分隔的 IP / CIDR）。
	// 只有来自这些对端的 X-Forwarded-For/Host/Proto 才会被采信；不设置时只信任本机回环地址。
	// 反向代理在别的机器、容器网络或云负载均衡后面时必须显式配置，否则客户端 IP 会记成
	// 代理地址，HTTPS 代理下生成的绝对链接也可能退回 http。
	TrustedProxies []netip.Prefix
	// 启动时等待数据库就绪的最长时间（docker compose 里数据库常比服务晚起来）。
	DBWait time.Duration
	// 管理模式「1 小时 / 8 小时」里「小时」的长度，默认 1h；只给自动化测试缩短用，生产不要设
	AdminModeUnit time.Duration
	// 扩展签名（docs/plugins.md §9.3）：受信任的签名公钥（PEM 文件，可以有多把）。
	// 安装计划里用它们核对扩展的签名；为空时签了名的扩展也只会显示「不受信任」。
	ExtensionTrustedKeys string
	// 强制验证签名（DSH_EE_EXTENSION_REQUIRE_SIGNATURE=1）：没签名或签名验不过的扩展不让安装。
	// 默认只在安装计划里提示，不阻断。
	ExtensionRequireSignature bool
	// 企业目录（docs/plugin-signing.md §2.2）：tarball 来源的主机白名单（DSH_EE_DIRECTORY_TARBALL_HOSTS，
	// 逗号分隔，支持 *.example.com）。为空时只接受公网 https 地址；名单里的主机允许是内网地址。
	DirectoryTarballHosts []string
	// 数据中台的库（DSH_EE_DATA_DSN）。默认和主库同一个服务器、库名加 _data；设成 off 关闭。
	DataDSN string
	// 数据中台的只读账号（DSH_EE_DATA_READ_DSN）；不填时用 DataDSN 的账号自动建一个只有 SELECT 权限的账号。
	DataReadDSN string
	// DeepSeek Chatbot 的网关注入选项（docs/dev/chatbot.md）：
	// SafetyCheck 是 DSH_EE_SAFETY_CHECK，默认空（off）不注入；high 时向 vendor=deepseek 的
	// 上游请求注入 safety_check:{level:"high"}。客户端已经带了 safety_check 时保留客户端的值。
	SafetyCheck string
	// ChatTitle 是 DSH_EE_CHAT_TITLE，默认 off 不注入；on 时向 vendor=deepseek 的上游请求
	// 注入 title:{enabled:true}。客户端已经带了 title 时保留客户端的值。
	// 这两个选项只影响发往 DeepSeek 上游的那次请求，不改模型/额度/记账，也不改写响应。
	ChatTitle bool
	// 公开演示站（DSH_EE_DEMO_MODE）：登录页显示初始管理员账号，关掉改密码等会把别人锁在外面的操作，
	// 对外请求只许连本机。DemoReset 是给访客看的重置周期说明，如「每 6 小时」。
	// DemoResetEvery 是实际的重置间隔（和 crontab 一致），用来在登录页算下次重置的时间点；0 表示不显示。
	DemoMode       bool
	DemoReset      string
	DemoResetEvery time.Duration
	// DemoVideoDir：放介绍视频 intro-zh.mp4、intro-en.mp4 的目录，演示站首页播放；空则不显示。
	// 视频不打进二进制，免得每个部署都背着它。
	DemoVideoDir string
	// DemoStatsFile：演示站的访问统计追加到这个文件（每行一个 JSON）；空则不记。
	DemoStatsFile string
	// 允许用 iframe 嵌入 dsh-ee 网页的宿主来源（DSH_EE_EMBED_ORIGINS，逗号分隔）。
	// 只接受完整 origin：https://host[:port]，http 只放行回环地址；为空时只允许同站嵌入。
	EmbedOrigins []string
	// 跨站 iframe 嵌入 v1（DSH_EE_EMBED_CROSS_SITE）：默认关闭。
	// 开启后 /api/embed/sessions 与 /api/auth/embed/exchange 才可用，且必须同时配置 EmbedOrigins。
	EmbedCrossSite bool
	// ee_emb_ 嵌入会话有效期（DSH_EE_EMBED_SESSION_TTL），默认 2h；设计建议 1h–8h。
	EmbedSessionTTL time.Duration
	// 内容安全策略（DSH_EE_CSP）：off（默认，只加现有的 frame-ancestors）/ report（只报告）/
	// enforce（阻断）。见 server/internal/api/security_headers.go。
	CSP string
	// CSP 违规报告地址（DSH_EE_CSP_REPORT_URI，可选）。必须是 http(s) 绝对地址；只在
	// DSH_EE_CSP 不是 off 时生效，用于 report 模式收集违规或 enforce 模式观测被拦的请求。
	CSPReportURI string
	// mint 的共享固定窗口限流：每 IP 每分钟次数（DSH_EE_EMBED_MINT_PER_MINUTE），默认 30。
	// 落数据库，多实例部署时是合计额度。
	EmbedMintPerMinute int
	// exchange 的共享固定窗口限流：每 IP 每分钟次数（DSH_EE_EMBED_EXCHANGE_PER_MINUTE），默认 60。
	EmbedExchangePerMinute int
	// 多实例后台任务租约（DSH_EE_BACKGROUND_LEASE）：周期任务每轮先抢数据库里的一条租约，
	// 只有抢到的实例执行，避免多副本重复跑。默认开启；单实例也安全。
	BackgroundLease bool
	// 后台任务租约有效期（DSH_EE_BACKGROUND_LEASE_TTL），默认 90s；持有者按 1/3 TTL 续约。
	BackgroundLeaseTTL time.Duration
}

func Load() (*Config, error) {
	c := &Config{
		Addr:                   getenv("DSH_EE_ADDR", "127.0.0.1:8080"),
		DSN:                    os.Getenv("DSH_EE_DSN"),
		BootstrapAdmin:         getenv("DSH_EE_BOOTSTRAP_ADMIN", "admin"),
		BootstrapAdminPassword: os.Getenv("DSH_EE_BOOTSTRAP_ADMIN_PASSWORD"),
		SessionTTL:             7 * 24 * time.Hour,
		LoginIPLimit:           50,
		LoginIPWindow:          15 * time.Minute,
		RateLimitPerMinute:     60,
		ClientTokenPerMinute:   120,
		DataDir:                getenv("DSH_EE_DATA_DIR", "./data"),
		PublicURL:              strings.TrimRight(os.Getenv("DSH_EE_PUBLIC_URL"), "/"),
		DBWait:                 2 * time.Minute,
		BackgroundLease:        true,
		BackgroundLeaseTTL:     90 * time.Second,
		// 刷新令牌的默认参数：access 1 小时、refresh 空闲 30 天、族绝对 180 天、宽限严格。
		RefreshAccessTTL: time.Hour,
		RefreshTokenTTL:  30 * 24 * time.Hour,
		RefreshFamilyTTL: 180 * 24 * time.Hour,
		RefreshGrace:     0,
	}
	if c.DSN == "" {
		return nil, errors.New("缺少 DSH_EE_DSN，例如 root:@tcp(127.0.0.1:4000)/dsh_ee")
	}
	trusted, err := ParseTrustedProxies(os.Getenv("DSH_EE_TRUSTED_PROXIES"))
	if err != nil {
		return nil, err
	}
	c.TrustedProxies = trusted
	c.DataReadDSN = os.Getenv("DSH_EE_DATA_READ_DSN")
	switch v := os.Getenv("DSH_EE_DATA_DSN"); v {
	case "off":
	case "":
		d, err := mysql.ParseDSN(c.DSN)
		if err != nil {
			return nil, fmt.Errorf("DSH_EE_DSN: %w", err)
		}
		if d.DBName != "" {
			d.DBName += "_data"
			c.DataDSN = d.FormatDSN()
		}
	default:
		c.DataDSN = v
	}
	raw := os.Getenv("DSH_EE_SECRET_KEY")
	if raw == "" {
		return nil, errors.New("缺少 DSH_EE_SECRET_KEY（32 字节 base64），可用 openssl rand -base64 32 生成")
	}
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil || len(key) != 32 {
		return nil, errors.New("DSH_EE_SECRET_KEY 必须是 32 字节的 base64")
	}
	c.SecretKey = key
	c.ExtensionTrustedKeys = strings.TrimSpace(os.Getenv("DSH_EE_EXTENSION_TRUSTED_KEYS"))
	if v := os.Getenv("DSH_EE_EXTENSION_REQUIRE_SIGNATURE"); v != "" {
		b, err := strconv.ParseBool(strings.TrimSpace(v))
		if err != nil {
			return nil, errors.New("DSH_EE_EXTENSION_REQUIRE_SIGNATURE 要写成 true / false（或 1 / 0）")
		}
		c.ExtensionRequireSignature = b
	}
	cookieSecureSet := false
	if v := strings.TrimSpace(os.Getenv("DSH_EE_DIRECTORY_TARBALL_HOSTS")); v != "" {
		hosts, err := directory.ParseTarballHosts(v)
		if err != nil {
			return nil, fmt.Errorf("DSH_EE_DIRECTORY_TARBALL_HOSTS: %w", err)
		}
		c.DirectoryTarballHosts = hosts
	}
	if v := os.Getenv("DSH_EE_COOKIE_SECURE"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return nil, fmt.Errorf("DSH_EE_COOKIE_SECURE: %w", err)
		}
		c.CookieSecure = b
		cookieSecureSet = true
	}
	if v := os.Getenv("DSH_EE_RESTRICT_LEGACY_TOKEN_SCOPES"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return nil, fmt.Errorf("DSH_EE_RESTRICT_LEGACY_TOKEN_SCOPES: %w", err)
		}
		c.RestrictLegacyTokenScopes = b
	}
	// 刷新令牌（docs/token-lifecycle.md §3）；开关默认关，参数只在开启后生效。
	if v := os.Getenv("DSH_EE_REFRESH_TOKENS"); v != "" {
		b, err := strconv.ParseBool(strings.TrimSpace(v))
		if err != nil {
			return nil, fmt.Errorf("DSH_EE_REFRESH_TOKENS: %w", err)
		}
		c.RefreshTokens = b
	}
	if v := os.Getenv("DSH_EE_REFRESH_ACCESS_TTL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d < time.Minute || d > 24*time.Hour {
			return nil, errors.New("DSH_EE_REFRESH_ACCESS_TTL 要在 1m 到 24h 之间，例如 30m")
		}
		c.RefreshAccessTTL = d
	}
	if v := os.Getenv("DSH_EE_REFRESH_TOKEN_TTL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d < time.Hour || d > 365*24*time.Hour {
			return nil, errors.New("DSH_EE_REFRESH_TOKEN_TTL 要在 1h 到 8760h（365 天）之间，例如 720h")
		}
		c.RefreshTokenTTL = d
	}
	if v := os.Getenv("DSH_EE_REFRESH_FAMILY_TTL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d < time.Hour || d > 10*365*24*time.Hour {
			return nil, errors.New("DSH_EE_REFRESH_FAMILY_TTL 要在 1h 到 87600h（10 年）之间，例如 4320h")
		}
		c.RefreshFamilyTTL = d
	}
	if c.RefreshFamilyTTL < c.RefreshTokenTTL {
		return nil, errors.New("DSH_EE_REFRESH_FAMILY_TTL 不能小于 DSH_EE_REFRESH_TOKEN_TTL：族上限必须能覆盖单枚 refresh 的有效期")
	}
	if v := os.Getenv("DSH_EE_REFRESH_GRACE"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d < 0 || d > 5*time.Minute {
			return nil, errors.New("DSH_EE_REFRESH_GRACE 要在 0 到 5m 之间；0 表示严格轮换（重试按复用吊销整族）")
		}
		c.RefreshGrace = d
	}
	if v := os.Getenv("DSH_EE_DEMO_MODE"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return nil, fmt.Errorf("DSH_EE_DEMO_MODE: %w", err)
		}
		c.DemoMode = b
	}
	c.DemoReset = strings.TrimSpace(os.Getenv("DSH_EE_DEMO_RESET"))
	c.DemoVideoDir = strings.TrimSpace(os.Getenv("DSH_EE_DEMO_VIDEO_DIR"))
	c.DemoStatsFile = strings.TrimSpace(os.Getenv("DSH_EE_DEMO_STATS_FILE"))
	if v := os.Getenv("DSH_EE_DEMO_RESET_EVERY"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("DSH_EE_DEMO_RESET_EVERY: %w", err)
		}
		if d < time.Minute || d > 24*time.Hour {
			return nil, errors.New("DSH_EE_DEMO_RESET_EVERY 要在 1m 到 24h 之间")
		}
		c.DemoResetEvery = d
	}
	if c.DemoMode && c.BootstrapAdminPassword == "" {
		return nil, errors.New("DSH_EE_DEMO_MODE 需要同时设置 DSH_EE_BOOTSTRAP_ADMIN_PASSWORD：登录页要把演示账号显示出来")
	}
	if v := os.Getenv("DSH_EE_RATE_LIMIT"); v != "" {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "off", "false":
			c.RateLimitPerMinute = 0
		default:
			n, err := strconv.Atoi(strings.TrimSpace(v))
			if err != nil || n < 0 {
				return nil, fmt.Errorf("DSH_EE_RATE_LIMIT 要是正整数（次/分钟），或 0/off 关闭")
			}
			c.RateLimitPerMinute = n
		}
	}
	if v := os.Getenv("DSH_EE_CLIENT_TOKEN_PER_MINUTE"); v != "" {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "off", "false":
			c.ClientTokenPerMinute = 0
		default:
			n, err := strconv.Atoi(strings.TrimSpace(v))
			if err != nil || n < 0 {
				return nil, fmt.Errorf("DSH_EE_CLIENT_TOKEN_PER_MINUTE 要是正整数（次/分钟），或 0/off 关闭")
			}
			c.ClientTokenPerMinute = n
		}
	}
	if v := os.Getenv("DSH_EE_SESSION_TTL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("DSH_EE_SESSION_TTL: %w", err)
		}
		c.SessionTTL = d
	}
	if v := os.Getenv("DSH_EE_ADMIN_MODE_UNIT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			return nil, fmt.Errorf("DSH_EE_ADMIN_MODE_UNIT 必须是正的时间长度，例如 15s")
		}
		c.AdminModeUnit = d
	}
	if v := os.Getenv("DSH_EE_DB_WAIT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("DSH_EE_DB_WAIT: %w", err)
		}
		c.DBWait = d
	}
	if v := strings.TrimSpace(os.Getenv("DSH_EE_CORS_ORIGINS")); v != "" {
		origins, err := parseCORSOrigins(v)
		if err != nil {
			return nil, fmt.Errorf("DSH_EE_CORS_ORIGINS: %w", err)
		}
		c.CORSOrigins = origins
	}
	if c.PublicURL != "" {
		u, err := url.Parse(c.PublicURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return nil, errors.New("DSH_EE_PUBLIC_URL 必须是 http(s)://host[:port][/path] 形式")
		}
	}
	if v := strings.TrimSpace(os.Getenv("DSH_EE_LOGIN_IP_LIMIT")); v != "" {
		switch strings.ToLower(v) {
		case "off", "false":
			c.LoginIPLimit = -1
		default:
			n, err := strconv.Atoi(v)
			if err != nil || n < 0 {
				return nil, errors.New("DSH_EE_LOGIN_IP_LIMIT 必须是正整数，或 0/off 关闭")
			}
			if n == 0 {
				c.LoginIPLimit = -1
			} else {
				c.LoginIPLimit = n
			}
		}
	}
	if v := os.Getenv("DSH_EE_LOGIN_IP_WINDOW"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d < time.Minute || d > 24*time.Hour {
			return nil, errors.New("DSH_EE_LOGIN_IP_WINDOW 必须是 1m 到 24h 之间的时间长度")
		}
		c.LoginIPWindow = d
	}
	if v := os.Getenv("DSH_EE_METRICS"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return nil, fmt.Errorf("DSH_EE_METRICS: %w", err)
		}
		c.Metrics = b
	}
	// DeepSeek Chatbot 选项：只接受文档里的取值，写错让启动失败，避免以为开了其实没开。
	switch v := strings.ToLower(strings.TrimSpace(os.Getenv("DSH_EE_SAFETY_CHECK"))); v {
	case "", "off":
	case "high":
		c.SafetyCheck = "high"
	default:
		return nil, errors.New("DSH_EE_SAFETY_CHECK 只能是 off 或 high")
	}
	switch v := strings.ToLower(strings.TrimSpace(os.Getenv("DSH_EE_CHAT_TITLE"))); v {
	case "", "off":
	case "on":
		c.ChatTitle = true
	default:
		return nil, errors.New("DSH_EE_CHAT_TITLE 只能是 off 或 on")
	}
	if !cookieSecureSet && strings.HasPrefix(c.PublicURL, "https://") {
		// 生产部署最常见的配置是只填 PUBLIC_URL。对外是 HTTPS 时会话 cookie 默认加 Secure，
		// 避免漏设 DSH_EE_COOKIE_SECURE 时 cookie 还能被同主机的 HTTP 请求带出去。
		c.CookieSecure = true
	}
	if v := strings.TrimSpace(os.Getenv("DSH_EE_EMBED_ORIGINS")); v != "" {
		origins, err := embed.ParseOrigins(v)
		if err != nil {
			return nil, fmt.Errorf("DSH_EE_EMBED_ORIGINS: %w", err)
		}
		c.EmbedOrigins = origins
	}
	if v := os.Getenv("DSH_EE_EMBED_CROSS_SITE"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return nil, fmt.Errorf("DSH_EE_EMBED_CROSS_SITE: %w", err)
		}
		c.EmbedCrossSite = b
	}
	c.EmbedSessionTTL = 2 * time.Hour
	if v := os.Getenv("DSH_EE_EMBED_SESSION_TTL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("DSH_EE_EMBED_SESSION_TTL: %w", err)
		}
		if d < time.Minute || d > 8*time.Hour {
			return nil, errors.New("DSH_EE_EMBED_SESSION_TTL 要在 1m 到 8h 之间")
		}
		c.EmbedSessionTTL = d
	}
	if c.EmbedCrossSite && len(c.EmbedOrigins) == 0 {
		return nil, errors.New("DSH_EE_EMBED_CROSS_SITE 需要同时设置 DSH_EE_EMBED_ORIGINS：跨站嵌入只放行配置的宿主来源")
	}
	c.EmbedMintPerMinute, err = positiveIntEnv("DSH_EE_EMBED_MINT_PER_MINUTE", 30)
	if err != nil {
		return nil, err
	}
	c.EmbedExchangePerMinute, err = positiveIntEnv("DSH_EE_EMBED_EXCHANGE_PER_MINUTE", 60)
	if err != nil {
		return nil, err
	}
	if v := strings.TrimSpace(os.Getenv("DSH_EE_BACKGROUND_LEASE")); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return nil, errors.New("DSH_EE_BACKGROUND_LEASE 要写成 true / false（或 1 / 0）")
		}
		c.BackgroundLease = b
	}
	if v := strings.TrimSpace(os.Getenv("DSH_EE_BACKGROUND_LEASE_TTL")); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("DSH_EE_BACKGROUND_LEASE_TTL: %w", err)
		}
		if d < time.Second || d > time.Hour {
			return nil, errors.New("DSH_EE_BACKGROUND_LEASE_TTL 要在 1s 到 1h 之间")
		}
		c.BackgroundLeaseTTL = d
	}
	// 内容安全策略：默认 off，保持现有「只加 frame-ancestors」行为不变。
	c.CSP, err = parseCSPMode(os.Getenv("DSH_EE_CSP"))
	if err != nil {
		return nil, err
	}
	if v := strings.TrimSpace(os.Getenv("DSH_EE_CSP_REPORT_URI")); v != "" {
		if err := validateCSPReportURI(v); err != nil {
			return nil, err
		}
		c.CSPReportURI = v
	}
	return c, nil
}

// positiveIntEnv 读一个正整数环境变量；不设时用默认值，非法值让启动失败（fail closed）。
func positiveIntEnv(key string, def int) (int, error) {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 || n > 100000 {
		return 0, fmt.Errorf("%s: 要是 1–100000 之间的整数", key)
	}
	return n, nil
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
