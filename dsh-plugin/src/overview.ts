import type { McpBridge } from "./bridge.js";
import type { PluginConfig } from "./dsh.js";
import type { HttpClient } from "./http.js";
import { t } from "./i18n/index.js";

export interface WhoamiViewData {
  username: string;
  department: string;
  token_prefix: string;
  expires_at?: string;
  client_name?: string;
  admin_live?: boolean;
  admin_until?: string;
}

export interface UsageViewData {
  quota: {
    monthly_token_limit: number | null;
    month_tokens: number;
  };
  current_token?: {
    monthly_token_limit: number | null;
    month_tokens: number;
  };
}

export interface EnterpriseOverview {
  me: WhoamiViewData;
  usage?: UsageViewData;
  models: string[];
  default_model?: string;
  version: string;
  latest_version?: string;
}

/** 读不到企业账号的原因，面板据此提示是该登录还是该查网络或访问地址。 */
export interface OverviewFailure {
  reason: "auth" | "server" | "unreachable";
  base: string;
  detail: string;
}

export function overviewFailure(config: PluginConfig, error: unknown): OverviewFailure {
  const err = error as { message?: string; name?: string; cause?: { code?: string; message?: string } };
  const message = String(err?.message ?? error);
  if (message === "HTTP 401") return { reason: "auth", base: config.base, detail: message };
  if (/^HTTP \d+$/.test(message)) return { reason: "server", base: config.base, detail: message };
  const detail = err?.name === "TimeoutError" ? t("overview.timeout") : err?.cause?.code || err?.cause?.message || message;
  return { reason: "unreachable", base: config.base, detail };
}

async function bridgeJson(bridge: McpBridge, name: string, args: unknown): Promise<any> {
  const result = await bridge.callTool(name, args);
  if (result?.isError) throw new Error(t("overview.toolFailed"));
  if (result?.structuredContent) return result.structuredContent;
  const text = result?.content?.find((part: any) => part.type === "text")?.text;
  if (typeof text !== "string") throw new Error(t("overview.toolNoJson"));
  return JSON.parse(text);
}

export async function readEnterpriseOverview(config: PluginConfig, http: HttpClient, bridge: McpBridge): Promise<EnterpriseOverview> {
  const me = await http.json<WhoamiViewData>("/api/dsh/whoami");
  const [usage, modelList, doctor] = await Promise.all([
    bridgeJson(bridge, "ee_me", { action: "usage" }).catch(() => undefined),
    http.json<{ data: { id: string }[] }>("/v1/models").catch(() => undefined),
    bridgeJson(bridge, "ee_doctor", { action: "check" }).catch(() => undefined),
  ]);
  return {
    me,
    usage,
    models: modelList?.data?.map((model) => model.id) ?? [],
    default_model: doctor?.default_model,
    version: config.version,
    latest_version: doctor?.connect_version,
  };
}
