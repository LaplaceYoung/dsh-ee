import { useEffect, useState } from "react";
import { Link, useNavigate, useParams } from "react-router";
import { useTranslation } from "react-i18next";
import { ArrowLeft, FilePlus, MoreHorizontal, Sparkles, Upload, ChevronRight } from "lucide-react";
import { api, formatTime, type KBase, type KBMaintainers, type KBSearchResult, type Scope } from "../lib/api";
import { useAction, useApi } from "../lib/use-api";
import { useStore } from "../lib/store";
import { useConfirm } from "../lib/confirm";
import { scopeSummary } from "../lib/hub";
import { LoadState, ErrorNote } from "../components/api-ui";
import { ScopeEditor } from "../components/scope-editor";
import { useScopeOptions } from "../components/scope-dialog";
import { MaintainersEditor, Snippet } from "../components/kb-access";
import { Badge, Button, Card, cx, Field, inputBase, inputCls, Segmented, Toggle, Notice, backLinkCls } from "../components/ui";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from "@/components/shadcn/dropdown-menu";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/shadcn/dialog";
import { kdocPath, QueryTable } from "./knowledge";
import { UsedByCard } from "../components/used-by";
import { filesFromDrop, UploadDialog, type QueuedFile } from "../components/kb-upload";
import { DocList } from "../components/kb-doc-list";
import { KBSources } from "../components/kb-sources";
import { ObjectFields } from "../components/object-fields";
import { useAuth } from "../lib/auth";
import { useMenus } from "../lib/menus";

type Tab = "docs" | "search" | "queries" | "sources";
type Mode = "any" | "all" | "phrase" | "regex";

function NewDocDialog({ kb, open, onOpenChange, onCreated }: { kb: KBase; open: boolean; onOpenChange: (v: boolean) => void; onCreated: (id: number) => void }) {
  const { t } = useTranslation();
  const [title, setTitle] = useState("");
  const [folder, setFolder] = useState("");
  const [body, setBody] = useState("");
  const [error, setError] = useState<string | null>(null);
  const { busy, run } = useAction();
  useEffect(() => {
    if (open) {
      setTitle("");
      setBody("");
      setError(null);
    }
  }, [open]);
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t("kbase.newDocTitle")}</DialogTitle>
        </DialogHeader>
        <div className="space-y-4">
          <div className="grid gap-4 md:grid-cols-2">
            <Field label={t("kbase.docTitle")}>
              <input className={inputCls} value={title} onChange={(e) => setTitle(e.target.value)} />
            </Field>
            <Field label={t("kbase.uploadFolder")}>
              <input className={inputCls} value={folder} onChange={(e) => setFolder(e.target.value)} />
            </Field>
          </div>
          <Field label={t("kbase.docBody")} hint={t("kbase.docBodyHint")}>
            <textarea className={cx(inputCls, "h-72 py-2 font-mono text-xs")} value={body} onChange={(e) => setBody(e.target.value)} />
          </Field>
        </div>
        <ErrorNote>{error}</ErrorNote>
        <DialogFooter>
          <Button variant="secondary" onClick={() => onOpenChange(false)}>
            {t("common.cancel")}
          </Button>
          <Button
            disabled={busy || !title.trim() || !body.trim()}
            onClick={async () => {
              const r = await run(() => api.createKBDoc(kb.id, title.trim(), body, folder));
              const doc = r.ok ? r.value.results[0]?.doc : undefined;
              if (doc) {
                onOpenChange(false);
                onCreated(doc.id);
              } else setError(r.ok ? r.value.results[0]?.error ?? "" : r.error);
            }}
          >
            {t("kbase.create")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function InfoDialog({ kb, open, onOpenChange, onSaved }: { kb: KBase; open: boolean; onOpenChange: (v: boolean) => void; onSaved: () => void }) {
  const { t } = useTranslation();
  const [title, setTitle] = useState(kb.title);
  const [desc, setDesc] = useState(kb.description);
  const [error, setError] = useState<string | null>(null);
  const { busy, run } = useAction();
  useEffect(() => {
    if (open) {
      setTitle(kb.title);
      setDesc(kb.description);
      setError(null);
    }
  }, [open, kb]);
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("kbase.infoTitle")}</DialogTitle>
        </DialogHeader>
        <div className="space-y-4">
          <Field label={t("knowledge.createForm.name")}>
            <input className={inputCls} value={title} onChange={(e) => setTitle(e.target.value)} />
          </Field>
          <Field label={t("knowledge.createForm.description")} hint={t("knowledge.createForm.descriptionHint")}>
            <textarea className={cx(inputCls, "h-24 py-2")} value={desc} onChange={(e) => setDesc(e.target.value)} />
          </Field>
        </div>
        <ErrorNote>{error}</ErrorNote>
        <DialogFooter>
          <Button variant="secondary" onClick={() => onOpenChange(false)}>
            {t("common.cancel")}
          </Button>
          <Button
            disabled={busy || !title.trim()}
            onClick={async () => {
              const r = await run(() => api.patchKB(kb.id, { title: title.trim(), description: desc.trim() }));
              if (r.ok) {
                onOpenChange(false);
                onSaved();
              } else setError(r.error);
            }}
          >
            {t("kbase.save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function AccessDialog({ kb, open, onOpenChange, onSaved }: { kb: KBase; open: boolean; onOpenChange: (v: boolean) => void; onSaved: () => void }) {
  const { t } = useTranslation();
  const opts = useScopeOptions();
  const [scope, setScope] = useState<Scope>(kb.scope);
  const [maint, setMaint] = useState<KBMaintainers>(kb.maintainers);
  const [error, setError] = useState<string | null>(null);
  const { busy, run } = useAction();
  useEffect(() => {
    if (open) {
      setScope(kb.scope);
      setMaint(kb.maintainers);
      setError(null);
    }
  }, [open, kb]);
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>{t("kbase.accessTitle")}</DialogTitle>
          <DialogDescription>{kb.title}</DialogDescription>
        </DialogHeader>
        <div className="space-y-5">
          <div>
            <div className="mb-1.5 text-sm font-semibold text-neutral-800">{t("knowledge.scope")}</div>
            <ScopeEditor value={scope} onChange={setScope} roles={opts.roles} users={opts.users} departments={opts.departments} />
          </div>
          <div>
            <div className="mb-1 text-sm font-semibold text-neutral-800">{t("knowledge.maintainers")}</div>
            <p className="mb-2 text-xs text-neutral-500">{t("kbase.maintainersHint")}</p>
            <MaintainersEditor value={maint} onChange={setMaint} roles={opts.roles} users={opts.users} />
          </div>
        </div>
        <ErrorNote>{error}</ErrorNote>
        <DialogFooter>
          <Button variant="secondary" onClick={() => onOpenChange(false)}>
            {t("common.cancel")}
          </Button>
          <Button
            disabled={busy}
            onClick={async () => {
              const r = await run(() => api.patchKB(kb.id, { scope, maintainers: maint, updated_at: kb.updated_at }));
              if (r.ok) {
                onOpenChange(false);
                onSaved();
              } else setError(r.error);
            }}
          >
            {t("kbase.save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function SearchTab({ kb }: { kb: KBase }) {
  const { t } = useTranslation();
  const [query, setQuery] = useState("");
  const [mode, setMode] = useState<Mode>("any");
  const [res, setRes] = useState<KBSearchResult | null>(null);
  const [error, setError] = useState<string | null>(null);
  const { busy, run } = useAction();
  const search = async () => {
    if (!query.trim()) return;
    const r = await run(() => api.searchKB({ query, mode, kb_ids: [kb.id] }));
    if (r.ok) {
      setRes(r.value);
      setError(null);
    } else setError(r.error);
  };
  return (
    <div className="space-y-4 p-5">
      <p className="text-sm text-neutral-600">{t("kbase.searchHint")}</p>
      <div className="flex flex-wrap items-center gap-2">
        <input
          className={cx(inputBase, "min-w-64 flex-1")}
          value={query}
          placeholder={t("kbase.searchPlaceholder")}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && void search()}
        />
        <Segmented value={mode} onChange={setMode} options={(["any", "all", "phrase", "regex"] as Mode[]).map((m) => ({ value: m, label: t(`kbase.modes.${m}`) }))} />
        <Button disabled={busy || !query.trim()} onClick={() => void search()}>
          {t("kbase.searchSubmit")}
        </Button>
      </div>
      <ErrorNote>{error}</ErrorNote>
      {res && (
        <div className="space-y-3">
          <div className="flex flex-wrap items-center gap-2 text-xs text-neutral-600">
            <span>{t("kbase.total", { count: res.total })}</span>
            <span>{t("kbase.termHits")}</span>
            {Object.entries(res.term_hits).map(([term, n]) => (
              <Badge key={term} tone={n === 0 ? "amber" : "slate"}>
                {term} {n}
              </Badge>
            ))}
          </div>
          {res.hits.length === 0 ? (
            <p className="text-sm text-neutral-600">{t("kbase.noHits")}</p>
          ) : (
            <ul className="space-y-3">
              {res.hits.map((h, i) => (
                <li key={i} className="rounded-md border border-neutral-100 p-3">
                  <div className="flex flex-wrap items-baseline gap-2 text-sm">
                    <Link to={kdocPath(h.doc, { from: h.from, to: h.to })} className="font-medium text-brand-600 hover:text-brand-700 dark:text-brand-300 dark:hover:text-brand-200">
                      {h.title}
                    </Link>
                    <span className="text-xs text-neutral-500">
                      {h.heading && `${h.heading} · `}L{h.from}–{h.to}
                    </span>
                  </div>
                  <p className="mt-1 text-sm text-neutral-700">
                    <Snippet text={h.snippet} />
                  </p>
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
    </div>
  );
}

/** 检索情况：最上面是没查到的（待补文档），其次是常被读的文档，检索流水收在最下面。 */
function QueriesTab({ kb, onUpload, onChanged }: { kb: KBase; onUpload: () => void; onChanged: () => void }) {
  const { t } = useTranslation();
  const { notify } = useStore();
  const gaps = useApi(() => api.kbGaps(kb.id));
  const q = useApi(() => api.kbQueries({ kb: kb.id }));
  const [zero, setZero] = useState(false);
  const items = (q.data ?? []).filter((x) => !zero || (x.tool === "search" && x.hits === 0));
  const days = gaps.data?.days ?? 30;
  return (
    <div className="divide-y divide-neutral-100">
      <section className="px-5 py-4">
        <h3 className="text-sm font-semibold text-neutral-900">{t("kbase.gapsTitle", { days })}</h3>
        <p className="mt-0.5 text-sm text-neutral-600">{t("kbase.gapsHint")}</p>
        <LoadState loading={gaps.loading && !gaps.data} error={gaps.data ? null : gaps.error} onRetry={gaps.reload}>
          {gaps.data && gaps.data.gaps.length === 0 ? (
            <Notice tone="success" className="mt-3">{t("kbase.gapsEmpty", { days })}</Notice>
          ) : (
            <ul className="mt-3 space-y-2">
              {gaps.data?.gaps.map((g) => (
                <li key={g.key} className="flex flex-wrap items-center justify-between gap-3 rounded-md border border-neutral-200 px-3 py-2">
                  <div className="min-w-0">
                    <div className="font-medium text-neutral-900">{g.query}</div>
                    <div className="text-xs text-neutral-500">{t("kbase.gapCount", { count: g.count, users: g.users, time: formatTime(g.last_at) })}</div>
                  </div>
                  {kb.can_edit && (
                    <div className="flex gap-2">
                      <Button size="sm" onClick={onUpload}>
                        <Upload size={14} /> {t("kbase.gapUpload")}
                      </Button>
                      <Button
                        size="sm"
                        variant="ghost"
                        onClick={async () => {
                          try {
                            await api.ignoreKBGap(kb.id, g.key);
                            notify(t("kbase.gapIgnored", { query: g.query }));
                            gaps.reload();
                            onChanged();
                          } catch (e) {
                            notify(e instanceof Error ? e.message : String(e));
                          }
                        }}
                      >
                        {t("kbase.gapIgnore")}
                      </Button>
                    </div>
                  )}
                </li>
              ))}
            </ul>
          )}
        </LoadState>
      </section>
      <section className="px-5 py-4">
        <h3 className="text-sm font-semibold text-neutral-900">{t("kbase.popularTitle")}</h3>
        {gaps.data && gaps.data.popular.length === 0 ? (
          <p className="mt-2 text-sm text-neutral-600">{t("kbase.popularEmpty", { days })}</p>
        ) : (
          <ol className="mt-2 space-y-1 text-sm">
            {gaps.data?.popular.map((p, i) => (
              <li key={p.doc} className="flex items-center justify-between gap-3">
                <span className="min-w-0 truncate">
                  <span className="mr-2 inline-block w-5 text-right text-neutral-400 tabular-nums">{i + 1}</span>
                  <Link to={kdocPath(p.doc)} className="text-neutral-800 hover:underline">
                    {p.title}
                  </Link>
                </span>
                <span className="shrink-0 text-xs text-neutral-500">{t("kbase.popularCount", { count: p.reads, users: p.users })}</span>
              </li>
            ))}
          </ol>
        )}
      </section>
      <details className="group">
        <summary className="flex cursor-pointer list-none items-center gap-1.5 rounded-lg px-5 py-3.5 text-sm font-semibold text-neutral-900 select-none [&::-webkit-details-marker]:hidden">
          <ChevronRight size={15} aria-hidden className="text-neutral-400 transition-transform group-open:rotate-90" />
          {t("kbase.logTitle")}
        </summary>
        <div className="flex flex-wrap items-center justify-between gap-3 px-5 pb-2">
          <p className="text-sm text-neutral-600">{t("kbase.queriesHint")}</p>
          <label className="flex items-center gap-2 text-sm text-neutral-700">
            <Toggle checked={zero} onChange={setZero} />
            {t("kbase.onlyZero")}
          </label>
        </div>
        <LoadState loading={q.loading && !q.data} error={q.data ? null : q.error} onRetry={q.reload}>
          <QueryTable items={items} showUser empty={t("kbase.queriesEmpty")} />
        </LoadState>
      </details>
    </div>
  );
}

export default function KnowledgeBase() {
  const { t } = useTranslation();
  const { id } = useParams();
  const navigate = useNavigate();
  const confirm = useConfirm();
  const { notify } = useStore();
  const opts = useScopeOptions();
  const { can } = useAuth();
  const { pathOf } = useMenus();
  const detail = useApi(() => api.kb(Number(id)));
  const [tab, setTab] = useState<Tab>("docs");
  const [folder, setFolder] = useState<string | null>(null);
  const [dialog, setDialog] = useState<"upload" | "new" | "info" | "access" | null>(null);
  // 拖到文档列表上的文件：打开上传窗口，目标目录是当前选的目录
  const [dropped, setDropped] = useState<QueuedFile[] | undefined>(undefined);
  const [dragging, setDragging] = useState(false);
  const kb = detail.data?.kb;
  const docs = detail.data?.docs ?? [];
  const userName = (uid: number) => {
    const u = opts.users?.find((x) => x.id === uid);
    return u ? u.display_name || u.username : `#${uid}`;
  };
  const maintainers = kb ? [...kb.maintainers.users.map(userName), ...kb.maintainers.roles.map(opts.roleName)] : [];

  return (
    <div>
      <Link to="/knowledge" className={backLinkCls}>
        <ArrowLeft size={14} /> {t("kbase.back")}
      </Link>
      <LoadState loading={detail.loading && !detail.data} error={detail.data ? null : detail.error} onRetry={detail.reload}>
        {kb && (
          <>
            <div className="mb-6 flex flex-wrap items-start justify-between gap-4">
              <div>
                <div className="flex flex-wrap items-center gap-2">
                  <h1 className="text-2xl text-neutral-900 leading-8 font-bold tracking-[-0.02em]">{kb.title}</h1>
                  <span className="font-mono text-sm text-neutral-500">{kb.slug}</span>
                  {!kb.readable && <Badge tone="amber">{t("knowledge.notReadable")}</Badge>}
                </div>
                {kb.description && <p className="mt-1 max-w-3xl text-sm text-neutral-600">{kb.description}</p>}
                <dl className="mt-3 flex flex-wrap gap-x-8 gap-y-1 text-sm">
                  <div>
                    <dt className="inline text-neutral-500">{t("knowledge.scope")}：</dt>
                    <dd className="inline text-neutral-900">{scopeSummary(t, kb.scope, opts.roleName)}</dd>
                  </div>
                  <div>
                    <dt className="inline text-neutral-500">{t("knowledge.maintainers")}：</dt>
                    <dd className="inline text-neutral-900">{maintainers.length ? maintainers.join("、") : t("knowledge.noMaintainers")}</dd>
                  </div>
                  <div className="text-neutral-600">
                    {t("knowledge.docs", { count: kb.docs })} · {t("knowledge.updated", { time: formatTime(kb.updated_at) })}
                  </div>
                </dl>
              </div>
              <div className="flex flex-wrap gap-2">
              {kb.readable && can("registry.publish") && (
                <Button variant="secondary" onClick={() => navigate(`${pathOf("app-new")}?knowledge=${encodeURIComponent(kb.slug)}`)}>
                  <Sparkles size={15} /> {t("studio.makeAppFromKB")}
                </Button>
              )}
              {kb.can_edit && (
                <div className="flex flex-wrap gap-2">
                  <Button onClick={() => setDialog("upload")}>
                    <Upload size={15} /> {t("kbase.upload")}
                  </Button>
                  <Button variant="secondary" onClick={() => setDialog("new")}>
                    <FilePlus size={15} /> {t("kbase.newDoc")}
                  </Button>
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button variant="secondary" aria-label="⋯">
                        <MoreHorizontal size={16} />
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      <DropdownMenuItem onSelect={() => setDialog("info")}>{t("kbase.editInfo")}</DropdownMenuItem>
                      {kb.can_manage && <DropdownMenuItem onSelect={() => setDialog("access")}>{t("kbase.access")}</DropdownMenuItem>}
                      {kb.can_manage && (
                        <>
                          <DropdownMenuSeparator />
                          <DropdownMenuItem
                            className="text-red-600"
                            onSelect={async () => {
                              if (!(await confirm({ kind: "delete", description: t("kbase.deleteConfirm", { title: kb.title, count: kb.docs }) }))) return;
                              try {
                                await api.deleteKB(kb.id);
                                notify(t("kbase.deleted", { title: kb.title }));
                                navigate("/knowledge");
                              } catch (e) {
                                notify(e instanceof Error ? e.message : String(e));
                              }
                            }}
                          >
                            {t("kbase.delete")}
                          </DropdownMenuItem>
                        </>
                      )}
                    </DropdownMenuContent>
                  </DropdownMenu>
                </div>
              )}
              </div>
            </div>
            {kb.readable && kb.docs > 0 && can("registry.publish") && (detail.data?.used_by?.length ?? 0) === 0 && (
              <div className="mb-4 flex flex-wrap items-center justify-between gap-3 rounded-lg border border-brand-200/80 bg-brand-50/70 px-4 py-3 dark:border-brand-400/25 dark:bg-brand-500/10">
                <div className="min-w-0">
                  <div className="text-sm font-medium text-neutral-900">{t("kbase.noAppTitle")}</div>
                  <p className="text-sm text-neutral-600">{t("kbase.noAppHint")}</p>
                </div>
                <Button size="sm" onClick={() => navigate(`${pathOf("app-new")}?knowledge=${encodeURIComponent(kb.slug)}`)}>
                  <Sparkles size={14} /> {t("studio.makeAppFromKB")}
                </Button>
              </div>
            )}
            <div className="mb-4">
              <Segmented
                value={tab}
                onChange={setTab}
                options={(["docs", "search", ...(kb.can_edit ? ["queries", "sources"] : [])] as Tab[]).map((x) => ({
                  value: x,
                  label:
                    x === "sources"
                      ? t("kbSources.title")
                      : x === "queries" && kb.gaps
                        ? `${t("kbase.tabs.queries")} · ${t("kbase.gapsShort", { count: kb.gaps })}`
                        : t(`kbase.tabs.${x}`),
                }))}
              />
            </div>
            <Card>
              {tab === "docs" && (
                <div
                  className="relative"
                  onDragOver={(e) => {
                    if (!kb.can_edit || !e.dataTransfer.types.includes("Files")) return;
                    e.preventDefault();
                    setDragging(true);
                  }}
                  onDragLeave={(e) => {
                    if (!e.currentTarget.contains(e.relatedTarget as Node)) setDragging(false);
                  }}
                  onDrop={async (e) => {
                    if (!kb.can_edit) return;
                    e.preventDefault();
                    setDragging(false);
                    setDropped(await filesFromDrop(e.dataTransfer));
                    setDialog("upload");
                  }}
                >
                  {dragging && (
                    <div className="pointer-events-none absolute inset-0 z-10 flex items-center justify-center rounded-lg border-2 border-dashed border-brand-400 bg-brand-50/80 text-sm font-medium text-brand-700 dark:bg-brand-500/15 dark:text-brand-200">
                      {t("kupload.dropHere", { folder: folder || t("kbase.folderRoot") })}
                    </div>
                  )}
                  <DocList kb={kb} docs={docs} folder={folder} setFolder={setFolder} onChanged={detail.reload} />
                </div>
              )}
              {tab === "search" && <SearchTab kb={kb} />}
              {tab === "queries" && kb.can_edit && <QueriesTab kb={kb} onUpload={() => setDialog("upload")} onChanged={detail.reload} />}
              {tab === "sources" && kb.can_edit && <KBSources kbId={kb.id} onChanged={detail.reload} />}
            </Card>
            {kb.can_edit && (
              <div className="mt-4">
                <ObjectFields object="kbase" id={kb.id} />
              </div>
            )}
            {detail.data?.used_by && (
              <div className="mt-4">
                <UsedByCard apps={detail.data.used_by} />
              </div>
            )}
            <UploadDialog
              kb={kb}
              open={dialog === "upload"}
              initial={dropped}
              initialFolder={folder ?? ""}
              onOpenChange={(v) => {
                setDialog(v ? "upload" : null);
                if (!v) setDropped(undefined);
              }}
              onDone={detail.reload}
            />
            <NewDocDialog kb={kb} open={dialog === "new"} onOpenChange={(v) => setDialog(v ? "new" : null)} onCreated={(doc) => navigate(kdocPath(doc))} />
            <InfoDialog kb={kb} open={dialog === "info"} onOpenChange={(v) => setDialog(v ? "info" : null)} onSaved={detail.reload} />
            <AccessDialog kb={kb} open={dialog === "access"} onOpenChange={(v) => setDialog(v ? "access" : null)} onSaved={detail.reload} />
          </>
        )}
      </LoadState>
    </div>
  );
}
