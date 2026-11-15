package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"maps"
)

// AppSettings 是系统设置，结构与前端 web/src/lib/types.ts 的 Settings 一一对应。
// 服务端用到其中的组织名、企业作用域、收录与回源策略；其余字段原样保存给前端。
type AppSettings struct {
	OrgName string `json:"orgName"`
	// 员工访问 dsh-ee 的地址；为空时前端用当前页面地址。
	BaseURL string `json:"baseUrl"`
	// 企业主作用域，如 @acme：只能在这里发布，永不回源上游。
	Scope string `json:"scope"`
	// allowlist：插件要审核后才上架 / open：发布即上架，靠下架处理问题
	AdmissionMode string `json:"admissionMode"`
	Scoring       struct {
		Enabled             bool `json:"enabled"`
		ShowUpstreamScore   bool `json:"showUpstreamScore"`
		AllowEmployeeRating bool `json:"allowEmployeeRating"`
		MinPreloadScore     int  `json:"minPreloadScore"`
	} `json:"scoring"`
	Upstream struct {
		// online：可以访问上游 / offline：完全离线，只用已预载与离线导入的包
		Mode        string `json:"mode"`
		DshfindFeed bool   `json:"dshfindFeed"`
		// npmjs / npmmirror / custom
		Registry       string `json:"registry"`
		CustomRegistry string `json:"customRegistry"`
		// 已预载插件出了新版本：auto 自动同步并上架 / review 同步后待审核 / off 不同步
		AutoSync string `json:"autoSync"`
	} `json:"upstream"`
	Distribution struct {
		Enabled               bool   `json:"enabled"`
		AutoSync              string `json:"autoSync"`
		KeepVersions          int    `json:"keepVersions"`
		OfferDmg              bool   `json:"offerDmg"`
		AllowAlpha            bool   `json:"allowAlpha"`
		AllowCommunityDesktop bool   `json:"allowCommunityDesktop"`
	} `json:"distribution"`
	// 企业目录的部署策略（docs/plugin-signing.md §8）。本期只存 / 只读展示，不执行自动升级：
	// 目录条目自己还要声明 auto_upgrade=true 才「允许」被自动升级；真正升级仍要管理员显式触发。
	Directory struct {
		// 总开关，默认 false。本期打开也只会让更新检查多看一眼条目的 auto_upgrade 声明，
		// 不会有任何自动安装 / 升级动作。
		AutoUpgrade bool `json:"autoUpgrade"`
	} `json:"directory"`
	Brand struct {
		Enabled bool   `json:"enabled"`
		Name    string `json:"name"`
		LogoRev string `json:"logoRev"`
		// DSH 首屏的大标题，例如「星河科技企业工作台」；空表示保留 DSH 自带的标题
		Title string `json:"title"`
		// logo 外形：circle 圆形裁切 / rounded 圆角方形 / plain 原样
		LogoShape string     `json:"logoShape"`
		Theme     BrandTheme `json:"theme"`
	} `json:"brand"`
	Auth string `json:"auth"`
	// DSH 接入插件（<主作用域>/dsh-connect）：由 dsh-ee 生成并发布到自己的 npm 源
	Connect struct {
		Enabled bool `json:"enabled"`
		// 把 dsh-ee 的模型网关加为 DSH 的模型提供方并设为默认
		Model        bool   `json:"model"`
		DefaultModel string `json:"defaultModel"`
		// 预置企业插件索引（DSH 插件页的「发现」）
		Market bool `json:"market"`
		// 自动安装设为「自动安装」的插件与微应用依赖的插件
		InstallRequired bool `json:"installRequired"`
		// 多久检查一次能力目录的变化
		PollSeconds       int  `json:"pollSeconds"`
		DeviceTokenDays   int  `json:"deviceTokenDays"`
		AllowManualTokens bool `json:"allowManualTokens"`
		// 总开关：关闭后 /mcp 不再提供 ee_* 工具，系统技能也不下发
		AgentEnabled bool `json:"agentEnabled"`
	} `json:"connect"`
	// 知识库与预置的「知识问答」微应用（kb-qa）
	Knowledge struct {
		// 员工至少能查一个知识库时，DSH 里出现「知识问答」
		QAEnabled  bool     `json:"qaEnabled"`
		QATitle    string   `json:"qaTitle"`
		QAIcon     string   `json:"qaIcon"`
		QAStarters []string `json:"qaStarters"`
		// 管理员补充的回答要求，追加在系统规则后面
		QAExtra string `json:"qaExtra"`
	} `json:"knowledge"`
	Registry struct {
		// cache：企业作用域之外的包按需回源并缓存 / preloaded：只提供已预载的包
		ProxyMode   string   `json:"proxyMode"`
		ExtraScopes []string `json:"extraScopes"`
		// 安装也要令牌；关闭后匿名可读，发布仍要令牌
		RequireToken bool `json:"requireToken"`
	} `json:"registry"`
	// 跨站嵌入 v1 的管理端设置（E1）：与环境变量合并成有效配置。
	// 环境变量是部署基线、只读；这里显式设置的值覆盖它（allowed_origins 取并集）。
	Embed struct {
		// 是否允许宿主后端 mint 跨站嵌入 code；nil = 跟随 DSH_EE_EMBED_CROSS_SITE
		CrossSite *bool `json:"crossSite,omitempty"`
		// 额外允许的宿主 origin（追加在 DSH_EE_EMBED_ORIGINS 之后，只读环境变量不变）
		AllowedOrigins []string `json:"allowedOrigins"`
		// 允许嵌入的应用 slug；空 = 所有开放了网页版的应用
		Apps []string `json:"apps"`
		// ee_emb_ 会话有效期（分钟）；0 = 跟随部署配置/默认 2h
		SessionMinutes int `json:"sessionMinutes"`
	} `json:"embed"`
}

// BrandTheme 是 DSH 界面的配色。Colors 是几个常用位置（主色、侧栏等），
// 服务端把它们换算成 DSH 的 --dsw-* 设计变量；Tokens 直接写变量，覆盖 Colors 的结果。
type BrandTheme struct {
	// 预设名，只给网页标记「当前用的是哪套」，不参与换算
	Preset string                `json:"preset"`
	Colors map[string]BrandColor `json:"colors"`
	Tokens map[string]BrandColor `json:"tokens"`
}

// BrandColor 是一个颜色在浅色、深色模式下的取值；空表示该模式保留 DSH 默认。
type BrandColor struct {
	Light string `json:"light"`
	Dark  string `json:"dark"`
}

// DefaultSettings 是首次启动时的设置。默认「按需回源 + 插件要审核」：依赖能直接装，
// 社区插件和内部发布的插件都要管理员放行。
func DefaultSettings() *AppSettings {
	s := &AppSettings{
		Scope:         "@company",
		AdmissionMode: "allowlist",
		Auth:          "local",
	}
	s.Scoring.Enabled = true
	s.Scoring.ShowUpstreamScore = true
	s.Scoring.AllowEmployeeRating = true
	s.Scoring.MinPreloadScore = 60
	s.Upstream.Mode = "online"
	s.Upstream.DshfindFeed = true
	s.Upstream.Registry = "npmmirror"
	s.Upstream.AutoSync = "review"
	s.Distribution.Enabled = true
	s.Distribution.AutoSync = "notify"
	s.Distribution.KeepVersions = 3
	s.Distribution.AllowCommunityDesktop = true
	// 企业目录：自动升级默认关闭，本期也不会执行（只存 / 只读展示）
	s.Directory.AutoUpgrade = false
	s.Connect.Enabled = true
	s.Connect.Model = true
	s.Connect.Market = true
	s.Connect.InstallRequired = true
	s.Connect.PollSeconds = 60
	s.Connect.DeviceTokenDays = 90
	s.Connect.AllowManualTokens = true
	s.Connect.AgentEnabled = true
	s.Knowledge.QAEnabled = true
	s.Knowledge.QATitle = "知识问答"
	s.Knowledge.QAIcon = "📚"
	s.Knowledge.QAStarters = []string{"出差住宿标准是多少？", "新员工入职要准备哪些材料？"}
	s.Brand.LogoShape = "circle"
	s.Brand.Theme.Colors = map[string]BrandColor{}
	s.Brand.Theme.Tokens = map[string]BrandColor{}
	s.Registry.ProxyMode = "cache"
	s.Registry.ExtraScopes = []string{}
	s.Registry.RequireToken = true
	s.Embed.AllowedOrigins = []string{}
	s.Embed.Apps = []string{}
	return s
}

// CompanyScopes 是全部企业作用域（主作用域 + 其它内部作用域）。
func (s *AppSettings) CompanyScopes() []string {
	out := []string{s.Scope}
	for _, sc := range s.Registry.ExtraScopes {
		if sc != s.Scope {
			out = append(out, sc)
		}
	}
	return out
}

// Clone 深拷贝，调用方可以放心修改。
func (s *AppSettings) Clone() *AppSettings {
	c := *s
	c.Registry.ExtraScopes = append([]string{}, s.Registry.ExtraScopes...)
	c.Embed.AllowedOrigins = append([]string{}, s.Embed.AllowedOrigins...)
	c.Embed.Apps = append([]string{}, s.Embed.Apps...)
	if s.Embed.CrossSite != nil {
		b := *s.Embed.CrossSite
		c.Embed.CrossSite = &b
	}
	c.Knowledge.QAStarters = append([]string{}, s.Knowledge.QAStarters...)
	c.Brand.Theme.Colors = maps.Clone(s.Brand.Theme.Colors)
	c.Brand.Theme.Tokens = maps.Clone(s.Brand.Theme.Tokens)
	return &c
}

const appSettingsKey = "app"

// GetSettings 读取设置；库里缺的字段用默认值补齐（版本升级新增字段时不会变成零值）。
func (s *Store) GetSettings(ctx context.Context) (*AppSettings, error) {
	out := DefaultSettings()
	var raw string
	err := s.db.QueryRowContext(ctx, "SELECT value FROM settings WHERE name = ?", appSettingsKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(raw), out); err != nil {
		return nil, err
	}
	if out.Registry.ExtraScopes == nil {
		out.Registry.ExtraScopes = []string{}
	}
	return out, nil
}

func (s *Store) SaveSettings(ctx context.Context, v *AppSettings, userID int64) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO settings (name, value, updated_by, updated_at) VALUES (?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE value = VALUES(value), updated_by = VALUES(updated_by), updated_at = VALUES(updated_at)`,
		appSettingsKey, string(b), userID, now())
	return err
}

// GetSettingJSON 读取一行独立保存的设置（如 LDAP 配置，员工不可读）；不存在时返回 false，dst 不变。
func (s *Store) GetSettingJSON(ctx context.Context, name string, dst any) (bool, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, "SELECT value FROM settings WHERE name = ?", name).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, json.Unmarshal([]byte(raw), dst)
}

// PutSettingJSON 保存一行独立的设置；userID 为 0 表示系统写入（如后台同步）。
func (s *Store) PutSettingJSON(ctx context.Context, name string, v any, userID int64) error {
	if name == appSettingsKey {
		return errors.New("系统设置请用 SaveSettings")
	}
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	var by any
	if userID > 0 {
		by = userID
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO settings (name, value, updated_by, updated_at) VALUES (?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE value = VALUES(value), updated_by = VALUES(updated_by), updated_at = VALUES(updated_at)`,
		name, string(b), by, now())
	return err
}

// UpdateSettingJSON 在一个事务里读出一行设置、交给 fn 改、再写回去：几个副本同时改同一行时不会互相覆盖。
// 这一行还不存在时 fn 收到 nil；fn 返回 nil 表示不改。
func (s *Store) UpdateSettingJSON(ctx context.Context, name string, userID int64, fn func(old []byte) ([]byte, error)) error {
	if name == appSettingsKey {
		return errors.New("系统设置请用 SaveSettings")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// 先占住这一行（没有就插一行空的），后面的 FOR UPDATE 才有东西可锁
	if _, err := tx.ExecContext(ctx, "INSERT IGNORE INTO settings (name, value, updated_at) VALUES (?, '', ?)", name, now()); err != nil {
		return err
	}
	var raw string
	if err := tx.QueryRowContext(ctx, "SELECT value FROM settings WHERE name = ? FOR UPDATE", name).Scan(&raw); err != nil {
		return err
	}
	var old []byte
	if raw != "" {
		old = []byte(raw)
	}
	next, err := fn(old)
	if err != nil {
		return err
	}
	if next == nil {
		if raw == "" {
			// 没改，把刚才占位的空行拿掉
			if _, err := tx.ExecContext(ctx, "DELETE FROM settings WHERE name = ? AND value = ''", name); err != nil {
				return err
			}
		}
		return tx.Commit()
	}
	var by any
	if userID > 0 {
		by = userID
	}
	if _, err := tx.ExecContext(ctx, "UPDATE settings SET value = ?, updated_by = ?, updated_at = ? WHERE name = ?", string(next), by, now(), name); err != nil {
		return err
	}
	return tx.Commit()
}
