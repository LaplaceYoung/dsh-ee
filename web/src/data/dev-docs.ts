/**
 * 「开发文档」页的数据入口。
 *
 * 单一事实源是仓库 `docs/`：构建期由 scripts/docs/build.mjs 按
 * scripts/docs/manifest.json 的白名单生成 web/src/generated/dev-docs.ts，
 * 页面优先读生成物；生成物为空（构建环境没有 docs/）时回退到旧的静态副本
 * （./dev-docs-fallback.ts），避免构建失败。
 *
 * 正文是 Markdown，用 react-markdown + remark-gfm + rehype-slug 渲染，
 * 支持 GFM 表格与 GitHub 同款标题锚点；新增一篇的流程见 docs/dev/README.md。
 * 不复制上游正文与图片（政策见 docs/plugin-quality.md 的「映射不复制」）。
 */
import { DOCS_REPO_BLOB, GENERATED_DEV_DOCS } from "../generated/dev-docs";
import { FALLBACK_DEV_DOCS } from "./dev-docs-fallback";
import { selectDevDocs } from "./dev-docs-select";
import { DOC_CATEGORIES, type DevDoc, type DocCategory, type DocLang } from "./dev-docs-types";

export { DOC_CATEGORIES };
export type { DevDoc, DocCategory };

/** 列表与详情页用：优先生成物，生成物为空时用旧静态副本兜底。 */
export const DEV_DOCS: DevDoc[] = selectDevDocs(GENERATED_DEV_DOCS, FALLBACK_DEV_DOCS);

/** 文档在 GitHub 上的源文件地址（仓库地址由 docs/manifest 生成，见 scripts/docs/build.mjs）。 */
export function sourceUrl(sourcePath: string): string {
  return `${DOCS_REPO_BLOB}/docs/${sourcePath}`;
}

/** 按 slug 找文档；找不到返回 null（详情页据此显示空态）。 */
export function findDevDoc(slug: string): DevDoc | null {
  return DEV_DOCS.find((d) => d.slug === slug) ?? null;
}

/** 粗略的阅读时长：按当前语言正文里的字母/数字字符数估算，只用于列表页与详情页提示。 */
export function readMinutes(body: string, lang: DocLang = "zh"): number {
  const plain = body.replace(/```[\s\S]*?```/g, "");
  // 中文按字计（\p{L} 覆盖汉字），其它语言按字符计；\p{M} 覆盖天城文等文字的附加符号
  const chars = Array.from(plain).filter((ch) => /[\p{L}\p{N}\p{M}]/u.test(ch)).length;
  const charsPerMinute = lang === "zh" ? 350 : 900;
  return Math.max(1, Math.round(chars / charsPerMinute));
}
