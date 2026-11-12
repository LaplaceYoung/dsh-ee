import { createElement, useEffect, useState } from "react";
import { createTypertRemoteInfo } from "@deepseek-ai/dsh-typert-protocol";
import { z } from "zod";
import { createBrandStore, loadCachedBrand, saveCachedBrand, themeOverrides, type BrandViewData } from "./brand-view.js";
import { registerBrand } from "./brand-client.js";
import type { EnterpriseOverview, OverviewFailure } from "./overview.js";
import { enterpriseHeader, enterprisePanel, enterpriseTabId, registerEnterpriseStatus, type DoctorViewData } from "./status-view.js";
import { registerApps } from "./apps-client.js";
import { registerWorkbench } from "./workbench-client.js";
import { registerVotes } from "./votes.js";
import { registerEeSave } from "./ee-save.js";
import { registerServerSettings } from "./settings-view.js";
import { registerTurnWrites } from "./turn-writes-client.js";

export const inject = ["slots", "remote", "sidebarRightTabs", "sidebarRight", "sessions", "conversation", "workspaces"];

const jsonParameter = (name: string) => {
  const codec = { mode: "strict" as const, typeSymbol: "unknown", create: () => z.any() };
  return { name, source: "json" as const, codec, strictCodec: codec };
};

const clientRemote = createTypertRemoteInfo({
  package: "__DSH_EE_PACKAGE__",
  methods: {
    "dshEE/brand": { service: "dshEE", parameters: [], result: { mode: "strict", typeSymbol: "unknown", create: () => z.any() }, invocation: { kind: "direct" } },
    "dshEE/apps": { service: "dshEE", parameters: [], result: { mode: "strict", typeSymbol: "unknown", create: () => z.any() }, invocation: { kind: "direct" } },
    "dshEE/availableApps": { service: "dshEE", parameters: [], result: { mode: "strict", typeSymbol: "unknown", create: () => z.any() }, invocation: { kind: "direct" } },
    "dshEE/enableApp": { service: "dshEE", parameters: [jsonParameter("request")], result: { mode: "strict", typeSymbol: "unknown", create: () => z.any() }, invocation: { kind: "direct" } },
    "dshEE/disableApp": { service: "dshEE", parameters: [jsonParameter("request")], result: { mode: "strict", typeSymbol: "unknown", create: () => z.any() }, invocation: { kind: "direct" } },
    "dshEE/appBindings": { service: "dshEE", parameters: [], result: { mode: "strict", typeSymbol: "unknown", create: () => z.any() }, invocation: { kind: "direct" } },
    "dshEE/bindApp": { service: "dshEE", parameters: [jsonParameter("request")], result: { mode: "strict", typeSymbol: "unknown", create: () => z.any() }, invocation: { kind: "direct" } },
    "dshEE/unbindApp": { service: "dshEE", parameters: [jsonParameter("request")], result: { mode: "strict", typeSymbol: "unknown", create: () => z.any() }, invocation: { kind: "direct" } },
    "dshEE/workbenchData": { service: "dshEE", parameters: [jsonParameter("request")], result: { mode: "strict", typeSymbol: "unknown", create: () => z.any() }, invocation: { kind: "direct" } },
    "dshEE/kbDoc": { service: "dshEE", parameters: [jsonParameter("request")], result: { mode: "strict", typeSymbol: "unknown", create: () => z.any() }, invocation: { kind: "direct" } },
    "dshEE/board": { service: "dshEE", parameters: [jsonParameter("request")], result: { mode: "strict", typeSymbol: "unknown", create: () => z.any() }, invocation: { kind: "direct" } },
    "dshEE/checklists": { service: "dshEE", parameters: [jsonParameter("request")], result: { mode: "strict", typeSymbol: "unknown", create: () => z.any() }, invocation: { kind: "direct" } },
    "dshEE/setChecklist": { service: "dshEE", parameters: [jsonParameter("request")], result: { mode: "strict", typeSymbol: "unknown", create: () => z.any() }, invocation: { kind: "direct" } },
    "dshEE/turnWrites": { service: "dshEE", parameters: [jsonParameter("request")], result: { mode: "strict", typeSymbol: "unknown", create: () => z.any() }, invocation: { kind: "direct" } },
    "dshEE/overview": { service: "dshEE", parameters: [], result: { mode: "strict", typeSymbol: "unknown", create: () => z.any() }, invocation: { kind: "direct" } },
    "dshEE/login": { service: "dshEE", parameters: [], result: { mode: "strict", typeSymbol: "unknown", create: () => z.any() }, invocation: { kind: "direct" } },
    "dshEE/doctor": { service: "dshEE", parameters: [], result: { mode: "strict", typeSymbol: "unknown", create: () => z.any() }, invocation: { kind: "direct" } },
    "dshEE/serverSettings": { service: "dshEE", parameters: [], result: { mode: "strict", typeSymbol: "unknown", create: () => z.any() }, invocation: { kind: "direct" } },
    "dshEE/testServer": { service: "dshEE", parameters: [jsonParameter("request")], result: { mode: "strict", typeSymbol: "unknown", create: () => z.any() }, invocation: { kind: "direct" } },
    "dshEE/saveServer": { service: "dshEE", parameters: [jsonParameter("request")], result: { mode: "strict", typeSymbol: "unknown", create: () => z.any() }, invocation: { kind: "direct" } },
    "dshEE/updatePlugin": { service: "dshEE", parameters: [], result: { mode: "strict", typeSymbol: "unknown", create: () => z.any() }, invocation: { kind: "direct" } },
    "dshEE/webLink": { service: "dshEE", parameters: [jsonParameter("request")], result: { mode: "strict", typeSymbol: "unknown", create: () => z.any() }, invocation: { kind: "direct" } },
    "dshEE/insertFormRow": { service: "dshEE", parameters: [jsonParameter("request")], result: { mode: "strict", typeSymbol: "unknown", create: () => z.any() }, invocation: { kind: "direct" } },
    "dshEE/writeTableRow": { service: "dshEE", parameters: [jsonParameter("request")], result: { mode: "strict", typeSymbol: "unknown", create: () => z.any() }, invocation: { kind: "direct" } },
    "dshEE/vote": { service: "dshEE", parameters: [jsonParameter("request")], result: { mode: "strict", typeSymbol: "unknown", create: () => z.any() }, invocation: { kind: "direct" } },
  },
}, z);

function unwrapRemote(result: any): any {
  if (result && typeof result === "object") {
    if (result.error) throw new Error(typeof result.error === "string" ? result.error : result.error.message || "企业远程调用失败");
    if (result.ok === false) throw new Error("企业远程调用失败");
    if ("value" in result) return result.value;
  }
  return result;
}

interface OverviewSnapshot {
  data?: EnterpriseOverview;
  failed: boolean;
  failure?: OverviewFailure;
  busy: boolean;
}

const OVERVIEW_TIMEOUT_MS = 20_000;
/** 没登录时每隔几秒看一次登录状态：在浏览器里确认授权后，不用重开 DSH 就能看到已登录。 */
export const LOGIN_POLL_MS = 5_000;

function createOverviewStore(remote: any) {
  let snapshot: OverviewSnapshot = { failed: false, busy: false };
  let inflight = false;
  const listeners = new Set<() => void>();
  const loginListeners = new Set<() => void>();
  const update = (patch: Partial<OverviewSnapshot>) => {
    const wasIn = !!snapshot.data?.me;
    snapshot = { ...snapshot, ...patch };
    for (const listener of listeners) listener();
    if (!wasIn && snapshot.data?.me) for (const listener of loginListeners) listener();
  };
  /** quiet：后台轮询用，不把界面切到「忙」，免得按钮每 5 秒闪一下。 */
  const refresh = async (quiet = false) => {
    if (inflight) return;
    inflight = true;
    if (!quiet) update({ busy: true });
    try {
      const data = unwrapRemote(await Promise.race([
        remote.overview(),
        new Promise<never>((_, reject) => setTimeout(() => reject(new Error("读取企业账号超时")), OVERVIEW_TIMEOUT_MS)),
      ]));
      update(data?.me ? { data, failed: false, failure: undefined } : { failed: true, failure: data?.failure });
    } catch {
      update({ failed: true, failure: undefined });
    } finally {
      inflight = false;
      if (!quiet) update({ busy: false });
    }
  };
  /** 没登录、并且有界面在看时，静默再查一次；页面在后台时不查。 */
  const pollLogin = () => {
    if (snapshot.data?.me || !listeners.size) return;
    if (typeof document !== "undefined" && document.hidden) return;
    void refresh(true);
  };
  const use = () => {
    const [local, setLocal] = useState(snapshot);
    useEffect(() => {
      const listener = () => setLocal(snapshot);
      listeners.add(listener);
      listener();
      if (!snapshot.data) void refresh();
      return () => { listeners.delete(listener); };
    }, []);
    return { ...local, hasData: () => !!snapshot.data, setBusy: (busy: boolean) => update({ busy }), refresh: () => refresh() };
  };
  const onLogin = (listener: () => void) => {
    loginListeners.add(listener);
    return () => { loginListeners.delete(listener); };
  };
  return { use, refresh: () => refresh(), pollLogin, onLogin };
}

export async function apply(ctx: any): Promise<void> {
  // 品牌最先注册、只依赖 slots：先用上次缓存的品牌（DSH 一打开就是企业 logo），连上 dsh-ee 后再换成最新的。
  // 不能排在远程服务和其它功能后面——前面任何一步卡住或出错，logo 就会一直是 DSH 自带的鱼。
  const brandStore = createBrandStore(loadCachedBrand() ?? { enabled: false, name: "", logo_url: "", revision: "" });
  ctx.effect(() => brandStore.subscribe(() => saveCachedBrand(brandStore.get())));
  ctx.effect(() => registerBrand(ctx, brandStore, { createElement, useState, useEffect }));

  await ctx.remote.$mount(clientRemote);
  ctx.inject(["remote.dshEE"], async (remoteCtx: any) => {
    const remote = remoteCtx.remote.dshEE;
    const overview = createOverviewStore(remote);
    // 设置页不等品牌：连不上 dsh-ee 时正是要靠它改地址
    registerServerSettings(ctx, { createElement, useState: useState as any, useEffect, remote, unwrapRemote, label: "dsh-ee", onSaved: () => void overview.refresh() });
    const useOverview = (_remote?: any) => overview.use();
    const apps = registerApps(ctx, remote, useOverview, unwrapRemote);
    // 登录状态轮询跟着插件生命周期走；一登录上就重新拉企业应用（不等 60 秒的目录轮询）
    ctx.effect(() => {
      const timer = setInterval(overview.pollLogin, LOGIN_POLL_MS);
      const off = overview.onLogin(() => void apps.loadApps());
      return () => { clearInterval(timer); off(); };
    });
    const { AppsPanel } = apps;
    const workbench = registerWorkbench(ctx, { remote, unwrapRemote, appsNow: apps.appsNow, appsLoaded: apps.appsLoaded, loadApps: apps.loadApps, subscribeApps: apps.subscribeApps });
    apps.setWorkbenchOpener(workbench.openWorkbench);
    registerVotes(ctx, remote, unwrapRemote);
    registerEeSave(ctx);
    // 一轮末尾的「已留痕」提示不能拖死后面的品牌：宿主没有这个槽位时 inject 会一直等着，不会抛错
    try {
      registerTurnWrites(ctx, remote, unwrapRemote);
    } catch {
      // 宿主不认这个槽位
    }
    const readBrand = async (): Promise<BrandViewData> => unwrapRemote(await Promise.race([
      remote.brand(),
      new Promise<never>((_, reject) => setTimeout(() => reject(new Error("读取企业品牌超时")), 10_000)),
    ]));
    brandStore.setReader(readBrand);
    await brandStore.refresh();
    const org = brandStore.get().name || "企业";
    ctx.effect(() => brandStore.start());
    // 配色与 logo 无关，不需要接管品牌位置：交给 DSH 的主题服务按设计变量覆盖，改了就跟着变。
    // 主题服务不存在或拒绝注入时不能拖死后面的品牌槽位注册，单独兜底。
    try {
      ctx.inject(["theme"], (themeCtx: any) => {
      let applied = "";
      let dispose: (() => void) | undefined;
      const sync = () => {
        const tokens = themeOverrides(brandStore.get());
        const key = JSON.stringify(tokens ?? null);
        if (key === applied) return;
        applied = key;
        dispose?.();
        dispose = undefined;
        if (!tokens) return;
        try {
          dispose = themeCtx.theme.overrideTokens("dsh-ee-brand", tokens);
        } catch {
          // 宿主不认某个取值时保留 DSH 默认配色
        }
      };
      sync();
      const off = brandStore.subscribe(sync);
      themeCtx.effect(() => () => { off(); dispose?.(); });
      });
    } catch {
      // 宿主没有主题服务或 inject 被拒：跳过配色，继续注册品牌位置
    }

    const EnterprisePanel = () => {
      const state = useOverview(remote);
      const [notice, setNotice] = useState("");
      const [doctor, setDoctor] = useState<DoctorViewData | undefined>(undefined);
      // 轮询发现已登录：去掉「请在浏览器确认」这类过时的提示
      const loggedIn = !!state.data?.me;
      useEffect(() => { if (loggedIn) setNotice(""); }, [loggedIn]);
      const login = async () => {
        state.setBusy(true);
        try {
          const result = unwrapRemote(await remote.login());
          setNotice(result?.text || "请在浏览器完成企业登录。");
          // 之后由全局的登录轮询（每 5 秒）发现登录成功
        } catch {
          setNotice("发起登录失败，请检查网络后重试。");
        } finally {
          state.setBusy(false);
        }
      };
      const diagnose = async () => {
        state.setBusy(true);
        try {
          setDoctor(unwrapRemote(await remote.doctor()));
        } catch {
          setNotice("诊断失败，请检查网络后重试。");
        } finally {
          state.setBusy(false);
        }
      };
      const refresh = () => {
        setNotice("");
        setDoctor(undefined);
        void state.refresh();
      };
      return enterprisePanel(createElement, org, { ...state, notice, doctor }, { login: () => void login(), refresh, doctor: () => void diagnose() }, AppsPanel);
    };
    const EnterpriseHeader = () => {
      const state = useOverview(remote);
      const onClick = () => {
        ctx.sidebarRight.openTab("dsh-ee-enterprise");
        if (state.failed) void remote.login().catch(() => undefined);
      };
      return enterpriseHeader(createElement, state.data, state.failed, onClick);
    };
    registerEnterpriseStatus(ctx, org, EnterprisePanel, EnterpriseHeader);
  });
}
