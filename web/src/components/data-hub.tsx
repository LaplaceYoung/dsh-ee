import { useRef, useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { Database, Eye, Pencil, Play, Plus, Trash2, Upload } from "lucide-react";
import { api, formatTime, type DataParam, type DataResult, type DataSet, type DataTable, type Scope } from "../lib/api";
import { useAction, useApi } from "../lib/use-api";
import { useConfirm } from "../lib/confirm";
import { scopeSummary } from "../lib/hub";
import { ErrorNote, LoadState } from "./api-ui";
import { ScopeEditor } from "./scope-editor";
import { useScopeOptions } from "./scope-dialog";
import { Badge, Button, Card, CardHeader, cx, Field, inputCls, Notice } from "./ui";

const textareaCls = "w-full rounded-md border border-neutral-300 bg-card px-3 py-2 font-mono text-xs outline-none focus:border-brand-500 focus:ring-2 focus:ring-brand-100";

/** 查询结果的小表格（最多显示 20 行）。 */
function ResultTable({ result }: { result: DataResult }) {
  const { t } = useTranslation();
  if (result.items.length === 0) return <p className="py-3 text-sm text-neutral-500">{t("dataHub.noRows")}</p>;
  return (
    <div className="overflow-x-auto">
      <table className="tbl tbl-compact">
        <thead>
          <tr className="border-b border-neutral-200 text-left text-neutral-500">
            {result.columns.map((c) => <th key={c} className="px-2 py-1 whitespace-nowrap">{c}</th>)}
          </tr>
        </thead>
        <tbody>
          {result.items.slice(0, 20).map((r, i) => (
            <tr key={i} className="border-b border-neutral-100">
              {result.columns.map((c) => <td key={c} className="px-2 py-1 whitespace-nowrap">{r[c] === null || r[c] === undefined ? "—" : String(r[c])}</td>)}
            </tr>
          ))}
        </tbody>
      </table>
      <p className="mt-1 text-xs text-neutral-500">{t("dataHub.resultCount", { shown: Math.min(20, result.items.length), total: result.total })}{result.truncated ? t("dataHub.truncated") : ""}</p>
    </div>
  );
}

/** 上传表格：新建、替换或追加。 */
function UploadForm({ tables, onDone }: { tables: DataTable[]; onDone: () => void }) {
  const { t } = useTranslation();
  const file = useRef<HTMLInputElement>(null);
  const [mode, setMode] = useState<"create" | "replace" | "append">("create");
  const [name, setName] = useState("");
  const [title, setTitle] = useState("");
  const [msg, setMsg] = useState<{ ok: boolean; text: string } | null>(null);
  const { busy, run } = useAction();
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    const f = file.current?.files?.[0];
    if (!f) return setMsg({ ok: false, text: t("dataHub.pickFile") });
    const res = await run(() => api.dataUpload({ file: f, name: name.trim(), title: title.trim(), mode }));
    if (!res.ok) return setMsg({ ok: false, text: res.error });
    setMsg({ ok: true, text: t("dataHub.uploaded", { name: res.value.table.name, added: res.value.added, rows: res.value.table.row_count }) });
    if (file.current) file.current.value = "";
    onDone();
  };
  return (
    <form onSubmit={submit} className="grid gap-3 rounded-md border border-dashed border-neutral-300 p-4 sm:grid-cols-2">
      <Field label={t("dataHub.file")} hint={t("dataHub.fileHint")}>
        <input ref={file} type="file" accept=".csv,.xlsx" className="block w-full text-sm" />
      </Field>
      <Field label={t("dataHub.mode")}>
        <select className={inputCls} value={mode} onChange={(e) => setMode(e.target.value as typeof mode)}>
          <option value="create">{t("dataHub.modes.create")}</option>
          <option value="replace" disabled={!tables.length}>{t("dataHub.modes.replace")}</option>
          <option value="append" disabled={!tables.length}>{t("dataHub.modes.append")}</option>
        </select>
      </Field>
      <Field label={t("dataHub.tableName")} hint={t("dataHub.tableNameHint")}>
        {mode === "create" ? (
          <input className={inputCls} value={name} onChange={(e) => setName(e.target.value)} placeholder="customers" required />
        ) : (
          <select className={inputCls} value={name} onChange={(e) => setName(e.target.value)} required>
            <option value="">—</option>
            {tables.map((x) => <option key={x.name} value={x.name}>{x.name} · {x.title}</option>)}
          </select>
        )}
      </Field>
      <Field label={t("dataHub.tableTitle")}>
        <input className={inputCls} value={title} onChange={(e) => setTitle(e.target.value)} placeholder={mode === "create" ? "客户主数据" : t("dataHub.keepTitle")} required={mode === "create"} />
      </Field>
      <div className="flex items-center gap-3 sm:col-span-2">
        <Button type="submit" disabled={busy}><Upload size={14} /> {t("dataHub.upload")}</Button>
        {msg && <span role={msg.ok ? "status" : "alert"} className={cx("text-sm", msg.ok ? "text-emerald-700" : "text-red-600")}>{msg.text}</span>}
      </div>
    </form>
  );
}

/** 数据集编辑：SQL、参数（按 SQL 里的 :名字 自动列出）、可用范围、试运行。 */
function DataSetEditor({ initial, autoParams, onDone, onCancel }: { initial?: DataSet; autoParams: Record<string, string>; onDone: () => void; onCancel: () => void }) {
  const { t } = useTranslation();
  const opts = useScopeOptions();
  const [slug, setSlug] = useState(initial?.slug ?? "");
  const [title, setTitle] = useState(initial?.title ?? "");
  const [description, setDescription] = useState(initial?.description ?? "");
  const [sql, setSql] = useState(initial?.sql ?? "SELECT 客户 AS name, 年合同额 AS arr\nFROM customers\nWHERE 负责人 = :me_name\nORDER BY arr DESC");
  const [params, setParams] = useState<DataParam[]>(initial?.params ?? []);
  const [values, setValues] = useState<Record<string, string>>({});
  const [scope, setScope] = useState<Scope>(initial?.scope ?? { all: true, departments: [], roles: [], users: [] });
  const [result, setResult] = useState<DataResult | null>(null);
  const [error, setError] = useState<string | null>(null);
  const { busy, run } = useAction();
  const editable = params.filter((p) => !p.auto);
  const setParam = (name: string, patch: Partial<DataParam>) => setParams((ps) => ps.map((p) => (p.name === name ? { ...p, ...patch } : p)));

  const tryRun = async () => {
    const res = await run(() => api.dataRun(sql, values, params));
    if (!res.ok) {
      setResult(null);
      return setError(res.error);
    }
    setError(null);
    setResult(res.value.result);
    // 参数表跟着 SQL 走：保留已经填的标题和默认值
    setParams(res.value.params.map((p) => ({ ...p, ...(params.find((x) => x.name === p.name && !p.auto) ?? {}), auto: p.auto })));
  };
  const save = async (e: FormEvent) => {
    e.preventDefault();
    const res = await run(() => api.dataSetSave({
      slug: slug.trim(), title: title.trim(), description: description.trim(), sql, params, scope,
      ...(initial ? { updated_at: initial.updated_at } : {}),
    }, !!initial));
    if (!res.ok) return setError(res.error);
    onDone();
  };
  return (
    <form onSubmit={save} className="space-y-4 rounded-md border border-brand-200 bg-brand-50/30 p-4">
      <div className="grid gap-3 sm:grid-cols-2">
        <Field label={t("dataHub.dsSlug")} hint={t("dataHub.dsSlugHint")}>
          <input className={inputCls} value={slug} onChange={(e) => setSlug(e.target.value)} disabled={!!initial} required placeholder="my-customers" />
        </Field>
        <Field label={t("dataHub.dsTitle")}>
          <input className={inputCls} value={title} onChange={(e) => setTitle(e.target.value)} required placeholder="我负责的客户" />
        </Field>
      </div>
      <Field label={t("dataHub.dsDescription")} hint={t("dataHub.dsDescriptionHint")}>
        <input className={inputCls} value={description} onChange={(e) => setDescription(e.target.value)} />
      </Field>
      <Field label="SQL" hint={t("dataHub.sqlHint", { auto: Object.keys(autoParams).map((k) => `:${k}`).join(" ") })}>
        <textarea className={cx(textareaCls, "h-36")} value={sql} spellCheck={false} onChange={(e) => setSql(e.target.value)} />
      </Field>
      {params.length > 0 && (
        <div className="space-y-2">
          <div className="text-sm font-medium text-neutral-800">{t("dataHub.params")}</div>
          {params.filter((p) => p.auto).map((p) => (
            <div key={p.name} className="text-xs text-neutral-600"><code>:{p.name}</code> · {autoParams[p.name] ?? p.title} <Badge>{t("dataHub.auto")}</Badge></div>
          ))}
          {editable.map((p) => (
            <div key={p.name} className="grid gap-2 sm:grid-cols-3">
              <code className="self-center text-xs">:{p.name}</code>
              <input className={inputCls} value={p.title} onChange={(e) => setParam(p.name, { title: e.target.value })} placeholder={t("dataHub.paramTitle")} aria-label={t("dataHub.paramTitle")} />
              <input className={inputCls} value={p.default ?? ""} onChange={(e) => setParam(p.name, { default: e.target.value })} placeholder={t("dataHub.paramDefault")} aria-label={t("dataHub.paramDefault")} />
            </div>
          ))}
        </div>
      )}
      <Field label={t("dataHub.scope")} hint={t("dataHub.scopeHint")}>
        <ScopeEditor value={scope} onChange={setScope} roles={opts.roles} users={opts.users} departments={opts.departments} />
      </Field>
      <div className="flex flex-wrap items-center gap-2">
        <Button type="button" variant="secondary" disabled={busy} onClick={() => void tryRun()}><Play size={14} /> {t("dataHub.run")}</Button>
        {editable.map((p) => (
          <input key={p.name} className={cx(inputCls, "w-40")} placeholder={`:${p.name}`} aria-label={`:${p.name}`} value={values[p.name] ?? ""} onChange={(e) => setValues((v) => ({ ...v, [p.name]: e.target.value }))} />
        ))}
      </div>
      {result && <ResultTable result={result} />}
      {error && <ErrorNote>{error}</ErrorNote>}
      <div className="flex gap-2">
        <Button type="submit" disabled={busy}>{t("dataHub.saveDataset")}</Button>
        <Button type="button" variant="secondary" onClick={onCancel}>{t("dataHub.cancel")}</Button>
      </div>
    </form>
  );
}

/** 「企业连接」页的「数据中台」页签。 */
export function DataHubPanel() {
  const { t } = useTranslation();
  const confirm = useConfirm();
  const opts = useScopeOptions();
  const data = useApi(api.dataOverview);
  const [preview, setPreview] = useState<{ name: string; result: DataResult } | null>(null);
  const [editing, setEditing] = useState<DataSet | "new" | null>(null);
  const [error, setError] = useState<string | null>(null);
  const { run } = useAction();
  const d = data.data;

  const showPreview = async (name: string) => {
    if (preview?.name === name) return setPreview(null);
    const res = await run(() => api.dataTablePreview(name));
    if (!res.ok) return setError(res.error);
    setPreview({ name, result: res.value.preview });
  };
  const dropTable = async (x: DataTable) => {
    if (!(await confirm({ kind: "delete", description: t("dataHub.dropConfirm", { name: x.name }) }))) return;
    const res = await run(() => api.dataTableDelete(x.name));
    if (!res.ok) return setError(res.error);
    data.reload();
  };
  const dropSet = async (x: DataSet) => {
    if (!(await confirm({ kind: "delete", description: t("dataHub.dropSetConfirm", { title: x.title }) }))) return;
    const res = await run(() => api.dataSetDelete(x.slug));
    if (!res.ok) return setError(res.error);
    data.reload();
  };

  return (
    <LoadState loading={data.loading && !d} error={d ? null : data.error} onRetry={data.reload}>
      {d && !d.enabled ? (
        <Card className="p-6 text-sm text-neutral-700">
          <div className="mb-2 flex items-center gap-2 font-medium text-neutral-900"><Database size={16} /> {t("dataHub.offTitle")}</div>
          <p>{d.error}</p>
          <p className="mt-2 text-neutral-600">{t("dataHub.offHint")}</p>
        </Card>
      ) : d ? (
        <div className="space-y-5">
          <p className="text-sm text-neutral-600">{t("dataHub.desc", { db: d.database })}</p>
          {d.read_error && <Notice tone="warn" role="alert">{d.read_error}</Notice>}
          {error && <ErrorNote>{error}</ErrorNote>}

          <Card>
            <CardHeader title={t("dataHub.tables")} />
            <div className="space-y-4 p-4">
              <UploadForm tables={d.tables} onDone={data.reload} />
              {d.tables.length === 0 ? (
                <p className="text-sm text-neutral-500">{t("dataHub.noTables")}</p>
              ) : (
                <ul className="divide-y divide-neutral-100">
                  {d.tables.map((x) => (
                    <li key={x.name} className="py-3">
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="font-mono text-sm text-neutral-900">{x.name}</span>
                        <span className="text-sm text-neutral-700">{x.title}</span>
                        <Badge>{t("dataHub.rows", { count: x.row_count })}</Badge>
                        <span className="text-xs text-neutral-500">{formatTime(x.updated_at)}</span>
                        <span className="ml-auto flex gap-2">
                          <button type="button" aria-label={t("dataHub.preview")} className="text-neutral-500 hover:text-brand-600 dark:hover:text-brand-200" onClick={() => void showPreview(x.name)}><Eye size={15} /></button>
                          <button type="button" aria-label={t("dataHub.drop")} className="text-neutral-500 hover:text-red-600" onClick={() => void dropTable(x)}><Trash2 size={15} /></button>
                        </span>
                      </div>
                      <div className="mt-1 truncate text-xs text-neutral-500">{x.columns.map((c) => `${c.name}（${t(`dataHub.types.${c.type}`)}）`).join(" · ")}</div>
                      {preview?.name === x.name && <div className="mt-2"><ResultTable result={preview.result} /></div>}
                    </li>
                  ))}
                </ul>
              )}
            </div>
          </Card>

          <Card>
            <CardHeader
              title={t("dataHub.datasets")}
              aside={editing === null ? <Button size="sm" onClick={() => setEditing("new")}><Plus size={14} /> {t("dataHub.newDataset")}</Button> : undefined}
            />
            <div className="space-y-4 p-4">
              <p className="text-sm text-neutral-600">{t("dataHub.datasetsDesc")}</p>
              {editing !== null && (
                <DataSetEditor
                  key={editing === "new" ? "new" : editing.slug}
                  initial={editing === "new" ? undefined : editing}
                  autoParams={d.auto_params}
                  onCancel={() => setEditing(null)}
                  onDone={() => { setEditing(null); data.reload(); }}
                />
              )}
              {d.datasets.length === 0 ? (
                <p className="text-sm text-neutral-500">{t("dataHub.noDatasets")}</p>
              ) : (
                <ul className="divide-y divide-neutral-100">
                  {d.datasets.map((x) => (
                    <li key={x.slug} className="py-3">
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="font-mono text-sm text-neutral-900">{x.slug}</span>
                        <span className="text-sm text-neutral-700">{x.title}</span>
                        <span className="text-xs text-neutral-500">{scopeSummary(t, x.scope, opts.roleName)}</span>
                        <span className="ml-auto flex gap-2">
                          <button type="button" aria-label={t("dataHub.edit")} className="text-neutral-500 hover:text-brand-600 dark:hover:text-brand-200" onClick={() => setEditing(x)}><Pencil size={15} /></button>
                          <button type="button" aria-label={t("dataHub.drop")} className="text-neutral-500 hover:text-red-600" onClick={() => void dropSet(x)}><Trash2 size={15} /></button>
                        </span>
                      </div>
                      {x.description && <div className="mt-1 text-xs text-neutral-600">{x.description}</div>}
                      <code className="mt-1 block truncate text-[11px] text-neutral-400">{`"source": { "dataset": "${x.slug}" }`}</code>
                    </li>
                  ))}
                </ul>
              )}
            </div>
          </Card>
        </div>
      ) : null}
    </LoadState>
  );
}
