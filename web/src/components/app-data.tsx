import { useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { Download, Pencil, Trash2 } from "lucide-react";
import { api, formatTime, type AppField, type AppRow, type AppTable, type AppTableInfo } from "../lib/api";
import { formatNumber } from "../lib/format";
import { useAction, useApi } from "../lib/use-api";
import { useConfirm } from "../lib/confirm";
import { LoadState } from "./api-ui";
import { Badge, Button, Card, CardHeader, cx, Field, inputCls } from "./ui";

const textareaCls = "w-full rounded-md border border-neutral-300 bg-card px-3 py-2 font-mono text-xs outline-none placeholder:text-neutral-500 focus:border-brand-500 focus:ring-2 focus:ring-brand-100";

/** 示例数据表：团建报名，照着改最快。 */
export function sampleTables(): AppTable[] {
  return [
    {
      name: "signups", title: "报名记录", visibility: "own",
      fields: [
        { name: "who", title: "姓名", kind: "user" },
        { name: "session", title: "场次", kind: "select", options: ["周六上午", "周六下午"], required: true },
        { name: "diet", title: "忌口", kind: "text" },
        { name: "people", title: "人数", kind: "number" },
      ],
    },
  ];
}

export function tablesText(tables?: AppTable[]): string {
  return tables?.length ? JSON.stringify(tables, null, 2) : "";
}

/** 解析编辑框里的文字：空表示没有数据表；格式不对返回错误。结构由服务端完整校验。 */
export function parseTables(text: string): { value?: AppTable[]; error?: string } {
  if (!text.trim()) return {};
  try {
    const value = JSON.parse(text);
    if (!Array.isArray(value)) return { error: "array" };
    return { value };
  } catch (e) {
    return { error: e instanceof Error ? e.message : String(e) };
  }
}

/** 编辑器里的「数据表」：JSON 编辑框 + 插入示例 + 字段一览。 */
export function TablesField({ text, onText }: { text: string; onText: (text: string) => void }) {
  const { t } = useTranslation();
  const parsed = parseTables(text);
  return (
    <Field label={t("appData.label")} hint={t("appData.hint")}>
      <div className="mb-2 flex flex-wrap gap-2">
        <Button type="button" variant="secondary" disabled={!!text.trim()} onClick={() => onText(tablesText(sampleTables()))}>
          {t("appData.insertSample")}
        </Button>
      </div>
      <textarea className={cx(textareaCls, "h-56")} value={text} spellCheck={false} placeholder={t("appData.placeholder")} onChange={(e) => onText(e.target.value)} />
      {parsed.error && <p className="mt-1 text-xs text-red-600">{parsed.error === "array" ? t("appData.needArray") : t("appWorkbench.badJson", { error: parsed.error })}</p>}
      {parsed.value?.map((tb) => (
        <div key={tb.name} className="mt-2 text-xs text-neutral-600">
          <code className="font-mono">{`"source": { "table": "${tb.name}" }`}</code>
          <span className="text-neutral-500"> · {tb.title} · {t(`appData.visibility.${tb.visibility ?? "own"}`, { defaultValue: tb.visibility })}</span>
        </div>
      ))}
    </Field>
  );
}

function cell(f: AppField, v: unknown): string {
  if (v === null || v === undefined || v === "") return "—";
  if (f.kind === "bool") return v ? "✓" : "✗";
  if (f.kind === "money" && typeof v === "number") return `¥${formatNumber(v)}`;
  return String(v);
}

function inputFor(f: AppField, value: string, onChange: (v: string) => void) {
  if (f.kind === "select" || f.kind === "bool") {
    const options = f.kind === "bool" ? ["是", "否"] : f.options ?? [];
    return (
      <select className={inputCls} value={value} required={f.required} onChange={(e) => onChange(e.target.value)}>
        <option value="">—</option>
        {options.map((o) => <option key={o} value={o}>{o}</option>)}
      </select>
    );
  }
  const type = f.kind === "number" || f.kind === "money" ? "number" : f.kind === "date" ? "date" : "text";
  return <input className={inputCls} type={type} step="any" value={value} required={f.required} onChange={(e) => onChange(e.target.value)} />;
}

/** 一张表：行列表、新增一行、删除能删的行；作者和管理员能导出 CSV（嵌入模式不显示导出）。 */
function TableData({ slug, table, admin, embed, onChanged }: { slug: string; table: AppTableInfo; admin: boolean; embed: boolean; onChanged: () => void }) {
  const { t } = useTranslation();
  const confirm = useConfirm();
  const [q, setQ] = useState("");
  const rows = useApi(() => api.appRows(slug, table.name, { q, limit: 100 }));
  const [draft, setDraft] = useState<Record<string, string>>({});
  // 正在编辑的行；为空时下面的表单是「新增一行」
  const [editing, setEditing] = useState<AppRow | null>(null);
  const [error, setError] = useState<string | null>(null);
  const { busy, run } = useAction();
  const fields = table.fields.filter((f) => !f.hidden);
  const editable = fields.filter((f) => f.kind !== "user");
  // 表里已经有「填写人」类字段时不再单独显示一列
  const ownerColumn = !fields.some((f) => f.kind === "user");
  const canDelete = (r: AppRow) => admin || table.visibility === "shared" || (table.visibility !== "author" && r.mine);

  const add = async (e: FormEvent) => {
    e.preventDefault();
    const row: Record<string, unknown> = {};
    for (const f of editable) {
      const v = draft[f.name];
      if (editing) {
        // 编辑：清空的字段写成空
        if (v !== undefined) row[f.name] = v === "" ? null : v;
      } else if (v !== undefined && v !== "") row[f.name] = v;
    }
    const res = await run<unknown>(() => (editing ? api.appUpdateRow(slug, table.name, editing.id, row) : api.appInsertRow(slug, table.name, row)));
    if (!res.ok) return setError(res.error);
    setError(null);
    setDraft({});
    setEditing(null);
    rows.reload();
    onChanged();
  };
  const startEdit = (r: AppRow) => {
    const next: Record<string, string> = {};
    for (const f of editable) {
      const v = r[f.name];
      next[f.name] = v === null || v === undefined ? "" : f.kind === "bool" ? (v ? "是" : "否") : String(v);
    }
    setDraft(next);
    setEditing(r);
    setError(null);
  };
  const remove = async (r: AppRow) => {
    if (!(await confirm({ kind: "delete", description: t("appData.deleteConfirm", { id: r.id }) }))) return;
    const res = await run(() => api.appDeleteRow(slug, table.name, r.id));
    if (!res.ok) return setError(res.error);
    rows.reload();
    onChanged();
  };

  return (
    <Card>
      <CardHeader
        title={<span>{table.title} <span className="font-normal text-neutral-500">· {t(`appData.visibility.${table.visibility ?? "own"}`)}</span></span>}
        aside={admin && !embed ? (
          <a href={api.appExportUrl(slug, table.name)} className="inline-flex items-center gap-1 text-sm text-brand-600 dark:text-brand-300 hover:underline">
            <Download size={14} />{t("appData.export")}
          </a>
        ) : undefined}
      />
      <div className="space-y-3 p-4">
        <input className={inputCls} placeholder={t("appData.search")} value={q} onChange={(e) => setQ(e.target.value)} onKeyDown={(e) => { if (e.key === "Enter") rows.reload(); }} />
        <LoadState loading={rows.loading && !rows.data} error={rows.error} onRetry={rows.reload}>
          {rows.data && (rows.data.items.length === 0 ? (
            <p className="py-4 text-center text-sm text-neutral-500">{t("appData.noRows")}</p>
          ) : (
            <div className="overflow-x-auto">
              <table className="tbl">
                <thead>
                  <tr className="border-b border-neutral-200 text-left text-xs text-neutral-500">
                    {fields.map((f) => <th key={f.name} className="px-2 py-1.5 whitespace-nowrap">{f.title}</th>)}
                    {ownerColumn && <th className="px-2 py-1.5 whitespace-nowrap">{t("appData.owner")}</th>}
                    <th className="px-2 py-1.5 whitespace-nowrap">{t("appData.createdAt")}</th>
                    <th />
                  </tr>
                </thead>
                <tbody>
                  {rows.data.items.map((r) => (
                    <tr key={r.id} className="border-b border-neutral-100">
                      {fields.map((f) => (
                        <td key={f.name} className="px-2 py-1.5 whitespace-nowrap">
                          {cell(f, r[f.name])}
                          {f.kind === "user" && r.mine && <Badge tone="brand">{t("appData.mine")}</Badge>}
                        </td>
                      ))}
                      {ownerColumn ? (
                        <td className="px-2 py-1.5 text-neutral-600">{r.owner}{r.mine && <Badge tone="brand">{t("appData.mine")}</Badge>}</td>
                      ) : null}
                      <td className="px-2 py-1.5 whitespace-nowrap text-neutral-500">{formatTime(r.created_at)}</td>
                      <td className="px-2 py-1.5 text-right">
                        {canDelete(r) && (
                          <span className="inline-flex gap-2">
                            {editable.length > 0 && (
                              <button type="button" aria-label={t("appData.edit")} className="text-neutral-400 hover:text-brand-600 dark:hover:text-brand-200" disabled={busy} onClick={() => startEdit(r)}>
                                <Pencil size={14} />
                              </button>
                            )}
                            <button type="button" aria-label={t("appData.delete")} className="text-neutral-400 hover:text-red-600" disabled={busy} onClick={() => void remove(r)}>
                              <Trash2 size={14} />
                            </button>
                          </span>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
              {rows.data.total > rows.data.items.length && <p className="mt-2 text-xs text-neutral-500">{t("appData.more", { shown: rows.data.items.length, total: rows.data.total })}</p>}
            </div>
          ))}
        </LoadState>
        {(table.can_insert || editing) && editable.length > 0 && (
          <form onSubmit={add} className={cx("grid gap-3 rounded-md border border-dashed p-3 sm:grid-cols-2", editing ? "border-brand-300 bg-brand-50/40" : "border-neutral-300")}>
            {editing && <p className="text-sm font-medium text-neutral-800 sm:col-span-2">{t("appData.editing", { id: editing.id })}</p>}
            {editable.map((f) => (
              <label key={f.name} className="block text-sm">
                <span className="mb-1 block text-neutral-700">{f.title}{f.required && " *"}</span>
                {inputFor(f, draft[f.name] ?? "", (v) => setDraft((d) => ({ ...d, [f.name]: v })))}
              </label>
            ))}
            <div className="sm:col-span-2 flex items-center gap-3">
              <Button type="submit" disabled={busy}>{editing ? t("appData.save") : t("appData.add")}</Button>
              {editing && <Button type="button" variant="secondary" onClick={() => { setEditing(null); setDraft({}); setError(null); }}>{t("appData.cancel")}</Button>}
              {error && <span role="alert" className="text-sm text-red-600">{error}</span>}
            </div>
          </form>
        )}
      </div>
    </Card>
  );
}

/** 应用详情里的「数据」：每张表一块。没有数据表的应用不显示。 */
export function AppDataPanel({ slug, embed = false }: { slug: string; embed?: boolean }) {
  const { t } = useTranslation();
  const info = useApi(() => api.appTables(slug));
  if (info.error || (info.data && info.data.tables.length === 0)) return null;
  return (
    <section className="space-y-3">
      <h2 className="text-neutral-900 text-[15px] leading-6 font-semibold">{t("appData.title")}</h2>
      <p className="text-sm text-neutral-600">{t("appData.desc")}</p>
      <LoadState loading={info.loading && !info.data} error={null} onRetry={info.reload}>
        {info.data?.tables.map((tb) => <TableData key={tb.name} slug={slug} table={tb} admin={info.data!.admin} embed={embed} onChanged={info.reload} />)}
      </LoadState>
    </section>
  );
}
