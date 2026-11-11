import type { DshContext, PluginConfig } from "./dsh.js";
import type { HttpClient } from "./http.js";

/** 服务端在 tools/list 的 _meta.dshee.actions 里标注的写 / 危险操作；没标注的 action 都是只读。 */
export interface ActionMeta {
  e: "write" | "danger";
  zh: string;
  en: string;
}

export type LocalAction = (args: any) => Promise<unknown>;

/** MCP 工具结果里的文字；服务端返回的 isError 原样带上「错误：」前缀。 */
export function renderResult(value: any): string {
  if (value && Array.isArray(value.content)) {
    const text = value.content.filter((part: any) => part?.type === "text").map((part: any) => String(part.text)).join("\n");
    return value.isError ? `错误：${text}` : text;
  }
  return JSON.stringify(value);
}

/** 参数规范化：键排序、去掉 confirm，用来把两步确认里的两次调用对上。 */
export function canonArgs(args: any): string {
  const sort = (value: any): any => {
    if (Array.isArray(value)) return value.map(sort);
    if (value && typeof value === "object") {
      return Object.fromEntries(Object.keys(value).filter((key) => key !== "confirm").sort().map((key) => [key, sort(value[key])]));
    }
    return value;
  };
  return JSON.stringify(sort(args ?? {}));
}

/**
 * 增量解析 SSE，数出每块数据里完整收到的 tools/list_changed 通知。
 * 注释行（`: ping` 保活）和其他通知不算；一条事件被拆在两块里也能认出来。
 */
export function listChangedCounter(): { push(chunk?: Uint8Array): number } {
  const decoder = new TextDecoder();
  let buffer = "";
  return {
    push(chunk) {
      if (!chunk) return 0;
      buffer += decoder.decode(chunk, { stream: true });
      let count = 0;
      for (;;) {
        const end = buffer.search(/\r?\n\r?\n/);
        if (end < 0) break;
        const event = buffer.slice(0, end);
        buffer = buffer.slice(end).replace(/^\r?\n\r?\n/, "");
        const data = event.split(/\r?\n/).filter((line) => line.startsWith("data:")).map((line) => line.slice(5).trim()).join("\n");
        if (!data) continue;
        try {
          if (JSON.parse(data)?.method === "notifications/tools/list_changed") count++;
        } catch {
          // 不是 JSON 的事件不关我们的事
        }
      }
      return count;
    },
  };
}

export class McpBridge {
  private disposers = new Map<string, () => void>();
  /** 已注册工具的定义指纹（DSH 里的工具全名 -> 定义的 JSON），用来发现「名字没变、定义变了」 */
  private signatures = new Map<string, string>();
  private session = "";
  private controller?: AbortController;
  /** 结束长连接的退避等待，马上重连（刚登录时用）。 */
  private wakeUp?: () => void;
  /** DSH 里的工具全名 -> 服务端工具名 */
  private serverNames = new Map<string, string>();
  private metas = new Map<string, Record<string, ActionMeta>>();
  private previews = new Map<string, string>();
  private local = new Map<string, LocalAction>();
  /** 服务端在 tools/list 里标了 readOnlyHint 的工具（服务端工具名）；工作台取数只能用它们 */
  private readOnly = new Set<string>();
  readonly status = { lastSyncAt: 0, tools: 0, error: "" };

  constructor(private ctx: DshContext, private config: PluginConfig, private http: HttpClient) {}

  private async rpc(method: string, params?: unknown, timeoutMs = 30_000, retried = false): Promise<any> {
    const headers: Record<string, string> = { "content-type": "application/json", accept: "application/json, text/event-stream" };
    if (this.session) headers["Mcp-Session-Id"] = this.session;
    const response = await this.http.fetch("/mcp", { method: "POST", headers, body: JSON.stringify({ jsonrpc: "2.0", id: 1, method, params }) }, timeoutMs);
    if (response.status === 404) {
      // 会话过期时 dsh-ee 返回 404；清掉会话重试一次即可。一直 404 说明地址/代理不对，
      // 再递归下去会以网络速度无限重发，必须停。
      this.session = "";
      if (retried) throw new Error(`MCP ${method} HTTP 404`);
      return this.rpc(method, params, timeoutMs, true);
    }
    if (!response.ok) throw new Error(`MCP ${method} HTTP ${response.status}`);
    this.session = response.headers.get("mcp-session-id") || this.session;
    const body = await response.json() as { result?: any; error?: { message: string } };
    if (body.error) throw new Error(body.error.message);
    return body.result;
  }

  /** 注册由本机完成的 action（例如要读本机文件的上传）。键是 `<工具>.<action>`。 */
  registerLocal(key: string, fn: LocalAction): void {
    this.local.set(key, fn);
  }

  /** 直接调服务端工具（本机诊断用）。 */
  async callTool(name: string, args: unknown): Promise<any> {
    return this.rpc("tools/call", { name, arguments: args });
  }

  /** 当前员工能用、并且服务端标为只读的工具；没权限的工具不在 tools/list 里，也就不在这里。 */
  isReadOnlyTool(serverName: string): boolean {
    return this.readOnly.has(serverName);
  }

  /** 当前员工能不能用这个工具（服务端按权限列出的工具）。 */
  hasTool(serverName: string): boolean {
    return [...this.serverNames.values()].includes(serverName);
  }

  /** 这次调用该不该先让用户批准；只读的返回 undefined。 */
  approval(fullName: string, args: any): { effect: "write" | "danger"; zh: string; en: string; preview?: string } | undefined {
    const metas = this.metas.get(fullName);
    const meta = metas?.[String(args?.action)];
    if (!meta) return undefined;
    if (meta.e === "danger") {
      // 危险操作的第一步只是拿预览，没有副作用，不必打扰用户；带 confirm 的第二步才真正执行
      if (!args?.confirm) return undefined;
      return { effect: "danger", zh: meta.zh, en: meta.en, preview: this.previews.get(`${fullName}\n${canonArgs(args)}`) };
    }
    return { effect: "write", zh: meta.zh, en: meta.en };
  }

  private remember(fullName: string, args: any, result: any): void {
    try {
      const text = result?.content?.[0]?.text;
      if (typeof text !== "string" || !text.includes("confirm_required")) return;
      const body = JSON.parse(text);
      if (body?.confirm_required && typeof body.preview === "string") {
        if (this.previews.size > 50) this.previews.clear();
        this.previews.set(`${fullName}\n${canonArgs(args)}`, body.preview);
      }
    } catch {
      // 不是预览就不用记
    }
  }

  private register(tool: any): void {
    const name = `mcp__${this.config.serverName}__${tool.name}`;
    const actions = tool._meta?.dshee?.actions;
    if (actions && typeof actions === "object") this.metas.set(name, actions);
    this.serverNames.set(name, tool.name);
    if (tool.annotations?.readOnlyHint === true) this.readOnly.add(tool.name);
    const dispose = this.ctx.tools.register({
      name,
      description: tool.description || tool.name,
      parameters: tool.inputSchema || { type: "object", properties: {} },
      output: { schema: {}, render: (_args: unknown, value: unknown) => [{ type: "text", text: renderResult(value) }] },
      execute: async (args: any) => {
        const local = this.local.get(`${tool.name}.${args?.action}`);
        if (local) return local(args);
        const result = await this.rpc("tools/call", { name: tool.name, arguments: args }, 0);
        this.remember(name, args, result);
        return result;
      },
    });
    this.signatures.set(name, JSON.stringify(tool));
    this.disposers.set(name, () => {
      dispose();
      this.signatures.delete(name);
      this.metas.delete(name);
      this.serverNames.delete(name);
      this.readOnly.delete(tool.name);
    });
  }

  async sync(): Promise<void> {
    let result: any;
    try {
      if (!this.session) await this.rpc("initialize", { protocolVersion: "2025-06-18", capabilities: {}, clientInfo: { name: "dsh-ee-connect", version: this.config.version } });
      result = await this.rpc("tools/list");
    } catch (error) {
      this.status.error = error instanceof Error ? error.message : String(error);
      throw error;
    }
    this.status.error = "";
    this.status.lastSyncAt = Date.now();
    this.status.tools = (result.tools ?? []).length;
    // 定义没变的工具原样留着：重新注册会让模型看到的工具清单变一次，白白打掉提示词缓存。
    // 定义变了的（说明里列的数据集 / 应用表多了一项、参数或只读标记改了）要换成新的，否则在线的 DSH 一直用旧定义。
    const next = new Map<string, any>((result.tools ?? []).map((tool: any) => [`mcp__${this.config.serverName}__${tool.name}`, tool]));
    for (const [name, dispose] of this.disposers) {
      const tool = next.get(name);
      if (!tool || JSON.stringify(tool) !== this.signatures.get(name)) {
        dispose();
        this.disposers.delete(name);
      }
    }
    for (const [name, tool] of next) {
      if (!this.disposers.has(name)) this.register(tool);
    }
  }

  async start(): Promise<void> {
    this.controller = new AbortController();
    const signal = this.controller.signal;
    await this.sync().catch(() => undefined);
    // 长连接收 tools/list_changed；断开（dsh-ee 重启、网络抖动）后退避重连，
    // 重连成功先补一次同步，避免错过断开期间的变化。插件停用时 abort 退出。
    // 只有真的收到 list_changed 才同步：流上还有开场注释和每 25 秒一行的保活，
    // 见到数据就同步的话，每个在线的 DSH 都会一直每 25 秒全量拉一次工具清单。
    void (async () => {
      let delay = 1000;
      while (!signal.aborted) {
        try {
          const response = await this.http.fetch("/mcp", { headers: { accept: "text/event-stream" }, signal }, 0);
          const reader = response.ok ? response.body?.getReader() : undefined;
          if (reader) {
            delay = 1000;
            await this.sync().catch(() => undefined);
            const events = listChangedCounter();
            while (true) {
              const { done, value } = await reader.read();
              if (done) break;
              if (events.push(value) > 0) await this.sync().catch(() => undefined);
            }
          }
        } catch {
          // 下面统一退避
        }
        if (signal.aborted) break;
        await new Promise((resolve) => {
          const timer = setTimeout(resolve, delay);
          this.wakeUp = () => { clearTimeout(timer); delay = 1000; resolve(undefined); };
          signal.addEventListener("abort", () => { clearTimeout(timer); resolve(undefined); }, { once: true });
        });
        this.wakeUp = undefined;
        delay = Math.min(delay * 2, 60_000);
      }
    })();
  }

  wake(): void {
    this.wakeUp?.();
  }

  /** 换了 dsh-ee 地址：断开旧的长连接和会话，连到新地址重新同步；已注册的工具留着，由同步增删。 */
  async restart(): Promise<void> {
    this.controller?.abort();
    this.session = "";
    await this.start();
  }

  stop(): void {
    this.controller?.abort();
    for (const dispose of this.disposers.values()) dispose();
    this.disposers.clear();
  }
}
