import type { CatalogApp } from "./skills.js";
import { t } from "./i18n/index.js";
import type { AppSessionStatus } from "./session-status.js";

export interface AppsViewState {
  apps: CatalogApp[];
  loggedIn: boolean;
  /** 整个「企业应用」区块收起，只剩一行标题 */
  collapsed?: boolean;
  /** 「查看全部」应用选择浮层是否打开 */
  galleryOpen?: boolean;
  /** 当前展开「⋯」菜单的应用名；空串表示都关着 */
  menuFor?: string;
  /** 「⋯」菜单的屏幕锚点（position: fixed），由点击位置算出来 */
  menuAnchor?: MenuAnchor;
  query?: string;
  busy?: boolean;
  notice?: string;
  statuses?: Record<string, AppSessionStatus | undefined>;
  /** 范围内、自选、还没启用的微应用 */
  available?: CatalogApp[];
  availableOpen?: boolean;
  /** 已点过一次「停用」、等待确认的应用 */
  confirmDisable?: string;
  /** 侧栏里最多直接列几个，其余进「查看更多」；默认 3 */
  limit?: number;
  /** 侧栏的「企业应用」区块能不能隐藏（右侧企业面板里的那份不能） */
  hideable?: boolean;
  /** 左侧栏现在是不是隐藏着（应用选择浮层里的开关用） */
  sidebarHidden?: boolean;
}

/** 「⋯」菜单用 fixed 定位：up 为真时锚在按钮上方（bottom），否则下方（top）；right 是距窗口右缘的像素。 */
export interface MenuAnchor {
  up: boolean;
  right: number;
  top?: number;
  bottom?: number;
}

export interface AppsViewActions {
  login: () => void;
  open: (app: CatalogApp) => void;
  fill: (app: CatalogApp) => void;
  web: (app: CatalogApp) => void;
  /** 打开网页上的应用详情：看缺哪些权限、一键申请 */
  details?: (app: CatalogApp) => void;
  newSession: (app: CatalogApp) => void;
  openWorkbench?: (app: CatalogApp) => void;
  /** 启用一个自选微应用 */
  enable?: (app: CatalogApp) => void;
  /** 第一次点「停用」只进入确认状态；confirmed 为真才真正停用 */
  disable?: (app: CatalogApp, confirmed: boolean) => void;
  toggleAvailable?: () => void;
  /** 新建会话并带着这条示例问题开始 */
  startWith?: (app: CatalogApp, starter: string) => void;
  unbind: (app: CatalogApp) => void;
  /** 收起/展开「企业应用」区块 */
  toggleCollapsed?: () => void;
  /** 开关「查看全部」应用选择浮层 */
  toggleGallery?: () => void;
  /** 开关某个应用的「⋯」菜单；event 用来算菜单位置 */
  toggleMenu?: (name: string, event?: any) => void;
  /** 关掉所有浮层（菜单、应用选择），菜单项点完都要走这里 */
  closeLayers?: () => void;
  /** 在左侧栏隐藏 / 显示「企业应用」区块（隐藏后在「查看更多」和 设置 → dsh-ee 里能再打开） */
  setSidebarHidden?: (hidden: boolean) => void;

  search: (query: string) => void;
}

const buttonStyle = { border: 0, background: "transparent", color: "inherit", cursor: "pointer", padding: 0, textAlign: "left" as const };

/** 按应用名给图标配一个底色：同一个应用每次都一样，亮色 / 暗色主题下都只是淡淡一层。 */
const tileHues = ["#4d6bfe", "#00a3bf", "#16a34a", "#f59e0b", "#db2777", "#8b5cf6", "#0ea5e9", "#ef4444"];
export function tileHue(name: string): string {
  let hash = 0;
  for (const ch of name) hash = (hash * 31 + ch.charCodeAt(0)) >>> 0;
  return tileHues[hash % tileHues.length];
}

/** 卡片上的大图标：圆角方块、按应用名配的淡色底。 */
export function appTileIcon(createElement: (...args: any[]) => any, app: CatalogApp, size = 40) {
  const hue = tileHue(app.name);
  return createElement("span", {
    "aria-hidden": true,
    style: { width: size, height: size, flex: `0 0 ${size}px`, display: "inline-flex", alignItems: "center", justifyContent: "center", borderRadius: Math.round(size * 0.28), fontSize: Math.round(size * 0.5), background: `linear-gradient(135deg, color-mix(in srgb, ${hue} 20%, transparent), color-mix(in srgb, ${hue} 7%, transparent))`, boxShadow: `inset 0 0 0 1px color-mix(in srgb, ${hue} 18%, transparent)` },
  }, app.icon || Array.from(app.title)[0] || "?");
}

export function appIcon(createElement: (...args: any[]) => any, app: CatalogApp) {
  return createElement("span", {
    "aria-hidden": true,
    style: { width: 30, height: 30, flex: "0 0 30px", display: "inline-flex", alignItems: "center", justifyContent: "center", borderRadius: "50%", background: "color-mix(in srgb, currentColor 10%, transparent)", fontSize: 16 },
  }, app.icon || Array.from(app.title)[0] || "?");
}

export function appTile(createElement: (...args: any[]) => any, app: CatalogApp, onOpen: () => void) {
  return createElement("button", {
    type: "button", onClick: onOpen, title: app.description,
    style: { ...buttonStyle, display: "flex", alignItems: "center", gap: 9, minWidth: 0, flex: 1, padding: "5px 0" },
  },
  appIcon(createElement, app),
  createElement("span", { style: { minWidth: 0, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" } }, app.title));
}

/** 「⋯」菜单里的示例问题：点一条就新建会话并带着它开始；超长的截断显示，悬停看全文。 */
function starterItems(createElement: (...args: any[]) => any, app: CatalogApp, actions: AppsViewActions, close: () => void) {
  const starters = (app.starters ?? []).filter(Boolean).slice(0, 6);
  if (!starters.length || !actions.startWith) return [];
  return [
    createElement("div", { key: "starters-title", style: { padding: "7px 7px 3px", fontSize: 11, opacity: 0.6, borderTop: "1px solid color-mix(in srgb, currentColor 12%, transparent)", marginTop: 4 } }, t("appsView.startersTitle")),
    ...starters.map((starter, index) => createElement("button", {
      key: `starter-${index}`, type: "button", title: starter,
      style: { ...buttonStyle, display: "block", width: "100%", padding: 7, maxWidth: 260, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" },
      onClick: () => { close(); actions.startWith!(app, starter); },
    }, starter)),
  ];
}

function disableItem(createElement: (...args: any[]) => any, app: CatalogApp, actions: AppsViewActions, confirming: boolean, close: () => void) {
  if (app.delivery !== "optional" || !actions.disable) return null;
  return createElement("button", {
    type: "button",
    style: { ...buttonStyle, display: "block", width: "100%", padding: 7, color: confirming ? "#b91c1c" : "inherit" },
    onClick: () => { close(); actions.disable!(app, confirming); },
  }, confirming ? t("appsView.confirmDisable") : t("appsView.disable"));
}

/**
 * 「⋯」菜单：受控开合。菜单用 fixed 定位贴着按钮（窗口下沿附近时向上展开），
 * 配一层全屏透明背板，点空白处或 Escape 都能关；超长内容内部滚动，不会顶出窗口。
 */
function appMenu(createElement: (...args: any[]) => any, app: CatalogApp, actions: AppsViewActions, confirming = false, open = false, anchor?: MenuAnchor) {
  const close = () => actions.closeLayers?.() ?? actions.toggleMenu?.(app.name);
  const item = (key: string, label: string, run: () => void) =>
    createElement("button", { key, type: "button", style: { ...buttonStyle, display: "block", width: "100%", padding: 7 }, onClick: () => { close(); run(); } }, label);
  const layer = open ? [
    createElement("div", { key: "backdrop", "data-dsh-ee-ui": "app-menu-backdrop", onClick: close, style: { position: "fixed", inset: 0, zIndex: 80 } }),
    createElement("div", {
      key: "menu", role: "menu", "data-dsh-ee-ui": "app-menu",
      style: {
        position: "fixed", zIndex: 90, right: anchor?.right ?? 8,
        top: anchor?.up === false ? anchor.top ?? 0 : undefined,
        bottom: anchor?.up === false ? undefined : anchor?.bottom ?? 64,
        minWidth: 150, maxWidth: "min(280px, calc(100vw - 16px))", maxHeight: "min(340px, calc(100vh - 16px))", overflowY: "auto",
        padding: 6, borderRadius: 7, border: "1px solid color-mix(in srgb, currentColor 15%, transparent)", background: "Canvas", color: "CanvasText", boxShadow: "0 5px 20px #0002",
      },
    },
      item("new-session", t("appsView.newSession"), () => actions.newSession(app)),
      app.workbench?.panes?.length && actions.openWorkbench ? item("workbench", t("appsView.openWorkbench"), () => actions.openWorkbench!(app)) : null,
      item("fill", t("appsView.fillInput"), () => actions.fill(app)),
      item("web", t("common.viewInWeb"), () => actions.web(app)),
      item("unbind", t("appsView.unbind"), () => actions.unbind(app)),
      disableItem(createElement, app, actions, confirming, close),
      ...starterItems(createElement, app, actions, close)),
  ] : null;
  return createElement("div", { style: { position: "relative", flex: "0 0 auto" } },
    createElement("button", {
      type: "button", "aria-label": t("appsView.moreActions", { title: app.title }), "aria-expanded": open, "aria-haspopup": "menu",
      onClick: (event: any) => { event?.stopPropagation?.(); actions.toggleMenu?.(app.name, event); },
      style: { ...buttonStyle, padding: "4px 6px" },
    }, "⋯"),
    layer);
}

/** 「部分能力不可用」：能打开网页详情时是按钮，点开看缺什么、一键申请。 */
function missingBadge(createElement: (...args: any[]) => any, app: CatalogApp, actions: AppsViewActions) {
  if (!app.missing?.length) return null;
  const style = { fontSize: 11, color: "#d97706", whiteSpace: "nowrap" };
  const title = t("appsView.missingList", { missing: app.missing.join(t("common.listSeparator")) });
  if (!actions.details || !app.detail_url) return createElement("span", { title, style }, t("appsView.missing"));
  return createElement("button", {
    type: "button", "data-dsh-ee-missing": app.name, title: t("appsView.missingHint", { title }),
    onClick: (event: any) => { event?.stopPropagation?.(); actions.closeLayers?.(); actions.details!(app); },
    style: { ...buttonStyle, ...style, padding: 0, textDecoration: "underline" },
  }, t("appsView.missingApply"));
}

const statusViewOf = (status?: AppSessionStatus) => status === "need" ? { label: t("common.pendingDecision"), color: "#d97706" } : status === "busy" ? { label: t("common.working"), color: "#2563eb" } : status === "done" ? { label: t("common.turnEnded"), color: "#737373" } : undefined;

function appRow(createElement: (...args: any[]) => any, app: CatalogApp, actions: AppsViewActions, status?: AppSessionStatus, confirming = false, menuOpen = false, anchor?: MenuAnchor) {
  const statusView = statusViewOf(status);
  return createElement("div", { key: app.name, "data-dsh-ee-app": app.name, style: { display: "flex", alignItems: "center", gap: 4 } },
    statusView ? createElement("span", { "aria-label": statusView.label, title: statusView.label, style: { width: 8, height: 8, flex: "0 0 8px", borderRadius: "50%", background: statusView.color } }) : null,
    appTile(createElement, app, () => actions.open(app)),
    missingBadge(createElement, app, actions),
    appMenu(createElement, app, actions, confirming, menuOpen, anchor));
}

const matchesQuery = (app: CatalogApp, query: string) => !query || `${app.title} ${app.description} ${app.name}`.toLowerCase().includes(query);

/** 浮层与卡片共用的样式：Stripe 式的细边框、柔和阴影、悬停微微上浮。 */
const galleryCss = [
  ".dshee-gcard{transition:border-color .15s,box-shadow .15s,transform .15s}",
  ".dshee-gcard:hover{border-color:color-mix(in srgb,#4d6bfe 45%,transparent)!important;box-shadow:0 10px 24px -12px color-mix(in srgb,#4d6bfe 45%,transparent),0 2px 6px -2px color-mix(in srgb,currentColor 10%,transparent)!important;transform:translateY(-2px)}",
  ".dshee-gcard:focus-visible{outline:2px solid #4d6bfe;outline-offset:2px}",
  ".dshee-gsearch:focus{outline:none;border-color:#4d6bfe!important;box-shadow:0 0 0 3px color-mix(in srgb,#4d6bfe 18%,transparent)}",
  "@keyframes dshee-pop{from{opacity:0;transform:translateY(8px) scale(.985)}to{opacity:1;transform:none}}",
  "@keyframes dshee-fade{from{opacity:0}to{opacity:1}}",
].join("");

const badgeStyle = (color: string) => ({ fontSize: 10.5, fontWeight: 600, padding: "1px 7px", borderRadius: 999, color, background: `color-mix(in srgb, ${color} 13%, transparent)`, whiteSpace: "nowrap" as const });

/** 卡片右上角的标记：状态、新上线、待处理数。 */
function cardBadges(createElement: (...args: any[]) => any, app: CatalogApp, status?: AppSessionStatus) {
  const statusView = statusViewOf(status);
  const todo = (app as any).todo;
  return [
    statusView ? createElement("span", { key: "status", style: badgeStyle(statusView.color) }, statusView.label) : null,
    todo ? createElement("span", { key: "todo", style: badgeStyle("#2563eb") }, t("appsView.todoBadge", { count: todo })) : null,
    (app as any).new ? createElement("span", { key: "new", style: badgeStyle("#d97706") }, t("common.newBadge")) : null,
  ].filter(Boolean);
}

/** 「查看更多」里的一张应用卡：大图标、标题、两行说明，点卡片打开应用；右上角是状态，右下角是「⋯」菜单。 */
function galleryCard(createElement: (...args: any[]) => any, app: CatalogApp, state: AppsViewState, actions: AppsViewActions) {
  const status = state.statuses?.[app.name];
  const open = () => { actions.closeLayers?.(); actions.open(app); };
  return createElement("div", {
    key: app.name, "data-dsh-ee-app": app.name, className: "dshee-gcard", role: "button", tabIndex: 0, title: app.description,
    onClick: open, onKeyDown: (event: any) => { if (event?.key === "Enter" || event?.key === " ") { event.preventDefault?.(); open(); } },
    style: { position: "relative", display: "flex", flexDirection: "column", gap: 10, padding: 14, borderRadius: 14, border: "1px solid color-mix(in srgb, currentColor 11%, transparent)", background: "color-mix(in srgb, currentColor 1.5%, Canvas)", boxShadow: "0 1px 2px color-mix(in srgb, currentColor 5%, transparent)", cursor: "pointer", minWidth: 0, textAlign: "left" },
  },
    createElement("div", { style: { display: "flex", alignItems: "flex-start", justifyContent: "space-between", gap: 8 } },
      appTileIcon(createElement, app, 40),
      createElement("div", { style: { display: "flex", flexWrap: "wrap", justifyContent: "flex-end", gap: 4 } }, ...cardBadges(createElement, app, status))),
    createElement("div", { style: { minWidth: 0 } },
      createElement("div", { style: { fontSize: 14, fontWeight: 600, letterSpacing: "0.01em", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" } }, app.title),
      createElement("div", { style: { marginTop: 4, fontSize: 12, lineHeight: 1.55, opacity: 0.62, minHeight: 37, overflow: "hidden", display: "-webkit-box", WebkitLineClamp: 2, WebkitBoxOrient: "vertical" } }, app.description || app.starters?.[0] || "")),
    createElement("div", { style: { display: "flex", alignItems: "center", justifyContent: "space-between", gap: 6, marginTop: "auto" }, onClick: (event: any) => event?.stopPropagation?.() },
      app.missing?.length ? missingBadge(createElement, app, actions) : createElement("button", { type: "button", onClick: open, style: { ...buttonStyle, fontSize: 12, fontWeight: 600, color: "#4d6bfe" } }, t("appsView.openArrow")),
      actions.toggleMenu ? appMenu(createElement, app, actions, state.confirmDisable === app.name, state.menuFor === app.name, state.menuAnchor) : null));
}

/** 「可启用」卡片：范围内、还没启用的自选应用，一键启用。 */
function availableCard(createElement: (...args: any[]) => any, app: CatalogApp, actions: AppsViewActions) {
  return createElement("div", {
    key: app.name, "data-dsh-ee-available": app.name,
    style: { display: "flex", alignItems: "center", gap: 10, padding: 12, borderRadius: 12, border: "1px dashed color-mix(in srgb, currentColor 18%, transparent)", minWidth: 0 },
  },
    appTileIcon(createElement, app, 34),
    createElement("span", { style: { flex: 1, minWidth: 0 } },
      createElement("span", { style: { display: "block", fontSize: 13, fontWeight: 600, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" } }, app.title),
      app.description ? createElement("span", { style: { display: "block", fontSize: 11.5, opacity: 0.6, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }, title: app.description }, app.description) : null),
    createElement("button", { type: "button", onClick: () => actions.enable!(app), style: { ...buttonStyle, border: "1px solid #4d6bfe", color: "#4d6bfe", borderRadius: 8, padding: "3px 10px", fontSize: 12, fontWeight: 600, flex: "0 0 auto" } }, t("appsView.enable")));
}

/**
 * 「查看更多」：居中的应用网格浮层——搜索、卡片网格、可启用的应用，底部可以把左侧栏的企业应用隐藏 / 显示回来。
 * 侧栏和首屏共用；点背板、✕ 或 Escape 关闭。
 */
export function appsGallery(createElement: (...args: any[]) => any, state: AppsViewState, actions: AppsViewActions, filtered: CatalogApp[]) {
  if (!state.galleryOpen) return null;
  const close = () => actions.closeLayers?.() ?? actions.toggleGallery?.();
  const available = (state.available ?? []).filter((app) => matchesQuery(app, state.query?.trim().toLowerCase() ?? ""));
  const grid = { display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(min(100%, 210px), 1fr))", gap: 12 };
  return createElement("div", { "data-dsh-ee-ui": "apps-gallery", onClick: close, style: { position: "fixed", inset: 0, zIndex: 60, background: "rgba(10, 37, 64, 0.38)", backdropFilter: "blur(3px)", display: "flex", alignItems: "center", justifyContent: "center", padding: 20, animation: "dshee-fade .15s ease-out" } },
    createElement("style", null, galleryCss),
    createElement("div", {
      role: "dialog", "aria-modal": "true", "aria-label": t("apps.enterpriseApps"), onClick: (event: any) => event?.stopPropagation?.(),
      style: { width: "min(920px, calc(100vw - 32px))", maxHeight: "min(720px, calc(100vh - 40px))", display: "flex", flexDirection: "column", borderRadius: 18, border: "1px solid color-mix(in srgb, currentColor 12%, transparent)", background: "Canvas", color: "CanvasText", boxShadow: "0 30px 60px -12px rgba(50, 50, 93, 0.3), 0 18px 36px -18px rgba(0, 0, 0, 0.35)", fontSize: 13, overflow: "hidden", animation: "dshee-pop .2s ease-out" },
    },
      createElement("div", { style: { display: "flex", alignItems: "center", gap: 12, flexWrap: "wrap", padding: "18px 22px 14px", borderBottom: "1px solid color-mix(in srgb, currentColor 9%, transparent)" } },
        createElement("div", { style: { flex: "1 1 200px", minWidth: 0 } },
          createElement("div", { style: { fontSize: 17, fontWeight: 650, letterSpacing: "-0.01em" } }, t("apps.enterpriseApps")),
          createElement("div", { style: { fontSize: 12, opacity: 0.6, marginTop: 2 } }, t("appsView.gallerySubtitle", { count: state.apps.length }))),
        createElement("input", { "aria-label": t("appsView.searchAppsAria"), className: "dshee-gsearch", value: state.query ?? "", onChange: (event: any) => actions.search(event.target.value), placeholder: t("appsView.searchAppsPlaceholder"), style: { flex: "1 1 220px", maxWidth: 300, boxSizing: "border-box", height: 34, padding: "0 12px", borderRadius: 9, border: "1px solid color-mix(in srgb, currentColor 16%, transparent)", background: "transparent", color: "inherit", fontSize: 13 } }),
        createElement("button", { type: "button", "aria-label": t("common.close"), onClick: close, style: { ...buttonStyle, fontSize: 16, padding: "4px 8px", opacity: 0.6, borderRadius: 8 } }, "✕")),
      createElement("div", { style: { overflowY: "auto", flex: 1, minHeight: 0, padding: "18px 22px 22px", display: "grid", gap: 20, alignContent: "start" } },
        filtered.length
          ? createElement("div", { style: grid }, ...filtered.map((app) => galleryCard(createElement, app, state, actions)))
          : createElement("div", { style: { padding: "36px 4px", textAlign: "center", opacity: 0.6 } }, t("appsView.noMatches")),
        available.length && actions.enable ? createElement("section", { "data-dsh-ee-ui": "available-apps" },
          createElement("div", { style: { fontSize: 12, fontWeight: 600, opacity: 0.65, margin: "0 0 10px" } }, t("appsView.availableCount", { count: available.length })),
          createElement("div", { style: grid }, ...available.map((app) => availableCard(createElement, app, actions)))) : null),
      actions.setSidebarHidden ? createElement("label", { style: { display: "flex", alignItems: "center", gap: 8, padding: "11px 22px", borderTop: "1px solid color-mix(in srgb, currentColor 9%, transparent)", fontSize: 12, opacity: 0.8, cursor: "pointer" } },
        createElement("input", { type: "checkbox", checked: !state.sidebarHidden, onChange: () => actions.setSidebarHidden!(!state.sidebarHidden), style: { accentColor: "#4d6bfe" } }),
        t("appsView.showInSidebar")) : null));
}

export function appsSection(createElement: (...args: any[]) => any, state: AppsViewState, actions: AppsViewActions) {
  if (!state.loggedIn) {
    return createElement("section", { "data-dsh-ee-ui": "apps", style: { padding: "10px 12px", fontSize: 13 } },
      createElement("div", { style: { marginBottom: 8 } }, t("appsView.loginToSee")),
      createElement("button", { type: "button", onClick: actions.login, style: { ...buttonStyle, border: "1px solid currentColor", borderRadius: 6, padding: "5px 10px" } }, t("common.login")));
  }
  if (!state.apps.length && !(state.available ?? []).length) return null;
  const query = state.query?.trim().toLowerCase() ?? "";
  const filtered = state.apps.filter((app) => matchesQuery(app, query));
  // 侧栏只列前几个（默认 3 个），免得把会话历史顶出去；其余在「查看更多」的网格里
  const shown = state.apps.slice(0, state.limit ?? 3);
  const more = state.apps.length - shown.length;
  // 标题行常驻：收起时区块只占这一行（钉在侧栏底部），展开后才是列表与「查看更多」
  const header = createElement("div", { style: { display: "flex", alignItems: "center", gap: 4, margin: "0 0 5px" } },
    createElement("button", {
      type: "button", onClick: () => actions.toggleCollapsed?.(), "aria-expanded": !state.collapsed,
      title: state.collapsed ? t("appsView.expandApps") : t("appsView.collapseApps"),
      style: { ...buttonStyle, display: "flex", alignItems: "center", gap: 6, flex: 1, minWidth: 0, padding: "2px 0", fontSize: 13, fontWeight: 600 },
    },
      createElement("span", { "aria-hidden": true, style: { fontSize: 10, opacity: 0.7 } }, state.collapsed ? "▸" : "▾"),
      t("apps.enterpriseApps"),
      state.collapsed ? createElement("span", { style: { fontSize: 12, fontWeight: 400, opacity: 0.6 } }, t("appsView.collapsedCount", { count: state.apps.length })) : null),
    state.hideable && actions.setSidebarHidden ? createElement("button", {
      type: "button", onClick: () => actions.setSidebarHidden!(true), title: t("appsView.hideSidebar"),
      style: { ...buttonStyle, fontSize: 11.5, opacity: 0.5, padding: "2px 4px" },
    }, t("common.hide")) : null);
  return createElement("section", { "data-dsh-ee-ui": "apps", style: { padding: "10px 12px", fontSize: 13 } },
    header,
    state.collapsed ? null : [
      createElement("div", { key: "list", style: { display: "grid", gap: 2 } }, ...shown.map((app) => appRow(createElement, app, actions, state.statuses?.[app.name], state.confirmDisable === app.name, state.menuFor === app.name && !state.galleryOpen, state.menuAnchor))),
      createElement("button", { key: "all", type: "button", onClick: () => actions.toggleGallery?.(), style: { ...buttonStyle, display: "flex", alignItems: "center", gap: 6, marginTop: 6, padding: "4px 0", opacity: 0.72, fontSize: 12.5 } },
        createElement("span", { "aria-hidden": true, style: { width: 30, textAlign: "center", fontSize: 14 } }, "⊞"),
        more > 0 ? t("common.viewMoreCount", { n: state.apps.length }) : t("appsView.allApps")),
      (state.available ?? []).length && actions.enable ? createElement("button", { key: "available", type: "button", onClick: () => actions.toggleGallery?.(), style: { ...buttonStyle, display: "block", marginTop: 4, fontSize: 12, opacity: 0.65 } }, t("appsView.availablePlus", { count: state.available!.length })) : null,
      state.notice ? createElement("p", { key: "notice", role: "status", style: { margin: "7px 0 0", lineHeight: 1.5, overflowWrap: "anywhere" } }, state.notice) : null,
    ],
    appsGallery(createElement, state, actions, filtered));
}

export function heroAppsMenu(createElement: (...args: any[]) => any, apps: CatalogApp[], onOpen: (app: CatalogApp) => void, query = "", onSearch: (query: string) => void = () => {}, notice = "", open = false, onToggle: () => void = () => {}) {
  if (!apps.length) return null;
  const normalized = query.trim().toLowerCase();
  const shown = apps.filter((app) => matchesQuery(app, normalized)).slice(0, 8);
  return createElement("div", { "data-dsh-ee-ui": "hero-apps", style: { position: "relative", display: "inline-block" } },
    createElement("button", { type: "button", onClick: onToggle, "aria-expanded": open, "aria-haspopup": "menu", style: { ...buttonStyle, fontSize: 12, padding: "4px 7px", whiteSpace: "nowrap" } }, t("apps.enterpriseApps")),
    open ? [
      createElement("div", { key: "backdrop", "data-dsh-ee-ui": "hero-apps-backdrop", onClick: onToggle, style: { position: "fixed", inset: 0, zIndex: 70 } }),
      createElement("div", {
        key: "menu", role: "menu",
        style: { position: "absolute", right: 0, top: "calc(100% + 6px)", zIndex: 80, width: 280, maxHeight: "min(420px, calc(100vh - 96px))", overflowY: "auto", padding: 8, borderRadius: 8, border: "1px solid color-mix(in srgb, currentColor 15%, transparent)", background: "Canvas", color: "CanvasText", boxShadow: "0 8px 30px #0003" },
      },
        apps.length > 8 ? createElement("input", { "aria-label": t("appsView.searchAppsAria"), value: query, onChange: (event: any) => onSearch(event.target.value), placeholder: t("appsView.heroSearchPlaceholder"), style: { width: "100%", boxSizing: "border-box", marginBottom: 7, padding: 6, borderRadius: 5, border: "1px solid #8886", background: "transparent", color: "inherit" } }) : null,
        createElement("div", { style: { display: "grid", gap: 4 } }, ...shown.map((app) => createElement("button", {
          key: app.name, type: "button", onClick: () => onOpen(app),
          style: { ...buttonStyle, display: "flex", alignItems: "center", gap: 9, padding: 7, borderRadius: 6, width: "100%" },
        },
        appIcon(createElement, app),
        createElement("span", { style: { minWidth: 0 } },
          createElement("span", { style: { display: "block", fontSize: 13 } }, app.title),
          createElement("span", { style: { display: "block", fontSize: 11, opacity: 0.65, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" } }, app.starters[0] || app.description))))),
        notice ? createElement("p", { role: "status", style: { margin: "7px 4px 0", fontSize: 11, lineHeight: 1.5, overflowWrap: "anywhere" } }, notice) : null),
    ] : null);
}

export function startText(app: CatalogApp, starter?: string): string {
  return `/${app.name}${starter ? `\n${starter}` : ""}`;
}

export interface InProgressItem {
  app: CatalogApp;
  sessionId: string;
  status: "need" | "busy";
}

/** 有绑定、并且「待你决定」或「工作中」的应用，待你决定在前；同状态保持原顺序。 */
export function inProgressItems(apps: CatalogApp[], bindings: Record<string, { sessionId: string } | undefined>, statusOf: (sessionId: string) => AppSessionStatus | undefined): InProgressItem[] {
  const items: InProgressItem[] = [];
  for (const app of apps) {
    const sessionId = bindings[app.name]?.sessionId;
    if (!sessionId) continue;
    const status = statusOf(sessionId);
    if (status === "need" || status === "busy") items.push({ app, sessionId, status });
  }
  return items.sort((a, b) => (a.status === b.status ? 0 : a.status === "need" ? -1 : 1));
}

/** 企业面板里的「进行中」：点一项只切到对应会话，不发消息。没有就不显示。 */
export function inProgressView(createElement: (...args: any[]) => any, items: InProgressItem[], onOpen: (item: InProgressItem) => void) {
  if (!items.length) return null;
  return createElement("section", { "data-dsh-ee-ui": "in-progress", style: { padding: "10px 12px", fontSize: 13 } },
    createElement("h2", { style: { margin: "0 0 7px", fontSize: 13 } }, t("common.inProgress")),
    createElement("div", { style: { display: "grid", gap: 2 } }, ...items.map((item) => {
      const label = item.status === "need" ? t("common.pendingDecision") : t("common.working");
      const color = item.status === "need" ? "#d97706" : "#2563eb";
      return createElement("button", {
        key: item.app.name, type: "button", onClick: () => onOpen(item), title: t("appsView.itemTitle", { title: item.app.title, label }),
        style: { ...buttonStyle, display: "flex", alignItems: "center", gap: 9, padding: "5px 0", width: "100%" },
      },
      createElement("span", { "aria-label": label, style: { width: 8, height: 8, flex: "0 0 8px", borderRadius: "50%", background: color } }),
      appIcon(createElement, item.app),
      createElement("span", { style: { flex: 1, minWidth: 0, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" } }, item.app.title),
      createElement("span", { style: { fontSize: 11, color } }, label));
    })));
}

/** 首屏横排的位置：贴在输入框正下方，宽度和输入框一样；maxHeight 是到窗口下沿还剩多少。 */
export interface StripPlace {
  left: number;
  top: number;
  width: number;
  maxHeight: number;
}

export const STRIP_CARD_MIN = 176;
export const STRIP_GAP = 10;

/** 一行放得下几张卡（含最后的「查看更多」）。 */
export function stripSlots(width: number): number {
  return Math.max(2, Math.min(6, Math.floor((width + STRIP_GAP) / (STRIP_CARD_MIN + STRIP_GAP))));
}

/**
 * 首屏：输入框下方一行横排的应用卡，放不下的进最后一张「查看更多」（打开网格浮层）。
 * place 由宿主量出来（输入框的位置），量到之前不显示；窗口太矮（输入框下面放不下一行）时也不显示。
 */
export function heroStrip(createElement: (...args: any[]) => any, apps: CatalogApp[], place: StripPlace | undefined, onOpen: (app: CatalogApp) => void, onMore: () => void, notice = "") {
  if (!apps.length || !place || place.maxHeight < 86) return null;
  const slots = stripSlots(place.width);
  const fits = apps.length <= slots;
  const shown = fits ? apps : apps.slice(0, slots - 1);
  const todos = apps.reduce((n, a) => n + ((a as any).todo ?? 0), 0);
  const tile = { display: "flex", alignItems: "center", gap: 10, minWidth: 0, padding: "10px 12px", borderRadius: 12, border: "1px solid color-mix(in srgb, currentColor 10%, transparent)", background: "color-mix(in srgb, currentColor 1.5%, transparent)", boxShadow: "0 1px 2px color-mix(in srgb, currentColor 5%, transparent)", color: "inherit", cursor: "pointer", textAlign: "left" as const, font: "inherit" };
  const card = (app: CatalogApp) => createElement("button", {
    key: app.name, type: "button", className: "dshee-gcard", onClick: () => onOpen(app), title: app.description || app.title, "data-dsh-ee-app": app.name, style: tile,
  },
    appTileIcon(createElement, app, 34),
    createElement("span", { style: { minWidth: 0, flex: 1 } },
      createElement("span", { style: { display: "flex", alignItems: "center", gap: 5, fontSize: 13, fontWeight: 600 } },
        createElement("span", { style: { overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" } }, app.title),
        (app as any).todo ? createElement("span", { style: badgeStyle("#2563eb") }, String((app as any).todo)) : (app as any).new ? createElement("span", { style: badgeStyle("#d97706") }, t("common.newBadge")) : null),
      createElement("span", { style: { display: "block", marginTop: 2, fontSize: 11.5, opacity: 0.58, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" } }, app.description || app.starters?.[0] || "")));
  const more = fits ? null : createElement("button", {
    key: "@more", type: "button", className: "dshee-gcard", onClick: onMore, "data-dsh-ee-ui": "hero-more",
    style: { ...tile, justifyContent: "center", borderStyle: "dashed", background: "transparent", boxShadow: "none" },
  },
    createElement("span", { "aria-hidden": true, style: { fontSize: 16, color: "#4d6bfe" } }, "⊞"),
    createElement("span", { style: { fontSize: 13, fontWeight: 600 } }, t("common.viewMoreCount", { n: apps.length })));
  return createElement("div", {
    "data-dsh-ee-ui": "hero-apps", role: "region", "aria-label": t("apps.enterpriseApps"),
    style: { position: "absolute", left: place.left, top: place.top, width: place.width, maxHeight: place.maxHeight, overflow: "hidden", zIndex: 4, animation: "dshee-fade .25s ease-out" },
  },
    createElement("style", null, galleryCss),
    createElement("div", { style: { display: "flex", alignItems: "baseline", justifyContent: "space-between", gap: 8, margin: "0 2px 8px", fontSize: 12 } },
      createElement("span", { style: { fontWeight: 600, opacity: 0.7 } }, t("apps.enterpriseApps")),
      todos ? createElement("span", { style: { color: "#2563eb" } }, t("appsView.todosNeed", { count: todos })) : createElement("span", { style: { opacity: 0.5 } }, t("appsView.startHint"))),
    createElement("div", { style: { display: "grid", gridTemplateColumns: `repeat(${slots}, minmax(0, 1fr))`, gap: STRIP_GAP } }, ...shown.map(card), more),
    notice ? createElement("p", { role: "status", style: { margin: "8px 2px 0", fontSize: 11.5, lineHeight: 1.5, overflowWrap: "anywhere" } }, notice) : null);
}
