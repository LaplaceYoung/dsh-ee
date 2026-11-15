import { useMemo, useState } from "react";
import { Link } from "react-router";
import { useTranslation } from "react-i18next";
import { Ban, FolderTree, Plus, RefreshCw, Search, Sparkles, Star, Trash2, X } from "lucide-react";
import { api, isExpert, type AssetBlock, type Category, type CategoryGroup, type EffectiveItem, type HubAsset, type Scope } from "../../lib/api";
import { useAction, useApi } from "../../lib/use-api";
import { useConfirm } from "../../lib/confirm";
import { useStore } from "../../lib/store";
import { allScope, assetPath, emptyScope, scopeSummary } from "../../lib/hub";
import { ErrorNote, LoadState } from "../../components/api-ui";
import { ScopeDialog, useScopeOptions } from "../../components/scope-dialog";
import { ScopeEditor } from "../../components/scope-editor";
import { Combobox } from "../../components/pickers";
import { Badge, Button, Card, CardHeader, cx, Empty, Field, inputBase, inputCls, PageHeader, Segmented, tabCls, tabListCls, type Tone } from "../../components/ui";

/**
 * 专家 · 技能 · 连接器的管理策略（docs/expert-center.md §6）：
 * 受管分类、精选与归类、禁用策略、按人查权限。后端接口见 server/internal/api/expert_center.go。
 */
type Tab = "categories" | "catalog" | "blocks" | "effective";
const TABS: Tab[] = ["categories", "catalog", "blocks", "effective"];
const GROUPS: CategoryGroup[] = ["app", "skill", "connection"];

/** 资产属于哪个分类分组（和 store.CategoryGroupOf 一致）。 */
function groupOf(a: Pick<HubAsset, "kind">): CategoryGroup {
  return a.kind === "app" ? "app" : a.kind === "connection" ? "connection" : "skill";
}

/** 展示用的类型：专家单列出来。 */
function typeKey(a: Pick<HubAsset, "kind" | "spec">): "expert" | HubAsset["kind"] {
  return isExpert(a) ? "expert" : a.kind;
}

export default function AdminExpertCenter() {
  const { t } = useTranslation();
  const [tab, setTab] = useState<Tab>("categories");
  const cats = useApi(api.categories);
  const hub = useApi(api.hub);
  const reload = () => {
    void cats.reload();
    void hub.reload();
  };
  return (
    <div data-testid="expert-center-page">
      <PageHeader
        title={t("expertCenter.admin.title")}
        desc={t("expertCenter.admin.desc")}
        actions={
          <Button variant="secondary" size="sm" disabled={cats.loading || hub.loading} onClick={reload}>
            <RefreshCw size={14} /> {t("expertCenter.admin.reload")}
          </Button>
        }
      />
      <div role="tablist" className={cx(tabListCls, "mb-4")}>
        {TABS.map((k) => (
          <button key={k} role="tab" type="button" aria-selected={tab === k} onClick={() => setTab(k)} className={tabCls(tab === k)}>
            {t(`expertCenter.admin.tabs.${k}`)}
          </button>
        ))}
      </div>
      <p className="mb-2 text-sm text-neutral-600">{t(`expertCenter.admin.tabHints.${tab}`)}</p>
      <p className="mb-4 flex items-center gap-1.5 text-xs text-neutral-500">
        <Sparkles size={13} className="text-brand-500" /> {t("expertCenter.admin.agentHint")}
      </p>
      <LoadState loading={(cats.loading && !cats.data) || (hub.loading && !hub.data)} error={cats.data && hub.data ? null : cats.error || hub.error} onRetry={reload}>
        {cats.data && hub.data && (
          <>
            {tab === "categories" && <Categories cats={cats.data} onChanged={reload} />}
            {tab === "catalog" && <CatalogTable assets={hub.data.assets.filter((a) => a.can_manage)} cats={cats.data} onChanged={reload} />}
            {tab === "blocks" && <Blocks assets={hub.data.assets.filter((a) => a.can_manage)} />}
            {tab === "effective" && <Effective />}
          </>
        )}
      </LoadState>
    </div>
  );
}

// ---- 分类 ----

function Categories({ cats, onChanged }: { cats: Category[]; onChanged: () => void }) {
  const { t } = useTranslation();
  const { notify } = useStore();
  const confirm = useConfirm();
  const opts = useScopeOptions();
  const { busy, run } = useAction();
  const [group, setGroup] = useState<CategoryGroup>("app");
  const [name, setName] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [scopeOf, setScopeOf] = useState<Category | null>(null);

  const create = async () => {
    setError(null);
    const r = await run(() => api.createCategory({ group, name: name.trim(), scope: allScope() }));
    if (!r.ok) return setError(r.error);
    setName("");
    notify(t("expertCenter.admin.categoryCreated", { name: r.value.name }));
    onChanged();
  };
  const rename = async (c: Category, next: string) => {
    if (!next.trim() || next.trim() === c.name) return;
    const r = await run(() => api.patchCategory(c.id, { name: next.trim() }));
    notify(r.ok ? t("expertCenter.admin.saved") : r.error);
    if (r.ok) onChanged();
  };
  const move = async (c: Category, sort: number) => {
    const r = await run(() => api.patchCategory(c.id, { sort }));
    if (!r.ok) notify(r.error);
    else onChanged();
  };
  const remove = async (c: Category) => {
    if (!(await confirm({ kind: "delete", description: t("expertCenter.admin.categoryDeleteConfirm", { name: c.name, count: c.assets }) }))) return;
    const r = await run(() => api.deleteCategory(c.id));
    notify(r.ok ? t("expertCenter.admin.categoryDeleted", { name: c.name }) : r.error);
    if (r.ok) onChanged();
  };

  return (
    <div className="space-y-4">
      <Card className="p-5">
        <div className="flex flex-wrap items-end gap-3">
          <Field label={t("expertCenter.admin.group")}>
            <Segmented value={group} onChange={setGroup} options={GROUPS.map((g) => ({ value: g, label: t(`expertCenter.groups.${g}`) }))} />
          </Field>
          <Field label={t("expertCenter.admin.categoryName")}>
            <input className={cx(inputCls, "w-56")} value={name} maxLength={32} onChange={(e) => setName(e.target.value)} placeholder={t("expertCenter.admin.categoryPlaceholder")} />
          </Field>
          <Button disabled={busy || !name.trim()} onClick={create}>
            <Plus size={14} /> {t("expertCenter.admin.addCategory")}
          </Button>
        </div>
        <ErrorNote>{error}</ErrorNote>
      </Card>
      {GROUPS.map((g) => {
        const list = cats.filter((c) => c.group === g);
        return (
          <Card key={g}>
            <CardHeader title={t(`expertCenter.groups.${g}`)} aside={<span className="text-xs text-neutral-500">{t("expertCenter.admin.count", { count: list.length })}</span>} />
            {list.length === 0 ? (
              <Empty icon={FolderTree}>{t("expertCenter.admin.noCategories")}</Empty>
            ) : (
              <div className="overflow-x-auto px-5 pb-5">
                <table className="tbl max-sm:min-w-[40rem]">
                  <thead>
                    <tr>
                      <th className="px-3 py-2">{t("expertCenter.admin.categoryName")}</th>
                      <th className="px-3 py-2">{t("expertCenter.admin.scope")}</th>
                      <th className="px-3 py-2">{t("expertCenter.admin.assets")}</th>
                      <th className="px-3 py-2">{t("expertCenter.admin.sort")}</th>
                      <th className="px-3 py-2 text-right">{t("expertCenter.admin.actions")}</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-neutral-100">
                    {list.map((c) => (
                      <tr key={c.id}>
                        <td className="px-3 py-2">
                          <input
                            aria-label={t("expertCenter.admin.categoryName")}
                            className={cx(inputBase, "w-44")}
                            defaultValue={c.name}
                            maxLength={32}
                            onBlur={(e) => rename(c, e.target.value)}
                          />
                        </td>
                        <td className="px-3 py-2 text-sm text-neutral-700">
                          <button type="button" className="text-left hover:text-brand-600" onClick={() => setScopeOf(c)}>
                            {scopeSummary(t, c.scope, opts.roleName)}
                          </button>
                        </td>
                        <td className="px-3 py-2 text-sm tabular-nums">{c.assets}</td>
                        <td className="px-3 py-2">
                          <input
                            aria-label={t("expertCenter.admin.sort")}
                            type="number"
                            className={cx(inputBase, "w-20")}
                            defaultValue={c.sort}
                            onBlur={(e) => Number(e.target.value) !== c.sort && move(c, Number(e.target.value) || 0)}
                          />
                        </td>
                        <td className="px-3 py-2 text-right">
                          <Button size="sm" variant="ghost" disabled={busy} onClick={() => remove(c)} aria-label={t("expertCenter.admin.delete")}>
                            <Trash2 size={14} />
                          </Button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </Card>
        );
      })}
      <ScopeDialog
        open={scopeOf !== null}
        onOpenChange={(v) => !v && setScopeOf(null)}
        title={t("expertCenter.admin.categoryScopeTitle", { name: scopeOf?.name ?? "" })}
        description={t("expertCenter.admin.categoryScopeDesc")}
        scope={scopeOf?.scope ?? allScope()}
        delivery="optional"
        canDelivery={false}
        labels={{ auto: "", optional: "", autoHint: "", optionalHint: "" }}
        onSave={async (scope) => {
          await api.patchCategory(scopeOf!.id, { scope });
          notify(t("expertCenter.admin.saved"));
          setScopeOf(null);
          onChanged();
        }}
      />
    </div>
  );
}

// ---- 精选与归类 ----

function CatalogTable({ assets, cats, onChanged }: { assets: HubAsset[]; cats: Category[]; onChanged: () => void }) {
  const { t } = useTranslation();
  const { notify } = useStore();
  const { busy, run } = useAction();
  const [type, setType] = useState<"all" | "expert" | "app" | "skill" | "prompt" | "connection">("all");
  const [q, setQ] = useState("");
  const list = useMemo(() => {
    const kw = q.trim().toLowerCase();
    return assets
      .filter((a) => (type === "all" || typeKey(a) === type) && (!kw || `${a.slug} ${a.spec.title}`.toLowerCase().includes(kw)))
      .sort((a, b) => Number(b.featured) - Number(a.featured) || a.slug.localeCompare(b.slug));
  }, [assets, type, q]);
  const featuredCount = (g: CategoryGroup) => assets.filter((a) => a.featured && groupOf(a) === g).length;

  const patch = async (a: HubAsset, p: { category_id?: number; featured?: boolean }) => {
    if (p.featured && featuredCount(groupOf(a)) >= 20) return notify(t("expertCenter.admin.featuredLimit"));
    const r = await run(() => api.patchAsset(a.id, p));
    notify(r.ok ? t("expertCenter.admin.saved") : r.error);
    if (r.ok) onChanged();
  };

  return (
    <Card>
      <div className="flex flex-wrap items-center gap-3 p-5 pb-3">
        <div className="relative w-full sm:w-64">
          <Search size={16} className="absolute top-1/2 left-3 -translate-y-1/2 text-neutral-500" />
          <input value={q} onChange={(e) => setQ(e.target.value)} placeholder={t("hub.search")} aria-label={t("hub.search")} className={cx(inputBase, "w-full pl-9")} />
        </div>
        <Segmented
          value={type}
          onChange={setType}
          options={(["all", "expert", "app", "skill", "prompt", "connection"] as const).map((k) => ({ value: k, label: t(`expertCenter.types.${k}`) }))}
        />
      </div>
      {list.length === 0 ? (
        <Empty>{t("hub.emptyFiltered")}</Empty>
      ) : (
        <div className="overflow-x-auto px-5 pb-5">
          <table className="tbl max-sm:min-w-[44rem]">
            <thead>
              <tr>
                <th className="px-3 py-2">{t("expertCenter.admin.colType")}</th>
                <th className="px-3 py-2">{t("expertCenter.admin.colTitle")}</th>
                <th className="px-3 py-2">{t("expertCenter.admin.colCategory")}</th>
                <th className="px-3 py-2">{t("expertCenter.featured")}</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-neutral-100">
              {list.map((a) => (
                <tr key={a.id}>
                  <td className="px-3 py-2">
                    <Badge tone={isExpert(a) ? "brand" : "slate"}>{t(`expertCenter.types.${typeKey(a)}`)}</Badge>
                  </td>
                  <td className="px-3 py-2">
                    <Link to={assetPath(a.id)} className="font-medium text-neutral-900 hover:text-brand-600">
                      {a.spec.title}
                    </Link>
                    <div className="font-mono text-xs text-neutral-500">{a.slug}</div>
                  </td>
                  <td className="px-3 py-2">
                    <select
                      aria-label={t("expertCenter.admin.colCategory")}
                      className={cx(inputBase, "w-40")}
                      disabled={busy}
                      value={a.category_id ?? 0}
                      onChange={(e) => patch(a, { category_id: Number(e.target.value) })}
                    >
                      <option value={0}>{t("expertCenter.admin.uncategorized")}</option>
                      {cats
                        .filter((c) => c.group === groupOf(a))
                        .map((c) => (
                          <option key={c.id} value={c.id}>
                            {c.name}
                          </option>
                        ))}
                    </select>
                  </td>
                  <td className="px-3 py-2">
                    <button
                      type="button"
                      disabled={busy}
                      aria-pressed={a.featured}
                      aria-label={t("expertCenter.featured")}
                      onClick={() => patch(a, { featured: !a.featured })}
                      className={cx("rounded-md p-1.5 transition-colors hover:bg-neutral-100", a.featured ? "text-amber-500" : "text-neutral-400")}
                    >
                      <Star size={16} fill={a.featured ? "currentColor" : "none"} />
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </Card>
  );
}

// ---- 禁用策略 ----

function Blocks({ assets }: { assets: HubAsset[] }) {
  const { t } = useTranslation();
  const { notify } = useStore();
  const confirm = useConfirm();
  const opts = useScopeOptions();
  const blocks = useApi(api.blocks);
  const { busy, run } = useAction();
  const [scope, setScope] = useState<Scope>(emptyScope());
  const [ids, setIds] = useState<number[]>([]);
  const [note, setNote] = useState("");
  const [error, setError] = useState<string | null>(null);
  const byID = new Map(assets.map((a) => [a.id, a]));
  const options = assets.filter((a) => !ids.includes(a.id)).map((a) => ({ value: String(a.id), label: a.spec.title, hint: `${t(`expertCenter.types.${typeKey(a)}`)} · ${a.slug}` }));

  const create = async () => {
    setError(null);
    const r = await run(() => api.createBlock({ scope, asset_ids: ids, note: note.trim() }));
    if (!r.ok) return setError(r.error);
    setScope(emptyScope());
    setIds([]);
    setNote("");
    notify(t("expertCenter.admin.blockCreated"));
    void blocks.reload();
  };
  const remove = async (b: AssetBlock) => {
    if (!(await confirm({ kind: "delete", description: t("expertCenter.admin.blockDeleteConfirm") }))) return;
    const r = await run(() => api.deleteBlock(b.id));
    notify(r.ok ? t("expertCenter.admin.blockDeleted") : r.error);
    if (r.ok) void blocks.reload();
  };

  return (
    <div className="space-y-4">
      <Card className="space-y-4 p-5">
        <h2 className="text-[15px] font-semibold text-neutral-900">{t("expertCenter.admin.newBlock")}</h2>
        <Field label={t("expertCenter.admin.blockScope")} hint={t("expertCenter.admin.blockScopeHint")}>
          <ScopeEditor value={scope} onChange={setScope} roles={opts.roles} users={opts.users} departments={opts.departments} noneHint={t("expertCenter.admin.blockScopeNone")} />
        </Field>
        <Field label={t("expertCenter.admin.blockAssets")}>
          <div className="flex flex-wrap items-center gap-1.5">
            {ids.map((id) => (
              <span key={id} className="inline-flex items-center gap-1 rounded bg-neutral-100 px-2 py-0.5 text-sm text-neutral-800">
                {byID.get(id)?.spec.title ?? `#${id}`}
                <button type="button" aria-label={t("scopeEditor.remove", { name: byID.get(id)?.spec.title ?? id })} onClick={() => setIds(ids.filter((x) => x !== id))} className="text-neutral-500 hover:text-neutral-900">
                  <X size={12} />
                </button>
              </span>
            ))}
            <Combobox className="w-72" value="" aria-label={t("expertCenter.admin.blockAssets")} placeholder={t("editor.pick")} options={options} onChange={(v) => v && setIds([...ids, Number(v)])} />
          </div>
        </Field>
        <Field label={t("expertCenter.admin.blockNote")} hint={t("expertCenter.admin.blockNoteHint")}>
          <input className={inputCls} value={note} maxLength={500} onChange={(e) => setNote(e.target.value)} />
        </Field>
        <ErrorNote>{error}</ErrorNote>
        <Button disabled={busy || ids.length === 0 || !note.trim()} onClick={create}>
          <Ban size={14} /> {t("expertCenter.admin.addBlock")}
        </Button>
      </Card>
      <Card>
        <CardHeader title={t("expertCenter.admin.blocksTitle")} />
        <LoadState loading={blocks.loading && !blocks.data} error={blocks.data ? null : blocks.error} onRetry={blocks.reload}>
          {(blocks.data ?? []).length === 0 ? (
            <Empty icon={Ban}>{t("expertCenter.admin.noBlocks")}</Empty>
          ) : (
            <div className="overflow-x-auto px-5 pb-5">
              <table className="tbl max-sm:min-w-[44rem]">
                <thead>
                  <tr>
                    <th className="px-3 py-2">{t("expertCenter.admin.blockScope")}</th>
                    <th className="px-3 py-2">{t("expertCenter.admin.blockAssets")}</th>
                    <th className="px-3 py-2">{t("expertCenter.admin.blockNote")}</th>
                    <th className="px-3 py-2 text-right">{t("expertCenter.admin.actions")}</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-neutral-100">
                  {blocks.data!.map((b) => (
                    <tr key={b.id}>
                      <td className="px-3 py-2 text-sm">{scopeSummary(t, b.scope, opts.roleName)}</td>
                      <td className="px-3 py-2">
                        <div className="flex flex-wrap gap-1">
                          {b.asset_ids.map((id) => (
                            <Badge key={id}>{byID.get(id)?.spec.title ?? `#${id}`}</Badge>
                          ))}
                        </div>
                      </td>
                      <td className="px-3 py-2 text-sm text-neutral-700">{b.note}</td>
                      <td className="px-3 py-2 text-right">
                        <Button size="sm" variant="ghost" disabled={busy} onClick={() => remove(b)} aria-label={t("expertCenter.admin.delete")}>
                          <Trash2 size={14} />
                        </Button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </LoadState>
      </Card>
    </div>
  );
}

// ---- 权限查询 ----

const resultTone: Record<EffectiveItem["result"], Tone> = { enabled: "green", available: "brand", hidden: "red" };

function Effective() {
  const { t } = useTranslation();
  const opts = useScopeOptions();
  const { busy, run } = useAction();
  const [user, setUser] = useState("");
  const [data, setData] = useState<Awaited<ReturnType<typeof api.effective>> | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [only, setOnly] = useState<"all" | "hidden">("all");

  const query = async (id: string) => {
    setUser(id);
    setError(null);
    if (!id) return setData(null);
    const r = await run(() => api.effective(Number(id)));
    if (r.ok) setData(r.value);
    else setError(r.error);
  };
  const items = (data?.items ?? []).filter((it) => only === "all" || it.result === "hidden");

  return (
    <Card className="p-5">
      <div className="flex flex-wrap items-end gap-3">
        <Field label={t("expertCenter.admin.pickUser")}>
          <Combobox
            className="w-72"
            value={user}
            aria-label={t("expertCenter.admin.pickUser")}
            placeholder={t("expertCenter.admin.pickUser")}
            options={(opts.users ?? []).map((u) => ({ value: String(u.id), label: u.display_name || u.username, hint: [u.username, u.department].filter(Boolean).join(" · ") }))}
            onChange={query}
          />
        </Field>
        {data && <Segmented value={only} onChange={setOnly} options={[{ value: "all", label: t("expertCenter.admin.showAll") }, { value: "hidden", label: t("expertCenter.admin.showHidden") }]} />}
      </div>
      <ErrorNote>{error}</ErrorNote>
      {busy && <p className="mt-4 text-sm text-neutral-500">{t("common.loading")}</p>}
      {data && !busy && (
        <div className="mt-4 overflow-x-auto">
          <table className="tbl max-sm:min-w-[40rem]">
            <thead>
              <tr>
                <th className="px-3 py-2">{t("expertCenter.admin.colType")}</th>
                <th className="px-3 py-2">{t("expertCenter.admin.colTitle")}</th>
                <th className="px-3 py-2">{t("expertCenter.admin.colResult")}</th>
                <th className="px-3 py-2">{t("expertCenter.admin.colReason")}</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-neutral-100">
              {items.map((it) => (
                <tr key={it.id}>
                  <td className="px-3 py-2">
                    <Badge>{t(`expertCenter.types.${it.kind === "app" && it.form === "expert" ? "expert" : it.kind}`)}</Badge>
                  </td>
                  <td className="px-3 py-2">
                    <Link to={assetPath(it.id)} className="font-medium text-neutral-900 hover:text-brand-600">
                      {it.title}
                    </Link>
                    <div className="font-mono text-xs text-neutral-500">{it.slug}</div>
                  </td>
                  <td className="px-3 py-2">
                    <Badge tone={resultTone[it.result]}>{t(`expertCenter.results.${it.result}`)}</Badge>
                  </td>
                  <td className="px-3 py-2 text-sm text-neutral-700">
                    {it.reason ? t(`expertCenter.reasons.${it.reason}`) : "—"}
                    {it.block_ids?.length ? <span className="ml-1 text-xs text-neutral-500">#{it.block_ids.join(", #")}</span> : null}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </Card>
  );
}
