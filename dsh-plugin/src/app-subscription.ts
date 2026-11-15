import type { HttpClient } from "./http.js";
import { t } from "./i18n/index.js";

const nameRe = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;

/** 能在 DSH 里自己启用、停用的三类能力，对应服务端 /api/dsh/<kind>/{name}/subscription。 */
export type SubscriptionKind = "apps" | "skills" | "connections";

const invalidNameKey = {
  apps: "appSubscription.invalidAppName",
  skills: "appSubscription.invalidSkillName",
  connections: "appSubscription.invalidConnectionName",
} as const satisfies Record<SubscriptionKind, string>;

/**
 * 在 DSH 里启用或停用一项「自选」能力（与网页上的启用是同一件事）。
 * 只给接入插件的界面用，不注册成工具：模型不能替员工改这项配置。
 * 成功后由调用方立刻刷新目录，让 /名字 或连接的工具马上可用或消失。
 */
export async function setSubscription(http: HttpClient, kind: SubscriptionKind, name: string, enabled: boolean): Promise<{ ok: true } | { ok: false; error: string }> {
  if (!(kind in invalidNameKey)) return { ok: false, error: t("appSubscription.invalidKind") };
  if (typeof name !== "string" || !nameRe.test(name) || name.length > 48) return { ok: false, error: t(invalidNameKey[kind]) };
  const response = await http.fetch(`/api/dsh/${kind}/${encodeURIComponent(name)}/subscription`, { method: enabled ? "POST" : "DELETE" });
  if (response.ok) return { ok: true };
  let error = `HTTP ${response.status}`;
  try {
    const body = await response.json() as { error?: string };
    if (body?.error) error = body.error;
  } catch {
    // 不是 JSON 就用状态码
  }
  return { ok: false, error };
}

/** 旧入口：只启用、停用微应用（含专家）。 */
export function setAppSubscription(http: HttpClient, name: string, enabled: boolean) {
  return setSubscription(http, "apps", name, enabled);
}
