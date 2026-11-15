// DeepSeek Chatbot 扩展：server tools 声明、safety_check / title 透传、工具过程与拒答解析。
// 全部 mock fetch；SDK 不实现服务端工具逻辑，只保证这些扩展字段原样进出。

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { DshHarnessClient } from '../dist/index.js';
import { chunkedSseResponse, jsonResponse, mockFetch } from './helpers.mjs';

const BASE = 'https://ee.example.com';

const SERVER_TOOLS = [{ type: 'deepseek_web_search' }, { type: 'deepseek_read_url' }];

test('chatbot：tools / safety_check / title 原样发送，多轮原样回传 kind/signature', async () => {
  const completion = {
    id: 'chatcmpl-bot',
    object: 'chat.completion',
    created: 1700000000,
    model: 'company-bot',
    choices: [{ index: 0, message: { role: 'assistant', content: 'ok' }, finish_reason: 'stop' }],
  };
  const { fetch, calls } = mockFetch(() => jsonResponse(completion));
  const client = new DshHarnessClient({ baseUrl: BASE, fetch, token: 'ee_at_test' });

  // 第二轮：把上一轮 assistant 消息（含 server_tool_calls 的 kind/signature）原样带回。
  const historyCall = {
    index: 0,
    id: 'srvtoolu_rl_00_abc',
    type: 'deepseek_read_url',
    kind: 'read_link',
    action: { type: 'read_link', url: 'https://example.com' },
    status: 'completed',
    page: { url: 'https://example.com', title: 'Example Domain', published_at: null },
    signature: 'tQU8k+8Ko7MY5XDAvJd0',
  };
  const messages = [
    { role: 'user', content: '总结 https://example.com/' },
    {
      role: 'assistant',
      content: '该域名用于文档示例。',
      server_tool_calls: [{ index: 0, reasoning_content: '', calls: [historyCall] }],
    },
    { role: 'user', content: '引用第一句' },
  ];

  await client.createChatCompletion({
    model: 'company-bot',
    messages,
    tools: SERVER_TOOLS,
    safety_check: { level: 'high' },
    title: { enabled: true },
  });

  // 原样转发：dsh-ee/SDK 不补、不删、不校验这些扩展字段。
  assert.deepEqual(calls[0].json(), {
    model: 'company-bot',
    messages,
    tools: SERVER_TOOLS,
    safety_check: { level: 'high' },
    title: { enabled: true },
    stream: false,
  });
});

test('chatbot（非流式）：解析 server_tool_calls / annotations / title / refusal / usage', async () => {
  const completion = {
    id: 'chatcmpl-bot',
    object: 'chat.completion',
    created: 1700000000,
    model: 'company-bot',
    choices: [
      {
        index: 0,
        message: {
          role: 'assistant',
          reasoning_content: '需要先查一下',
          content: 'DeepSeek V4.1 发布了新能力。',
          title: 'DeepSeek V4.1 新特性',
          annotations: [
            {
              type: 'url_citation',
              url_citation: {
                url: 'https://example.com/news',
                title: 'V4.1 release',
                start_index: 0,
                end_index: 25,
              },
            },
          ],
          server_tool_calls: [
            {
              index: 0,
              reasoning_content: '',
              calls: [
                {
                  index: 0,
                  id: 'srvtoolu_s_00_abc',
                  type: 'deepseek_web_search',
                  kind: 'search',
                  action: { type: 'search', queries: ['DeepSeek V4.1'] },
                  status: 'completed',
                  sources: [
                    {
                      type: 'url',
                      url: 'https://example.com/news',
                      title: 'V4.1 release',
                      snippet: 'DeepSeek released...',
                      site_name: 'Example News',
                      site_icon: 'https://cdn.example.com/icon.png',
                      published_at: '2026-11-13T16:00:00+00:00',
                    },
                  ],
                },
              ],
            },
            {
              index: 1,
              reasoning_content: '读一下链接',
              calls: [
                {
                  index: 0,
                  id: 'srvtoolu_rl_00_def',
                  type: 'deepseek_read_url',
                  kind: 'read_link',
                  action: { type: 'read_link', url: 'https://example.com/post' },
                  status: 'completed',
                  page: { url: 'https://example.com/post', title: 'Post', published_at: '2026-01-02' },
                  signature: 'q3J0b1RmZ1c4cE1x',
                },
              ],
            },
          ],
        },
        finish_reason: 'stop',
      },
    ],
    usage: {
      prompt_tokens: 100,
      completion_tokens: 20,
      total_tokens: 120,
      completion_tokens_details: { safety_tokens: 3 },
      server_tool_use: { web_search_requests: 1, web_fetch_requests: 1, tool_search: 1, tool_fetch: 1, tool_image_search: 0 },
    },
  };
  const { fetch } = mockFetch(() => jsonResponse(completion));
  const client = new DshHarnessClient({ baseUrl: BASE, fetch, token: 'ee_at_test' });

  const result = await client.createChatCompletion({
    model: 'company-bot',
    messages: [{ role: 'user', content: 'V4.1 有什么新特性？' }],
    tools: SERVER_TOOLS,
    safety_check: { level: 'high' },
    title: { enabled: true },
  });

  const message = result.choices[0].message;
  assert.equal(message.title, 'DeepSeek V4.1 新特性');
  assert.equal(message.annotations[0].url_citation.url, 'https://example.com/news');
  const searchCall = message.server_tool_calls[0].calls[0];
  assert.equal(searchCall.kind, 'search');
  assert.equal(searchCall.action.queries[0], 'DeepSeek V4.1');
  assert.equal(searchCall.sources[0].site_name, 'Example News');
  const readCall = message.server_tool_calls[1].calls[0];
  assert.equal(readCall.kind, 'read_link');
  assert.equal(readCall.signature, 'q3J0b1RmZ1c4cE1x');
  assert.equal(readCall.page.title, 'Post');
  assert.equal(result.usage.completion_tokens_details.safety_tokens, 3);
  assert.equal(result.usage.server_tool_use.tool_search, 1);
});

test('chatbot：拒答（refusal + finish_reason content_filter）原样暴露', async () => {
  const completion = {
    id: 'chatcmpl-refusal',
    object: 'chat.completion',
    created: 1700000000,
    model: 'company-bot',
    choices: [
      {
        index: 0,
        message: {
          role: 'assistant',
          content: '',
          refusal: 'The following images were blocked by content moderation: messages.1.content.0',
        },
        finish_reason: 'content_filter',
      },
    ],
  };
  const { fetch } = mockFetch(() => jsonResponse(completion));
  const client = new DshHarnessClient({ baseUrl: BASE, fetch, token: 'ee_at_test' });
  const result = await client.createChatCompletion({
    model: 'company-bot',
    messages: [{ role: 'user', content: '看看这张图' }],
    safety_check: { level: 'high' },
  });
  assert.equal(result.choices[0].finish_reason, 'content_filter');
  assert.match(result.choices[0].message.refusal, /blocked by content moderation/);
  assert.equal(result.choices[0].message.content, '');
});

test('chatbot（流式）：server_tool_event / delta.title / 最后一块完整 server_tool_calls 透出', async () => {
  const call = {
    index: 0,
    id: 'srvtoolu_s_00_abc',
    type: 'deepseek_web_search',
    kind: 'search',
    action: { type: 'search', queries: ['DeepSeek V4.1'] },
    status: 'completed',
    sources: [{ type: 'url', url: 'https://example.com/news', title: 'V4.1 release' }],
  };
  const chunks = [
    {
      id: 'c1',
      object: 'chat.completion.chunk',
      created: 1700000000,
      model: 'company-bot',
      choices: [{ index: 0, delta: { role: 'assistant', reasoning_content: '先搜索' } }],
    },
    {
      id: 'c1',
      object: 'chat.completion.chunk',
      created: 1700000000,
      model: 'company-bot',
      choices: [
        {
          index: 0,
          delta: {},
          server_tool_event: { type: 'call_started', step: 0, call: { ...call, status: 'completed' } },
        },
      ],
    },
    {
      id: 'c1',
      object: 'chat.completion.chunk',
      created: 1700000000,
      model: 'company-bot',
      choices: [{ index: 0, delta: { content: 'DeepSeek V4.1 发布了。' } }],
    },
    {
      id: 'c1',
      object: 'chat.completion.chunk',
      created: 1700000000,
      model: 'company-bot',
      choices: [
        {
          index: 0,
          delta: {
            title: 'DeepSeek V4.1 新特性',
            server_tool_calls: [{ index: 0, reasoning_content: '先搜索', calls: [call] }],
          },
          finish_reason: 'stop',
        },
      ],
      usage: { prompt_tokens: 10, completion_tokens: 5, total_tokens: 15 },
    },
  ];
  const text = chunks.map((chunk) => `data: ${JSON.stringify(chunk)}\n\n`).join('') + 'data: [DONE]\n\n';
  const { fetch, calls } = mockFetch(() => chunkedSseResponse(text, 5));
  const client = new DshHarnessClient({ baseUrl: BASE, fetch, token: 'ee_at_test' });

  const events = [];
  let title = null;
  let finalToolCalls = null;
  for await (const chunk of client.streamChatCompletion({
    model: 'company-bot',
    messages: [{ role: 'user', content: 'V4.1 有什么新特性？' }],
    tools: SERVER_TOOLS,
    safety_check: { level: 'high' },
    title: { enabled: true },
  })) {
    for (const choice of chunk.choices) {
      if (choice.server_tool_event) events.push(choice.server_tool_event);
      if (choice.delta.title) title = choice.delta.title;
      if (choice.delta.server_tool_calls) finalToolCalls = choice.delta.server_tool_calls;
    }
  }
  assert.equal(events.length, 1);
  assert.equal(events[0].type, 'call_started');
  assert.equal(events[0].call.kind, 'search');
  assert.equal(title, 'DeepSeek V4.1 新特性');
  assert.equal(finalToolCalls[0].calls[0].sources[0].title, 'V4.1 release');
  assert.deepEqual(calls[0].json().tools, SERVER_TOOLS);
});
