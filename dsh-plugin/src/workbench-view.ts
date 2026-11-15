// 业务工作台的界面：纯函数，createElement 作参数（照 status-view.ts），方便单测。
// 颜色都从 currentColor 混出来，跟随 DSH 的亮色 / 暗色主题；只有图表序列用固定的几种颜色。
import { computeMarks, docView, parseBlocks } from "./kb-doc-render.js";
import { localeTag, t } from "./i18n/index.js";
import { chartSeries, checkForm, checklistItems, formatDelta, formatValue, getPath, parseDocLines, rowWriteOf, statusTone, tableRows, type Board, type Citation, type KBaseItem, type WorkbenchAction, type WorkbenchPane, type WorkbenchWidget, type WidgetWidth } from "./workbench.js";

type H = (...args: any[]) => any;

/** 一个组件的数据状态；没有数据源的组件不用。 */
export type WidgetState =
  | { status: "loading" }
  | { status: "ready"; value: any }
  | { status: "forbidden" }
  | { status: "error"; error: string };

export interface PanelHandlers {
  /** 正在发送的按钮键；发送返回前同一个按钮不能再点 */
  sending: Set<string>;
  send: (key: string, action: WorkbenchAction, data: { row?: any; form?: Record<string, any> }) => void;
  refresh: (index: number) => void;
  formValues: (index: number) => Record<string, any>;
  setFormValue: (index: number, name: string, value: any) => void;
  formError: (index: number) => string;
  setFormError: (index: number, error: string) => void;
  /** 「查哪些库」里勾选的知识库标识 */
  selectedKBs?: string[];
  toggleKB?: (kb: string) => void;
  /** 点看板上的引用：在「原文」窗格打开 */
  openCitation?: (citation: Citation) => void;
  citationHref?: (citation: Citation) => string | undefined;
  /** 清单：勾选了哪些条目、勾选 / 取消 */
  checked?: (index: number) => string[];
  toggleCheck?: (index: number, id: string) => void;
  /** 表格：展开了哪一行的详情卡 */
  expandedRow?: (index: number) => number | undefined;
  toggleRow?: (index: number, row: number) => void;
  /** 无权限时「去申请」的链接（dsh-ee 网页上这个连接的页面） */
  requestHref?: (connection: string) => string | undefined;
  /** 网页版：没有 AI，发给 AI 的按钮标成「↗」，点了到 DSH 里继续 */
  viaDSH?: boolean;
  /** 删除行的按钮点过一次、等待再点一次确认 */
  armed?: (key: string) => boolean;
}

/** 按钮文字：网页版里发给 AI 的按钮后面加「↗」（提交到表的表单不算）。 */
function actionLabel(handlers: PanelHandlers, action: WorkbenchAction): string {
  return handlers.viaDSH && !action.table ? `${action.label} ↗` : action.label;
}

/** 按钮的样式：写表删除是红字，点过一次待确认时整个变红并改成「确认删除」。 */
function actionButton(h: H, handlers: PanelHandlers, key: string, action: WorkbenchAction, onClick: () => void) {
  const write = rowWriteOf(action);
  const armed = write === "delete" && !!handlers.armed?.(key);
  return button(h, armed ? t("workbenchView.confirmDelete") : actionLabel(handlers, action), onClick, handlers.sending.has(key), key, armed ? "armed" : write === "delete" ? "danger" : "default");
}

// 视觉：Stripe 式的仪表盘——白底卡片（宿主可用 --ee-card-bg 指定）、细边框、柔和的双层阴影、品牌蓝点缀。
// 颜色从 currentColor 混出来，跟随 DSH 的亮色 / 暗色主题；版式用容器查询，右侧栏窄时自动叠成一列。
const muted = { color: "var(--ee-muted)", fontSize: 12 };
const card = {
  border: "1px solid var(--ee-border)", borderRadius: 12, padding: "14px 16px",
  background: "var(--ee-card-bg, color-mix(in srgb, currentColor 2.5%, transparent))",
  boxShadow: "0 1px 1px color-mix(in srgb, currentColor 4%, transparent), 0 2px 6px -2px color-mix(in srgb, currentColor 8%, transparent)",
  boxSizing: "border-box" as const, minWidth: 0, height: "100%",
};
const buttonStyle = { border: "1px solid var(--ee-border-strong)", borderRadius: 8, background: "var(--ee-card-bg, transparent)", color: "inherit", cursor: "pointer", padding: "5px 11px", fontSize: 12, fontWeight: 500, lineHeight: 1.4, whiteSpace: "nowrap" as const };
const primaryButton = { ...buttonStyle, border: "1px solid var(--ee-accent)", background: "var(--ee-accent)", color: "#fff", padding: "7px 16px", fontSize: 13 };
const warn = "#b45309";
/** 图表序列的颜色：亮色、暗色背景上都看得清 */
export const seriesColors = ["#4d6bfe", "#00a3bf", "#f59e0b", "#db2777"];
const widgetNames = (): Record<WorkbenchWidget["type"], string> => ({ metric: t("workbenchView.widgetMetric"), table: t("workbenchView.widgetTable"), chart: t("workbenchView.widgetChart"), checklist: t("workbenchView.widgetChecklist"), form: t("workbenchView.widgetForm"), actions: t("workbenchView.widgetActions"), markdown: t("workbenchView.widgetMarkdown"), heading: "", board: t("workbenchView.widgetBoard"), kb_scope: t("workbenchView.widgetKbScope") });
const toneColors = { good: "#16a34a", warn: "#d97706", bad: "#dc2626", neutral: "currentColor" };

/**
 * 面板的样式表：变量、动画、12 栏网格与容器查询（窄于 600 像素时每个组件占满一行，指标两个一行）、
 * 悬停与焦点效果。内联样式写不了这些，所以放在面板里的一个 <style> 里，类名都带 dshee- 前缀。
 */
export const panelCss = [
  "@keyframes dshee-in{from{opacity:.2;transform:translateY(3px)}to{opacity:1;transform:none}}",
  "@keyframes dshee-pulse{0%,100%{opacity:.45}50%{opacity:.9}}",
  ".dshee-root{container-type:inline-size;--ee-accent:#4d6bfe;--ee-border:color-mix(in srgb,currentColor 10%,transparent);--ee-border-strong:color-mix(in srgb,currentColor 18%,transparent);--ee-soft:color-mix(in srgb,currentColor 4%,transparent);--ee-muted:color-mix(in srgb,currentColor 60%,transparent)}",
  ".dshee-grid{display:grid;grid-template-columns:repeat(12,minmax(0,1fr));gap:12px;align-items:stretch}",
  ".dshee-grid>.dshee-w{grid-column:span 12;min-width:0}",
  ".dshee-grid>.dshee-w-metric{grid-column:span 6}",
  "@container (min-width:600px){.dshee-grid>.dshee-w-quarter{grid-column:span 3}.dshee-grid>.dshee-w-third{grid-column:span 4}.dshee-grid>.dshee-w-half{grid-column:span 6}.dshee-grid>.dshee-w-wide{grid-column:span 8}.dshee-grid>.dshee-w-full{grid-column:span 12}.dshee-grid{gap:16px}}",
  ".dshee-btn{transition:background .15s,border-color .15s,box-shadow .15s,transform .05s}.dshee-btn:hover:not(:disabled){border-color:var(--ee-accent);color:var(--ee-accent)}.dshee-btn:active:not(:disabled){transform:translateY(1px)}",
  ".dshee-primary:hover:not(:disabled){filter:brightness(1.08);color:#fff!important;box-shadow:0 4px 12px -4px color-mix(in srgb,var(--ee-accent) 60%,transparent)}",
  ".dshee-danger{color:#dc2626!important}.dshee-danger:hover:not(:disabled){border-color:#dc2626!important}.dshee-armed{background:#dc2626!important;border-color:#dc2626!important;color:#fff!important}",
  ".dshee-tile{transition:border-color .15s,box-shadow .15s,transform .15s}.dshee-tile:hover{border-color:color-mix(in srgb,var(--ee-accent) 55%,transparent);box-shadow:0 6px 16px -8px color-mix(in srgb,var(--ee-accent) 45%,transparent);transform:translateY(-1px)}",
  ".dshee-row{transition:background .12s}.dshee-row:hover{background:var(--ee-soft)}",
  ".dshee-input{transition:border-color .15s,box-shadow .15s}.dshee-input:focus{outline:none;border-color:var(--ee-accent);box-shadow:0 0 0 3px color-mix(in srgb,var(--ee-accent) 18%,transparent)}",
  ".dshee-card{transition:box-shadow .2s}",
].join("");

function button(h: H, label: string, onClick: () => void, disabled = false, key?: string, variant: "default" | "primary" | "danger" | "armed" = "default") {
  const style = variant === "primary" ? primaryButton : buttonStyle;
  const className = `dshee-btn${variant === "primary" ? " dshee-primary" : variant === "danger" ? " dshee-danger" : variant === "armed" ? " dshee-armed" : ""}`;
  return h("button", { key, type: "button", onClick, disabled, className, style: { ...style, opacity: disabled ? 0.5 : 1, cursor: disabled ? "default" : "pointer" } }, label);
}

function header(h: H, widget: WorkbenchWidget, refresh?: () => void, aside?: any) {
  return h("div", { style: { display: "flex", alignItems: "center", justifyContent: "space-between", gap: 8, marginBottom: 10, minHeight: 20 } },
    h("strong", { style: { fontSize: 13, fontWeight: 600, letterSpacing: "0.01em", minWidth: 0, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" } }, widget.title || widgetNames()[widget.type]),
    h("span", { style: { display: "flex", alignItems: "center", gap: 6, flex: "0 0 auto" } },
      aside ?? null,
      refresh ? h("button", { type: "button", onClick: refresh, "aria-label": t("common.refresh"), title: t("common.refresh"), className: "dshee-btn", style: { ...buttonStyle, border: 0, background: "transparent", padding: "0 4px", fontSize: 13, color: "var(--ee-muted)" } }, "↻") : null));
}

/** 状态标签：带颜色的小圆点 + 文字，颜色按文字里的关键词选。 */
export function statusPill(h: H, value: unknown, text: string) {
  const color = toneColors[statusTone(value)];
  return h("span", { "data-status": statusTone(value), style: { display: "inline-flex", alignItems: "center", gap: 5, padding: "1px 8px", borderRadius: 999, fontSize: 11.5, fontWeight: 500, lineHeight: "18px", color, background: `color-mix(in srgb, ${color} 12%, transparent)`, whiteSpace: "nowrap" } },
    h("span", { "aria-hidden": true, style: { width: 6, height: 6, borderRadius: "50%", background: color, opacity: 0.85 } }),
    text);
}

/** 单元格：status 格式显示成标签，其余按格式出文字。 */
function cell(h: H, value: unknown, format?: string) {
  const text = formatValue(value, format === "status" ? undefined : format);
  return format === "status" && text !== "—" ? statusPill(h, value, text) : text;
}

/** 骨架屏：几条呼吸的灰条，按组件的样子排。 */
export function skeleton(h: H, shape: "metric" | "rows" | "chart" = "rows") {
  const bar = (key: number, width: string, height = 10) => h("div", { key, style: { width, height, borderRadius: 4, background: "color-mix(in srgb, currentColor 12%, transparent)", animation: "dshee-pulse 1.4s ease-in-out infinite" } });
  const bars = shape === "metric" ? [bar(0, "45%", 22), bar(1, "30%", 8)]
    : shape === "chart" ? [bar(0, "100%", 90)]
    : [bar(0, "90%"), bar(1, "75%"), bar(2, "82%")];
  return h("div", { "data-state": "loading", "aria-busy": "true", style: { display: "grid", gap: 7, padding: "2px 0" } },
    ...bars,
    h("span", { style: { position: "absolute", width: 1, height: 1, overflow: "hidden", clip: "rect(0 0 0 0)" } }, t("common.loading")));
}

/** 加载中、出错、无权限三种状态；返回 null 表示数据已就绪。原文窗格等简单场景用。 */
export function stateNote(h: H, state: WidgetState | undefined) {
  if (!state || state.status === "loading") return skeleton(h);
  if (state.status === "forbidden") return h("div", { "data-state": "forbidden", style: muted }, t("workbenchView.forbidden"));
  if (state.status === "error") return h("div", { "data-state": "error", style: { ...muted, color: warn, opacity: 1, overflowWrap: "anywhere" } }, t("workbenchView.fetchFailed", { error: state.error }));
  return null;
}

/**
 * 有数据源的组件的四种状态：加载中（骨架屏）、无权限（缺什么、去申请）、出错（原因 + 重试）；
 * 数据就绪返回 null，空数据由各组件调 emptyView。
 */
export function stateView(h: H, index: number, widget: WorkbenchWidget, state: WidgetState | undefined, handlers: PanelHandlers, shape: "metric" | "rows" | "chart" = "rows") {
  if (!state || state.status === "loading") return skeleton(h, shape);
  if (state.status === "forbidden") {
    const connection = widget.source?.connection ?? "";
    const href = connection ? handlers.requestHref?.(connection) : undefined;
    return h("div", { "data-state": "forbidden", style: { fontSize: 12, display: "grid", gap: 4 } },
      h("span", { style: { opacity: 0.8 } }, t("workbenchView.forbiddenDetail", { connection: connection || t("workbenchView.thisDataSource"), tool: widget.source?.tool ?? "" })),
      href ? h("a", { href, target: "_blank", rel: "noopener noreferrer", style: { color: "inherit", textDecoration: "underline", width: "fit-content" } }, t("workbenchView.requestAccess")) : h("span", { style: muted }, t("workbenchView.contactAdmin")));
  }
  if (state.status === "error") {
    return h("div", { "data-state": "error", role: "alert", style: { fontSize: 12, display: "grid", gap: 6, justifyItems: "start" } },
      h("span", { style: { color: warn, overflowWrap: "anywhere" } }, t("workbenchView.fetchFailed", { error: state.error })),
      button(h, t("common.retry"), () => handlers.refresh(index), false, "retry"));
  }
  return null;
}

/** 空状态：一句说明 + 一个建议操作（配置里没写时给默认说明）。 */
export function emptyView(h: H, index: number, widget: WorkbenchWidget, handlers: PanelHandlers, fallback = t("common.noData")) {
  const action = widget.empty?.action;
  const key = `${index}:empty`;
  return h("div", { "data-state": "empty", style: { display: "grid", gap: 10, justifyItems: "center", textAlign: "center", fontSize: 12, padding: "18px 12px", borderRadius: 10, border: "1px dashed var(--ee-border-strong)", background: "var(--ee-soft)" } },
    h("span", { "aria-hidden": true, style: { fontSize: 18, opacity: 0.55 } }, "∅"),
    h("span", { style: { color: "var(--ee-muted)", lineHeight: 1.6, maxWidth: 360 } }, widget.empty?.text || fallback),
    action ? button(h, actionLabel(handlers, action), () => handlers.send(key, action, {}), handlers.sending.has(key), key) : null);
}

export function metricView(h: H, widget: WorkbenchWidget, state: WidgetState | undefined, refresh: () => void, handlers?: PanelHandlers, index = 0) {
  const note = handlers ? stateView(h, index, widget, state, handlers, "metric") : stateNote(h, state);
  let body = note;
  if (!note) {
    const value = getPath((state as any).value, widget.field);
    const text = formatValue(value, widget.format);
    const delta = widget.delta_field ? formatDelta(getPath((state as any).value, widget.delta_field)) : undefined;
    const deltaColor = delta?.direction === "up" ? "#16a34a" : delta?.direction === "down" ? "#dc2626" : "currentColor";
    body = h("div", { style: { display: "flex", alignItems: "baseline", gap: 8, flexWrap: "wrap" } },
      // key 跟着值走：值变了重新挂载，播一次淡入
      h("span", { key: text, "data-value": text, style: { fontSize: 26, lineHeight: 1.15, fontWeight: 600, letterSpacing: "-0.02em", fontVariantNumeric: "tabular-nums", animation: "dshee-in .35s ease-out" } }, text),
      delta ? h("span", { style: { fontSize: 11.5, fontWeight: 600, padding: "1px 7px", borderRadius: 999, color: deltaColor, background: `color-mix(in srgb, ${deltaColor} 12%, transparent)`, opacity: delta.direction === "flat" ? 0.7 : 1 } }, delta.text) : null);
  }
  return h("section", { className: "dshee-card", style: { ...card, padding: "13px 16px 15px" } },
    h("div", { style: { display: "flex", alignItems: "center", justifyContent: "space-between", gap: 6, marginBottom: 8 } },
      h("span", { style: { fontSize: 12, fontWeight: 500, color: "var(--ee-muted)", minWidth: 0, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" } }, widget.title || widgetNames().metric),
      h("button", { type: "button", onClick: refresh, "aria-label": t("common.refresh"), title: t("common.refresh"), className: "dshee-btn", style: { ...buttonStyle, border: 0, background: "transparent", padding: "0 2px", fontSize: 12, color: "var(--ee-muted)" } }, "↻")),
    body);
}

const th = { textAlign: "left" as const, padding: "8px 10px", borderBottom: "1px solid var(--ee-border)", whiteSpace: "nowrap" as const, fontWeight: 500, fontSize: 11.5, color: "var(--ee-muted)", background: "var(--ee-soft)" };
const td = { padding: "9px 10px", borderBottom: "1px solid var(--ee-border)", fontVariantNumeric: "tabular-nums" as const, whiteSpace: "nowrap" as const, maxWidth: 260, overflow: "hidden", textOverflow: "ellipsis", verticalAlign: "middle" as const };

export function tableView(h: H, index: number, widget: WorkbenchWidget, state: WidgetState | undefined, handlers: PanelHandlers) {
  const note = stateView(h, index, widget, state, handlers);
  const columns = widget.columns ?? [];
  const actions = widget.row_actions ?? [];
  const detail = widget.detail ?? [];
  const open = handlers.expandedRow?.(index);
  let body: any = note;
  let count: any = null;
  if (!note) {
    const { rows, total } = tableRows((state as any).value, widget.rows);
    const span = columns.length + (actions.length ? 1 : 0) + (detail.length ? 1 : 0);
    if (rows.length) count = h("span", { style: { ...muted, fontSize: 11.5, fontVariantNumeric: "tabular-nums" } }, t("workbenchView.countRows", { total }));
    body = rows.length === 0 ? emptyView(h, index, widget, handlers) : h("div", { style: { overflowX: "auto", margin: "0 -16px", borderTop: "1px solid var(--ee-border)" } },
      h("table", { style: { width: "100%", borderCollapse: "collapse", fontSize: 12.5 } },
        h("thead", null, h("tr", null,
          detail.length ? h("th", { key: "toggle", style: { ...th, width: 18, paddingLeft: 16, paddingRight: 0 } }) : null,
          ...columns.map((column, c) => h("th", { key: c, style: { ...th, paddingLeft: c === 0 && !detail.length ? 16 : 10, textAlign: column.format === "money" || column.format === "number" || column.format === "compact" ? "right" : "left" } }, column.title)),
          actions.length ? h("th", { key: "actions", style: { ...th, paddingRight: 16 } }) : null)),
        h("tbody", null, ...rows.flatMap((row, r) => {
          const expanded = open === r;
          const line = h("tr", { key: r, className: "dshee-row", style: { cursor: detail.length ? "pointer" : "default", background: expanded ? "var(--ee-soft)" : undefined }, onClick: detail.length ? () => handlers.toggleRow?.(index, r) : undefined },
            detail.length ? h("td", { key: "toggle", style: { ...td, padding: "9px 0 9px 16px", color: "var(--ee-muted)" } }, h("button", { type: "button", "aria-expanded": expanded, "aria-label": expanded ? t("workbenchView.collapseDetail") : t("workbenchView.expandDetail"), onClick: (event: any) => { event?.stopPropagation?.(); handlers.toggleRow?.(index, r); }, style: { ...buttonStyle, border: 0, background: "transparent", padding: 0, color: "inherit" } }, expanded ? "▾" : "▸")) : null,
            ...columns.map((column, c) => {
              const value = getPath(row, column.field);
              const numeric = column.format === "money" || column.format === "number" || column.format === "compact";
              return h("td", { key: c, style: { ...td, paddingLeft: c === 0 && !detail.length ? 16 : 10, fontWeight: c === 0 ? 500 : 400, textAlign: numeric ? "right" : "left" }, title: String(formatValue(value, column.format === "status" ? undefined : column.format) ?? "") }, cell(h, value, column.format));
            }),
            actions.length ? h("td", { key: "actions", style: { ...td, paddingRight: 16, textAlign: "right", maxWidth: "none" }, onClick: (event: any) => event?.stopPropagation?.() },
              h("span", { style: { display: "inline-flex", gap: 6 } },
                ...actions.map((action, a) => {
                  const key = `${index}:row:${r}:${a}`;
                  return actionButton(h, handlers, key, action, () => handlers.send(key, action, { row }));
                }))) : null);
          if (!expanded) return [line];
          return [line, h("tr", { key: `${r}:detail` }, h("td", { colSpan: span, style: { padding: "10px 16px 14px 40px", borderBottom: "1px solid var(--ee-border)", background: "var(--ee-soft)" } },
            h("dl", { "data-detail": r, style: { margin: 0, display: "grid", gridTemplateColumns: "max-content minmax(0, 1fr)", gap: "6px 16px", fontSize: 12.5, animation: "dshee-in .2s ease-out" } },
              ...detail.flatMap((field, f) => [
                h("dt", { key: `t${f}`, style: { color: "var(--ee-muted)" } }, field.title),
                h("dd", { key: `d${f}`, style: { margin: 0, overflowWrap: "anywhere" } }, cell(h, getPath(row, field.field), field.format)),
              ]))))];
        }))),
      total > rows.length ? h("div", { style: { ...muted, padding: "8px 16px 0" } }, t("workbenchView.truncatedRows", { total, shown: rows.length })) : null);
  }
  return h("section", { className: "dshee-card", style: { ...card, paddingBottom: note || !count ? 14 : 6 } }, header(h, widget, () => handlers.refresh(index), count), body);
}

/** 图表：SVG 自己画（折线、柱状、饼图），不引入图表库。 */
export function chartView(h: H, index: number, widget: WorkbenchWidget, state: WidgetState | undefined, handlers: PanelHandlers) {
  const note = stateView(h, index, widget, state, handlers, "chart");
  let body: any = note;
  if (!note) {
    const data = chartSeries((state as any).value, widget);
    body = data.labels.length === 0 ? emptyView(h, index, widget, handlers) : widget.chart === "pie" ? pieChart(h, data) : xyChart(h, widget.chart === "bar" ? "bar" : "line", data);
  }
  return h("section", { className: "dshee-card", style: card }, header(h, widget, () => handlers.refresh(index)), body);
}

type ChartData = ReturnType<typeof chartSeries>;

function legend(h: H, items: { label: string; color: string; value?: string }[]) {
  return h("div", { style: { display: "flex", flexWrap: "wrap", gap: "4px 12px", marginTop: 6, fontSize: 11 } },
    ...items.map((item, i) => h("span", { key: i, style: { display: "inline-flex", alignItems: "center", gap: 4 } },
      h("span", { style: { width: 8, height: 8, borderRadius: 2, background: item.color, display: "inline-block" } }),
      item.label,
      item.value ? h("span", { style: { opacity: 0.6 } }, item.value) : null)));
}

function niceMax(max: number) {
  if (max <= 0) return 1;
  const step = 10 ** Math.floor(Math.log10(max));
  return Math.ceil(max / step) * step;
}

function xyChart(h: H, kind: "line" | "bar", data: ChartData) {
  const W = 480, H = 170, left = 38, bottom = 18, top = 8, right = 6;
  const max = niceMax(Math.max(0, ...data.series.flatMap((s) => s.values)));
  const n = data.labels.length;
  const plotW = W - left - right, plotH = H - top - bottom;
  const y = (v: number) => top + plotH - (Math.max(0, v) / max) * plotH;
  const slot = plotW / Math.max(n, 1);
  const xMid = (i: number) => left + slot * i + slot / 2;
  const fmt = (v: number) => formatValue(v, data.series[0]?.format === "money" || data.series[0]?.format === "compact" ? "compact" : "number");
  const grid = [0, 0.5, 1].map((f) => h("g", { key: `g${f}` },
    h("line", { x1: left, x2: W - right, y1: y(max * f), y2: y(max * f), stroke: "currentColor", strokeOpacity: f === 0 ? 0.3 : 0.1, strokeDasharray: f === 0 ? undefined : "3 3" }),
    h("text", { x: left - 4, y: y(max * f) + 3, textAnchor: "end", fontSize: 9, fill: "currentColor", fillOpacity: 0.55 }, fmt(max * f))));
  const marks = data.series.flatMap((s, si) => {
    const color = seriesColors[si % seriesColors.length];
    if (kind === "bar") {
      const groupW = slot * 0.7;
      const barW = groupW / data.series.length;
      return s.values.map((v, i) => h("rect", { key: `b${si}-${i}`, x: left + slot * i + (slot - groupW) / 2 + barW * si, y: y(v), width: Math.max(barW - 1, 1), height: Math.max(top + plotH - y(v), 0), rx: 3, fill: color, fillOpacity: 0.9 },
        h("title", null, t("workbenchView.pointTip", { label: data.labels[i], series: s.title, value: formatValue(v, s.format) }))));
    }
    const points = s.values.map((v, i) => `${xMid(i).toFixed(1)},${y(v).toFixed(1)}`).join(" ");
    const area = si === 0 && n > 1 ? h("polygon", { key: `a${si}`, points: `${xMid(0).toFixed(1)},${(top + plotH).toFixed(1)} ${points} ${xMid(n - 1).toFixed(1)},${(top + plotH).toFixed(1)}`, fill: color, fillOpacity: 0.08 }) : null;
    return [
      area,
      h("polyline", { key: `l${si}`, points, fill: "none", stroke: color, strokeWidth: 2, strokeLinejoin: "round", strokeLinecap: "round" }),
      ...s.values.map((v, i) => h("circle", { key: `c${si}-${i}`, cx: xMid(i), cy: y(v), r: n > 24 ? 1.5 : 2.5, fill: color },
        h("title", null, t("workbenchView.pointTip", { label: data.labels[i], series: s.title, value: formatValue(v, s.format) })))),
    ];
  });
  const ticks = n <= 1 ? [0] : n <= 6 ? data.labels.map((_, i) => i) : [0, Math.floor((n - 1) / 2), n - 1];
  const xLabels = ticks.map((i) => h("text", { key: `x${i}`, x: xMid(i), y: H - 4, textAnchor: "middle", fontSize: 9, fill: "currentColor", fillOpacity: 0.6 }, data.labels[i]));
  return h("div", null,
    h("svg", { viewBox: `0 0 ${W} ${H}`, width: "100%", role: "img", "aria-label": data.series.map((s) => s.title).join(t("common.listSeparator")), style: { display: "block", overflow: "visible", maxHeight: 240 } }, ...grid, ...marks, ...xLabels),
    data.series.length > 1 ? legend(h, data.series.map((s, i) => ({ label: s.title, color: seriesColors[i % seriesColors.length] }))) : null);
}

function pieChart(h: H, data: ChartData) {
  const series = data.series[0];
  const values = series?.values.map((v) => Math.max(0, v)) ?? [];
  const total = values.reduce((a, b) => a + b, 0);
  if (!total) return h("div", { "data-state": "empty", style: muted }, t("common.noData"));
  const R = 54, C = 60;
  const palette = (i: number) => seriesColors[i % seriesColors.length];
  const shade = (i: number) => (i < seriesColors.length ? palette(i) : `color-mix(in srgb, ${palette(i)} ${100 - 25 * Math.floor(i / seriesColors.length)}%, transparent)`);
  let angle = -Math.PI / 2;
  const slices = values.map((v, i) => {
    const sweep = (v / total) * Math.PI * 2;
    const a0 = angle;
    angle += sweep;
    const tip = t("workbenchView.pieTip", { label: data.labels[i], value: formatValue(v, series.format), percent: Math.round((v / total) * 100) });
    if (sweep >= Math.PI * 2 - 1e-6) return h("circle", { key: i, cx: C, cy: C, r: R, fill: shade(i) }, h("title", null, tip));
    const p = (a: number) => `${(C + R * Math.cos(a)).toFixed(2)} ${(C + R * Math.sin(a)).toFixed(2)}`;
    return h("path", { key: i, d: `M ${C} ${C} L ${p(a0)} A ${R} ${R} 0 ${sweep > Math.PI ? 1 : 0} 1 ${p(angle)} Z`, fill: shade(i), stroke: "var(--background, transparent)", strokeWidth: 1 }, h("title", null, tip));
  });
  return h("div", { style: { display: "flex", alignItems: "center", gap: 12, flexWrap: "wrap" } },
    h("svg", { viewBox: "0 0 120 120", width: 120, height: 120, role: "img", "aria-label": series.title }, ...slices, h("circle", { cx: C, cy: C, r: 26, fill: "var(--background, Canvas)" })),
    h("div", { style: { flex: 1, minWidth: 120 } }, legend(h, data.labels.map((label, i) => ({ label, color: shade(i), value: `${Math.round((values[i] / total) * 100)}%` })))));
}

/** 清单：员工可以勾选；AI 也能用 ee_workbench 更新。勾选状态按会话保存在本机。 */
export function checklistView(h: H, index: number, widget: WorkbenchWidget, state: WidgetState | undefined, handlers: PanelHandlers) {
  const fromSource = !!widget.source && !widget.items?.length;
  const note = fromSource ? stateView(h, index, widget, state, handlers) : null;
  const items = note ? [] : checklistItems(widget, (state as any)?.value);
  const checked = handlers.checked?.(index) ?? [];
  const done = items.filter((item) => checked.includes(item.id)).length;
  let body: any = note;
  if (!note) {
    body = items.length === 0 ? emptyView(h, index, widget, handlers) : h("div", { style: { display: "grid", gap: 10 } },
      h("div", { style: { height: 6, borderRadius: 999, background: "var(--ee-soft)", overflow: "hidden" } },
        h("div", { style: { width: `${(done / items.length) * 100}%`, height: "100%", borderRadius: 999, background: "linear-gradient(90deg, var(--ee-accent), #00a3bf)", transition: "width .3s ease-out" } })),
      h("ul", { style: { listStyle: "none", margin: 0, padding: 0, display: "grid", gap: 2 } },
        ...items.map((item) => {
          const on = checked.includes(item.id);
          return h("li", { key: item.id },
            h("label", { className: "dshee-row", style: { display: "flex", alignItems: "flex-start", gap: 8, fontSize: 12.5, lineHeight: 1.5, cursor: "pointer", padding: "5px 6px", borderRadius: 6 } },
              h("input", { type: "checkbox", checked: on, onChange: () => handlers.toggleCheck?.(index, item.id), style: { marginTop: 3, accentColor: "var(--ee-accent)" } }),
              h("span", { style: { textDecoration: on ? "line-through" : "none", opacity: on ? 0.5 : 1 } }, item.text)));
        })));
  }
  const progress = items.length ? h("span", { style: { fontSize: 11.5, fontWeight: 600, padding: "1px 8px", borderRadius: 999, color: done === items.length ? "#16a34a" : "var(--ee-accent)", background: `color-mix(in srgb, ${done === items.length ? "#16a34a" : "var(--ee-accent)"} 12%, transparent)`, fontVariantNumeric: "tabular-nums" } }, `${done}/${items.length}`) : null;
  return h("section", { className: "dshee-card", style: card }, header(h, widget, fromSource ? () => handlers.refresh(index) : undefined, progress), body);
}

/** 分组标题：把后面几个组件归到一组。 */
export function headingView(h: H, widget: WorkbenchWidget) {
  return h("h3", { style: { margin: "8px 2px 0", fontSize: 14, fontWeight: 600, letterSpacing: "0.01em", display: "flex", alignItems: "center", gap: 8 } }, widget.title);
}

const inputStyle = { width: "100%", boxSizing: "border-box" as const, height: 34, padding: "0 10px", borderRadius: 8, border: "1px solid var(--ee-border-strong)", background: "var(--ee-card-bg, transparent)", color: "inherit", fontSize: 13 };

export function formView(h: H, index: number, widget: WorkbenchWidget, handlers: PanelHandlers) {
  const values = handlers.formValues(index);
  const fields = widget.fields ?? [];
  const key = `${index}:form`;
  const submit = () => {
    const error = checkForm(fields, values);
    handlers.setFormError(index, error);
    if (!error && widget.submit) handlers.send(key, widget.submit, { form: values });
  };
  const input = (field: (typeof fields)[number]) => {
    const common = {
      "aria-label": field.label,
      value: values[field.name] ?? "",
      className: "dshee-input",
      onChange: (event: any) => handlers.setFormValue(index, field.name, event.target.value),
      style: inputStyle,
    };
    if (field.kind === "select") {
      return h("select", common, h("option", { value: "" }, t("workbenchView.selectPlaceholder")), ...(field.options ?? []).map((option) => h("option", { key: option, value: option }, option)));
    }
    return h("input", { ...common, type: field.kind === "number" ? "number" : field.kind === "date" ? "date" : "text" });
  };
  return h("section", { className: "dshee-card", style: card },
    header(h, widget),
    h("div", { style: { display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(min(100%, 170px), 1fr))", gap: "10px 12px" } },
      ...fields.map((field) => h("label", { key: field.name, style: { display: "grid", gap: 5, fontSize: 12, fontWeight: 500, alignContent: "start" } },
        h("span", null, field.label, field.required ? h("span", { style: { color: "#dc2626" } }, " *") : null),
        input(field)))),
    handlers.formError(index) ? h("div", { role: "alert", style: { color: "#dc2626", fontSize: 12, marginTop: 8 } }, handlers.formError(index)) : null,
    h("div", { style: { marginTop: 12, display: "flex", justifyContent: "flex-end" } }, button(h, widget.submit ? actionLabel(handlers, widget.submit) || t("common.submit") : t("common.submit"), submit, handlers.sending.has(key), undefined, "primary")));
}

export function actionsView(h: H, index: number, widget: WorkbenchWidget, handlers: PanelHandlers) {
  return h("section", { className: "dshee-card", style: card },
    header(h, widget),
    h("div", { style: { display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(min(100%, 130px), 1fr))", gap: 8 } },
      ...(widget.actions ?? []).map((action, a) => {
        const key = `${index}:action:${a}`;
        const disabled = handlers.sending.has(key);
        return h("button", {
          key, type: "button", disabled, className: "dshee-tile", onClick: () => handlers.send(key, action, {}),
          style: { display: "flex", alignItems: "center", justifyContent: "space-between", gap: 6, padding: "9px 11px", borderRadius: 9, border: "1px solid var(--ee-border)", background: "var(--ee-card-bg, var(--ee-soft))", color: "inherit", cursor: disabled ? "default" : "pointer", opacity: disabled ? 0.5 : 1, fontSize: 12.5, fontWeight: 500, textAlign: "left" },
        }, actionLabel(handlers, action), h("span", { "aria-hidden": true, style: { color: "var(--ee-accent)", fontSize: 14, lineHeight: 1 } }, "›"));
      })));
}

/** 说明文字里的一行：只认 **加粗**，其余按纯文字显示，不解析 HTML。 */
function inline(h: H, text: string) {
  const parts = text.split(/(\*\*[^*]+\*\*)/g).filter(Boolean);
  return parts.map((part, i) => (part.startsWith("**") && part.endsWith("**") && part.length > 4 ? h("strong", { key: i, style: { fontWeight: 600 } }, part.slice(2, -2)) : part));
}

/** 说明文字：段落、「- 」列表和 **加粗**，其余按纯文字显示（不解析 HTML）。 */
export function markdownView(h: H, widget: WorkbenchWidget) {
  const blocks: any[] = [];
  let list: string[] = [];
  const flush = () => {
    if (!list.length) return;
    blocks.push(h("ul", { key: `l${blocks.length}`, style: { margin: 0, paddingLeft: 18, display: "grid", gap: 3 } }, ...list.map((item, i) => h("li", { key: i }, ...inline(h, item)))));
    list = [];
  };
  for (const line of (widget.text ?? "").split("\n")) {
    const item = /^\s*[-*]\s+(.*)$/.exec(line);
    if (item) {
      list.push(item[1]);
      continue;
    }
    flush();
    if (line.trim()) blocks.push(h("p", { key: `p${blocks.length}`, style: { margin: 0 } }, ...inline(h, line)));
  }
  flush();
  return h("section", { className: "dshee-card", style: card }, widget.title ? header(h, widget) : null, h("div", { style: { display: "grid", gap: 6, fontSize: 12.5, lineHeight: 1.7, color: "color-mix(in srgb, currentColor 85%, transparent)" } }, ...blocks));
}

const citationBadge = (status: Citation["status"]): { text: string; color: string } => {
  switch (status) {
    case "ok": return { text: t("workbenchView.citationOk"), color: "#15803d" };
    case "stale": return { text: t("workbenchView.citationStale"), color: "#a16207" };
    case "mismatch": return { text: t("workbenchView.citationMismatch"), color: "#b91c1c" };
    case "missing": return { text: t("workbenchView.citationMissing"), color: "#b91c1c" };
    default: return { text: t("workbenchView.citationUnchecked"), color: "inherit" };
  }
};

/** 引用编号的小标签：绿色对得上、红色对不上、灰色原文已更新或没核对。 */
function citationChip(h: H, n: number, status: Citation["status"]) {
  const color = status === "ok" ? "#15803d" : status === "mismatch" || status === "missing" ? "#b91c1c" : "var(--ee-muted)";
  return h("span", {
    "aria-hidden": true,
    style: { display: "inline-flex", alignItems: "center", justifyContent: "center", minWidth: 18, height: 18, padding: "0 5px", borderRadius: 999, fontSize: 10.5, fontWeight: 600,
      color, border: `1px solid color-mix(in srgb, ${color} 45%, transparent)`, background: `color-mix(in srgb, ${color} 10%, transparent)`, boxSizing: "border-box" },
  }, String(n));
}

/** 看板上的引用：编号、文档、行号与核对结果；点了在「原文」窗格打开。鼠标移上去能看到摘句。 */
export function citationsView(h: H, items: Citation[], key: number, open?: (citation: Citation) => void, hrefOf?: (citation: Citation) => string | undefined) {
  return h("ol", { key, style: { margin: 0, padding: 0, listStyle: "none", fontSize: 12, display: "grid", gap: 8 } },
    ...items.map((item, i) => {
      const badge = citationBadge(item.status);
      const range = item.from === item.to ? `${item.from}` : `${item.from}–${item.to}`;
      const where = t("workbenchView.citationWhere", { title: item.title || t("workbenchClient.docLabel", { doc: item.doc }), range });
      const tip = t("workbenchView.citationTip", { where, quote: item.quote });
      return h("li", { key: i, "data-citation": i + 1, style: { display: "grid", gridTemplateColumns: "auto 1fr", gap: "2px 8px", alignItems: "start" } },
        citationChip(h, i + 1, item.status),
        h("div", { style: { minWidth: 0 } },
          h("div", { style: { display: "flex", flexWrap: "wrap", alignItems: "center", gap: 6 } },
            open ? h("button", { type: "button", onClick: () => open(item), title: tip, "aria-label": t("workbenchView.viewSource", { where }), className: "dshee-btn", style: { ...buttonStyle, padding: "1px 7px" } }, where)
              : hrefOf?.(item) ? h("a", { href: hrefOf(item), target: "_blank", rel: "noopener noreferrer", title: t("workbenchView.webSourceTip", { tip }), style: { color: "inherit", textDecoration: "underline" } }, `${where} ↗`)
              : h("span", { title: tip }, where),
            h("span", { style: { color: badge.color, fontSize: 11, opacity: item.status === "unchecked" || !item.status ? 0.6 : 1 } }, badge.text),
            item.near_skipped ? h("span", { title: t("workbenchView.nearSkippedTip"), style: { fontSize: 11, color: "#a16207" } }, t("workbenchView.nearSkipped", { page: item.near_skipped })) : null),
          h("div", { style: { ...muted, marginTop: 2, whiteSpace: "pre-wrap" } }, t("workbenchView.quoteText", { quote: item.quote }))));
    }));
}

/** 「查哪些库」：勾选要查的知识库；不勾就是全部。 */
export function kbScopeView(h: H, widget: WorkbenchWidget, state: WidgetState | undefined, handlers: PanelHandlers, index = 0) {
  const note = stateView(h, index, widget, state, handlers);
  const bases: KBaseItem[] = note ? [] : (getPath((state as any).value, "items") ?? []);
  const selected = handlers.selectedKBs ?? [];
  const body = note ?? (bases.length === 0 ? h("div", { style: muted }, t("workbenchView.noKbs")) : h("div", { style: { display: "flex", flexWrap: "wrap", gap: "4px 12px" } },
    ...bases.map((item) => h("label", { key: item.kb, style: { display: "flex", alignItems: "center", gap: 4, fontSize: 12, cursor: "pointer" } },
      h("input", { type: "checkbox", checked: selected.includes(item.kb), onChange: () => handlers.toggleKB?.(item.kb) }),
      item.title,
      item.docs !== undefined ? h("span", { style: muted }, t("workbenchView.kbDocs", { count: item.docs })) : null))));
  return h("section", { className: "dshee-card", style: card },
    header(h, widget),
    body,
    bases.length > 0 ? h("div", { style: { ...muted, marginTop: 6 } }, selected.length ? t("workbenchView.scopeOnly", { count: selected.length }) : t("workbenchView.scopeAllHint")) : null);
}

/** AI 看板：内容只当文字显示。 */
export function boardView(h: H, widget: WorkbenchWidget, board: Board | null | undefined, openCitation?: (citation: Citation) => void, citationHref?: (citation: Citation) => string | undefined) {
  let body: any;
  if (!board?.blocks?.length) {
    body = h("div", { style: { ...muted, padding: "16px 12px", borderRadius: 10, border: "1px dashed var(--ee-border-strong)", textAlign: "center", lineHeight: 1.6 } }, t("workbenchView.boardEmpty"));
  } else {
    body = h("div", { style: { display: "grid", gap: 10 } },
      ...board.blocks.map((block, b) => {
        if (block.type === "text") return h("div", { key: b, style: { whiteSpace: "pre-wrap", fontSize: 12.5, lineHeight: 1.7 } }, block.text);
        if (block.type === "list") return h("ul", { key: b, style: { margin: 0, paddingLeft: 18, fontSize: 12.5, lineHeight: 1.7 } }, ...block.items.map((item, i) => h("li", { key: i }, item)));
        if (block.type === "citations") return citationsView(h, block.items, b, openCitation, citationHref);
        return h("div", { key: b, style: { overflowX: "auto", border: "1px solid var(--ee-border)", borderRadius: 8 } },
          h("table", { style: { width: "100%", borderCollapse: "collapse", fontSize: 12.5 } },
            h("thead", null, h("tr", null, ...block.columns.map((column, c) => h("th", { key: c, style: th }, column)))),
            h("tbody", null, ...block.rows.map((row, r) => h("tr", { key: r, className: "dshee-row" }, ...row.map((value, c) => h("td", { key: c, style: { ...td, whiteSpace: "normal", maxWidth: "none", borderBottom: r === block.rows.length - 1 ? 0 : td.borderBottom } }, value)))))));
      }),
      h("div", { style: { ...muted, fontSize: 11 } }, t("workbenchView.updatedAt", { time: new Date(board.updatedAt).toLocaleString(localeTag()) })));
  }
  const badge = h("span", { style: { fontSize: 10.5, fontWeight: 700, letterSpacing: "0.04em", color: "#fff", padding: "1px 7px", borderRadius: 999, background: "linear-gradient(135deg, var(--ee-accent), #8b5cf6)" } }, "AI");
  return h("section", { className: "dshee-card", style: { ...card, borderTop: "2px solid color-mix(in srgb, var(--ee-accent) 55%, transparent)" } }, header(h, widget, undefined, badge), body);
}

export interface PanelState {
  data: Record<number, WidgetState>;
  board?: Board | null;
  notice?: string;
}

/** 没写宽度时按类型排：连续的几个指标平分一行（最多 4 个一行），其余占满。 */
export function widgetWidths(widgets: WorkbenchWidget[]): WidgetWidth[] {
  const out: WidgetWidth[] = widgets.map((w) => w.width ?? "full");
  for (let i = 0; i < widgets.length;) {
    if (widgets[i].type !== "metric" || widgets[i].width) { i++; continue; }
    let j = i;
    while (j < widgets.length && widgets[j].type === "metric" && !widgets[j].width) j++;
    const run = j - i;
    for (let k = i; k < j; k++) {
      const left = run - Math.floor((k - i) / 4) * 4;
      const n = Math.min(left, 4);
      out[k] = n >= 4 ? "quarter" : n === 3 ? "third" : n === 2 ? "half" : "full";
    }
    i = j;
  }
  return out;
}

/** 一个业务面板窗格：12 栏网格的仪表盘，宽面板上按 width 并排，窄面板上叠成一列。 */
export function panelView(h: H, pane: WorkbenchPane, state: PanelState, handlers: PanelHandlers) {
  const widgets = pane.widgets ?? [];
  const widths = widgetWidths(widgets);
  const render = (widget: WorkbenchWidget, index: number) => {
    switch (widget.type) {
      case "metric": return metricView(h, widget, state.data[index], () => handlers.refresh(index), handlers, index);
      case "table": return tableView(h, index, widget, state.data[index], handlers);
      case "chart": return chartView(h, index, widget, state.data[index], handlers);
      case "checklist": return checklistView(h, index, widget, state.data[index], handlers);
      case "form": return formView(h, index, widget, handlers);
      case "actions": return actionsView(h, index, widget, handlers);
      case "markdown": return markdownView(h, widget);
      case "heading": return headingView(h, widget);
      case "board": return boardView(h, widget, state.board, handlers.openCitation, handlers.citationHref);
      case "kb_scope": return kbScopeView(h, widget, state.data[index], handlers, index);
      default: return null;
    }
  };
  return h("div", { "data-dsh-ee-ui": "workbench", className: "dshee-root", style: { padding: 14, fontSize: 13, overflowY: "auto", height: "100%", boxSizing: "border-box" } },
    h("style", { key: "css" }, panelCss),
    state.notice ? h("p", { role: "status", style: { margin: "0 0 12px", fontSize: 12, padding: "8px 12px", borderRadius: 8, background: "color-mix(in srgb, var(--ee-accent) 9%, transparent)", border: "1px solid color-mix(in srgb, var(--ee-accent) 22%, transparent)", overflowWrap: "anywhere" } }, state.notice) : null,
    h("div", { className: "dshee-grid" },
      ...widgets.map((widget, index) => {
        const width = widget.type === "heading" ? "full" : widths[index];
        // 窄面板里指标默认两个一行；明确写了 full 的指标照样占满
        const narrow = widget.type === "metric" && width !== "full" ? " dshee-w-metric" : "";
        return h("div", { key: `w${index}`, className: `dshee-w dshee-w-${width}${narrow}`, "data-widget": widget.type }, render(widget, index));
      })));
}

/** 地址对应的应用或窗格已经不在目录里（被停用、改了配置）。 */
export function missingPaneView(h: H) {
  return h("div", { style: { padding: 16, fontSize: 13, opacity: 0.7 } }, t("workbenchView.unavailable"));
}

/** 「原文」窗格要看的那条引用。 */
export interface DocTarget {
  doc: number;
  version?: number;
  from: number;
  to: number;
  quote?: string;
  title?: string;
}

export interface DocPaneHandlers {
  /** 往前、往后翻一页（-1 / 1） */
  page: (delta: number) => void;
  ask: () => void;
  asking: boolean;
  /** 看「原文」（带行号的 Markdown）还是「阅读」视图；不传就只有阅读视图 */
  raw?: boolean;
  toggleRaw?: () => void;
}

/** 原文视图：等宽字体、左边是行号，引用的几行底色高亮。 */
function rawDocLines(h: H, lines: { n: number; text: string }[], target: DocTarget) {
  let marked = false;
  return h("div", { style: { fontFamily: "ui-monospace, SFMono-Regular, Menlo, monospace", fontSize: 12, lineHeight: 1.6 } },
    ...lines.map((line) => {
      const hit = line.n >= target.from && line.n <= target.to;
      const first = hit && !marked;
      if (first) marked = true;
      return h("div", { key: line.n, "data-kb-hit": first ? "1" : undefined, style: { display: "grid", gridTemplateColumns: "3.5em 1fr", gap: 8, background: hit ? "color-mix(in srgb, #facc15 28%, transparent)" : "transparent", borderRadius: 3 } },
        h("span", { style: { textAlign: "right", opacity: 0.45, userSelect: "none" } }, String(line.n)),
        h("span", { style: { whiteSpace: "pre-wrap", overflowWrap: "anywhere" } }, line.text || " "));
    }));
}

/**
 * 知识库原文：默认是阅读视图（标题、表格正常排版，引用的段落左边一条竖线、摘句精确标出来），可以切到带行号的原文。
 * data-kb-hit 标在第一处高亮上，方便滚过去。
 */
export function kbDocView(h: H, target: DocTarget | undefined, state: WidgetState | undefined, handlers: DocPaneHandlers) {
  const wrap = (...children: any[]) => h("div", { "data-dsh-ee-ui": "kb-doc", style: { display: "grid", gridTemplateColumns: "minmax(0, 1fr)", gap: 10, padding: 12, fontSize: 13, overflowY: "auto", height: "100%", boxSizing: "border-box", alignContent: "start" } }, ...children);
  if (!target) return wrap(h("div", { style: muted }, t("workbenchView.openCitationHint")));
  const note = stateNote(h, state);
  if (note) return wrap(h("strong", null, target.title || t("workbenchClient.docLabel", { doc: target.doc })), note);
  const value = (state as any).value ?? {};
  const lines = parseDocLines(value.text);
  const total = Number(value.total_lines) || 0;
  const from = Number(value.from) || 1;
  const to = Number(value.to) || from;
  const highlight = { from: target.from, to: target.to, quote: target.quote };
  let body: any;
  let quoteFound = true;
  if (handlers.raw) {
    body = rawDocLines(h, lines, target);
  } else {
    const blocks = parseBlocks(lines);
    const marks = computeMarks(blocks, highlight);
    quoteFound = !target.quote || [...marks.values()].some((m) => m.ranges.length > 0);
    body = docView(h, blocks, marks, { format: value.format, highlight });
  }
  return wrap(
    h("div", { style: { display: "flex", alignItems: "baseline", justifyContent: "space-between", gap: 8 } },
      h("strong", { style: { fontSize: 14 } }, value.title || target.title || t("workbenchClient.docLabel", { doc: target.doc })),
      h("span", { style: { ...muted, whiteSpace: "nowrap" } }, t("workbenchView.versionLine", { version: value.version ?? "?", from, to, total }))),
    value.stale ? h("div", { role: "status", style: { fontSize: 12, color: "#a16207" } }, t("workbenchView.staleNotice", { stale: value.stale })) : null,
    h("div", { style: { display: "flex", gap: 6, flexWrap: "wrap" } },
      button(h, t("workbenchView.prevPage"), () => handlers.page(-1), from <= 1, "prev"),
      button(h, t("workbenchView.nextPage"), () => handlers.page(1), total > 0 && to >= total, "next"),
      button(h, t("workbenchClient.ask"), handlers.ask, handlers.asking, "ask"),
      handlers.toggleRaw ? button(h, handlers.raw ? t("workbenchView.readView") : t("workbenchView.rawView"), handlers.toggleRaw, false, "raw") : null),
    body,
    target.quote && (handlers.raw || !quoteFound) ? h("div", { style: muted }, t("workbenchView.quotedSource", { quote: target.quote })) : null);
}
