// MCP（Model Context Protocol）客户端：POST /mcp 的 JSON-RPC 会话。
//
// dsh-ee 的实现要点（见 #128）：
//   - initialize 响应头带 Mcp-Session-Id，后续请求原样回带；
//   - 第三方客户端 tools/list 默认分页（默认 100/页，上限 500，nextCursor 不透明，
//     limit=0 表示不分页），SDK 的 listAllTools 自动翻页；
//   - 写操作需要人工确认时，tools/call 结果保持 content/isError，并额外在结果级
//     `_meta["dsh-ee/confirmation"] = {id, url, expires_at}`。
//   - 限流时 HTTP 429 + JSON-RPC error（code -32000），另带 Retry-After 与 X-RateLimit-*。

import { DshError, DshJsonRpcError } from './errors.js';
import { errorFromResponse, parseSseData, type HttpResponseMeta } from './http.js';
import type {
  CallToolOptions,
  DshConfirmation,
  ListAllToolsOptions,
  ListToolsOptions,
  McpClientInfo,
  McpInitializeResult,
  McpServerInfo,
  McpTool,
  McpToolCallResult,
  McpToolsPage,
} from './types.js';

/** 会话级请求参数（由 McpSession 维护，调用方一般不用直接传）。 */
export interface McpTransportOptions {
  method?: 'POST' | 'DELETE';
  sessionId?: string;
  protocolVersion?: string;
  signal?: AbortSignal;
  timeoutMs?: number;
}

/** 一次 MCP HTTP 响应；body 已读成文本，由 McpSession 按 content-type 解析。 */
export interface McpTransportResult {
  meta: HttpResponseMeta;
  headers: Headers;
  text: string;
}

/** McpSession 与 HTTP 层之间的最小接口；DshHarnessClient 负责认证与 URL。 */
export type McpTransport = (
  body: unknown | null,
  options: McpTransportOptions,
) => Promise<McpTransportResult>;

export interface McpSessionOptions {
  /** initialize 时上报的客户端名/版本。 */
  clientInfo?: McpClientInfo;
  /** 期望的 MCP 协议版本；服务端不支持时会在 initialize 结果里给出版本。 */
  protocolVersion?: string;
  /** initialize 时上报的客户端 capabilities，默认 {}（dsh-ee 用不到 roots/sampling）。 */
  capabilities?: Record<string, unknown>;
  /** 是否在创建会话时自动 initialize 并发送 notifications/initialized，默认 true。 */
  initialize?: boolean;
  signal?: AbortSignal;
}

/** SDK 默认请求的 MCP 协议版本；服务端不支持时会在 initialize 结果里协商。 */
export const MCP_DEFAULT_PROTOCOL_VERSION = '2025-06-18';
const DEFAULT_CLIENT_INFO: McpClientInfo = { name: 'dsh-ee-harness-sdk', version: '0.1.0' };
const MAX_TOOL_PAGES = 10_000;

interface RpcResponseMessage {
  jsonrpc?: unknown;
  id?: unknown;
  result?: unknown;
  error?: unknown;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

/**
 * MCP 会话。通常通过 `DshHarnessClient.connectMcp()` 拿到，已自动完成 initialize。
 *
 *   const session = await client.connectMcp();
 *   const tools = await session.listAllTools();
 *   const result = await session.callTool('crm__lookup', { q: '张三' });
 *   const confirm = extractConfirmation(result);
 *   await session.close();
 */
export class McpSession {
  readonly clientInfo: McpClientInfo;
  readonly requestedProtocolVersion: string;

  private readonly transport: McpTransport;
  private readonly capabilities: Record<string, unknown>;
  private nextId = 1;
  private sessionIdValue?: string;
  private negotiatedVersion: string;
  private closed = false;
  private initialized = false;
  private serverInfoValue?: McpServerInfo;
  private instructionsValue?: string;
  private serverCapabilitiesValue: Record<string, unknown> = {};

  constructor(transport: McpTransport, options: McpSessionOptions = {}) {
    this.transport = transport;
    this.clientInfo = options.clientInfo ?? DEFAULT_CLIENT_INFO;
    this.requestedProtocolVersion = options.protocolVersion ?? MCP_DEFAULT_PROTOCOL_VERSION;
    this.negotiatedVersion = this.requestedProtocolVersion;
    this.capabilities = options.capabilities ?? {};
  }

  /** 创建会话；默认自动 initialize（initialize=false 时由调用方自己调 start）。 */
  static async create(transport: McpTransport, options: McpSessionOptions = {}): Promise<McpSession> {
    const session = new McpSession(transport, options);
    if (options.initialize !== false) {
      await session.start({ signal: options.signal });
    }
    return session;
  }

  /** 当前会话 id（initialize 后才有）。 */
  get sessionId(): string | undefined {
    return this.sessionIdValue;
  }

  /** 协商后的 MCP 协议版本。 */
  get protocolVersion(): string {
    return this.negotiatedVersion;
  }

  /** 是否已完成 initialize 与 notifications/initialized。 */
  get isInitialized(): boolean {
    return this.initialized;
  }

  /** 会话是否已关闭。 */
  get isClosed(): boolean {
    return this.closed;
  }

  /** 服务端信息（initialize 结果）。 */
  get serverInfo(): McpServerInfo | undefined {
    return this.serverInfoValue;
  }

  /** 服务端返回的 instructions（dsh-ee 用它说明可用连接与用法）。 */
  get instructions(): string | undefined {
    return this.instructionsValue;
  }

  /** 服务端 capabilities（不透明）。 */
  get serverCapabilities(): Record<string, unknown> {
    return this.serverCapabilitiesValue;
  }

  /** initialize + notifications/initialized；重复调用是幂等的。 */
  async start(options: { signal?: AbortSignal } = {}): Promise<McpInitializeResult> {
    if (this.initialized) {
      return {
        protocolVersion: this.negotiatedVersion,
        capabilities: this.serverCapabilitiesValue,
        serverInfo: this.serverInfoValue ?? { name: 'dsh-ee', version: '' },
        instructions: this.instructionsValue,
      };
    }
    const { result, response } = await this.requestWithResponse<McpInitializeResult>(
      'initialize',
      {
        protocolVersion: this.requestedProtocolVersion,
        capabilities: this.capabilities,
        clientInfo: this.clientInfo,
      },
      { signal: options.signal },
    );
    const sessionId = response.headers.get('mcp-session-id');
    if (sessionId !== null && sessionId !== '') this.sessionIdValue = sessionId;
    if (typeof result?.protocolVersion === 'string' && result.protocolVersion !== '') {
      this.negotiatedVersion = result.protocolVersion;
    }
    this.serverInfoValue = result?.serverInfo;
    this.instructionsValue = typeof result?.instructions === 'string' ? result.instructions : undefined;
    this.serverCapabilitiesValue = isRecord(result?.capabilities) ? result.capabilities : {};
    await this.notify('notifications/initialized', undefined, { signal: options.signal });
    this.initialized = true;
    return result;
  }

  /** MCP ping。 */
  async ping(options: { signal?: AbortSignal } = {}): Promise<Record<string, unknown>> {
    return this.request<Record<string, unknown>>('ping', undefined, { signal: options.signal });
  }

  /**
   * 取一页工具。不传 limit 时按服务端默认页大小（100）返回，并可能带 nextCursor；
   * 想一次取全量可以传 `{limit: 0}`。
   */
  async listTools(options: ListToolsOptions = {}): Promise<McpToolsPage> {
    const params: Record<string, unknown> = {};
    if (options.limit !== undefined) params.limit = options.limit;
    if (options.cursor !== undefined) params.cursor = options.cursor;
    const result = await this.request<{ tools?: unknown; nextCursor?: unknown }>('tools/list', params, {
      signal: options.signal,
    });
    if (!isRecord(result) || !Array.isArray(result.tools)) {
      throw new DshError('MCP tools/list response is missing the tools array');
    }
    if (result.nextCursor !== undefined && typeof result.nextCursor !== 'string') {
      throw new DshError('MCP tools/list nextCursor is not a string');
    }
    return {
      tools: result.tools as McpTool[],
      nextCursor: result.nextCursor as string | undefined,
    };
  }

  /** 跟随 nextCursor 翻页，取回全部工具（保持服务端稳定排序）。 */
  async listAllTools(options: ListAllToolsOptions = {}): Promise<McpTool[]> {
    const tools: McpTool[] = [];
    const seenCursors = new Set<string>();
    let cursor: string | undefined;
    for (let page = 0; page < MAX_TOOL_PAGES; page += 1) {
      const result = await this.listTools({
        limit: options.pageSize,
        cursor,
        signal: options.signal,
      });
      tools.push(...result.tools);
      options.onPage?.(result);
      if (result.nextCursor === undefined) return tools;
      if (seenCursors.has(result.nextCursor)) {
        throw new DshError('MCP tools/list returned a duplicate nextCursor; stopped paginating');
      }
      seenCursors.add(result.nextCursor);
      cursor = result.nextCursor;
    }
    throw new DshError(`MCP tools/list exceeded ${MAX_TOOL_PAGES} pages; stopped paginating`);
  }

  /**
   * 调用一个工具。结果里的 isError 不会抛错（这是工具执行结果），
   * 需要确认时用 extractConfirmation(result) 拿确认链接。
   */
  async callTool(
    name: string,
    args: Record<string, unknown> = {},
    options: CallToolOptions = {},
  ): Promise<McpToolCallResult> {
    const result = await this.request<McpToolCallResult>(
      'tools/call',
      { name, arguments: args },
      { signal: options.signal },
    );
    if (!isRecord(result) || !Array.isArray(result.content)) {
      throw new DshError(`MCP tools/call (${name}) response is missing the content array`);
    }
    return result as unknown as McpToolCallResult;
  }

  /** 结束会话：有 session id 时发 DELETE /mcp；错误被忽略（服务端可能已清理）。 */
  async close(): Promise<void> {
    if (this.closed) return;
    this.closed = true;
    if (!this.initialized || this.sessionIdValue === undefined) return;
    try {
      await this.transport(null, {
        method: 'DELETE',
        sessionId: this.sessionIdValue,
        protocolVersion: this.negotiatedVersion,
      });
    } catch {
      // 关闭是尽力而为；会话最终会由服务端/网关清理。
    }
  }

  private async request<T>(
    method: string,
    params: unknown,
    options: { signal?: AbortSignal },
  ): Promise<T> {
    return (await this.requestWithResponse<T>(method, params, options)).result;
  }

  private async requestWithResponse<T>(
    method: string,
    params: unknown,
    options: { signal?: AbortSignal },
  ): Promise<{ result: T; response: McpTransportResult }> {
    if (this.closed) throw new DshError('MCP session is closed; call connectMcp() again');
    const id = this.nextId++;
    const response = await this.transport(
      { jsonrpc: '2.0', id, method, ...(params === undefined ? {} : { params }) },
      {
        sessionId: this.sessionIdValue,
        protocolVersion: this.negotiatedVersion,
        signal: options.signal,
      },
    );
    const message = this.parseMessages(response);
    if (message === undefined) {
      throw new DshError(`MCP ${method} did not return a JSON-RPC response matching id=${String(id)}`);
    }
    if (message.result === undefined) {
      throw new DshError(`MCP ${method} response has neither result nor error`);
    }
    return { result: message.result as T, response };
  }

  private async notify(
    method: string,
    params: unknown,
    options: { signal?: AbortSignal },
  ): Promise<void> {
    if (this.closed) throw new DshError('MCP session is closed; call connectMcp() again');
    const response = await this.transport(
      { jsonrpc: '2.0', method, ...(params === undefined ? {} : { params }) },
      {
        sessionId: this.sessionIdValue,
        protocolVersion: this.negotiatedVersion,
        signal: options.signal,
      },
    );
    // 通知成功时服务端返回 202/204（body 为空）；有 JSON-RPC error 或 HTTP 错误时这里抛出。
    this.parseMessages(response);
  }

  /**
   * 从响应里解析 JSON-RPC 消息。POST /mcp 正常返回 application/json（单个响应），
   * 经过代理或未来实现时也可能是 text/event-stream（可能先来通知，再来响应）。
   */
  private parseMessages(response: McpTransportResult): RpcResponseMessage | undefined {
    const messages: unknown[] = [];
    if (response.meta.contentType.includes('text/event-stream')) {
      for (const data of parseSseData(response.text)) {
        try {
          messages.push(JSON.parse(data) as unknown);
        } catch {
          // 忽略无法解析的 SSE 事件（注释/保活/非 JSON 数据）
        }
      }
    } else {
      try {
        messages.push(JSON.parse(response.text) as unknown);
      } catch {
        // 空 body（通知的 202）或非 JSON 错误体
      }
    }

    // 任何 JSON-RPC error 都表示这次调用失败；限流的 429 会返回 id=null 的 -32000。
    for (const message of messages) {
      if (!isRecord(message)) continue;
      const error = message.error;
      if (isRecord(error) && typeof error.code === 'number') {
        throw this.rpcError(error, response.meta);
      }
    }
    if (!response.meta.ok) {
      throw errorFromResponse(response.meta, {
        json: this.parseBodyJson(response),
        text: response.text,
      });
    }
    // 只取响应消息（有 id、没有 method）；SSE 里可能混有服务端通知。
    for (const message of messages) {
      if (!isRecord(message)) continue;
      if (message.id !== undefined && message.method === undefined) {
        return message as RpcResponseMessage;
      }
    }
    return undefined;
  }

  private parseBodyJson(response: McpTransportResult): unknown {
    try {
      return JSON.parse(response.text) as unknown;
    } catch {
      return undefined;
    }
  }

  private rpcError(error: Record<string, unknown>, meta: HttpResponseMeta): DshJsonRpcError {
    const message = typeof error.message === 'string' ? error.message : 'JSON-RPC error';
    return new DshJsonRpcError(message, error.code as number, {
      status: meta.status,
      requestId: meta.requestId,
      apiVersion: meta.apiVersion,
      rateLimit: meta.rateLimit,
      retryAfter: meta.retryAfter,
      rpcData: error.data,
      body: error,
    });
  }
}

/**
 * 从 tools/call 结果里取写确认信息。
 *
 * dsh-ee 需要网页确认时返回 `_meta["dsh-ee/confirmation"] = {id, url, expires_at}`；
 * 不需要确认或旧版服务端没有 `_meta` 时返回 undefined。
 */
export function extractConfirmation(result: McpToolCallResult): DshConfirmation | undefined {
  if (!isRecord(result._meta)) return undefined;
  const raw = result._meta['dsh-ee/confirmation'];
  if (!isRecord(raw)) return undefined;
  const id = raw.id;
  const url = raw.url;
  const expiresAtRaw = raw.expires_at;
  if ((typeof id !== 'number' && typeof id !== 'string') || typeof url !== 'string') return undefined;
  if (typeof expiresAtRaw !== 'string' || expiresAtRaw === '') return undefined;
  const expiresAt = new Date(expiresAtRaw);
  return {
    id,
    url,
    expires_at: expiresAtRaw,
    expiresAt: Number.isNaN(expiresAt.getTime()) ? undefined : expiresAt,
  };
}

/** 判断一个 tools/call 结果是否是「等待网页确认」。 */
export function isPendingConfirmation(result: McpToolCallResult): boolean {
  return extractConfirmation(result) !== undefined;
}

