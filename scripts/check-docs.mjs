#!/usr/bin/env node
// 文档检查：docs/ 的相对链接与锚点、上游 pin 表、开发文档 front matter。
//
// 用法：
//   node scripts/check-docs.mjs            # 在仓库根运行
//   node scripts/check-docs.mjs --root .   # 指定仓库根
//
// 只依赖 Node 内置模块，不需要安装依赖；CI 的 docs job 也会跑这个脚本。
// 失败时退出码 1，并逐条打印文件、行号和原因。
//
// 由 companion PR（尚未合并）引入的文件用显式白名单处理：
//   - pin 表「本地引用」里路径末尾的 †：跳过存在性检查；
//   - PENDING_LINKS 里的文档链接：目标文件尚不存在时不报错。
// companion PR 合并后应删掉 † 和白名单条目，让 CI 恢复存在性校验。

import { readdirSync, readFileSync, statSync } from 'node:fs';
import { dirname, join, relative, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const HERE = dirname(fileURLToPath(import.meta.url));

function argValue(name) {
  const argv = process.argv.slice(2);
  for (let i = 0; i < argv.length; i++) {
    if (argv[i] === name) return argv[i + 1] ?? '';
    if (argv[i].startsWith(`${name}=`)) return argv[i].slice(name.length + 1);
  }
  return '';
}

const ROOT = resolve(argValue('--root') || join(HERE, '..'));
const DOCS = join(ROOT, 'docs');
const SOURCES = join(DOCS, 'plugin-dev', 'SOURCES.md');
const DEV_DIR = join(DOCS, 'dev');
const NOTICE = join(ROOT, 'NOTICE');

const errors = [];
const stats = {
  markdown: 0,
  relativeLinks: 0,
  anchors: 0,
  externalLinks: 0,
  pinRows: 0,
  pendingRefs: 0,
  pendingLinks: 0,
  adaptedRows: 0,
  frontMatter: 0,
};

const fail = (kind, file, line, message) =>
  errors.push({ kind, file: relative(ROOT, file), line, message });

// ---------- 工具函数 ----------

function walkMarkdown(dir, out = []) {
  let entries;
  try {
    entries = readdirSync(dir, { withFileTypes: true });
  } catch {
    return out;
  }
  for (const entry of entries) {
    if (entry.name === 'node_modules' || entry.name.startsWith('.')) continue;
    const full = join(dir, entry.name);
    if (entry.isDirectory()) walkMarkdown(full, out);
    else if (entry.name.endsWith('.md')) out.push(full);
  }
  return out;
}

// 把代码围栏和行内代码替换成等长空白，保留行号，避免把代码示例里的链接当真。
function stripCode(text) {
  const lines = text.split('\n');
  let inFence = false;
  return lines.map((line) => {
    if (/^\s*(```|~~~)/.test(line)) {
      inFence = !inFence;
      return '';
    }
    if (inFence) return '';
    return line.replace(/`[^`]*`/g, (m) => ' '.repeat(m.length));
  });
}

// GitHub 标题锚点算法（与 GitHub 渲染结果对齐：小写、去标点、空格转连字符、重名加 -1/-2）。
function githubSlug(text) {
  let slug = text.trim().toLowerCase();
  slug = slug.replace(/<[^>]*>/g, '');
  slug = slug.replace(/!\[([^\]]*)\]\([^)]*\)/g, '$1');
  slug = slug.replace(/\[([^\]]*)\]\([^)]*\)/g, '$1');
  slug = slug.replace(/[`*~]/g, '');
  slug = slug.replace(/[^\p{L}\p{N}\p{M}\s_-]/gu, '');
  slug = slug.replace(/\s/g, '-');
  return slug;
}

function headingSlugs(text) {
  const slugs = new Set();
  const counts = new Map();
  const lines = stripCode(text);
  for (const raw of lines) {
    const match = /^\s{0,3}(#{1,6})\s+(.+?)\s*#*\s*$/.exec(raw);
    if (!match) continue;
    const base = githubSlug(match[2]);
    const seen = counts.get(base) ?? 0;
    counts.set(base, seen + 1);
    slugs.add(seen === 0 ? base : `${base}-${seen}`);
  }
  return slugs;
}

function isExternal(dest) {
  return /^[a-z][a-z0-9+.-]*:/i.test(dest) || dest.startsWith('//');
}

function exists(path) {
  try {
    statSync(path);
    return true;
  } catch {
    return false;
  }
}

const LINK_RE = /!?\[[^\]]*\]\(\s*([^()\s]+)(?:\s+["'][^"']*["'])?\s*\)/g;

// 把路径统一成仓库相对的 POSIX 形式（Windows 下 path 用反斜杠）。
function relPosix(path) {
  return relative(ROOT, path).split('\\').join('/');
}

// 由 companion PR 引入、本分支尚不存在的文档链接（文件相对路径 → 链接目标）。
// 对应 PR 合并后删除这里的条目。
const PENDING_LINKS = new Map([
  ['docs/dev/harness.md', new Set(['../harness-integration.md'])], // #117
]);

// ---------- 1. 相对链接 / 锚点 ----------

function checkLinks(file, text) {
  const stripped = stripCode(text);
  for (let i = 0; i < stripped.length; i++) {
    const line = stripped[i];
    let match;
    LINK_RE.lastIndex = 0;
    while ((match = LINK_RE.exec(line)) !== null) {
      let dest = match[1];
      if (isExternal(dest)) {
        stats.externalLinks++;
        continue;
      }
      if (dest.startsWith('/')) {
        // 站内绝对路径不是仓库相对链接（产品路由或域名路径），跳过。
        continue;
      }
      stats.relativeLinks++;
      const hashIndex = dest.indexOf('#');
      const pathPart = (hashIndex === -1 ? dest : dest.slice(0, hashIndex)).split('?')[0];
      const anchor = hashIndex === -1 ? '' : dest.slice(hashIndex + 1);
      let decodedPath;
      try {
        decodedPath = decodeURIComponent(pathPart);
      } catch {
        decodedPath = pathPart;
      }
      let decodedAnchor = '';
      try {
        decodedAnchor = decodeURIComponent(anchor);
      } catch {
        decodedAnchor = anchor;
      }
      const target = decodedPath ? resolve(dirname(file), decodedPath) : file;
      if (!exists(target)) {
        // 目标文件由 companion PR 引入时，本分支先跳过存在性检查。
        if (PENDING_LINKS.get(relPosix(file))?.has(dest)) {
          stats.pendingLinks++;
          continue;
        }
        fail('broken-link', file, i + 1, `链接目标不存在：${dest}`);
        continue;
      }
      if (!decodedAnchor) continue;
      stats.anchors++;
      let stat;
      try {
        stat = statSync(target);
      } catch {
        continue;
      }
      if (!stat.isFile() || !target.endsWith('.md')) {
        fail('bad-anchor', file, i + 1, `锚点只支持 .md 文件：${dest}`);
        continue;
      }
      const slugs = headingSlugs(readFileSync(target, 'utf8'));
      if (!slugs.has(decodedAnchor)) {
        fail('broken-anchor', file, i + 1, `锚点不存在：${dest}`);
      }
    }
  }
}

// ---------- 2. 上游 pin 表 ----------

const PIN_START = '<!-- docs-check:pin-table:start -->';
const PIN_END = '<!-- docs-check:pin-table:end -->';
const PIN_COLUMNS = 9;
const PIN_REQUIRED_REPOS = ['PerryLink/dsh-primer', 'PerryLink/dsh-plugin-guide', 'PerryLink/dsh-standards'];

function parsePinTable(file) {
  if (!exists(file)) {
    fail('missing-sources', file, 0, '缺少 docs/plugin-dev/SOURCES.md（NOTICE 指向该文件）');
    return;
  }
  const lines = readFileSync(file, 'utf8').split('\n');
  const start = lines.findIndex((l) => l.trim() === PIN_START);
  const end = lines.findIndex((l) => l.trim() === PIN_END);
  if (start === -1 || end === -1 || end <= start) {
    fail('pin-table', file, 0, `pin 表缺少标记：${PIN_START} / ${PIN_END}`);
    return;
  }
  const tableLines = [];
  for (let i = start + 1; i < end; i++) {
    if (lines[i].trim().startsWith('|')) tableLines.push({ line: i + 1, text: lines[i].trim() });
  }
  if (tableLines.length < 3) {
    fail('pin-table', file, start + 1, 'pin 表至少要有一行数据（表头 + 分隔行 + 数据行）');
    return;
  }
  const seen = new Map();
  // 跳过表头和分隔行
  for (let rowIndex = 2; rowIndex < tableLines.length; rowIndex++) {
    const { line, text } = tableLines[rowIndex];
    if (/^\|[\s:|-]+\|$/.test(text)) continue; // 容错：多余的分隔行
    const cells = text.replace(/^\|/, '').replace(/\|$/, '').split('|').map((c) => c.trim());
    stats.pinRows++;
    if (cells.length !== PIN_COLUMNS) {
      fail('pin-row', file, line, `pin 行列数应为 ${PIN_COLUMNS}，实际 ${cells.length}：${text.slice(0, 80)}`);
      continue;
    }
    const [name, repo, path, version, commit, license, localRefs, changes, synced] = cells;
    if (!name) fail('pin-row', file, line, '缺少来源名');
    if (!/^[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+$/.test(repo)) fail('pin-row', file, line, `仓库格式应为 owner/repo：${repo}`);
    if (!path) fail('pin-row', file, line, '缺少上游路径');
    if (!/^\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?$/.test(version)) fail('pin-row', file, line, `版本不是 semver：${version}`);
    if (!/^[0-9a-f]{7,40}$/.test(commit)) fail('pin-row', file, line, `commit 应为 7-40 位小写十六进制：${commit}`);
    if (!/(Apache-2\.0|CC-BY-4\.0|MIT|BSD-3-Clause)/.test(license)) fail('pin-row', file, line, `许可列未写可识别的许可：${license}`);
    if (!/^\d{4}-\d{2}-\d{2}$/.test(synced) || Number.isNaN(Date.parse(`${synced}T00:00:00Z`))) {
      fail('pin-row', file, line, `同步日期应为 YYYY-MM-DD：${synced}`);
    }
    if (!changes) fail('pin-row', file, line, '缺少修改说明');
    if (!isPlaceholder(localRefs)) {
      for (const rawRef of localRefs.split(/[、,;；]/).map((r) => r.trim()).filter(Boolean)) {
        // 文件由 companion PR 引入时在路径末尾标 †，本分支先跳过存在性检查（见 SOURCES §3）。
        const pending = rawRef.endsWith('†');
        const ref = pending ? rawRef.slice(0, -1).trim() : rawRef;
        if (!ref) continue;
        if (pending) {
          stats.pendingRefs++;
          continue;
        }
        if (!exists(join(ROOT, ref))) fail('pin-row', file, line, `本地引用不存在：${ref}`);
      }
    }
    const key = `${repo}@${commit}`;
    if (seen.has(key)) fail('pin-row', file, line, `重复的 pin：${key}（另一处在第 ${seen.get(key)} 行）`);
    else seen.set(key, line);
    const repoKey = repo.toLowerCase();
    if (!seen.has(`repo:${repoKey}`)) seen.set(`repo:${repoKey}`, line);
  }
  for (const required of PIN_REQUIRED_REPOS) {
    if (!seen.has(`repo:${required.toLowerCase()}`)) {
      fail('pin-table', file, start + 1, `缺少必需来源：${required}`);
    }
  }
}

function isPlaceholder(value) {
  return value === '' || ['—', '-', '–', '无', 'N/A', 'n/a'].includes(value);
}

// ---------- 3. 已改编内容清单 ----------

const ADAPTED_START = '<!-- docs-check:adapted-table:start -->';
const ADAPTED_END = '<!-- docs-check:adapted-table:end -->';
const ADAPTED_COLUMNS = 5;

function parseAdaptedTable(file) {
  if (!exists(file)) return; // 缺少 SOURCES 已由 pin 表检查报错
  const lines = readFileSync(file, 'utf8').split('\n');
  const start = lines.findIndex((l) => l.trim() === ADAPTED_START);
  const end = lines.findIndex((l) => l.trim() === ADAPTED_END);
  if (start === -1 || end === -1 || end <= start) {
    fail('adapted-table', file, 0, `已改编清单缺少标记：${ADAPTED_START} / ${ADAPTED_END}`);
    return;
  }
  const rows = [];
  for (let i = start + 1; i < end; i++) {
    if (lines[i].trim().startsWith('|')) rows.push({ line: i + 1, text: lines[i].trim() });
  }
  if (rows.length < 3) {
    fail('adapted-table', file, start + 1, '已改编清单至少要有一行数据');
    return;
  }
  const seen = new Map();
  for (let rowIndex = 2; rowIndex < rows.length; rowIndex++) {
    const { line, text } = rows[rowIndex];
    if (/^\|[\s:|-]+\|$/.test(text)) continue;
    const cells = text.replace(/^\|/, '').replace(/\|$/, '').split('|').map((c) => c.trim());
    stats.adaptedRows++;
    if (cells.length !== ADAPTED_COLUMNS) {
      fail('adapted-row', file, line, `改编清单列数应为 ${ADAPTED_COLUMNS}，实际 ${cells.length}：${text.slice(0, 80)}`);
      continue;
    }
    const [localFile, upstream, license, pr, status] = cells;
    if (!localFile.startsWith('docs/plugin-dev/')) fail('adapted-row', file, line, `文件应在 docs/plugin-dev/ 下：${localFile}`);
    if (!upstream) fail('adapted-row', file, line, '缺少上游来源');
    if (!license) fail('adapted-row', file, line, '缺少许可 / 署名');
    if (!/^#\d+$/.test(pr)) fail('adapted-row', file, line, `引入 PR 应为 #NNN：${pr}`);
    if (!/(已改编|已引入|原创索引)/.test(status)) fail('adapted-row', file, line, `状态应标明 已改编 / 已引入 / 原创索引：${status}`);
    if (seen.has(localFile)) fail('adapted-row', file, line, `重复登记：${localFile}（另一处在第 ${seen.get(localFile)} 行）`);
    else seen.set(localFile, line);
  }
}

// ---------- 3. front matter（docs/dev 文档） ----------

const REQUIRED_FRONT_MATTER = ['title', 'status', 'since', 'lang'];
const STATUS_VALUES = ['draft', 'review', 'stable'];
// lang 是文档正文的主语言。站内 /docs 的五语元数据用 title_es / title_pt / title_hi 等字段表达，
// 不靠这个键（见 docs/dev/README.md）；es / pt / hi 预留给将来的正文镜像文档。
const LANG_VALUES = ['zh-CN', 'zh', 'en', 'en-US', 'es', 'pt', 'hi'];

function parseFrontMatter(text) {
  const lines = text.split('\n');
  if ((lines[0] ?? '').trim() !== '---') return null;
  const end = lines.findIndex((line, index) => index > 0 && line.trim() === '---');
  if (end === -1) return null;
  const data = {};
  for (const line of lines.slice(1, end)) {
    const match = /^([A-Za-z_][\w-]*):\s*(.*)$/.exec(line);
    if (match) data[match[1]] = match[2].trim().replace(/^["']|["']$/g, '');
  }
  return data;
}

function checkFrontMatter(files) {
  for (const file of files) {
    const rel = relPosix(file);
    if (dirname(rel).split('\\').join('/') !== 'docs/dev') continue; // 只要求 docs/dev 下的正文
    stats.frontMatter++;
    const text = readFileSync(file, 'utf8');
    const data = parseFrontMatter(text);
    if (!data) {
      fail('front-matter', file, 1, '缺少 front matter（--- 包裹的 title/status/since/lang）');
      continue;
    }
    for (const key of REQUIRED_FRONT_MATTER) {
      if (!data[key]) fail('front-matter', file, 1, `front matter 缺少 ${key}`);
    }
    if (data.status && !STATUS_VALUES.includes(data.status)) fail('front-matter', file, 1, `status 只能是 ${STATUS_VALUES.join(' / ')}`);
    if (data.lang && !LANG_VALUES.includes(data.lang)) fail('front-matter', file, 1, `lang 只能是 ${LANG_VALUES.join(' / ')}`);
    if (data.since && (!/^\d{4}-\d{2}-\d{2}$/.test(data.since) || Number.isNaN(Date.parse(`${data.since}T00:00:00Z`)))) {
      fail('front-matter', file, 1, `since 应为 YYYY-MM-DD：${data.since}`);
    }
  }
}

// docs/dev/README.md 的文档清单必须列全同目录下的正文。
function checkDevIndex(devFiles) {
  const indexFile = join(DEV_DIR, 'README.md');
  if (!exists(indexFile)) {
    fail('dev-index', indexFile, 0, '缺少 docs/dev/README.md 索引');
    return;
  }
  const text = stripCode(readFileSync(indexFile, 'utf8')).join('\n');
  for (const file of devFiles) {
    if (file === indexFile) continue;
    const name = relative(DEV_DIR, file).split('\\').join('/');
    if (!text.includes(`(./${name})`)) fail('dev-index', indexFile, 0, `索引里没有登记 ${name}`);
  }
}

// ---------- 4. NOTICE 与 SOURCES 的指向 ----------

function checkNotice() {
  if (!exists(NOTICE)) {
    fail('missing-notice', NOTICE, 0, '缺少仓库根 NOTICE');
    return;
  }
  const text = readFileSync(NOTICE, 'utf8');
  if (!/Apache-2\.0/.test(text)) fail('notice', NOTICE, 0, 'NOTICE 应声明 Apache-2.0');
  if (!text.includes('docs/plugin-dev/SOURCES.md')) fail('notice', NOTICE, 0, 'NOTICE 应指向 docs/plugin-dev/SOURCES.md');
  if (!/CC BY 4\.0/.test(text)) fail('notice', NOTICE, 0, 'NOTICE 应单独声明 dsh-standards 改编文本的 CC BY 4.0 许可');
  if (!/2026 PerryLink/.test(text)) fail('notice', NOTICE, 0, 'CC BY 4.0 部分应标明 © 2026 PerryLink');
  if (!/contributors/.test(text)) fail('notice', NOTICE, 0, 'Apache-2.0 改编部分应标明上游 contributors 署名');
}

// ---------- 主流程 ----------

function main() {
  if (!exists(DOCS)) {
    console.error(`找不到 docs 目录：${DOCS}`);
    process.exit(2);
  }
  const files = walkMarkdown(DOCS);
  stats.markdown = files.length;
  for (const file of files) checkLinks(file, readFileSync(file, 'utf8'));
  parsePinTable(SOURCES);
  parseAdaptedTable(SOURCES);
  checkNotice();
  const devFiles = files.filter((file) => dirname(relPosix(file)).split('\\').join('/') === 'docs/dev');
  checkFrontMatter(files);
  checkDevIndex(devFiles);

  errors.sort((a, b) => (a.file === b.file ? a.line - b.line : a.file.localeCompare(b.file)));
  if (errors.length) {
    console.error(`文档检查失败：${errors.length} 个问题\n`);
    for (const error of errors) {
      const where = error.line ? `${error.file}:${error.line}` : error.file;
      console.error(`  [${error.kind}] ${where} ${error.message}`);
    }
    console.error(
      `\n统计：markdown ${stats.markdown} 个 · 相对链接 ${stats.relativeLinks}（锚点 ${stats.anchors}，companion 待合并 ${stats.pendingLinks}）· ` +
        `外部链接 ${stats.externalLinks}（跳过）· pin 行 ${stats.pinRows}（待合并引用 ${stats.pendingRefs}）· ` +
        `改编清单 ${stats.adaptedRows} 行 · front matter ${stats.frontMatter}`,
    );
    process.exit(1);
  }
  console.log(
    `文档检查通过：markdown ${stats.markdown} 个 · 相对链接 ${stats.relativeLinks}（锚点 ${stats.anchors}，companion 待合并 ${stats.pendingLinks}）· ` +
      `外部链接 ${stats.externalLinks}（跳过）· pin 行 ${stats.pinRows}（待合并引用 ${stats.pendingRefs}）· ` +
      `改编清单 ${stats.adaptedRows} 行 · front matter ${stats.frontMatter}`,
  );
}

main();
