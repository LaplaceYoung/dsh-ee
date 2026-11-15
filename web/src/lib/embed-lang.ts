/**
 * 嵌入页 `?lang=` 的运行时应用：只改本次 iframe 的界面语言，**不写 localStorage**。
 *
 * 不能直接 `i18n.changeLanguage(lang)`：i18next 的 localStorage 语言检测器会在切换时把语言
 * 写回 `dsh-ee:lang`，宿主每打开一次 iframe 就会覆盖用户自己的界面语言偏好。这里在切换后
 * 把键恢复成原值，语义与 `?theme=` 一致（只作用于本次渲染，见 docs/embedding.md）。
 */
import i18n from "../i18n";
import { LANGUAGES, LANG_STORAGE_KEY, fallbackLng, type Lang } from "../i18n/langs";
import type { EmbedLang } from "./embed";

/** 未注册的语言（例如语言包还没上线的 es/pt/hi）按 fallback 规则回退英文，而不是显示原始 key。 */
export function resolveEmbedLang(lang: EmbedLang | null): Lang | null {
  if (!lang) return null;
  return LANGUAGES.some((l) => l.code === lang) ? (lang as Lang) : fallbackLng;
}

/** 应用 URL 上的语言；调用方在切换完成前不要发 hello/ready，否则 locale 会报旧值。 */
export async function applyEmbedLang(lang: EmbedLang | null): Promise<void> {
  const target = resolveEmbedLang(lang);
  if (!target) return;
  // changeLanguage 会同步触发检测器的 localStorage 缓存，先记下原值，切换后恢复。
  let readable = false;
  let previous: string | null = null;
  try {
    previous = localStorage.getItem(LANG_STORAGE_KEY);
    readable = true;
  } catch {
    // localStorage 被禁用（隐私模式）：本来也写不进去，不用恢复。
  }
  await i18n.changeLanguage(target);
  if (!readable) return;
  try {
    if (previous === null) localStorage.removeItem(LANG_STORAGE_KEY);
    else localStorage.setItem(LANG_STORAGE_KEY, previous);
  } catch {
    // 见上：存储不可用时只影响「下次打开」，不影响本次语言。
  }
}
