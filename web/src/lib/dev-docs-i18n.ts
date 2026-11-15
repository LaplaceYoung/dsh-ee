/**
 * 站内开发文档的五语取值（标题 / 摘要 / 正文）。
 *
 * 构建期生成物：
 * - web/src/generated/dev-docs.ts：元数据（title / summary 五键）+ 中文正文 markdown
 *   + hasBody（哪些语言有译文正文）；
 * - web/src/generated/dev-docs-bodies.<lang>.ts：en / es / pt / hi 各一个「slug → 正文」映射。
 *
 * 正文按语言懒加载：本文件里有四个**显式**的 import()（不动态拼路径，否则 Vite 无法把每种语言
 * 拆成独立 chunk）。模块级缓存 Promise，同一语言只下载/解析一次；中文正文恒在元数据里，直接
 * 同步取，不触发任何 chunk。页面与搜索都走这里的 pickLocalized / docBodyForAsync / hasBodyFor。
 *
 * 正文是渐进补译的：`hasBody` 只标实际存在的译文语言，所以正文的取值要区分「请求语言」与
 * 「实际拿到的语言」，用 fallback 标记是否发生了回退（请求语言 → en → zh）。
 *
 * 注意：本文件被 scripts/docs/build.test.mjs 用 Node 直接 import（--experimental-strip-types），
 * 所以运行期只依赖下面四个生成的正文模块（import() 是静态字面量），不 import 任何页面数据；
 * 动态 import 显式写 `.ts` 后缀，Node 的类型擦除也能直接解析（tsconfig 已开 allowImportingTsExtensions）。
 */
import type { DevDoc, DocLang, LocalizedText } from "../data/dev-docs-types";

/** 五语键；顺序即生成物里的书写顺序，与 scripts/docs/build.mjs 的 DOC_LANGS 一致。 */
export const DOC_LANGS = ["zh", "en", "es", "pt", "hi"] as const;

/** 有独立正文模块的语言（中文是源语言，正文在 DevDoc.body）。 */
export const BODY_LANGS = ["en", "es", "pt", "hi"] as const;

export type BodyLang = (typeof BODY_LANGS)[number];

export type { DocLang, LocalizedText };

/** 某语言的译文正文索引：slug → Markdown。 */
export type DocBodies = Record<string, string>;

/** 多语言的译文正文（docBodyForAsync / 测试注入用）。 */
export type DocBodiesByLang = Partial<Record<BodyLang, DocBodies>>;

/** 运行时判断某个语言键是否属于五语（resolvedLanguage 是自由字符串，不能直接下标取值）。 */
export function isDocLang(value: string): value is DocLang {
  return (DOC_LANGS as readonly string[]).includes(value);
}

/** 把 i18n 的 resolvedLanguage（zh-CN / en-US / es / pt-BR / hi-IN …）归一到文档语言；未知语言按英文处理。 */
export function docLang(language: string | undefined | null): DocLang {
  const tag = (language ?? "").trim().toLowerCase();
  if (tag.startsWith("zh")) return "zh";
  if (tag.startsWith("es")) return "es";
  if (tag.startsWith("pt")) return "pt";
  if (tag.startsWith("hi")) return "hi";
  return "en";
}

/**
 * 取某语言的标题 / 摘要：该语言 → en → zh。
 * es / pt / hi 缺译（旧静态兜底数据）时回退英文；英文也空时回退中文。
 */
export function pickLocalized(field: LocalizedText, language: string | undefined | null): string {
  const value = field[docLang(language)];
  if (value && value.trim()) return value;
  return field.en || field.zh;
}

/** 正文取值结果：text 是要渲染的 Markdown；lang 是实际拿到的语言；fallback 表示不是请求语言。 */
export interface DocBody {
  text: string;
  lang: DocLang;
  fallback: boolean;
}

/** 请求语言 → 实际正文语言的回退链（与生成器的「该语言 → en → zh」一致）。 */
export function bodyLangChain(language: string | undefined | null): DocLang[] {
  const requested = docLang(language);
  if (requested === "zh") return ["zh"];
  if (requested === "en") return ["en", "zh"];
  return [requested, "en", "zh"];
}

/**
 * 四个正文模块的加载器：显式字面量 import()，Vite 会据此把每种语言拆成独立 chunk。
 * 不要在运行时拼路径（例如 `import(\`../generated/dev-docs-bodies.${lang}\`)`），否则 Vite
 * 只能把四个模块打进同一个包、失去按语言懒加载的意义。
 */
const bodyImporters: Record<BodyLang, () => Promise<DocBodies>> = {
  en: () => import("../generated/dev-docs-bodies.en.ts").then((mod) => mod.default),
  es: () => import("../generated/dev-docs-bodies.es.ts").then((mod) => mod.default),
  pt: () => import("../generated/dev-docs-bodies.pt.ts").then((mod) => mod.default),
  hi: () => import("../generated/dev-docs-bodies.hi.ts").then((mod) => mod.default),
};

const bodyCache = new Map<BodyLang, Promise<DocBodies>>();

/**
 * 加载某语言的译文正文（slug → Markdown）。模块级缓存 Promise：同一语言并发/重复调用只加载一次。
 * zh 是源语言，正文在元数据里，这里返回空对象（调用方应从 doc.body 直接取）。
 * 加载失败时清掉缓存项，允许下次重试；失败的 chunk 不会让页面白屏（调用方回退中文正文）。
 */
export function loadDocBodies(language: string | undefined | null): Promise<DocBodies> {
  const lang = docLang(language);
  if (lang === "zh") return Promise.resolve({});
  const cached = bodyCache.get(lang);
  if (cached) return cached;
  const pending = bodyImporters[lang]().catch((error: unknown) => {
    bodyCache.delete(lang);
    throw error;
  });
  bodyCache.set(lang, pending);
  return pending;
}

/**
 * 取某篇文档在请求语言下的译文正文（只查该语言，不拼中文回退）；没有该语言正文时返回 null。
 * 需要完整回退链（请求语言 → en → zh）时用 docBodyForAsync。
 */
export async function loadDocBody(slug: string, language: string | undefined | null): Promise<{ text: string; lang: BodyLang } | null> {
  const lang = docLang(language);
  if (lang === "zh") return null;
  const text = (await loadDocBodies(lang))[slug];
  return text?.trim() ? { text, lang } : null;
}

/**
 * 从已加载的正文里按「请求语言 → en → zh」选正文（纯函数，不触发加载；测试与搜索注入用）。
 * 中文正文从 doc.body 取，所以回退链最后一定能落到中文。
 */
export function pickDocBody(doc: DevDoc, language: string | undefined | null, bodies: DocBodiesByLang = {}): DocBody {
  const requested = docLang(language);
  for (const lang of bodyLangChain(requested)) {
    if (lang === "zh") {
      if (doc.body.trim()) return { text: doc.body, lang: "zh", fallback: requested !== "zh" };
      continue;
    }
    const text = bodies[lang]?.[doc.slug] ?? "";
    if (text.trim()) return { text, lang, fallback: lang !== requested };
  }
  return { text: doc.body, lang: "zh", fallback: requested !== "zh" };
}

/**
 * 异步取正文：按回退链逐语言加载对应 chunk（先用 hasBody 元数据跳过确定没有的语言），
 * 再按「请求语言 → en → zh」选。请求语言是 zh 时完全同步、不加载任何 chunk。
 */
export async function docBodyForAsync(doc: DevDoc, language: string | undefined | null): Promise<DocBody> {
  const requested = docLang(language);
  const bodies: DocBodiesByLang = {};
  for (const lang of bodyLangChain(requested)) {
    if (lang === "zh") continue;
    // 元数据说这语言没译文就不加载 chunk；没有元数据（旧静态兜底数据）时按回退链尝试加载
    if (doc.hasBody && doc.hasBody[lang] !== true) continue;
    bodies[lang] = await loadDocBodies(lang);
  }
  return pickDocBody(doc, requested, bodies);
}

/**
 * 请求语言是否已有正文（同步读 hasBody 元数据；zh 恒为 true）。
 * 徽标与列表页用它判断，不触发正文 chunk 加载。
 */
export function hasBodyFor(doc: DevDoc, language: string | undefined | null): boolean {
  const lang = docLang(language);
  if (lang === "zh") return true;
  return doc.hasBody?.[lang] === true;
}

/**
 * 请求语言没有正文时，回退链实际会落到的语言（同步、不加载 chunk）；徽标据此判断是否
 * 回退到中文正文（只对中文回退显示既有「正文为中文」）。没有 hasBody 的旧静态兜底数据一律回退中文。
 */
export function fallbackBodyLang(doc: DevDoc, language: string | undefined | null): DocLang {
  for (const lang of bodyLangChain(language)) {
    if (lang === "zh") return "zh";
    if (doc.hasBody?.[lang] === true) return lang;
  }
  return "zh";
}
