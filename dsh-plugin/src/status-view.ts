import type { EnterpriseOverview, OverviewFailure, UsageViewData } from "./overview.js";
import { t } from "./i18n/index.js";

export const enterpriseTabId = "__DSH_EE_PACKAGE__";

export interface DoctorViewData {
  summary: string;
  checks: { id: string; status: "ok" | "warn" | "fail"; title: string; detail?: string; fix?: string }[];
}

export interface EnterprisePanelState {
  data?: EnterpriseOverview;
  failed: boolean;
  failure?: OverviewFailure;
  busy?: boolean;
  notice?: string;
  doctor?: DoctorViewData;
}

export interface EnterprisePanelActions {
  login: () => void;
  refresh: () => void;
  doctor: () => void;
}

export interface QuotaViewData {
  used: number;
  limit: number;
  remainingPercent: number;
  usedRatio: number;
}

export function effectiveQuota(usage?: UsageViewData): QuotaViewData | null | undefined {
  if (!usage) return undefined;
  const quotas = [usage.quota, ...(usage.current_token ? [usage.current_token] : [])].filter((quota) => quota.monthly_token_limit !== null && quota.monthly_token_limit !== undefined);
  if (!quotas.length) return null;
  const selected = quotas.reduce((best, quota) => {
    const limit = quota.monthly_token_limit as number;
    const remaining = Math.max(0, limit - quota.month_tokens);
    const bestRemaining = Math.max(0, (best.monthly_token_limit as number) - best.month_tokens);
    return remaining < bestRemaining ? quota : best;
  });
  const limit = selected.monthly_token_limit as number;
  const used = selected.month_tokens;
  return {
    used,
    limit,
    remainingPercent: limit > 0 ? Math.max(0, Math.floor(((limit - used) / limit) * 100)) : 0,
    usedRatio: limit > 0 ? Math.min(1, Math.max(0, used / limit)) : 1,
  };
}

export function formatTokens(value: number): string {
  if (value >= 1_000_000) return `${Number((value / 1_000_000).toFixed(1))}m`;
  if (value >= 1_000) return `${Number((value / 1_000).toFixed(1))}k`;
  return String(value);
}

export function adminRemaining(me: EnterpriseOverview["me"], now = Date.now()): string {
  if (!me.admin_live || !me.admin_until) return me.admin_live ? t("statusView.adminOn") : t("statusView.adminOff");
  const minutes = Math.max(0, Math.ceil((new Date(me.admin_until).getTime() - now) / 60_000));
  if (minutes >= 60) return t("statusView.adminRemainingHours", { hours: Math.floor(minutes / 60), minutes: minutes % 60 });
  return t("statusView.adminRemainingMinutes", { minutes });
}

function button(createElement: (...args: any[]) => any, label: string, onClick: () => void, busy = false) {
  return createElement("button", {
    type: "button",
    onClick,
    disabled: busy,
    style: { padding: "6px 10px", border: "1px solid currentColor", borderRadius: 6, background: "transparent", color: "inherit", cursor: busy ? "wait" : "pointer", opacity: busy ? 0.55 : 1 },
  }, label);
}

function section(createElement: (...args: any[]) => any, title: string, ...children: any[]) {
  return createElement("section", { style: { marginTop: 18, paddingTop: 14, borderTop: "1px solid color-mix(in srgb, currentColor 14%, transparent)" } },
    createElement("h3", { style: { margin: "0 0 10px", fontSize: 13 } }, title),
    ...children,
  );
}

function failureText(failure?: OverviewFailure): string {
  switch (failure?.reason) {
    case "auth": return t("statusView.failureAuth");
    case "server": return t("statusView.failureServer", { detail: failure.detail });
    case "unreachable": return t("statusView.failureUnreachable", { base: failure.base, detail: failure.detail });
    default: return t("statusView.failureDefault");
  }
}

export function enterprisePanel(createElement: (...args: any[]) => any, org: string, state: EnterprisePanelState, actions: EnterprisePanelActions, AppsPanel?: any) {
  const { data, failed, failure, busy, notice, doctor } = state;
  const me = data?.me;
  const quota = effectiveQuota(data?.usage);
  const children: any[] = [createElement("h2", { key: "title", style: { margin: "0 0 16px", fontSize: 16 } }, org)];
  if (!data || !me) {
    children.push(createElement("p", { key: "state", style: { lineHeight: 1.6 } }, failed ? failureText(failure) : t("statusView.reading")));
    if (failed) children.push(createElement("div", { key: "actions", style: { display: "flex", gap: 8 } },
      button(createElement, busy ? t("statusView.startingLogin") : t("common.login"), actions.login, busy),
      button(createElement, t("common.refresh"), actions.refresh, busy),
    ));
  } else {
    const rows = [
      [t("statusView.account"), me.username],
      [t("statusView.department"), me.department || t("common.notSet")],
      [t("statusView.tokenExpiry"), me.expires_at ? t("statusView.expiresOn", { date: me.expires_at.slice(0, 10) }) : t("statusView.neverExpires")],
      [t("statusView.adminMode"), adminRemaining(me)],
    ];
    children.push(createElement("dl", { key: "account", style: { margin: 0, display: "grid", gridTemplateColumns: "80px 1fr", gap: "12px 8px" } },
      ...rows.flatMap(([label, value]) => [
        createElement("dt", { key: `${label}-label`, style: { opacity: 0.65 } }, label),
        createElement("dd", { key: `${label}-value`, style: { margin: 0 } }, value),
      ]),
    ));
    if (AppsPanel) children.push(createElement(AppsPanel, { key: "apps" }));
    children.push(section(createElement, t("statusView.quota"),
      quota === undefined ? t("statusView.quotaUnavailable") : quota === null ? t("statusView.quotaUnlimited") : createElement("div", {},
        createElement("div", { style: { display: "flex", justifyContent: "space-between", marginBottom: 7 } },
          createElement("span", {}, `${formatTokens(quota.used)} / ${formatTokens(quota.limit)}`),
          createElement("span", { style: { color: quota.usedRatio >= 0.9 ? "#d97706" : "inherit" } }, t("statusView.quotaRemaining", { percent: quota.remainingPercent })),
        ),
        createElement("div", { role: "progressbar", "aria-valuemin": 0, "aria-valuemax": 100, "aria-valuenow": Math.round(quota.usedRatio * 100), style: { height: 6, borderRadius: 3, overflow: "hidden", background: "color-mix(in srgb, currentColor 12%, transparent)" } },
          createElement("div", { style: { width: `${quota.usedRatio * 100}%`, height: "100%", background: quota.usedRatio >= 0.9 ? "#d97706" : "#2563eb" } }),
        ),
      ),
    ));
    children.push(section(createElement, t("statusView.models"),
      createElement("div", {}, t("statusView.defaultModel", { model: data.default_model || t("common.notSet") })),
      createElement("div", { style: { marginTop: 6, opacity: 0.75, lineHeight: 1.5 } }, data.models.length ? data.models.join(t("common.listSeparator")) : t("statusView.modelsUnavailable")),
    ));
    children.push(section(createElement, t("statusView.status"),
      createElement("div", { style: { lineHeight: 1.6 } }, t("statusView.pluginVersion", { version: data.version, latest: data.latest_version && data.latest_version !== data.version ? t("statusView.pluginNewVersion", { version: data.latest_version }) : t("statusView.pluginUpToDate") })),
      createElement("div", { style: { display: "flex", gap: 8, marginTop: 10 } },
        button(createElement, busy ? t("statusView.diagnosing") : t("statusView.diagnose"), actions.doctor, busy),
        button(createElement, t("common.refresh"), actions.refresh, busy),
      ),
    ));
  }
  if (notice) children.push(createElement("p", { key: "notice", role: "status", style: { lineHeight: 1.6, overflowWrap: "anywhere" } }, notice));
  if (doctor) children.push(section(createElement, t("statusView.diagnoseResult", { summary: doctor.summary }),
    createElement("ul", { style: { margin: 0, paddingLeft: 18, lineHeight: 1.6 } },
      ...doctor.checks.map((check) => createElement("li", { key: check.id, style: { color: check.status === "ok" ? "inherit" : check.status === "warn" ? "#d97706" : "#d92d20" } }, t("statusView.checkLine", { title: check.title, fix: check.fix ? t("statusView.checkFixSeparator") + check.fix : "" }))),
    ),
  ));
  return createElement("main", {
    "data-dsh-ee-ui": "panel",
    "data-dsh-ee-state": data ? "connected" : failed ? "disconnected" : "loading",
    style: { padding: 16, color: "inherit", fontSize: 13 },
  }, ...children);
}

export function enterpriseHeader(createElement: (...args: any[]) => any, data: EnterpriseOverview | undefined, failed: boolean, onClick: () => void) {
  const me = data?.me;
  const quota = effectiveQuota(data?.usage);
  const label = !me ? failed ? t("statusView.headerDisconnected") : t("statusView.headerChecking") : me.admin_live ? t("statusView.headerAdmin", { remaining: adminRemaining(me) }) : quota ? t("statusView.headerQuota", { percent: quota.remainingPercent }) : t("statusView.headerUser", { username: me.username });
  return createElement("button", {
    type: "button",
    onClick,
    "data-dsh-ee-ui": "header",
    "data-dsh-ee-state": me ? "connected" : failed ? "disconnected" : "loading",
    "aria-label": label,
    title: me ? t("statusView.headerTitleUser", { username: me.username, department: me.department ? t("statusView.headerDepartment", { department: me.department }) : "" }) : t("statusView.headerTitle"),
    style: { whiteSpace: "nowrap", fontSize: 12, color: me ? "inherit" : failed ? "#d92d20" : "inherit", background: "transparent", border: 0, padding: 0, cursor: "pointer" },
  }, label);
}

export function registerEnterpriseStatus(ctx: any, org: string, EnterprisePanel: any, EnterpriseHeader: any): void {
  ctx.effect(() => ctx.sidebarRightTabs.register({
    id: enterpriseTabId,
    kind: "dsh-ee-enterprise",
    title: () => org,
    guide: [{ id: "open", order: 10, title: () => org, description: () => t("statusView.guideDescription") }],
  }));
  ctx.slots.inject("sidebar.right.pane.tab", () => ctx.slots.register(
    { name: "sidebar.right.pane.tab", key: enterpriseTabId },
    EnterprisePanel,
  ));
  ctx.slots.inject("conversation.session.header.utilities", () => ctx.slots.register(
    { name: "conversation.session.header.utilities", id: "dsh-ee-status", order: 100, label: org },
    EnterpriseHeader,
  ));
}
