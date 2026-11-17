/**
 * 开发文档的数据结构。
 *
 * 页面用 `DevDoc`（正文叫 body）；构建期从 docs/ 生成的产物用 `GeneratedDevDoc`
 * （字段与 docs/ 里的 front matter 对齐，正文叫 markdown）。
 * 生成器 scripts/docs/build.mjs 只依赖这里的类型形状，不引第三方依赖。
 */

/** 文档分类：列表页按它展示标签，顺序即导航顺序。 */
export type DocCategory = "start" | "harness" | "plugin" | "extension" | "access" | "publish" | "quality";

/**
 * 元数据的语言键。中文是源语言；英文缺省回退中文；es / pt / hi 缺译回退英文，再回退中文。
 * 运行时列表与取值回退见 web/src/lib/dev-docs-i18n.ts（DOC_LANGS / pickLocalized）。
 */
export type DocLang = "zh" | "en" | "es" | "pt" | "hi";

/**
 * 五语元数据字段（title / summary 的形状）。
 * zh / en 必有；es / pt / hi 可缺——生成物始终写全五键，旧的静态兜底数据只有 zh / en，
 * 页面与搜索统一用 web/src/lib/dev-docs-i18n.ts 的 pickLocalized 取值（缺失回退 en → zh）。
 */
export interface LocalizedText {
  zh: string;
  en: string;
  es?: string;
  pt?: string;
  hi?: string;
}

/** 顺序即列表页分类筛选的展示顺序。 */
export const DOC_CATEGORIES: DocCategory[] = ["start", "harness", "plugin", "extension", "access", "publish", "quality"];

/**
 * 已译正文的语言标记：键是已有译文的语言（en / es / pt / hi），值固定为 true。
 * 正文本身不进元数据：构建期按语言拆到 web/src/generated/dev-docs-bodies.<lang>.ts，
 * 页面用 web/src/lib/dev-docs-i18n.ts 的 loadDocBodies / docBodyForAsync 按需加载。
 * 中文是源语言，正文恒有（在 `DevDoc.body` / `GeneratedDevDoc.markdown`），不在这里标记。
 */
export type DocBodyAvailability = Partial<Record<DocLang, true>>;

/** 站内页面使用的文档结构。 */
export interface DevDoc {
  /** 稳定 slug：详情路由 /docs/:slug */
  slug: string;
  category: DocCategory;
  title: LocalizedText;
  summary: LocalizedText;
  /** 中文正文（Markdown）。docs/ 是唯一事实源，不复制上游正文。 */
  body: string;
  /** 哪些语言有译文正文（只读元数据；正文本身按语言懒加载，见 lib/dev-docs-i18n.ts）。 */
  hasBody?: DocBodyAvailability;
  /** 内容复核日期，展示在列表与详情页；空字符串表示未标注。 */
  updatedAt: string;
  tags: string[];
  /** 仓库内源文件路径（相对 docs/，如 dev/overview.md）；旧静态兜底数据没有时为空。 */
  sourcePath?: string;
}

/** 构建期生成物（web/src/generated/dev-docs.ts）的元素结构。 */
export interface GeneratedDevDoc {
  slug: string;
  category: DocCategory;
  title: LocalizedText;
  summary: LocalizedText;
  tags: string[];
  /** front matter 的 updated_at，YYYY-MM-DD；未标注为空字符串。 */
  updated_at: string;
  /** 仓库内源文件路径（相对 docs/），详情页「查看源文件」用。 */
  source: string;
  /** 中文正文（Markdown），已由生成器去掉 front matter、改写站内链接。 */
  markdown: string;
  /** 哪些语言有译文正文；正文在 dev-docs-bodies.<lang>.ts 里按语言懒加载。 */
  hasBody?: DocBodyAvailability;
}
