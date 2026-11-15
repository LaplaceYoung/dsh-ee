// 测试用的 fetch / Response 构造工具（不依赖任何测试框架，用 node:test）。

export function jsonResponse(payload, { status = 200, headers = {} } = {}) {
  return new Response(JSON.stringify(payload), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', ...headers },
  });
}

export function emptyResponse(status = 202, headers = {}) {
  return new Response(null, { status, headers });
}

export function textResponse(text, { status = 200, contentType = 'text/plain', headers = {} } = {}) {
  return new Response(text, { status, headers: { 'content-type': contentType, ...headers } });
}

/** 用 SSE 事件文本构造响应；元素是字符串时原样拼，不是字符串时按 data: JSON 拼。 */
export function sseResponse(events, { status = 200, headers = {}, crlf = false } = {}) {
  const eol = crlf ? '\r\n' : '\n';
  const text = events
    .map((event) => (typeof event === 'string' ? event : `data: ${JSON.stringify(event)}${eol}${eol}`))
    .join('');
  return new Response(text, {
    status,
    headers: { 'content-type': 'text/event-stream', ...headers },
  });
}

/** 分块 SSE 响应：把整段文本切成多块推入流，用于验证跨 chunk 的行/事件拼接。 */
export function chunkedSseResponse(text, chunkSize = 7, headers = {}) {
  const encoder = new TextEncoder();
  const bytes = encoder.encode(text);
  const body = new ReadableStream({
    start(controller) {
      for (let offset = 0; offset < bytes.length; offset += chunkSize) {
        controller.enqueue(bytes.slice(offset, offset + chunkSize));
      }
      controller.close();
    },
  });
  return new Response(body, {
    status: 200,
    headers: { 'content-type': 'text/event-stream; charset=utf-8', ...headers },
  });
}

/**
 * 记录所有请求的 mock fetch。handler(call, index) 返回 Response；
 * call = { url, method, headers, body, signal, json() }
 */
export function mockFetch(handler) {
  const calls = [];
  const fetch = async (url, init = {}) => {
    const call = {
      url,
      method: init.method ?? 'GET',
      headers: new Headers(init.headers ?? {}),
      body: init.body,
      signal: init.signal,
      json() {
        if (typeof init.body !== 'string') throw new Error('请求没有 JSON body');
        return JSON.parse(init.body);
      },
    };
    calls.push(call);
    return handler(call, calls.length - 1);
  };
  return { fetch, calls };
}

export const DISCOVERY = {
  name: 'dsh-ee',
  version: '1.2.3',
  api_version: '1',
  device_authorization_endpoint: 'https://ee.example.com/api/device/code',
  token_endpoint: 'https://ee.example.com/api/device/token',
  scopes_supported: ['models', 'registry:read', 'registry:publish', 'hub', 'connections', 'agent'],
  models: {
    protocol: 'openai-chat-completions',
    streaming: true,
    reasoning: {
      supported: true,
      parameters: ['thinking', 'reasoning_effort'],
      content_field: 'reasoning_content',
      per_model: true,
    },
    input_modalities: ['text', 'image', 'file'],
  },
  mcp: { transport: 'streamable-http', protocol_versions: ['2025-11-25', '2025-06-18'] },
  apps: { web: true, data_tools: true },
  limits: { max_request_bytes: 16777216, max_response_bytes: 33554432 },
  deprecation: { sunset_header: 'Sunset' },
};

export const CAPABILITIES = {
  api_version: '1',
  authenticated: true,
  server: { name: 'dsh-ee', version: '1.2.3' },
  user: { username: 'alice', department: '研发' },
  scopes: ['agent', 'connections', 'hub', 'models'],
  models: ['company-chat'],
  reasoning: {
    supported: true,
    parameters: ['thinking', 'reasoning_effort'],
    content_field: 'reasoning_content',
    per_model: true,
  },
  input_modalities: ['text', 'image', 'file'],
  tools: { count: 7 },
  apps: [
    {
      name: 'crm',
      title: '客户管理',
      description: '查询客户',
      icon: '',
      starters: ['查一下客户'],
      launch: 'chat',
      delivery: 'chat',
      version: 2,
      web: true,
    },
  ],
  revision: 'rev-1',
};
