/**
 * 扩展第 2～5 批的接口：写操作确认、字段与访问规则、管理约束、配置变更、知识源、服务扩展、OAuth。
 * 类型与 server/internal/api 里的 JSON 字段一一对应。
 */
import { ApiError, request } from "./api";
import i18n from "../i18n";

// ---- 写操作确认（server/internal/api/write_confirm.go）----

export type ConfirmStatus = "pending" | "approved" | "rejected" | "used";

export interface WriteConfirmation {
  id: number;
  user_id: number;
  asset_id: number;
  tool: string;
  args: string;
  status: ConfirmStatus;
  client: string;
  created_at: string;
  expires_at: string;
  decided_at: string | null;
  /** 企业系统名（连接标题；端侧扩展是「端侧扩展 作者/名字」） */
  system: string;
  slug: string;
  description: string;
  expired: boolean;
}

export type WriteConfirmMode = "off" | "non_dsh" | "always";

// ---- 字段与访问规则（server/internal/access、api/fields.go）----

export type FieldObject = "document" | "kbase" | "user";
export type FieldType = "text" | "number" | "date" | "select" | "tags" | "level";
export type RuleOp = "level_gte" | "intersects" | "contains";

export interface AccessField {
  id?: number;
  object: FieldObject;
  key: string;
  label: string;
  type: FieldType;
  options: string[] | null;
  access: boolean;
  source?: string;
}

export interface FieldItem extends AccessField {
  /** 填了值的对象数 */
  values: number;
  /** 用到这个字段的规则名 */
  rules: string[];
}

export interface AccessRule {
  id?: number;
  name: string;
  enabled: boolean;
  op: RuleOp;
  user_field: string;
  doc_field: string;
  missing: "deny" | "open" | "";
  kbases: string[] | null;
}

export interface ObjectFieldView extends AccessField {
  value: unknown;
  editable: boolean;
}

export interface FieldSyncMapping {
  source: "ldap" | "feishu";
  attr: string;
  field: string;
  override: boolean;
}

export interface ExplainStep {
  stage: string;
  result: string;
  reason: string;
}

// ---- 管理约束与配置变更（api/admin_policy.go、api/config_code.go）----

export interface AdminPolicy {
  super_disabled: boolean;
  no_self_grant: boolean;
  exclusive: string[][];
  dual_approval: string[];
}

export interface PolicyPreset {
  name: string;
  title: string;
  description: string;
}

export interface ChangeRequest {
  id: number;
  kind: "config" | "user_roles";
  title: string;
  /** 配置内容（JSON）；回滚类的变更也是一整份配置 */
  payload: string;
  status: "pending" | "applied" | "rejected" | "failed";
  note: string;
  error: string;
  created_by: number;
  created_by_name?: string;
  via_token: boolean;
  decided_by: number | null;
  decided_by_name?: string;
  created_at: string;
  decided_at: string | null;
}

export interface PlanResult {
  ok: boolean;
  problems: string[];
  changes: string[];
  notes: string[];
  impact: { user: string; kb: string; lose?: string[]; gain?: string[]; recently_read_lost?: string[] }[];
  perm_impact: { user: string; lose?: string[]; gain?: string[] }[];
  summary: string;
}

// ---- 知识源（api/kb_sources.go）----

export interface KBSourceNode {
  id: string;
  title: string;
}

export interface KBSource {
  id: number;
  kb_id: number;
  asset_id: number;
  mode: "sync" | "federated";
  title: string;
  scope: KBSourceNode[];
  folder: string;
  last_sync_at: string | null;
  last_error: string;
  doc_count: number;
  created_at: string;
}

export interface KBSourceExtension {
  asset_id: number;
  slug: string;
  title: string;
  extension: string;
  modes: ("sync" | "federated")[];
  builtin?: boolean;
}

export interface KBBrowseNode {
  id: string;
  title: string;
  kind: "folder" | "document";
  hasChildren?: boolean;
}

export interface KBSyncResult {
  added: number;
  updated: number;
  deleted: number;
  skipped: number;
  docs: number;
  error?: string;
}

// ---- 服务扩展（api/service_ext.go）----

export interface ExtHealth {
  ok: boolean;
  error?: string;
  version?: string;
  /** 扩展现在提供的清单和装的时候不一样：新版本要管理员确认后才生效 */
  changed: boolean;
  new_version?: string;
  new_digest?: string;
  checked_at: string;
}

export interface ExtInfo {
  id: string;
  version: string;
  digest: string;
  kind: "declarative" | "service";
  manifest: Record<string, unknown>;
  config: Record<string, unknown> | null;
  admin: Record<string, unknown> | null;
  service_url?: string;
  endpoint_url?: string;
  oauth_client_id?: string;
  oauth_redirect_uri?: string;
  health?: ExtHealth;
}

// ---- 扩展安装历史与回滚（server/internal/api/extensions_rollback.go）----

export type ExtSignatureStatus = "valid" | "unsigned" | "invalid" | "revoked" | "unknown";

/** 历史里的一个扩展版本；asset_version 是连接的版本号，回滚时用它指定目标。 */
export interface ExtHistoryVersion {
  asset_version: number;
  /** 扩展身份 作者/名字@版本 */
  extension: string;
  version: string;
  digest: string;
  kind: "declarative" | "service";
  /** 按当前信任根重新核对过的签名状态；本改动之前的旧版本没存签名材料，是 unknown */
  signature: ExtSignatureStatus;
  signature_key?: string;
  installed_by: string;
  installed_at: string;
  current: boolean;
  /** 当前策略下能不能回滚到这个版本 */
  can_rollback: boolean;
}

export interface ExtHistory {
  asset: { id: number; slug: string; title: string };
  extension_id: string;
  current_version: number;
  versions: ExtHistoryVersion[];
}

/** 回滚前给管理员看的计划：和安装计划同一个口径的变化 / 敏感项 / 检查结果。 */
export interface ExtRollbackPlan {
  asset: { id: number; slug: string; title: string };
  from: ExtHistoryVersion;
  to: ExtHistoryVersion;
  changes: string[];
  sensitive: string[];
  warnings: string[];
  check?: { ok: boolean; errors: number; results: { code: string; severity: string; message: string; fix?: string }[] };
  signature?: { status: ExtSignatureStatus; key_id?: string; message?: string };
}

export interface ExtRollbackResult {
  version: number;
  tools: number;
  rolled_back_to: ExtHistoryVersion;
  rolled_back_from: ExtHistoryVersion;
  warnings: string[];
}

const extTargetPath = (target: string, action: string) => `/api/admin/extensions/${encodeURIComponent(target)}/${action}`;

export const extApi = {
  // 写操作确认
  myConfirmations: () => request<{ items: WriteConfirmation[] }>("GET", "/api/me/confirmations").then((r) => r.items ?? []),
  confirmation: (id: number) => request<WriteConfirmation>("GET", `/api/me/confirmations/${id}`),
  decideConfirmation: (id: number, approve: boolean) => request<WriteConfirmation>("POST", `/api/me/confirmations/${id}`, { approve }),
  writeConfirmMode: () => request<{ mode: WriteConfirmMode }>("GET", "/api/admin/write-confirm").then((r) => r.mode),
  setWriteConfirmMode: (mode: WriteConfirmMode) => request<{ mode: WriteConfirmMode }>("PUT", "/api/admin/write-confirm", { mode }),

  // 字段与规则
  fields: () => request<{ fields: FieldItem[]; rules: AccessRule[]; builtins: string[] }>("GET", "/api/admin/fields"),
  createField: (f: AccessField) => request<AccessField>("POST", "/api/admin/fields", f),
  patchField: (id: number, p: { label?: string; options?: string[]; access?: boolean }) => request<AccessField>("PATCH", `/api/admin/fields/${id}`, p),
  deleteField: (id: number) => request<void>("DELETE", `/api/admin/fields/${id}`),
  createRule: (r: AccessRule) => request<AccessRule>("POST", "/api/admin/access-rules", r),
  updateRule: (id: number, r: AccessRule) => request<AccessRule>("PUT", `/api/admin/access-rules/${id}`, r),
  deleteRule: (id: number) => request<void>("DELETE", `/api/admin/access-rules/${id}`),
  objectFields: (object: FieldObject, id: number) =>
    request<{ fields: ObjectFieldView[] }>("GET", `/api/fields/${object}/${id}`).then((r) => r.fields ?? []),
  setObjectFields: (object: FieldObject, id: number, values: Record<string, unknown>) =>
    request<{ fields: ObjectFieldView[] }>("PUT", `/api/fields/${object}/${id}`, { values }),
  fieldSync: () => request<{ mappings: FieldSyncMapping[] }>("GET", "/api/admin/field-sync").then((r) => r.mappings ?? []),
  setFieldSync: (mappings: FieldSyncMapping[]) => request<{ mappings: FieldSyncMapping[] }>("PUT", "/api/admin/field-sync", { mappings }),
  explain: (user: string, doc: number) =>
    request<{ allowed: boolean; user: string; doc: number; title: string; steps: ExplainStep[] }>("POST", "/api/admin/access/explain", { user, doc }),

  // 管理约束与变更请求
  policy: () => request<{ policy: AdminPolicy; presets: PolicyPreset[] }>("GET", "/api/admin/policy"),
  setPolicy: (p: AdminPolicy) => request<{ policy: AdminPolicy; presets: PolicyPreset[] }>("PUT", "/api/admin/policy", p),
  applyPreset: (name: string) => request<{ policy: AdminPolicy; presets: PolicyPreset[] }>("POST", "/api/admin/policy/preset", { name }),
  changes: () => request<{ items: ChangeRequest[] }>("GET", "/api/admin/config/changes").then((r) => r.items ?? []),
  decideChange: (id: number, approve: boolean) => request<ChangeRequest>("POST", `/api/admin/config/changes/${id}`, { approve }),
  rollbackChange: (id: number) => request<{ change_request: ChangeRequest }>("POST", `/api/admin/config/changes/${id}/rollback`),
  exportConfig: () =>
    fetch("/api/admin/config", { credentials: "same-origin" }).then(async (r) => {
      if (!r.ok) throw new Error(`HTTP ${r.status}`);
      return r.text();
    }),
  planConfig: (config: string) => request<PlanResult>("POST", "/api/admin/config/plan", { config }),
  submitConfig: (config: string, note: string) =>
    request<{ change_request: ChangeRequest; plan: PlanResult; message: string }>("POST", "/api/admin/config/changes", { config, note }),

  // 知识源
  kbSources: (kb: number) => request<{ sources: KBSource[]; extensions: KBSourceExtension[] }>("GET", `/api/kb/${kb}/sources`),
  kbBrowse: (kb: number, asset_id: number, parent: string) =>
    request<{ items: KBBrowseNode[]; unsupported?: boolean }>("POST", `/api/kb/${kb}/sources/browse`, { asset_id, parent }),
  kbAddSource: (kb: number, input: { asset_id: number; mode: "sync" | "federated"; scope: KBSourceNode[]; folder: string; title: string }) =>
    request<{ source: KBSource; sync?: KBSyncResult }>("POST", `/api/kb/${kb}/sources`, input),
  kbSyncSource: (kb: number, source: number) => request<KBSyncResult>("POST", `/api/kb/${kb}/sources/${source}/sync`),
  kbDeleteSource: (kb: number, source: number, deleteDocs: boolean) =>
    request<void>("DELETE", `/api/kb/${kb}/sources/${source}${deleteDocs ? "?docs=delete" : ""}`),

  // 安装历史与回滚：target 是连接标识（slug）或扩展 id（作者/名字）
  extHistory: (target: string) => request<ExtHistory>("GET", extTargetPath(target, "history")),
  /** 预览回滚计划：接口在没带 confirm 时返回 409 + plan，这里把它当成正常结果取出来。 */
  extRollbackPlan: async (target: string, version: number): Promise<ExtRollbackPlan> => {
    try {
      await request<ExtRollbackResult>("POST", extTargetPath(target, "rollback"), { version });
    } catch (e) {
      if (e instanceof ApiError && e.status === 409 && (e.data as { plan?: ExtRollbackPlan } | null)?.plan) {
        return (e.data as { plan: ExtRollbackPlan }).plan;
      }
      throw e;
    }
    // 不该发生：服务端要求 confirm，永远会先给 409 计划
    throw new Error(i18n.t("extAdmin.rollbackNoPlan"));
  },
  extRollback: (target: string, version: number) => request<ExtRollbackResult>("POST", extTargetPath(target, "rollback"), { version, confirm: true }),

  // 服务扩展与 OAuth
  extInfo: (connection: number) => request<ExtInfo>("GET", `/api/admin/connections/${connection}/extension`),
  extHealth: (connection: number) => request<ExtHealth>("POST", `/api/admin/connections/${connection}/extension/health`),
  extOptions: (connection: number, field: string) =>
    request<{ options: { value: string; label?: string }[] }>("POST", `/api/admin/connections/${connection}/extension/options`, { field }),
  oauthStart: (connection: number) => request<{ url: string }>("POST", `/api/me/connections/${connection}/oauth/start`),
};
