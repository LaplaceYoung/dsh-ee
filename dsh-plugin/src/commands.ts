import type { DshContext, PluginConfig } from "./dsh.js";
import { DeviceFlow, openBrowser } from "./device.js";
import { t } from "./i18n/index.js";
import type { TokenManager } from "./token.js";

/** /ee-login 后面跟的接入码（网页「开始使用」页给的），形如 ABCD-EFGH；没有就是空串。 */
export function parseSetupCode(rawInput: string | undefined): string {
  const code = String(rawInput ?? "").trim().toUpperCase();
  return /^[A-Z0-9]{4}-?[A-Z0-9]{4}$/.test(code) ? code : "";
}

export interface CommandResult {
  kind: "success" | "error";
  text: string;
}

export interface CommandsHandle {
  login: (rawInput?: string) => Promise<CommandResult>;
}

export function registerCommands(ctx: DshContext, config: PluginConfig, tokens: TokenManager, onLogin: () => Promise<void> = async () => {}): CommandsHandle {
  const flow = new DeviceFlow(config);
  let pending: { code: string; url: string; task: Promise<void> } | null = null;
  // flow.start 是网络请求；pending 要等它回来才赋值，中间这段用 starting 防并发。
  let starting = false;
  const login = async (rawInput?: string): Promise<CommandResult> => {
    if (pending) return { kind: "success", text: t("commands.pending", { code: pending.code, url: pending.url }) };
    if (starting) return { kind: "success", text: t("commands.starting") };
    const setupCode = parseSetupCode(rawInput);
    if (String(rawInput ?? "").trim() && !setupCode) return { kind: "error", text: t("commands.badSetupCode") };
    starting = true;
    let started;
    try {
      started = await flow.start(setupCode);
    } finally {
      starting = false;
    }
    const url = started.verification_uri_complete;
    // 带接入码时网页「开始使用」页正开着，会自己显示确认按钮，不再另开浏览器
    if (!setupCode) openBrowser(url);
    const task = (async () => {
      try {
        const result = await flow.wait(started.device_code, started.interval);
        await tokens.set(result.access_token);
        await tokens.syncNpmrc(result.access_token);
        // 立刻拉工具与技能，不等长连接退避重连和目录轮询
        await onLogin().catch(() => undefined);
      } finally {
        pending = null;
      }
    })();
    pending = { code: started.user_code, url, task };
    void task.catch(() => undefined);
    if (setupCode) return { kind: "success", text: t("commands.setupConfirm", { code: started.user_code, url }) };
    return { kind: "success", text: t("commands.pending", { code: started.user_code, url }) };
  };

  const commands = ctx.get("commands");
  if (commands) {
    const register = (definition: any) => {
      const dispose = commands.register(definition);
      ctx.effect(() => dispose);
    };
    register({
      name: "ee-login",
      description: t("commands.loginDescription"),
      input: { hint: t("commands.setupCodeHint") },
      handler: (invocation: any) => login(invocation?.rawInput),
    });
    register({
      name: "ee-status",
      description: t("commands.statusDescription"),
      handler: async () => {
        const current = await tokens.resolve();
        if (!current) return { kind: "success", text: t("commands.notLoggedIn") };
        let response: Response;
        try {
          response = await fetch(`${config.base}/api/dsh/whoami`, { headers: { Authorization: `Bearer ${current.value}` }, signal: AbortSignal.timeout(30_000) });
        } catch {
          return { kind: "error", text: t("commands.unreachable") };
        }
        if (!response.ok) {
          if (response.status === 401) return { kind: "success", text: t("commands.sessionExpired") };
          return { kind: "error", text: t("commands.httpError", { status: response.status }) };
        }
        const me = await response.json() as { username: string; department: string; token_prefix: string; expires_at?: string; client_name?: string };
        const matches = await tokens.npmrcMatches(current.value);
        const expiry = me.expires_at ? t("commands.expiresOn", { date: new Date(me.expires_at).toLocaleDateString() }) : t("commands.neverExpires");
        return { kind: "success", text: t("commands.statusLine", { username: me.username, department: me.department, token: me.token_prefix, expiry, version: config.version, npmrc: matches ? t("commands.npmrcMatch") : t("commands.npmrcMismatch") }) };
      },
    });
    register({
      name: "ee-logout",
      description: t("commands.logoutDescription"),
      handler: async () => {
        const current = await tokens.resolve();
        if (!current) return { kind: "success", text: t("commands.noAccount") };
        const revoked = await fetch(`${config.base}/api/dsh/logout`, { method: "POST", headers: { Authorization: `Bearer ${current.value}` }, signal: AbortSignal.timeout(30_000) })
          .then((response) => response.ok, () => false);
        await tokens.unset(current.value);
        // unset 只能清掉 DSH 凭据库里的值；令牌若来自环境变量或 $DSH_HOME/.env，DSH 仍会读到它
        const left = await tokens.resolve();
        if (left?.value === current.value) {
          return {
            kind: "error",
            text: `${revoked ? t("commands.revokedPrefix") : ""}${t("commands.envToken", { ref: config.tokenRef })}`,
          };
        }
        return { kind: "success", text: revoked ? t("commands.loggedOut") : t("commands.loggedOutOffline") };
      },
    });
  }
  return { login };
}
