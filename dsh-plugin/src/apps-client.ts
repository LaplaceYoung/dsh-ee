import { createElement, useEffect, useRef, useState } from "react";
import { appsGallery, appsSection, heroStrip, inProgressItems, inProgressView, type AppsViewActions, type MenuAnchor, type StripPlace } from "./apps-view.js";
import { readSidebarMode, setSidebarMode, subscribeSidebarMode, type SidebarAppsMode } from "./sidebar-pref.js";
import { fillInput, openSession } from "./host-session.js";
import { createLauncher, singleFlight } from "./app-launcher.js";
import type { CatalogApp } from "./skills.js";
import type { AppBindingMap } from "./app-bindings.js";
import { createWebOpener } from "./web-link.js";
import { applicationStatus, createSessionSnapshotAdapter, visibleAppStatus } from "./session-status.js";

interface AppsSnapshot {
  apps: CatalogApp[];
  available: CatalogApp[];
  bindings: AppBindingMap;
  sessionSnapshot?: any;
  busy: boolean;
}

export function registerApps(ctx: any, remote: any, useOverview: () => any, unwrapRemote: (result: any) => any) {
  let snapshot: AppsSnapshot = { apps: [], available: [], bindings: {}, busy: false };
  const adaptSessions = createSessionSnapshotAdapter();
  const seen = new Set<string>();
  let started = false;
  // 第一次拉目录是否已经结束：工作台标签页据此区分「还在加载」和「应用已经不在了」
  let loaded = false;
  const listeners = new Set<() => void>();
  const update = (patch: Partial<AppsSnapshot>) => {
    snapshot = { ...snapshot, ...patch };
    for (const listener of listeners) listener();
  };
  const refresh = singleFlight(async () => {
    update({ busy: true });
    try {
      const apps = unwrapRemote(await remote.apps());
      update({ apps: Array.isArray(apps) ? apps : [] });
    } catch {
      // 一次 5xx/断网不清空上次列表；保留快照，等下一轮刷新
    }
    try {
      // 旧版接入插件后端没有这个方法
      const available = typeof remote.availableApps === "function" ? unwrapRemote(await remote.availableApps()) : [];
      update({ available: Array.isArray(available) ? available : [] });
    } catch {
      // 保留上次可用应用列表
    }
    try {
      if (typeof remote.appBindings === "function") {
        const bindings = unwrapRemote(await remote.appBindings());
        if (bindings && typeof bindings === "object") update({ bindings });
      }
    } catch {
      // 保留上次绑定
    } finally {
      loaded = true;
      update({ busy: false });
    }
  });
  // 目录轮询跟着插件的生命周期走：插件卸载或热重载时撤销
  ctx.effect(() => {
    const timer = setInterval(() => { if (started) void refresh(); }, 60_000);
    return () => clearInterval(timer);
  });
  const readSessions = () => adaptSessions(ctx.sessions?.list?.getSnapshot?.(), ctx.get("uiSession")?.sessionStatus);
  ctx.effect(() => {
    update({ sessionSnapshot: readSessions() });
    const disposals = [ctx.sessions?.list?.subscribe?.(() => update({ sessionSnapshot: readSessions() })), ctx.get("uiSession")?.sessionStatus?.subscribe?.(() => update({ sessionSnapshot: readSessions() }))];
    return () => { for (const dispose of disposals) if (typeof dispose === "function") dispose(); };
  });
  const useApps = () => {
    const [local, setLocal] = useState(snapshot);
    useEffect(() => {
      const listener = () => setLocal(snapshot);
      listeners.add(listener);
      listener();
      started = true;
      void refresh();
      return () => { listeners.delete(listener); };
    }, []);
    return local;
  };

  const copy = async (app: CatalogApp) => {
    const command = `/${app.name}`;
    await navigator.clipboard?.writeText(command);
    return `已复制 ${command}，粘贴到输入框发送`;
  };
  const fill = async (app: CatalogApp) => {
    if (fillInput(ctx, `/${app.name}`)) return "已填进输入框，确认后发送";
    return copy(app);
  };
  const bind = async (app: CatalogApp, id: string) => {
    const bindings = unwrapRemote(await remote.bindApp({ name: app.name, sessionId: id }));
    if (bindings && typeof bindings === "object") update({ bindings });
  };
  const markSeen = (app: CatalogApp) => {
    const id = snapshot.bindings[app.name]?.sessionId;
    if (id) seen.add(`${app.name}:${id}`);
    update({ sessionSnapshot: snapshot.sessionSnapshot });
  };
  // 应用启动后打开它的工作台；由 client.ts 在注册工作台以后设置
  let workbenchOpener: ((app: CatalogApp, sessionId: string, force?: boolean) => Promise<boolean>) | undefined;
  const launcher = createLauncher(ctx, { bind, afterLaunch: async (app, id) => { await workbenchOpener?.(app, id); } });
  const launchSidebar = async (app: CatalogApp) => {
    const id = snapshot.bindings[app.name]?.sessionId;
    const archived = ctx.workspaces?.list?.getSnapshot?.()?.archivedSessionIds;
    const listed = !Array.isArray(snapshot.sessionSnapshot?.ids) || snapshot.sessionSnapshot.ids.includes(id);
    if (app.launch === "resume" && id && listed && snapshot.sessionSnapshot?.byId?.[id] && !archived?.includes?.(id)) {
      markSeen(app);
      await launcher.resume(app, id);
      return;
    }
    await launcher.launchNew(app);
  };
  const openWithFallback = (app: CatalogApp, launch: (app: CatalogApp) => Promise<void>) => launcher.run(() => launch(app), () => fill(app));
  const openWeb = createWebOpener(remote, unwrapRemote);
  const web = (app: CatalogApp) => {
    if (app.web_url) void openWeb(app.web_url);
  };

  const useActions = (setNotice: (notice: string) => void, launch: (app: CatalogApp) => Promise<void>, setConfirmDisable: (name: string) => void = () => {}): AppsViewActions => ({
    login: () => { void remote.login().then((result: any) => setNotice(unwrapRemote(result)?.text || "请在浏览器完成企业登录。")).catch(() => setNotice("发起登录失败，请稍后重试。")); },
    open: (app) => { markSeen(app); void openWithFallback(app, launch).then(setNotice); },
    newSession: (app) => { void openWithFallback(app, (target) => launcher.launchNew(target)).then(setNotice); },
    startWith: (app, starter) => { void openWithFallback(app, (target) => launcher.launchNew(target, starter)).then(setNotice); },
    openWorkbench: (app) => {
      const id = snapshot.bindings[app.name]?.sessionId;
      const current = ctx.sidebarRight?.mounted?.getSnapshot?.();
      if (!id || id !== current) {
        setNotice("先点应用回到它的会话，再打开工作台");
        return;
      }
      void workbenchOpener?.(app, id, true).then((opened) => { if (!opened) setNotice("工作台没有打开，请稍后再试"); });
    },
    enable: (app) => {
      void remote.enableApp({ name: app.name }).then(async (result: any) => {
        const value = unwrapRemote(result);
        if (!value?.ok) return setNotice(`启用失败：${value?.error || "请稍后重试"}`);
        await refresh();
        setNotice(`已启用「${app.title}」，点一下开始使用`);
      }).catch(() => setNotice("启用失败，请检查网络后重试。"));
    },
    disable: (app, confirmed) => {
      if (!confirmed) return setConfirmDisable(app.name);
      setConfirmDisable("");
      void remote.disableApp({ name: app.name }).then(async (result: any) => {
        const value = unwrapRemote(result);
        if (!value?.ok) return setNotice(`停用失败：${value?.error || "请稍后重试"}`);
        await refresh();
        setNotice(`已停用「${app.title}」`);
      }).catch(() => setNotice("停用失败，请检查网络后重试。"));
    },
    unbind: (app) => { void remote.unbindApp({ name: app.name }).then((result: any) => { const bindings = unwrapRemote(result); update({ bindings }); setNotice("已解除绑定"); }).catch(() => setNotice("解除绑定失败，请稍后重试。")); },
    fill: (app) => { void fill(app).then(setNotice); },
    web,
    details: (app) => { if (app.detail_url) void openWeb(app.detail_url); },
    search: () => {},
  });

  const useSidebarMode = () => {
    const [mode, setMode] = useState<SidebarAppsMode>(readSidebarMode());
    useEffect(() => subscribeSidebarMode(() => setMode(readSidebarMode())), []);
    return mode;
  };

  /** 「⋯」菜单与「查看更多」浮层的开合、Escape 关闭、滚动时关菜单。 */
  const useLayers = () => {
    const [galleryOpen, setGalleryOpen] = useState(false);
    const [menuFor, setMenuFor] = useState("");
    const [menuAnchor, setMenuAnchor] = useState<MenuAnchor | undefined>(undefined);
    const [query, setQuery] = useState("");
    const closeMenu = () => { setMenuFor(""); setMenuAnchor(undefined); };
    const closeLayers = () => { closeMenu(); setGalleryOpen(false); setQuery(""); };
    // 「⋯」菜单用 fixed 定位贴着按钮：靠窗口下沿时向上展开，避免被窗口裁掉
    const toggleMenu = (name: string, event?: any) => {
      if (!name || name === menuFor) { closeMenu(); return; }
      const rect = event?.currentTarget?.getBoundingClientRect?.();
      const innerW = typeof window === "undefined" ? 360 : window.innerWidth;
      const innerH = typeof window === "undefined" ? 640 : window.innerHeight;
      const up = !rect || rect.bottom + 340 > innerH - 8;
      setMenuAnchor({ up, right: rect ? Math.max(8, innerW - rect.right) : 8, top: rect ? rect.bottom + 4 : undefined, bottom: rect ? innerH - rect.top + 4 : 64 });
      setMenuFor(name);
    };
    // Escape 关浮层；滚动/缩放后 fixed 菜单会失去锚点，直接关掉
    useEffect(() => {
      if (!menuFor && !galleryOpen) return;
      if (typeof window === "undefined") return;
      const onKey = (event: any) => { if (event?.key === "Escape") { if (menuFor) closeMenu(); else closeLayers(); } };
      const onScroll = (event: any) => { if (!event?.target?.closest?.('[data-dsh-ee-ui="apps-gallery"]') || menuFor) closeMenu(); };
      window.addEventListener("keydown", onKey);
      window.addEventListener("resize", onScroll);
      window.document?.addEventListener?.("scroll", onScroll, true);
      return () => {
        window.removeEventListener("keydown", onKey);
        window.removeEventListener("resize", onScroll);
        window.document?.removeEventListener?.("scroll", onScroll, true);
      };
    }, [menuFor, galleryOpen]);
    return { galleryOpen, setGalleryOpen, menuFor, menuAnchor, query, setQuery, closeMenu, closeLayers, toggleMenu };
  };

  /** sidebar：左侧栏，最多列 3 个、可以收起和隐藏，偏好存在本机；panel：右侧企业面板里的那份，列 6 个、不能隐藏。 */
  const makeAppsSection = (placement: "sidebar" | "panel") => () => {
    const overview = useOverview();
    const state = useApps();
    const mode = useSidebarMode();
    const layers = useLayers();
    const [panelCollapsed, setPanelCollapsed] = useState(false);
    const [notice, setNotice] = useState("");
    const [availableOpen, setAvailableOpen] = useState(false);
    const [confirmDisable, setConfirmDisable] = useState("");
    if (!overview.data && !overview.failed) return null;
    const sidebar = placement === "sidebar";
    if (sidebar && mode === "hidden") return null;
    const collapsed = sidebar ? mode === "collapsed" : panelCollapsed;
    const actions = {
      ...useActions(setNotice, launchSidebar, setConfirmDisable),
      search: layers.setQuery,
      toggleAvailable: () => setAvailableOpen(!availableOpen),
      toggleCollapsed: () => { if (sidebar) setSidebarMode(collapsed ? "shown" : "collapsed"); else setPanelCollapsed(!collapsed); layers.closeMenu(); },
      toggleGallery: () => { layers.setGalleryOpen(!layers.galleryOpen); layers.closeMenu(); },
      toggleMenu: layers.toggleMenu,
      closeLayers: layers.closeLayers,
      setSidebarHidden: (hidden: boolean) => { setSidebarMode(hidden ? "hidden" : "shown"); if (hidden) layers.closeLayers(); },
    };
    const statuses = Object.fromEntries(state.apps.map((app) => {
      const id = state.bindings[app.name]?.sessionId;
      return [app.name, id ? visibleAppStatus(applicationStatus(state.sessionSnapshot, id), `${app.name}:${id}`, seen) : undefined];
    }));
    return appsSection(createElement, {
      apps: state.apps, available: state.available, availableOpen, confirmDisable, loggedIn: !!overview.data?.me, collapsed,
      galleryOpen: layers.galleryOpen, menuFor: layers.menuFor, menuAnchor: layers.menuAnchor, query: layers.query, notice, statuses,
      limit: sidebar ? 3 : 6, hideable: sidebar, sidebarHidden: mode === "hidden",
    }, actions);
  };
  const AppsSection = makeAppsSection("sidebar");
  const PanelAppsSection = makeAppsSection("panel");

  /**
   * 输入框所在的那一列：座位（seat）可能铺满整个主区，输入框在里面居中、有最大宽度。
   * 从输入框往上找，取座位里面最外层、又比座位窄的那个祖先；找不到（窄窗口里一样宽）就用座位本身。
   */
  const inputColumn = (seat: any): any => {
    const editor = seat.querySelector?.("textarea, [contenteditable='true'], [role='textbox']");
    const seatWidth = seat.getBoundingClientRect?.().width ?? 0;
    let column = seat;
    for (let node = editor?.parentElement; node && node !== seat; node = node.parentElement) {
      // display: contents 的包装层宽度是 0，跳过
      const width = node.getBoundingClientRect?.().width ?? 0;
      if (width > 0 && width < seatWidth - 1) column = node;
    }
    return column;
  };

  /**
   * 量输入框的位置，让首屏的横排贴在它正下方：输入框所在的 [data-composer-seat] 由宿主渲染，
   * 我们的组件在它上面一行（hero.options）里，所以用 fixed 定位；probe 是 fixed 在 (0,0) 的空元素，
   * 祖先有 transform 时 fixed 会相对那个祖先，减去 probe 的位置就对了。
   */
  const useBelowComposer = (anchor: { current: any }, probe: { current: any }, ready: boolean) => {
    const [place, setPlace] = useState<StripPlace | undefined>(undefined);
    const [missing, setMissing] = useState(false);
    useEffect(() => {
      if (!ready || typeof window === "undefined") return;
      const seat = anchor.current?.closest?.("[data-composer-seat]") ?? window.document?.querySelector?.("[data-composer-seat]");
      if (!seat) { setMissing(true); return; }
      let frame = 0;
      const measure = () => {
        cancelAnimationFrame(frame);
        frame = requestAnimationFrame(() => {
          const r = inputColumn(seat).getBoundingClientRect?.();
          const origin = probe.current?.getBoundingClientRect?.() ?? { left: 0, top: 0 };
          if (!r || r.width < 200) return setPlace(undefined);
          const top = r.bottom + 18;
          const next = { left: Math.round(r.left - origin.left), top: Math.round(top - origin.top), width: Math.round(r.width), maxHeight: Math.round(window.innerHeight - top - 12) };
          setPlace((current) => (current && current.left === next.left && current.top === next.top && current.width === next.width && current.maxHeight === next.maxHeight ? current : next));
        });
      };
      measure();
      const observer = typeof ResizeObserver === "undefined" ? undefined : new ResizeObserver(measure);
      observer?.observe(seat);
      observer?.observe(window.document.body);
      window.addEventListener("resize", measure);
      window.document.addEventListener("scroll", measure, true);
      // 侧栏开合的动画只移动位置、不一定改变大小，低频兜底再量一次
      const timer = setInterval(measure, 800);
      return () => {
        cancelAnimationFrame(frame);
        observer?.disconnect();
        window.removeEventListener("resize", measure);
        window.document.removeEventListener("scroll", measure, true);
        clearInterval(timer);
      };
    }, [ready]);
    return { place, missing };
  };

  /** 首屏：输入框下方一行横排的应用卡 +「查看更多」网格浮层。 */
  const HeroApps = () => {
    const state = useApps();
    const mode = useSidebarMode();
    const layers = useLayers();
    const [notice, setNotice] = useState("");
    const anchor = useRef<any>(null);
    const probe = useRef<any>(null);
    const { place, missing } = useBelowComposer(anchor, probe, state.apps.length > 0);
    const open = (app: CatalogApp) => { layers.closeLayers(); void openWithFallback(app, launcher.launchHero).then(setNotice); };
    const query = layers.query.trim().toLowerCase();
    const filtered = state.apps.filter((app) => !query || `${app.title} ${app.description} ${app.name}`.toLowerCase().includes(query));
    const gallery = appsGallery(createElement, { apps: state.apps, available: state.available, loggedIn: true, galleryOpen: layers.galleryOpen, query: layers.query, sidebarHidden: mode === "hidden" }, {
      ...useActions(setNotice, launcher.launchHero),
      open,
      search: layers.setQuery,
      closeLayers: layers.closeLayers,
      toggleGallery: () => layers.setGalleryOpen(!layers.galleryOpen),
      setSidebarHidden: (hidden: boolean) => setSidebarMode(hidden ? "hidden" : "shown"),
    }, filtered);
    if (!state.apps.length) return null;
    // 宿主没有输入框座位（旧版本）时退回一个按钮，点了打开网格浮层
    const fallback = missing ? createElement("button", { type: "button", onClick: () => layers.setGalleryOpen(true), "data-dsh-ee-ui": "hero-apps", style: { border: 0, background: "transparent", color: "inherit", cursor: "pointer", fontSize: 12, padding: "4px 7px", whiteSpace: "nowrap" } }, "企业应用") : null;
    return createElement("span", { ref: anchor, "data-dsh-ee-ui": "hero-anchor", style: { display: "inline-flex", width: fallback ? undefined : 0, height: fallback ? undefined : 0 } },
      fallback,
      createElement("span", { ref: probe, style: { position: "fixed", left: 0, top: 0, width: 0, height: 0, zIndex: 4 } },
        heroStrip(createElement, state.apps, place, open, () => layers.setGalleryOpen(true), notice)),
      gallery);
  };
  const InProgress = () => {
    const state = useApps();
    const items = inProgressItems(state.apps, state.bindings, (id) => applicationStatus(state.sessionSnapshot, id));
    return inProgressView(createElement, items, (item) => { void openSession(ctx, item.sessionId).catch(() => undefined); });
  };
  const AppsPanel = () => createElement("div", null, createElement(InProgress), createElement(PanelAppsSection));

  ctx.slots.inject("sidebar.footer.action", () => ctx.slots.register({ name: "sidebar.footer.action", id: "dsh-ee-apps", order: 30, label: "企业应用" }, AppsSection));
  ctx.slots.inject("conversation.hero.options", () => ctx.slots.register({ name: "conversation.hero.options", id: "dsh-ee-apps", order: 100, label: "企业应用" }, HeroApps));
  return {
    AppsPanel,
    appsNow: () => snapshot.apps,
    appsLoaded: () => loaded,
    loadApps: () => {
      started = true;
      return refresh();
    },
    subscribeApps: (listener: () => void) => {
      listeners.add(listener);
      return () => { listeners.delete(listener); };
    },
    setWorkbenchOpener: (opener: typeof workbenchOpener) => { workbenchOpener = opener; },
    /** 新会话里打开一个应用或专家（可带一条示例开始）；返回给员工看的提示，空串表示成功 */
    summon: (app: CatalogApp, starter?: string) => {
      markSeen(app);
      return openWithFallback(app, (target) => launcher.launchNew(target, starter));
    },
    /** 启用、停用之后刷新应用列表（左侧栏和首屏） */
    reloadApps: () => refresh(),
  };
}
