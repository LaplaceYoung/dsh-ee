/**
 * 语言注册表：支持哪些语言、语言包从哪里来、浏览器语言怎么归一化。
 *
 * 加一门语言的唯一入口（详见同目录 README.md）：
 *   ① 新建 `i18n/<code>/`，照 `zh/` 的结构写，每个命名空间 `satisfies Messages<typeof zhNs>`；
 *   ② 在本文件 import 语言包，在 LANGUAGES 加一行、在 resources 加一项。
 * 组件、`index.ts` 与 e2e 脚本都只读这张表，不要在各处再写死语言列表。
 *
 * 五语目标是 zh-CN（源）/ en / es / pt / hi；本轮（P0）先只注册 zh-CN 与 en，
 * 语言包上线时按上面两步加。不要为了加语言改成异步 init / 懒加载（独立 PR 再做）。
 */
import zh from "./zh";
import en from "./en";

export const LANGUAGES = [
  { code: "zh-CN", label: "简体中文" },
  { code: "en", label: "English" },
] as const;

export type Lang = (typeof LANGUAGES)[number]["code"];

/** 语言选择的 localStorage 键；`web/index.html` 的首屏内联脚本也读它，改键要一起改。 */
export const LANG_STORAGE_KEY = "dsh-ee:lang";

/** 兜底语言：不支持的浏览器语言、缺翻译的 key 都落到英文（en 缺 key 时再用 zh 源文案，见 README）。 */
export const fallbackLng: Lang = "en";

/** 静态语言包：键必须是 LANGUAGES 里的 code。当前不做异步加载，加语言时在这里补一项。 */
export const resources: Record<Lang, { translation: typeof zh }> = {
  "zh-CN": { translation: zh },
  en: { translation: en },
};

/**
 * 浏览器语言 / localStorage 里的值 → 已注册语言。
 *
 * 规则：zh* → zh-CN（繁中也用简体）；已注册语言的区域变体按主语言归一（en-US → en，
 * 将来注册 es 后 es-MX → es）；其它未注册语言（ja、fr…）→ fallbackLng（en）。
 * 加语言时不需要改这里：注册进 LANGUAGES 后，es* / pt* / hi* 会自动归一。
 */
export function detectLanguage(raw: string): Lang {
  const tag = String(raw ?? "")
    .trim()
    .toLowerCase()
    .replace(/_/g, "-");
  if (!tag) return fallbackLng;
  const primary = tag.split("-")[0];
  if (primary === "zh") return "zh-CN";
  for (const l of LANGUAGES) {
    const code = l.code.toLowerCase();
    if (code === tag || code.split("-")[0] === primary) return l.code;
  }
  return fallbackLng;
}
