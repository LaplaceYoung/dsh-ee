// 类型编译示例（不运行）：由 `npm run typecheck` 的 tsconfig.test.json 编译检查。
// 覆盖 DeepSeek Chatbot、latest_reminder 与 file / document 内容块的公开类型。

import {
  formatLatestReminder,
  type ChatCompletionRequest,
  type ChatMessage,
  type ChatRole,
  type DeepSeekDocumentBlock,
  type DeepSeekFileContentPart,
  type DeepSeekLatestReminder,
  type DeepSeekLatestReminderMessage,
  type DeepSeekSafetyCheck,
  type DeepSeekServerToolDeclaration,
  type DeepSeekTitleConfig,
} from '../src/index.js';

// ---- latest_reminder：格式 date,weekday,location,platform,language ----
const reminder: DeepSeekLatestReminder = {
  date: '2026-11-11',
  weekday: 'Wednesday',
  location: 'Singapore',
  platform: 'Web',
  language: 'English',
};
const reminderContent: string = formatLatestReminder(reminder);
const reminderMessage: DeepSeekLatestReminderMessage = { role: 'latest_reminder', content: reminderContent };
const reminderRole: ChatRole = 'latest_reminder';

// location 可空、可含逗号；platform 只接受 App / Web。
export const reminders: DeepSeekLatestReminder[] = [
  { date: '2026-11-11', weekday: '星期三', location: '', platform: 'App', language: '中文' },
  { date: '2026-11-12', weekday: 'Thursday', location: 'Paris, France', platform: 'Web', language: 'English' },
];
export const messages: ChatMessage[] = [reminderMessage, { role: 'user', content: '今天有什么新闻？' }];

// ---- file / document 内容块 ----
// file_id：引用 DeepSeek Files API 上传并解析后的文件。
const filePart: DeepSeekFileContentPart = { type: 'file', file_id: 'file-api-xxxxxxxxxxxxxxxx' };
// file_data：内联 base64 data URL（可带 filename），与 file_id 互斥。
const inlineFilePart: DeepSeekFileContentPart = {
  type: 'file',
  file_data: 'data:image/jpeg;base64,/9j/4AAQSkZJRg==',
  filename: 'image.jpg',
};
// Messages API 形状的 document 块：source 为 file 或 text。
const documentBlock: DeepSeekDocumentBlock = {
  type: 'document',
  source: { type: 'file', file_id: 'file-api-xxxxxxxxxxxxxxxx' },
};
const textDocumentBlock: DeepSeekDocumentBlock = {
  type: 'document',
  source: { type: 'text', media_type: 'text/plain', data: 'The document text' },
};

const fileMessage: ChatMessage = {
  role: 'user',
  content: [{ type: 'text', text: '总结这份文档的重点。' }, filePart],
};
const inlineImageMessage: ChatMessage = {
  role: 'user',
  content: [inlineFilePart, { type: 'text', text: '这张图里有什么？' }],
};

// ---- 组合成请求 ----
const tools: DeepSeekServerToolDeclaration[] = [{ type: 'deepseek_web_search' }, { type: 'deepseek_read_url' }];
const safetyCheck: DeepSeekSafetyCheck = { level: 'high' };
const title: DeepSeekTitleConfig = { enabled: true };
export const request: ChatCompletionRequest = {
  model: 'company-bot',
  messages: [reminderMessage, fileMessage, inlineImageMessage],
  tools,
  safety_check: safetyCheck,
  title,
};
export const typedContent = [documentBlock, textDocumentBlock, reminderRole];

// ---- 类型约束：file_id 与 file_data 不能同时出现 ----
// @ts-expect-error file_id 与 file_data 互斥
const bothFields: DeepSeekFileContentPart = { type: 'file', file_id: 'file-api-x', file_data: 'data:image/png;base64,x' };
// @ts-expect-error 必须提供 file_id 或 file_data 之一
const missingFile: DeepSeekFileContentPart = { type: 'file' };
export const negativeExamples = [bothFields, missingFile];
