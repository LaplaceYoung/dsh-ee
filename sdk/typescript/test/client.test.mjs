// 客户端 HTTP 面测试：发现、能力、模型、chat、设备码、错误/限流头与网络错误。
// 全部用 mock fetch，不访问网络、不依赖 dsh-ee 实例。

import { test } from 'node:test';
import assert from 'node:assert/strict';

import {
  DeviceFlowError,
  DshApiError,
  DshError,
  DshHarnessClient,
  DshNetworkError,
  SDK_VERSION,
} from '../dist/index.js';
import {
  CAPABILITIES,
  DISCOVERY,
  chunkedSseResponse,
  jsonResponse,
  mockFetch,
} from './helpers.mjs';

const BASE = 'https://ee.example.com';

test('SDK_VERSION 是 0.1.0', () => {
  assert.equal(SDK_VERSION, '0.1.0');
});

test('discovery：解析发现文档、发送版本头、ETag 304 走缓存', async () => {
  const { fetch, calls } = mockFetch((call) => {
    if (call.headers.get('if-none-match') === '"doc-v1"') {
      return new Response(null, { status: 304, headers: { etag: '"doc-v1"' } });
    }
    return jsonResponse(DISCOVERY, {
      headers: { etag: '"doc-v1"', 'Dsh-EE-API-Version': '1', 'X-Request-Id': 'rid-discovery' },
    });
  });
  const client = new DshHarnessClient({ baseUrl: `${BASE}/`, fetch });

  const doc = await client.discovery();
  assert.equal(doc.name, 'dsh-ee');
  assert.equal(doc.api_version, '1');
  assert.deepEqual(doc.mcp.protocol_versions, ['2025-11-25', '2025-06-18']);
  assert.equal(doc.models.reasoning?.content_field, 'reasoning_content');
  assert.deepEqual(doc.models.input_modalities, ['text', 'image', 'file']);
  assert.deepEqual(doc.models.reasoning?.parameters, ['thinking', 'reasoning_effort']);
  assert.equal(calls[0].url, `${BASE}/.well-known/dsh-ee`);
  assert.equal(calls[0].method, 'GET');
  assert.equal(calls[0].headers.get('dsh-ee-api-version'), '1');
  assert.equal(calls[0].headers.get('authorization'), null);
  assert.equal(client.lastRequestId, 'rid-discovery');
  assert.equal(client.lastApiVersion, '1');

  const cached = await client.discovery();
  assert.deepEqual(cached, doc);
  assert.equal(calls.length, 2);
  assert.equal(calls[1].headers.get('if-none-match'), '"doc-v1"');

  const refreshed = await client.discovery({ refresh: true });
  assert.deepEqual(refreshed, doc);
  assert.equal(calls.length, 3);
  assert.equal(calls[2].headers.get('if-none-match'), null);
});

test('capabilities：匿名不带 Authorization，带令牌带 Bearer', async () => {
  const { fetch, calls } = mockFetch((call) =>
    jsonResponse(
      call.headers.get('authorization') === null
        ? { ...CAPABILITIES, authenticated: false, user: null, scopes: [], models: [] }
        : CAPABILITIES,
    ),
  );

  const anonymous = new DshHarnessClient({ baseUrl: BASE, fetch });
  const caps = await anonymous.capabilities();
  assert.equal(caps.authenticated, false);
  assert.equal(calls[0].url, `${BASE}/v1/capabilities`);
  assert.equal(calls[0].headers.get('authorization'), null);

  const authed = new DshHarnessClient({ baseUrl: BASE, fetch, token: 'ee_at_test' });
  const caps2 = await authed.capabilities();
  assert.equal(caps2.authenticated, true);
  assert.equal(caps2.reasoning?.supported, true);
  assert.equal(caps2.reasoning?.per_model, true);
  assert.deepEqual(caps2.input_modalities, ['text', 'image', 'file']);
  assert.equal(calls[1].headers.get('authorization'), 'Bearer ee_at_test');
  assert.equal(caps2.tools.count, 7);
});

test('capabilities：中立错误壳（含 request_id / 错误码）', async () => {
  const { fetch } = mockFetch(() =>
    jsonResponse(
      {
        error: {
          type: 'authentication_error',
          code: 'token_invalid',
          message: '令牌无效、已吊销或已过期',
          request_id: 'rid-err',
        },
      },
      { status: 401 },
    ),
  );
  const client = new DshHarnessClient({ baseUrl: BASE, fetch, token: 'ee_at_bad' });
  await assert.rejects(client.capabilities(), (error) => {
    assert.ok(error instanceof DshApiError);
    assert.equal(error.status, 401);
    assert.equal(error.apiType, 'authentication_error');
    assert.equal(error.apiCode, 'token_invalid');
    assert.equal(error.requestId, 'rid-err');
    return true;
  });
});

test('models：解析模型列表、记录限流头', async () => {
  const { fetch, calls } = mockFetch(() =>
    jsonResponse(
      {
        object: 'list',
        data: [
          {
            id: 'company-chat',
            object: 'model',
            owned_by: 'dsh-ee',
            capabilities: { reasoning: { supported: false, efforts: [] }, input: ['text'] },
          },
          {
            id: 'company-reasoner',
            object: 'model',
            owned_by: 'dsh-ee',
            capabilities: {
              reasoning: { supported: true, efforts: ['low', 'high', 'max'] },
              input: ['text', 'image', 'file'],
            },
          },
        ],
      },
      {
        headers: {
          'X-RateLimit-Limit': '60',
          'X-RateLimit-Remaining': '59',
          'X-RateLimit-Reset': '1700000000',
        },
      },
    ),
  );
  const client = new DshHarnessClient({ baseUrl: BASE, fetch, token: 'ee_at_test' });
  const models = await client.listModels();
  assert.deepEqual(
    models.map((model) => model.id),
    ['company-chat', 'company-reasoner'],
  );
  assert.equal(calls[0].url, `${BASE}/v1/models`);
  // additive 能力声明：旧字段不变，新客户端可按模型读思考能力。
  assert.equal(models[0].capabilities?.reasoning?.supported, false);
  assert.equal(models[1].capabilities?.reasoning?.supported, true);
  assert.deepEqual(models[1].capabilities?.reasoning?.efforts, ['low', 'high', 'max']);
  assert.deepEqual(models[0].capabilities?.input, ['text']);
  assert.deepEqual(models[1].capabilities?.input, ['text', 'image', 'file']);
  assert.equal(client.lastRateLimit.limit, 60);
  assert.equal(client.lastRateLimit.remaining, 59);
  assert.equal(client.lastRateLimit.resetAt.getTime(), 1700000000 * 1000);
});

test('models：OpenAI 错误壳映射成 DshApiError', async () => {
  const { fetch } = mockFetch(() =>
    jsonResponse(
      {
        error: {
          message: '你的账号没有调用大模型的权限，请联系管理员',
          type: 'permission_error',
          code: 'permission_denied',
        },
      },
      { status: 403 },
    ),
  );
  const client = new DshHarnessClient({ baseUrl: BASE, fetch, token: 'ee_at_test' });
  await assert.rejects(client.listModels(), (error) => {
    assert.ok(error instanceof DshApiError);
    assert.equal(error.status, 403);
    assert.equal(error.apiType, 'permission_error');
    assert.equal(error.apiCode, 'permission_denied');
    assert.equal(error.isRateLimited, false);
    return true;
  });
});

test('chat（非流式）：请求体、响应解析与 stream:false', async () => {
  const completion = {
    id: 'chatcmpl-1',
    object: 'chat.completion',
    created: 1700000000,
    model: 'company-chat',
    choices: [
      { index: 0, message: { role: 'assistant', content: 'pong' }, finish_reason: 'stop' },
    ],
    usage: { prompt_tokens: 3, completion_tokens: 2, total_tokens: 5 },
  };
  const { fetch, calls } = mockFetch(() => jsonResponse(completion));
  const client = new DshHarnessClient({ baseUrl: BASE, fetch, token: 'ee_at_test' });

  const result = await client.createChatCompletion({
    model: 'company-chat',
    messages: [{ role: 'user', content: 'ping' }],
    temperature: 0.2,
  });
  assert.equal(result.choices[0].message.content, 'pong');
  assert.equal(result.usage.total_tokens, 5);
  assert.equal(calls[0].url, `${BASE}/v1/chat/completions`);
  assert.equal(calls[0].method, 'POST');
  assert.equal(calls[0].headers.get('authorization'), 'Bearer ee_at_test');
  assert.match(calls[0].headers.get('content-type'), /application\/json/);
  assert.deepEqual(calls[0].json(), {
    model: 'company-chat',
    messages: [{ role: 'user', content: 'ping' }],
    temperature: 0.2,
    stream: false,
  });
});

test('chat：429 限流错误带 Retry-After、X-RateLimit-* 与 OpenAI 壳', async () => {
  const { fetch } = mockFetch(() =>
    jsonResponse(
      {
        error: {
          message: '请求过于频繁，请稍后重试',
          type: 'rate_limit_error',
          code: 'rate_limit_exceeded',
        },
      },
      {
        status: 429,
        headers: {
          'Retry-After': '30',
          'X-RateLimit-Limit': '60',
          'X-RateLimit-Remaining': '0',
          'X-RateLimit-Reset': '1700000060',
        },
      },
    ),
  );
  const client = new DshHarnessClient({ baseUrl: BASE, fetch, token: 'ee_at_test' });
  await assert.rejects(
    client.createChatCompletion({ model: 'company-chat', messages: [] }),
    (error) => {
      assert.ok(error instanceof DshApiError);
      assert.equal(error.status, 429);
      assert.equal(error.isRateLimited, true);
      assert.equal(error.retryAfter, 30);
      assert.equal(error.rateLimit.remaining, 0);
      assert.equal(error.apiCode, 'rate_limit_exceeded');
      assert.equal(error.rateLimit.resetAt.getTime(), 1700000060 * 1000);
      return true;
    },
  );
});

test('chat：createChatCompletion 拒绝 stream:true', async () => {
  const client = new DshHarnessClient({ baseUrl: BASE, fetch: async () => jsonResponse({}) });
  await assert.rejects(
    client.createChatCompletion({ model: 'm', messages: [], stream: true }),
    (error) => {
      assert.ok(error instanceof DshError);
      assert.match(error.message, /streamChatCompletion/);
      return true;
    },
  );
});

test('chat（流式）：跨 chunk 解析 SSE、CRLF、[DONE] 与 usage', async () => {
  const chunks = [
    {
      id: 'c1',
      object: 'chat.completion.chunk',
      created: 1700000000,
      model: 'company-chat',
      choices: [{ index: 0, delta: { role: 'assistant' }, finish_reason: null }],
    },
    {
      id: 'c1',
      object: 'chat.completion.chunk',
      created: 1700000000,
      model: 'company-chat',
      choices: [{ index: 0, delta: { content: '你' }, finish_reason: null }],
    },
    {
      id: 'c1',
      object: 'chat.completion.chunk',
      created: 1700000000,
      model: 'company-chat',
      choices: [],
      usage: { prompt_tokens: 3, completion_tokens: 1, total_tokens: 4 },
    },
  ];
  const text =
    chunks.map((chunk) => `data: ${JSON.stringify(chunk)}\r\n\r\n`).join('') +
    'data: [DONE]\r\n\r\n';
  const { fetch, calls } = mockFetch(() => chunkedSseResponse(text, 5));
  const client = new DshHarnessClient({ baseUrl: BASE, fetch, token: 'ee_at_test' });

  const received = [];
  for await (const chunk of client.streamChatCompletion({
    model: 'company-chat',
    messages: [{ role: 'user', content: 'hi' }],
  })) {
    received.push(chunk);
  }

  assert.equal(received.length, 3);
  assert.equal(received[0].choices[0].delta.role, 'assistant');
  assert.equal(received[1].choices[0].delta.content, '你');
  assert.equal(received[2].usage.total_tokens, 4);
  assert.equal(calls[0].json().stream, true);
  assert.equal(calls[0].headers.get('accept'), 'text/event-stream');
});

test('chat（思考模式）：thinking/reasoning_effort 原样发送，工具轮次带 reasoning_content', async () => {
  const completion = {
    id: 'chatcmpl-think',
    object: 'chat.completion',
    created: 1700000000,
    model: 'company-reasoner',
    choices: [
      {
        index: 0,
        message: {
          role: 'assistant',
          content: '',
          reasoning_content: '先查日期',
          tool_calls: [
            {
              id: 'call_1',
              type: 'function',
              function: { name: 'get_date', arguments: '{}' },
            },
          ],
        },
        finish_reason: 'tool_calls',
      },
    ],
    usage: { prompt_tokens: 9, completion_tokens: 4, total_tokens: 13 },
  };
  const { fetch, calls } = mockFetch(() => jsonResponse(completion));
  const client = new DshHarnessClient({ baseUrl: BASE, fetch, token: 'ee_at_test' });

  const messages = [
    { role: 'user', content: '明天天气怎么样' },
    {
      role: 'assistant',
      content: '',
      reasoning_content: '先查日期',
      tool_calls: [{ id: 'call_0', type: 'function', function: { name: 'get_date', arguments: '{}' } }],
    },
    { role: 'tool', tool_call_id: 'call_0', content: '2026-11-14' },
  ];
  const tools = [{ type: 'function', function: { name: 'get_weather', parameters: { type: 'object' } } }];

  const result = await client.createChatCompletion({
    model: 'company-reasoner',
    messages,
    tools,
    thinking: { type: 'enabled' },
    reasoning_effort: 'high',
  });

  // 请求体原样转发：如果 SDK/网关丢了历史 reasoning_content，上游会 400。
  assert.deepEqual(calls[0].json(), {
    model: 'company-reasoner',
    messages,
    tools,
    thinking: { type: 'enabled' },
    reasoning_effort: 'high',
    stream: false,
  });
  assert.equal(result.choices[0].message.reasoning_content, '先查日期');
  assert.equal(result.choices[0].message.tool_calls[0].function.name, 'get_date');
});

test('chat（流式思考模式）：delta.reasoning_content 原样透出、可与 content 分离', async () => {
  const chunks = [
    {
      id: 'c1',
      object: 'chat.completion.chunk',
      created: 1700000000,
      model: 'company-reasoner',
      choices: [{ index: 0, delta: { role: 'assistant', reasoning_content: '先想' }, finish_reason: null }],
    },
    {
      id: 'c1',
      object: 'chat.completion.chunk',
      created: 1700000000,
      model: 'company-reasoner',
      choices: [{ index: 0, delta: { reasoning_content: '一下' }, finish_reason: null }],
    },
    {
      id: 'c1',
      object: 'chat.completion.chunk',
      created: 1700000000,
      model: 'company-reasoner',
      choices: [{ index: 0, delta: { content: '答案' }, finish_reason: null }],
    },
    {
      id: 'c1',
      object: 'chat.completion.chunk',
      created: 1700000000,
      model: 'company-reasoner',
      choices: [],
      usage: { prompt_tokens: 3, completion_tokens: 1, total_tokens: 4 },
    },
  ];
  const text = chunks.map((chunk) => `data: ${JSON.stringify(chunk)}\n\n`).join('') + 'data: [DONE]\n\n';
  const { fetch, calls } = mockFetch(() => chunkedSseResponse(text, 5));
  const client = new DshHarnessClient({ baseUrl: BASE, fetch, token: 'ee_at_test' });

  const reasoning = [];
  const content = [];
  for await (const chunk of client.streamChatCompletion({
    model: 'company-reasoner',
    messages: [{ role: 'user', content: 'hi' }],
    thinking: { type: 'enabled' },
    reasoning_effort: 'medium',
  })) {
    for (const choice of chunk.choices) {
      if (choice.delta.reasoning_content) reasoning.push(choice.delta.reasoning_content);
      if (choice.delta.content) content.push(choice.delta.content);
    }
  }
  assert.equal(reasoning.join(''), '先想一下');
  assert.equal(content.join(''), '答案');
  assert.equal(calls[0].json().thinking.type, 'enabled');
  assert.equal(calls[0].json().reasoning_effort, 'medium');
});

test('device code：申请参数、公开端点不带令牌', async () => {
  const { fetch, calls } = mockFetch(() =>
    jsonResponse({
      device_code: 'dc-1',
      user_code: 'ABCD-EFGH',
      verification_uri: `${BASE}/device`,
      verification_uri_complete: `${BASE}/device?code=ABCD-EFGH`,
      interval: 5,
      expires_in: 600,
    }),
  );
  const client = new DshHarnessClient({ baseUrl: BASE, fetch, token: 'ee_at_should_not_be_sent' });
  const code = await client.requestDeviceCode({
    clientName: 'my-harness',
    clientOs: 'linux',
    clientVersion: '1.0.0',
    scopes: ['models', 'agent'],
    setupCode: 'setup-1',
  });
  assert.equal(code.device_code, 'dc-1');
  assert.equal(calls[0].url, `${BASE}/api/device/code`);
  assert.equal(calls[0].headers.get('authorization'), null);
  assert.deepEqual(calls[0].json(), {
    client_name: 'my-harness',
    client_os: 'linux',
    client_version: '1.0.0',
    scopes: ['models', 'agent'],
    setup_code: 'setup-1',
  });
});

test('device code：轮询错误码映射与 retryable', async () => {
  const scenarios = [
    { status: 400, body: { error: 'authorization_pending' }, retryable: true },
    { status: 400, body: { error: 'slow_down' }, retryable: true },
    { status: 400, body: { error: 'access_denied' }, retryable: false },
    { status: 400, body: { error: 'expired_token' }, retryable: false },
    { status: 429, body: { error: 'rate_limited' }, retryable: true },
  ];
  for (const scenario of scenarios) {
    const { fetch } = mockFetch(() => jsonResponse(scenario.body, { status: scenario.status }));
    const client = new DshHarnessClient({ baseUrl: BASE, fetch });
    await assert.rejects(client.pollDeviceToken('dc-1'), (error) => {
      assert.ok(error instanceof DeviceFlowError);
      assert.equal(error.flowCode, scenario.body.error);
      assert.equal(error.retryable, scenario.retryable);
      return true;
    });
  }
});

test('waitForDeviceToken：pending → slow_down → 成功，间隔按 slow_down 增加 5 秒', async () => {
  let poll = 0;
  const token = {
    access_token: 'ee_at_new',
    scopes: ['models', 'agent'],
    user: { username: 'alice', department: '研发' },
    expires_at: '2026-01-01T00:00:00Z',
  };
  const { fetch } = mockFetch(() => {
    poll += 1;
    if (poll === 1) return jsonResponse({ error: 'authorization_pending' }, { status: 400 });
    if (poll === 2) return jsonResponse({ error: 'slow_down' }, { status: 400 });
    return jsonResponse(token);
  });
  const sleeps = [];
  const pendingCodes = [];
  const client = new DshHarnessClient({
    baseUrl: BASE,
    fetch,
    sleep: async (ms) => {
      sleeps.push(ms);
    },
  });

  const result = await client.waitForDeviceToken(
    {
      device_code: 'dc-1',
      user_code: 'ABCD',
      verification_uri: `${BASE}/device`,
      verification_uri_complete: `${BASE}/device?code=ABCD`,
      interval: 2,
      expires_in: 60,
    },
    { onPending: ({ code }) => pendingCodes.push(code) },
  );

  assert.equal(result.access_token, 'ee_at_new');
  assert.deepEqual(sleeps, [2000, 7000]);
  assert.deepEqual(pendingCodes, ['authorization_pending', 'slow_down']);
});

test('waitForDeviceToken：本地等待预算用尽抛 expired_token', async () => {
  const { fetch } = mockFetch(() =>
    jsonResponse({ error: 'authorization_pending' }, { status: 400 }),
  );
  const client = new DshHarnessClient({
    baseUrl: BASE,
    fetch,
    sleep: async () => {
      throw new Error('不应该轮询');
    },
  });
  await assert.rejects(
    client.waitForDeviceToken('dc-1', { timeoutMs: 0 }),
    (error) => {
      assert.ok(error instanceof DeviceFlowError);
      assert.equal(error.flowCode, 'expired_token');
      return true;
    },
  );
});

test('loginWithDeviceCode：onCode 回调并自动保存令牌', async () => {
  const callsByPath = [];
  const { fetch, calls } = mockFetch((call) => {
    callsByPath.push(new URL(call.url).pathname);
    if (call.url.endsWith('/api/device/code')) {
      return jsonResponse({
        device_code: 'dc-1',
        user_code: 'ABCD',
        verification_uri: `${BASE}/device`,
        verification_uri_complete: `${BASE}/device?code=ABCD`,
        interval: 1,
        expires_in: 60,
      });
    }
    if (call.url.endsWith('/api/device/token')) {
      return jsonResponse({
        access_token: 'ee_at_new',
        scopes: ['models'],
        user: { username: 'alice' },
        expires_at: '2026-01-01T00:00:00Z',
      });
    }
    return jsonResponse({ object: 'list', data: [{ id: 'company-chat' }] });
  });
  const client = new DshHarnessClient({
    baseUrl: BASE,
    fetch,
    sleep: async () => {},
  });
  let shownCode;
  const result = await client.loginWithDeviceCode(
    { clientName: 'test-harness' },
    { onCode: (code) => (shownCode = code) },
  );

  assert.equal(shownCode.device_code, 'dc-1');
  assert.equal(result.access_token, 'ee_at_new');
  assert.equal(client.hasToken, true);

  await client.listModels();
  const modelsCall = calls[calls.length - 1];
  assert.equal(modelsCall.headers.get('authorization'), 'Bearer ee_at_new');
  assert.deepEqual(callsByPath, ['/api/device/code', '/api/device/token', '/v1/models']);
});

test('listApps / getApp：解析中立微应用字段并做 URL 编码', async () => {
  const app = CAPABILITIES.apps[0];
  const { fetch, calls } = mockFetch((call) => {
    if (call.url.endsWith('/v1/apps')) {
      return jsonResponse({ api_version: '1', revision: 'rev-1', apps: [app] });
    }
    return jsonResponse({ api_version: '1', app });
  });
  const client = new DshHarnessClient({ baseUrl: BASE, fetch, token: 'ee_at_test' });
  const page = await client.listApps();
  assert.equal(page.apps[0].name, 'crm');
  const fetched = await client.getApp('crm/vip');
  assert.equal(fetched.title, '客户管理');
  assert.equal(calls[1].url, `${BASE}/v1/apps/crm%2Fvip`);
});

test('onResponse 钩子与 last* 观测字段', async () => {
  const seen = [];
  const { fetch } = mockFetch((call) =>
    jsonResponse(
      { object: 'list', data: [] },
      {
        headers: {
          'X-Request-Id': 'rid-onresponse',
          'Dsh-EE-API-Version': '1',
          'X-RateLimit-Limit': '10',
          'X-RateLimit-Remaining': '9',
          'X-RateLimit-Reset': '1700000010',
        },
      },
    ),
  );
  const client = new DshHarnessClient({
    baseUrl: BASE,
    fetch,
    token: 'ee_at_test',
    onResponse: (info) => seen.push(info),
  });
  await client.listModels();
  assert.equal(seen.length, 1);
  assert.equal(seen[0].path, '/v1/models');
  assert.equal(seen[0].method, 'GET');
  assert.equal(seen[0].status, 200);
  assert.equal(seen[0].requestId, 'rid-onresponse');
  assert.equal(seen[0].rateLimit.remaining, 9);
  assert.equal(client.lastRequestId, 'rid-onresponse');
  assert.equal(client.lastApiVersion, '1');
  assert.equal(client.lastRateLimit.limit, 10);
});

test('网络错误与超时映射成 DshNetworkError', async () => {
  const broken = new DshHarnessClient({
    baseUrl: BASE,
    fetch: async () => {
      throw new Error('ECONNREFUSED');
    },
  });
  await assert.rejects(broken.discovery(), (error) => {
    assert.ok(error instanceof DshNetworkError);
    assert.equal(error.networkCode, 'network');
    return true;
  });

  const slow = new DshHarnessClient({
    baseUrl: BASE,
    timeoutMs: 20,
    fetch: (url, init) =>
      new Promise((resolve, reject) => {
        init.signal.addEventListener('abort', () => reject(new Error('aborted by client')));
      }),
  });
  await assert.rejects(slow.discovery(), (error) => {
    assert.ok(error instanceof DshNetworkError);
    assert.equal(error.networkCode, 'timeout');
    return true;
  });
});

test('非法 baseUrl / 缺 fetch 时抛出明确错误', () => {
  assert.throws(() => new DshHarnessClient({ baseUrl: '   ' }), TypeError);
  const client = new DshHarnessClient({ baseUrl: BASE, fetch: async () => jsonResponse({}) });
  assert.equal(client.baseUrl, BASE);
});
