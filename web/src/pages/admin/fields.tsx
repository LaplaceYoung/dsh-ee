import { useState, type ReactNode } from "react";
import { useSearchParams } from "react-router";
import { useTranslation } from "react-i18next";
import { Check, Plus, RotateCcw, Trash2, X } from "lucide-react";
import { formatTime } from "../../lib/api";
import {
  extApi,
  type AccessField,
  type AccessRule,
  type AdminPolicy,
  type ChangeRequest,
  type FieldItem,
  type FieldSyncMapping,
  type FieldObject,
  type FieldType,
  type PlanResult,
  type RuleOp,
} from "../../lib/ext-api";
import { useAction, useApi } from "../../lib/use-api";
import { useAuth } from "../../lib/auth";
import { useStore } from "../../lib/store";
import { useConfirm } from "../../lib/confirm";
import { LoadState } from "../../components/api-ui";
import { Badge, Button, Card, CardHeader, cx, Field, inputCls, Notice, PageHeader, Segmented, tabCls, tabListCls, Toggle, ToggleRow, type Tone } from "../../components/ui";

type Tab = "fields" | "rules" | "sync" | "policy" | "changes" | "explain";

const OBJECTS: FieldObject[] = ["document", "kbase", "user"];
const TYPES: FieldType[] = ["text", "number", "date", "select", "tags", "level"];
const OPS: RuleOp[] = ["level_gte", "intersects", "contains"];
const textareaCls = "w-full rounded-md border border-neutral-300 bg-card px-3 py-2 text-sm outline-none placeholder:text-neutral-500 focus:border-brand-500 focus:ring-2 focus:ring-brand-100";

const lines = (s: string) =>
  s
    .split("\n")
    .map((x) => x.trim())
    .filter(Boolean);

/**
 * 「字段与访问规则」（docs/access-control.md §4～§5、§3；docs/agent-dev.md §6.2）：
 * 字段、规则、管理约束、配置变更请求，以及「为什么看不到」的解释。批准变更永远在这个页面上由人来点。
 */
export default function AdminFields() {
  const { t } = useTranslation();
  const { can } = useAuth();
  const [params, setParams] = useSearchParams();
  const tabs: Tab[] = ["fields", "rules", "sync", ...(can("roles.manage") ? (["policy"] as const) : []), "changes", "explain"];
  const tab = (tabs.includes(params.get("tab") as Tab) ? params.get("tab") : "fields") as Tab;
  const data = useApi(extApi.fields);
  const policy = useApi(() => (can("roles.manage") ? extApi.policy() : Promise.resolve(null)));
  const dual = policy.data?.policy.dual_approval.includes("config") ?? false;

  return (
    <div className="max-w-5xl">
      <PageHeader title={t("fieldsAdmin.title")} desc={t("fieldsAdmin.desc")} />
      <div role="tablist" className={cx(tabListCls, "mb-5")}>
        {tabs.map((k) => (
          <button key={k} type="button" role="tab" aria-selected={tab === k} className={tabCls(tab === k)} onClick={() => setParams(k === "fields" ? {} : { tab: k })}>
            {t(`fieldsAdmin.tabs.${k}`)}
          </button>
        ))}
      </div>
      {dual && tab !== "changes" && tab !== "explain" && (
        <Notice tone="warn" className="mb-4">
          {t("fieldsAdmin.dualNotice")}
        </Notice>
      )}
      {(tab === "fields" || tab === "rules" || tab === "sync") && (
      <LoadState loading={data.loading && !data.data} error={data.error} onRetry={data.reload}>
        {data.data && tab === "fields" && <FieldsTab fields={data.data.fields} reload={data.reload} />}
        {data.data && tab === "rules" && <RulesTab fields={data.data.fields} rules={data.data.rules} builtins={data.data.builtins} reload={data.reload} />}
        {data.data && tab === "sync" && <SyncTab fields={data.data.fields} />}
      </LoadState>
      )}
      {tab === "policy" && policy.data && <PolicyTab policy={policy.data.policy} presets={policy.data.presets} reload={policy.reload} />}
      {tab === "changes" && <ChangesTab onApplied={() => { data.reload(); policy.reload(); }} />}
      {tab === "explain" && <ExplainTab />}
      <p className="mt-6 text-xs text-neutral-500">{t("fieldsAdmin.cliHint")}</p>
    </div>
  );
}

// ---- 字段 ----

function FieldsTab({ fields, reload }: { fields: FieldItem[]; reload: () => void }) {
  const { t } = useTranslation();
  const [adding, setAdding] = useState(false);
  return (
    <div className="space-y-5">
      <div className="flex justify-end">
        <Button size="sm" onClick={() => setAdding((x) => !x)}>
          <Plus size={14} /> {t("fieldsAdmin.newField")}
        </Button>
      </div>
      {adding && <FieldForm onDone={() => { setAdding(false); reload(); }} onCancel={() => setAdding(false)} />}
      {OBJECTS.map((o) => {
        const list = fields.filter((f) => f.object === o);
        return (
          <Card key={o}>
            <CardHeader title={t(`fieldsAdmin.object.${o}`)} aside={<span className="text-xs text-neutral-500">{list.length}</span>} />
            {list.length === 0 ? (
              <p className="px-5 pb-5 text-sm text-neutral-500">{t("fieldsAdmin.noFields")}</p>
            ) : (
              <ul className="divide-y divide-neutral-100 border-t border-neutral-100">
                {list.map((f) => (
                  <FieldRow key={f.id} f={f} reload={reload} />
                ))}
              </ul>
            )}
          </Card>
        );
      })}
    </div>
  );
}

function FieldForm({ onDone, onCancel }: { onDone: () => void; onCancel: () => void }) {
  const { t } = useTranslation();
  const { notify } = useStore();
  const { busy, run } = useAction();
  const [f, setF] = useState<AccessField>({ object: "document", key: "", label: "", type: "select", options: [], access: true });
  const [opts, setOpts] = useState("");
  const withOptions = f.type === "select" || f.type === "tags" || f.type === "level";
  const save = async () => {
    const r = await run(() => extApi.createField({ ...f, key: f.key.trim(), label: f.label.trim(), options: withOptions ? lines(opts) : null }));
    if (!r.ok) return notify(r.error);
    notify(t("fieldsAdmin.saved"));
    onDone();
  };
  return (
    <Card className="p-5" data-field-form>
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label={t("fieldsAdmin.objectLabel")}>
          <Segmented value={f.object} onChange={(v) => setF({ ...f, object: v })} options={OBJECTS.map((o) => ({ value: o, label: t(`fieldsAdmin.object.${o}`) }))} />
        </Field>
        <Field label={t("fieldsAdmin.fieldType")}>
          <select className={inputCls} value={f.type} onChange={(e) => setF({ ...f, type: e.target.value as FieldType })}>
            {TYPES.map((x) => (
              <option key={x} value={x}>
                {t(`fieldsAdmin.type.${x}`)}
              </option>
            ))}
          </select>
        </Field>
        <Field label={t("fieldsAdmin.key")} hint={t("fieldsAdmin.keyHint")}>
          <input className={cx(inputCls, "font-mono")} value={f.key} onChange={(e) => setF({ ...f, key: e.target.value })} placeholder="level" />
        </Field>
        <Field label={t("fieldsAdmin.label")}>
          <input className={inputCls} value={f.label} onChange={(e) => setF({ ...f, label: e.target.value })} />
        </Field>
        {withOptions && (
          <div className="sm:col-span-2">
            <Field label={t("fieldsAdmin.options")} hint={t("fieldsAdmin.optionsHint")}>
              <textarea className={textareaCls} rows={4} value={opts} onChange={(e) => setOpts(e.target.value)} />
            </Field>
          </div>
        )}
        <div className="sm:col-span-2">
          <ToggleRow label={t("fieldsAdmin.access")} desc={t("fieldsAdmin.accessHint")} checked={f.access} onChange={(v) => setF({ ...f, access: v })} />
        </div>
      </div>
      <div className="mt-4 flex justify-end gap-2">
        <Button variant="secondary" onClick={onCancel}>
          {t("common.cancel")}
        </Button>
        <Button disabled={busy || !f.key.trim() || !f.label.trim()} onClick={() => void save()}>
          {t("common.save")}
        </Button>
      </div>
    </Card>
  );
}

function FieldRow({ f, reload }: { f: FieldItem; reload: () => void }) {
  const { t } = useTranslation();
  const { notify } = useStore();
  const confirm = useConfirm();
  const { busy, run } = useAction();
  const [editing, setEditing] = useState(false);
  const [label, setLabel] = useState(f.label);
  const [opts, setOpts] = useState((f.options ?? []).join("\n"));
  const withOptions = f.type === "select" || f.type === "tags" || f.type === "level";
  const patch = async (p: { label?: string; options?: string[]; access?: boolean }) => {
    const r = await run(() => extApi.patchField(f.id!, p));
    notify(r.ok ? t("fieldsAdmin.saved") : r.error);
    if (r.ok) {
      setEditing(false);
      reload();
    }
  };
  return (
    <li className="px-5 py-3 text-sm" data-field={f.key}>
      <div className="flex flex-wrap items-center gap-2">
        <span className="font-medium text-neutral-900">{f.label}</span>
        <span className="font-mono text-xs text-neutral-500">{f.key}</span>
        <Badge>{t(`fieldsAdmin.type.${f.type}`)}</Badge>
        {f.access && <Badge tone="amber">{t("objectFields.accessBadge")}</Badge>}
        {f.source && <Badge tone="brand">{t("fieldsAdmin.source", { source: f.source })}</Badge>}
        <span className="ml-auto flex items-center gap-2">
          <Toggle checked={f.access} disabled={busy} onChange={(v) => void patch({ access: v })} />
          <Button variant="ghost" size="sm" onClick={() => setEditing((x) => !x)}>
            {t("common.edit")}
          </Button>
          <Button
            variant="ghost"
            size="sm"
            disabled={busy || f.rules.length > 0}
            aria-label={t("common.delete")}
            onClick={async () => {
              if (!(await confirm({ kind: "delete", description: t("fieldsAdmin.deleteConfirm", { label: f.label }) }))) return;
              const r = await run(() => extApi.deleteField(f.id!));
              notify(r.ok ? t("fieldsAdmin.saved") : r.error);
              if (r.ok) reload();
            }}
          >
            <Trash2 size={14} />
          </Button>
        </span>
      </div>
      <div className="mt-1 flex flex-wrap gap-x-4 text-xs text-neutral-500">
        {withOptions && <span>{(f.options ?? []).join(f.type === "level" ? " < " : " / ")}</span>}
        <span>{t("fieldsAdmin.values", { count: f.values })}</span>
        {f.rules.length > 0 && <span>{t("fieldsAdmin.usedBy", { rules: f.rules.join("、") })}</span>}
      </div>
      {editing && (
        <div className="mt-3 grid gap-3 sm:grid-cols-2">
          <Field label={t("fieldsAdmin.label")}>
            <input className={inputCls} value={label} onChange={(e) => setLabel(e.target.value)} />
          </Field>
          {withOptions && (
            <Field label={t("fieldsAdmin.options")} hint={t("fieldsAdmin.optionsHint")}>
              <textarea className={textareaCls} rows={4} value={opts} onChange={(e) => setOpts(e.target.value)} />
            </Field>
          )}
          <div className="flex gap-2 sm:col-span-2">
            <Button size="sm" disabled={busy} onClick={() => void patch({ label: label.trim(), ...(withOptions ? { options: lines(opts) } : {}) })}>
              {t("common.save")}
            </Button>
            <Button size="sm" variant="secondary" onClick={() => setEditing(false)}>
              {t("common.cancel")}
            </Button>
          </div>
        </div>
      )}
    </li>
  );
}

// ---- 访问规则 ----

const emptyRule: AccessRule = { name: "", enabled: true, op: "level_gte", user_field: "", doc_field: "", missing: "deny", kbases: [] };

function RulesTab({ fields, rules, builtins, reload }: { fields: FieldItem[]; rules: AccessRule[]; builtins: string[]; reload: () => void }) {
  const { t } = useTranslation();
  const [editing, setEditing] = useState<AccessRule | null>(null);
  return (
    <div className="space-y-4">
      <div className="flex justify-end">
        <Button size="sm" onClick={() => setEditing({ ...emptyRule })}>
          <Plus size={14} /> {t("fieldsAdmin.newRule")}
        </Button>
      </div>
      {editing && !editing.id && <RuleForm rule={editing} fields={fields} builtins={builtins} onDone={() => { setEditing(null); reload(); }} onCancel={() => setEditing(null)} />}
      <Card>
        {rules.length === 0 ? (
          <p className="p-5 text-sm text-neutral-500">{t("fieldsAdmin.noRules")}</p>
        ) : (
          <ul className="divide-y divide-neutral-100">
            {rules.map((r) =>
              editing?.id === r.id ? (
                <li key={r.id} className="p-3">
                  <RuleForm rule={r} fields={fields} builtins={builtins} onDone={() => { setEditing(null); reload(); }} onCancel={() => setEditing(null)} />
                </li>
              ) : (
                <RuleRow key={r.id} r={r} fields={fields} onEdit={() => setEditing({ ...r })} reload={reload} />
              ),
            )}
          </ul>
        )}
      </Card>
    </div>
  );
}

function fieldLabel(fields: FieldItem[], object: FieldObject, key: string, t: (k: string) => string) {
  if (key.startsWith("@")) return t(`fieldsAdmin.builtin.${key}`);
  return fields.find((f) => f.object === object && f.key === key)?.label ?? key;
}

function RuleRow({ r, fields, onEdit, reload }: { r: AccessRule; fields: FieldItem[]; onEdit: () => void; reload: () => void }) {
  const { t } = useTranslation();
  const { notify } = useStore();
  const confirm = useConfirm();
  const { busy, run } = useAction();
  const tt = (k: string) => t(k as never);
  return (
    <li className="px-5 py-3 text-sm" data-rule={r.name}>
      <div className="flex flex-wrap items-center gap-2">
        <span className="font-medium text-neutral-900">{r.name}</span>
        {!r.enabled && <Badge>{t("fieldsAdmin.disabledBadge")}</Badge>}
        <span className="ml-auto flex items-center gap-2">
          <Toggle
            checked={r.enabled}
            disabled={busy}
            onChange={async (v) => {
              const res = await run(() => extApi.updateRule(r.id!, { ...r, enabled: v }));
              notify(res.ok ? t("fieldsAdmin.saved") : res.error);
              reload();
            }}
          />
          <Button variant="ghost" size="sm" onClick={onEdit}>
            {t("common.edit")}
          </Button>
          <Button
            variant="ghost"
            size="sm"
            disabled={busy}
            aria-label={t("common.delete")}
            onClick={async () => {
              if (!(await confirm({ kind: "delete", description: t("fieldsAdmin.ruleDeleteConfirm", { name: r.name }) }))) return;
              const res = await run(() => extApi.deleteRule(r.id!));
              notify(res.ok ? t("fieldsAdmin.saved") : res.error);
              if (res.ok) reload();
            }}
          >
            <Trash2 size={14} />
          </Button>
        </span>
      </div>
      <p className="mt-1 text-xs text-neutral-600">
        {t(`fieldsAdmin.op.${r.op}`)} · {fieldLabel(fields, "user", r.user_field, tt)} · {fieldLabel(fields, "document", r.doc_field, tt)}
        {" · "}
        {r.missing === "open" ? t("fieldsAdmin.missingOpen") : t("fieldsAdmin.missingDeny")}
        {(r.kbases ?? []).length > 0 && ` · ${(r.kbases ?? []).join(", ")}`}
      </p>
    </li>
  );
}

function RuleForm({ rule, fields, builtins, onDone, onCancel }: { rule: AccessRule; fields: FieldItem[]; builtins: string[]; onDone: () => void; onCancel: () => void }) {
  const { t } = useTranslation();
  const { notify } = useStore();
  const { busy, run } = useAction();
  const [r, setR] = useState<AccessRule>(rule);
  const [kbases, setKbases] = useState((rule.kbases ?? []).join(", "));
  const userFields = fields.filter((f) => f.object === "user");
  const docFields = fields.filter((f) => f.object === "document");
  const save = async () => {
    const body: AccessRule = { ...r, name: r.name.trim(), kbases: kbases.split(",").map((x) => x.trim()).filter(Boolean) };
    const res = await run(() => (r.id ? extApi.updateRule(r.id, body) : extApi.createRule(body)));
    if (!res.ok) return notify(res.error);
    notify(t("fieldsAdmin.saved"));
    onDone();
  };
  return (
    <Card className="p-5" data-rule-form>
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label={t("fieldsAdmin.ruleName")}>
          <input className={inputCls} value={r.name} onChange={(e) => setR({ ...r, name: e.target.value })} />
        </Field>
        <Field label={t("fieldsAdmin.missing")}>
          <Segmented
            value={r.missing === "open" ? "open" : "deny"}
            onChange={(v) => setR({ ...r, missing: v })}
            options={[
              { value: "deny", label: t("fieldsAdmin.missingDeny") },
              { value: "open", label: t("fieldsAdmin.missingOpen") },
            ]}
          />
        </Field>
        <div className="sm:col-span-2">
          <Field label="">
            <select className={inputCls} value={r.op} onChange={(e) => setR({ ...r, op: e.target.value as RuleOp })} aria-label="op">
              {OPS.map((o) => (
                <option key={o} value={o}>
                  {t(`fieldsAdmin.op.${o}`)}
                </option>
              ))}
            </select>
          </Field>
        </div>
        <Field label={t("fieldsAdmin.userField")}>
          <select className={inputCls} value={r.user_field} onChange={(e) => setR({ ...r, user_field: e.target.value })}>
            <option value="">—</option>
            {userFields.map((f) => (
              <option key={f.key} value={f.key}>
                {f.label}
              </option>
            ))}
            {builtins.map((b) => (
              <option key={b} value={b}>
                {t(`fieldsAdmin.builtin.${b}` as never)}
              </option>
            ))}
          </select>
        </Field>
        <Field label={t("fieldsAdmin.docField")}>
          <select className={inputCls} value={r.doc_field} onChange={(e) => setR({ ...r, doc_field: e.target.value })}>
            <option value="">—</option>
            {docFields.map((f) => (
              <option key={f.key} value={f.key}>
                {f.label}
              </option>
            ))}
          </select>
        </Field>
        <div className="sm:col-span-2">
          <Field label={t("fieldsAdmin.kbases")} hint={t("fieldsAdmin.kbasesHint")}>
            <input className={cx(inputCls, "font-mono")} value={kbases} onChange={(e) => setKbases(e.target.value)} />
          </Field>
        </div>
        <div className="sm:col-span-2">
          <ToggleRow label={t("fieldsAdmin.enabled")} checked={r.enabled} onChange={(v) => setR({ ...r, enabled: v })} />
        </div>
      </div>
      <div className="mt-4 flex justify-end gap-2">
        <Button variant="secondary" onClick={onCancel}>
          {t("common.cancel")}
        </Button>
        <Button disabled={busy || !r.name.trim() || !r.user_field || !r.doc_field} onClick={() => void save()}>
          {t("common.save")}
        </Button>
      </div>
    </Card>
  );
}

// ---- 组织同步带字段 ----

function SyncTab({ fields }: { fields: FieldItem[] }) {
  const { t } = useTranslation();
  const { notify } = useStore();
  const { busy, run } = useAction();
  const data = useApi(extApi.fieldSync);
  const [rows, setRows] = useState<FieldSyncMapping[] | null>(null);
  const userFields = fields.filter((f) => f.object === "user");
  const list = rows ?? data.data ?? [];
  const set = (i: number, p: Partial<FieldSyncMapping>) => setRows(list.map((r, j) => (j === i ? { ...r, ...p } : r)));
  if (!data.data) return null;
  return (
    <Card className="space-y-4 p-5" data-field-sync>
      <p className="text-sm text-neutral-600">{t("fieldSync.desc")}</p>
      {userFields.length === 0 ? (
        <Notice>{t("fieldSync.noUserFields")}</Notice>
      ) : (
        <>
          {list.length === 0 && <p className="text-sm text-neutral-500">{t("fieldSync.empty")}</p>}
          {list.map((m, i) => (
            <div key={i} className="grid items-end gap-3 sm:grid-cols-[7rem_1fr_1fr_auto_auto]">
              <Field label={t("fieldSync.source")}>
                <select className={inputCls} value={m.source} onChange={(e) => set(i, { source: e.target.value as FieldSyncMapping["source"] })}>
                  <option value="ldap">LDAP</option>
                  <option value="feishu">{t("fieldSync.sourceFeishu")}</option>
                </select>
              </Field>
              <Field label={t("fieldSync.attr")}>
                <input className={cx(inputCls, "font-mono")} value={m.attr} onChange={(e) => set(i, { attr: e.target.value })} />
              </Field>
              <Field label={t("fieldSync.field")}>
                <select className={inputCls} value={m.field} onChange={(e) => set(i, { field: e.target.value })}>
                  <option value="">—</option>
                  {userFields.map((f) => (
                    <option key={f.key} value={f.key}>
                      {f.label}
                    </option>
                  ))}
                </select>
              </Field>
              <label className="mb-2 flex items-center gap-1.5 text-sm text-neutral-700">
                <input type="checkbox" className="size-4 accent-brand-600" checked={m.override} onChange={(e) => set(i, { override: e.target.checked })} />
                {t("fieldSync.override")}
              </label>
              <Button variant="ghost" size="sm" className="mb-1" aria-label={t("common.delete")} onClick={() => setRows(list.filter((_, j) => j !== i))}>
                <Trash2 size={14} />
              </Button>
            </div>
          ))}
          <p className="text-xs text-neutral-500">{t("fieldSync.attrHint")}</p>
          <div className="flex justify-between gap-2">
            <Button variant="secondary" size="sm" onClick={() => setRows([...list, { source: "ldap", attr: "", field: "", override: false }])}>
              <Plus size={14} /> {t("fieldSync.add")}
            </Button>
            <Button
              size="sm"
              disabled={busy || rows === null}
              onClick={async () => {
                const r = await run(() => extApi.setFieldSync(list.map((m) => ({ ...m, attr: m.attr.trim() }))));
                notify(r.ok ? t("fieldSync.saved") : r.error);
                if (r.ok) {
                  setRows(null);
                  data.reload();
                }
              }}
            >
              {t("fieldSync.save")}
            </Button>
          </div>
        </>
      )}
    </Card>
  );
}

// ---- 管理约束 ----

function PolicyTab({ policy, presets, reload }: { policy: AdminPolicy; presets: { name: string; title: string; description: string }[]; reload: () => void }) {
  const { t } = useTranslation();
  const { notify } = useStore();
  const { busy, run } = useAction();
  const [p, setP] = useState<AdminPolicy>(policy);
  const [exclusive, setExclusive] = useState(policy.exclusive.map((g) => g.join(", ")).join("\n"));
  const toggleDual = (k: string, on: boolean) => setP({ ...p, dual_approval: on ? [...p.dual_approval.filter((x) => x !== k), k] : p.dual_approval.filter((x) => x !== k) });
  const save = async () => {
    const body = { ...p, exclusive: lines(exclusive).map((l) => l.split(",").map((x) => x.trim()).filter(Boolean)) };
    const r = await run(() => extApi.setPolicy(body));
    notify(r.ok ? t("policyAdmin.saved") : r.error);
    if (r.ok) reload();
  };
  return (
    <div className="space-y-5" data-policy>
      <Card>
        <CardHeader title={t("policyAdmin.presets")} />
        <ul className="divide-y divide-neutral-100 border-t border-neutral-100">
          {presets.map((pr) => (
            <li key={pr.name} className="flex flex-wrap items-center gap-3 px-5 py-3 text-sm">
              <div className="min-w-0 flex-1">
                <div className="font-medium text-neutral-900">{pr.title}</div>
                <div className="text-neutral-600">{pr.description}</div>
              </div>
              <Button
                size="sm"
                variant="secondary"
                disabled={busy}
                onClick={async () => {
                  const r = await run(() => extApi.applyPreset(pr.name));
                  notify(r.ok ? t("policyAdmin.applied", { title: pr.title }) : r.error);
                  if (r.ok) {
                    setP(r.value.policy);
                    setExclusive(r.value.policy.exclusive.map((g) => g.join(", ")).join("\n"));
                    reload();
                  }
                }}
              >
                {t("policyAdmin.apply")}
              </Button>
            </li>
          ))}
        </ul>
      </Card>
      <Card className="space-y-5 p-5">
        <ToggleRow label={t("policyAdmin.superDisabled")} desc={t("policyAdmin.superDisabledDesc")} checked={p.super_disabled} onChange={(v) => setP({ ...p, super_disabled: v })} />
        <ToggleRow label={t("policyAdmin.noSelfGrant")} desc={t("policyAdmin.noSelfGrantDesc")} checked={p.no_self_grant} onChange={(v) => setP({ ...p, no_self_grant: v })} />
        <Field label={t("policyAdmin.exclusive")} hint={t("policyAdmin.exclusiveDesc")}>
          <textarea className={cx(textareaCls, "font-mono")} rows={3} value={exclusive} onChange={(e) => setExclusive(e.target.value)} placeholder="sys_admin, perm_admin, auditor" />
        </Field>
        <Field label={t("policyAdmin.dual")} hint={t("policyAdmin.dualDesc")}>
          <div className="space-y-1.5">
            {(["roles.assign", "config"] as const).map((k) => (
              <label key={k} className="flex items-center gap-2 text-sm text-neutral-800">
                <input type="checkbox" className="size-4 accent-brand-600" checked={p.dual_approval.includes(k)} onChange={(e) => toggleDual(k, e.target.checked)} />
                {t(k === "config" ? "policyAdmin.dualConfig" : "policyAdmin.dualRoles")}
              </label>
            ))}
          </div>
        </Field>
        <div className="flex justify-end">
          <Button disabled={busy} onClick={() => void save()}>
            {t("policyAdmin.save")}
          </Button>
        </div>
      </Card>
      <p className="text-xs text-neutral-500">{t("policyAdmin.recover")}</p>
    </div>
  );
}

// ---- 变更请求 ----

const changeTone: Record<ChangeRequest["status"], Tone> = { pending: "amber", applied: "green", rejected: "slate", failed: "red" };

function ChangesTab({ onApplied }: { onApplied: () => void }) {
  const { t } = useTranslation();
  const { notify } = useStore();
  const confirm = useConfirm();
  const { busy, run } = useAction();
  const list = useApi(extApi.changes);
  const [open, setOpen] = useState<number | null>(null);
  const decide = async (c: ChangeRequest, approve: boolean) => {
    const r = await run(() => extApi.decideChange(c.id, approve));
    notify(r.ok ? t(approve ? "changesAdmin.approved" : "changesAdmin.rejected") : r.error);
    list.reload();
    if (r.ok && approve) onApplied();
  };
  return (
    <div className="space-y-5">
      <SubmitConfig onSubmitted={list.reload} />
      <Card>
        <LoadState loading={list.loading && !list.data} error={list.error} onRetry={list.reload}>
          {(list.data ?? []).length === 0 ? (
            <p className="p-5 text-sm text-neutral-500">{t("changesAdmin.empty")}</p>
          ) : (
            <ul className="divide-y divide-neutral-100">
              {list.data!.map((c) => (
                <li key={c.id} className="px-5 py-3 text-sm" data-change={c.id}>
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="font-mono text-xs text-neutral-500">#{c.id}</span>
                    <span className="font-medium text-neutral-900">{c.title}</span>
                    <Badge tone={changeTone[c.status]}>{t(`changesAdmin.status.${c.status}`)}</Badge>
                    <Badge>{t(`changesAdmin.kind.${c.kind}`)}</Badge>
                    {c.via_token && <Badge tone="brand">{t("changesAdmin.viaToken")}</Badge>}
                    <span className="ml-auto flex flex-wrap gap-2">
                      <Button variant="ghost" size="sm" onClick={() => setOpen(open === c.id ? null : c.id)}>
                        {t("changesAdmin.view")}
                      </Button>
                      {c.status === "pending" && (
                        <>
                          <Button size="sm" disabled={busy} onClick={() => void decide(c, true)}>
                            <Check size={14} /> {t("changesAdmin.approve")}
                          </Button>
                          <Button size="sm" variant="secondary" disabled={busy} onClick={() => void decide(c, false)}>
                            <X size={14} /> {t("changesAdmin.reject")}
                          </Button>
                        </>
                      )}
                      {c.status === "applied" && c.kind === "config" && (
                        <Button
                          size="sm"
                          variant="secondary"
                          disabled={busy}
                          onClick={async () => {
                            if (!(await confirm({ kind: "restore", description: t("changesAdmin.rollbackConfirm") }))) return;
                            const r = await run(() => extApi.rollbackChange(c.id));
                            notify(r.ok ? t("changesAdmin.rollbackDone", { id: r.value.change_request.id }) : r.error);
                            list.reload();
                          }}
                        >
                          <RotateCcw size={14} /> {t("changesAdmin.rollback")}
                        </Button>
                      )}
                    </span>
                  </div>
                  <div className="mt-1 flex flex-wrap gap-x-4 text-xs text-neutral-500">
                    <span>{t("changesAdmin.by", { name: c.created_by_name ?? c.created_by })}</span>
                    <span>{formatTime(c.created_at)}</span>
                    {c.decided_by_name && <span>{t("changesAdmin.decidedBy", { name: c.decided_by_name })}</span>}
                    {c.note && c.note !== c.title && <span>{c.note}</span>}
                  </div>
                  {c.error && <p className="mt-1 text-xs text-red-700">{c.error}</p>}
                  {open === c.id && <pre className="mt-2 max-h-80 overflow-auto rounded-md bg-neutral-50 p-3 font-mono text-xs whitespace-pre-wrap text-neutral-800">{pretty(c.payload)}</pre>}
                </li>
              ))}
            </ul>
          )}
        </LoadState>
      </Card>
      <p className="text-xs text-neutral-500">{t("changesAdmin.selfNote")}</p>
    </div>
  );
}

function pretty(text: string) {
  try {
    return JSON.stringify(JSON.parse(text), null, 2);
  } catch {
    return text;
  }
}

function SubmitConfig({ onSubmitted }: { onSubmitted: () => void }) {
  const { t } = useTranslation();
  const { notify } = useStore();
  const { busy, run } = useAction();
  const [text, setText] = useState("");
  const [note, setNote] = useState("");
  const [plan, setPlan] = useState<PlanResult | null>(null);
  return (
    <Card className="p-5">
      <h2 className="text-[15px] font-semibold text-neutral-900">{t("changesAdmin.submitTitle")}</h2>
      <p className="mt-0.5 mb-3 text-sm text-neutral-500">{t("changesAdmin.submitHint")}</p>
      <textarea className={cx(textareaCls, "font-mono text-xs")} rows={8} value={text} onChange={(e) => { setText(e.target.value); setPlan(null); }} spellCheck={false} />
      <div className="mt-3 flex flex-wrap items-center gap-2">
        <Button
          variant="secondary"
          size="sm"
          disabled={busy}
          onClick={async () => {
            const r = await run(() => extApi.exportConfig());
            if (r.ok) setText(r.value);
            else notify(r.error);
          }}
        >
          {t("changesAdmin.load")}
        </Button>
        <Button
          variant="secondary"
          size="sm"
          disabled={busy || !text.trim()}
          onClick={async () => {
            const r = await run(() => extApi.planConfig(text));
            if (r.ok) setPlan(r.value);
            else notify(r.error);
          }}
        >
          {t("changesAdmin.plan")}
        </Button>
        <input className={cx(inputCls, "max-w-xs flex-1")} placeholder={t("changesAdmin.note")} value={note} onChange={(e) => setNote(e.target.value)} />
        <Button
          size="sm"
          disabled={busy || !plan?.ok}
          onClick={async () => {
            const r = await run(() => extApi.submitConfig(text, note));
            if (!r.ok) return notify(r.error);
            notify(t("changesAdmin.submitted", { id: r.value.change_request.id }));
            setText("");
            setNote("");
            setPlan(null);
            onSubmitted();
          }}
        >
          {t("changesAdmin.submit")}
        </Button>
      </div>
      {plan && <PlanView plan={plan} />}
    </Card>
  );
}

function PlanView({ plan }: { plan: PlanResult }) {
  const { t } = useTranslation();
  const section = (title: string, children: ReactNode) => (
    <div>
      <p className="mb-1 font-medium text-neutral-900">{title}</p>
      {children}
    </div>
  );
  return (
    <div className="mt-4 space-y-3 text-sm" data-plan>
      {plan.summary && <p className="text-neutral-800">{plan.summary}</p>}
      {plan.problems.length > 0 && (
        <Notice tone="danger">
          <p className="font-medium">{t("changesAdmin.planProblems")}</p>
          <ul className="mt-1 list-disc pl-5">
            {plan.problems.map((p) => (
              <li key={p}>{p}</li>
            ))}
          </ul>
        </Notice>
      )}
      {plan.changes.length > 0 &&
        section(
          t("changesAdmin.planChanges"),
          <ul className="list-disc pl-5 text-neutral-700">
            {plan.changes.map((c) => (
              <li key={c}>{c}</li>
            ))}
          </ul>,
        )}
      {plan.perm_impact.length > 0 &&
        section(
          t("changesAdmin.planPerm"),
          <ul className="list-disc pl-5 text-neutral-700">
            {plan.perm_impact.map((p) => (
              <li key={p.user}>
                {(p.gain ?? []).length > 0 && t("changesAdmin.permGain", { user: p.user, perms: (p.gain ?? []).join("、") })}
                {(p.lose ?? []).length > 0 && ` ${t("changesAdmin.permLose", { user: p.user, perms: (p.lose ?? []).join("、") })}`}
              </li>
            ))}
          </ul>,
        )}
      {plan.impact.length > 0 &&
        section(
          t("changesAdmin.planImpact"),
          <ul className="list-disc pl-5 text-neutral-700">
            {plan.impact.map((im) => (
              <li key={im.user + im.kb}>
                {(im.lose ?? []).length > 0 && t("changesAdmin.lose", { user: im.user, kb: im.kb, count: (im.lose ?? []).length })}
                {(im.gain ?? []).length > 0 && ` ${t("changesAdmin.gain", { user: im.user, kb: im.kb, count: (im.gain ?? []).length })}`}
              </li>
            ))}
          </ul>,
        )}
      {plan.notes.map((n) => (
        <p key={n} className="text-xs text-neutral-500">
          {n}
        </p>
      ))}
    </div>
  );
}

// ---- 为什么看不到 ----

function ExplainTab() {
  const { t } = useTranslation();
  const { busy, run } = useAction();
  const [user, setUser] = useState("");
  const [doc, setDoc] = useState("");
  const [res, setRes] = useState<Awaited<ReturnType<typeof extApi.explain>> | null>(null);
  const [error, setError] = useState<string | null>(null);
  return (
    <Card className="p-5" data-explain>
      <p className="mb-3 text-sm text-neutral-600">{t("explainAdmin.desc")}</p>
      <div className="flex flex-wrap items-end gap-3">
        <Field label={t("explainAdmin.user")}>
          <input className={cx(inputCls, "w-48")} value={user} onChange={(e) => setUser(e.target.value)} />
        </Field>
        <Field label={t("explainAdmin.doc")}>
          <input className={cx(inputCls, "w-32 font-mono")} inputMode="numeric" value={doc} onChange={(e) => setDoc(e.target.value)} />
        </Field>
        <Button
          disabled={busy || !user.trim() || !Number(doc)}
          onClick={async () => {
            setError(null);
            const r = await run(() => extApi.explain(user.trim(), Number(doc)));
            if (r.ok) setRes(r.value);
            else {
              setRes(null);
              setError(r.error);
            }
          }}
        >
          {t("explainAdmin.run")}
        </Button>
      </div>
      {error && (
        <Notice tone="danger" className="mt-4">
          {error}
        </Notice>
      )}
      {res && (
        <div className="mt-4 space-y-2 text-sm">
          <Notice tone={res.allowed ? "success" : "warn"}>{t(res.allowed ? "explainAdmin.allowed" : "explainAdmin.denied", { user: res.user, title: res.title })}</Notice>
          <ol className="list-decimal space-y-1 pl-5 text-neutral-700">
            {res.steps.map((s, i) => (
              <li key={i}>
                <Badge tone={s.result === "deny" ? "red" : s.result === "allow" ? "green" : "slate"}>{s.result}</Badge> <span className="font-medium">{s.stage}</span>：{s.reason}
              </li>
            ))}
          </ol>
        </div>
      )}
    </Card>
  );
}
