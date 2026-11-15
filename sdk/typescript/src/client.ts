// dsh-ee Harness 客户端：发现、设备码登录、模型网关与 MCP 的统一入口。
//
// 依赖的 dsh-ee 契约（均已在服务端 PR 中实现）：
//   - #123：GET /.well-known/dsh-ee、GET /v1/capabilities、GET /v1/apps、Dsh-EE-API-Version
//   - #128：MCP tools/list cursor 分页与写确认结果级 _meta["dsh-ee/confirmation"]
//   - #129：/v1/* 与 /mcp 的每令牌限流（X-RateLimit-*、Retry-After）与 X-Request-Id
//   - #132：令牌 client_kind；第三方 Harness 用 third_party 令牌，写操作始终人工确认
//
// 这是一个通用 HTTP/MCP 客户端，不是 Anthropic Messages / OpenAI Responses 协议实现：
// 模型面只覆盖 dsh-ee 的 OpenAI chat completions 子集。

import { DshError, DeviceFlowError } from './errors.js';
import {
  errorFromResponse,
  fetchPending,
  iterateSseData,
  readBody,
  sleep as defaultSleep,
  type HttpResponseMeta,
  type PendingResponse,
  type ReadBodyResult,
} from './http.js';
import {
  McpSession,
  type McpSessionOptions,
  type McpTransport,
  type McpTransportOptions,
  type McpTransportResult,
} from './mcp.js';
import type {
  CallToolOptions,
  ChatCompletion,
  ChatCompletionChunk,
  ChatCompletionRequest,
  DeviceCode,
  DeviceCodeRequest,
  DeviceFlowErrorCode,
  DeviceToken,
  DeviceTokenWaitOptions,
  FetchLike,
  HarnessApp,
  HarnessAppDetail,
  HarnessAppsPage,
  HarnessCapabilities,
  HarnessDiscovery,
  ListAllToolsOptions,
  ListToolsOptions,
  McpClientInfo,
  McpTool,
  McpToolCallResult,
  McpToolsPage,
  ModelInfo,
  RateLimitInfo,
} from './types.js';

/** 服务端在一次响应上的观测信息（请求号 / 限流 / API 版本）。 */
export interface DshResponseInfo extends HttpResponseMeta {
  method: string;
  path: string;
}

export interface DshClientOptions {
  /** dsh-ee 部署地址，例如 https://dsh-ee.example.com（不要带结尾斜杠也行）。 */
  baseUrl: string;
  /** 统一令牌（ee_at_…）；也可以设备码登录后由 SDK 自动设置。 */
  token?: string;
  /** 可注入的 fetch（默认用全局 fetch；Node 18+ / 浏览器都有）。 */
  fetch?: FetchLike;
  /** 附加到每个请求的默认头（例如企业代理需要的头）。 */
  headers?: Record<string, string>;
  /** 单次请求超时（毫秒），默认 60000；0 表示不设超时。 */
  timeoutMs?: number;
  /** 请求头 Dsh-EE-API-Version 的值，默认 "1"；传 null 不发送。 */
  apiVersion?: string | null;
  /** MCP initialize 期望的协议版本，默认 2025-06-18。 */
  mcpProtocolVersion?: string;
  /** MCP initialize 上报的客户端信息。 */
  mcpClientInfo?: McpClientInfo;
  /** 观测钩子：每个响应（成功/失败）都会回调，便于记录 request id 与限流余量。 */
  onResponse?: (info: DshResponseInfo) => void;
  /** 覆盖设备码轮询的 sleep（主要给测试用；默认真实等待）。 */
  sleep?: (ms: number, signal?: AbortSignal) => Promise<void>;
}

interface RequestOptions {
  method?: 'GET' | 'POST' | 'DELETE';
  body?: unknown;
  signal?: AbortSignal;
  headers?: Record<string, string>;
  accept?: string;
  /** 是否带 Authorization；默认带（如果已配置 token）。 */
  auth?: boolean;
  /** 覆盖客户端的 API 版本头；null 表示这个请求不发。 */
  apiVersion?: string | null;
  /** 覆盖超时。 */
  timeoutMs?: number;
}

const DEFAULT_TIMEOUT_MS = 60_000;
const DEFAULT_API_VERSION = '1';
const DEFAULT_DEVICE_INTERVAL_MS = 5_000;
const DEFAULT_DEVICE_EXPIRES_MS = 600_000;
const DEVICE_ERROR_CODES = new Set<DeviceFlowErrorCode>([
  'authorization_pending',
  'slow_down',
  'access_denied',
  'expired_token',
  'rate_limited',
]);

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function hasHeader(headers: Record<string, string>, name: string): boolean {
  const lower = name.toLowerCase();
  return Object.keys(headers).some((key) => key.toLowerCase() === lower);
}

function isVersionedPath(path: string): boolean {
  return path === '/v1' || path.startsWith('/v1/') || path === '/.well-known/dsh-ee';
}

function truncate(text: string, max = 300): string {
  return text.length <= max ? text : `${text.slice(0, max)}…`;
}

/**
 * dsh-ee Harness 客户端。
 *
 * ```ts
 * const client = new DshHarnessClient({ baseUrl: 'https://dsh-ee.example.com' });
 * const [discovery, capabilities] = await Promise.all([client.discovery(), client.capabilities()]);
 * ```
 */
export class DshHarnessClient {
  /** 规范化后的部署地址（无结尾斜杠）。 */
  readonly baseUrl: string;

  private readonly fetchImpl: FetchLike;
  private readonly defaultHeaders: Record<string, string>;
  private readonly timeoutMs: number;
  private readonly apiVersion: string | null;
  private readonly mcpProtocolVersion: string;
  private readonly mcpClientInfo?: McpClientInfo;
  private readonly onResponse?: (info: DshResponseInfo) => void;
  private readonly sleepFn: (ms: number, signal?: AbortSignal) => Promise<void>;
  private readonly mcpTransport: McpTransport;

  private tokenValue?: string;
  private discoveryCache?: { doc: HarnessDiscovery; etag?: string };
  private mcpSession?: McpSession;

  /** 最近一次响应的请求号（便于和服务端日志对账）。 */
  lastRequestId?: string;
  /** 最近一次响应的 API 版本头。 */
  lastApiVersion?: string;
  /** 最近一次响应的限流状态（端点未限流时为 undefined）。 */
  lastRateLimit?: RateLimitInfo;

  constructor(options: DshClientOptions) {
    if (!isRecord(options) || typeof options.baseUrl !== 'string' || options.baseUrl.trim() === '') {
      throw new TypeError('DshHarnessClient requires baseUrl');
    }
    this.baseUrl = options.baseUrl.trim().replace(/\/+$/, '');
    if (this.baseUrl === '') throw new TypeError('DshHarnessClient baseUrl cannot consist only of slashes');

    const globalFetch = (globalThis as { fetch?: FetchLike }).fetch;
    if (options.fetch !== undefined) {
      this.fetchImpl = options.fetch;
    } else if (typeof globalFetch === 'function') {
      this.fetchImpl = (url, init) => globalFetch(url, init);
    } else {
      throw new DshError('No fetch in the current runtime; Node 18+ provides fetch, or inject one via options.fetch');
    }

    this.tokenValue = options.token;
    this.defaultHeaders = { ...(options.headers ?? {}) };
    this.timeoutMs = options.timeoutMs ?? DEFAULT_TIMEOUT_MS;
    this.apiVersion = options.apiVersion === undefined ? DEFAULT_API_VERSION : options.apiVersion;
    this.mcpProtocolVersion = options.mcpProtocolVersion ?? '2025-06-18';
    this.mcpClientInfo = options.mcpClientInfo;
    this.onResponse = options.onResponse;
    this.sleepFn = options.sleep ?? defaultSleep;
    this.mcpTransport = async (body, transportOptions) => this.sendMcp(body, transportOptions);
  }

  // ---- 令牌 ----

  /** 设置/清除令牌（设备码登录成功后会自动设置）。 */
  setToken(token: string | undefined): void {
    this.tokenValue = token;
  }

  /** 是否已配置令牌。 */
  get hasToken(): boolean {
    return this.tokenValue !== undefined && this.tokenValue !== '';
  }

  // ---- 发现与能力 ----

  /**
   * GET /.well-known/dsh-ee。默认使用 ETag 缓存：第二次调用带 If-None-Match，
   * 服务端 304 时返回缓存；要强制刷新传 `{refresh: true}`。
   */
  async discovery(options: { refresh?: boolean; signal?: AbortSignal } = {}): Promise<HarnessDiscovery> {
    const headers: Record<string, string> = {};
    const cached = this.discoveryCache;
    if (cached?.etag !== undefined && options.refresh !== true) {
      headers['If-None-Match'] = cached.etag;
    }
    const pending = await this.send('/.well-known/dsh-ee', {
      headers,
      auth: false,
      signal: options.signal,
    });
    try {
      if (pending.meta.status === 304) {
        if (cached !== undefined) return cached.doc;
        throw new DshError('Discovery document returned 304 but there is no local cache');
      }
      const body = await readBody(pending.response);
      this.throwIfError(pending.meta, body);
      if (!isRecord(body.json)) {
        throw new DshError('Discovery document is not a JSON object');
      }
      const doc = body.json as unknown as HarnessDiscovery;
      this.discoveryCache = { doc, etag: pending.meta.etag };
      return doc;
    } finally {
      pending.cleanup();
    }
  }

  /** GET /v1/capabilities（不带令牌时返回公开子集）。 */
  async capabilities(options: { signal?: AbortSignal } = {}): Promise<HarnessCapabilities> {
    return this.requestJson<HarnessCapabilities>('/v1/capabilities', {
      signal: options.signal,
    });
  }

  /** GET /v1/apps：当前令牌可见的中立微应用描述。 */
  async listApps(options: { signal?: AbortSignal } = {}): Promise<HarnessAppsPage> {
    return this.requestJson<HarnessAppsPage>('/v1/apps', { signal: options.signal });
  }

  /** GET /v1/apps/{name}。 */
  async getApp(name: string, options: { signal?: AbortSignal } = {}): Promise<HarnessApp> {
    if (typeof name !== 'string' || name === '') throw new TypeError('getApp requires name');
    const detail = await this.requestJson<HarnessAppDetail>(
      `/v1/apps/${encodeURIComponent(name)}`,
      { signal: options.signal },
    );
    return detail.app;
  }

  // ---- 模型网关 ----

  /** GET /v1/models：返回当前令牌可用的对外模型名。 */
  async listModels(options: { signal?: AbortSignal } = {}): Promise<ModelInfo[]> {
    const payload = await this.requestJson<{ data?: unknown }>('/v1/models', {
      signal: options.signal,
    });
    if (!isRecord(payload) || !Array.isArray(payload.data)) {
      throw new DshError('GET /v1/models response is missing the data array');
    }
    return payload.data as ModelInfo[];
  }

  /**
   * POST /v1/chat/completions（非流式）。请求体会原样转发给 dsh-ee 的模型网关。
   * 流式请用 streamChatCompletion()。
   */
  async createChatCompletion(
    request: ChatCompletionRequest,
    options: { signal?: AbortSignal } = {},
  ): Promise<ChatCompletion> {
    if (request.stream === true) {
      throw new DshError('createChatCompletion is for non-streaming requests only; use streamChatCompletion() for streaming');
    }
    const payload = await this.requestJson<ChatCompletion>('/v1/chat/completions', {
      method: 'POST',
      body: { ...request, stream: false },
      signal: options.signal,
    });
    if (!isRecord(payload) || !Array.isArray(payload.choices)) {
      throw new DshError('POST /v1/chat/completions response is missing the choices array');
    }
    return payload;
  }

  /**
   * POST /v1/chat/completions（流式）：逐个产出 SSE data 分块，`[DONE]` 后结束。
   * 服务端会强制 `stream_options.include_usage=true`，最后一块通常带 usage。
   *
   * 注意：默认不设整条流的总超时（长回答可能很久），用 signal 取消；
   * 需要总超时时传 `options.timeoutMs`。
   */
  async *streamChatCompletion(
    request: ChatCompletionRequest,
    options: { signal?: AbortSignal; timeoutMs?: number } = {},
  ): AsyncGenerator<ChatCompletionChunk, void, undefined> {
    const pending = await this.send('/v1/chat/completions', {
      method: 'POST',
      body: { ...request, stream: true },
      accept: 'text/event-stream',
      signal: options.signal,
      timeoutMs: options.timeoutMs ?? 0,
    });
    try {
      if (!pending.meta.ok) {
        const body = await readBody(pending.response);
        this.throwIfError(pending.meta, body);
        throw new DshError(`HTTP ${pending.meta.status}`);
      }
      const stream = pending.response.body;
      if (stream === null) {
        const body = await readBody(pending.response);
        throw new DshError(
          `Expected an SSE stream, but the response has no body (content-type: ${pending.meta.contentType || 'unknown'})${body.text ? `: ${truncate(body.text)}` : ''}`,
        );
      }
      if (!pending.meta.contentType.includes('text/event-stream')) {
        const body = await readBody(pending.response);
        throw new DshError(
          `Expected text/event-stream, got content-type: ${pending.meta.contentType || 'unknown'}${body.text ? `: ${truncate(body.text)}` : ''}`,
        );
      }
      for await (const data of iterateSseData(stream)) {
        if (data === '[DONE]') return;
        let chunk: unknown;
        try {
          chunk = JSON.parse(data) as unknown;
        } catch {
          throw new DshError(`Unparseable SSE data: ${truncate(data)}`);
        }
        if (!isRecord(chunk)) throw new DshError('SSE data is not a JSON object');
        yield chunk as ChatCompletionChunk;
      }
    } finally {
      pending.cleanup();
      const stream = pending.response.body;
      if (stream !== null && !stream.locked) {
        stream.cancel().catch(() => {
          // 已结束或已断开，无需处理
        });
      }
    }
  }

  // ---- 设备码 ----

  /** POST /api/device/code：申请设备码（不需要令牌）。 */
  async requestDeviceCode(
    request: DeviceCodeRequest = {},
    options: { signal?: AbortSignal } = {},
  ): Promise<DeviceCode> {
    const body: Record<string, unknown> = {};
    if (request.clientName !== undefined) body.client_name = request.clientName;
    if (request.clientOs !== undefined) body.client_os = request.clientOs;
    if (request.clientVersion !== undefined) body.client_version = request.clientVersion;
    if (request.scopes !== undefined) body.scopes = request.scopes;
    if (request.setupCode !== undefined) body.setup_code = request.setupCode;
    return this.requestJson<DeviceCode>('/api/device/code', {
      method: 'POST',
      body,
      auth: false,
      signal: options.signal,
    });
  }

  /**
   * POST /api/device/token：轮询一次。
   * 等待用户确认（authorization_pending / slow_down）会抛 retryable 的 DeviceFlowError。
   */
  async pollDeviceToken(
    deviceCode: string,
    options: { signal?: AbortSignal } = {},
  ): Promise<DeviceToken> {
    const pending = await this.send('/api/device/token', {
      method: 'POST',
      body: { device_code: deviceCode },
      auth: false,
      signal: options.signal,
    });
    try {
      const body = await readBody(pending.response);
      if (pending.meta.ok) {
        if (!isRecord(body.json) || typeof body.json.access_token !== 'string') {
          throw new DshError('Device token response is missing access_token');
        }
        return body.json as unknown as DeviceToken;
      }
      const flowCode = deviceErrorCode(body.json);
      if (flowCode !== undefined) {
        throw new DeviceFlowError(flowCode, {
          status: pending.meta.status,
          requestId: pending.meta.requestId,
          rateLimit: pending.meta.rateLimit,
          retryAfter: pending.meta.retryAfter,
          body: body.json,
        });
      }
      throw errorFromResponse(pending.meta, body);
    } finally {
      pending.cleanup();
    }
  }

  /**
   * 轮询直到用户确认、设备码过期或被拒绝。
   * 认领 scope 后的 access_token 不会自动写回客户端；用 loginWithDeviceCode() 会自动写回。
   */
  async waitForDeviceToken(
    deviceCode: DeviceCode | string,
    options: DeviceTokenWaitOptions = {},
  ): Promise<DeviceToken> {
    const code = typeof deviceCode === 'string' ? undefined : deviceCode;
    const deviceCodeValue = typeof deviceCode === 'string' ? deviceCode : deviceCode.device_code;
    const baseIntervalMs =
      options.intervalMs ??
      (code !== undefined && Number.isFinite(code.interval) ? Math.max(0, code.interval * 1000) : DEFAULT_DEVICE_INTERVAL_MS);
    const timeoutMs =
      options.timeoutMs ??
      (code !== undefined && Number.isFinite(code.expires_in)
        ? Math.max(0, code.expires_in * 1000)
        : DEFAULT_DEVICE_EXPIRES_MS);
    const deadline = Date.now() + timeoutMs;
    let intervalMs = baseIntervalMs;

    for (let attempt = 1; ; attempt += 1) {
      if (Date.now() >= deadline) {
        throw new DeviceFlowError('expired_token', { status: 400 });
      }
      try {
        return await this.pollDeviceToken(deviceCodeValue, { signal: options.signal });
      } catch (error) {
        if (!(error instanceof DeviceFlowError) || !error.retryable) throw error;
        options.onPending?.({ code: error.flowCode, attempt });
        let delayMs = intervalMs;
        if (error.flowCode === 'slow_down') {
          intervalMs += 5_000;
          delayMs = intervalMs;
        } else if (error.flowCode === 'rate_limited') {
          delayMs =
            error.retryAfter !== undefined ? error.retryAfter * 1000 : Math.max(intervalMs, 5_000);
        }
        const remaining = deadline - Date.now();
        if (remaining <= 0) throw new DeviceFlowError('expired_token', { status: 400 });
        await this.sleepFn(Math.min(delayMs, remaining), options.signal);
      }
    }
  }

  /**
   * 便捷方法：申请设备码 → 交给 onCode 展示/打开链接 → 轮询 → 保存令牌。
   * 返回的 access_token 已写入客户端，后续 listModels()/connectMcp() 直接可用。
   */
  async loginWithDeviceCode(
    request: DeviceCodeRequest = {},
    options: {
      signal?: AbortSignal;
      onCode?: (code: DeviceCode) => void | Promise<void>;
      intervalMs?: number;
      timeoutMs?: number;
      onPending?: DeviceTokenWaitOptions['onPending'];
    } = {},
  ): Promise<DeviceToken> {
    const code = await this.requestDeviceCode(request, { signal: options.signal });
    await options.onCode?.(code);
    const token = await this.waitForDeviceToken(code, {
      signal: options.signal,
      intervalMs: options.intervalMs,
      timeoutMs: options.timeoutMs,
      onPending: options.onPending,
    });
    this.setToken(token.access_token);
    return token;
  }

  // ---- MCP ----

  /**
   * 建立（或复用）MCP 会话。默认已完成 initialize 与 notifications/initialized；
   * 需要自己控制握手时传 `{initialize: false}` 然后调用 session.start()。
   */
  async connectMcp(options: McpSessionOptions = {}): Promise<McpSession> {
    const existing = this.mcpSession;
    if (existing !== undefined && !existing.isClosed) {
      // 之前用 initialize:false 建过会话的话，这里补上握手再复用。
      if (options.initialize !== false && !existing.isInitialized) {
        await existing.start({ signal: options.signal });
      }
      return existing;
    }
    const session = await McpSession.create(this.mcpTransport, {
      protocolVersion: options.protocolVersion ?? this.mcpProtocolVersion,
      clientInfo: options.clientInfo ?? this.mcpClientInfo,
      capabilities: options.capabilities,
      initialize: options.initialize,
      signal: options.signal,
    });
    this.mcpSession = session;
    return session;
  }

  /** 取一页 MCP 工具（默认服务端页大小；`{limit: 0}` 取全量）。 */
  async listTools(options: ListToolsOptions = {}): Promise<McpToolsPage> {
    const session = await this.connectMcp({ signal: options.signal });
    return session.listTools(options);
  }

  /** 跟随 nextCursor 取全部 MCP 工具。 */
  async listAllTools(options: ListAllToolsOptions = {}): Promise<McpTool[]> {
    const session = await this.connectMcp({ signal: options.signal });
    return session.listAllTools(options);
  }

  /** 调用一个 MCP 工具；结果里的 _meta["dsh-ee/confirmation"] 可用 extractConfirmation() 取。 */
  async callTool(
    name: string,
    args: Record<string, unknown> = {},
    options: CallToolOptions = {},
  ): Promise<McpToolCallResult> {
    const session = await this.connectMcp({ signal: options.signal });
    return session.callTool(name, args, options);
  }

  /** 关闭缓存的 MCP 会话（尽力 DELETE /mcp）；之后可以重新 connectMcp()。 */
  async close(): Promise<void> {
    const session = this.mcpSession;
    this.mcpSession = undefined;
    if (session !== undefined) await session.close();
  }

  // ---- 内部 ----

  private async sendMcp(
    body: unknown | null,
    options: McpTransportOptions,
  ): Promise<McpTransportResult> {
    const headers: Record<string, string> = {
      Accept: 'application/json, text/event-stream',
    };
    if (options.sessionId !== undefined) headers['Mcp-Session-Id'] = options.sessionId;
    if (options.protocolVersion !== undefined) headers['MCP-Protocol-Version'] = options.protocolVersion;
    const pending = await this.send('/mcp', {
      method: options.method ?? 'POST',
      body: options.method === 'DELETE' ? undefined : body,
      headers,
      signal: options.signal,
      timeoutMs: options.timeoutMs,
    });
    try {
      const bodyRead = await readBody(pending.response);
      return {
        meta: pending.meta,
        headers: pending.response.headers,
        text: bodyRead.text,
      };
    } finally {
      pending.cleanup();
    }
  }

  /** 发一次 HTTP 请求并更新 last* 观测字段；body 由调用方读取后 cleanup()。 */
  private async send(path: string, options: RequestOptions = {}): Promise<PendingResponse> {
    if (typeof path !== 'string' || !path.startsWith('/')) {
      throw new TypeError(`Request path must be an absolute path starting with /: ${String(path)}`);
    }
    const headers = { ...this.defaultHeaders, ...(options.headers ?? {}) };
    if (!hasHeader(headers, 'accept')) headers.Accept = options.accept ?? 'application/json';
    const method = options.method ?? 'GET';
    let requestBody: string | undefined;
    if (options.body !== undefined) {
      headers['Content-Type'] = headers['Content-Type'] ?? 'application/json';
      requestBody = JSON.stringify(options.body);
    }
    if ((options.auth ?? true) && this.tokenValue !== undefined && this.tokenValue !== '') {
      if (!hasHeader(headers, 'authorization')) headers.Authorization = `Bearer ${this.tokenValue}`;
    }
    const apiVersion = options.apiVersion === undefined ? this.apiVersion : options.apiVersion;
    if (apiVersion !== null && isVersionedPath(path) && !hasHeader(headers, 'dsh-ee-api-version')) {
      headers['Dsh-EE-API-Version'] = apiVersion;
    }

    const pending = await fetchPending(
      this.fetchImpl,
      `${this.baseUrl}${path}`,
      { method, headers, body: requestBody, signal: options.signal },
      options.timeoutMs ?? this.timeoutMs,
    );
    this.lastRequestId = pending.meta.requestId;
    this.lastApiVersion = pending.meta.apiVersion;
    this.lastRateLimit = pending.meta.rateLimit;
    this.onResponse?.({ ...pending.meta, method, path });
    return pending;
  }

  private async requestJson<T>(path: string, options: RequestOptions = {}): Promise<T> {
    const pending = await this.send(path, options);
    try {
      const body = await readBody(pending.response);
      this.throwIfError(pending.meta, body);
      if (body.json === undefined) {
        throw new DshError(
          `HTTP ${pending.meta.status} response is not JSON (content-type: ${pending.meta.contentType || 'unknown'})${body.text ? `: ${truncate(body.text)}` : ''}`,
        );
      }
      return body.json as T;
    } finally {
      pending.cleanup();
    }
  }

  private throwIfError(meta: HttpResponseMeta, body: ReadBodyResult): void {
    if (!meta.ok) throw errorFromResponse(meta, body);
  }
}

function deviceErrorCode(payload: unknown): DeviceFlowErrorCode | undefined {
  if (!isRecord(payload)) return undefined;
  const code = payload.error;
  if (typeof code !== 'string') return undefined;
  return DEVICE_ERROR_CODES.has(code as DeviceFlowErrorCode)
    ? (code as DeviceFlowErrorCode)
    : undefined;
}
