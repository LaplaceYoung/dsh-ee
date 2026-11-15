import type { CatalogApp, CenterCategory, CenterConnection, CenterData, CenterSkill } from "./skills.js";
import { appTileIcon } from "./apps-view.js";

/**
 * 「专家 · 技能 · 连接器」面板（docs/expert-center.md §7）：右侧栏一个标签页，三个 Tab，
 * 顶部搜索与分类筛选，精选排在最前。这里只画界面，动作由 center-client.ts 接到后端。
 */

export type CenterTab = "experts" | "skills" | "connections";

export interface CenterViewState {
  data?: CenterData;
  loggedIn: boolean;
  /** 还没拿到过数据 */
  loading?: boolean;
  tab: CenterTab;
  query: string;
  /** 0 表示全部分类 */
  category: number;
  /** 展开详情的专家名 */
  open?: string;
  busy?: boolean;
  notice?: string;
}

export interface CenterViewActions {
  login: () => void;
  refresh: () => void;
  setTab: (tab: CenterTab) => void;
  search: (query: string) => void;
  setCategory: (id: number) => void;
  toggleOpen: (name: string) => void;
  /** 召唤专家：新会话里加载它 */
  summon: (app: CatalogApp) => void;
  /** 新会话里加载专家，并带着这条任务示例开始 */
  startWith: (app: CatalogApp, example: string) => void;
  /** 启用或停用一项自选能力 */
  toggle: (kind: "apps" | "skills" | "connections", name: string, enabled: boolean, title: string) => void;
  /** 打开网页（详情、授权、申请权限） */
  web: (url: string) => void;
}

const groupOf: Record<CenterTab, CenterCategory["group"]> = { experts: "app", skills: "skill", connections: "connection" };
const tabLabels: Record<CenterTab, string> = { experts: "专家", skills: "技能", connections: "连接器" };

const plainButton = { border: 0, background: "transparent", color: "inherit", cursor: "pointer", padding: 0, font: "inherit", textAlign: "left" as const };
const chip = (active: boolean) => ({
  ...plainButton,
  padding: "3px 9px",
  borderRadius: 999,
  fontSize: 12,
  whiteSpace: "nowrap" as const,
  border: "1px solid var(--color-border, rgba(127,127,127,.28))",
  background: active ? "var(--color-primary, #4d6bfe)" : "transparent",
  color: active ? "#fff" : "inherit",
});
const smallButton = (primary = false) => ({
  ...plainButton,
  padding: "3px 10px",
  borderRadius: 6,
  fontSize: 12,
  whiteSpace: "nowrap" as const,
  border: primary ? 0 : "1px solid var(--color-border, rgba(127,127,127,.28))",
  background: primary ? "var(--color-primary, #4d6bfe)" : "transparent",
  color: primary ? "#fff" : "inherit",
});
const tag = (tone: "brand" | "green" | "amber" | "red" | "plain" = "plain") => ({
  display: "inline-block",
  padding: "0 6px",
  borderRadius: 4,
  fontSize: 11,
  lineHeight: "18px",
  whiteSpace: "nowrap" as const,
  background: { brand: "rgba(77,107,254,.12)", green: "rgba(22,163,74,.12)", amber: "rgba(245,158,11,.16)", red: "rgba(217,45,32,.12)", plain: "rgba(127,127,127,.12)" }[tone],
});
const card = { padding: "10px 12px", borderRadius: 10, border: "1px solid var(--color-border, rgba(127,127,127,.22))", display: "flex", flexDirection: "column" as const, gap: 6 };
const muted = { fontSize: 12, opacity: 0.7, lineHeight: 1.5 };

type Item = { title: string; name: string; description: string; featured?: boolean; enabled?: boolean; category_id?: number; expertise?: string[] };

/** 精选在前，其次已生效的，再按标题。 */
export function sortItems<T extends Item>(items: T[]): T[] {
  return [...items].sort((a, b) => Number(!!b.featured) - Number(!!a.featured) || Number(!!b.enabled) - Number(!!a.enabled) || a.title.localeCompare(b.title, "zh"));
}

/** 按搜索词与分类过滤。 */
export function filterItems<T extends Item>(items: T[], query: string, category: number): T[] {
  const kw = query.trim().toLowerCase();
  return items.filter((x) => (!category || x.category_id === category) && (!kw || `${x.title} ${x.name} ${x.description} ${(x.expertise ?? []).join(" ")}`.toLowerCase().includes(kw)));
}

/** 某个 Tab 下的条目数（标签上的数字）。 */
export function tabCount(data: CenterData | undefined, tab: CenterTab): number {
  if (!data) return 0;
  if (tab === "experts") return data.experts.length + data.availableExperts.length;
  return tab === "skills" ? data.skills.length : data.connections.length;
}

function expertCard(h: any, app: CatalogApp, enabled: boolean, state: CenterViewState, actions: CenterViewActions) {
  const open = state.open === app.name;
  const examples = app.starters ?? [];
  return h("div", { key: app.name, style: card, "data-dsh-ee-ui": "center-expert" },
    h("div", { style: { display: "flex", gap: 10, alignItems: "flex-start" } },
      appTileIcon(h, app, 34),
      h("div", { style: { minWidth: 0, flex: 1 } },
        h("button", { type: "button", style: { ...plainButton, fontWeight: 600, fontSize: 13 }, onClick: () => actions.toggleOpen(app.name), "aria-expanded": open }, app.title),
        h("div", { style: { ...muted, overflow: "hidden", textOverflow: "ellipsis", display: "-webkit-box", WebkitLineClamp: open ? undefined : 2, WebkitBoxOrient: "vertical" } }, app.description)),
      (app.members?.length ?? 0) > 0 ? h("span", { style: tag("brand"), title: app.members!.join("、") }, `团队 ${app.members!.length + 1} 人`) : null,
      app.featured ? h("span", { style: tag("amber") }, "精选") : null),
    (app.expertise?.length ?? 0) > 0
      ? h("div", { style: { display: "flex", flexWrap: "wrap", gap: 4 } }, ...app.expertise!.map((x) => h("span", { key: x, style: tag("brand") }, x)))
      : null,
    open && examples.length > 0
      ? h("div", { style: { display: "flex", flexDirection: "column", gap: 4 } },
          h("div", { style: { ...muted, fontWeight: 600 } }, "任务示例"),
          ...examples.map((ex) => h("button", { key: ex, type: "button", disabled: !enabled, title: enabled ? "带着这条示例开始" : "先启用", style: { ...plainButton, fontSize: 12, padding: "4px 8px", borderRadius: 6, background: "rgba(127,127,127,.08)", opacity: enabled ? 1 : 0.6 }, onClick: () => actions.startWith(app, ex) }, ex)))
      : null,
    open && (app.missing?.length ?? 0) > 0
      ? h("div", { style: { ...muted, color: "#b54708" } }, `部分能力你还没有权限：${app.missing!.join("、")}`, " ",
          app.detail_url ? h("button", { type: "button", style: { ...plainButton, textDecoration: "underline" }, onClick: () => actions.web(app.detail_url!) }, "申请使用") : null)
      : null,
    h("div", { style: { display: "flex", gap: 6, flexWrap: "wrap", alignItems: "center" } },
      enabled
        ? h("button", { type: "button", style: smallButton(true), disabled: state.busy, onClick: () => actions.summon(app) }, "召唤")
        : h("button", { type: "button", style: smallButton(true), disabled: state.busy, onClick: () => actions.toggle("apps", app.name, true, app.title) }, "启用"),
      enabled && app.delivery === "optional"
        ? h("button", { type: "button", style: smallButton(), disabled: state.busy, onClick: () => actions.toggle("apps", app.name, false, app.title) }, "停用")
        : null,
      h("button", { type: "button", style: smallButton(), onClick: () => actions.toggleOpen(app.name) }, open ? "收起" : "详情"),
      app.detail_url && open ? h("button", { type: "button", style: smallButton(), onClick: () => actions.web(app.detail_url!) }, "在网页查看") : null));
}

function skillCard(h: any, k: CenterSkill, state: CenterViewState, actions: CenterViewActions) {
  return h("div", { key: k.name, style: card, "data-dsh-ee-ui": "center-skill" },
    h("div", { style: { display: "flex", gap: 6, alignItems: "center" } },
      h("span", { style: { fontWeight: 600, fontSize: 13, flex: 1, minWidth: 0, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" } }, k.title),
      k.featured ? h("span", { style: tag("amber") }, "精选") : null,
      h("span", { style: tag(k.enabled ? "green" : "plain") }, k.enabled ? (k.delivery === "auto" ? "默认生效" : "已启用") : "未启用")),
    h("div", { style: muted }, k.description),
    h("div", { style: { display: "flex", gap: 6, alignItems: "center", flexWrap: "wrap" } },
      h("code", { style: { fontSize: 11, opacity: 0.7 } }, k.kind === "prompt" ? `/${k.name}` : k.name),
      h("span", { style: { flex: 1 } }),
      k.delivery === "optional"
        ? h("button", { type: "button", style: smallButton(!k.enabled), disabled: state.busy, onClick: () => actions.toggle("skills", k.name, !k.enabled, k.title) }, k.enabled ? "停用" : "启用")
        : null,
      k.detail_url ? h("button", { type: "button", style: smallButton(), onClick: () => actions.web(k.detail_url!) }, "预览") : null));
}

function connectionCard(h: any, c: CenterConnection, state: CenterViewState, actions: CenterViewActions) {
  const needsAuth = c.enabled && c.auth === "needs_auth";
  return h("div", { key: c.name, style: card, "data-dsh-ee-ui": "center-connection" },
    h("div", { style: { display: "flex", gap: 6, alignItems: "center" } },
      h("span", { style: { fontWeight: 600, fontSize: 13, flex: 1, minWidth: 0, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" } }, c.title),
      c.featured ? h("span", { style: tag("amber") }, "精选") : null,
      needsAuth
        ? h("span", { style: tag("red") }, "待授权")
        : h("span", { style: tag(c.enabled ? "green" : "plain") }, c.enabled ? (c.delivery === "auto" ? "默认生效" : "已启用") : "未启用")),
    h("div", { style: muted }, c.description),
    h("div", { style: { display: "flex", gap: 6, alignItems: "center", flexWrap: "wrap" } },
      h("span", { style: tag() }, `${c.tools} 个工具`),
      h("span", { style: tag(c.writes ? "amber" : "plain") }, c.writes ? "可读写" : "只读"),
      h("span", { style: { flex: 1 } }),
      needsAuth && c.detail_url
        ? h("button", { type: "button", style: smallButton(true), onClick: () => actions.web(c.detail_url!) }, "去授权")
        : null,
      c.delivery === "optional"
        ? h("button", { type: "button", style: smallButton(!c.enabled && !needsAuth), disabled: state.busy, onClick: () => actions.toggle("connections", c.name, !c.enabled, c.title) }, c.enabled ? "停用" : "启用")
        : null,
      !needsAuth && c.detail_url ? h("button", { type: "button", style: smallButton(), onClick: () => actions.web(c.detail_url!) }, "详情") : null),
    needsAuth ? h("div", { style: { ...muted, color: "#b54708" } }, c.auth_mode === "oauth" ? "要先用你自己的账号授权，授权后回来点刷新。" : "要先在网页上填你自己的凭据，填好后回来点刷新。") : null);
}

/** 整个面板。 */
export function centerPanel(h: any, state: CenterViewState, actions: CenterViewActions) {
  const root = (...children: any[]) => h("div", { "data-dsh-ee-ui": "center", style: { display: "flex", flexDirection: "column", gap: 10, padding: "12px 12px 20px", fontSize: 13 } }, ...children);
  if (!state.loggedIn) {
    return root(
      h("div", { style: muted }, "登录企业账号后，可以在这里召唤专家、启用技能和连接企业系统。"),
      h("button", { type: "button", style: smallButton(true), onClick: actions.login }, "登录"),
      state.notice ? h("p", { role: "status", style: muted }, state.notice) : null);
  }
  const data = state.data;
  if (!data) return root(h("div", { style: muted }, state.loading ? "正在读取…" : "读取失败，请检查网络后刷新。"), h("button", { type: "button", style: smallButton(), onClick: actions.refresh }, "刷新"));
  const tabs: CenterTab[] = data.legacy ? ["experts"] : ["experts", "skills", "connections"];
  const tab = tabs.includes(state.tab) ? state.tab : "experts";
  const cats = data.categories.filter((c) => c.group === groupOf[tab]).sort((a, b) => a.sort - b.sort || a.name.localeCompare(b.name, "zh"));
  const category = cats.some((c) => c.id === state.category) ? state.category : 0;

  let list: any[];
  if (tab === "experts") {
    const enabled = new Set(data.experts.map((x) => x.name));
    const all = [...data.experts.map((x) => ({ ...x, enabled: true })), ...data.availableExperts.map((x) => ({ ...x, enabled: false }))];
    list = sortItems(filterItems(all, state.query, category)).map((x) => expertCard(h, x, enabled.has(x.name), state, actions));
  } else if (tab === "skills") {
    list = sortItems(filterItems(data.skills, state.query, category)).map((k) => skillCard(h, k, state, actions));
  } else {
    list = sortItems(filterItems(data.connections, state.query, category)).map((c) => connectionCard(h, c, state, actions));
  }
  const pending = tab === "connections" ? data.connections.filter((c) => c.enabled && c.auth === "needs_auth").length : 0;

  return root(
    h("div", { role: "tablist", style: { display: "flex", gap: 4, borderBottom: "1px solid var(--color-border, rgba(127,127,127,.22))", paddingBottom: 6 } },
      ...tabs.map((k) => h("button", {
        key: k, type: "button", role: "tab", "aria-selected": k === tab, onClick: () => actions.setTab(k),
        style: { ...plainButton, padding: "4px 8px", borderRadius: 6, fontWeight: k === tab ? 600 : 400, background: k === tab ? "rgba(127,127,127,.12)" : "transparent" },
      }, `${tabLabels[k]} ${tabCount(data, k)}`)),
      h("span", { style: { flex: 1 } }),
      h("button", { type: "button", title: "刷新", "aria-label": "刷新", style: { ...plainButton, fontSize: 12, opacity: 0.7 }, onClick: actions.refresh }, "刷新")),
    h("input", {
      type: "search", value: state.query, placeholder: `搜索${tabLabels[tab]}…`, "aria-label": `搜索${tabLabels[tab]}`,
      onChange: (e: any) => actions.search(e?.target?.value ?? ""),
      style: { width: "100%", boxSizing: "border-box", padding: "6px 9px", borderRadius: 7, border: "1px solid var(--color-border, rgba(127,127,127,.3))", background: "transparent", color: "inherit", font: "inherit", fontSize: 12 },
    }),
    cats.length > 0
      ? h("div", { style: { display: "flex", gap: 6, overflowX: "auto", paddingBottom: 2 } },
          h("button", { type: "button", style: chip(category === 0), onClick: () => actions.setCategory(0) }, "全部"),
          ...cats.map((c) => h("button", { key: c.id, type: "button", style: chip(category === c.id), onClick: () => actions.setCategory(c.id) }, c.name)))
      : null,
    pending > 0 ? h("div", { style: { ...muted, color: "#b54708" } }, `有 ${pending} 个连接器要先授权才能用。`) : null,
    state.notice ? h("p", { role: "status", style: { ...muted, margin: 0 } }, state.notice) : null,
    list.length > 0
      ? h("div", { style: { display: "flex", flexDirection: "column", gap: 8 } }, ...list)
      : h("div", { style: muted }, state.query || category ? "没有匹配的条目。" : tab === "experts" ? "还没有可用的专家。" : tab === "skills" ? "还没有可用的技能。" : "还没有可用的连接器。"));
}
