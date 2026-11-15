/**
 * 前端页面目录：每个页面组件一个稳定的 page_key。
 *
 * 页面叫什么、挂在哪个路由、放在哪个分组、谁能看，都在「菜单与页面」里在线配置（存后端 menus 表）；
 * 这里只记录组件本身的信息，供菜单编辑器选择，以及菜单里还没配置这个页面时作为站内链接的兜底路由。
 * 新增页面：在这里加一项，在 pages/registry.tsx 里登记组件，然后去「菜单与页面」把它挂出来。
 */
export interface PageMeta {
  title: string;
  /** 菜单里没有这个页面时，站内链接回退到这个路由。 */
  defaultPath: string;
  description: string;
}

const META = {
  workbench: { title: "我的工作台", defaultPath: "/workbench", description: "个人指标、装过的能力、部门在用、我分享的" },
  apps: { title: "企业应用", defaultPath: "/apps", description: "我能用的企业应用，在 DSH 里打开" },
  market: { title: "能力中心", defaultPath: "/hub", description: "旧的能力中心入口，跳到企业应用或原料" },
  knowledge: { title: "知识问答", defaultPath: "/knowledge", description: "制度、手册与资料；在 DSH 的「知识问答」里提问" },
  "app-new": { title: "新建应用", defaultPath: "/build/new", description: "新建一个应用，先自己用" },
  build: { title: "我的应用", defaultPath: "/build", description: "我做的应用：自己用、分享、审核状态" },
  templates: { title: "从样板复制", defaultPath: "/build/templates", description: "从样板和公司已发布的应用复制一个改" },
  materials: { title: "原料", defaultPath: "/build/materials", description: "做应用能用的知识库、连接、技能与提示词" },
  "dsh-download": { title: "下载 DSH", defaultPath: "/dsh", description: "DSH 核心与桌面版下载" },
  setup: { title: "接入指南", defaultPath: "/setup", description: "一条命令接入企业源" },
  publish: { title: "分享插件", defaultPath: "/publish", description: "上传插件或申请引入社区插件" },
  "dev-docs": { title: "开发文档", defaultPath: "/docs", description: "dsh-ee 视角的开发入口：插件、扩展、权限与发布" },
  "dev-examples": { title: "开发样例", defaultPath: "/examples", description: "精选 DSH 插件样例：固定版本、安装命令、来源与许可" },
  "my-keys": { title: "我的令牌", defaultPath: "/tokens", description: "我的额度，自助创建与吊销令牌" },
  "admin-users": { title: "用户", defaultPath: "/admin/users", description: "账号、禁用、重置密码、分配角色" },
  "admin-roles": { title: "角色与权限", defaultPath: "/admin/roles", description: "角色、权限点与授权" },
  "admin-menus": { title: "菜单与页面", defaultPath: "/admin/menus", description: "页面名称、路由、图标、分组与访问权限" },
  "admin-fields": { title: "字段与访问规则", defaultPath: "/admin/fields", description: "扩展字段、按字段的访问规则、管理约束与配置变更请求" },
  "admin-providers": { title: "模型接入", defaultPath: "/admin/providers", description: "上游模型服务、企业 key 与对外模型" },
  "admin-quotas": { title: "额度分配", defaultPath: "/admin/quotas", description: "用户的月度 token 额度与可用模型" },
  "admin-keys": { title: "令牌管理", defaultPath: "/admin/tokens", description: "全部员工的令牌：查看、代发、吊销" },
  "admin-dashboard": { title: "总览", defaultPath: "/admin", description: "全链路总览：资产、治理、员工与终端" },
  "admin-stats": { title: "运营数据", defaultPath: "/admin/stats", description: "安装、模型用量、部门与存储" },
  "admin-audit": { title: "审计日志", defaultPath: "/admin/audit", description: "登录、权限、设置、发布与审核等操作记录" },
  "admin-plugins": { title: "能力管理", defaultPath: "/admin/plugins", description: "能力上下架" },
  "admin-connections": { title: "企业连接", defaultPath: "/admin/connections", description: "接入企业 MCP 服务与 HTTP 接口，托管凭据，配置工具与授权" },
  "admin-review": { title: "待审核", defaultPath: "/admin/review", description: "审核队列" },
  "admin-registry": { title: "npm 源", defaultPath: "/admin/registry", description: "路由规则、上游、访问令牌" },
  "admin-upstream": { title: "上游预载", defaultPath: "/admin/upstream", description: "在线同步与离线导入" },
  "admin-distribution": { title: "DSH 分发", defaultPath: "/admin/distribution", description: "DSH 本体分发" },
  "admin-brand": { title: "品牌与外观", defaultPath: "/admin/brand", description: "DSH 里的企业名称、logo、首屏标题与配色" },
  "admin-auth": { title: "登录方式", defaultPath: "/admin/auth", description: "LDAP / AD 与企业统一登录（OIDC）" },
  "admin-expert-center": { title: "专家与连接器策略", defaultPath: "/admin/expert-center", description: "专家 · 技能 · 连接器的分类、精选、禁用策略与权限查询" },
  "admin-signing-keys": { title: "扩展签名", defaultPath: "/admin/signing-keys", description: "扩展签名的受信任公钥：新增、换下、撤销" },
  "admin-directory": { title: "企业插件目录", defaultPath: "/admin/directory", description: "登记过的企业目录：校验 / 登记目录 JSON，按条目安装、升级扩展与插件" },
  "admin-settings": { title: "系统设置", defaultPath: "/admin/settings", description: "全局设置" },
} satisfies Record<string, PageMeta>;

export type PageKey = keyof typeof META;

/** 按任意字符串取页面信息（菜单里的 page_key 来自数据库，可能不在目录里）。 */
export const PAGE_META: Record<string, PageMeta> = META;
