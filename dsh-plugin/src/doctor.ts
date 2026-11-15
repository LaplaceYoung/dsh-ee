import { npmrcPath } from "./config.js";
import { npmrcState } from "./token.js";
import { t } from "./i18n/index.js";
import type { McpBridge } from "./bridge.js";
import type { DshContext, PluginConfig } from "./dsh.js";
import type { HttpClient } from "./http.js";
import type { TokenManager } from "./token.js";
import type { AppBindings } from "./app-bindings.js";

export interface Check {
  id: string;
  status: "ok" | "warn" | "fail";
  title: string;
  detail?: string;
  fix?: string;
}

/** 比较两个 x.y.z[-pre] 版本，a 比 b 新返回正数。 */
export function compareVersions(a: string, b: string): number {
  const parse = (v: string) => {
    const [core, pre] = v.replace(/^v/, "").split("-", 2);
    return { nums: core.split(".").map((n) => Number.parseInt(n, 10) || 0), pre };
  };
  const x = parse(a);
  const y = parse(b);
  for (let i = 0; i < Math.max(x.nums.length, y.nums.length); i++) {
    const d = (x.nums[i] ?? 0) - (y.nums[i] ?? 0);
    if (d) return d;
  }
  if (x.pre === y.pre) return 0;
  if (!x.pre) return 1;
  if (!y.pre) return -1;
  return x.pre < y.pre ? -1 : 1;
}

export async function runLocalDoctor(ctx: DshContext, config: PluginConfig, tokens: TokenManager, http: HttpClient, bridge: McpBridge, bindings?: AppBindings): Promise<{ summary: string; checks: Check[] }> {
  const checks: Check[] = [];
  const add = (check: Check) => checks.push(check);
  if (bindings?.warning()) add({ id: "app_bindings", status: "warn", title: bindings.warning(), fix: t("doctor.appBindingsFix") });

  const current = await tokens.resolve();
  if (!current?.value) {
    add({ id: "credential", status: "fail", title: t("doctor.noCredential"), fix: t("doctor.loginFix") });
    return { summary: summarize(checks), checks };
  }
  add({ id: "credential", status: "ok", title: t("doctor.hasCredential"), detail: t("doctor.source", { source: current.source }) });

  // 令牌是否还有效
  let reachable = true;
  try {
    const response = await http.fetch("/api/dsh/whoami");
    if (response.ok) {
      const me = await response.json() as { username: string; department: string; expires_at?: string };
      const days = me.expires_at ? (new Date(me.expires_at).getTime() - Date.now()) / 86_400_000 : Infinity;
      add(days < 7
        ? { id: "token", status: "warn", title: t("doctor.expiringSoon", { days: Math.max(0, Math.ceil(days)) }), detail: me.username, fix: t("doctor.reLoginFix") }
        : { id: "token", status: "ok", title: t("doctor.signedInAs", { username: me.username, department: me.department }) });
    } else {
      const body = await response.json().catch(() => null) as { code?: string } | null;
      if (response.status === 401) {
        add({ id: "token", status: "fail", title: t("doctor.loginExpired"), detail: body?.code ?? "HTTP 401", fix: t("doctor.reLoginFix2") });
      } else {
        add({ id: "token", status: "fail", title: t("doctor.readAccountFailed", { status: response.status }), detail: t("doctor.serverRestarting"), fix: t("doctor.retryDiagnose") });
      }
    }
  } catch {
    reachable = false;
    add({ id: "token", status: "fail", title: t("doctor.unreachable", { base: config.base }), fix: t("doctor.unreachableFix") });
  }

  // npm 源令牌
  const npmrc = npmrcState(ctx, config, current.value);
  add(npmrc === "ok"
    ? { id: "npmrc", status: "ok", title: t("doctor.npmrcOk"), detail: npmrcPath(ctx) }
    : {
      id: "npmrc",
      status: "warn",
      title: npmrc === "missing" ? t("doctor.npmrcMissing") : t("doctor.npmrcMismatch"),
      detail: t("doctor.npmrcDetail", { path: npmrcPath(ctx) }),
      fix: t("doctor.npmrcFix"),
    });

  if (!reachable) return { summary: summarize(checks), checks };

  // 模型
  let models: string[] = [];
  try {
    const response = await http.fetch("/v1/models");
    if (response.ok) models = ((await response.json()) as { data: { id: string }[] }).data.map((m) => m.id);
    add(models.length
      ? { id: "models", status: "ok", title: t("doctor.modelsAvailable", { models: models.join(t("common.listSeparator")) }) }
      : { id: "models", status: "fail", title: response.ok ? t("doctor.noModels") : t("doctor.readModelsHttp", { status: response.status }), fix: t("doctor.modelPermissionFix") });
  } catch {
    add({ id: "models", status: "fail", title: t("doctor.readModelsFailed") });
  }

  // 服务端检查（账号、额度、模型、目录），以及版本与必装插件的比对依据
  let server: any;
  try {
    const result = await bridge.callTool("ee_doctor", { action: "check" });
    if (!result?.isError) server = JSON.parse(result.content[0].text);
  } catch {
    // Agent 关闭或令牌没有 agent 范围：服务端检查不可用，不算失败
  }
  if (server) {
    for (const c of server.checks ?? []) add({ ...c, id: `server.${c.id}` });
    if (server.default_model && models.length && !models.includes(server.default_model)) {
      add({ id: "default_model", status: "warn", title: t("doctor.defaultModelMissing", { model: server.default_model }), fix: t("doctor.chooseModelFix") });
    }
    if (server.connect_version && config.version !== "dev") {
      add(compareVersions(server.connect_version, config.version) > 0
        ? { id: "plugin_version", status: "warn", title: t("doctor.pluginNewer", { latest: server.connect_version, version: config.version }), fix: t("doctor.updateFix") }
        : { id: "plugin_version", status: "ok", title: t("doctor.pluginLatest", { version: config.version }) });
    }
    const manager = ctx.get("pluginManager");
    if (manager && (server.required_plugins ?? []).length) {
      try {
        const have = new Set((await manager.listBundles()).filter((b: any) => b.installed || b.enabled).map((b: any) => b.name));
        const missing = server.required_plugins.map((p: any) => p.name).filter((n: string) => !have.has(n));
        add(missing.length
          ? { id: "required_plugins", status: "warn", title: t("doctor.missingPlugins", { plugins: missing.join(t("common.listSeparator")) }), fix: t("doctor.missingPluginsFix") }
          : { id: "required_plugins", status: "ok", title: t("doctor.pluginsOk") });
      } catch {
        // 读不到插件列表就不下结论
      }
    }
  }

  // 桥
  add(bridge.status.error
    ? { id: "bridge", status: "fail", title: t("doctor.bridgeFailed"), detail: bridge.status.error, fix: t("doctor.bridgeFix") }
    : { id: "bridge", status: "ok", title: t("doctor.bridgeOk", { count: bridge.status.tools }) });

  return { summary: summarize(checks), checks };
}

function summarize(checks: Check[]): string {
  const fails = checks.filter((c) => c.status === "fail").length;
  const warns = checks.filter((c) => c.status === "warn").length;
  return fails ? t("doctor.summaryFail", { fails, warns }) : warns ? t("doctor.summaryWarn", { warns }) : t("doctor.summaryOk");
}

/** 注册本机只读工具 ee_local_doctor。修复交给模型用 DSH 自带的文件 / shell 工具做（有 DSH 自己的审批），这里不改任何文件。 */
export function registerDoctor(ctx: DshContext, config: PluginConfig, tokens: TokenManager, http: HttpClient, bridge: McpBridge, bindings?: AppBindings): void {
  const dispose = ctx.tools.register({
    name: "ee_local_doctor",
    description: t("doctor.description"),
    parameters: { type: "object", properties: {} },
    output: { schema: {}, render: (_args: unknown, value: unknown) => [{ type: "text", text: JSON.stringify(value) }] },
    execute: async () => runLocalDoctor(ctx, config, tokens, http, bridge, bindings),
  });
  ctx.effect(() => dispose);
}
