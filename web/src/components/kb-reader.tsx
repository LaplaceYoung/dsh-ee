import { createElement, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { computeMarks, docSheets, docView, markdownLines, parseBlocks, sheetBlocks, type Highlight } from "@workbench/kb-doc-render";
import { cx } from "./ui";

// 一次先渲染这么多块，滚到底再加；5 万行的表格不会一下子塞进页面
const FIRST = 240;
const MORE = 400;

/**
 * 知识库文档的阅读视图（跟 DSH 原文窗格是同一套渲染，在 dsh-plugin/src/kb-doc-render.ts）。
 * 高亮的几行会滚到中间；Excel 文档顶上按工作表切换。
 */
export function KBReader({ markdown, format, highlight, jump }: { markdown: string; format?: string; highlight: Highlight | null; jump?: number }) {
  const { t } = useTranslation();
  const blocks = useMemo(() => parseBlocks(markdownLines(markdown)), [markdown]);
  const sheets = useMemo(() => docSheets(blocks, format), [blocks, format]);
  const marks = useMemo(() => computeMarks(blocks, highlight), [blocks, highlight?.from, highlight?.to, highlight?.quote]);
  // 高亮或要跳到的那一行在哪个工作表里
  const target = jump ?? highlight?.from;
  const sheetOf = (line: number | undefined) => {
    if (!line || sheets.length === 0) return sheets[0]?.name;
    let name = sheets[0].name;
    for (const s of sheets) if (s.line <= line) name = s.name;
    return name;
  };
  const [sheet, setSheet] = useState<string | undefined>(() => sheetOf(target));
  useEffect(() => setSheet(sheetOf(target)), [target, sheets]);
  const shown = useMemo(() => sheetBlocks(blocks, sheet), [blocks, sheet]);
  // 要看到的那一行所在的块必须渲染出来
  const needed = useMemo(() => {
    if (!target) return 0;
    const i = shown.findIndex((b) => blockEnd(b) >= target);
    return i < 0 ? 0 : i + 40;
  }, [shown, target]);
  const [limit, setLimit] = useState(FIRST);
  useEffect(() => setLimit(Math.max(FIRST, needed)), [needed, sheet]);
  const sentinel = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const el = sentinel.current;
    if (!el || limit >= shown.length || typeof IntersectionObserver === "undefined") return;
    const io = new IntersectionObserver((entries) => {
      if (entries.some((e) => e.isIntersecting)) setLimit((l) => l + MORE);
    }, { rootMargin: "800px" });
    io.observe(el);
    return () => io.disconnect();
  }, [limit, shown.length]);
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!target) return;
    const el = ref.current?.querySelector(highlight && !jump ? '[data-kb-hit="1"]' : `[data-line="${target}"]`) ?? ref.current?.querySelector(`[data-line="${target}"]`);
    el?.scrollIntoView({ block: highlight && !jump ? "center" : "start" });
  }, [target, highlight?.quote, sheet, limit >= needed, markdown]);

  return (
    <div ref={ref}>
      {sheets.length > 1 && (
        <div role="tablist" aria-label={t("kbReader.sheets")} className="-mx-1 mb-4 flex flex-wrap gap-1 border-b border-neutral-100 pb-3">
          {sheets.map((s) => (
            <button
              key={s.name}
              role="tab"
              aria-selected={s.name === sheet}
              onClick={() => setSheet(s.name)}
              className={cx(
                "rounded-md px-3 py-1 text-sm transition",
                s.name === sheet ? "bg-emerald-50 font-medium text-emerald-800 dark:bg-emerald-500/15 dark:text-emerald-200" : "text-neutral-600 hover:bg-neutral-100 hover:text-neutral-900",
              )}
            >
              {s.name}
            </button>
          ))}
        </div>
      )}
      <div className="text-neutral-800 [--ee-accent:var(--color-brand-500,#4d6bfe)]">
        {docView(createElement, shown.slice(0, limit), marks, { format, highlight })}
      </div>
      {limit < shown.length && (
        <div ref={sentinel} className="py-6 text-center text-xs text-neutral-500">
          <button className="hover:text-neutral-800" onClick={() => setLimit((l) => l + MORE)}>
            {t("kbReader.more", { n: shown.length - limit })}
          </button>
        </div>
      )}
    </div>
  );
}

function blockEnd(b: ReturnType<typeof parseBlocks>[number]): number {
  switch (b.kind) {
    case "heading":
      return b.line.n;
    case "list":
      return b.items[b.items.length - 1].line.n;
    case "table":
      return b.rows.length ? b.rows[b.rows.length - 1].line.n : b.headLine?.n ?? 0;
    default:
      return b.lines[b.lines.length - 1].n;
  }
}
