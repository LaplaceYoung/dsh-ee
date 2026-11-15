// 业务工作台的前端：在官方右侧栏注册标签页类型，打开应用时按配置打开窗格。
import { createElement, useEffect, useRef, useState } from "react";
import { sendUnknownNotice } from "./app-launcher.js";
import { currentSessionOf, sendToSession } from "./host-session.js";
import { t } from "./i18n/index.js";
import type { CatalogApp } from "./skills.js";
import { checklistKey, docWindow, fillPrompt, kbDocUrl, parseWorkbenchAddress, rowWriteOf, scopeText, workbenchAddress, workbenchScheme, type Board, type Citation, type KBaseItem, type WorkbenchAction, type WorkbenchPane } from "./workbench.js";
import { openWorkbench } from "./workbench-open.js";
import { createWebOpener, interceptWebLinks } from "./web-link.js";
import { kbDocView, missingPaneView, panelView, type DocTarget, type PanelHandlers, type WidgetState } from "./workbench-view.js";

export const workbenchTabId = "__DSH_EE_PACKAGE__/workbench";
export const workbenchKind = "dsh-ee-workbench";

function onScreenSession(ctx: any): string {
  const mounted = ctx.sidebarRight?.mounted?.getSnapshot?.();
  return typeof mounted === "string" && mounted ? mounted : currentSessionOf(ctx.sessions?.list?.getSnapshot?.());
}

/** 每个应用的「原文」窗格此刻要看的引用；面板上点引用时设置，原文窗格订阅它。 */
function createDocTargets() {
  const targets = new Map<string, DocTarget>();
  const listeners = new Set<() => void>();
  return {
    get: (app: string) => targets.get(app),
    set: (app: string, target: DocTarget) => {
      targets.set(app, target);
      for (const listener of listeners) listener();
    },
    subscribe: (listener: () => void) => {
      listeners.add(listener);
      return () => { listeners.delete(listener); };
    },
  };
}

export interface WorkbenchDeps {
  remote: any;
  unwrapRemote: (result: any) => any;
  appsNow: () => CatalogApp[];
  /** 第一次拉目录是否已经结束 */
  appsLoaded: () => boolean;
  /** 拉一次目录（刷新页面后，标签页可能比侧栏先出现） */
  loadApps: () => Promise<void>;
  /** 订阅目录变化，返回撤销函数 */
  subscribeApps: (listener: () => void) => () => void;
}

export function registerWorkbench(ctx: any, deps: WorkbenchDeps) {
  const { remote, unwrapRemote } = deps;
  // 出处链接用的 dsh-ee 地址；员工在设置里改了地址，下次打开面板时重读
  let docBase = "";
  const loadDocBase = () => Promise.resolve(remote.serverSettings?.()).then((value: any) => { docBase = unwrapRemote(value)?.base ?? docBase; }, () => undefined);
  void loadDocBase();
  // 面板里指向 dsh-ee 网页的链接（看原文、去申请权限）走免登录链接
  const onLinkClick = interceptWebLinks(() => docBase, createWebOpener(remote, unwrapRemote));
  const titleOf = (address: string) => {
    const parsed = parseWorkbenchAddress(address);
    const pane = deps.appsNow().find((app) => app.name === parsed?.app)?.workbench?.panes.find((item) => item.id === parsed?.pane);
    return pane?.title || t("workbenchClient.fallbackTitle");
  };

  const useAppsList = () => {
    const [apps, setApps] = useState(deps.appsNow());
    const [loaded, setLoaded] = useState(deps.appsLoaded());
    useEffect(() => {
      const dispose = deps.subscribeApps(() => {
        setApps(deps.appsNow());
        setLoaded(deps.appsLoaded());
      });
      if (!deps.appsLoaded()) void deps.loadApps();
      return dispose;
    }, []);
    return { apps, loaded };
  };

  const docTargets = createDocTargets();

  const useSending = () => {
    const [sending, setSending] = useState<Set<string>>(new Set());
    const [notice, setNotice] = useState("");
    /** 把一段话发到屏幕上的会话；返回是否发出 */
    const send = (key: string, text: string, label: string, done?: () => void) => {
      if (sending.has(key)) return;
      const sessionId = onScreenSession(ctx);
      if (!sessionId) return setNotice(t("workbenchClient.noSession"));
      setSending((current) => new Set(current).add(key));
      setNotice("");
      sendToSession(ctx, sessionId, text)
        .then(() => {
          setNotice(t("workbenchClient.sentToAi", { label }));
          done?.();
        })
        // 发送失败时消息可能已经到了宿主：只提示，不自动重试
        .catch(() => setNotice(sendUnknownNotice()))
        .finally(() => setSending((current) => {
          const next = new Set(current);
          next.delete(key);
          return next;
        }));
    };
    return { sending, notice, setNotice, send };
  };

  const PanelPane = ({ app, pane, info, visible }: { app: CatalogApp; pane: WorkbenchPane; info: any; visible: boolean }) => {
    const [data, setData] = useState<Record<number, WidgetState>>({});
    const [board, setBoard] = useState<Board | null | undefined>(undefined);
    const [forms, setForms] = useState<Record<number, Record<string, any>>>({});
    const [formErrors, setFormErrors] = useState<Record<number, string>>({});
    const [selectedKBs, setSelectedKBs] = useState<string[]>([]);
    const [checks, setChecks] = useState<Record<string, string[]>>({});
    // 写表的行按钮：正在写的按钮、点过一次等确认的删除按钮
    const [writing, setWriting] = useState<Set<string>>(new Set());
    const [armed, setArmed] = useState("");
    const [expanded, setExpanded] = useState<Record<number, number | undefined>>({});
    const { sending, notice, setNotice, send: sendText } = useSending();
    const sessionRef = useRef("");
    if (!sessionRef.current) sessionRef.current = onScreenSession(ctx);

    const load = async (index: number) => {
      setData((current) => ({ ...current, [index]: { status: "loading" } }));
      try {
        const result = unwrapRemote(await remote.workbenchData({ app: app.name, pane: pane.id, widget: index }));
        const state: WidgetState = result?.forbidden ? { status: "forbidden" } : result?.error ? { status: "error", error: String(result.error) } : { status: "ready", value: result?.value };
        setData((current) => ({ ...current, [index]: state }));
      } catch (error) {
        setData((current) => ({ ...current, [index]: { status: "error", error: error instanceof Error ? error.message : t("workbenchClient.networkError") } }));
      }
    };
    const loadAll = () => (pane.widgets ?? []).forEach((widget, index) => { if (widget.source) void load(index); });
    const loadBoard = async () => {
      if (!sessionRef.current) return;
      try {
        setBoard(unwrapRemote(await remote.board({ app: app.name, sessionId: sessionRef.current })));
      } catch {
        // 看板读不到时保留上一次的内容
      }
    };
    const hasBoard = !!pane.widgets?.some((widget) => widget.type === "board");
    const hasChecklist = !!pane.widgets?.some((widget) => widget.type === "checklist");
    const loadChecks = async () => {
      if (!sessionRef.current) return;
      try {
        const lists = unwrapRemote(await remote.checklists({ app: app.name, sessionId: sessionRef.current }));
        if (lists && typeof lists === "object") setChecks(lists);
      } catch {
        // 读不到时保留上一次的勾选
      }
    };
    const toggleCheck = (index: number, id: string) => {
      const key = checklistKey(pane.id, index);
      if (!sessionRef.current) return setNotice(t("workbenchClient.noSessionCheck"));
      const done = !(checks[key] ?? []).includes(id);
      setChecks((current) => ({ ...current, [key]: done ? [...(current[key] ?? []), id] : (current[key] ?? []).filter((x) => x !== id) }));
      Promise.resolve(remote.setChecklist({ app: app.name, sessionId: sessionRef.current, key, id, done }))
        .then((raw: any) => {
          const next = unwrapRemote(raw);
          if (Array.isArray(next)) setChecks((current) => ({ ...current, [key]: next }));
        })
        .catch(() => setNotice(t("workbenchClient.checkNotSaved")));
    };

    // 打开标签页或配置变化时取一次数；不做定时刷新，组件上有刷新按钮
    useEffect(loadAll, [app.name, app.version, pane.id]);
    useEffect(() => { void loadDocBase(); }, [app.name]);
    // 看板与清单：标签页可见时每 3 秒读一次本机文件（AI 会更新它们），不可见时停止
    useEffect(() => {
      if ((!hasBoard && !hasChecklist) || !visible) return;
      const tick = () => {
        if (hasBoard) void loadBoard();
        if (hasChecklist) void loadChecks();
      };
      tick();
      const timer = setInterval(tick, 3000);
      return () => clearInterval(timer);
    }, [hasBoard, hasChecklist, visible, app.name]);
    // 右侧栏的「刷新」命令：重新取数
    useEffect(() => info?.tab?.actions?.bindCommands?.({ refresh: () => { loadAll(); void loadBoard(); void loadChecks(); } }), [info?.tab?.id, app.version]);

    // {{scope.kbs}}：「查哪些库」组件取到的库里勾选了哪些
    const scopeIndex = (pane.widgets ?? []).findIndex((widget) => widget.type === "kb_scope");
    const scopeState = scopeIndex >= 0 ? data[scopeIndex] : undefined;
    const bases: KBaseItem[] = scopeState?.status === "ready" && Array.isArray(scopeState.value?.items) ? scopeState.value.items : [];
    const sendPrompt = (key: string, action: WorkbenchAction, values: { row?: any; form?: Record<string, any> }) => {
      sendText(key, fillPrompt(action.prompt, { ...values, scope: { kbs: scopeText(bases, selectedKBs) } }), action.label, () => {
        if (values.form) {
          const index = Number(key.split(":")[0]);
          setForms((current) => ({ ...current, [index]: {} }));
        }
      });
    };
    // 表单提交到应用自带的表：直接插入一行，刷新读这张表的组件；配置了 prompt 的再发给 AI
    const saveToTable = (key: string, action: WorkbenchAction, form: Record<string, any>) => {
      const index = Number(key.split(":")[0]);
      if (sending.has(key)) return;
      setFormError(index, "");
      Promise.resolve(remote.insertFormRow?.({ app: app.name, pane: pane.id, widget: index, values: form }))
        .then((raw: any) => {
          const result = unwrapRemote(raw);
          if (!result?.ok) return setFormError(index, result?.error || t("workbenchClient.saveFailedDefault"));
          setForms((current) => ({ ...current, [index]: {} }));
          setNotice(t("workbenchClient.saved", { label: action.label }));
          (pane.widgets ?? []).forEach((widget, i) => { if (widget.source?.table === action.table) void load(i); });
          if (action.prompt) sendPrompt(key, action, { form });
        })
        .catch(() => setFormError(index, t("workbenchClient.saveFailedNetwork")));
    };
    const setFormError = (index: number, error: string) => setFormErrors((current) => ({ ...current, [index]: error }));
    // 表格的行按钮写表：改状态或删除这一行（删除要点两次），完了刷新读这张表的组件
    const writeRow = (key: string, action: WorkbenchAction, row: any) => {
      const write = rowWriteOf(action);
      if (writing.has(key)) return;
      if (write === "delete" && armed !== key) {
        setArmed(key);
        setTimeout(() => setArmed((current) => (current === key ? "" : current)), 4000);
        return;
      }
      setArmed("");
      const [index, , , a] = key.split(":").map(Number);
      setWriting((current) => new Set(current).add(key));
      Promise.resolve(remote.writeTableRow?.({ app: app.name, pane: pane.id, widget: index, action: a, id: Number(row?.id) }))
        .then((raw: any) => {
          const result = unwrapRemote(raw);
          if (!result?.ok) return setNotice(t("workbenchClient.writeFailed", { action: write === "delete" ? t("common.delete") : t("common.save"), error: result?.error || t("workbenchClient.updatePlugin") }));
          setNotice(write === "delete" ? t("workbenchClient.deletedRow") : t("workbenchClient.updated", { label: action.label }));
          (pane.widgets ?? []).forEach((widget, i) => { if (widget.source?.table === action.table) void load(i); });
          if (action.prompt) sendPrompt(key, action, { row });
        })
        .catch(() => setNotice(t("workbenchClient.saveFailedNetwork")))
        .finally(() => setWriting((current) => {
          const next = new Set(current);
          next.delete(key);
          return next;
        }));
    };
    const send = (key: string, action: WorkbenchAction, values: { row?: any; form?: Record<string, any> }) => {
      if (action.table && values.form) return saveToTable(key, action, values.form);
      if (values.row && rowWriteOf(action)) return writeRow(key, action, values.row);
      sendPrompt(key, action, values);
    };
    const docPane = app.workbench?.panes.find((item) => item.type === "kb_doc");
    const handlers: PanelHandlers = {
      sending: writing.size ? new Set([...sending, ...writing]) : sending,
      armed: (key) => armed === key,
      send,
      refresh: (index) => void load(index),
      formValues: (index) => forms[index] ?? {},
      setFormValue: (index, name, value) => setForms((current) => ({ ...current, [index]: { ...(current[index] ?? {}), [name]: value } })),
      formError: (index) => formErrors[index] ?? "",
      setFormError: (index, error) => setFormErrors((current) => ({ ...current, [index]: error })),
      selectedKBs,
      toggleKB: (kb) => setSelectedKBs((current) => (current.includes(kb) ? current.filter((item) => item !== kb) : [...current, kb])),
      checked: (index) => checks[checklistKey(pane.id, index)] ?? [],
      toggleCheck,
      expandedRow: (index) => expanded[index],
      toggleRow: (index, row) => setExpanded((current) => ({ ...current, [index]: current[index] === row ? undefined : row })),
      // 无权限时「去申请」：dsh-ee 网页上这个连接的页面（有申请入口）
      requestHref: (connection) => (docBase ? `${docBase}/connections/${encodeURIComponent(connection)}?app=${encodeURIComponent(app.name)}` : undefined),
      openCitation: docPane ? (citation: Citation) => {
        docTargets.set(app.name, { doc: citation.doc, version: citation.version, from: citation.from, to: citation.to, quote: citation.quote, title: citation.title });
        try {
          ctx.sidebarRight.openResource(workbenchAddress(app.name, docPane.id), { preferNewPane: true });
        } catch {
          // 打不开新窗格时，原文窗格如果已经开着，也会跟着切过去
        }
      } : undefined,
      // 没有原文窗格（默认）：出处是链接，点开在 dsh-ee 网页上看
      citationHref: docPane ? undefined : (citation: Citation) => kbDocUrl(docBase, citation),
    };
    return createElement("div", { onClickCapture: onLinkClick, style: { display: "contents" } }, panelView(createElement, pane, { data, board, notice }, handlers));
  };

  const DocPane = ({ app }: { app: CatalogApp }) => {
    const [target, setTarget] = useState<DocTarget | undefined>(docTargets.get(app.name));
    const [range, setRange] = useState<{ from: number; to: number } | undefined>(target ? docWindow(target.from) : undefined);
    const [state, setState] = useState<WidgetState | undefined>(undefined);
    const [raw, setRaw] = useState(false);
    const { sending, notice, send } = useSending();
    const scrolled = useRef("");
    useEffect(() => docTargets.subscribe(() => {
      const next = docTargets.get(app.name);
      setTarget(next);
      if (next) setRange(docWindow(next.from));
    }), [app.name]);
    useEffect(() => {
      if (!target || !range) return;
      let live = true;
      setState({ status: "loading" });
      remote.kbDoc({ doc: target.doc, version: target.version, from: range.from, to: range.to })
        .then((raw: any) => {
          if (!live) return;
          const result = unwrapRemote(raw);
          setState(result?.forbidden ? { status: "forbidden" } : result?.error ? { status: "error", error: String(result.error) } : { status: "ready", value: result?.value });
        })
        .catch((error: unknown) => { if (live) setState({ status: "error", error: error instanceof Error ? error.message : t("workbenchClient.networkError") }); });
      return () => { live = false; };
    }, [target, range?.from, range?.to]);
    // 每条引用第一次显示出来时，滚到高亮的那一行
    useEffect(() => {
      if (state?.status !== "ready" || !target) return;
      const key = `${target.doc}:${target.from}:${target.quote ?? ""}`;
      if (scrolled.current === key) return;
      scrolled.current = key;
      const hit = typeof document !== "undefined" ? document.querySelector('[data-dsh-ee-ui="kb-doc"] [data-kb-hit="1"]') : null;
      (hit as any)?.scrollIntoView?.({ block: "center" });
    }, [state, target]);
    const value: any = state?.status === "ready" ? state.value : undefined;
    const view = kbDocView(createElement, target, state, {
      page: (delta) => setRange((current) => {
        if (!current) return current;
        const size = current.to - current.from + 1;
        const from = Math.max(1, current.from + delta * size);
        return { from, to: from + size - 1 };
      }),
      asking: sending.has("ask"),
      raw,
      toggleRaw: () => setRaw((v: boolean) => !v),
      ask: () => {
        if (!target) return;
        const title = value?.title || target.title || t("workbenchClient.docLabel", { doc: target.doc });
        send("ask", t("workbenchClient.askPrompt", { title, doc: target.doc, from: target.from, to: target.to }), t("workbenchClient.ask"));
      },
    });
    return notice ? createElement("div", { style: { height: "100%", display: "grid", gridTemplateRows: "auto 1fr" } },
      createElement("p", { role: "status", style: { margin: "8px 12px 0", fontSize: 12 } }, notice), view) : view;
  };

  const WorkbenchTab = (props: any) => {
    const info = props.useTabInfo?.();
    const address = info?.tab?.navigation?.address || info?.tab?.contentId || "";
    const visible = info?.tab?.visible !== false;
    const parsed = parseWorkbenchAddress(address);
    const { apps, loaded } = useAppsList();
    const app = apps.find((item) => item.name === parsed?.app);
    const pane = app?.workbench?.panes.find((item) => item.id === parsed?.pane && (item.type === "panel" || item.type === "kb_doc"));
    if (!app || !pane) return loaded ? missingPaneView(createElement) : createElement("div", { style: { padding: 16, fontSize: 13, opacity: 0.65 } }, t("common.loading"));
    if (pane.type === "kb_doc") return createElement(DocPane, { key: `${app.name}/${pane.id}`, app });
    return createElement(PanelPane, { key: `${app.name}/${pane.id}`, app, pane, info, visible });
  };

  ctx.effect(() => ctx.sidebarRightTabs.register({
    id: workbenchTabId,
    kind: workbenchKind,
    patterns: [`${workbenchScheme}**`],
    keepMounted: true,
    title: titleOf,
  }));
  ctx.slots.inject("sidebar.right.pane.tab", () => ctx.slots.register({ name: "sidebar.right.pane.tab", key: workbenchTabId }, WorkbenchTab));

  return {
    openWorkbench: (app: CatalogApp, sessionId: string, force = false) => openWorkbench(ctx, app, sessionId, force),
  };
}
