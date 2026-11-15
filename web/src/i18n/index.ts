/**
 * 界面多语言入口。中文是源语言（i18n/zh），其它语言按中文的结构写，缺键会编译报错。
 * 语言注册表（支持哪些语言、语言包、浏览器语言归一化）在 ./langs.ts：
 * 加一门语言只改那里 + 新增语言目录，见 ./README.md。
 * 新增文案：先在 zh 对应模块加键，再在其它已翻译语言的同一模块补上；组件里用 useTranslation() 的 t("模块.键")。
 * 后端返回的错误信息与模拟数据暂未翻译。
 */
import i18n from "i18next";
import LanguageDetector from "i18next-browser-languagedetector";
import { initReactI18next } from "react-i18next";
import { LANGUAGES, LANG_STORAGE_KEY, detectLanguage, fallbackLng, resources } from "./langs";
import type zh from "./zh";

export { LANGUAGES, LANG_STORAGE_KEY, detectLanguage, fallbackLng, resources, type Lang } from "./langs";

declare module "i18next" {
  interface CustomTypeOptions {
    defaultNS: "translation";
    resources: { translation: typeof zh };
  }
}

void i18n
  .use(LanguageDetector)
  .use(initReactI18next)
  .init({
    resources,
    supportedLngs: LANGUAGES.map((l) => l.code),
    // 不支持的浏览器语言看英文；缺 key 也回落英文，再缺用 zh 源文案（见 README 的 fallback 规则）。
    fallbackLng,
    detection: {
      order: ["localStorage", "navigator"],
      lookupLocalStorage: LANG_STORAGE_KEY,
      caches: ["localStorage"],
      // zh-TW、zh-HK 等都用简体中文，en-US、en-GB 用英文，未注册语言回落英文；
      // 加语言后区域变体自动归一（规则见 ./langs.ts 的 detectLanguage）。
      // 注意不要改用 nonExplicitSupportedLngs：在当前 i18next 版本下它会让 zh-CN 取不到文案。
      convertDetectedLanguage: detectLanguage,
    },
    interpolation: { escapeValue: false },
    returnNull: false,
  });

const syncHtmlLang = (lng: string) => {
  document.documentElement.lang = lng.startsWith("zh") ? "zh-CN" : lng;
};
syncHtmlLang(i18n.resolvedLanguage ?? "en");
i18n.on("languageChanged", syncHtmlLang);

/** 当前界面语言是否是中文（用于挑选数据里自带的多语言字段）。 */
export const isZh = () => (i18n.resolvedLanguage ?? "zh-CN").startsWith("zh");

export default i18n;
