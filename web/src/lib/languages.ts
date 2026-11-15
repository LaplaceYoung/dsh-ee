/** 语言偏好：与服务端约定的规范短码是 zh / en / es / pt / hi（见 server/internal/i18n）。 */
import i18n from "../i18n";

export const LANGUAGE_CODES = ["zh", "en", "es", "pt", "hi"] as const;
export type LanguageCode = (typeof LANGUAGE_CODES)[number];

/** 语言选项的标签用各语言自己的写法（endonym），不翻译。 */
export const LANGUAGE_OPTIONS: { code: LanguageCode; label: string }[] = [
  { code: "zh", label: "简体中文" },
  { code: "en", label: "English" },
  { code: "es", label: "Español" },
  { code: "pt", label: "Português" },
  { code: "hi", label: "हिन्दी" },
];

export function isLanguageCode(value: unknown): value is LanguageCode {
  return typeof value === "string" && (LANGUAGE_CODES as readonly string[]).includes(value);
}

/**
 * 服务端规范短码 → 本地 i18n 语言码。
 * 界面语言包目前只有简体中文与英文；es/pt/hi 先回退英文，等语言包 PR 合并后改成各自的码。
 * 服务端偏好仍按 es/pt/hi 保存，飞书等消息已经按这些语言渲染。
 */
export function i18nCodeOf(code: LanguageCode): string {
  return code === "zh" ? "zh-CN" : "en";
}

/** 立即切换界面语言（es/pt/hi 暂时回退英文）。 */
export function setUiLanguage(code: LanguageCode): void {
  void i18n.changeLanguage(i18nCodeOf(code));
}

/** 把账号上的语言偏好应用到界面；空/非法值不做任何事（保留浏览器检测结果）。 */
export function applyLanguagePreference(code: unknown): void {
  if (isLanguageCode(code)) setUiLanguage(code);
}
