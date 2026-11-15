// Package rbac 定义代码内置的权限点目录与默认菜单。
//
// 权限点随版本演进：启动时同步进数据库，新出现的权限点自动授予 DefaultRoles 里的角色；
// 页面名称、路由、分组等都存在数据库的 menus 表，管理员可随时修改，这里只提供初始值。
package rbac

// 内置角色。
const (
	RoleSuper  = "admin"
	RoleMember = "member"
)

// 权限点。接口处理函数通过这些常量检查权限。
const (
	PortalView      = "portal.view"
	LLMUse          = "llm.use"
	KeysSelf        = "keys.self"
	RegistryUse     = "registry.use"
	RegistryPublish = "registry.publish"

	UsersView   = "users.view"
	UsersManage = "users.manage"
	RolesManage = "roles.manage"
	MenusManage = "menus.manage"

	ProvidersManage  = "providers.manage"
	QuotasManage     = "quotas.manage"
	KeysManage       = "keys.manage"
	TokensDeptManage = "tokens.dept_manage"

	DashboardView      = "dashboard.view"
	AuditView          = "audit.view"
	PluginsManage      = "plugins.manage"
	ConnectionsManage  = "connections.manage"
	KnowledgeManage    = "knowledge.manage"
	RegistryManage     = "registry.manage"
	UpstreamManage     = "upstream.manage"
	DistributionManage = "distribution.manage"
	SettingsManage     = "settings.manage"

	// 扩展字段和访问规则（docs/access-control.md）
	FieldsManage = "fields.manage"
	// 写扩展、跑开发命令（docs/agent-dev.md §7）
	ExtensionsDevelop = "extensions.develop"
)

type Permission struct {
	Code        string
	Name        string
	Group       string
	Description string
	// 首次出现时自动授予的角色（按角色 code）。
	DefaultRoles []string
}

// Catalog 是代码内置的全部权限点，顺序即展示顺序。
var Catalog = []Permission{
	{PortalView, "使用工作台", "工作台", "我的工作台、微应用、能力市场、下载与接入指南等员工页面", []string{RoleMember}},
	{LLMUse, "调用大模型", "工作台", "用自己的 API key 通过 dsh-ee 调用模型", []string{RoleMember}},
	{KeysSelf, "管理自己的 API key", "工作台", "查看额度，自助创建和吊销自己的 key", []string{RoleMember}},
	{RegistryUse, "安装能力", "工作台", "从企业 npm 源安装插件与依赖，创建自己的 npm 令牌", []string{RoleMember}},
	{RegistryPublish, "发布能力", "工作台", "发布插件、技能、提示词与微应用（发布后进入审核），申请引入社区插件", []string{RoleMember}},

	{UsersView, "查看用户", "用户与权限", "查看用户列表（分配额度、分发 key、分配角色时也需要）", nil},
	{UsersManage, "管理用户", "用户与权限", "新建、编辑、禁用用户，重置密码", nil},
	{RolesManage, "管理角色与权限", "用户与权限", "新建和编辑角色，给用户分配角色", nil},
	{MenusManage, "管理菜单与页面", "用户与权限", "修改页面名称、路由、图标、分组、排序与访问权限", nil},

	{ProvidersManage, "管理模型接入", "大模型", "配置上游模型服务、企业 key 与对外模型", nil},
	{QuotasManage, "分配额度", "大模型", "设置用户的月度 token 额度、可用模型与有效期", nil},
	{KeysManage, "管理全部 API key", "大模型", "查看、代发和吊销所有员工的 key", nil},
	{TokensDeptManage, "本部门令牌管理", "大模型", "查看、下发和吊销本部门机器人账号的令牌", nil},

	{DashboardView, "查看管理总览", "能力与分发", "管理总览与运营数据", nil},
	{AuditView, "查看审计日志", "能力与分发", "登录、账号、权限、设置、发布与审核等操作记录", nil},
	{PluginsManage, "管理能力与审核", "能力与分发", "插件、技能、提示词与微应用的审核、上下架、可用范围与下发方式", nil},
	{ConnectionsManage, "管理企业连接", "能力与分发", "接入企业 MCP 服务与 API，托管凭据，配置工具与授权", nil},
	{KnowledgeManage, "管理知识库", "能力与分发", "新建和删除知识库，设置谁能查、谁来维护，管理全部文档，查看全部检索记录，配置「知识问答」", nil},
	{RegistryManage, "管理 npm 源", "能力与分发", "路由规则、上游与访问令牌", nil},
	{UpstreamManage, "管理上游预载", "能力与分发", "在线同步与离线导入", nil},
	{DistributionManage, "管理 DSH 分发", "能力与分发", "DSH 本体与桌面版的分发", nil},
	{SettingsManage, "管理系统设置", "能力与分发", "评分、收录策略、上游、LDAP 登录等全局设置", nil},
	{FieldsManage, "管理字段和访问规则", "用户与权限", "给文档、知识库、用户加扩展字段，设置按字段控制访问的规则，改用于访问控制的字段值", nil},
	{ExtensionsDevelop, "开发扩展", "能力与分发", "拿到写扩展用的技能和命令说明（dsh-ee-extension-dev）", nil},
}

// Menu 是默认菜单树的一项。Kind: group / page / link。Hidden 的页面有路由、不在侧边栏显示。
type Menu struct {
	Kind       string
	Title      string
	Path       string
	PageKey    string
	Icon       string
	Permission string
	Hidden     bool
	Children   []Menu
}

// DefaultMenus 是首次启动与「恢复默认」时写入的菜单。PageKey 对应前端页面注册表。
// 按「用 / 造 / 管」组织：员工先看到应用；做应用的入口和原料在「造」；管理页面只有管理员看得到。
// 已有部署的菜单由迁移 0013 升级（只改还是默认值的项）。
var DefaultMenus = []Menu{
	{Kind: "group", Title: "用", Children: []Menu{
		{Kind: "page", Title: "企业应用", Path: "/apps", PageKey: "apps", Icon: "AppWindow", Permission: PortalView},
		{Kind: "page", Title: "知识问答", Path: "/knowledge", PageKey: "knowledge", Icon: "BookOpen", Permission: PortalView},
		{Kind: "page", Title: "我的工作台", Path: "/workbench", PageKey: "workbench", Icon: "House", Permission: PortalView},
	}},
	{Kind: "group", Title: "造", Children: []Menu{
		{Kind: "page", Title: "新建应用", Path: "/build/new", PageKey: "app-new", Icon: "SquarePlus", Permission: RegistryPublish},
		{Kind: "page", Title: "我的应用", Path: "/build", PageKey: "build", Icon: "LayoutGrid", Permission: PortalView},
		{Kind: "page", Title: "从样板复制", Path: "/build/templates", PageKey: "templates", Icon: "Copy", Permission: RegistryPublish},
		{Kind: "page", Title: "原料", Path: "/build/materials", PageKey: "materials", Icon: "Layers", Permission: PortalView},
		// 插件只在「原料 → 高级选项」里出现，这一页不进侧边栏
		{Kind: "page", Title: "分享插件", Path: "/publish", PageKey: "publish", Icon: "Upload", Permission: PortalView, Hidden: true},
	}},
	{Kind: "group", Title: "接入", Children: []Menu{
		{Kind: "page", Title: "下载 DSH", Path: "/dsh", PageKey: "dsh-download", Icon: "Download", Permission: PortalView},
		{Kind: "page", Title: "接入指南", Path: "/setup", PageKey: "setup", Icon: "Terminal", Permission: PortalView},
		{Kind: "page", Title: "我的令牌", Path: "/tokens", PageKey: "my-keys", Icon: "KeyRound", Permission: KeysSelf},
	}},
	// 开发文档与开发样例：页面是固定路由（/docs、/examples），菜单只负责侧栏入口与权限
	{Kind: "group", Title: "开发", Children: []Menu{
		{Kind: "page", Title: "开发文档", Path: "/docs", PageKey: "dev-docs", Icon: "BookMarked", Permission: PortalView},
		{Kind: "page", Title: "开发样例", Path: "/examples", PageKey: "dev-examples", Icon: "FlaskConical", Permission: PortalView},
	}},
	// 以下是管理后台：网页侧栏在 /admin 页面里只显示这些分组（见 web/src/components/layout.tsx）
	{Kind: "group", Title: "运营", Children: []Menu{
		{Kind: "page", Title: "总览", Path: "/admin", PageKey: "admin-dashboard", Icon: "Gauge", Permission: DashboardView},
		{Kind: "page", Title: "运营数据", Path: "/admin/stats", PageKey: "admin-stats", Icon: "LayoutDashboard", Permission: DashboardView},
		{Kind: "page", Title: "审计日志", Path: "/admin/audit", PageKey: "admin-audit", Icon: "ScrollText", Permission: AuditView},
	}},
	{Kind: "group", Title: "账号与权限", Children: []Menu{
		{Kind: "page", Title: "用户", Path: "/admin/users", PageKey: "admin-users", Icon: "Users", Permission: UsersView},
		{Kind: "page", Title: "角色与权限", Path: "/admin/roles", PageKey: "admin-roles", Icon: "ShieldCheck", Permission: RolesManage},
		{Kind: "page", Title: "菜单与页面", Path: "/admin/menus", PageKey: "admin-menus", Icon: "PanelsTopLeft", Permission: MenusManage},
		{Kind: "page", Title: "字段与访问规则", Path: "/admin/fields", PageKey: "admin-fields", Icon: "Tags", Permission: FieldsManage},
	}},
	{Kind: "group", Title: "模型与额度", Children: []Menu{
		{Kind: "page", Title: "模型接入", Path: "/admin/providers", PageKey: "admin-providers", Icon: "Bot", Permission: ProvidersManage},
		{Kind: "page", Title: "额度分配", Path: "/admin/quotas", PageKey: "admin-quotas", Icon: "Wallet", Permission: QuotasManage},
		{Kind: "page", Title: "令牌管理", Path: "/admin/tokens", PageKey: "admin-keys", Icon: "KeyRound", Permission: KeysManage},
	}},
	{Kind: "group", Title: "能力与连接", Children: []Menu{
		{Kind: "page", Title: "能力管理", Path: "/admin/plugins", PageKey: "admin-plugins", Icon: "Boxes", Permission: PluginsManage},
		{Kind: "page", Title: "待审核", Path: "/admin/review", PageKey: "admin-review", Icon: "ClipboardCheck", Permission: PluginsManage},
		// 专家 · 技能 · 连接器的分类、精选、禁用策略与权限查询（docs/expert-center.md §6）
		{Kind: "page", Title: "专家与连接器策略", Path: "/admin/expert-center", PageKey: "admin-expert-center", Icon: "GraduationCap", Permission: PluginsManage},
		{Kind: "page", Title: "企业连接", Path: "/admin/connections", PageKey: "admin-connections", Icon: "Cable", Permission: ConnectionsManage},
		// 扩展签名的信任根（docs/plugin-signing.md §7）：装扩展的管理员顺手管受信任公钥；
		// 只有 settings.manage 的管理员也能直接打开这个路由（web/src/main.tsx 的兜底）。
		{Kind: "page", Title: "扩展签名", Path: "/admin/signing-keys", PageKey: "admin-signing-keys", Icon: "ShieldCheck", Permission: ConnectionsManage},
		// 企业插件目录（docs/plugin-signing.md §8）：登记来源 + 对单条显式安装 / 升级；
		// 同时服务管连接（扩展）和管插件（npm 包）两种管理员，默认挂在「能力与连接」。
		{Kind: "page", Title: "企业插件目录", Path: "/admin/directory", PageKey: "admin-directory", Icon: "FolderOpen", Permission: ConnectionsManage},
	}},
	{Kind: "group", Title: "分发与源", Children: []Menu{
		{Kind: "page", Title: "DSH 分发", Path: "/admin/distribution", PageKey: "admin-distribution", Icon: "Download", Permission: DistributionManage},
		{Kind: "page", Title: "npm 源", Path: "/admin/registry", PageKey: "admin-registry", Icon: "Server", Permission: RegistryManage},
		{Kind: "page", Title: "上游预载", Path: "/admin/upstream", PageKey: "admin-upstream", Icon: "CloudDownload", Permission: UpstreamManage},
	}},
	{Kind: "group", Title: "系统", Children: []Menu{
		{Kind: "page", Title: "品牌与外观", Path: "/admin/brand", PageKey: "admin-brand", Icon: "Palette", Permission: SettingsManage},
		{Kind: "page", Title: "登录方式", Path: "/admin/auth", PageKey: "admin-auth", Icon: "LogIn", Permission: SettingsManage},
		{Kind: "page", Title: "系统设置", Path: "/admin/settings", PageKey: "admin-settings", Icon: "Settings", Permission: SettingsManage},
	}},
}

// Access 是一个用户的有效权限。
type Access struct {
	Super     bool
	RoleIDs   []int64
	RoleCodes []string
	Perms     map[string]bool
}

// Has 判断是否拥有任一权限点；超级管理员拥有全部。空参数表示只要登录即可。
func (a *Access) Has(codes ...string) bool {
	if a == nil {
		return false
	}
	if a.Super || len(codes) == 0 {
		return true
	}
	for _, c := range codes {
		if a.Perms[c] {
			return true
		}
	}
	return false
}
