/**
 * 老 /api/* 错误信封 → 界面文案。
 *
 * 后端（server/internal/api/errcodes.go）在保持 error 字段（中文默认文案）逐字不变的前提下，
 * 给语义稳定的错误加了 code 与 params。这里按 code 查 i18n 译文；没有登记译文的 code
 * （还没迁移的领域、还没补的语言包）一律回退服务端 error，行为与引入 code 之前完全一致。
 *
 * 抽成不依赖 i18next 的纯函数，方便在 /tmp 脚本里单独验证映射与回退。
 */

/** 后端错误响应里我们关心的字段（其余字段忽略）。 */
export interface ParsedApiError {
  error: string;
  code: string;
  params: Record<string, unknown>;
}

/** 纯函数需要的最小 i18n 能力，由调用方注入（生产环境是 i18next 单例）。 */
export interface ApiErrorI18n {
  /** i18n 里是否登记了 apiErrors.<code> 的译文 */
  hasCode(code: string): boolean;
  /** 取 apiErrors.<code> 的译文，params 用于 {{占位符}} 插值 */
  translateCode(code: string, params: Record<string, unknown>): string;
  /** 响应里没有可显示的 error 文本时按状态码兜底 */
  statusText(status: number): string;
}

/** 解析响应体里的错误字段；非对象、字段类型不对时按空处理。 */
export function parseApiErrorBody(data: unknown): ParsedApiError {
  const obj = data && typeof data === "object" ? (data as Record<string, unknown>) : {};
  const params = obj.params;
  return {
    error: typeof obj.error === "string" ? obj.error : "",
    code: typeof obj.code === "string" ? obj.code : "",
    params: params && typeof params === "object" && !Array.isArray(params) ? (params as Record<string, unknown>) : {},
  };
}

/**
 * 错误响应 → 展示文案。优先级：
 *   1. 有 code 且 i18n 登记了 apiErrors.<code> → 用译文（params 插值）；
 *   2. 否则回退服务端 error（中文默认文案，老行为）；
 *   3. 再否则按状态码兜底（5xx 用 serverError，其余 requestFailed）。
 */
export function resolveApiErrorText(data: unknown, status: number, i18n: ApiErrorI18n): string {
  const { error, code, params } = parseApiErrorBody(data);
  if (code && i18n.hasCode(code)) {
    try {
      const text = i18n.translateCode(code, params);
      // i18next 的 t() 在缺键时返回 key 本身；保险起见再判一次，别把 key 显示给用户。
      if (text && text !== `apiErrors.${code}`) return text;
    } catch {
      // 译文/插值异常时退回服务端文案，绝不让错误处理本身再抛错。
    }
  }
  return error || i18n.statusText(status);
}
