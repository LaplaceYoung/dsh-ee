/** 插件在私有市场里的来源：从上游（dshfind / npm）预载，或企业员工内部发布。 */
export type PluginOrigin = "upstream" | "internal";

/** 审核状态（与 npm 源一致）：pending 待审 / published 已上架 / rejected 已驳回 / unpublished 已下架。 */
export type PluginStatus = "pending" | "published" | "rejected" | "unpublished";

export type Visibility = "company" | "department" | "private";

/**
 * 能力形态：企业资产封装成插件后的几种样子。
 * skill 流程与规范 / connector 信息系统连接器 / knowledge 数据与上下文知识包 / plugin 通用插件（含上游社区插件）。
 */
export type PluginKind = "skill" | "connector" | "knowledge" | "plugin";

export interface Plugin {
  name: string;
  title: string;
  description: string;
  author: string;
  icon: string | null;
  homepage: string | null;
  repository: string | null;
  category: string;
  kind?: PluginKind;
  featured: boolean;
  stars: number;
  /** dshfind 上游评分 0-100；内部插件没有。 */
  score: number | null;
  version: string;
  publishedAt: number;
  origin: PluginOrigin;
  /** 企业内安装次数（来自 registry tarball 下载日志）。 */
  installs: number;
  /** 依赖闭包里的包数。 */
  depCount: number;
  /** 插件加依赖闭包的总大小。 */
  sizeKb: number;
  /** 依赖闭包能否在无外网时装好（无 postinstall 下载二进制等）。 */
  offlineReady: boolean;
  hasInstallScript: boolean;
  status: PluginStatus;
  /** 企业托管包的负责人（用户名）。 */
  owner?: string;
  /** 待审核的版本数（管理端）。 */
  pending?: number;
  visibility?: Visibility;
  department?: string;
  /** 员工评分（1-5），仅在开启员工评分时展示。 */
  rating?: number;
  ratingCount?: number;
  submittedAt?: number;
}

export type DshTarget = "mac-arm64" | "mac-x64" | "win-x64";
export type DshReleaseState = "available" | "downloading" | "mirrored" | "failed" | "removed";

export interface DshRelease {
  id: number;
  target: DshTarget;
  kind: "zip" | "dmg" | "exe";
  version: string;
  released_at: string;
  size: number;
  sha512: string;
  recommended: boolean;
  verified: boolean;
  download_url: string;
  file?: string;
  state?: DshReleaseState;
  downloaded?: number;
  error?: string;
}

export interface DshClient {
  client_name: string;
  last_client: string;
  last_host: string;
  last_used_at: string;
}

export interface Settings {
  orgName: string;
  baseUrl: string;
  /** 组织默认语言（规范短码 zh/en/es/pt/hi）；员工没设置语言偏好时用它。 */
  defaultLanguage: "zh" | "en" | "es" | "pt" | "hi";
  scope: string;
  admissionMode: "allowlist" | "open";
  scoring: {
    enabled: boolean;
    showUpstreamScore: boolean;
    allowEmployeeRating: boolean;
    minPreloadScore: number;
  };
  upstream: {
    mode: "online" | "offline";
    dshfindFeed: boolean;
    registry: "npmjs" | "npmmirror" | "custom";
    customRegistry: string;
    autoSync: "auto" | "review" | "off";
  };
  distribution: {
    enabled: boolean;
    autoSync: "off" | "notify" | "mirror";
    keepVersions: number;
    offerDmg: boolean;
  };
  brand: {
    enabled: boolean;
    name: string;
    logoRev: string;
    /** DSH 首屏的大标题；空表示保留 DSH 自带的标题。 */
    title: string;
    logoShape: BrandLogoShape;
    theme: BrandTheme;
  };
  auth: "local" | "ldap" | "oidc";
  /** DSH 接入插件（<主作用域>/dsh-connect）：由 dsh-ee 生成并发布到自己的 npm 源。 */
  connect: {
    enabled: boolean;
    /** 把 dsh-ee 的模型网关加为 DSH 的模型提供方并设为默认。 */
    model: boolean;
    defaultModel: string;
    /** 预置企业插件索引。 */
    market: boolean;
    /** 自动安装设为「自动安装」的插件与微应用依赖的插件。 */
    installRequired: boolean;
    pollSeconds: number;
    deviceTokenDays: number;
    allowManualTokens: boolean;
    /** Agent 总开关：关闭后 DSH 里不再有 ee_* 工具与系统技能。 */
    agentEnabled: boolean;
  };
  registry: {
    /** 企业作用域之外的包：按需回源上游并缓存，或只提供已预载 / 已审核的包。 */
    proxyMode: "cache" | "preloaded";
    /** 主作用域（scope）之外的其它内部作用域，同样永不回源。 */
    extraScopes: string[];
    /** 安装（读取）也要求令牌；关闭后只有发布需要令牌。 */
    requireToken: boolean;
  };
  /** 跨站嵌入 v1 的管理端设置；非管理员从 /api/settings 拿不到这一段。 */
  embed: {
    /** true/false = 数据库显式覆盖；null/undefined = 跟随部署环境变量 */
    crossSite?: boolean | null;
    /** 追加在 DSH_EE_EMBED_ORIGINS 之后的宿主 origin；部署环境变量只读 */
    allowedOrigins: string[];
    /** 允许嵌入的应用 slug；空 = 所有开放网页版的应用 */
    apps: string[];
    /** ee_emb_ 会话有效期（分钟）；0 = 跟随部署配置 */
    sessionMinutes: number;
  };
}

/** 基于 DSH 运行的企业微应用：界面由 dsh-ee 托管，推理与工具调用交给员工本机 DSH。 */
export interface MicroApp {
  id: string;
  title: string;
  description: string;
  department: string;
  owner: string;
  /** 依赖的企业能力（插件包名），打开微应用时由 DSH 按需安装。 */
  uses: string[];
  /** 近 14 天运行次数。 */
  runs: number;
  status: "online" | "beta";
  updatedAt: string;
}

export type AuditAction = "run" | "install" | "publish" | "approve" | "yank";

export interface AuditEvent {
  at: string;
  user: string;
  department: string;
  action: AuditAction;
  target: string;
  /** 执行端：DSH 版本与操作系统。 */
  client: string;
}

export type BrandLogoShape = "circle" | "rounded" | "plain";

/** 一个颜色在浅色、深色模式下的取值；空表示该模式用 DSH 默认。 */
export interface BrandColor {
  light: string;
  dark: string;
}

/** DSH 配色：colors 是常用位置，服务端换算成设计变量；tokens 直接写 --dsw-* 变量。 */
export interface BrandTheme {
  preset: string;
  colors: Partial<Record<BrandColorKey, BrandColor>>;
  tokens: Record<string, BrandColor>;
}

export type BrandColorKey = "accent" | "accentText" | "accentSoft" | "sidebar" | "sidebarActive" | "background" | "input";
