import { createElement, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { api } from "../lib/api";
import { copyText } from "../lib/clipboard";
import { fillPrompt, panelView, rowWriteOf, type PanelHandlers, type WidgetState, type WorkbenchAction, type WorkbenchPane, type WorkbenchWidget } from "../lib/workbench-render";

/**
 * 一个业务面板：取数、表单、清单、到 DSH 里继续。网页版应用（M14）和编辑器的实时预览（M5）共用，
 * 两边和 DSH 右侧栏是同一套渲染（workbench-view.ts）。
 *
 * - web：表单提交到自带的表时直接保存；发给 AI 的按钮把那段话复制下来，到 DSH 里继续；清单勾选存在本机浏览器。
 * - preview：不写数据也不发给 AI，只提示「在 DSH 里会发送什么」；清单勾选只在这次预览里有效。
 */

export type SourceResult = { value?: unknown; forbidden?: boolean; error?: string; note?: string };
export type LoadSource = (paneId: string, index: number, widget: WorkbenchWidget) => Promise<SourceResult>;

const checkKey = (slug: string, pane: string, index: number) => `dsh-ee:checklist:${slug}:${pane}:${index}`;

function readChecks(slug: string, pane: string, index: number): string[] {
  try {
    const v = JSON.parse(localStorage.getItem(checkKey(slug, pane, index)) || "[]");
    return Array.isArray(v) ? v.map(String) : [];
  } catch {
    return [];
  }
}

export function LivePane({
  slug,
  pane: raw,
  load: loadSource,
  mode,
  embed = false,
  requestHref,
}: {
  slug: string;
  pane: WorkbenchPane;
  load: LoadSource;
  mode: "web" | "preview";
  /** 同站 iframe 嵌入模式：隐藏「打开 DSH」这类会跳出宿主上下文的入口。 */
  embed?: boolean;
  requestHref?: (connection: string) => string | undefined;
}) {
  const { t } = useTranslation();
  // AI 看板只在 DSH 里有内容：换成一句说明，组件的位置不变（取数按位置）
  const pane = useMemo<WorkbenchPane>(() => ({
    ...raw,
    widgets: (raw.widgets ?? []).map((w) => (w.type === "board" ? { type: "markdown", title: w.title || t("appWeb.board"), text: t("appWeb.boardNote"), width: w.width } : w)),
  }), [raw, t]);
  const [data, setData] = useState<Record<number, WidgetState>>({});
  const [forms, setForms] = useState<Record<number, Record<string, unknown>>>({});
  const [formErrors, setFormErrors] = useState<Record<number, string>>({});
  const [checks, setChecks] = useState<Record<number, string[]>>({});
  const [expanded, setExpanded] = useState<Record<number, number | undefined>>({});
  const [selectedKBs, setSelectedKBs] = useState<string[]>([]);
  const [sending, setSending] = useState<Set<string>>(new Set());
  // 删除行的按钮点过一次、等再点一次确认
  const [armed, setArmed] = useState("");
  const [notice, setNotice] = useState("");
  const [handoff, setHandoff] = useState("");
  const live = useRef(true);
  useEffect(() => {
    live.current = true;
    return () => { live.current = false; };
  }, []);

  const load = async (index: number) => {
    const widget = pane.widgets?.[index];
    if (!widget) return;
    setData((d) => ({ ...d, [index]: d[index]?.status === "ready" && mode === "preview" ? d[index] : { status: "loading" } }));
    try {
      const r = await loadSource(pane.id, index, widget);
      const state: WidgetState = r.forbidden ? { status: "forbidden" } : r.error ? { status: "error", error: r.error } : { status: "ready", value: r.value };
      if (live.current) setData((d) => ({ ...d, [index]: state }));
    } catch (e) {
      if (live.current) setData((d) => ({ ...d, [index]: { status: "error", error: e instanceof Error ? e.message : String(e) } }));
    }
  };
  // 预览里改了配置要重新取数：按数据源的内容判断，只改标题、列名不会重新请求（预览的 load 自己有缓存）
  const sourcesKey = JSON.stringify((pane.widgets ?? []).map((w) => w.source ?? null));
  useEffect(() => {
    (pane.widgets ?? []).forEach((w, i) => {
      if (w.source) void load(i);
      if (w.type === "checklist" && mode === "web") setChecks((c) => ({ ...c, [i]: readChecks(slug, pane.id, i) }));
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [slug, pane.id, sourcesKey]);

  const busy = (key: string, on: boolean) => setSending((s) => {
    const next = new Set(s);
    if (on) next.add(key);
    else next.delete(key);
    return next;
  });

  // 网页上没有 AI：把这段话（带上 /应用名）复制下来，到 DSH 里粘贴发送
  const continueInDSH = async (text: string) => {
    const full = `/${slug} ${text}`;
    if (mode === "preview") {
      setNotice(t("studio.previewSend"));
      setHandoff(text);
      return;
    }
    const copied = await copyText(full);
    setNotice(t(copied ? "appWeb.copied" : "appWeb.copyFailed"));
    setHandoff(full);
  };

  const scopeIndex = (pane.widgets ?? []).findIndex((w) => w.type === "kb_scope");
  const scopeState = scopeIndex >= 0 ? data[scopeIndex] : undefined;
  const kbsText = () => {
    const items = scopeState?.status === "ready" && Array.isArray((scopeState.value as { items?: unknown })?.items) ? (scopeState.value as { items: { kb: string; title: string }[] }).items : [];
    const picked = items.filter((x) => selectedKBs.includes(x.kb));
    return picked.length ? picked.map((x) => t("studio.kbItem", { title: x.title })).join(t("studio.kbSeparator")) : t("studio.allKnowledgeBases");
  };

  const send = (key: string, action: WorkbenchAction, values: { row?: Record<string, unknown>; form?: Record<string, unknown> }) => {
    const index = Number(key.split(":")[0]);
    if (action.table && values.form) {
      if (mode === "preview") {
        setNotice(t("studio.previewInsert", { table: action.table }));
        setHandoff("");
        return;
      }
      if (sending.has(key)) return;
      busy(key, true);
      const widget = pane.widgets?.[index];
      const allowed = new Set((widget?.fields ?? []).map((f) => f.name));
      const row: Record<string, unknown> = {};
      for (const [k, v] of Object.entries(values.form)) if (allowed.has(k) && v !== "" && v !== undefined) row[k] = v;
      api.appInsertRow(slug, action.table, row)
        .then(() => {
          setForms((f) => ({ ...f, [index]: {} }));
          setFormErrors((e) => ({ ...e, [index]: "" }));
          setNotice(t("appWeb.saved", { label: action.label }));
          setHandoff("");
          (pane.widgets ?? []).forEach((w, i) => { if (w.source?.table === action.table) void load(i); });
          if (action.prompt) void continueInDSH(fillPrompt(action.prompt, { form: values.form }));
        })
        .catch((e: unknown) => setFormErrors((x) => ({ ...x, [index]: e instanceof Error ? e.message : String(e) })))
        .finally(() => busy(key, false));
      return;
    }
    // 表格的行按钮写表：改状态或删除这一行，完了刷新读这张表的组件
    const write = values.row ? rowWriteOf(action) : undefined;
    if (write && values.row) {
      if (mode === "preview") {
        setNotice(t("studio.previewInsert", { table: action.table }));
        setHandoff("");
        return;
      }
      if (sending.has(key)) return;
      if (write === "delete" && armed !== key) {
        setArmed(key);
        setTimeout(() => setArmed((a) => (a === key ? "" : a)), 4000);
        return;
      }
      setArmed("");
      const row = values.row;
      busy(key, true);
      const done = write === "delete" ? api.appDeleteRow(slug, action.table!, Number(row.id)) : api.appUpdateRow(slug, action.table!, Number(row.id), action.set ?? {});
      Promise.resolve(done)
        .then(() => {
          setNotice(write === "delete" ? t("appWeb.deleted") : t("appWeb.updated", { label: action.label }));
          setHandoff("");
          (pane.widgets ?? []).forEach((w, i) => { if (w.source?.table === action.table) void load(i); });
          if (action.prompt) void continueInDSH(fillPrompt(action.prompt, { row }));
        })
        .catch((e: unknown) => setNotice(e instanceof Error ? e.message : String(e)))
        .finally(() => busy(key, false));
      return;
    }
    void continueInDSH(fillPrompt(action.prompt, { ...values, scope: { kbs: kbsText() } }));
    if (values.form) setForms((f) => ({ ...f, [index]: {} }));
  };

  const handlers: PanelHandlers = {
    sending,
    send,
    refresh: (i) => void load(i),
    formValues: (i) => forms[i] ?? {},
    setFormValue: (i, name, value) => setForms((f) => ({ ...f, [i]: { ...(f[i] ?? {}), [name]: value } })),
    formError: (i) => formErrors[i] ?? "",
    setFormError: (i, error) => setFormErrors((e) => ({ ...e, [i]: error })),
    selectedKBs,
    toggleKB: (kb) => setSelectedKBs((s) => (s.includes(kb) ? s.filter((x) => x !== kb) : [...s, kb])),
    checked: (i) => checks[i] ?? [],
    toggleCheck: (i, id) => setChecks((c) => {
      const cur = c[i] ?? [];
      const next = cur.includes(id) ? cur.filter((x) => x !== id) : [...cur, id];
      if (mode === "web") {
        try {
          localStorage.setItem(checkKey(slug, pane.id, i), JSON.stringify(next));
        } catch {
          // 存不下就只在这次打开里有效
        }
      }
      return { ...c, [i]: next };
    }),
    expandedRow: (i) => expanded[i],
    toggleRow: (i, row) => setExpanded((e) => ({ ...e, [i]: e[i] === row ? undefined : row })),
    requestHref,
    viaDSH: true,
    armed: (key) => armed === key,
  };

  return (
    <div data-app-web={mode === "web" ? "" : undefined} data-live-preview={mode === "preview" ? "" : undefined} className="space-y-3">
      {notice && (
        <div role="status" className="mx-3.5 rounded-md border border-brand-200/80 bg-brand-50/70 px-3.5 py-2.5 text-sm leading-6 text-neutral-800 dark:border-brand-400/25 dark:bg-brand-500/10">
          {notice}
          {handoff && (
            <>
              <code className="mt-1 block overflow-x-auto rounded bg-card px-2 py-1 font-mono text-xs whitespace-pre-wrap">{handoff}</code>
              {mode === "web" && !embed && <a href="dsh://open" className="mt-1 inline-block font-medium text-brand-600 hover:underline dark:text-brand-300">{t("appWeb.openDSH")}</a>}
            </>
          )}
        </div>
      )}
      {/* 卡片底色：浅色主题是白卡片配浅灰页面（Stripe 式），深色跟随主题的卡片色 */}
      <div className="text-neutral-900 [--ee-card-bg:#fff] dark:[--ee-card-bg:var(--card)]">{panelView(createElement, pane, { data }, handlers)}</div>
    </div>
  );
}
