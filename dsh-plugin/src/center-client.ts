import { createElement, useEffect, useState } from "react";
import { centerPanel, type CenterTab, type CenterViewActions } from "./center-view.js";
import type { CatalogApp, CenterData } from "./skills.js";
import { createWebOpener } from "./web-link.js";

/** 右侧栏的「专家 · 技能 · 连接器」标签页（docs/expert-center.md §7）。 */
export const centerTabId = "__DSH_EE_PACKAGE__/center";

interface CenterDeps {
  remote: any;
  unwrapRemote: (result: any) => any;
  useOverview: () => any;
  summon: (app: CatalogApp, starter?: string) => Promise<string>;
  reloadApps: () => Promise<void>;
}

interface Snapshot {
  data?: CenterData;
  loading: boolean;
  busy: boolean;
}

export function registerCenter(ctx: any, deps: CenterDeps) {
  const { remote, unwrapRemote } = deps;
  // 旧版接入插件后端没有 center：整个标签页不注册，免得出现一个永远空着的面板
  if (typeof remote?.center !== "function") return;
  let snapshot: Snapshot = { loading: true, busy: false };
  const listeners = new Set<() => void>();
  const update = (patch: Partial<Snapshot>) => {
    snapshot = { ...snapshot, ...patch };
    for (const l of listeners) l();
  };
  let inflight: Promise<void> | undefined;
  const refresh = () => {
    inflight ??= (async () => {
      try {
        update({ data: unwrapRemote(await remote.center()), loading: false });
      } catch {
        // 一次失败不清空上次的数据
        update({ loading: false });
      } finally {
        inflight = undefined;
      }
    })();
    return inflight;
  };
  ctx.effect(() => {
    const timer = setInterval(() => { if (listeners.size) void refresh(); }, 60_000);
    return () => clearInterval(timer);
  });
  const openWeb = createWebOpener(remote, unwrapRemote);

  const Panel = () => {
    const overview = deps.useOverview();
    const [local, setLocal] = useState(snapshot);
    const [tab, setTab] = useState<CenterTab>("experts");
    const [query, setQuery] = useState("");
    const [category, setCategory] = useState(0);
    const [open, setOpen] = useState("");
    const [notice, setNotice] = useState("");
    useEffect(() => {
      const l = () => setLocal(snapshot);
      listeners.add(l);
      l();
      void refresh();
      return () => { listeners.delete(l); };
    }, []);
    const loggedIn = !!overview.data?.me;
    useEffect(() => { if (loggedIn) void refresh(); }, [loggedIn]);
    const actions: CenterViewActions = {
      login: () => { void remote.login().then((r: any) => setNotice(unwrapRemote(r)?.text || "请在浏览器完成企业登录。")).catch(() => setNotice("发起登录失败，请稍后重试。")); },
      refresh: () => { setNotice(""); void refresh(); },
      setTab: (next) => { setTab(next); setCategory(0); setQuery(""); },
      search: setQuery,
      setCategory,
      toggleOpen: (name) => setOpen(open === name ? "" : name),
      summon: (app) => { void deps.summon(app).then(setNotice); },
      startWith: (app, example) => { void deps.summon(app, example).then(setNotice); },
      toggle: (kind, name, enabled, title) => {
        update({ busy: true });
        void remote.setSubscription({ kind, name, enabled }).then(async (r: any) => {
          const value = unwrapRemote(r);
          if (!value?.ok) return setNotice(`${enabled ? "启用" : "停用"}失败：${value?.error || "请稍后重试"}`);
          await refresh();
          if (kind === "apps") await deps.reloadApps().catch(() => undefined);
          setNotice(enabled ? `已启用「${title}」` : `已停用「${title}」`);
        }).catch(() => setNotice(`${enabled ? "启用" : "停用"}失败，请检查网络后重试。`)).finally(() => update({ busy: false }));
      },
      web: (url) => { void openWeb(url); },
    };
    return centerPanel(createElement, { data: local.data, loading: local.loading, busy: local.busy, loggedIn, tab, query, category, open, notice }, actions);
  };

  ctx.effect(() => ctx.sidebarRightTabs.register({
    id: centerTabId,
    kind: "dsh-ee-center",
    title: () => "专家 · 技能 · 连接器",
    guide: [{ id: "open", order: 20, title: () => "专家 · 技能 · 连接器", description: () => "召唤企业专家、启用技能、连接企业系统" }],
  }));
  ctx.slots.inject("sidebar.right.pane.tab", () => ctx.slots.register({ name: "sidebar.right.pane.tab", key: centerTabId }, Panel));
}
