// MCP 面测试：initialize / 会话头 / tools/list 分页 / tools/call + 写确认 _meta /
// JSON-RPC 错误 / 429 限流 / SSE 响应 / DELETE 关闭。全部用 mock fetch。

import { test } from 'node:test';
import assert from 'node:assert/strict';

import {
  DshApiError,
  DshError,
  DshHarnessClient,
  DshJsonRpcError,
  extractConfirmation,
  isPendingConfirmation,
  McpSession,
} from '../dist/index.js';
import { emptyResponse, jsonResponse, mockFetch, sseResponse } from './helpers.mjs';

const BASE = 'https://ee.example.com';
const INITIALIZE_RESULT = {
  protocolVersion: '2025-06-18',
  capabilities: { tools: { listChanged: true } },
  serverInfo: { name: 'dsh-ee', version: '1.2.3' },
  instructions: '这些工具连着企业内部系统',
};

/** 创建一个处理 MCP 握手的 mock fetch；返回的 calls 里可以按 method 过滤。 */
function mcpFetch({ onToolsList, onToolsCall } = {}) {
  return mockFetch((call) => {
    if (call.method === 'DELETE') return emptyResponse(200);
    const request = call.json();
    switch (request.method) {
      case 'initialize':
        return jsonResponse(
          { jsonrpc: '2.0', id: request.id, result: INITIALIZE_RESULT },
          { headers: { 'mcp-session-id': 'sess-1' } },
        );
      case 'notifications/initialized':
        return emptyResponse(202);
      case 'tools/list':
        return onToolsList(request, call);
      case 'tools/call':
        return onToolsCall(request, call);
      default:
        throw new Error(`未预期的 MCP 方法：${request.method}`);
    }
  });
}

test('MCP initialize：会话头、参数、通知、会话复用', async () => {
  const { fetch, calls } = mcpFetch({
    onToolsList: () => jsonResponse({ jsonrpc: '2.0', id: 1, result: { tools: [] } }),
  });
  const client = new DshHarnessClient({
    baseUrl: BASE,
    fetch,
    token: 'ee_at_test',
    mcpClientInfo: { name: 'my-harness', version: '1.2.0' },
  });

  const session = await client.connectMcp();
  assert.equal(session.sessionId, 'sess-1');
  assert.equal(session.isInitialized, true);
  assert.equal(session.protocolVersion, '2025-06-18');
  assert.equal(session.serverInfo.name, 'dsh-ee');
  assert.equal(session.instructions, '这些工具连着企业内部系统');
  assert.equal(calls.length, 2);

  const init = calls[0];
  assert.equal(init.url, `${BASE}/mcp`);
  assert.equal(init.method, 'POST');
  assert.equal(init.headers.get('authorization'), 'Bearer ee_at_test');
  assert.match(init.headers.get('accept'), /application\/json/);
  assert.match(init.headers.get('accept'), /text\/event-stream/);
  assert.match(init.headers.get('content-type'), /application\/json/);
  assert.equal(init.headers.get('mcp-session-id'), null);
  const initBody = init.json();
  assert.equal(initBody.jsonrpc, '2.0');
  assert.equal(initBody.method, 'initialize');
  assert.equal(initBody.params.protocolVersion, '2025-06-18');
  assert.deepEqual(initBody.params.clientInfo, { name: 'my-harness', version: '1.2.0' });
  assert.deepEqual(initBody.params.capabilities, {});

  const notification = calls[1].json();
  assert.equal(notification.method, 'notifications/initialized');
  assert.equal(notification.id, undefined);
  assert.equal(calls[1].headers.get('mcp-session-id'), 'sess-1');
  assert.equal(calls[1].headers.get('mcp-protocol-version'), '2025-06-18');

  const reused = await client.connectMcp();
  assert.equal(reused, session);
  assert.equal(calls.length, 2, '复用会话不应再发 initialize');
});

test('MCP tools/list：单页、cursor 翻页与 listAllTools 拼接', async () => {
  const allTools = [{ name: 'a' }, { name: 'b' }, { name: 'c' }];
  const { fetch, calls } = mcpFetch({
    onToolsList: (request) => {
      const params = request.params ?? {};
      if (params.limit === 0) {
        return jsonResponse({ jsonrpc: '2.0', id: request.id, result: { tools: allTools } });
      }
      if (params.cursor === 'cur-1') {
        return jsonResponse({ jsonrpc: '2.0', id: request.id, result: { tools: [allTools[2]] } });
      }
      return jsonResponse({
        jsonrpc: '2.0',
        id: request.id,
        result: { tools: [allTools[0], allTools[1]], nextCursor: 'cur-1' },
      });
    },
  });
  const client = new DshHarnessClient({ baseUrl: BASE, fetch });

  const firstPage = await client.listTools();
  assert.deepEqual(
    firstPage.tools.map((tool) => tool.name),
    ['a', 'b'],
  );
  assert.equal(firstPage.nextCursor, 'cur-1');

  const secondPage = await client.listTools({ cursor: 'cur-1' });
  assert.deepEqual(
    secondPage.tools.map((tool) => tool.name),
    ['c'],
  );
  assert.equal(secondPage.nextCursor, undefined);

  const all = await client.listAllTools({ pageSize: 2 });
  assert.deepEqual(
    all.map((tool) => tool.name),
    ['a', 'b', 'c'],
  );

  const toolsListCalls = calls.filter((call) => call.json().method === 'tools/list');
  const paged = toolsListCalls.filter((call) => call.json().params?.limit === 2);
  assert.equal(paged.length, 2);
  assert.equal(paged[0].json().params.cursor, undefined);
  assert.equal(paged[1].json().params.cursor, 'cur-1');

  const full = await client.listTools({ limit: 0 });
  assert.equal(full.tools.length, 3);
  assert.equal(full.nextCursor, undefined);
});

test('MCP tools/list：重复 cursor 会停止翻页并抛错', async () => {
  const { fetch } = mcpFetch({
    onToolsList: (request) =>
      jsonResponse({
        jsonrpc: '2.0',
        id: request.id,
        result: { tools: [{ name: 'a' }], nextCursor: 'same-cursor' },
      }),
  });
  const client = new DshHarnessClient({ baseUrl: BASE, fetch });
  await assert.rejects(client.listAllTools(), (error) => {
    assert.ok(error instanceof DshError);
    assert.match(error.message, /duplicate nextCursor/);
    return true;
  });
});

test('MCP tools/call：写确认 _meta 提取与参数透传', async () => {
  let callCount = 0;
  const { fetch, calls } = mcpFetch({
    onToolsList: () => jsonResponse({ jsonrpc: '2.0', id: 1, result: { tools: [] } }),
    onToolsCall: (request) => {
      callCount += 1;
      if (callCount === 1) {
        return jsonResponse({
          jsonrpc: '2.0',
          id: request.id,
          result: {
            content: [
              {
                type: 'text',
                text: '这个操作会修改「客户管理」里的数据，要先由你本人在网页上确认，现在还没有执行。',
              },
            ],
            isError: true,
            _meta: {
              'dsh-ee/confirmation': {
                id: 42,
                url: `${BASE}/confirm/42`,
                expires_at: '2026-01-01T00:00:00Z',
              },
            },
          },
        });
      }
      return jsonResponse({
        jsonrpc: '2.0',
        id: request.id,
        result: { content: [{ type: 'text', text: '已查询到 1 条记录' }], isError: false },
      });
    },
  });
  const client = new DshHarnessClient({ baseUrl: BASE, fetch });

  const pendingResult = await client.callTool('crm__lookup', { q: '张三' });
  assert.equal(pendingResult.isError, true);
  assert.equal(pendingResult.content[0].text.includes('网页上确认'), true);
  const confirmation = extractConfirmation(pendingResult);
  assert.equal(confirmation.id, 42);
  assert.equal(confirmation.url, `${BASE}/confirm/42`);
  assert.equal(confirmation.expires_at, '2026-01-01T00:00:00Z');
  assert.ok(confirmation.expiresAt instanceof Date);
  assert.equal(confirmation.expiresAt.toISOString(), '2026-01-01T00:00:00.000Z');
  assert.equal(isPendingConfirmation(pendingResult), true);

  const normalResult = await client.callTool('crm__lookup', { q: '李四' });
  assert.equal(normalResult.isError, false);
  assert.equal(extractConfirmation(normalResult), undefined);
  assert.equal(isPendingConfirmation(normalResult), false);

  const toolsCall = calls.filter((call) => call.json().method === 'tools/call')[0];
  assert.deepEqual(toolsCall.json().params, {
    name: 'crm__lookup',
    arguments: { q: '张三' },
  });
});

test('extractConfirmation：字段不完整或没有 _meta 时返回 undefined', () => {
  assert.equal(extractConfirmation({ content: [] }), undefined);
  assert.equal(extractConfirmation({ content: [], _meta: {} }), undefined);
  assert.equal(
    extractConfirmation({ content: [], _meta: { 'dsh-ee/confirmation': { url: 'x' } } }),
    undefined,
  );
  assert.equal(
    extractConfirmation({
      content: [],
      _meta: { 'dsh-ee/confirmation': { id: 1, url: 'https://x/confirm/1', expires_at: 'not-a-date' } },
    }).expiresAt,
    undefined,
  );
});

test('MCP JSON-RPC 错误（HTTP 200 的 error body）', async () => {
  const { fetch } = mcpFetch({
    onToolsList: () => jsonResponse({ jsonrpc: '2.0', id: 1, result: { tools: [] } }),
    onToolsCall: (request) =>
      jsonResponse({
        jsonrpc: '2.0',
        id: request.id,
        error: { code: -32602, message: '没有这个工具，或你没有使用它的权限：nope' },
      }),
  });
  const client = new DshHarnessClient({ baseUrl: BASE, fetch });
  await assert.rejects(client.callTool('nope'), (error) => {
    assert.ok(error instanceof DshJsonRpcError);
    assert.equal(error.rpcCode, -32602);
    assert.equal(error.status, 200);
    assert.match(error.message, /没有这个工具/);
    return true;
  });
});

test('MCP 429 限流：JSON-RPC -32000 + Retry-After + X-RateLimit-*', async () => {
  const { fetch } = mcpFetch({
    onToolsList: () =>
      jsonResponse(
        {
          jsonrpc: '2.0',
          id: null,
          error: { code: -32000, message: '请求过于频繁，请稍后重试' },
        },
        {
          status: 429,
          headers: {
            'Retry-After': '5',
            'X-RateLimit-Limit': '60',
            'X-RateLimit-Remaining': '0',
            'X-RateLimit-Reset': '1700000060',
          },
        },
      ),
  });
  const client = new DshHarnessClient({ baseUrl: BASE, fetch });
  await assert.rejects(client.listTools(), (error) => {
    assert.ok(error instanceof DshJsonRpcError);
    assert.equal(error.rpcCode, -32000);
    assert.equal(error.status, 429);
    assert.equal(error.isRateLimited, true);
    assert.equal(error.retryAfter, 5);
    assert.equal(error.rateLimit.remaining, 0);
    return true;
  });
});

test('MCP HTTP 鉴权/权限错误（{"error":"...","code":...} 壳）', async () => {
  const { fetch } = mcpFetch({
    onToolsList: () =>
      jsonResponse(
        { error: '企业账号登录已失效，在对话框输入 /ee-login 重新登录', code: 'token_invalid' },
        { status: 401 },
      ),
  });
  const client = new DshHarnessClient({ baseUrl: BASE, fetch });
  await assert.rejects(client.listTools(), (error) => {
    assert.ok(error instanceof DshApiError);
    assert.equal(error.status, 401);
    assert.equal(error.apiCode, 'token_invalid');
    assert.match(error.message, /登录已失效/);
    return true;
  });
});

test('MCP 响应为 SSE 时也能解析 initialize', async () => {
  const { fetch } = mockFetch((call) => {
    if (call.method === 'DELETE') return emptyResponse(200);
    const request = call.json();
    if (request.method === 'initialize') {
      const text = `event: message\ndata: ${JSON.stringify({
        jsonrpc: '2.0',
        id: request.id,
        result: INITIALIZE_RESULT,
      })}\n\n`;
      return sseResponse([text], { headers: { 'mcp-session-id': 'sess-sse' } });
    }
    return emptyResponse(202);
  });
  const client = new DshHarnessClient({ baseUrl: BASE, fetch });
  const session = await client.connectMcp();
  assert.equal(session.sessionId, 'sess-sse');
  assert.equal(session.protocolVersion, '2025-06-18');
  assert.equal(session.isInitialized, true);
});

test('MCP close：DELETE /mcp 带会话头；之后可重新握手', async () => {
  const { fetch, calls } = mcpFetch({
    onToolsList: () => jsonResponse({ jsonrpc: '2.0', id: 1, result: { tools: [] } }),
  });
  const client = new DshHarnessClient({ baseUrl: BASE, fetch });
  const session = await client.connectMcp();
  await client.close();
  assert.equal(session.isClosed, true);

  const deleteCall = calls.find((call) => call.method === 'DELETE');
  assert.ok(deleteCall, '应该发出 DELETE /mcp');
  assert.equal(deleteCall.url, `${BASE}/mcp`);
  assert.equal(deleteCall.headers.get('mcp-session-id'), 'sess-1');
  assert.equal(deleteCall.body, undefined);

  const before = calls.length;
  const newSession = await client.connectMcp();
  assert.notEqual(newSession, session);
  assert.equal(newSession.isInitialized, true);
  assert.ok(calls.length > before);
});

test('McpSession：initialize:false 时由调用方控制握手，之后可补握手复用', async () => {
  const { fetch, calls } = mcpFetch({
    onToolsList: () => jsonResponse({ jsonrpc: '2.0', id: 2, result: { tools: [] } }),
  });
  const client = new DshHarnessClient({ baseUrl: BASE, fetch });
  const session = await client.connectMcp({ initialize: false });
  assert.equal(session.isInitialized, false);
  assert.equal(calls.length, 0);
  assert.ok(session instanceof McpSession);

  // 再次 connectMcp()（默认 initialize:true）会复用会话并补上握手。
  const reused = await client.connectMcp();
  assert.equal(reused, session);
  assert.equal(session.isInitialized, true);
  assert.equal(calls.length, 2);

  const tools = await client.listTools();
  assert.deepEqual(tools.tools, []);
});
