import { useEffect, useState } from "react";
import { KeyRound, Plus, RefreshCw } from "lucide-react";
import { useTranslation } from "react-i18next";
import { api, type AssetSpec } from "../lib/api";
import { useStore } from "../lib/store";
import { useAction, useApi } from "../lib/use-api";
import { ErrorNote } from "./api-ui";
import { Button, Field, inputCls, Spinner, Badge, formatDate } from "./ui";

/**
 * 连接当前使用的命名共享凭据及管理入口。这里只写密文，列表永远不显示完整值。
 * 每个应用绑定哪份共享凭据属于 follow-up，当前选择在这个连接的版本中。
 */
export function NamedSharedCredentials({ assetId, spec, set }: { assetId?: number; spec: AssetSpec; set: (p: Partial<AssetSpec>) => void }) {
  const { t } = useTranslation();
  const { notify } = useStore();
  const auth = spec.auth ?? { type: "none" };
  const query = useApi(() => (auth.type === "none" || !assetId ? Promise.resolve(null) : api.sharedCredentials(assetId)));
  const { busy, run } = useAction();
  useEffect(() => {
    // 编辑器先从空 spec 变成已保存的连接，认证类型也会改变；此时必须重新取列表。
    if (assetId && auth.type !== "none") query.reload();
  }, [assetId, query.reload, auth.type]);
  const [name, setName] = useState("");
  const [secret, setSecret] = useState("");
  const [adding, setAdding] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const selected = auth.shared_name ?? "";
  const items = query.data?.credentials ?? [];

  if (auth.type === "none") return null;
  if (!assetId) {
    return <div className="border-t border-neutral-200 pt-5 text-sm text-neutral-600" data-testid="named-credentials-first-save">{t("sharedCredentials.createFirst")}</div>;
  }

  const choose = (value: string) => set({ auth: { ...auth, shared_name: value || undefined } });
  const save = async () => {
    setError(null);
    const target = name.trim();
    const result = await run(() => api.saveSharedCredential(assetId, target, secret));
    if (!result.ok) return setError(result.error);
    setName("");
    setSecret("");
    setAdding(false);
    query.reload();
    notify(t("sharedCredentials.saved", { name: target }));
  };
  const remove = async (target: string) => {
    if (!window.confirm(t("sharedCredentials.deleteConfirm", { name: target }))) return;
    setError(null);
    const result = await run(() => api.deleteSharedCredential(assetId, target));
    if (!result.ok) return setError(result.error);
    if (selected === target) choose("");
    query.reload();
    notify(t("sharedCredentials.deleted", { name: target }));
  };

  return (
    <section className="space-y-4 border-t border-neutral-200 pt-5" data-testid="named-shared-credentials">
      <div className="space-y-1">
        <h3 className="flex items-center gap-2 text-sm font-semibold text-neutral-900"><KeyRound size={16} /> {t("sharedCredentials.title")}</h3>
        <p className="text-sm text-neutral-500">{t("sharedCredentials.desc")}</p>
      </div>
      <Field label={t("sharedCredentials.choose")}>
        <select aria-label={t("sharedCredentials.choose")} className={inputCls} value={selected} onChange={(e) => choose(e.target.value)} data-testid="shared-credential-select">
          <option value="">{t("sharedCredentials.legacy")}{query.data?.legacy_hint ? ` (${query.data.legacy_hint})` : ` ${t("sharedCredentials.noLegacy")}`}</option>
          {items.map((c) => <option key={c.name} value={c.name}>{c.name} · {c.secret_hint}</option>)}
        </select>
        {selected && selected !== query.data?.default_name && <p className="mt-1 text-xs text-amber-700">{t("sharedCredentials.unsavedName", { name: selected })}</p>}
      </Field>
      <div className="flex items-center justify-between gap-3">
        <h4 className="text-sm font-medium text-neutral-900">{t("sharedCredentials.list")}</h4>
        <Button type="button" size="sm" variant="secondary" disabled={adding || busy} onClick={() => { setAdding(true); setName(""); setSecret(""); setError(null); }}><Plus size={14} /> {t("sharedCredentials.add")}</Button>
      </div>
      {query.loading && !query.data ? <div className="flex justify-center py-4"><Spinner /></div> : query.error ? <ErrorNote>{query.error}</ErrorNote> : (
        <div className="space-y-2" data-testid="shared-credential-list">
          {!items.length && <p className="text-sm text-neutral-500">{t("sharedCredentials.empty")}</p>}
          {items.map((c) => (
            <div key={c.name} className="flex flex-wrap items-center justify-between gap-2 rounded-md bg-neutral-50 px-3 py-2" data-testid={`shared-credential-${c.name}`}>
              <div className="flex flex-wrap items-center gap-2 text-sm"><Badge tone={selected === c.name ? "brand" : "slate"}>{c.name}</Badge><span>{c.secret_hint}</span>{c.updated_at && <span className="text-xs text-neutral-500">{t("sharedCredentials.updatedAt", { time: formatDate(Date.parse(c.updated_at)) })}</span>}</div>
              <div className="flex gap-1">
                <Button type="button" size="sm" variant="ghost" disabled={busy} aria-label={t("sharedCredentials.replace") + ` ${c.name}`} onClick={() => { setAdding(true); setName(c.name); setSecret(""); setError(null); }}><RefreshCw size={13} /> {t("sharedCredentials.replace")}</Button>
                <Button type="button" size="sm" variant="ghost" disabled={busy} aria-label={t("sharedCredentials.delete") + ` ${c.name}`} onClick={() => remove(c.name)}>{t("sharedCredentials.delete")}</Button>
              </div>
            </div>
          ))}
        </div>
      )}
      {adding && (
        <div className="space-y-3 rounded-md border border-neutral-200 p-3" data-testid="shared-credential-form">
          <div className="grid gap-4 sm:grid-cols-2">
            <Field label={t("sharedCredentials.name")} hint={t("sharedCredentials.nameHint")}>
              <input aria-label={t("sharedCredentials.name")} className={inputCls} disabled={busy} maxLength={40} value={name} onChange={(e) => setName(e.target.value)} placeholder={t("sharedCredentials.namePlaceholder")} />
            </Field>
            <Field label={t("sharedCredentials.secret")}>
              <input aria-label={t("sharedCredentials.secret")} className={inputCls} type="password" disabled={busy} autoComplete="new-password" value={secret} onChange={(e) => setSecret(e.target.value)} placeholder={t("sharedCredentials.secretPlaceholder")} />
            </Field>
          </div>
          <div className="flex gap-2"><Button type="button" disabled={busy || !name.trim() || !secret.trim()} onClick={save}><Spinner className={busy ? "" : "hidden"} size={14} /> {t("sharedCredentials.save")}</Button><Button type="button" variant="secondary" disabled={busy} onClick={() => { setAdding(false); setError(null); }}>{t("sharedCredentials.cancel")}</Button></div>
        </div>
      )}
      {error && <ErrorNote>{error}</ErrorNote>}
    </section>
  );
}
