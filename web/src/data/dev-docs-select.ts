/**
 * 从生成物里选站内开发文档：优先读 `docs/`（见 scripts/docs/build.mjs）的生成结果，
 * 生成物为空（例如构建环境里没有 docs/）时才回退到旧静态副本。
 *
 * 这里保持纯函数、不 import 任何正文，方便 Node 直接测（scripts/docs/build.test.mjs）。
 */
import type { DevDoc, GeneratedDevDoc } from "./dev-docs-types";

/** 生成物的字段名与页面结构不同（snake_case / markdown → camelCase / body），在这里做一次映射。 */
export function fromGeneratedDoc(doc: GeneratedDevDoc): DevDoc {
  return {
    slug: doc.slug,
    category: doc.category,
    title: doc.title,
    summary: doc.summary,
    body: doc.markdown,
    // 译文正文不在生成物里（按语言懒加载），只带「哪些语言有译文」的同步标记；
    // 没有译文时不加这个键（旧静态兜底数据也没有），页面统一按回退链取正文
    ...(doc.hasBody ? { hasBody: doc.hasBody } : {}),
    updatedAt: doc.updated_at,
    tags: doc.tags,
    sourcePath: doc.source,
  };
}

/** 生成物非空时用生成物，否则用旧静态数据兜底；两者都按白名单顺序输出。 */
export function selectDevDocs(generated: GeneratedDevDoc[], fallback: DevDoc[]): DevDoc[] {
  return generated.length > 0 ? generated.map(fromGeneratedDoc) : fallback;
}

/** 当前页面内容来自生成物还是旧静态兜底；用于调试与测试断言。 */
export function devDocsSource(generated: GeneratedDevDoc[]): "generated" | "fallback" {
  return generated.length > 0 ? "generated" : "fallback";
}
