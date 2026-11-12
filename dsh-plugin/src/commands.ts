import type { DshContext, PluginConfig } from "./dsh.js";
import { DeviceFlow, openBrowser } from "./device.js";
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
    if (pending) return { kind: "success", text: `请在浏览器确认，确认码 ${pending.code}，链接 ${pending.url}` };
    if (starting) return { kind: "success", text: "正在发起企业登录，请稍候…" };
    const setupCode = parseSetupCode(rawInput);
    if (String(rawInput ?? "").trim() && !setupCode) return { kind: "error", text: "接入码格式不对，应形如 ABCD-EFGH；不带接入码直接输入 /ee-login 也可以" };
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
    if (setupCode) return { kind: "success", text: `回到 dsh-ee 网页的「开始使用」页，核对确认码 ${started.user_code} 后点「确认授权」（网页关了的话打开 ${url}）。确认后回到这里继续对话。` };
    return { kind: "success", text: `请在浏览器确认，确认码 ${started.user_code}，链接 ${url}` };
  };

  const commands = ctx.get("commands");
  if (commands) {
    const register = (definition: any) => {
      const dispose = commands.register(definition);
      ctx.effect(() => dispose);
    };
    register({
      name: "ee-login",
      description: "登录企业账号（可以带上网页「开始使用」页给的接入码）",
      input: { hint: "[接入码]" },
      handler: (invocation: any) => login(invocation?.rawInput),
    });
    register({
      name: "ee-status",
      description: "查看企业登录状态",
      handler: async () => {
        const current = await tokens.resolve();
        if (!current) return { kind: "success", text: "未登录，在对话框输入 /ee-login" };
        const response = await fetch(`${config.base}/api/dsh/whoami`, { headers: { Authorization: `Bearer ${current.value}` }, signal: AbortSignal.timeout(30_000) });
        if (!response.ok) return { kind: "success", text: "登录已失效，在对话框输入 /ee-login" };
        const me = await response.json() as { username: string; department: string; token_prefix: string; expires_at?: string; client_name?: string };
        const matches = await tokens.npmrcMatches(current.value);
        const expiry = me.expires_at ? ` · ${new Date(me.expires_at).toLocaleDateString()} 到期` : " · 长期有效";
        return { kind: "success", text: `已登录 ${me.username}（${me.department}）· 令牌 ${me.token_prefix}${expiry} · 插件 ${config.version} · npmrc ${matches ? "一致" : "不一致"}` };
      },
    });
    register({
      name: "ee-logout",
      description: "退出企业账号",
      handler: async () => {
        const current = await tokens.resolve();
        if (!current) return { kind: "success", text: "当前没有登录企业账号" };
        const revoked = await fetch(`${config.base}/api/dsh/logout`, { method: "POST", headers: { Authorization: `Bearer ${current.value}` }, signal: AbortSignal.timeout(30_000) })
          .then((response) => response.ok, () => false);
        await tokens.unset(current.value);
        // unset 只能清掉 DSH 凭据库里的值；令牌若来自环境变量或 $DSH_HOME/.env，DSH 仍会读到它
        const left = await tokens.resolve();
        if (left?.value === current.value) {
          return {
            kind: "error",
            text: `${revoked ? "已在服务端吊销该令牌，" : ""}但它来自环境变量或 $DSH_HOME/.env（${config.tokenRef}），请手动删除后重启 DSH`,
          };
        }
        return { kind: "success", text: revoked ? "已退出企业账号" : "本机已退出；连不上 dsh-ee，令牌未能在服务端吊销，可在网页「我的令牌」里吊销" };
      },
    });
  }
  return { login };
}
