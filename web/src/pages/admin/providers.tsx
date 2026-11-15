import { useMemo, useState, type FormEvent } from "react";
import { CircleCheck, CircleAlert, CircleDashed, Bot } from "lucide-react";
import { api, formatTime, type CheckResult, type Model, type Provider } from "../../lib/api";
import { useStore } from "../../lib/store";
import { useAction, useApi } from "../../lib/use-api";
import { ErrorNote, LoadState } from "../../components/api-ui";
import zhLlm from "../../i18n/zh/llm-admin";
import { Badge, Button, Card, CardHeader, cx, Field, inputBase, inputCls, PageHeader, PasswordInput, Toggle, Empty } from "../../components/ui";
import { useTranslation } from "react-i18next";
import { useConfirm } from "@/lib/confirm";
import { Tip } from "@/components/tip";

/**
 * 常见厂商的 OpenAI 兼容地址。只是预填，管理员可以改；厂商列表与模型名变化快，
 * 所以不预置模型清单，建好后用「从上游导入」读取上游实际提供的模型。
 */
const VENDORS: { id: string; baseUrl: string; note?: boolean; models?: string[] }[] = [
  { id: "deepseek", baseUrl: "https://api.deepseek.com", models: ["deepseek-flash", "deepseek-v4-pro"] },
  { id: "qwen", baseUrl: "https://dashscope.aliyuncs.com/compatible-mode/v1" },
  { id: "zhipu", baseUrl: "https://open.bigmodel.cn/api/paas/v4" },
  { id: "doubao", baseUrl: "https://ark.cn-beijing.volces.com/api/v3", note: true },
  { id: "moonshot", baseUrl: "https://api.moonshot.cn/v1" },
  { id: "siliconflow", baseUrl: "https://api.siliconflow.cn/v1" },
  { id: "openai", baseUrl: "https://api.openai.com/v1" },
  { id: "ollama", baseUrl: "http://127.0.0.1:11434/v1", note: true },
  { id: "vllm", baseUrl: "http://127.0.0.1:8000/v1" },
  { id: "custom", baseUrl: "" },
];
type VendorId = keyof typeof zhLlm.providers.vendors;
const isVendor = (id: string): id is VendorId => id in zhLlm.providers.vendors;

/** 管理端填写的思考强度是逗号分隔字符串（中英文逗号都行），转成去空的小写数组交给 API。 */
function parseEfforts(raw: string): string[] {
  return raw
    .split(/[,，]/)
    .map((s) => s.trim().toLowerCase())
    .filter(Boolean);
}

/** 输入模态的规范顺序；text 始终包含（服务端也会补齐）。 */
const INPUT_MODALITIES = ["text", "image", "file"] as const;

/** 把一个模态加入/移出数组；text 始终保留，供表单多选使用。 */
function toggleModality(current: string[], modality: string): string[] {
  const next = current.includes(modality) ? current.filter((m) => m !== modality) : [...current, modality];
  return INPUT_MODALITIES.filter((m) => m === "text" || next.includes(m));
}

function ProviderForm({ initial, onDone, onCancel }: { initial?: Provider; onDone: () => void; onCancel: () => void }) {
  const { t } = useTranslation();
  const [vendor, setVendor] = useState(initial?.vendor ?? "deepseek");
  const preset = VENDORS.find((v) => v.id === vendor);
  const [name, setName] = useState(initial?.name ?? "DeepSeek");
  const [baseUrl, setBaseUrl] = useState(initial?.base_url ?? VENDORS[0].baseUrl);
  const [apiKey, setApiKey] = useState("");
  const [priority, setPriority] = useState(String(initial?.priority ?? 100));
  const [models, setModels] = useState<string[]>(initial ? [] : (VENDORS[0].models ?? []));
  const [error, setError] = useState<string | null>(null);
  const { busy, run } = useAction();

  const pickVendor = (id: string) => {
    const v = VENDORS.find((x) => x.id === id)!;
    setVendor(id);
    if (!initial) {
      setBaseUrl(v.baseUrl);
      setName(v.id === "custom" ? "" : t(`providers.vendors.${v.id as VendorId}`).replace(/[（(].*[）)]/, "").trim());
      setModels(v.models ?? []);
    }
  };

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setError(null);
    const pr = Number(priority);
    if (!Number.isInteger(pr)) return setError(t("providers.priorityInt"));
    const r = await run(() =>
      initial
        ? api.updateProvider(initial.id, { vendor, base_url: baseUrl, priority: pr, ...(apiKey ? { api_key: apiKey } : {}) })
        : api.createProvider({ name: name.trim(), vendor, base_url: baseUrl, api_key: apiKey, priority: pr, models }),
    );
    if (!r.ok) return setError(r.error);
    onDone();
  };

  return (
    <form onSubmit={submit} className="space-y-4 px-5 pb-5">
      <Field label={t("providers.vendor")} hint={preset?.note ? t(`providers.vendorNotes.${preset.id as "doubao" | "ollama"}`) : t("providers.vendorHint")}>
        <select className={inputCls} value={vendor} onChange={(e) => pickVendor(e.target.value)}>
          {VENDORS.map((v) => (
            <option key={v.id} value={v.id}>
              {t(`providers.vendors.${v.id as VendorId}`)}
            </option>
          ))}
        </select>
      </Field>
      <div className="grid gap-4 sm:grid-cols-2">
        {!initial && (
          <Field label={t("providers.name")} hint={t("providers.nameHint")}>
            <input className={inputCls} required maxLength={64} value={name} onChange={(e) => setName(e.target.value)} />
          </Field>
        )}
        <Field label="Base URL" hint={t("providers.baseUrlHint")}>
          <input className={cx(inputCls, "font-mono")} required value={baseUrl} onChange={(e) => setBaseUrl(e.target.value)} />
        </Field>
        <Field label={t("providers.apiKey")} hint={initial ? t("providers.apiKeyKeep", { hint: initial.api_key_hint }) : t("providers.apiKeyNew")}>
          <PasswordInput label={t("providers.apiKey")} className={inputCls} autoComplete="off" required={!initial} value={apiKey} onChange={(e) => setApiKey(e.target.value)} placeholder="sk-…" />
        </Field>
        <Field label={t("providers.priority")} hint={t("providers.priorityHint")}>
          <input className={inputCls} inputMode="numeric" value={priority} onChange={(e) => setPriority(e.target.value)} />
        </Field>
      </div>
      {!initial && (preset?.models?.length ?? 0) > 0 && (
        <fieldset>
          <legend className="mb-1.5 text-sm font-medium text-neutral-700">{t("providers.addModels")}</legend>
          <div className="flex flex-wrap gap-x-5 gap-y-2">
            {preset!.models!.map((m) => (
              <label key={m} className="flex items-center gap-2 text-sm text-neutral-800">
                <input
                  type="checkbox"
                  className="h-4 w-4 accent-neutral-900"
                  checked={models.includes(m)}
                  onChange={(e) => setModels((xs) => (e.target.checked ? [...xs, m] : xs.filter((x) => x !== m)))}
                />
                <span className="font-mono">{m}</span>
              </label>
            ))}
          </div>
        </fieldset>
      )}
      <ErrorNote>{error}</ErrorNote>
      <div className="flex gap-2">
        <Button type="submit" disabled={busy}>
          {busy ? t("common.saving") : initial ? t("common.save") : t("providers.addProvider")}
        </Button>
        <Button type="button" variant="ghost" onClick={onCancel}>
          {t("common.cancel")}
        </Button>
      </div>
    </form>
  );
}

function HealthBadge({ p }: { p: Provider }) {
  const { t } = useTranslation();
  if (p.last_check_ok === null)
    return (
      <span className="inline-flex items-center gap-1 text-sm text-neutral-600">
        <CircleDashed size={14} /> {t("providers.unchecked")}
      </span>
    );
  return (
    <Tip label={p.last_check_msg || "—"}>
      <span tabIndex={0} className={cx("inline-flex cursor-help items-center gap-1 text-sm", p.last_check_ok ? "text-emerald-700" : "text-red-700")}>
        {p.last_check_ok ? <CircleCheck size={14} /> : <CircleAlert size={14} />}
        {p.last_check_ok ? t("providers.healthy") : t("providers.unhealthy")}
        <span className="text-neutral-500">· {formatTime(p.last_check_at)}</span>
      </span>
    </Tip>
  );
}

/** 从上游 /models 读到的模型里挑选导入；已添加的不再列出。 */
function ImportPicker({ result, existing, onImport, onClose, busy }: { result: CheckResult; existing: Set<string>; onImport: (ids: string[]) => void; onClose: () => void; busy: boolean }) {
  const { t } = useTranslation();
  const [q, setQ] = useState("");
  const [picked, setPicked] = useState<string[]>([]);
  const candidates = result.models.filter((m) => !existing.has(m) && m.toLowerCase().includes(q.trim().toLowerCase()));
  return (
    <div className="space-y-3 rounded-md bg-neutral-50 p-4">
      <div className="flex flex-wrap items-center gap-3">
        <span className="text-sm text-neutral-800">{t("providers.importSummary", { total: result.models.length, added: result.models.filter((m) => existing.has(m)).length })}</span>
        <input aria-label={t("providers.filterLabel")} className={cx(inputBase, "h-8 w-56")} placeholder={t("providers.filter")} value={q} onChange={(e) => setQ(e.target.value)} />
      </div>
      <div className="grid max-h-64 gap-1 overflow-y-auto sm:grid-cols-2 lg:grid-cols-3">
        {candidates.map((m) => (
          <label key={m} className="flex items-center gap-2 rounded px-1 py-0.5 text-sm hover:bg-neutral-100">
            <input
              type="checkbox"
              className="h-4 w-4 accent-neutral-900"
              checked={picked.includes(m)}
              onChange={(e) => setPicked((xs) => (e.target.checked ? [...xs, m] : xs.filter((x) => x !== m)))}
            />
            <span className="truncate font-mono">{m}</span>
          </label>
        ))}
        {candidates.length === 0 && <span className="text-sm text-neutral-600">{t("providers.nothingToImport")}</span>}
      </div>
      <div className="flex gap-2">
        <Button size="sm" disabled={busy || picked.length === 0} onClick={() => onImport(picked)}>
          {picked.length ? t("providers.importN", { n: picked.length }) : t("providers.importAction")}
        </Button>
        <Button size="sm" variant="ghost" onClick={onClose}>
          {t("common.close")}
        </Button>
      </div>
    </div>
  );
}

/** 输入模态多选：text 固定勾选，image/file 可切换（服务端也总会补齐 text）。 */
function ModalityPicker({ value, onChange }: { value: string[]; onChange: (next: string[]) => void }) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
      {INPUT_MODALITIES.map((modality) => (
        <label key={modality} className="flex items-center gap-1.5 text-xs text-neutral-800">
          <input
            type="checkbox"
            className="size-3.5 accent-brand-600"
            checked={modality === "text" || value.includes(modality)}
            disabled={modality === "text"}
            onChange={() => onChange(toggleModality(value, modality))}
          />
          {t(`providers.modalities.${modality}`)}
        </label>
      ))}
    </div>
  );
}

/** 只读的输入模态徽标；text 用中性色，image/file 用品牌/琥珀色。 */
function ModalityBadges({ value }: { value: string[] }) {
  const { t } = useTranslation();
  const shown = INPUT_MODALITIES.filter((m) => value.includes(m));
  return (
    <span className="inline-flex flex-wrap items-center gap-1">
      {shown.map((modality) => (
        <Badge key={modality} tone={modality === "image" ? "brand" : modality === "file" ? "amber" : "slate"}>
          {t(`providers.modalities.${modality}`)}
        </Badge>
      ))}
    </span>
  );
}

function ModelRow({ m, onChanged }: { m: Model; onChanged: () => void }) {
  const { t } = useTranslation();
  const confirm = useConfirm();
  const { notify } = useStore();
  const { busy, run } = useAction();
  const [editing, setEditing] = useState(false);
  const [name, setName] = useState(m.name);
  const [upstream, setUpstream] = useState(m.upstream_model);
  const [display, setDisplay] = useState(m.display_name);
  const [reasoning, setReasoning] = useState(m.supports_reasoning);
  const [efforts, setEfforts] = useState(m.reasoning_efforts.join(","));
  const [modalities, setModalities] = useState<string[]>(m.input_modalities);

  const save = async () => {
    const r = await run(() =>
      api.updateModel(m.id, {
        name,
        upstream_model: upstream,
        display_name: display,
        supports_reasoning: reasoning,
        reasoning_efforts: parseEfforts(efforts),
        input_modalities: modalities,
      }),
    );
    notify(r.ok ? t("providers.modelSaved", { name }) : r.error);
    if (r.ok) {
      setEditing(false);
      onChanged();
    }
  };
  const toggle = async (enabled: boolean) => {
    const r = await run(() => api.updateModel(m.id, { enabled }));
    if (!r.ok) notify(r.error);
    onChanged();
  };
  const remove = async () => {
    if (!(await confirm({ kind: "delete", description: t("providers.modelDeleteConfirm", { name: m.name }) }))) return;
    const r = await run(() => api.deleteModel(m.id));
    notify(r.ok ? t("providers.modelDeleted", { name: m.name }) : r.error);
    onChanged();
  };

  if (editing) {
    return (
      <tr className="border-t border-neutral-100 align-top">
        <td className="px-5 py-2">
          <input aria-label={t("providers.publicName")} className={cx(inputBase, "h-8 w-full font-mono")} value={name} onChange={(e) => setName(e.target.value)} />
        </td>
        <td className="px-5 py-2">
          <input aria-label={t("providers.upstreamName")} className={cx(inputBase, "h-8 w-full font-mono")} value={upstream} onChange={(e) => setUpstream(e.target.value)} />
        </td>
        <td className="px-5 py-2">
          <input aria-label={t("providers.displayName")} className={cx(inputBase, "h-8 w-full")} value={display} onChange={(e) => setDisplay(e.target.value)} />
        </td>
        <td className="px-5 py-2">
          <label className="flex items-center gap-2 text-sm text-neutral-800">
            <input type="checkbox" className="size-4 accent-brand-600" checked={reasoning} onChange={(e) => setReasoning(e.target.checked)} />
            {t("providers.reasoning")}
          </label>
          <input
            aria-label={t("providers.reasoningEfforts")}
            className={cx(inputBase, "mt-1 h-8 w-full font-mono")}
            placeholder={t("providers.reasoningEffortsPlaceholder")}
            value={efforts}
            onChange={(e) => setEfforts(e.target.value)}
          />
        </td>
        <td className="px-5 py-2">
          <ModalityPicker value={modalities} onChange={setModalities} />
        </td>
        <td className="px-5 py-2" colSpan={2}>
          <div className="flex justify-end gap-2">
            <Button size="sm" disabled={busy} onClick={save}>
              {t("common.save")}
            </Button>
            <Button size="sm" variant="ghost" onClick={() => setEditing(false)}>
              {t("common.cancel")}
            </Button>
          </div>
        </td>
      </tr>
    );
  }
  return (
    <tr className={cx("border-t border-neutral-100", !m.enabled && "text-neutral-500")}>
      <td className="px-5 py-2.5 font-mono">{m.name}</td>
      <td className="px-5 py-2.5 font-mono text-neutral-600">{m.upstream_model === m.name ? t("providers.sameName") : m.upstream_model}</td>
      <td className="px-5 py-2.5">{m.display_name || "—"}</td>
      <td className="px-5 py-2.5">
        {m.supports_reasoning ? (
          <span className="inline-flex flex-wrap items-center gap-1">
            <Badge tone="green">{t("providers.reasoningOn")}</Badge>
            {m.reasoning_efforts.length > 0 && <span className="font-mono text-xs text-neutral-600">{m.reasoning_efforts.join("/")}</span>}
          </span>
        ) : (
          <span className="text-neutral-400">—</span>
        )}
      </td>
      <td className="px-5 py-2.5">
        <ModalityBadges value={m.input_modalities} />
      </td>
      <td className="px-5 py-2.5">
        <Toggle checked={m.enabled} disabled={busy} onChange={toggle} />
      </td>
      <td className="px-5 py-2.5">
        <div className="flex justify-end gap-3 text-sm">
          <button type="button" onClick={() => setEditing(true)} className="text-neutral-700 hover:text-neutral-900">
            {t("common.edit")}
          </button>
          <button type="button" disabled={busy} onClick={remove} className="text-neutral-600 hover:text-red-700 disabled:opacity-50">
            {t("common.delete")}
          </button>
        </div>
      </td>
    </tr>
  );
}

function ProviderCard({ p, models, onChanged }: { p: Provider; models: Model[]; onChanged: () => void }) {
  const { t } = useTranslation();
  const confirm = useConfirm();
  const { notify } = useStore();
  const { busy, run } = useAction();
  const [editing, setEditing] = useState(false);
  const [check, setCheck] = useState<CheckResult | null>(null);
  const [importing, setImporting] = useState(false);
  const [newModel, setNewModel] = useState("");
  const [newAlias, setNewAlias] = useState("");
  const [newReasoning, setNewReasoning] = useState(false);
  const [newEfforts, setNewEfforts] = useState("");
  const [newModalities, setNewModalities] = useState<string[]>(["text"]);

  const doCheck = async (forImport: boolean) => {
    const r = await run(() => api.checkProvider(p.id));
    if (!r.ok) return notify(r.error);
    setCheck(r.value);
    onChanged();
    if (forImport) {
      if (r.value.ok && r.value.models.length) setImporting(true);
      else notify(r.value.ok ? t("providers.noModelList") : r.value.message);
    } else notify(r.value.message);
  };
  const importModels = async (ids: string[]) => {
    const r = await run(() => api.createModels(p.id, ids.map((upstream_model) => ({ upstream_model }))));
    if (!r.ok) return notify(r.error);
    notify(r.value.skipped.length ? t("providers.importedSkipped", { created: r.value.created.length, skipped: r.value.skipped.length }) : t("providers.imported", { created: r.value.created.length }));
    setImporting(false);
    onChanged();
  };
  const addOne = async (e: FormEvent) => {
    e.preventDefault();
    const r = await run(() =>
      api.createModels(p.id, [
        {
          upstream_model: newModel.trim(),
          name: newAlias.trim() || undefined,
          supports_reasoning: newReasoning,
          reasoning_efforts: parseEfforts(newEfforts),
          input_modalities: newModalities,
        },
      ]),
    );
    if (!r.ok) return notify(r.error);
    if (r.value.skipped.length) notify(t("providers.duplicateModel", { name: r.value.skipped[0] }));
    setNewModel("");
    setNewAlias("");
    setNewReasoning(false);
    setNewEfforts("");
    setNewModalities(["text"]);
    onChanged();
  };
  const toggle = async (enabled: boolean) => {
    const r = await run(() => api.updateProvider(p.id, { enabled }));
    notify(r.ok ? t(enabled ? "providers.providerEnabled" : "providers.providerDisabled", { name: p.name }) : r.error);
    onChanged();
  };
  const remove = async () => {
    if (!(await confirm({ kind: "delete", description: t("providers.deleteConfirm", { name: p.name, count: models.length }) }))) return;
    const r = await run(() => api.deleteProvider(p.id));
    notify(r.ok ? t("providers.deleted", { name: p.name }) : r.error);
    onChanged();
  };

  return (
    <Card>
      <div className="flex flex-wrap items-start gap-4 px-5 pt-4 pb-4">
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <h2 className="text-neutral-900 text-[15px] leading-6 font-semibold">{p.name}</h2>
            <Badge>{isVendor(p.vendor) ? t(`providers.vendors.${p.vendor}`) : p.vendor}</Badge>
            <span className="text-sm text-neutral-600">{t("providers.priorityOf", { n: p.priority })}</span>
            {!p.enabled && <Badge tone="amber">{t("providers.disabled")}</Badge>}
          </div>
          <div className="mt-1 font-mono text-sm break-all text-neutral-700">{p.base_url}</div>
          <div className="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-sm text-neutral-600">
            <span>{t("providers.companyKey", { hint: p.api_key_hint })}</span>
            <HealthBadge p={p} />
          </div>
          {check && <p className={cx("mt-1 text-sm", check.ok ? "text-neutral-700" : "text-red-700")}>{check.message}{check.ok && ` · ${check.latency_ms} ms`}</p>}
        </div>
        <div className="flex flex-wrap items-center gap-3">
          <label className="flex items-center gap-2 text-sm text-neutral-700">
            {t("providers.enable")}
            <Toggle checked={p.enabled} disabled={busy} onChange={toggle} />
          </label>
          <Button variant="secondary" size="sm" disabled={busy} onClick={() => doCheck(false)}>
            {t("providers.check")}
          </Button>
          <Button variant="secondary" size="sm" onClick={() => setEditing((v) => !v)}>
            {editing ? t("common.close") : t("common.edit")}
          </Button>
          <button type="button" disabled={busy} onClick={remove} className="text-sm text-neutral-600 hover:text-red-700 disabled:opacity-50">
            {t("common.delete")}
          </button>
        </div>
      </div>
      {editing && (
        <div className="border-t border-neutral-100 pt-4">
          <ProviderForm
            initial={p}
            onCancel={() => setEditing(false)}
            onDone={() => {
              notify(t("providers.updated", { name: p.name }));
              setEditing(false);
              onChanged();
            }}
          />
        </div>
      )}
      <div className="border-t border-neutral-100">
        <div className="flex flex-wrap items-center justify-between gap-3 px-5 pt-3 pb-2">
          <h3 className="text-sm font-semibold text-neutral-900">{t("providers.modelsCount", { count: models.length })}</h3>
          <Button variant="secondary" size="sm" disabled={busy} onClick={() => doCheck(true)}>
            {t("providers.importFromUpstream")}
          </Button>
        </div>
        {importing && check && (
          <div className="px-5 pb-3">
            <ImportPicker result={check} existing={new Set(models.map((m) => m.upstream_model))} busy={busy} onImport={importModels} onClose={() => setImporting(false)} />
          </div>
        )}
        {models.length > 0 && (
          <div className="overflow-x-auto">
            <table className="tbl max-sm:min-w-[36rem]">
              <thead>
                <tr>
                  <th className="px-5 py-2">{t("providers.colPublic")}</th>
                  <th className="px-5 py-2">{t("providers.colUpstream")}</th>
                  <th className="px-5 py-2">{t("providers.colDisplay")}</th>
                  <th className="px-5 py-2">{t("providers.colReasoning")}</th>
                  <th className="px-5 py-2">{t("providers.colInput")}</th>
                  <th className="px-5 py-2">{t("providers.colEnabled")}</th>
                  <th className="px-5 py-2" />
                </tr>
              </thead>
              <tbody>
                {models.map((m) => (
                  <ModelRow key={m.id} m={m} onChanged={onChanged} />
                ))}
              </tbody>
            </table>
          </div>
        )}
        <form onSubmit={addOne} className="flex flex-wrap items-center gap-2 border-t border-neutral-100 px-5 py-3">
          <input aria-label={t("providers.upstreamName")} className={cx(inputBase, "h-8 w-56 font-mono")} placeholder={t("providers.upstreamName")} value={newModel} onChange={(e) => setNewModel(e.target.value)} />
          <input aria-label={t("providers.publicName")} className={cx(inputBase, "h-8 w-56 font-mono")} placeholder={t("providers.publicOptional")} value={newAlias} onChange={(e) => setNewAlias(e.target.value)} />
          <label className="flex items-center gap-2 text-sm text-neutral-800">
            <input type="checkbox" className="size-4 accent-brand-600" checked={newReasoning} onChange={(e) => setNewReasoning(e.target.checked)} />
            {t("providers.reasoning")}
          </label>
          <input
            aria-label={t("providers.reasoningEfforts")}
            title={t("providers.reasoningEffortsHint")}
            className={cx(inputBase, "h-8 w-44 font-mono")}
            placeholder={t("providers.reasoningEffortsPlaceholder")}
            value={newEfforts}
            onChange={(e) => setNewEfforts(e.target.value)}
          />
          <ModalityPicker value={newModalities} onChange={setNewModalities} />
          <span className="text-xs text-neutral-500">{t("providers.inputModalitiesHint")}</span>
          <Button size="sm" variant="secondary" type="submit" disabled={busy || !newModel.trim()}>
            {t("providers.addModel")}
          </Button>
        </form>
      </div>
    </Card>
  );
}

/** 对外模型一览：每个名称按优先级列出线路，员工调用时依次尝试。 */
function Routing({ providers, models }: { providers: Provider[]; models: Model[] }) {
  const { t } = useTranslation();
  const rows = useMemo(() => {
    const byName = new Map<string, { m: Model; p: Provider }[]>();
    for (const m of models) {
      const p = providers.find((x) => x.id === m.provider_id);
      if (!p || !m.enabled || !p.enabled) continue;
      byName.set(m.name, [...(byName.get(m.name) ?? []), { m, p }]);
    }
    return [...byName.entries()]
      .map(([name, lines]) => ({ name, lines: lines.sort((a, b) => a.p.priority - b.p.priority || a.p.id - b.p.id) }))
      .sort((a, b) => a.name.localeCompare(b.name));
  }, [providers, models]);
  if (rows.length === 0) return null;
  return (
    <Card className="mb-4">
      <CardHeader title={t("providers.available")} aside={t("providers.availableCount", { count: rows.length })} />
      <div className="overflow-x-auto">
        <table className="tbl max-sm:min-w-[36rem]">
          <thead>
            <tr>
              <th className="px-5 py-2">{t("providers.colModel")}</th>
              <th className="px-5 py-2">{t("providers.colRoutes")}</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((r) => (
              <tr key={r.name} className="border-t border-neutral-100">
                <td className="px-5 py-2.5">
                  <span className="font-mono text-neutral-900">{r.name}</span>
                  {r.lines[0].m.display_name && <span className="ml-2 text-neutral-600">{r.lines[0].m.display_name}</span>}
                </td>
                <td className="px-5 py-2.5 text-neutral-700">
                  {r.lines.map((l, i) => (
                    <span key={l.m.id}>
                      {i > 0 && <span className="mx-1.5 text-neutral-400">→</span>}
                      {l.p.name}
                      {l.m.upstream_model !== r.name && <span className="font-mono text-neutral-500">（{l.m.upstream_model}）</span>}
                    </span>
                  ))}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </Card>
  );
}

export default function AdminProviders() {
  const { t } = useTranslation();
  const providers = useApi(api.providers);
  const models = useApi(api.models);
  const [creating, setCreating] = useState(false);
  const reload = () => {
    providers.reload();
    models.reload();
  };

  return (
    <div>
      <PageHeader
        title={t("providers.title")}
        desc={t("providers.desc")}
        actions={
          <Button size="sm" onClick={() => setCreating((v) => !v)}>
            {creating ? t("common.close") : t("providers.addProvider")}
          </Button>
        }
      />

      {creating && (
        <Card className="mb-4">
          <CardHeader title={t("providers.addProvider")} />
          <ProviderForm
            onCancel={() => setCreating(false)}
            onDone={() => {
              setCreating(false);
              reload();
            }}
          />
        </Card>
      )}

      <LoadState loading={(providers.loading && !providers.data) || (models.loading && !models.data)} error={providers.error ?? models.error} onRetry={reload}>
        <Routing providers={providers.data ?? []} models={models.data ?? []} />
        {providers.data?.length === 0 && !creating && (
          <Card>
            <Empty icon={Bot}>{t("providers.empty")}</Empty>
          </Card>
        )}
        <div className="space-y-4">
          {providers.data?.map((p) => (
            <ProviderCard key={p.id} p={p} models={(models.data ?? []).filter((m) => m.provider_id === p.id)} onChanged={reload} />
          ))}
        </div>
      </LoadState>
    </div>
  );
}
