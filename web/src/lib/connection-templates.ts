import type { AssetSpec } from "./api";

/**
 * 连接器模板（docs/expert-center.md §8 第 4 批）：管理员从常见的接法开始，而不是空表单。
 * 只预填结构（类型、认证方式、员工身份、说明的写法），地址和凭据仍由管理员填。
 */
export type ConnectionTemplateId = "mcpBearer" | "mcpHeader" | "openapiReadonly" | "openapiEEJWT" | "openapiUserSecret";

export interface ConnectionTemplate {
  id: ConnectionTemplateId;
  spec: Partial<AssetSpec>;
}

export const CONNECTION_TEMPLATES: ConnectionTemplate[] = [
  { id: "mcpBearer", spec: { type: "mcp", url: "", auth: { type: "bearer" }, timeout_seconds: 30 } },
  { id: "mcpHeader", spec: { type: "mcp", url: "", auth: { type: "header", header: "X-API-Key" }, timeout_seconds: 30 } },
  { id: "openapiReadonly", spec: { type: "openapi", url: "", auth: { type: "bearer" }, openapi: "", timeout_seconds: 30 } },
  { id: "openapiEEJWT", spec: { type: "openapi", url: "", auth: { type: "none" }, user_auth: { mode: "ee_jwt" }, openapi: "", timeout_seconds: 30 } },
  { id: "openapiUserSecret", spec: { type: "openapi", url: "", auth: { type: "none" }, user_auth: { mode: "user_secret", header: "X-User-Token", hint: "" }, openapi: "", timeout_seconds: 30 } },
];
