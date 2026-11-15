// @dsh-ee/harness-client 的公开 API。
//
// 这个包是 dsh-ee 的通用 HTTP/MCP 客户端：
//   - 发现文档与能力摘要（#123 契约）
//   - 设备码登录（OAuth 设备码风格）
//   - OpenAI chat completions 子集（非流式 + SSE 流式）
//   - MCP 会话：initialize / tools/list 分页 / tools/call + 写确认 _meta（#128）
//   - 统一错误与限流头解析（#129）
//
// 不实现 Anthropic Messages、OpenAI Responses 等协议；模型面只覆盖 dsh-ee 转发的那部分
// OpenAI chat completions。浏览器内使用时需要服务端配置 CORS 白名单（#127）。

export { DshHarnessClient } from './client.js';
export type { DshClientOptions, DshResponseInfo } from './client.js';

export {
  McpSession,
  extractConfirmation,
  isPendingConfirmation,
  MCP_DEFAULT_PROTOCOL_VERSION,
} from './mcp.js';
export type {
  McpSessionOptions,
  McpTransport,
  McpTransportOptions,
  McpTransportResult,
} from './mcp.js';

export {
  DeviceFlowError,
  DshApiError,
  DshError,
  DshJsonRpcError,
  DshNetworkError,
} from './errors.js';
export type { DshErrorOptions, DshNetworkErrorCode } from './errors.js';

export { formatLatestReminder } from './types.js';
export { parseRateLimitHeaders, parseRetryAfter } from './http.js';
export type { HttpResponseMeta } from './http.js';

export type {
  CallToolOptions,
  ChatCompletion,
  ChatCompletionChoice,
  ChatCompletionChunk,
  ChatCompletionChunkChoice,
  ChatCompletionRequest,
  ChatContentPart,
  ChatContentPartUnion,
  ChatMessage,
  ChatRole,
  ChatToolDefinition,
  ChatUsage,
  DeepSeekCitation,
  DeepSeekDocumentBlock,
  DeepSeekDocumentSource,
  DeepSeekFileContentPart,
  DeepSeekLatestReminder,
  DeepSeekLatestReminderMessage,
  DeepSeekRefusal,
  DeepSeekSafetyCheck,
  DeepSeekServerToolAction,
  DeepSeekServerToolCall,
  DeepSeekServerToolContentBlock,
  DeepSeekServerToolDeclaration,
  DeepSeekServerToolEvent,
  DeepSeekServerToolPage,
  DeepSeekServerToolSource,
  DeepSeekServerToolStep,
  DeepSeekServerToolType,
  DeepSeekServerToolUsage,
  DeepSeekServerToolUseBlock,
  DeepSeekStopDetails,
  DeepSeekTitleConfig,
  DeepSeekUrlCitation,
  DeepSeekWebFetchResult,
  DeepSeekWebFetchToolResultBlock,
  DeepSeekWebFetchToolResultError,
  DeepSeekWebSearchResult,
  DeepSeekWebSearchToolResultBlock,
  DeepSeekWebSearchToolResultError,
  DeviceCode,
  DeviceCodeRequest,
  DeviceFlowErrorCode,
  DeviceToken,
  DeviceTokenWaitOptions,
  DshConfirmation,
  DshDeviceScope,
  FetchLike,
  HarnessApp,
  HarnessAppDetail,
  HarnessAppsPage,
  HarnessCapabilities,
  HarnessDiscovery,
  HarnessReasoningCapability,
  JsonRpcErrorObject,
  JsonRpcId,
  JsonRpcRequest,
  ListAllToolsOptions,
  ListToolsOptions,
  McpClientInfo,
  McpInitializeResult,
  McpServerInfo,
  McpTool,
  McpToolCallResult,
  McpToolContent,
  McpToolsPage,
  ModelCapabilities,
  ModelInfo,
  RateLimitInfo,
  ReasoningEffort,
  ThinkingConfig,
} from './types.js';

/** 本 SDK 的版本；与服务端 Dsh-EE-API-Version 无关。 */
export const SDK_VERSION = '0.1.0';
