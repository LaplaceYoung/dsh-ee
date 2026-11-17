/**
 * 站内开发文档的全文搜索（标题 + 摘要 + 标签 + 正文）。
 *
 * 正文不再随元数据进主 bundle：构建期按语言拆到 web/src/generated/dev-docs-bodies.<lang>.ts，
 * 由 web/src/lib/dev-docs-i18n.ts 的 loadDocBodies 懒加载。这里维护一份「已加载的译文正文」缓存
 * （lang → slug → Markdown）：列表页与 ⌘K 打开时调用 primeBodies(lang) 预热，预热完成后触发重算，
 * 之后英文 / es / pt / hi 正文也能搜到；尚未预热（或该语言没有正文）时正文按中文兜底，
 * 保证中文搜索行为与以前完全一致。
 *
 * 匹配方式：Markdown 转纯文本后做「子串 + 多词全部命中」——中文两字词直接命中，英文按单词子串命中。
 * 纯函数、无依赖；索引按「文档 + 语言」懒构建（首次搜索时才算，并缓存在模块内），
 * /docs 页过滤与 ⌘K 的「文档」分组共用这一套逻辑。
 */
import type { DevDoc } from "./dev-docs-types";
// 显式 .ts 后缀：本模块被 scripts/docs/build.test.mjs 用 Node 直接 import，
// 而 Node 不解析无扩展名的相对导入（Vite/tsc 两者都能处理，见 web/tsconfig.json 的 allowImportingTsExtensions）。
import { docLang, loadDocBodies, pickLocalized, type BodyLang } from "../lib/dev-docs-i18n.ts";

export interface DevDocHit {
  doc: DevDoc;
  /** 命中分：标题 > 摘要 > 标签/正文，用于排序。 */
  score: number;
  /** 正文里第一处命中的一小段（没有正文命中时用摘要）。 */
  snippet: string;
}

/** 去掉 Markdown 语法，保留可读正文（代码块内容也保留，方便搜 `ee_at_` 这类词）。 */
export function plainText(markdown: string): string {
  return markdown
    .replace(/```[^\n]*\n?/g, " ")
    .replace(/~~~[^\n]*\n?/g, " ")
    .replace(/!\[([^\]]*)\]\([^)]*\)/g, "$1")
    .replace(/\[([^\]]*)\]\([^)]*\)/g, "$1")
    .replace(/<[^>]+>/g, " ")
    .replace(/^\s{0,3}#{1,6}\s+/gm, "")
    .replace(/^\s{0,3}>\s?/gm, "")
    .replace(/[`*_~]/g, "")
    .replace(/\|/g, " ")
    .replace(/\s+/g, " ")
    .trim();
}

const textCache = new Map<string, string>();

/** 已加载的译文正文（只含非中文；primeBodies / 测试注入后才有值）。 */
const loadedBodies = new Map<BodyLang, Record<string, string>>();

/**
 * 注入某语言的译文正文缓存（primeBodies 内部用；测试用它模拟「正文已加载」）。
 * 注入后清空纯文本缓存，让 `slug:lang` 索引按新正文重建。
 */
export function registerDocBodies(language: string, bodies: Record<string, string>): void {
  const lang = docLang(language);
  if (lang === "zh") return;
  loadedBodies.set(lang, bodies);
  textCache.clear();
}

/** 某语言的译文正文是否已经加载（zh 是源语言，恒为 true）；页面据此决定是否要预热。 */
export function hasLoadedDocBodies(language: string | undefined | null): boolean {
  const lang = docLang(language);
  return lang === "zh" || loadedBodies.has(lang);
}

/**
 * 预热某语言的正文：/docs 列表页与 ⌘K 打开时调用，await 之后该语言的正文可被搜索命中。
 * zh 不需要加载（正文在元数据里），返回 true；加载失败返回 false（搜索退回中文正文，不白屏）。
 */
export async function primeBodies(language: string | undefined | null): Promise<boolean> {
  const lang = docLang(language);
  if (lang === "zh" || loadedBodies.has(lang)) return true;
  try {
    registerDocBodies(lang, await loadDocBodies(lang));
    return true;
  } catch {
    return false;
  }
}

/**
 * 正文纯文本（懒构建并缓存）。正文按界面语言取：优先用显式传入 / 已预热的该语言正文，
 * 其次已加载的英文正文，最后回退中文正文（与详情页「请求语言 → en → zh」的语义一致；
 * 区别只是未加载的语言不会在这里触发 chunk 下载）。缓存键是 `slug:lang`，不同语言互不覆盖。
 */
export function docBodyText(doc: DevDoc, lang: string = "zh", bodies?: Record<string, string>): string {
  const language = docLang(lang);
  const key = `${doc.slug}:${language}`;
  const cached = textCache.get(key);
  if (cached !== undefined) return cached;
  let markdown: string;
  if (language === "zh") {
    markdown = doc.body;
  } else {
    const requested = bodies ?? loadedBodies.get(language);
    markdown = requested?.[doc.slug] ?? loadedBodies.get("en")?.[doc.slug] ?? doc.body;
  }
  const text = plainText(markdown);
  textCache.set(key, text);
  return text;
}

/** 搜索用的小写 haystack：当前语言的标题、摘要 + 标签 + 正文（未加载时正文按中文兜底）。 */
function haystack(doc: DevDoc, lang: string): { title: string; summary: string; tags: string; body: string } {
  return {
    title: pickLocalized(doc.title, lang).toLowerCase(),
    summary: pickLocalized(doc.summary, lang).toLowerCase(),
    tags: doc.tags.join(" ").toLowerCase(),
    body: docBodyText(doc, lang).toLowerCase(),
  };
}

function countOccurrences(haystackText: string, needle: string): number {
  if (!needle) return 0;
  let count = 0;
  let index = haystackText.indexOf(needle);
  while (index !== -1 && count < 5) {
    count += 1;
    index = haystackText.indexOf(needle, index + needle.length);
  }
  return count;
}

/**
 * 是否命中：查询按空白拆成多个词，**每个词都要出现**（中文两字词直接子串匹配）。
 * 空查询返回 true（列表页显示全部）。lang 是界面语言，决定标题/摘要/正文用哪一语的字段。
 */
export function matchesDevDoc(doc: DevDoc, query: string, lang: string = "zh"): boolean {
  const words = query.trim().toLowerCase().split(/\s+/).filter(Boolean);
  if (!words.length) return true;
  const hay = haystack(doc, lang);
  return words.every((word) => hay.title.includes(word) || hay.summary.includes(word) || hay.tags.includes(word) || hay.body.includes(word));
}

/** 正文命中的上下文片段；找不到正文命中时退回当前语言的摘要。 */
function bodySnippet(doc: DevDoc, word: string, lang: string): string {
  const fallback = pickLocalized(doc.summary, lang);
  if (!word) return fallback;
  const body = docBodyText(doc, lang);
  const index = body.toLowerCase().indexOf(word);
  if (index === -1) return fallback;
  const start = Math.max(0, index - 24);
  const end = Math.min(body.length, index + word.length + 40);
  const prefix = start > 0 ? "…" : "";
  const suffix = end < body.length ? "…" : "";
  return `${prefix}${body.slice(start, end).trim()}${suffix}`;
}

/** 按命中质量排序的搜索结果；limit 默认 6（⌘K 分组用），lang 是界面语言。 */
export function searchDevDocs(docs: DevDoc[], query: string, limit = 6, lang: string = "zh"): DevDocHit[] {
  const words = query.trim().toLowerCase().split(/\s+/).filter(Boolean);
  if (!words.length) return [];
  const hits: DevDocHit[] = [];
  docs.forEach((doc, order) => {
    const hay = haystack(doc, lang);
    let score = 0;
    for (const word of words) {
      const inTitle = hay.title.includes(word);
      const inSummary = hay.summary.includes(word);
      const inTags = hay.tags.includes(word);
      const bodyHits = countOccurrences(hay.body, word);
      if (!inTitle && !inSummary && !inTags && bodyHits === 0) return;
      if (inTitle) score += 6;
      if (inSummary) score += 4;
      if (inTags) score += 2;
      score += Math.min(bodyHits, 3);
    }
    if (!score) return;
    // 同分按白名单顺序（命中多的文档自然排在前面）
    hits.push({ doc, score: score * 1000 - order, snippet: bodySnippet(doc, words[0], lang) });
  });
  return hits.sort((a, b) => b.score - a.score).slice(0, limit);
}
