/**
 * 同站嵌入（v0）的前端参数与 postMessage 协议。
 *
 * 宿主用 iframe 打开微应用网页版：
 *   /a/<标识>?embed=1&host=<宿主 origin>[&fit=auto][&theme=light|dark|system][&lang=zh-CN|en|es|pt|hi]
 *
 * 约定：
 * - 消息只发给 URL 里的 host origin，绝不使用 "*"；
 * - 只接受来自 host origin、且 source 是 window.parent 的消息；
 * - 不传 host 或 host 非法时页面照常显示，但不发任何消息（宿主自己加了 sandbox 也能用）；
 * - 同站用现有 Lax cookie；跨站（v1）由宿主后端 mint 一次性 code，iframe 用 postMessage
 *   `dsh-ee:connect` 换取只存内存的 ee_emb_ Bearer（见 use-embed.ts 的握手与 docs/embedding.md）。
 */

export const EMBED_VERSION = 1;
/** 协议 v2（增量）：hello/supports、navigate、navigate-request、open-external、logout、deeplink。 */
export const EMBED_PROTOCOL_V2 = 2;

/** fit=auto 上报内容高度的上限：宿主按消息设高度时，避免异常页面把宿主布局撑爆。 */
export const EMBED_MAX_HEIGHT = 4000;

/**
 * iframe 自身握手超时（ready 发出后等 connect/session）：超时即进入失败态并发 `dsh-ee:error`。
 * 宿主侧建议 8 秒没有 ready/error 就显示手动的「在浏览器打开」兜底（不自动顶层打开），
 * 两者关系见 docs/embedding.md §5；`dsh-ee:hello` 的 timeouts 会带上这两个数字。
 */
export const EMBED_HANDSHAKE_TIMEOUT_MS = 10_000;
export const EMBED_HOST_FALLBACK_MS = 8_000;

/** 移动端判定阈值：窄屏（<640px，Tailwind sm 断点以下）或移动 UA 都算移动端。 */
export const EMBED_MOBILE_MAX_WIDTH = 640;

export type EmbedTheme = "light" | "dark";
/** URL 上的主题偏好：`light`/`dark` 固定，`system` 跟随 prefers-color-scheme。 */
export type EmbedThemePreference = EmbedTheme | "system";
export type EmbedFit = "fill" | "auto";

/**
 * URL 上的界面语言：五语目标语言码。当前只注册了 zh-CN / en，其余语言在语言包上线前
 * 由前端按 fallback 规则回退英文；宿主可以提前传，等语言包上线即自动生效。
 */
export const EMBED_LANGS = ["zh-CN", "en", "es", "pt", "hi"] as const;
export type EmbedLang = (typeof EMBED_LANGS)[number];

export type EmbedErrorCode = "auth_required" | "forbidden" | "load_failed";

/**
 * 失败的阶段，宿主/支持按它判断问题出在哪一层：
 * - `load`：页面/接口加载（应用不存在、无权限、网络错误等）；
 * - `session`：未登录、会话过期、握手超时（对齐服务端 v1 错误体的 `stage: "session"`）；
 * - `exchange`：跨站一次性 code 交换失败（对齐服务端 v1 错误体的 `stage: "exchange"`）。
 */
export type EmbedErrorStage = "load" | "session" | "exchange";

export interface EmbedOptions {
  embed: boolean;
  /** 宿主 origin（校验后的 `scheme://host[:port]`）；缺省或非法时为 null。 */
  host: string | null;
  fit: EmbedFit;
  /** URL 上的主题偏好；`system` 跟随系统且不写 localStorage。 */
  theme: EmbedThemePreference | null;
  /** URL 上的界面语言；只作用于本次渲染，不写 localStorage（与 theme 语义一致）。 */
  lang: EmbedLang | null;
}

export interface EmbedViewport {
  /** 窄屏（<640px）或移动 UA：iframe 内显示「在浏览器打开」常驻兜底，但不自动顶层跳转。 */
  mobile: boolean;
  width: number;
  height: number;
}

export interface EmbedError {
  code: EmbedErrorCode;
  message: string;
  retryable?: boolean;
  stage: EmbedErrorStage;
}

const LOOPBACK_HOSTS = new Set(["localhost", "127.0.0.1", "::1", "[::1]"]);

export function isEmbedSearch(search: string): boolean {
  return new URLSearchParams(search).get("embed") === "1";
}

export function parseEmbedOptions(search: string): EmbedOptions {
  const params = new URLSearchParams(search);
  const embed = isEmbedSearch(search);
  return {
    embed,
    host: embed ? parseHostOrigin(params.get("host")) : null,
    fit: params.get("fit") === "auto" ? "auto" : "fill",
    theme: embed ? parseTheme(params.get("theme")) : null,
    lang: embed ? parseLang(params.get("lang")) : null,
  };
}

function parseTheme(raw: string | null): EmbedThemePreference | null {
  return raw === "light" || raw === "dark" || raw === "system" ? raw : null;
}

/** `lang` 归一化：大小写与区域变体都接受（en-US → en、zh-CN/zh-TW → zh-CN），非法值忽略。 */
function parseLang(raw: string | null): EmbedLang | null {
  if (!raw) return null;
  const tag = raw.trim().toLowerCase().replace(/_/g, "-");
  const primary = tag.split("-")[0];
  if (primary === "zh") return "zh-CN";
  for (const code of EMBED_LANGS) {
    if (code.toLowerCase() === primary) return code;
  }
  return null;
}

/** host 必须是完整 origin：https，或回环地址上的 http（本地联调）。 */
export function parseHostOrigin(raw: string | null): string | null {
  if (!raw) return null;
  let u: URL;
  try {
    u = new URL(raw);
  } catch {
    return null;
  }
  const loopback = LOOPBACK_HOSTS.has(u.hostname.toLowerCase());
  if (u.protocol !== "https:" && !(u.protocol === "http:" && loopback)) return null;
  if (u.username || u.password || u.pathname !== "/" || u.search || u.hash) return null;
  return u.origin;
}

export function embedMessage(type: string, payload: Record<string, unknown> = {}): Record<string, unknown> {
  return { v: EMBED_VERSION, type, ...payload };
}

export function isEmbedMessageType(data: unknown, type: string): data is Record<string, unknown> {
  if (!data || typeof data !== "object") return false;
  const m = data as Record<string, unknown>;
  return m.v === EMBED_VERSION && m.type === type;
}

/** v1/v2 兼容的入站消息判断：新宿主可以带 v:2，老宿主 v:1 仍然接受。 */
export function isEmbedMessage(data: unknown, type: string): boolean {
  if (!data || typeof data !== "object") return false;
  const m = data as Record<string, unknown>;
  return (m.v === EMBED_VERSION || m.v === EMBED_PROTOCOL_V2) && m.type === type;
}

/** v2 消息（增量，老宿主按未知 type 忽略）。 */
export function embedMessageV2(type: string, payload: Record<string, unknown> = {}): Record<string, unknown> {
  return { v: EMBED_PROTOCOL_V2, type, ...payload };
}

/** 宿主 → iframe 的消息必须是 host origin 的父窗口发的。 */
export function isTrustedParentMessage(origin: string, source: unknown, host: string | null, parent: unknown): boolean {
  return !!host && origin === host && source === parent;
}

/** 宿主 -> iframe 的 connect 消息：一次性 code，只用于交换内存 Bearer。 */
export interface EmbedConnectPayload {
  code: string;
}

/** exchange 返回：ee_emb_ 内存 Bearer（只存内存，不回传给宿主，不写 localStorage）。 */
export interface EmbedExchangeResult {
  token: string;
  expires_in: number;
  limited: true;
  /** v2：mint 时绑定的站内路径（deeplink），exchange 后由 iframe 消费。 */
  next?: string;
  app: { slug: string; title: string };
  user: { username: string; display_name: string };
}

export class EmbedExchangeError extends Error {
  constructor(
    /** embed_code_invalid / embed_code_expired / embed_disabled / rate_limited */
    public code: string,
    message: string,
    /** 服务端 v1 错误体的 stage（exchange 阶段固定为 `exchange`，缺省按 exchange 处理）。 */
    public stage: string = "exchange",
  ) {
    super(message);
  }
}

/**
 * 用宿主给的一次性 code 换 ee_emb_ 内存 Bearer。必须同源调用（iframe 自己发），
 * 不带 cookie、响应也不会有 Set-Cookie。
 */
export async function exchangeEmbedCode(code: string): Promise<EmbedExchangeResult> {
  let res: Response;
  try {
    res = await fetch("/api/auth/embed/exchange", {
      method: "POST",
      credentials: "same-origin",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ code }),
    });
  } catch {
    throw new EmbedExchangeError("network", "无法连接 dsh-ee");
  }
  const text = await res.text();
  let data: unknown = null;
  try {
    data = text ? JSON.parse(text) : null;
  } catch {
    // 代理错误页之类的纯文本
  }
  const body = (data ?? {}) as { code?: string; error?: string; stage?: string };
  if (!res.ok) {
    throw new EmbedExchangeError(body.code || "embed_code_invalid", body.error || "登录码无效", body.stage || "exchange");
  }
  return data as EmbedExchangeResult;
}

/** 给宿主发消息；没配 host 或 host 非法时什么都不发（绝不用 "*"）。 */
export function postToHost(options: EmbedOptions, type: string, payload: Record<string, unknown> = {}): void {
  if (!options.embed || !options.host || typeof window === "undefined") return;
  window.parent.postMessage(embedMessage(type, payload), options.host);
}

/** v2 消息发送（hello/navigate-request/open-external/logged-out/deeplink）。 */
export function postToHostV2(options: EmbedOptions, type: string, payload: Record<string, unknown> = {}): void {
  if (!options.embed || !options.host || typeof window === "undefined") return;
  window.parent.postMessage(embedMessageV2(type, payload), options.host);
}

/**
 * 校验宿主发来的站内路径：只允许本应用 `/a/<slug>`（可带 query/hash），
 * 拒绝 `//`、反斜杠、控制字符、`..`/`.` 段。返回原 path 或 null。
 */
export function normalizeAppPath(path: unknown, slug: string): string | null {
  if (typeof path !== "string" || !slug) return null;
  if (path.length > 2048 || /[\\\u0000-\u001f\u007f]/.test(path)) return null;
  const prefix = `/a/${slug}`;
  if (!path.startsWith(prefix)) return null;
  const rest = path.slice(prefix.length);
  if (rest && !rest.startsWith("/") && !rest.startsWith("?") && !rest.startsWith("#")) return null;
  const pathOnly = path.split(/[?#]/, 1)[0];
  for (const seg of pathOnly.split("/")) {
    if (seg === ".." || seg === ".") return null;
  }
  return path;
}

/** 浏览器点击的链接是否留在本应用内（交给前端路由，不跳整页）。 */
export function isAppPath(path: string, slug: string): boolean {
  return normalizeAppPath(path, slug) !== null;
}

export function capEmbedHeight(height: number, max = EMBED_MAX_HEIGHT): number {
  if (!Number.isFinite(height)) return 0;
  return Math.max(0, Math.min(Math.round(height), max));
}

/** 移动 UA 判定（只用于显示兜底提示，不影响内容与认证）。 */
export function isMobileUserAgent(ua: string): boolean {
  return /Android|iPhone|iPad|iPod|Mobile|HarmonyOS|Silk/i.test(ua);
}

/** 当前 iframe 视口是否算移动端：窄屏或移动 UA。 */
export function isMobileEmbedViewport(width: number, ua: string): boolean {
  return width > 0 && width < EMBED_MOBILE_MAX_WIDTH ? true : isMobileUserAgent(ua);
}

/** `ready`/`hello` 里上报的视口信息与 `dsh-ee:resize` 共用同一套尺寸口径。 */
export function embedViewport(): EmbedViewport {
  const width = typeof window === "undefined" ? 0 : window.innerWidth || document.documentElement.clientWidth || 0;
  const height = typeof window === "undefined" ? 0 : window.innerHeight || document.documentElement.clientHeight || 0;
  const ua = typeof navigator === "undefined" ? "" : navigator.userAgent;
  return { mobile: isMobileEmbedViewport(width, ua), width, height };
}

/**
 * fit=auto 上报的内容高度：量页面根节点（`#root`）的内容高度，而不是
 * `document.documentElement.scrollHeight`——后者至少等于 iframe 视口高度，
 * 宿主按它撑高后内容变少时高度再也缩不回来。`min-h-screen` 只保留在 fill 模式，
 * 嵌入 + auto 时页面改为内容驱动，见 app-web.tsx。
 */
export function embedContentHeight(root: HTMLElement | null = typeof document === "undefined" ? null : document.getElementById("root")): number {
  if (!root) {
    const html = typeof document === "undefined" ? null : document.documentElement;
    return capEmbedHeight(html ? html.scrollHeight : 0);
  }
  // 向上取整：宿主按上报值设 iframe 高度时不会因小数差 1px 出现内部滚动条。
  const measured = Math.ceil(Math.max(root.getBoundingClientRect().height, root.scrollHeight));
  return capEmbedHeight(measured || root.clientHeight);
}

/** 当前实际渲染的主题：嵌入页只改 DOM，不写 localStorage。 */
export function resolvedDocumentTheme(): EmbedTheme {
  return document.documentElement.classList.contains("dark") ? "dark" : "light";
}

export function applyEmbedTheme(theme: EmbedTheme): void {
  const root = document.documentElement;
  root.classList.toggle("dark", theme === "dark");
  root.style.colorScheme = theme;
}

/** 失败态的「在浏览器打开」：去掉嵌入参数，用顶层窗口打开，避免在 iframe 里跳登录。 */
export function nonEmbedURL(): string {
  const u = new URL(window.location.href);
  for (const k of ["embed", "host", "fit", "theme", "lang"]) u.searchParams.delete(k);
  return `${u.pathname}${u.search}${u.hash}`;
}

/** 接口错误 → 宿主可判断的失败码；stage 固定为 `load`（页面/接口加载阶段）。 */
export function embedErrorForStatus(status: number | null, message: string): EmbedError {
  if (status === 401) return { code: "auth_required", message, retryable: false, stage: "load" };
  if (status === 403 || status === 404) return { code: "forbidden", message, retryable: false, stage: "load" };
  return { code: "load_failed", message, retryable: true, stage: "load" };
}
