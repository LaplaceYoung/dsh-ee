/**
 * 支持的语言码与语言探测。
 *
 * 设施固定支持 5 个语言码：zh-CN / en / es / pt / hi。五份文案表均已补全（431 键）；
 * 缺键时运行时逐键回退英文，不需要改这里的代码。
 *
 * 探测顺序（第一个能识别的语言胜出）：
 * 1. DSH 宿主 locale 服务（客户端 `ctx.get("locale")` 的 getSnapshot().active）；
 * 2. 浏览器 navigator.languages / navigator.language（只在浏览器环境读）；
 * 3. 环境变量 DSH_EE_LANG / LC_ALL / LC_MESSAGES / LANG；
 * 4. 默认 zh-CN（与插件原先的中文界面一致，中文用户看不到变化）。
 *
 * 说明：Node 也暴露了一个全局 navigator（报的是机器的语言），它不能决定界面语言；
 * 只有存在 `window` 的浏览器环境才看 navigator，这一点与 DSH 自己的 locale 插件一致。
 */

/** 支持的语言码（也是语言选择的固定集合）。 */
export const LANGS = ["zh-CN", "en", "es", "pt", "hi"] as const;

/** 语言码。 */
export type Lang = (typeof LANGS)[number];

/** 探测不到语言时的默认语言：与插件原先的界面语言一致。 */
export const DEFAULT_LANG: Lang = "zh-CN";

/** 语言码 → 主语言子标签：把 BCP 47 标签（zh-TW / en_US / pt-BR）归一到五选一。 */
const PRIMARY_LANG: Record<string, Lang> = { zh: "zh-CN", en: "en", es: "es", pt: "pt", hi: "hi" };

/** 把任意 BCP 47 风格标签归一成支持的语言码；不支持的语言（ja / fr / de / C …）返回 undefined。 */
export function normalizeLang(tag: unknown): Lang | undefined {
  if (typeof tag !== "string") return undefined;
  const primary = tag.trim().toLowerCase().replace(/_/g, "-").split("-")[0];
  return primary ? PRIMARY_LANG[primary] : undefined;
}

/** 语言探测的输入。除 host 外都可能是多个候选（浏览器语言列表、LC_ALL / LANG 的依次回退）。 */
export interface LangSources {
  /** DSH 宿主 locale 服务的当前语言（`ctx.get("locale")` 的 getSnapshot().active）。 */
  host?: string | readonly (string | undefined)[] | undefined;
  /** 浏览器 navigator.languages（末尾补 navigator.language）。只在浏览器环境读。 */
  browser?: readonly (string | undefined)[] | undefined;
  /** 环境变量：DSH_EE_LANG / LC_ALL / LC_MESSAGES / LANG。服务端与测试用。 */
  env?: readonly (string | undefined)[] | undefined;
}

/** 按 宿主 → 浏览器 → 环境变量 → 默认 zh-CN 的顺序取第一个能识别的语言。 */
export function detectLang(sources: LangSources): Lang {
  for (const source of [sources.host, sources.browser, sources.env]) {
    if (source === undefined) continue;
    for (const tag of typeof source === "string" ? [source] : source) {
      const lang = normalizeLang(tag);
      if (lang) return lang;
    }
  }
  return DEFAULT_LANG;
}

/**
 * 读 DSH 宿主的 locale 服务（DSH 0.2.2 线客户端的 `locale` 服务）。
 * 旧版 DSH 没有这个服务时返回 undefined；探测失败也返回 undefined，绝不拖垮插件启动。
 */
export function hostLocaleId(ctx: any): string | undefined {
  try {
    const service = typeof ctx?.get === "function" ? ctx.get("locale") : undefined;
    const snapshot = typeof service?.getSnapshot === "function"
      ? service.getSnapshot()
      : typeof service?.getLocale === "function" ? service.getLocale() : undefined;
    return typeof snapshot?.active === "string" ? snapshot.active : undefined;
  } catch {
    return undefined;
  }
}

/** 浏览器声明的语言列表。Node 的 navigator 不算：只有存在 window 的浏览器环境才采信。 */
export function browserLanguages(): (string | undefined)[] {
  if (typeof window === "undefined") return [];
  try {
    const nav = (globalThis as { navigator?: { languages?: readonly string[]; language?: string } }).navigator;
    const list = [...(Array.isArray(nav?.languages) ? nav.languages : []), nav?.language];
    return list.filter((tag) => typeof tag === "string" && tag !== "");
  } catch {
    return [];
  }
}

/** 环境变量里声明的语言；DSH_EE_LANG 优先，供「这台机器/测试」显式指定界面语言。 */
export function envLanguages(env: Record<string, string | undefined> | undefined = typeof process === "undefined" ? undefined : process.env): (string | undefined)[] {
  if (!env) return [];
  return [env.DSH_EE_LANG, env.LC_ALL, env.LC_MESSAGES, env.LANG];
}
