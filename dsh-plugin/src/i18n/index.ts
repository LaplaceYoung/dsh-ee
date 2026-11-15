/**
 * 插件自建的轻量 i18n（不引第三方库，随现有 esbuild 打包）。
 *
 * 用法：
 * - 文案按插件模块分命名空间放在 zh.ts（源语言）与 en.ts；组件里用 `t("模块.键", { name })` 取文案。
 * - zh 是键集合的唯一来源；en 用 `satisfies Messages<typeof zh>` 镜像它，缺键编译报错。
 * - es / pt / hi 用 DeepPartial 镜像（当前已补全 431 键）；缺键时运行时逐键回退英文。
 * - 缺键回退 en，再缺回退键原文；插值只做简单的 `{{name}}` 字符串替换（不做复数机制）。
 * - 日志（console.*）、命令名、URL、JSON、包名、环境变量不翻译，保持原样。
 */
import zh from "./zh.js";
import en from "./en.js";
import es from "./es.js";
import pt from "./pt.js";
import hi from "./hi.js";
import { DEFAULT_LANG, LANGS, browserLanguages, detectLang, envLanguages, hostLocaleId, normalizeLang } from "./langs.js";
import type { Lang } from "./langs.js";
import type { DeepPartial, DotPaths, Messages } from "./types.js";

export { DEFAULT_LANG, LANGS, browserLanguages, detectLang, envLanguages, hostLocaleId, normalizeLang };
export type { Lang };

/** 五种语言的文案表。es / pt / hi 是增量覆盖，缺的键在 lookup 时回退 en。 */
const TABLES: Record<Lang, DeepPartial<Messages<typeof zh>>> = { "zh-CN": zh, en, es, pt, hi };

/** 全部可用文案键（点分路径）；由 zh.ts 推导，写错键编译报错。 */
export type MessageKey = DotPaths<typeof zh>;

let active: Lang | undefined;
const listeners = new Set<() => void>();

/** 文案键 → 文案；没写 params 时不替换占位符。 */
export function t(key: MessageKey, params?: Record<string, string | number | null | undefined>): string {
  const template = lookup(getLang(), key) ?? lookup("en", key) ?? key;
  if (!params) return template;
  return template.replace(/\{\{(\w+)\}\}/g, (match, name: string) => {
    const value = params[name];
    return value === undefined || value === null ? match : String(value);
  });
}

/** 当前语言：先由 initI18n/setLang 决定；还没决定时按浏览器/环境变量探测一次。 */
export function getLang(): Lang {
  if (active === undefined) active = detectLang({ browser: browserLanguages(), env: envLanguages() });
  return active;
}

/** 切换当前语言（宿主 locale 变了、或测试里指定语言时用）。返回实际生效的语言码。 */
export function setLang(lang: Lang | string): Lang {
  const next = normalizeLang(lang) ?? DEFAULT_LANG;
  if (next === getLang()) return next;
  active = next;
  for (const listener of [...listeners]) {
    try {
      listener();
    } catch {
      // 一个订阅者出错不影响其它
    }
  }
  return next;
}

/** 当前语言的 BCP 47 标签，给 toLocaleString / toLocaleDateString 用（zh-CN / en / es / pt / hi）。 */
export function localeTag(lang: Lang = getLang()): string {
  return lang;
}

/** 订阅语言变化（宿主切换语言时刷新界面用）。 */
export function subscribeLang(listener: () => void): () => void {
  listeners.add(listener);
  return () => { listeners.delete(listener); };
}

/** 清掉已决定的语言，回到「下一次按环境重新探测」（测试用）。 */
export function resetLang(): void {
  active = undefined;
}

/**
 * 初始化语言：按「宿主 locale → 浏览器 → 环境变量 → 默认 zh」探测一次；
 * 宿主有 locale 服务时再订阅它，DSH 设置里切语言后保持同步。
 * 客户端与宿主端的 apply 入口都应在注册任何界面/工具之前调用一次。
 */
export function initI18n(ctx?: any): Lang {
  active = detectLang({ host: hostLocaleId(ctx), browser: browserLanguages(), env: envLanguages() });
  watchHostLang(ctx);
  return active;
}

/** 跟随宿主 locale 服务切换语言；宿主没有这个服务（旧版 DSH）时什么也不做。 */
function watchHostLang(ctx: any): void {
  try {
    const service = typeof ctx?.get === "function" ? ctx.get("locale") : undefined;
    if (typeof service?.subscribe !== "function") return;
    const sync = (): void => {
      const lang = normalizeLang(hostLocaleId(ctx));
      if (lang) setLang(lang);
    };
    if (typeof ctx?.effect === "function") {
      ctx.effect(() => {
        const off = service.subscribe(sync);
        sync();
        return () => {
          try {
            off?.();
          } catch {
            // 退订失败不影响卸载
          }
        };
      });
    } else {
      sync();
    }
  } catch {
    // 宿主 locale 服务行为不符预期：保留探测到的语言
  }
}

/** 按点分键在一张表里找文案；中途不是对象或最终不是字符串都返回 undefined。 */
function lookup(lang: Lang, key: string): string | undefined {
  let node: unknown = TABLES[lang];
  for (const part of key.split(".")) {
    if (node === null || typeof node !== "object") return undefined;
    node = (node as Record<string, unknown>)[part];
  }
  return typeof node === "string" ? node : undefined;
}
