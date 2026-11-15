/**
 * 统一格式化层：日期 / 时间 / 数字 / 字节 / 相对时间一律按「界面语言」渲染。
 *
 * 为什么不用浏览器 locale：界面语言由 i18n 决定（中文界面 + 英文系统很常见，反之亦然），
 * 所以格式化语言必须跟 `i18n.resolvedLanguage` 走，而不是 JS 默认的浏览器 locale。
 *
 * 语言来源：每个函数都接受可选的 `lang`；不传时读 i18next 单例的 `resolvedLanguage`
 * —— 就是 `@/i18n` 里 `.init()` 的那个实例（i18next 的默认导出是同一份单例，
 * react-i18next 在没有 Provider 时也用它）。这里刻意不 import `@/i18n`：那个模块
 * 初始化时直接碰 `document`/localStorage，在浏览器外无法 import，而本文件需要能被
 * 纯 Node 直接 import 做五语言自检（见设计约定：Intl 逻辑必须是可直接 import 的纯函数）。
 * 因此本文件除了 i18next 单例不碰任何全局状态，显式传 `lang` 时连单例都不读。
 *
 * 只依赖浏览器内建 Intl（不引日期库），invalid 日期统一回退 "—"。
 * 语言归一到 FORMAT_LANGS：zh-CN / en / es / pt / hi；未知或空值回退 en。
 */
import i18n from "i18next";

/** 界面支持的语言；新增语言时改这里即可（不改 i18n 的注册结构）。 */
export const FORMAT_LANGS = ["zh-CN", "en", "es", "pt", "hi"] as const;
export type FormatLang = (typeof FORMAT_LANGS)[number];

/** 日期类入参：Date、ISO 字符串或毫秒时间戳。 */
export type DateInput = Date | string | number;

/** 各场景的默认展示口径；需要别的样式时由调用方传 options 覆盖。 */
const DATE_TIME_OPTIONS: Intl.DateTimeFormatOptions = { dateStyle: "medium", timeStyle: "short" };
const DATE_ONLY_OPTIONS: Intl.DateTimeFormatOptions = { year: "numeric", month: "short", day: "numeric" };
const TIME_ONLY_OPTIONS: Intl.DateTimeFormatOptions = { hour: "2-digit", minute: "2-digit" };

/** 把 i18n / 浏览器 / react-day-picker 给的 locale 归一到支持的语言；非法或空值回退 en。 */
export function resolveLang(lang?: string | null): FormatLang {
  const raw = lang ?? i18n.resolvedLanguage ?? i18n.language ?? "";
  const code = String(raw).toLowerCase();
  // 先按语言前缀归一：zh-TW 也显示简体中文（与 i18n 的 convertDetectedLanguage 一致）。
  if (code.startsWith("zh")) return "zh-CN";
  if (code.startsWith("en")) return "en";
  if (code.startsWith("es")) return "es";
  if (code.startsWith("pt")) return "pt";
  if (code.startsWith("hi")) return "hi";
  return "en";
}

/**
 * 转 Date：支持 Date / 毫秒时间戳 / ISO 字符串。
 * 纯日期字符串 "YYYY-MM-DD" 按本地日期解析 —— `new Date("2026-11-14")` 是 UTC 零点，
 * 在西半球时区会显示成前一天，这里按调用方语义（日历日期）处理成本地零点。
 */
function toDate(value: DateInput | null | undefined): Date | null {
  // 空值统一按「没有值」处理：调用方拿到的可空时间字段不用自己判空。
  if (value == null || value === "") return null;
  if (value instanceof Date) return Number.isNaN(value.getTime()) ? null : value;
  if (typeof value === "string" && /^\d{4}-\d{2}-\d{2}$/.test(value)) {
    const [y, m, d] = value.split("-").map(Number);
    const local = new Date(y, m - 1, d);
    return Number.isNaN(local.getTime()) ? null : local;
  }
  const d = new Date(value);
  return Number.isNaN(d.getTime()) ? null : d;
}

function formatWith(value: DateInput | null | undefined, lang: string | null | undefined, options: Intl.DateTimeFormatOptions): string {
  const d = toDate(value);
  if (!d) return "—";
  return new Intl.DateTimeFormat(resolveLang(lang), options).format(d);
}

/** 日期 + 时间，例如 zh-CN「2026年11月14日 18:14」、en「Nov 14, 2026, 6:14 PM」。 */
export function formatDateTime(value: DateInput, lang?: string | null): string {
  return formatWith(value, lang, DATE_TIME_OPTIONS);
}

/** 仅日期；默认 medium（2026年11月14日 / Nov 14, 2026），可用 options 覆盖口径。 */
export function formatDateOnly(
  value: DateInput,
  lang?: string | null,
  options: Intl.DateTimeFormatOptions = DATE_ONLY_OPTIONS,
): string {
  return formatWith(value, lang, options);
}

/** 仅时间（时:分），例如「18:14」/「6:14 PM」。 */
export function formatTimeOnly(value: DateInput, lang?: string | null): string {
  return formatWith(value, lang, TIME_ONLY_OPTIONS);
}

/**
 * 数字：默认千分位（按语言分组，如 en「1,234,567.89」、es「1.234.567,89」、hi「12,34,567.89」）。
 * 需要固定小数位时用 formatDecimal；需要货币/百分比等样式时传 Intl.NumberFormatOptions。
 */
export function formatNumber(value: number, lang?: string | null, options?: Intl.NumberFormatOptions): string {
  return new Intl.NumberFormat(resolveLang(lang), options).format(value);
}

/** 固定小数位的数字（原本用 toFixed 的地方改用它，小数点/分组才跟语言）。 */
export function formatDecimal(value: number, digits: number, lang?: string | null): string {
  return formatNumber(value, lang, { minimumFractionDigits: digits, maximumFractionDigits: digits });
}

const RELATIVE_UNITS: ReadonlyArray<readonly [Intl.RelativeTimeFormatUnit, number]> = [
  ["second", 1],
  ["minute", 60],
  ["hour", 3_600],
  ["day", 86_400],
  ["month", 2_592_000], // 30 天
  ["year", 31_536_000], // 365 天
];

/**
 * 相对时间，例如 zh「3 小时前」、en「3 hours ago」、es「hace 3 horas」。
 * 走 Intl.RelativeTimeFormat，不依赖 i18n 文案 key（该 API 自带各语言词形与方向）。
 * 注意：`review.hoursAgo/daysAgo` 目前仍走 i18n key（本 PR 不新增五语 key），
 * 等语言包合并后可以切到本函数。
 */
export function formatRelativeTime(value: DateInput, lang?: string | null): string {
  const d = toDate(value);
  if (!d) return "—";
  const diffSeconds = (d.getTime() - Date.now()) / 1000;
  const abs = Math.abs(diffSeconds);
  let unit: Intl.RelativeTimeFormatUnit = "second";
  let unitSeconds = 1;
  for (const [u, s] of RELATIVE_UNITS) {
    if (abs >= s) {
      unit = u;
      unitSeconds = s;
    }
  }
  return new Intl.RelativeTimeFormat(resolveLang(lang), { numeric: "auto" }).format(
    Math.round(diffSeconds / unitSeconds),
    unit,
  );
}

/**
 * 字节数：沿用原有口径（GB/MB 保留 1 位小数、KB 取整、B 为整数，单位符号保持 B/KB/MB/GB），
 * 只把数字换成当前语言的分组/小数点。原实现是 `${(n / (1 << 30)).toFixed(1)} GB` 这类硬编码。
 */
export function formatBytes(n: number, lang?: string | null): string {
  if (n >= 1 << 30) return `${formatDecimal(n / (1 << 30), 1, lang)} GB`;
  if (n >= 1 << 20) return `${formatDecimal(n / (1 << 20), 1, lang)} MB`;
  if (n >= 1024) return `${formatNumber(Math.round(n / 1024), lang)} KB`;
  return `${formatNumber(n, lang)} B`;
}
