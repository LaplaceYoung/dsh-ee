/** 审计日志的展示：动作名翻译、颜色、详情摘要。 */
import type { TFunction } from "i18next";
import type { AuditLog } from "./api";

export const AUDIT_CATEGORIES = [
  "auth",
  "user",
  "role",
  "permission",
  "menu",
  "provider",
  "model",
  "files",
  "key",
  "settings",
  "ldap",
  "oidc",
  "package",
  "review",
  "upstream",
  "npm_token",
  "token",
  "device",
  "asset",
  "connection",
  "extension",
  "connect",
  "brand",
  "kb",
  "app_data",
  "data",
  "dsh_release",
  "embed",
  "directory",
  "client",
] as const;

/** 动作名；没有翻译的新动作原样显示。 */
export function auditAction(t: TFunction, action: string) {
  return t(`auditLog.actions.${action}`, { defaultValue: action });
}

export function auditTone(action: string): "slate" | "brand" | "green" | "amber" | "red" {
  if (action === "auth.login_failed" || action.endsWith(".delete") || action.endsWith(".revoke") || action === "review.reject") return "red";
  if (action === "review.approve") return "green";
  if (action === "package.publish" || action === "package.import_request") return "amber";
  if (action.startsWith("auth.")) return "slate";
  return "brand";
}

export function auditActor(t: TFunction, l: AuditLog) {
  if (l.actor_id === null) return l.action.startsWith("auth.") ? t("auditLog.anonymous") : t("auditLog.system");
  return l.actor_display_name ? `${l.actor_display_name}（${l.actor_name}）` : l.actor_name;
}

/** 详情摘要：key=value，数组用顿号连接，对象压成 JSON。 */
export function auditDetail(l: AuditLog) {
  if (!l.detail) return "";
  return Object.entries(l.detail)
    .filter(([, v]) => v !== null && v !== "" && !(Array.isArray(v) && v.length === 0))
    .map(([k, v]) => `${k}=${Array.isArray(v) ? v.join("、") : typeof v === "object" ? JSON.stringify(v) : String(v)}`)
    .join(" · ");
}
