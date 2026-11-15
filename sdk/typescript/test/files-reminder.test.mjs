// DeepSeek latest_reminder 与 file / document 内容块：格式化 helper 与请求透传。
// 文件/文档本身由 DeepSeek Files API 上传解析，SDK 只负责把引用块原样放进 chat 请求。

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { DshHarnessClient, formatLatestReminder } from '../dist/index.js';
import { jsonResponse, mockFetch } from './helpers.mjs';

const BASE = 'https://ee.example.com';
const COMPLETION = {
  id: 'chatcmpl-files',
  object: 'chat.completion',
  created: 1700000000,
  model: 'company-bot',
  choices: [{ index: 0, message: { role: 'assistant', content: 'ok' }, finish_reason: 'stop' }],
};

test('latest_reminder：格式化 date,weekday,location,platform,language', () => {
  assert.equal(
    formatLatestReminder({
      date: '2026-11-11',
      weekday: 'Wednesday',
      location: 'Singapore',
      platform: 'Web',
      language: 'English',
    }),
    '2026-11-11,Wednesday,Singapore,Web,English',
  );
  // location 可空
  assert.equal(
    formatLatestReminder({ date: '2026-11-11', weekday: '星期三', platform: 'App', language: '中文' }),
    '2026-11-11,星期三,,App,中文',
  );
  // location 允许包含逗号（位于第三段，不能用 split(',') 反解）
  assert.equal(
    formatLatestReminder({
      date: '2026-11-12',
      weekday: 'Thursday',
      location: 'Paris, France',
      platform: 'Web',
      language: 'English',
    }),
    '2026-11-12,Thursday,Paris, France,Web,English',
  );
});

test('latest_reminder：消息紧贴 user 消息原样透传（含历史多轮）', async () => {
  const { fetch, calls } = mockFetch(() => jsonResponse(COMPLETION));
  const client = new DshHarnessClient({ baseUrl: BASE, fetch, token: 'ee_at_test' });

  const messages = [
    // 历史轮次的 reminder 也原样带回（回传可提高 context cache 命中）。
    { role: 'latest_reminder', content: '2026-11-10,Tuesday,Singapore,Web,English' },
    { role: 'user', content: 'What is DeepSeek?' },
    { role: 'assistant', content: 'DeepSeek is ...' },
    {
      role: 'latest_reminder',
      content: formatLatestReminder({
        date: '2026-11-11',
        weekday: 'Wednesday',
        location: 'Singapore',
        platform: 'Web',
        language: 'English',
      }),
    },
    { role: 'user', content: 'What is in the news today?' },
  ];

  await client.createChatCompletion({ model: 'company-bot', messages });
  assert.deepEqual(calls[0].json().messages, messages);
});

test('file / document 内容块原样透传（file_id / file_data / document source）', async () => {
  const { fetch, calls } = mockFetch(() => jsonResponse(COMPLETION));
  const client = new DshHarnessClient({ baseUrl: BASE, fetch, token: 'ee_at_test' });

  const messages = [
    // file_id：引用 Files API 上传并解析后的文档/图片。
    {
      role: 'user',
      content: [
        { type: 'text', text: '总结这份文档的重点。' },
        { type: 'file', file_id: 'file-api-xxxxxxxxxxxxxxxx' },
      ],
    },
    // file_data：内联 base64（可带 filename），与 file_id 互斥。
    {
      role: 'user',
      content: [
        { type: 'file', file_data: 'data:image/jpeg;base64,/9j/4AAQSkZJRg==', filename: 'image.jpg' },
        { type: 'text', text: '这张图里有什么？' },
      ],
    },
    // Messages API 的 document 块（source: file / text）；dsh-ee 的 /v1 面按 Chat Completions
    // 透传 Chat 请求，这里只验证 SDK 不会改写调用方传入的内容块。
    {
      role: 'user',
      content: [{ type: 'document', source: { type: 'file', file_id: 'file-api-yyyyyyyyyyyyyyyy' } }],
    },
    {
      role: 'user',
      content: [{ type: 'document', source: { type: 'text', media_type: 'text/plain', data: 'The document text' } }],
    },
  ];

  await client.createChatCompletion({ model: 'company-bot', messages });
  assert.deepEqual(calls[0].json().messages, messages);
});
