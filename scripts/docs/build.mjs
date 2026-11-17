#!/usr/bin/env node
/**
 * 站内开发文档生成器（D2：docs/ 单一事实源）。
 *
 * 输入：docs/ 下由 scripts/docs/manifest.json 白名单列出的 Markdown（front matter 可选）。
 * 输出：
 *   - web/src/generated/dev-docs.ts：站内页面的元数据 + 中文正文（体积小，进主 bundle）；
 *   - web/src/generated/dev-docs-bodies.<lang>.ts：en / es / pt / hi 各一个「slug → 正文」映射，
 *     页面运行时按语言动态 import()（web/src/lib/dev-docs-i18n.ts），Vite 拆成按需 chunk；
 *   - web/public/llms.txt：给 Agent 的入口（文档清单 + 发现/模型/MCP/错误码等契约链接）。
 * 全部生成物都提交入库；CI 用 --check 校验它们与 docs/ 一致。
 *
 * 只写 meta 的 front matter 子集（title / title_en / title_es / title_pt / title_hi /
 * summary / summary_en / summary_es / summary_pt / summary_hi / category / tags /
 * updated_at / slug），正文原样搬进生成物，不复制任何上游正文或图片。
 * 标题与摘要生成五语（zh / en / es / pt / hi）：缺译按「该语言 → en → zh」回退。
 * 正文也分语言：中文正文来自 docs/<entry>，其它语言来自镜像目录 docs/<lang>/<entry>
 * （如 docs/en/dev/overview.md）。中文在 dev-docs.ts 的 markdown，元数据用 hasBody 标记
 * 哪些语言有译文正文；译文按语言拆到 dev-docs-bodies.<lang>.ts，避免四种语言正文全部
 * 塞进主 bundle。某个语言没有译文文件时不报错（渐进补译，该语言模块为空对象），
 * 页面按「请求语言 → en → zh」回退。
 * 生成时把发布集内的相对 Markdown 链接（./x.md）改写成站内路由 /docs/<slug>，
 * 源文件保持 GitHub 上可读；发布集外的相对链接原样保留。译文正文也跑同一次改写
 * （译文的站内链接已经写成 /docs/<slug>，改写对它是幂等的），除此之外不加工译文。
 *
 * 只用 Node 内置模块，不引第三方依赖。
 *
 * 用法：
 *   node scripts/docs/build.mjs            按 docs/ + manifest 生成/更新生成物
 *   node scripts/docs/build.mjs --check    只校验生成物是否与 docs/ 同步（CI 用）
 *   node scripts/docs/build.mjs --help
 * 可覆盖路径（测试或特殊环境用）：
 *   --docs-dir <dir> --manifest <file> --out <file> --llms-out <file>
 */
import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, join, posix, relative, resolve, sep } from "node:path";
import { fileURLToPath } from "node:url";

const HERE = dirname(fileURLToPath(import.meta.url));
const REPO_ROOT = resolve(HERE, "..", "..");

const DEFAULTS = {
  docsDir: join(REPO_ROOT, "docs"),
  manifest: join(HERE, "manifest.json"),
  out: join(REPO_ROOT, "web", "src", "generated", "dev-docs.ts"),
  llmsOut: join(REPO_ROOT, "web", "public", "llms.txt"),
};

/** manifest 缺 repository 或 docs/ 不存在时，给 Agent 的源文件链接用的默认仓库地址。 */
const DEFAULT_REPOSITORY = "https://github.com/hikariming/dsh-ee";

/** 站内分类；与 web/src/data/dev-docs-types.ts 的 DocCategory 保持一致。 */
export const CATEGORIES = ["start", "harness", "plugin", "extension", "access", "publish", "quality"];

/** 元数据语言；zh 是源语言，en 缺省回退 zh，es/pt/hi 缺译回退 en（与 web/src/data/dev-docs-types.ts 的 DocLang 一致）。 */
export const DOC_LANGS = ["zh", "en", "es", "pt", "hi"];

/** 译文正文的语言目录（docs/<lang>/<entry>）；中文是源语言，正文就在 docs/<entry>，不镜像到 docs/zh/。 */
export const BODY_LANGS = DOC_LANGS.filter((lang) => lang !== "zh");

/** 每语言一个正文模块（与元数据模块同目录）：dev-docs-bodies.<lang>.ts。 */
const BODIES_MODULE_PREFIX = "dev-docs-bodies";

/** 生成器认识的 front matter 字段；其它键（如 status/since/lang）忽略，方便文档共用 front matter。 */
const FRONT_MATTER_KEYS = new Set([
  "slug",
  "title",
  "title_en",
  "title_es",
  "title_pt",
  "title_hi",
  "summary",
  "summary_en",
  "summary_es",
  "summary_pt",
  "summary_hi",
  "category",
  "tags",
  "updated_at",
]);

class DocsBuildError extends Error {}
class UsageError extends Error {}

function json(value) {
  return JSON.stringify(value);
}

/** 相对仓库根显示路径；仓库外的路径（测试临时目录）直接显示绝对路径。 */
function displayPath(file) {
  const rel = relative(REPO_ROOT, file);
  return rel && !rel.startsWith("..") && !rel.includes(`..${sep}`) ? rel : file;
}

/** 每语言的正文模块路径：跟元数据模块同目录（测试用 --out 指到临时目录时也一起过去）。 */
function bodiesModuleFile(outFile, lang) {
  return join(dirname(outFile), `${BODIES_MODULE_PREFIX}.${lang}.ts`);
}

function parseArgs(argv) {
  const opts = {
    check: false,
    help: false,
    docsDir: DEFAULTS.docsDir,
    manifest: DEFAULTS.manifest,
    out: DEFAULTS.out,
    llmsOut: DEFAULTS.llmsOut,
  };
  const args = [...argv];
  while (args.length) {
    const arg = args.shift();
    if (arg === "--check") {
      opts.check = true;
      continue;
    }
    if (arg === "--help" || arg === "-h") {
      opts.help = true;
      continue;
    }
    const match = /^--(docs-dir|manifest|out|llms-out)(?:=(.*))?$/.exec(arg);
    if (match) {
      const value = match[2] ?? args.shift();
      if (!value) throw new UsageError(`--${match[1]} 需要一个路径`);
      opts[{ "docs-dir": "docsDir", manifest: "manifest", out: "out", "llms-out": "llmsOut" }[match[1]]] = value;
      continue;
    }
    throw new UsageError(`未知参数：${arg}`);
  }
  return opts;
}

function usage() {
  return [
    "站内开发文档生成器：docs/ 单一事实源 → web/src/generated/dev-docs*.ts + public/llms.txt",
    "",
    "  node scripts/docs/build.mjs              生成/更新生成物（元数据 + 4 个按语言拆分的正文模块 + llms.txt）",
    "  node scripts/docs/build.mjs --check      只校验生成物是否与 docs/ 同步（CI 用）",
    "  node scripts/docs/build.mjs --help",
    "",
    "可选：--docs-dir <dir> --manifest <file> --out <file> --llms-out <file>",
    "     正文模块写在 --out 所在目录（dev-docs-bodies.<lang>.ts）。",
  ].join("\n");
}

function readManifest(manifestFile) {
  const label = displayPath(manifestFile);
  let raw;
  try {
    raw = readFileSync(manifestFile, "utf8");
  } catch {
    throw new DocsBuildError(`读不到发布白名单 ${label}（可用 --manifest 指定）`);
  }
  let data;
  try {
    data = JSON.parse(raw);
  } catch (error) {
    throw new DocsBuildError(`${label}: JSON 解析失败：${error.message}`);
  }
  if (!data || typeof data !== "object" || Array.isArray(data)) {
    throw new DocsBuildError(`${label}: 顶层必须是对象，形如 {"defaultCategory":"start","docs":["dev/x.md"]}`);
  }
  if (data.defaultCategory !== undefined && !CATEGORIES.includes(data.defaultCategory)) {
    throw new DocsBuildError(`${label}: defaultCategory 必须是 ${CATEGORIES.join(" / ")} 之一`);
  }
  if (!Array.isArray(data.docs)) {
    throw new DocsBuildError(`${label}: docs 必须是字符串数组（相对 docs/ 的路径）`);
  }
  const seen = new Set();
  const entries = data.docs.map((entry, index) => {
    if (typeof entry !== "string" || !entry.trim()) {
      throw new DocsBuildError(`${label}: docs[${index}] 必须是非空字符串`);
    }
    const normalized = entry.trim().replace(/\\/g, "/").replace(/^\.\//, "");
    const parts = normalized.split("/");
    if (normalized.startsWith("/") || parts.includes("..")) {
      throw new DocsBuildError(`${label}: docs[${index}]「${entry}」必须是 docs/ 内的相对路径`);
    }
    if (!normalized.endsWith(".md")) {
      throw new DocsBuildError(`${label}: docs[${index}]「${entry}」不是 .md 文件`);
    }
    if (seen.has(normalized)) {
      throw new DocsBuildError(`${label}: 白名单重复列出 ${normalized}`);
    }
    seen.add(normalized);
    return normalized;
  });
  const repository = validateRepository(data.repository, label);
  const llms = validateLlms(data.llms, label);
  return { entries, defaultCategory: data.defaultCategory ?? CATEGORIES[0], repository, llms };
}

function validateRepository(value, label) {
  if (value === undefined) return DEFAULT_REPOSITORY;
  if (typeof value !== "string" || !/^https?:\/\/[^\s]+$/.test(value)) {
    throw new DocsBuildError(`${label}: repository 必须是 http(s) 地址，例如 ${DEFAULT_REPOSITORY}`);
  }
  return value.replace(/\/+$/, "");
}

/** llms.txt 的配置：标题/摘要可选，契约入口每项是「站内 url」或「仓库 docs 路径」二选一。 */
function validateLlms(value, label) {
  const defaults = {
    title: "dsh-ee 开发文档",
    summary: "dsh-ee 的开发文档入口：站内文档与发现、模型、MCP、错误码等契约。",
    contracts: [],
  };
  if (value === undefined) return defaults;
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    throw new DocsBuildError(`${label}: llms 必须是对象，形如 {"title":"…","contracts":[…]}）`);
  }
  const title = value.title === undefined ? defaults.title : String(value.title).trim();
  if (!title) throw new DocsBuildError(`${label}: llms.title 不能为空`);
  const summary = value.summary === undefined ? defaults.summary : String(value.summary).trim();
  const rawContracts = value.contracts ?? [];
  if (!Array.isArray(rawContracts)) throw new DocsBuildError(`${label}: llms.contracts 必须是数组`);
  const contracts = rawContracts.map((item, index) => {
    if (!item || typeof item !== "object" || Array.isArray(item)) {
      throw new DocsBuildError(`${label}: llms.contracts[${index}] 必须是对象`);
    }
    const contractTitle = String(item.title ?? "").trim();
    if (!contractTitle) throw new DocsBuildError(`${label}: llms.contracts[${index}].title 不能为空`);
    const hasUrl = item.url !== undefined;
    const hasDocs = item.docs !== undefined;
    if (hasUrl === hasDocs) {
      throw new DocsBuildError(`${label}: llms.contracts[${index}] 需要且只能有 url 或 docs 之一`);
    }
    let url = "";
    let docs = "";
    if (hasDocs) {
      docs = String(item.docs).trim().replace(/\\/g, "/").replace(/^\.\//, "");
      if (!docs || docs.startsWith("/") || docs.split("/").includes("..") || !docs.endsWith(".md")) {
        throw new DocsBuildError(`${label}: llms.contracts[${index}].docs「${item.docs}」必须是 docs/ 内的 .md 相对路径`);
      }
    } else {
      url = String(item.url).trim();
      if (!url.startsWith("/") && !/^https?:\/\//.test(url)) {
        throw new DocsBuildError(`${label}: llms.contracts[${index}].url「${item.url}」必须以 / 或 http(s):// 开头`);
      }
    }
    return {
      title: contractTitle,
      description: String(item.description ?? "").trim(),
      url,
      docs,
    };
  });
  return { title, summary, contracts };
}

/** 去掉行尾空白并保证正文以单个换行结束，保证同一输入生成同一字节。 */
function normalizeBody(body) {
  const stripped = body.replace(/^\n+/, "").replace(/\s+$/, "");
  return stripped ? `${stripped}\n` : "";
}

function unquote(value, label, line) {
  const text = value.trim();
  if (text.length >= 2) {
    const first = text[0];
    const last = text[text.length - 1];
    if ((first === '"' || first === "'") && last === first) return text.slice(1, -1);
  }
  if (text.startsWith('"') || text.startsWith("'")) {
    throw new DocsBuildError(`${label}:${line}: 引号没有闭合`);
  }
  return text;
}

function parseTags(value, label, line) {
  let inner = value.trim();
  if (inner.startsWith("[")) {
    if (!inner.endsWith("]")) throw new DocsBuildError(`${label}:${line}: tags 数组缺少结尾 ]`);
    inner = inner.slice(1, -1).trim();
  }
  if (!inner) return [];
  return inner.split(",").map((item) => {
    const tag = unquote(item, label, line);
    if (!tag) throw new DocsBuildError(`${label}:${line}: tags 里有空项`);
    return tag;
  });
}

function parseFrontMatter(rawText, label) {
  const text = rawText.replace(/^\uFEFF/, "").replace(/\r\n?/g, "\n");
  const lines = text.split("\n");
  if (lines[0]?.trim() !== "---") {
    return { data: {}, body: normalizeBody(text) };
  }
  const data = {};
  for (let index = 1; index < lines.length; index += 1) {
    const line = lines[index];
    const lineNo = index + 1;
    if (line.trim() === "---") {
      const body = lines.slice(index + 1).join("\n");
      return { data, body: normalizeBody(body) };
    }
    if (line.trim() === "" || line.trim().startsWith("#")) continue;
    const match = /^([A-Za-z][A-Za-z0-9_]*)\s*:\s*(.*)$/.exec(line);
    if (!match) {
      throw new DocsBuildError(`${label}:${lineNo}: front matter 行格式错误，应为 "键: 值"`);
    }
    const key = match[1];
    if (!FRONT_MATTER_KEYS.has(key)) continue;
    if (Object.hasOwn(data, key)) {
      throw new DocsBuildError(`${label}:${lineNo}: front matter 字段 ${key} 重复`);
    }
    data[key] = key === "tags" ? parseTags(match[2], label, lineNo) : unquote(match[2], label, lineNo);
  }
  throw new DocsBuildError(`${label}: front matter 缺少结束分隔线 ---`);
}

function isIsoDate(value) {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) return false;
  const [year, month, day] = value.split("-").map(Number);
  const date = new Date(Date.UTC(year, month - 1, day));
  return date.getUTCFullYear() === year && date.getUTCMonth() === month - 1 && date.getUTCDate() === day;
}

/**
 * 五语元数据：zh 是源语言、en 缺省回退 zh；es/pt/hi 缺译按「该语言 → en → zh」回退。
 * 生成物里始终写全五键（页面端 pickLocalized 对旧静态兜底数据再兜一层）。
 * @param {string} zh 中文源文本
 * @param {Array<string | undefined>} raw es/pt/hi 的 front matter 原值
 * @param {string | undefined} enRaw 英文值（缺省回退 zh）
 */
function localizedText(zh, enRaw, ...raw) {
  const en = (enRaw ?? "").trim() || zh;
  const pick = (value) => (value ?? "").trim() || en;
  return { zh, en, es: pick(raw[0]), pt: pick(raw[1]), hi: pick(raw[2]) };
}

function stripInlineMarkdown(text) {
  return text
    .replace(/!\[([^\]]*)\]\([^)]*\)/g, "$1")
    .replace(/\[([^\]]*)\]\([^)]*\)/g, "$1")
    .replace(/`([^`]*)`/g, "$1")
    .replace(/\*\*([^*]+)\*\*/g, "$1")
    .replace(/\*([^*]+)\*/g, "$1")
    .trim();
}

function firstHeading(body) {
  for (const line of body.split("\n")) {
    const match = /^#{1,3}\s+(.+?)\s*#*\s*$/.exec(line);
    if (match) return stripInlineMarkdown(match[1]);
  }
  return "";
}

function firstParagraph(body) {
  const lines = body.split("\n");
  const notParagraph = /^(#{1,6}\s|```|>|\||[-*+]\s|\d+\.\s)/;
  for (let index = 0; index < lines.length; index += 1) {
    const line = lines[index].trim();
    if (!line || notParagraph.test(line)) continue;
    const paragraph = [line];
    while (index + 1 < lines.length) {
      const next = lines[index + 1].trim();
      if (!next || notParagraph.test(next)) break;
      paragraph.push(next);
      index += 1;
    }
    return stripInlineMarkdown(paragraph.join(" "));
  }
  return "";
}

/** 把发布集内的相对 .md 链接改写成站内路由；其它链接原样保留。 */
function rewriteLinks(body, entry, docsByEntry) {
  return body.replace(/(!?)\[([^\]]*)\]\(([^)\s]+)((?:\s+"[^"]*")?)\)/g, (whole, bang, text, target, title) => {
    if (bang) return whole;
    if (/^(?:[a-zA-Z][a-zA-Z0-9+.-]*:|\/|#)/.test(target)) return whole;
    const hashIndex = target.indexOf("#");
    const pathPart = hashIndex === -1 ? target : target.slice(0, hashIndex);
    const anchor = hashIndex === -1 ? "" : target.slice(hashIndex);
    if (!pathPart.endsWith(".md")) return whole;
    const resolvedEntry = posix.normalize(posix.join(posix.dirname(entry), pathPart));
    const linked = docsByEntry.get(resolvedEntry);
    if (!linked) return whole;
    return `[${text}](/docs/${linked.slug}${anchor}${title})`;
  });
}

/**
 * 译文正文：docs/<lang>/<entry>（例如白名单 dev/overview.md → docs/en/dev/overview.md）。
 *
 * - 与中文正文一样：可选 front matter 被去掉、行尾与首尾空行规范化，并跑一次与中文正文相同的
 *   站内链接改写（译文的站内链接已写成 /docs/<slug>，改写对它是幂等的）；除此之外不加工译文；
 * - 文件不存在、或正文为空 → 该语言还没译，跳过不报错（渐进补译，页面按回退链取值）；
 * - `entry` 用中文源文档的相对路径，译文里若有 ./x.md 也按同一目录解析。
 * @returns {Partial<Record<"en" | "es" | "pt" | "hi", string>>} 只含实际存在的语言
 */
function loadBodies(docsDir, entry, docsByEntry) {
  const bodies = {};
  for (const lang of BODY_LANGS) {
    const file = resolve(docsDir, lang, entry);
    const inside = relative(docsDir, file);
    if (!inside || inside.startsWith("..") || inside.split(sep).includes("..")) continue;
    if (!existsSync(file)) continue;
    const label = displayPath(file);
    const { body } = parseFrontMatter(readFileSync(file, "utf8"), label);
    if (!body) continue;
    bodies[lang] = rewriteLinks(body, entry, docsByEntry);
  }
  return bodies;
}

function loadDocs(docsDir, manifest) {
  const docs = [];
  const docsByEntry = new Map();
  for (const entry of manifest.entries) {
    const file = resolve(docsDir, entry);
    const inside = relative(docsDir, file);
    if (!inside || inside.startsWith("..") || inside.split(sep).includes("..")) {
      throw new DocsBuildError(`白名单路径越界：${entry}`);
    }
    if (!existsSync(file)) {
      throw new DocsBuildError(`白名单里的文件不存在：${displayPath(file)}（检查 scripts/docs/manifest.json）`);
    }
    const label = displayPath(file);
    const { data, body } = parseFrontMatter(readFileSync(file, "utf8"), label);
    if (!body) throw new DocsBuildError(`${label}: 正文为空`);

    const slug = (data.slug ?? posix.basename(entry, ".md")).trim();
    if (!/^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(slug)) {
      throw new DocsBuildError(`${label}: slug「${slug}」不合法（只允许小写字母、数字与连字符）`);
    }
    const category = (data.category ?? manifest.defaultCategory).trim();
    if (!CATEGORIES.includes(category)) {
      throw new DocsBuildError(`${label}: category「${category}」不在允许值内：${CATEGORIES.join(" / ")}`);
    }
    const title = (data.title ?? firstHeading(body)).trim();
    if (!title) {
      throw new DocsBuildError(`${label}: 缺少标题（front matter 的 title 或正文里的一级标题）`);
    }
    const summary = (data.summary ?? firstParagraph(body)).trim();
    const updatedAt = (data.updated_at ?? "").trim();
    if (updatedAt && !isIsoDate(updatedAt)) {
      throw new DocsBuildError(`${label}: updated_at「${updatedAt}」不是合法的 YYYY-MM-DD`);
    }
    const doc = {
      entry,
      source: entry,
      slug,
      category,
      title: localizedText(title, data.title_en, data.title_es, data.title_pt, data.title_hi),
      summary: localizedText(summary, data.summary_en, data.summary_es, data.summary_pt, data.summary_hi),
      tags: data.tags ?? [],
      updatedAt,
      body,
    };
    if (docsByEntry.has(entry)) throw new DocsBuildError(`白名单重复：${entry}`);
    docsByEntry.set(entry, doc);
    docs.push(doc);
  }
  const bySlug = new Map();
  for (const doc of docs) {
    const existing = bySlug.get(doc.slug);
    if (existing) {
      throw new DocsBuildError(`slug「${doc.slug}」重复：${existing.entry} 与 ${doc.entry}`);
    }
    bySlug.set(doc.slug, doc);
  }
  for (const doc of docs) doc.markdown = rewriteLinks(doc.body, doc.entry, docsByEntry);
  for (const doc of docs) doc.bodies = loadBodies(docsDir, doc.entry, docsByEntry);
  return docs;
}

function renderModule(docs, repository) {
  const lines = [
    "// 本文件由 scripts/docs/build.mjs 生成，请勿手改。",
    "// 单一事实源是仓库的 docs/：站内 /docs 的正文与元数据都从那里生成。",
    "// 发布白名单见 scripts/docs/manifest.json；改文档后运行 `cd web && pnpm docs:build`",
    "// 重新生成并提交，CI 用 `pnpm docs:check` 校验生成物与 docs/ 一致。",
    "// 元数据 + 中文正文（markdown）。en / es / pt / hi 的译文正文拆到同目录的",
    "// dev-docs-bodies.<lang>.ts（按语言懒加载）；hasBody 标记哪些语言有译文。",
    'import type { GeneratedDevDoc } from "../data/dev-docs-types";',
    "",
    `/** 文档源文件的固定链接前缀（详情页「查看源文件」用），来自 manifest.repository。 */`,
    `export const DOCS_REPO_BLOB = ${json(`${repository}/blob/main`)};`,
    "",
  ];
  if (docs.length === 0) {
    lines.push("// 白名单为空：站内页面回退到旧静态数据。");
    lines.push("export const GENERATED_DEV_DOCS: GeneratedDevDoc[] = [];", "");
    return lines.join("\n");
  }
  lines.push(`/** ${docs.length} 篇站内开发文档；顺序与 scripts/docs/manifest.json 的白名单一致。 */`);
  lines.push("export const GENERATED_DEV_DOCS: GeneratedDevDoc[] = [");
  const renderLocalized = (field) => `{ ${DOC_LANGS.map((lang) => `${lang}: ${json(field[lang])}`).join(", ")} }`;
  for (const doc of docs) {
    lines.push("  {");
    lines.push(`    slug: ${json(doc.slug)},`);
    lines.push(`    category: ${json(doc.category)},`);
    lines.push(`    title: ${renderLocalized(doc.title)},`);
    lines.push(`    summary: ${renderLocalized(doc.summary)},`);
    lines.push(`    tags: [${doc.tags.map(json).join(", ")}],`);
    lines.push(`    updated_at: ${json(doc.updatedAt)},`);
    lines.push(`    source: ${json(doc.source)},`);
    lines.push(`    markdown: ${json(doc.markdown)},`);
    // 只写实际存在的译文语言；一个都没有时省掉整行（渐进补译，页面按回退链取值）
    const bodyLangs = BODY_LANGS.filter((lang) => doc.bodies[lang]);
    if (bodyLangs.length) {
      lines.push(`    hasBody: { ${bodyLangs.map((lang) => `${lang}: true`).join(", ")} },`);
    }
    lines.push("  },");
  }
  lines.push("];", "");
  return lines.join("\n");
}

/**
 * 每语言一个译文正文模块：`export default { "<slug>": "<正文>", ... }`。
 * 运行时由 web/src/lib/dev-docs-i18n.ts 用显式 import() 按语言加载，Vite 据此拆 chunk
 * （主 bundle 不再包含 en / es / pt / hi 正文）。
 */
function renderBodiesModule(docs, lang) {
  const entries = docs.filter((doc) => doc.bodies[lang]);
  const lines = [
    "// 本文件由 scripts/docs/build.mjs 生成，请勿手改。",
    `// 译文正文（${lang}）：源是 docs/${lang}/<entry>，键是站内路由用的 ASCII slug。`,
    "// 中文正文不在这里（在 dev-docs.ts 的 markdown 字段）；本模块由 web/src/lib/dev-docs-i18n.ts",
    "// 用显式 import() 按语言懒加载，Vite 会把每种语言拆成独立 chunk。",
  ];
  if (entries.length === 0) {
    lines.push(`// ${lang} 还没有译文（渐进补译）：空对象，页面按回退链取 en / 中文正文。`);
    lines.push("export default {};", "");
    return lines.join("\n");
  }
  lines.push("export default {");
  for (const doc of entries) {
    lines.push(`  ${json(doc.slug)}: ${json(doc.bodies[lang])},`);
  }
  lines.push("};", "");
  return lines.join("\n");
}

/** 生成给 Agent 的 llms.txt：文档清单 + 发现/模型/MCP/错误码等契约入口；引用仓库文档时校验文件存在（断链检查）。 */
function renderLlms(docsDir, manifest, docs) {
  const { title, summary, contracts } = manifest.llms;
  const repository = manifest.repository;
  const lines = [`# ${title}`, ""];
  if (summary) lines.push(`> ${summary}`, "");
  lines.push("## 开发文档（站内 /docs）", "");
  for (const doc of docs) {
    // llms.txt 是给 Agent 的稳定契约：链接形态与「中文 / English」标题行保持不变，避免下游解析回归。
    lines.push(`- [${doc.title.zh} / ${doc.title.en}](/docs/${doc.slug}): ${doc.summary.zh || doc.summary.en}`);
  }
  if (contracts.length) {
    lines.push("", "## 契约与错误入口", "");
    for (const contract of contracts) {
      let url = contract.url;
      if (contract.docs) {
        const file = resolve(docsDir, contract.docs);
        const inside = relative(docsDir, file);
        if (!inside || inside.startsWith("..") || inside.split(sep).includes("..")) {
          throw new DocsBuildError(`llms.contracts「${contract.title}」的 docs 路径越界：${contract.docs}`);
        }
        if (!existsSync(file)) {
          throw new DocsBuildError(
            `llms.contracts「${contract.title}」指向的文档不存在：docs/${contract.docs}（llms.txt 断链检查）`,
          );
        }
        url = `${repository}/blob/main/docs/${contract.docs}`;
      }
      lines.push(`- [${contract.title}](${url})${contract.description ? `: ${contract.description}` : ""}`);
    }
  }
  lines.push(
    "",
    "## 仓库",
    "",
    `- [仓库主页](${repository})`,
    `- [仓库文档目录](${repository}/tree/main/docs)`,
    "",
    "本文件由 scripts/docs/build.mjs 从 docs/ 生成；改文档后运行 `cd web && pnpm docs:build` 重新生成。",
    "",
  );
  return lines.join("\n");
}

function firstDiffLine(current, expected) {
  const currentLines = current.split("\n");
  const expectedLines = expected.split("\n");
  const total = Math.max(currentLines.length, expectedLines.length);
  for (let index = 0; index < total; index += 1) {
    if (currentLines[index] !== expectedLines[index]) return index + 1;
  }
  return total;
}

/** 写一个生成物并打印一行日志（元数据模块 / 每个译文模块共用）。 */
function writeGenerated(file, rendered, message) {
  mkdirSync(dirname(file), { recursive: true });
  writeFileSync(file, rendered);
  console.log(message);
}

/** docs/ 不存在：不报错（无文档环境仍要能构建），生成物保留或写空内容，页面回退旧静态数据。 */
function fallbackWithoutDocs(docsDir, outFile, llmsOutFile, check) {
  const reason = `docs/ 不存在（${displayPath(docsDir)}），跳过生成，站内 /docs 回退旧静态数据`;
  if (check) {
    console.warn(`docs: ${reason}`);
    return;
  }
  if (existsSync(outFile)) {
    console.warn(`docs: ${reason}；保留现有生成物 ${displayPath(outFile)}`);
  } else {
    writeGenerated(outFile, renderModule([], DEFAULT_REPOSITORY), `docs: 已写入空生成物 ${displayPath(outFile)}`);
  }
  // 译文模块必须存在（web/src/lib/dev-docs-i18n.ts 里有显式的 import()），没有就写空对象。
  const missingBodies = BODY_LANGS.filter((lang) => !existsSync(bodiesModuleFile(outFile, lang)));
  for (const lang of missingBodies) {
    writeGenerated(
      bodiesModuleFile(outFile, lang),
      renderBodiesModule([], lang),
      `docs: 已写入空译文模块 ${displayPath(bodiesModuleFile(outFile, lang))}`,
    );
  }
  if (missingBodies.length === 0) {
    console.warn(`docs: ${reason}；保留现有 ${BODY_LANGS.length} 个译文模块`);
  }
  if (existsSync(llmsOutFile)) {
    console.warn(`docs: ${reason}；保留现有 llms.txt ${displayPath(llmsOutFile)}`);
  } else {
    console.warn(`docs: ${reason}；没有文档可生成，跳过 ${displayPath(llmsOutFile)}`);
  }
}

/** 校验一个生成物是否与 docs/ 生成结果一致；不一致时报出文件名与第一处差异行。 */
function checkGenerated(file, expected, name, docsLen) {
  if (!existsSync(file)) {
    throw new DocsBuildError(`生成物不存在：${name}（${displayPath(file)}）；请运行 node scripts/docs/build.mjs`);
  }
  const current = readFileSync(file, "utf8");
  if (current !== expected) {
    throw new DocsBuildError(
      `生成物与 docs/ 不一致：${name}（${displayPath(file)}，第一处差异在第 ${firstDiffLine(current, expected)} 行）；` +
        "请运行 node scripts/docs/build.mjs 重新生成并提交",
    );
  }
  console.log(`docs: 生成物与 docs/ 一致：${name}（${docsLen} 篇）`);
}

function build(opts) {
  const docsDir = resolve(opts.docsDir);
  const outFile = resolve(opts.out);
  const llmsOutFile = resolve(opts.llmsOut);
  if (!existsSync(docsDir)) {
    fallbackWithoutDocs(docsDir, outFile, llmsOutFile, opts.check);
    return;
  }
  const manifest = readManifest(resolve(opts.manifest));
  if (manifest.entries.length === 0) {
    console.warn("docs: scripts/docs/manifest.json 的白名单为空，生成空清单");
  }
  const docs = loadDocs(docsDir, manifest);
  if (docs.length) {
    // 渐进补译：缺哪个语言的正文不报错，但把覆盖率打出来，便于发现漏译。
    const coverage = BODY_LANGS.map((lang) => `${lang} ${docs.filter((doc) => doc.bodies[lang]).length}/${docs.length}`).join(" · ");
    console.log(`docs: 译文正文覆盖 ${coverage}（缺译回退 en → zh，不报错）`);
  }
  const rendered = renderModule(docs, manifest.repository);
  const renderedBodies = new Map(BODY_LANGS.map((lang) => [bodiesModuleFile(outFile, lang), renderBodiesModule(docs, lang)]));
  const llms = renderLlms(docsDir, manifest, docs);
  if (opts.check) {
    checkGenerated(outFile, rendered, "dev-docs.ts", docs.length);
    for (const lang of BODY_LANGS) {
      const file = bodiesModuleFile(outFile, lang);
      checkGenerated(file, renderedBodies.get(file), `${BODIES_MODULE_PREFIX}.${lang}.ts`, docs.filter((doc) => doc.bodies[lang]).length);
    }
    checkGenerated(llmsOutFile, llms, "llms.txt", docs.length);
    return;
  }
  writeGenerated(outFile, rendered, `docs: ${docs.length} 篇元数据 + 中文正文 → ${displayPath(outFile)}`);
  for (const lang of BODY_LANGS) {
    const file = bodiesModuleFile(outFile, lang);
    const count = docs.filter((doc) => doc.bodies[lang]).length;
    writeGenerated(file, renderedBodies.get(file), `docs: 译文正文 ${lang} ${count}/${docs.length} 篇 → ${displayPath(file)}`);
  }
  mkdirSync(dirname(llmsOutFile), { recursive: true });
  writeFileSync(llmsOutFile, llms);
  console.log(`docs: ${docs.length} 篇 + 契约入口 → ${displayPath(llmsOutFile)}（llms.txt）`);
}

function main(argv) {
  let opts;
  try {
    opts = parseArgs(argv);
  } catch (error) {
    if (error instanceof UsageError) {
      console.error(`docs: ${error.message}\n\n${usage()}`);
      return 2;
    }
    throw error;
  }
  if (opts.help) {
    console.log(usage());
    return 0;
  }
  try {
    build(opts);
    return 0;
  } catch (error) {
    if (error instanceof DocsBuildError) {
      console.error(`docs: ${error.message}`);
      return 1;
    }
    throw error;
  }
}

const exitCode = main(process.argv.slice(2));
if (exitCode !== 0) process.exitCode = exitCode;
