import { readSidebarMode, setSidebarMode, subscribeSidebarMode, type SidebarAppsMode } from "./sidebar-pref.js";
import type { ConnectUpdate } from "./connect-update.js";
import type { ServerSettings, ServerTest } from "./server-settings.js";
import { t } from "./i18n/index.js";

// DSH 设置面板里的「dsh-ee」页：看、改、测试接入插件连的 dsh-ee 地址，以及一键更新接入插件。

export const settingsSectionId = "dsh-ee";

export interface ServerSettingsState {
  settings?: ServerSettings;
  draft: string;
  busy?: "test" | "save" | "update";
  test?: ServerTest;
  update?: ConnectUpdate;
  notice?: string;
  error?: string;
}

export interface ServerSettingsActions {
  setDraft: (value: string) => void;
  test: () => void;
  save: () => void;
  reset: () => void;
  update: () => void;
}

const muted = { opacity: 0.65 };

function button(createElement: (...args: any[]) => any, label: string, onClick: () => void, disabled: boolean, primary = false) {
  return createElement("button", {
    type: "button",
    onClick,
    disabled,
    style: {
      padding: "6px 12px", borderRadius: 6, cursor: disabled ? "default" : "pointer", opacity: disabled ? 0.55 : 1,
      border: "1px solid currentColor", color: "inherit", fontWeight: primary ? 600 : 400,
      background: primary ? "color-mix(in srgb, currentColor 12%, transparent)" : "transparent",
    },
  }, label);
}

export function serverSettingsView(createElement: (...args: any[]) => any, state: ServerSettingsState, actions: ServerSettingsActions) {
  const { settings, draft, busy, test, notice, error } = state;
  const children: any[] = [
    createElement("h2", { key: "title", style: { margin: "0 0 6px", fontSize: 16 } }, t("settingsView.title")),
    createElement("p", { key: "desc", style: { margin: "0 0 18px", lineHeight: 1.6, ...muted } },
      t("settingsView.description")),
  ];
  if (!settings) {
    children.push(createElement("p", { key: "loading" }, error || t("settingsView.reading")));
    return createElement("div", { "data-dsh-ee-settings": "loading", style: { maxWidth: 640 } }, ...children);
  }
  children.push(createElement("label", { key: "label", htmlFor: "dsh-ee-base", style: { display: "block", marginBottom: 6, fontWeight: 500 } }, t("settingsView.addressLabel")));
  children.push(createElement("input", {
    key: "input",
    id: "dsh-ee-base",
    type: "url",
    value: draft,
    placeholder: "https://ee.example.com",
    spellCheck: false,
    onChange: (event: any) => actions.setDraft(event.target.value),
    onKeyDown: (event: any) => { if (event.key === "Enter") actions.test(); },
    style: { width: "100%", boxSizing: "border-box", padding: "8px 10px", borderRadius: 6, border: "1px solid color-mix(in srgb, currentColor 30%, transparent)", background: "transparent", color: "inherit", font: "inherit" },
  }));
  children.push(createElement("p", { key: "packaged", style: { margin: "6px 0 0", fontSize: 12, lineHeight: 1.6, ...muted } },
    settings.overridden ? t("settingsView.packagedOverridden", { packaged: settings.packaged }) : t("settingsView.packagedDefault")));
  const changed = draft.trim().replace(/\/+$/, "") !== settings.base;
  children.push(createElement("div", { key: "actions", style: { display: "flex", gap: 8, marginTop: 14, flexWrap: "wrap" } },
    button(createElement, busy === "test" ? t("settingsView.testing") : t("settingsView.testConnection"), actions.test, !!busy),
    button(createElement, busy === "save" ? t("settingsView.saving") : t("common.save"), actions.save, !!busy || !changed, true),
    settings.overridden ? button(createElement, t("settingsView.resetAddress"), actions.reset, !!busy) : null,
  ));
  if (test) {
    children.push(createElement("ul", { key: "checks", "data-dsh-ee-test": test.ok ? "ok" : "fail", style: { listStyle: "none", padding: 0, margin: "16px 0 0", display: "grid", gap: 8 } },
      ...test.checks.map((check) => createElement("li", { key: check.id, style: { display: "flex", gap: 8, lineHeight: 1.5 } },
        createElement("span", { style: { color: check.ok ? "#16a34a" : "#d92d20", fontWeight: 600 } }, check.ok ? "✓" : "✕"),
        createElement("span", {}, check.title, check.detail ? createElement("span", { style: { display: "block", fontSize: 12, ...muted } }, check.detail) : null),
      )),
    ));
  }
  if (notice) children.push(createElement("p", { key: "notice", style: { marginTop: 14, lineHeight: 1.6 } }, notice));
  if (error) children.push(createElement("p", { key: "error", style: { marginTop: 14, color: "#d92d20", lineHeight: 1.6 } }, error));
  children.push(createElement("div", { key: "plugin", "data-dsh-ee-settings": "plugin-update", style: { marginTop: 20, paddingTop: 16, borderTop: "1px solid color-mix(in srgb, currentColor 14%, transparent)" } },
    createElement("div", { style: { display: "flex", alignItems: "center", gap: 10, flexWrap: "wrap" } },
      createElement("span", { style: { fontWeight: 500 } }, t("settingsView.plugin")),
      createElement("span", { style: { fontSize: 12, ...muted } }, t("settingsView.currentVersion", { version: settings.version })),
      button(createElement, busy === "update" ? t("settingsView.updating") : t("settingsView.updateLatest"), actions.update, !!busy)),
    state.update ? createElement("p", { "data-dsh-ee-update": state.update.ok ? "ok" : "fail", style: { margin: "8px 0 0", fontSize: 12, lineHeight: 1.6, color: state.update.ok ? "inherit" : "#d92d20" } }, state.update.message) : null,
    createElement("p", { style: { margin: "6px 0 0", fontSize: 12, lineHeight: 1.6, ...muted } }, t("settingsView.updateHint"))));
  children.push(createElement("p", { key: "hint", style: { marginTop: 22, fontSize: 12, lineHeight: 1.6, ...muted } },
    t("settingsView.changeHint")));
  return createElement("div", { "data-dsh-ee-settings": "ready", style: { maxWidth: 640 } }, ...children);
}

/** 在 DSH 设置面板里注册「dsh-ee」页。onSaved 在地址生效后调用（刷新企业面板等）。 */
/** 「在左侧栏显示企业应用」：侧栏里点了「隐藏」以后，从这里打开回来。 */
export function sidebarAppsToggle(createElement: (...args: any[]) => any, mode: SidebarAppsMode, onChange: (shown: boolean) => void) {
  return createElement("div", { "data-dsh-ee-settings": "sidebar-apps", style: { maxWidth: 640, marginTop: 20, paddingTop: 16, borderTop: "1px solid color-mix(in srgb, currentColor 14%, transparent)" } },
    createElement("label", { style: { display: "flex", alignItems: "center", gap: 8, fontWeight: 500, cursor: "pointer" } },
      createElement("input", { type: "checkbox", checked: mode !== "hidden", onChange: () => onChange(mode === "hidden") }),
      t("appsView.showInSidebar")),
    createElement("p", { style: { margin: "6px 0 0", fontSize: 12, lineHeight: 1.6, ...muted } }, t("settingsView.sidebarHint")));
}

export function registerServerSettings(ctx: any, deps: {
  createElement: (...args: any[]) => any;
  useState: <T>(value: T) => [T, (value: T | ((prev: T) => T)) => void];
  useEffect: (fn: () => void | (() => void), deps: unknown[]) => void;
  remote: any;
  unwrapRemote: (value: any) => any;
  label: string;
  onSaved: () => void;
}) {
  const { createElement, useState, useEffect, remote, unwrapRemote } = deps;
  const message = (error: unknown) => (error instanceof Error ? error.message : String(error));
  const Section = () => {
    const [state, setState] = useState<ServerSettingsState>({ draft: "" });
    const patch = (next: Partial<ServerSettingsState>) => setState((prev) => ({ ...prev, ...next }));
    useEffect(() => {
      void (async () => {
        try {
          const settings = unwrapRemote(await remote.serverSettings()) as ServerSettings;
          patch({ settings, draft: settings.base });
        } catch (error) {
          patch({ error: t("settingsView.readFailed", { error: message(error) }) });
        }
      })();
    }, []);
    const test = async () => {
      patch({ busy: "test", test: undefined, notice: undefined, error: undefined });
      try {
        patch({ test: unwrapRemote(await remote.testServer({ base: state.draft })) });
      } catch (error) {
        patch({ error: t("settingsView.testFailed", { error: message(error) }) });
      } finally {
        patch({ busy: undefined });
      }
    };
    const save = async (base: string | null) => {
      patch({ busy: "save", notice: undefined, error: undefined });
      try {
        const settings = unwrapRemote(await remote.saveServer({ base })) as ServerSettings;
        patch({ settings, draft: settings.base, notice: t("settingsView.savedNotice") });
        deps.onSaved();
      } catch (error) {
        patch({ error: t("settingsView.saveFailed", { error: message(error) }) });
      } finally {
        patch({ busy: undefined });
      }
    };
    const update = async () => {
      patch({ busy: "update", update: undefined, error: undefined });
      try {
        patch({ update: unwrapRemote(await remote.updatePlugin()) });
      } catch (error) {
        patch({ update: { ok: false, version: state.settings?.version ?? "", message: t("settingsView.updateFailed", { error: message(error) }) } });
      } finally {
        patch({ busy: undefined });
      }
    };
    const [mode, setMode] = useState<SidebarAppsMode>(readSidebarMode());
    useEffect(() => subscribeSidebarMode(() => setMode(readSidebarMode())), []);
    return createElement("div", null,
      serverSettingsView(createElement, state, {
        setDraft: (draft) => patch({ draft, test: undefined }),
        test: () => void test(),
        save: () => void save(state.draft),
        reset: () => void save(null),
        update: () => void update(),
      }),
      sidebarAppsToggle(createElement, mode, (shown) => setSidebarMode(shown ? "shown" : "hidden")));
  };
  ctx.slots.inject("settings.section", () => ctx.slots.register(
    { name: "settings.section", id: settingsSectionId, order: 80, label: () => deps.label },
    Section,
  ));
}
