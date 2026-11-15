import os from "node:os";
import path from "node:path";
import type { DshContext, PluginConfig } from "./dsh.js";
import { t } from "./i18n/index.js";

export function normalizeConfig(input: Partial<PluginConfig>): PluginConfig {
  const base = String(input.base ?? "").replace(/\/+$/, "");
  if (!/^https?:\/\//.test(base)) throw new Error(t("config.badBase"));
  return {
    base,
    org: input.org || t("config.defaultOrg"),
    serverName: input.serverName || "ee",
    tokenRef: input.tokenRef || "DSHEE_TOKEN",
    autoInstall: input.autoInstall !== false,
    version: input.version || "dev",
    scopes: Array.isArray(input.scopes) ? input.scopes.filter((s) => typeof s === "string" && s.startsWith("@")) : [],
  };
}

export function dshHome(): string {
  return process.env.DSH_HOME || path.join(os.homedir(), ".dsh");
}

/**
 * 企业源配置写在哪个 .npmrc：DSH 跑 pnpm 时的工作目录是 profile 目录，profile 里的 .npmrc 只对 DSH 生效，
 * 不影响员工自己的 npm / pnpm。宿主没有提供 profileContext 时才退回用户级 .npmrc。
 */
export function npmrcPath(ctx: DshContext): string {
  const dir = ctx.get("profileContext")?.dir;
  if (typeof dir === "string" && dir) return path.join(dir, ".npmrc");
  return process.env.NPM_CONFIG_USERCONFIG || path.join(os.homedir(), ".npmrc");
}

/** dsh-ee 在 .npmrc 里管的几行：各企业作用域指向 dsh-ee 源，以及源的令牌。键 → 值。 */
export function npmrcEntries(config: PluginConfig, token: string): Map<string, string> {
  const registry = `${config.base}/npm/`;
  const entries = new Map<string, string>();
  for (const scope of config.scopes) entries.set(`${scope}:registry`, registry);
  entries.set(`${registry.replace(/^https?:/, "")}:_authToken`, token);
  return entries;
}
