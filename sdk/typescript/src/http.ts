// HTTP 传输层：统一处理超时、响应头（X-Request-Id / Dsh-EE-API-Version / X-RateLimit-* /
// Retry-After）解析、错误壳解析与 SSE 读取。不依赖任何第三方库。

import { DshApiError, DshJsonRpcError, DshNetworkError } from './errors.js';
import type { FetchLike, RateLimitInfo } from './types.js';

/** 一次响应的元数据（不包含 body）。 */
export interface HttpResponseMeta {
  status: number;
  ok: boolean;
  /** 服务端或代理回写的请求号。 */
  requestId?: string;
  /** 契约响应头 Dsh-EE-API-Version。 */
  apiVersion?: string;
  /** X-RateLimit-* 解析结果；端点未限流时没有。 */
  rateLimit?: RateLimitInfo;
  /** Retry-After 解析结果（秒）。 */
  retryAfter?: number;
  /** 发现文档的缓存校验头（#123 的 /.well-known/dsh-ee）。 */
  etag?: string;
  contentType: string;
}

/** 已发出但 body 尚未读取的响应；调用方读完 body 后必须调用 cleanup。 */
export interface PendingResponse {
  response: Response;
  meta: HttpResponseMeta;
  cleanup: () => void;
}

function headerValue(headers: Headers, name: string): string | undefined {
  const value = headers.get(name);
  return value === null ? undefined : value;
}

function parseIntHeader(value: string | null): number | undefined {
  if (value === null || value.trim() === '') return undefined;
  const parsed = Number(value.trim());
  return Number.isFinite(parsed) ? parsed : undefined;
}

/**
 * 解析 X-RateLimit-Limit / Remaining / Reset。
 *
 * dsh-ee 在 /v1/* 与 /mcp 的每个响应上写这三个头（#129）；三个头缺任一就返回 undefined，
 * 避免把半套头当成有效状态。
 */
export function parseRateLimitHeaders(headers: Headers): RateLimitInfo | undefined {
  const limit = parseIntHeader(headers.get('x-ratelimit-limit'));
  const remaining = parseIntHeader(headers.get('x-ratelimit-remaining'));
  const reset = parseIntHeader(headers.get('x-ratelimit-reset'));
  if (limit === undefined || remaining === undefined || reset === undefined) return undefined;
  return {
    limit,
    remaining,
    reset,
    resetAt: new Date(reset * 1000),
  };
}

/**
 * 解析 Retry-After：既支持秒数，也支持 HTTP-date；返回秒（向上取整，至少 0）。
 * 服务端的 429 会带这个头（#129），调用方可直接用它计算退避。
 */
export function parseRetryAfter(headers: Headers): number | undefined {
  const raw = headerValue(headers, 'retry-after');
  if (raw === undefined || raw.trim() === '') return undefined;
  const seconds = Number(raw.trim());
  if (Number.isFinite(seconds) && seconds >= 0) return Math.ceil(seconds);
  const date = Date.parse(raw);
  if (!Number.isNaN(date)) return Math.max(0, Math.ceil((date - Date.now()) / 1000));
  return undefined;
}

function readMeta(response: Response): HttpResponseMeta {
  return {
    status: response.status,
    ok: response.ok,
    requestId: headerValue(response.headers, 'x-request-id'),
    apiVersion: headerValue(response.headers, 'dsh-ee-api-version'),
    rateLimit: parseRateLimitHeaders(response.headers),
    retryAfter: parseRetryAfter(response.headers),
    etag: headerValue(response.headers, 'etag'),
    contentType: headerValue(response.headers, 'content-type') ?? '',
  };
}

/**
 * 发请求并返回尚未读取 body 的 Response + 元数据。
 *
 * - 合并调用方 signal 与 timeoutMs（超时后 fetch 会被 abort，抛 DshNetworkError，networkCode=timeout）；
 * - 不在这里判断 HTTP 状态码，由调用方决定怎样读 body 和抛错；
 * - 读完 body 后务必调用 cleanup，清掉定时器和 abort 监听。
 */
export async function fetchPending(
  fetchImpl: FetchLike,
  url: string,
  init: RequestInit,
  timeoutMs: number,
): Promise<PendingResponse> {
  const controller = new AbortController();
  let timedOut = false;
  const callerSignal = init.signal ?? undefined;
  const onCallerAbort = () => controller.abort();
  callerSignal?.addEventListener('abort', onCallerAbort, { once: true });
  const timer =
    timeoutMs > 0
      ? setTimeout(() => {
          timedOut = true;
          controller.abort();
        }, timeoutMs)
      : undefined;

  const cleanup = () => {
    if (timer !== undefined) clearTimeout(timer);
    callerSignal?.removeEventListener('abort', onCallerAbort);
  };

  let response: Response;
  try {
    response = await fetchImpl(url, { ...init, signal: controller.signal });
  } catch (cause) {
    cleanup();
    if (timedOut) {
      throw new DshNetworkError(`Request timed out (${timeoutMs}ms): ${url}`, 'timeout', { cause });
    }
    if (controller.signal.aborted) {
      throw new DshNetworkError(`Request aborted: ${url}`, 'aborted', { cause });
    }
    throw new DshNetworkError(`Network request failed: ${url}`, 'network', { cause });
  }
  return { response, meta: readMeta(response), cleanup };
}

export interface ReadBodyResult {
  /** 解析成功的 JSON；不是 JSON 或空 body 时为 undefined。 */
  json: unknown;
  /** 原始文本（截断前）；空 body 为空串。 */
  text: string;
}

/** 一次性读掉 body 并按 JSON 解析（解析失败返回 undefined，不抛错）。 */
export async function readBody(response: Response): Promise<ReadBodyResult> {
  const text = await response.text();
  if (text === '') return { json: undefined, text: '' };
  try {
    return { json: JSON.parse(text) as unknown, text };
  } catch {
    return { json: undefined, text };
  }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function asString(value: unknown): string | undefined {
  return typeof value === 'string' ? value : undefined;
}

function asStringArray(value: unknown): string[] | undefined {
  if (!Array.isArray(value)) return undefined;
  const out = value.filter((item): item is string => typeof item === 'string');
  return out.length === value.length ? out : undefined;
}

function truncate(text: string, max: number): string {
  return text.length <= max ? text : `${text.slice(0, max)}…`;
}

/**
 * 把非 2xx 响应解析成带上下文的错误。
 *
 * 覆盖 dsh-ee 实际用到的三种壳：
 *   1. OpenAI 形状（/v1 网关）：`{"error":{"message", "type", "code"}}`
 *   2. 中立错误壳（#123 的 /.well-known/dsh-ee、/v1/capabilities、/v1/apps）：
 *      `{"error":{"type","code","message","request_id","supported_versions"}}`
 *   3. JSON-RPC 错误（/mcp 限流等）：`{"jsonrpc":"2.0","id":null,"error":{"code":-32000,"message":...}}`
 * 另外兼容 `/mcp` HTTP 鉴权失败用的 `{"error":"中文说明","code":"token_invalid"}`。
 */
export function errorFromResponse(meta: HttpResponseMeta, body: ReadBodyResult): DshErrorLike {
  const bodyBase = body.json === undefined ? body.text : body.json;
  const base = {
    status: meta.status,
    requestId: meta.requestId,
    apiVersion: meta.apiVersion,
    rateLimit: meta.rateLimit,
    retryAfter: meta.retryAfter,
    body: bodyBase,
  };

  if (isRecord(body.json)) {
    const payload = body.json;
    const error = payload.error;

    if (payload.jsonrpc === '2.0' && isRecord(error) && typeof error.code === 'number') {
      return new DshJsonRpcError(
        asString(error.message) ?? `JSON-RPC error ${String(error.code)}`,
        error.code,
        { ...base, rpcData: error.data },
      );
    }
    if (isRecord(error)) {
      const requestId = asString(error.request_id) ?? base.requestId;
      const retryAfter = typeof error.retry_after === 'number' ? error.retry_after : base.retryAfter;
      return new DshApiError(asString(error.message) ?? `HTTP ${meta.status}`, {
        ...base,
        requestId,
        retryAfter,
        apiType: asString(error.type),
        apiCode: asString(error.code),
        supportedVersions: asStringArray(error.supported_versions),
      });
    }
    if (typeof error === 'string') {
      return new DshApiError(error, {
        ...base,
        apiCode: asString(payload.code),
      });
    }
  }

  const detail = body.text === '' ? '' : `: ${truncate(body.text, 300)}`;
  return new DshApiError(`HTTP ${meta.status}${detail}`, base);
}

// DshErrorLike 只是给 errorFromResponse 一个返回类型，避免把 errors.ts 的所有类都导进来。
export type DshErrorLike = DshApiError | DshJsonRpcError;

/**
 * 解析完整 SSE 文本里的所有 data 事件（MCP 的 POST 响应可能是 SSE）。
 * 注释行（以 `:` 开头）与非 data 字段被忽略；多行 data 用换行拼起来。
 */
export function parseSseData(text: string): string[] {
  const events: string[] = [];
  const blocks = text.split(/\r?\n\r?\n/);
  for (const block of blocks) {
    const dataLines: string[] = [];
    for (const line of block.split(/\r?\n/)) {
      if (line === '' || line.startsWith(':')) continue;
      const colon = line.indexOf(':');
      const field = colon === -1 ? line : line.slice(0, colon);
      if (field !== 'data') continue;
      let value = colon === -1 ? '' : line.slice(colon + 1);
      if (value.startsWith(' ')) value = value.slice(1);
      dataLines.push(value);
    }
    if (dataLines.length > 0) events.push(dataLines.join('\n'));
  }
  return events;
}

/**
 * 增量读取 fetch 的响应流，逐个产出 SSE data 事件（用于 /v1/chat/completions 的 stream:true）。
 * 调用方负责处理 `[DONE]` 与 JSON 解析。
 */
export async function* iterateSseData(
  stream: ReadableStream<Uint8Array>,
): AsyncGenerator<string, void, undefined> {
  const reader = stream.getReader();
  const decoder = new TextDecoder();
  let buffer = '';
  let dataLines: string[] = [];

  const consumeLine = (line: string): string | undefined => {
    if (line === '') {
      if (dataLines.length === 0) return undefined;
      const data = dataLines.join('\n');
      dataLines = [];
      return data;
    }
    if (line.startsWith(':')) return undefined;
    const colon = line.indexOf(':');
    const field = colon === -1 ? line : line.slice(0, colon);
    if (field !== 'data') return undefined;
    let value = colon === -1 ? '' : line.slice(colon + 1);
    if (value.startsWith(' ')) value = value.slice(1);
    dataLines.push(value);
    return undefined;
  };

  try {
    while (true) {
      const { value, done } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });
      let newline = buffer.indexOf('\n');
      while (newline >= 0) {
        let line = buffer.slice(0, newline);
        buffer = buffer.slice(newline + 1);
        if (line.endsWith('\r')) line = line.slice(0, -1);
        const data = consumeLine(line);
        if (data !== undefined) yield data;
        newline = buffer.indexOf('\n');
      }
    }
    buffer += decoder.decode();
    if (buffer !== '') {
      const line = buffer.endsWith('\r') ? buffer.slice(0, -1) : buffer;
      const data = consumeLine(line);
      if (data !== undefined) yield data;
    }
    if (dataLines.length > 0) yield dataLines.join('\n');
  } finally {
    reader.releaseLock();
  }
}

/** 可取消的 sleep；waitForDeviceToken 用它做轮询间隔（测试可注入替代实现）。 */
export function sleep(ms: number, signal?: AbortSignal): Promise<void> {
  if (ms <= 0) return Promise.resolve();
  return new Promise<void>((resolve, reject) => {
    if (signal?.aborted) {
      reject(new DshNetworkError('Wait aborted', 'aborted'));
      return;
    }
    const onAbort = () => {
      cleanup();
      reject(new DshNetworkError('Wait aborted', 'aborted'));
    };
    const cleanup = () => {
      clearTimeout(timer);
      signal?.removeEventListener('abort', onAbort);
    };
    const timer = setTimeout(() => {
      cleanup();
      resolve();
    }, ms);
    signal?.addEventListener('abort', onAbort, { once: true });
  });
}
