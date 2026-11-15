// 知识库文档的「阅读视图」：把转换出来的 Markdown 按块渲染成正常排版的文档，网页文档页和 DSH 原文窗格共用。
// 只认我们自己的转换器会写出来的语法（标题、段落、列表、表格、引用块、代码块、粗体、行内代码、链接），
// 认不出来的按普通文字显示。每一行都记着原文行号：悬停显示行号，#L40 能跳过去，引用的几行和摘句能精确高亮。
// 纯函数，createElement 作参数（跟 workbench-view.ts 一样），方便单测。

import { t } from "./i18n/index.js";

type H = (...args: any[]) => any;

export interface DocLine {
  n: number;
  text: string;
}

export type DocBlock =
  | { kind: "heading"; level: number; text: string; line: DocLine }
  | { kind: "para"; lines: DocLine[] }
  | { kind: "list"; ordered: boolean; items: { marker: string; line: DocLine; text: string; indent: number }[] }
  | { kind: "table"; head: string[]; headLine?: DocLine; rows: { cells: string[]; line: DocLine }[] }
  | { kind: "quote"; notice: boolean; lines: DocLine[]; texts: string[] }
  | { kind: "code"; lines: DocLine[] };

/** 文档格式（转换体检里的 format）：决定 PDF 分页线、PPT 每页卡片、Excel 工作表这些特别的样子。 */
export type DocFormat = "pdf" | "docx" | "xlsx" | "csv" | "pptx" | "md" | "txt" | string | undefined;

const tableSep = /^\|(\s*:?-{3,}:?\s*\|)+\s*$/;
const listItem = /^(\s*)([-*+]|\d{1,3}[.)])\s+(.*)$/;
const headingRe = /^(#{1,6})\s+(.*?)\s*#*\s*$/;

/** splitRow 把「| a | b\|c |」拆成格子（「\|」是格子里的竖线）。 */
export function splitRow(line: string): string[] {
  let s = line.trim();
  if (s.startsWith("|")) s = s.slice(1);
  if (s.endsWith("|") && !s.endsWith("\\|")) s = s.slice(0, -1);
  const cells: string[] = [];
  let cur = "";
  for (let i = 0; i < s.length; i++) {
    if (s[i] === "\\" && s[i + 1] === "|") {
      cur += "|";
      i++;
    } else if (s[i] === "|") {
      cells.push(cur.trim());
      cur = "";
    } else cur += s[i];
  }
  cells.push(cur.trim());
  return cells;
}

/** parseBlocks 把带行号的几行切成块；行号可以不连续（DSH 窗格只读了一段）。 */
export function parseBlocks(lines: DocLine[]): DocBlock[] {
  const out: DocBlock[] = [];
  let i = 0;
  const isTableStart = (k: number) => k + 1 < lines.length && lines[k].text.trim().startsWith("|") && tableSep.test(lines[k + 1].text.trim());
  while (i < lines.length) {
    const line = lines[i];
    const t = line.text;
    if (t.trim() === "") {
      i++;
      continue;
    }
    if (t.trim().startsWith("```")) {
      const code: DocLine[] = [line];
      i++;
      while (i < lines.length) {
        code.push(lines[i]);
        if (lines[i++].text.trim().startsWith("```")) break;
      }
      out.push({ kind: "code", lines: code });
      continue;
    }
    const hm = headingRe.exec(t);
    if (hm) {
      out.push({ kind: "heading", level: hm[1].length, text: hm[2], line });
      i++;
      continue;
    }
    if (isTableStart(i)) {
      const head = splitRow(t);
      const rows: { cells: string[]; line: DocLine }[] = [];
      i += 2;
      while (i < lines.length && lines[i].text.trim().startsWith("|")) {
        rows.push({ cells: splitRow(lines[i].text), line: lines[i] });
        i++;
      }
      out.push({ kind: "table", head, headLine: line, rows });
      continue;
    }
    if (t.trim().startsWith("|")) {
      // 没有表头的表格：DSH 窗格只读了一段，开头正好落在表格中间
      const rows: { cells: string[]; line: DocLine }[] = [];
      while (i < lines.length && lines[i].text.trim().startsWith("|") && !isTableStart(i)) {
        if (!tableSep.test(lines[i].text.trim())) rows.push({ cells: splitRow(lines[i].text), line: lines[i] });
        i++;
      }
      out.push({ kind: "table", head: [], rows });
      continue;
    }
    if (t.trimStart().startsWith(">")) {
      const ql: DocLine[] = [];
      const texts: string[] = [];
      while (i < lines.length && lines[i].text.trimStart().startsWith(">")) {
        ql.push(lines[i]);
        texts.push(lines[i].text.trimStart().replace(/^>\s?/, ""));
        i++;
      }
      out.push({ kind: "quote", notice: /^\s*⚠/.test(texts[0] ?? ""), lines: ql, texts });
      continue;
    }
    const lm = listItem.exec(t);
    if (lm) {
      const items: { marker: string; line: DocLine; text: string; indent: number }[] = [];
      const ordered = /\d/.test(lm[2]);
      while (i < lines.length) {
        const m = listItem.exec(lines[i].text);
        if (!m || /\d/.test(m[2]) !== ordered) break;
        items.push({ marker: m[2], line: lines[i], text: m[3], indent: Math.min(3, Math.floor(m[1].replace(/\t/g, "  ").length / 2)) });
        i++;
      }
      out.push({ kind: "list", ordered, items });
      continue;
    }
    const para: DocLine[] = [];
    while (i < lines.length) {
      const x = lines[i].text;
      if (x.trim() === "" || headingRe.test(x) || x.trim().startsWith("```") || x.trimStart().startsWith(">") || listItem.test(x) || isTableStart(i)) break;
      para.push(lines[i]);
      i++;
    }
    if (para.length === 0) {
      // 认不出来的行（例如没有分隔行的「| … |」）：当普通段落
      para.push(line);
      i++;
    }
    out.push({ kind: "para", lines: para });
  }
  return out;
}

/** markdownLines 把整篇正文变成带行号的行。 */
export function markdownLines(markdown: string): DocLine[] {
  return String(markdown ?? "").replace(/\n$/, "").split("\n").map((text, i) => ({ n: i + 1, text }));
}

/** 文档里的工作表（Excel 的一级标题）。 */
export function docSheets(blocks: DocBlock[], format: DocFormat): { name: string; line: number }[] {
  if (format !== "xlsx") return [];
  return blocks.flatMap((b) => (b.kind === "heading" && b.level === 1 ? [{ name: b.text, line: b.line.n }] : []));
}

// ---- 行内：粗体、行内代码、链接 ----

type Seg = { text: string; bold?: boolean; code?: boolean; href?: string };

const inlineRe = /(\*\*[^*]+\*\*|`[^`]+`|\[[^\]]+\]\([^)\s]+\))/g;

export function inlineSegs(text: string): Seg[] {
  const out: Seg[] = [];
  let last = 0;
  for (const m of text.matchAll(inlineRe)) {
    const at = m.index ?? 0;
    if (at > last) out.push({ text: text.slice(last, at) });
    const tok = m[0];
    if (tok.startsWith("**")) out.push({ text: tok.slice(2, -2), bold: true });
    else if (tok.startsWith("`")) out.push({ text: tok.slice(1, -1), code: true });
    else {
      const lm = /^\[([^\]]+)\]\(([^)\s]+)\)$/.exec(tok)!;
      out.push({ text: lm[1], href: safeHref(lm[2]) });
    }
    last = at + tok.length;
  }
  if (last < text.length) out.push({ text: text.slice(last) });
  return out;
}

/** 只放行 http(s)、站内地址和锚点，别的（javascript: 之类）当普通文字。 */
function safeHref(href: string): string | undefined {
  return /^(https?:\/\/|\/|#)/i.test(href) ? href : undefined;
}

export function plainOf(text: string): string {
  return inlineSegs(text).map((s) => s.text).join("");
}

// ---- 引用高亮：摘句精确标出来 ----

/** 每行要标出来的字（[开始, 结束) 是行内纯文字的位置）；whole 表示整行都算命中（表格行、摘句对不上时）。 */
export interface LineMarks {
  ranges: [number, number][];
  whole?: boolean;
}

export interface Highlight {
  from: number;
  to: number;
  quote?: string;
}

/** 一行的纯文字（跟渲染出来的文字一致），用来找摘句。 */
function linePlain(blocks: DocBlock[]): Map<number, { text: string; table: boolean }> {
  const m = new Map<number, { text: string; table: boolean }>();
  for (const b of blocks) {
    switch (b.kind) {
      case "heading":
        m.set(b.line.n, { text: plainOf(b.text), table: false });
        break;
      case "para":
      case "code":
        for (const l of b.lines) m.set(l.n, { text: b.kind === "para" ? plainOf(l.text) : l.text, table: false });
        break;
      case "list":
        for (const it of b.items) m.set(it.line.n, { text: plainOf(it.text), table: false });
        break;
      case "quote":
        b.lines.forEach((l, k) => m.set(l.n, { text: plainOf(b.texts[k]), table: false }));
        break;
      case "table":
        if (b.headLine) m.set(b.headLine.n, { text: b.head.map(plainOf).join(" "), table: true });
        for (const r of b.rows) m.set(r.line.n, { text: r.cells.map(plainOf).join(" "), table: true });
        break;
    }
  }
  return m;
}

const fold = (c: string) => c.toLowerCase().replace(/[！-～]/g, (x) => String.fromCharCode(x.charCodeAt(0) - 0xfee0));
const skippable = (c: string) => /[\s\p{P}\p{S}]/u.test(c);

/**
 * computeMarks 在引用的几行（前后各放宽 2 行，跟服务端核对引用一样）里找摘句，算出每行要标的字。
 * 摘句里的「…」当作省略，分几截按顺序找；比的时候忽略空白、标点、大小写和全角半角。找不到就整段高亮。
 */
export function computeMarks(blocks: DocBlock[], hl: Highlight | null | undefined): Map<number, LineMarks> {
  const out = new Map<number, LineMarks>();
  if (!hl) return out;
  const plain = linePlain(blocks);
  const wholeRange = () => {
    for (const n of plain.keys()) if (n >= hl.from && n <= hl.to) out.set(n, { ranges: [], whole: true });
  };
  const quote = (hl.quote ?? "").replace(/\.\.\./g, "…");
  if (!quote.trim()) {
    wholeRange();
    return out;
  }
  // 把这几行的字连起来（去掉可以忽略的字），记下每个字来自哪一行的第几个字
  const nums = [...plain.keys()].filter((n) => n >= hl.from - 2 && n <= hl.to + 2).sort((a, b) => a - b);
  let hay = "";
  const at: { n: number; i: number }[] = [];
  for (const n of nums) {
    const text = plain.get(n)!.text;
    for (let i = 0; i < text.length; i++) {
      if (skippable(text[i])) continue;
      hay += fold(text[i]);
      at.push({ n, i });
    }
  }
  const parts = quote.split("…").map((p) => [...p].filter((c) => !skippable(c)).map(fold).join("")).filter(Boolean);
  if (parts.join("").length < 2) {
    wholeRange();
    return out;
  }
  let pos = 0;
  const found: [number, number][] = [];
  for (const p of parts) {
    const k = hay.indexOf(p, pos);
    if (k < 0) {
      wholeRange();
      return out;
    }
    found.push([k, k + p.length]);
    pos = k + p.length;
  }
  const hitChars = new Map<number, Set<number>>();
  for (const [s, e] of found) {
    for (let k = s; k < e; k++) {
      const { n, i } = at[k];
      if (!hitChars.has(n)) hitChars.set(n, new Set());
      hitChars.get(n)!.add(i);
    }
  }
  for (const [n, set] of hitChars) {
    const text = plain.get(n)!.text;
    const idx = [...set].sort((a, b) => a - b);
    // 连续的字合成一段；两段之间只隔着空白、标点时也连起来
    const ranges: [number, number][] = [];
    for (const i of idx) {
      const prev = ranges[ranges.length - 1];
      if (prev && [...text.slice(prev[1], i)].every(skippable)) prev[1] = i + 1;
      else ranges.push([i, i + 1]);
    }
    out.set(n, { ranges, whole: plain.get(n)!.table || undefined });
  }
  return out;
}

// ---- 渲染 ----

export const docCss = [
  ".kbr{--kbr-accent:var(--ee-accent,#4d6bfe);--kbr-muted:var(--ee-muted,color-mix(in srgb,currentColor 60%,transparent));--kbr-border:var(--ee-border,color-mix(in srgb,currentColor 10%,transparent));--kbr-soft:var(--ee-soft,color-mix(in srgb,currentColor 4%,transparent));",
  "font-size:14px;line-height:1.75;padding-left:2.6em;position:relative;overflow-wrap:anywhere}",
  ".kbr-ln{position:relative;scroll-margin-top:96px}",
  ".kbr-ln::before{content:attr(data-line);position:absolute;left:-2.9em;width:2.3em;text-align:right;font:11px/inherit ui-monospace,SFMono-Regular,Menlo,monospace;color:var(--kbr-muted);opacity:0;transition:opacity .12s;pointer-events:none;user-select:none}",
  ".kbr-ln:hover::before,.kbr-hit::before{opacity:1}",
  ".kbr-hit{background:color-mix(in srgb,var(--kbr-accent) 7%,transparent);box-shadow:inset 3px 0 0 var(--kbr-accent);border-radius:2px}",
  ".kbr-mark{background:color-mix(in srgb,#facc15 45%,transparent);color:inherit;border-radius:2px;padding:0 1px}",
  ".kbr h1,.kbr h2,.kbr h3,.kbr h4,.kbr h5,.kbr h6{margin:1.4em 0 .5em;line-height:1.4;font-weight:600}",
  ".kbr h1{font-size:1.45em}.kbr h2{font-size:1.2em}.kbr h3{font-size:1.06em}.kbr h4,.kbr h5,.kbr h6{font-size:1em}",
  ".kbr>:first-child{margin-top:0}",
  ".kbr p{margin:0 0 .9em}",
  ".kbr ul,.kbr ol{margin:0 0 .9em;padding-left:1.4em}",
  ".kbr code{font:12.5px ui-monospace,SFMono-Regular,Menlo,monospace;background:var(--kbr-soft);padding:1px 4px;border-radius:4px}",
  ".kbr pre{font:12.5px/1.6 ui-monospace,SFMono-Regular,Menlo,monospace;background:var(--kbr-soft);padding:10px 12px;border-radius:8px;overflow-x:auto;margin:0 0 .9em}",
  ".kbr a{color:var(--kbr-accent);text-decoration:underline;text-underline-offset:2px}",
  ".kbr blockquote{margin:0 0 .9em;padding:2px 12px;border-left:3px solid var(--kbr-border);color:var(--kbr-muted)}",
  ".kbr-notice{margin:0 0 .9em;padding:12px 14px;border:1px dashed color-mix(in srgb,currentColor 22%,transparent);border-radius:10px;background:var(--kbr-soft);color:var(--kbr-muted);font-size:13px}",
  ".kbr-table{overflow-x:auto;margin:0 0 1em;border:1px solid var(--kbr-border);border-radius:8px}",
  ".kbr table{border-collapse:collapse;width:100%;font-size:13px;line-height:1.5}",
  ".kbr th{text-align:left;font-weight:600;background:var(--kbr-soft);white-space:nowrap}",
  ".kbr th,.kbr td{padding:6px 10px;border-bottom:1px solid var(--kbr-border);vertical-align:top}",
  ".kbr tbody tr:nth-child(even) td{background:color-mix(in srgb,currentColor 2%,transparent)}",
  ".kbr tbody tr:last-child td{border-bottom:0}",
  ".kbr td.kbr-num,.kbr th.kbr-num{text-align:right;font-variant-numeric:tabular-nums;white-space:nowrap}",
  ".kbr tr.kbr-ln::before{content:none}",
  ".kbr tr.kbr-hit td{background:color-mix(in srgb,var(--kbr-accent) 9%,transparent)!important}",
  ".kbr tr.kbr-hit td:first-child{box-shadow:inset 3px 0 0 var(--kbr-accent)}",
  ".kbr-page{display:flex;align-items:center;gap:10px;margin:1.6em 0 1em;color:var(--kbr-muted);font-size:12px}",
  ".kbr-page::after{content:'';flex:1;border-top:1px solid var(--kbr-border)}",
  ".kbr-page span{border:1px solid var(--kbr-border);border-radius:999px;padding:1px 9px;background:var(--kbr-soft)}",
  ".kbr>.kbr-page:first-child{margin-top:0}",
  ".kbr-slide{border:1px solid var(--kbr-border);border-radius:12px;padding:14px 18px 6px;margin:0 0 14px;background:var(--ee-card-bg,transparent)}",
  ".kbr-slide-no{font-size:11.5px;color:var(--kbr-muted);margin-bottom:2px}",
  ".kbr-slide h2{margin:0 0 .6em;font-size:1.12em}",
  ".kbr-notes{color:var(--kbr-muted);font-size:13px;border-top:1px dashed var(--kbr-border);padding-top:6px}",
  ".kbr-rows{font-size:12px;font-weight:500;color:var(--kbr-muted);margin:1.2em 0 .4em}",
  ".kbr-sheet{font-size:1.15em;font-weight:600;margin:1.4em 0 .6em;display:flex;align-items:center;gap:8px}",
  ".kbr-sheet::before{content:'';width:10px;height:10px;border-radius:2px;background:#16a34a}",
].join("");

const numberCell = /^[-+]?[¥$€£]?\s*[\d,]+(\.\d+)?\s*%?$/;

/** 一列大部分（八成以上有字的格子）是数字就右对齐。 */
function numericColumns(rows: string[][], width: number): boolean[] {
  return Array.from({ length: width }, (_, j) => {
    let filled = 0;
    let num = 0;
    for (const r of rows) {
      const c = (r[j] ?? "").trim();
      if (!c) continue;
      filled++;
      if (numberCell.test(c)) num++;
    }
    return filled > 0 && num / filled >= 0.8;
  });
}

function inline(h: H, text: string, marks: LineMarks | undefined, keyBase: string) {
  const segs = inlineSegs(text);
  const ranges = marks?.ranges ?? [];
  const out: any[] = [];
  let pos = 0;
  segs.forEach((seg, si) => {
    // 把这一截按标记切开
    const pieces: { text: string; mark: boolean }[] = [];
    let start = 0;
    const end = seg.text.length;
    const cuts = new Set<number>([0, end]);
    for (const [s, e] of ranges) {
      if (e <= pos || s >= pos + end) continue;
      cuts.add(Math.max(0, s - pos));
      cuts.add(Math.min(end, e - pos));
    }
    const sorted = [...cuts].sort((a, b) => a - b);
    for (let k = 0; k + 1 < sorted.length; k++) {
      start = sorted[k];
      const stop = sorted[k + 1];
      if (stop <= start) continue;
      const abs = pos + start;
      pieces.push({ text: seg.text.slice(start, stop), mark: ranges.some(([s, e]) => abs >= s && abs < e) });
    }
    pieces.forEach((p, pi) => {
      const key = `${keyBase}:${si}:${pi}`;
      let node: any = p.text;
      if (seg.code) node = h("code", { key: `${key}c` }, node);
      if (seg.bold) node = h("strong", { key: `${key}b` }, node);
      if (seg.href) node = h("a", { key: `${key}a`, href: seg.href, target: seg.href.startsWith("#") ? undefined : "_blank", rel: "noreferrer" }, node);
      if (p.mark) node = h("mark", { key: `${key}m`, className: "kbr-mark" }, node);
      out.push(typeof node === "string" ? h("span", { key }, node) : node);
    });
    pos += end;
  });
  return out;
}

/** 渲染时的上下文：标记、高亮范围，以及第一处高亮有没有标过（data-kb-hit 只标第一处，用来滚过去）。 */
interface Ctx {
  marks: Map<number, LineMarks>;
  hl: Highlight | null | undefined;
  hitDone: boolean;
}

function lineProps(n: number, ctx: Ctx, extra = "") {
  const hit = (!!ctx.hl && n >= ctx.hl.from && n <= ctx.hl.to) || !!ctx.marks.get(n)?.whole;
  const first = hit && !ctx.hitDone;
  if (first) ctx.hitDone = true;
  return { key: `L${n}`, id: `L${n}`, "data-line": String(n), className: `kbr-ln${hit ? " kbr-hit" : ""}${extra ? " " + extra : ""}`, "data-kb-hit": first ? "1" : undefined };
}

export interface DocViewOptions {
  format?: DocFormat;
  highlight?: Highlight | null;
  /** 只显示这个工作表（Excel）；不填显示全部 */
  sheet?: string;
}

const pageHeading = /^第\s*(\d+)\s*页(?:\s+(.*))?$/;
const rowsHeading = /^第\s*\d+\s*～\s*\d+\s*行$/;

/** blockView 渲染一个块。 */
function blockView(h: H, b: DocBlock, ctx: Ctx, opt: DocViewOptions): any {
  const marks = ctx.marks;
  switch (b.kind) {
    case "heading": {
      const n = b.line.n;
      const pm = pageHeading.exec(b.text);
      if (opt.format === "pdf" && b.level === 2 && pm) {
        return h("div", { ...lineProps(n, ctx, "kbr-page"), role: "separator" }, h("span", null, t("kbDocRender.page", { page: pm[1] })));
      }
      if ((opt.format === "xlsx" || opt.format === "csv") && rowsHeading.test(b.text)) {
        return h("div", lineProps(n, ctx, "kbr-rows"), b.text);
      }
      if (opt.format === "xlsx" && b.level === 1) {
        // 已经按工作表切换了（上面有标签），不用再写一遍名字；留个空锚点，#L 还能跳
        if (opt.sheet) return h("span", lineProps(n, ctx));
        return h("div", lineProps(n, ctx, "kbr-sheet"), inline(h, b.text, marks.get(n), `h${n}`));
      }
      return h(`h${Math.min(6, b.level)}`, lineProps(n, ctx), inline(h, b.text, marks.get(n), `h${n}`));
    }
    case "para": {
      const notes = opt.format === "pptx" && b.lines[0]?.text.startsWith("备注：");
      return h("p", { key: `p${b.lines[0].n}`, className: notes ? "kbr-notes" : undefined },
        ...b.lines.map((l) => h("span", { ...lineProps(l.n, ctx), style: { display: "block" } }, inline(h, l.text, marks.get(l.n), `p${l.n}`))));
    }
    case "list": {
      const tag = b.ordered ? "ol" : "ul";
      const start = b.ordered ? Number.parseInt(b.items[0].marker, 10) : undefined;
      return h(tag, { key: `l${b.items[0].line.n}`, start: start && start !== 1 ? start : undefined },
        ...b.items.map((it) => h("li", { ...lineProps(it.line.n, ctx), style: it.indent ? { marginLeft: `${it.indent * 1.2}em` } : undefined }, inline(h, it.text, marks.get(it.line.n), `li${it.line.n}`))));
    }
    case "quote": {
      if (b.notice) {
        return h("div", { key: `q${b.lines[0].n}`, className: "kbr-notice", role: "note" },
          ...b.lines.map((l, k) => h("span", { ...lineProps(l.n, ctx), style: { display: "block" } }, inline(h, b.texts[k], marks.get(l.n), `q${l.n}`))));
      }
      return h("blockquote", { key: `q${b.lines[0].n}` },
        ...b.lines.map((l, k) => h("span", { ...lineProps(l.n, ctx), style: { display: "block" } }, inline(h, b.texts[k], marks.get(l.n), `q${l.n}`))));
    }
    case "code":
      return h("pre", { key: `c${b.lines[0].n}` }, ...b.lines.map((l) => h("span", { ...lineProps(l.n, ctx), style: { display: "block" } }, l.text || " ")));
    case "table": {
      const width = Math.max(b.head.length, ...b.rows.map((r) => r.cells.length));
      const num = numericColumns(b.rows.map((r) => r.cells), width);
      const cell = (tag: "th" | "td", text: string, j: number, n: number) =>
        h(tag, { key: `${tag}${n}:${j}`, className: num[j] ? "kbr-num" : undefined }, inline(h, text, undefined, `t${n}:${j}`));
      const first = b.headLine?.n ?? b.rows[0]?.line.n ?? 0;
      return h("div", { key: `t${first}`, className: "kbr-table" },
        h("table", null,
          b.headLine ? h("thead", null, h("tr", lineProps(b.headLine.n, ctx), ...Array.from({ length: width }, (_, j) => cell("th", b.head[j] ?? "", j, b.headLine!.n)))) : null,
          h("tbody", null, ...b.rows.map((r) => h("tr", lineProps(r.line.n, ctx), ...Array.from({ length: width }, (_, j) => cell("td", r.cells[j] ?? "", j, r.line.n)))))));
    }
  }
  return null;
}

/** 只留下某个工作表的块（从它的一级标题到下一个一级标题）。 */
export function sheetBlocks(blocks: DocBlock[], sheet: string | undefined): DocBlock[] {
  if (!sheet) return blocks;
  const out: DocBlock[] = [];
  let on = false;
  for (const b of blocks) {
    if (b.kind === "heading" && b.level === 1) on = b.text === sheet;
    if (on) out.push(b);
  }
  return out;
}

/**
 * docView 渲染阅读视图。marks 由调用方用 computeMarks 先算好（整篇文档的块算一次，分批显示时不用重算）。
 * PPT：每页（## 第 N 页 …）收进一张卡片。
 */
export function docView(h: H, blocks: DocBlock[], marks: Map<number, LineMarks>, opt: DocViewOptions = {}) {
  const shown = sheetBlocks(blocks, opt.sheet);
  const ctx: Ctx = { marks, hl: opt.highlight, hitDone: false };
  const children: any[] = [h("style", { key: "css" }, docCss)];
  if (opt.format === "pptx") {
    let slide: any[] | null = null;
    let slideKey = "";
    let slideNo = "";
    const flush = () => {
      if (slide) children.push(h("section", { key: slideKey, className: "kbr-slide", "aria-label": slideNo }, ...slide));
      slide = null;
    };
    shown.forEach((b, i) => {
      const pm = b.kind === "heading" && b.level === 2 ? pageHeading.exec(b.text) : null;
      if (pm) {
        flush();
        slideKey = `s${b.kind === "heading" ? b.line.n : i}`;
        slideNo = t("kbDocRender.page", { page: pm[1] });
        const n = (b as any).line.n as number;
        slide = [
          h("div", { key: "no", className: "kbr-slide-no" }, slideNo),
          pm[2] ? h("h2", lineProps(n, ctx), inline(h, pm[2], marks.get(n), `h${n}`)) : h("span", lineProps(n, ctx)),
        ];
        return;
      }
      const v = blockView(h, b, ctx, opt);
      if (slide) slide.push(v);
      else children.push(v);
    });
    flush();
  } else {
    shown.forEach((b) => children.push(blockView(h, b, ctx, opt)));
  }
  return h("div", { className: "kbr", "data-dsh-ee-ui": "kb-read" }, ...children);
}
