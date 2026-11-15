// SDK 的错误类型。所有错误都带可选的 HTTP 元数据（状态码、请求号、API 版本、限流头），
// 便于调用方做退避、日志对账和排障。

import type { DeviceFlowErrorCode, RateLimitInfo } from './types.js';

export interface DshErrorOptions {
  /** HTTP 状态码（网络错误 / 协议错误时没有）。 */
  status?: number;
  /** 服务端返回或客户端生成的 X-Request-Id。 */
  requestId?: string;
  /** 响应头 Dsh-EE-API-Version。 */
  apiVersion?: string;
  /** 解析后的 X-RateLimit-*。 */
  rateLimit?: RateLimitInfo;
  /** 解析后的 Retry-After（秒）。 */
  retryAfter?: number;
  /** 原始响应体（JSON 或文本），便于调用方自己展示/记录。 */
  body?: unknown;
  cause?: unknown;
}

/** SDK 错误基类。 */
export class DshError extends Error {
  readonly status?: number;
  readonly requestId?: string;
  readonly apiVersion?: string;
  readonly rateLimit?: RateLimitInfo;
  readonly retryAfter?: number;
  readonly body?: unknown;

  constructor(message: string, options: DshErrorOptions = {}) {
    super(message, options.cause === undefined ? undefined : { cause: options.cause });
    this.name = 'DshError';
    this.status = options.status;
    this.requestId = options.requestId;
    this.apiVersion = options.apiVersion;
    this.rateLimit = options.rateLimit;
    this.retryAfter = options.retryAfter;
    this.body = options.body;
  }

  /** 是否被限流（HTTP 429）。 */
  get isRateLimited(): boolean {
    return this.status === 429;
  }
}

/** dsh-ee HTTP 接口返回的错误（OpenAI 壳、中立错误壳、通用 {"error": "..."} 壳）。 */
export class DshApiError extends DshError {
  readonly apiType?: string;
  readonly apiCode?: string;
  readonly supportedVersions?: string[];

  constructor(
    message: string,
    options: DshErrorOptions & {
      apiType?: string;
      apiCode?: string;
      supportedVersions?: string[];
    } = {},
  ) {
    super(message, options);
    this.name = 'DshApiError';
    this.apiType = options.apiType;
    this.apiCode = options.apiCode;
    this.supportedVersions = options.supportedVersions;
  }
}

/** JSON-RPC 层错误（MCP）；HTTP 状态可能仍是 200，也可能是 429 等服务端错误。 */
export class DshJsonRpcError extends DshError {
  readonly rpcCode: number;
  readonly rpcData?: unknown;

  constructor(
    message: string,
    rpcCode: number,
    options: DshErrorOptions & { rpcData?: unknown } = {},
  ) {
    super(message, options);
    this.name = 'DshJsonRpcError';
    this.rpcCode = rpcCode;
    this.rpcData = options.rpcData;
  }
}

export type DshNetworkErrorCode = 'timeout' | 'aborted' | 'network';

/** fetch 本身失败（DNS/连接/超时/调用方取消），不是服务端返回的业务错误。 */
export class DshNetworkError extends DshError {
  readonly networkCode: DshNetworkErrorCode;

  constructor(message: string, networkCode: DshNetworkErrorCode, options: DshErrorOptions = {}) {
    super(message, options);
    this.name = 'DshNetworkError';
    this.networkCode = networkCode;
  }
}

/** 设备码轮询的业务错误；retryable 为 true 时按 interval 继续轮询即可。 */
export class DeviceFlowError extends DshError {
  readonly flowCode: DeviceFlowErrorCode;
  readonly retryable: boolean;

  constructor(flowCode: DeviceFlowErrorCode, options: DshErrorOptions = {}) {
    super(deviceFlowMessage(flowCode), options);
    this.name = 'DeviceFlowError';
    this.flowCode = flowCode;
    this.retryable =
      flowCode === 'authorization_pending' || flowCode === 'slow_down' || flowCode === 'rate_limited';
  }
}

function deviceFlowMessage(code: DeviceFlowErrorCode): string {
  switch (code) {
    case 'authorization_pending':
      return 'Waiting for the user to confirm the device code in the browser';
    case 'slow_down':
      return 'Polling too fast; slow down';
    case 'access_denied':
      return 'The user denied this device code authorization';
    case 'expired_token':
      return 'The device code has expired or already been used';
    case 'rate_limited':
      return 'Device code polling is rate limited';
    default:
      return 'Device code flow failed';
  }
}
