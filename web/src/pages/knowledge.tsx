import { useEffect, useRef, useState } from "react";
import { Link, useNavigate } from "react-router";
import { BookOpen, Download, ExternalLink, FolderPlus, MessageSquare, Plus, Search, Upload, SearchX, ChevronRight } from "lucide-react";
import { useTranslation } from "react-i18next";
import { api, formatTime, type KBase, type KBQASettings, type KBQuery, type KBSearchResult, type Scope } from "../lib/api";
import { formatDateOnly } from "../lib/format";
import { copyText } from "../lib/clipboard";
import { useAction, useApi } from "../lib/use-api";
import { useStore } from "../lib/store";
import { scopeSummary } from "../lib/hub";
import { LoadState, ErrorNote } from "../components/api-ui";
import { ScopeEditor } from "../components/scope-editor";
import { useScopeOptions } from "../components/scope-dialog";
import { Badge, Button, Card, CardHeader, cx, Field, inputCls, PageHeader, ToggleRow, Empty, cardHover } from "../components/ui";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/shadcn/dialog";
import { Snippet } from "../components/kb-access";

export const kbPath = (id: number) => `/knowledge/${id}`;
export const kdocPath = (id: number, line?: { from: number; to: number }) => `/knowledge/docs/${id}${line ? `#L${line.from}-${line.to}` : ""}`;

function CreateDialog({ open, onOpenChange, onCreated }: { open: boolean; onOpenChange: (v: boolean) => void; onCreated: (b: KBase) => void }) {
  const { t } = useTranslation();
  const opts = useScopeOptions();
  const [slug, setSlug] = useState("");
  const [title, setTitle] = useState("");
  const [desc, setDesc] = useState("");
  const [scope, setScope] = useState<Scope>({ all: true, departments: [], roles: [], users: [] });
  const [error, setError] = useState<string | null>(null);
  const { busy, run } = useAction();
  useEffect(() => {
    if (open) {
      setSlug("");
      setTitle("");
      setDesc("");
      setScope({ all: true, departments: [], roles: [], users: [] });
      setError(null);
    }
  }, [open]);
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>{t("knowledge.createForm.title")}</DialogTitle>
        </DialogHeader>
        <div className="space-y-4">
          <Field label={t("knowledge.createForm.name")}>
            <input className={inputCls} value={title} onChange={(e) => setTitle(e.target.value)} />
          </Field>
          <Field label={t("knowledge.createForm.slug")} hint={t("knowledge.createForm.slugHint")}>
            <input className={cx(inputCls, "font-mono")} value={slug} onChange={(e) => setSlug(e.target.value.toLowerCase())} />
          </Field>
          <Field label={t("knowledge.createForm.description")} hint={t("knowledge.createForm.descriptionHint")}>
            <textarea className={cx(inputCls, "h-20 py-2")} value={desc} onChange={(e) => setDesc(e.target.value)} />
          </Field>
          <div>
            <div className="mb-1.5 text-sm font-medium text-neutral-700">{t("knowledge.scope")}</div>
            <ScopeEditor value={scope} onChange={setScope} roles={opts.roles} users={opts.users} departments={opts.departments} />
            <p className="mt-1 text-xs text-neutral-500">{t("knowledge.createForm.scopeHint")}</p>
          </div>
        </div>
        <ErrorNote>{error}</ErrorNote>
        <DialogFooter>
          <Button variant="secondary" onClick={() => onOpenChange(false)}>
            {t("common.cancel")}
          </Button>
          <Button
            disabled={busy || !slug.trim() || !title.trim()}
            onClick={async () => {
              const r = await run(() => api.createKB({ slug: slug.trim(), title: title.trim(), description: desc.trim(), scope }));
              if (r.ok) {
                onOpenChange(false);
                onCreated(r.value);
              } else setError(r.error);
            }}
          >
            {t("knowledge.createForm.submit")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function QASettings({ value, onSaved }: { value: KBQASettings; onSaved: () => void }) {
  const { t } = useTranslation();
  const { notify } = useStore();
  const [s, setS] = useState(value);
  const [starters, setStarters] = useState(value.qaStarters.join("\n"));
  const [error, setError] = useState<string | null>(null);
  const { busy, run } = useAction();
  useEffect(() => {
    setS(value);
    setStarters(value.qaStarters.join("\n"));
  }, [value]);
  return (
    <Card>
      <CardHeader title={t("knowledge.qa.title")} />
      <div className="space-y-4 px-5 pb-5">
        <p className="text-sm text-neutral-600">{t("knowledge.qa.desc")}</p>
        <ToggleRow label={t("knowledge.qa.enabled")} checked={s.qaEnabled} onChange={(v) => setS({ ...s, qaEnabled: v })} />
        <div className="grid gap-4 md:grid-cols-[1fr_120px]">
          <Field label={t("knowledge.qa.name")}>
            <input className={inputCls} value={s.qaTitle} onChange={(e) => setS({ ...s, qaTitle: e.target.value })} />
          </Field>
          <Field label={t("knowledge.qa.icon")}>
            <input className={inputCls} value={s.qaIcon} onChange={(e) => setS({ ...s, qaIcon: e.target.value })} />
          </Field>
        </div>
        <Field label={t("knowledge.qa.starters")} hint={t("knowledge.qa.startersHint")}>
          <textarea className={cx(inputCls, "h-20 py-2")} value={starters} onChange={(e) => setStarters(e.target.value)} />
        </Field>
        <Field label={t("knowledge.qa.extra")} hint={t("knowledge.qa.extraHint")}>
          <textarea className={cx(inputCls, "h-20 py-2")} value={s.qaExtra} onChange={(e) => setS({ ...s, qaExtra: e.target.value })} />
        </Field>
        <ErrorNote>{error}</ErrorNote>
        <Button
          disabled={busy}
          onClick={async () => {
            const r = await run(() => api.saveKBSettings({ ...s, qaStarters: starters.split("\n").map((x) => x.trim()).filter(Boolean) }));
            if (r.ok) {
              setError(null);
              notify(t("knowledge.qa.saved"));
              onSaved();
            } else setError(r.error);
          }}
        >
          {t("knowledge.qa.save")}
        </Button>
      </div>
    </Card>
  );
}

/** 检索记录表：我的、某个库的都用它。 */
export function QueryTable({ items, showUser, empty, bases }: { items: KBQuery[]; showUser?: boolean; empty: string; bases?: Map<number, string> }) {
  const { t } = useTranslation();
  if (items.length === 0) return <Empty>{empty}</Empty>;
  return (
    <div className="overflow-x-auto">
      <table className="tbl">
        <thead>
          <tr className="border-b border-neutral-100 text-left text-xs text-neutral-500">
            <th className="px-5 py-2">{t("knowledge.colTime")}</th>
            {showUser && <th className="px-3 py-2">{t("knowledge.colUser")}</th>}
            <th className="px-3 py-2">{t("knowledge.colTool")}</th>
            <th className="px-3 py-2">{t("knowledge.colQuery")}</th>
            <th className="px-5 py-2 text-right">{t("knowledge.colHits")}</th>
          </tr>
        </thead>
        <tbody>
          {items.map((q) => (
            <tr key={q.id} className="border-b border-neutral-50 last:border-0">
              <td className="px-5 py-2 whitespace-nowrap text-neutral-600">{formatTime(q.created_at)}</td>
              {showUser && <td className="px-3 py-2 text-neutral-800">{q.user || `#${q.user_id}`}</td>}
              <td className="px-3 py-2 text-neutral-700">{t(`knowledge.tools.${q.tool}`)}</td>
              <td className="px-3 py-2 text-neutral-900">
                {q.tool === "read" && q.docs.length > 0 ? (
                  <Link to={kdocPath(q.docs[0])} className="hover:underline">
                    #{q.docs[0]} · {q.query}
                  </Link>
                ) : (
                  q.query || (bases && q.kb_ids.map((id) => bases.get(id) ?? `#${id}`).join("、")) || "—"
                )}
              </td>
              <td className="px-5 py-2 text-right">{q.tool === "search" && q.hits === 0 ? <Badge tone="amber">{t("knowledge.zeroHits")}</Badge> : q.hits}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

/** 在全部能查的库里搜标题和内容，结果按文档聚合；也可以把问题带到 DSH 的「知识问答」里问。 */
function KnowledgeSearch({ qaTitle }: { qaTitle: string }) {
  const { t } = useTranslation();
  const { notify } = useStore();
  const [query, setQuery] = useState("");
  const [result, setResult] = useState<{ query: string; data: KBSearchResult } | null>(null);
  const [error, setError] = useState<string | null>(null);
  const { busy, run } = useAction();
  const ref = useRef<HTMLInputElement>(null);
  // 「/」聚焦搜索框
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const el = e.target as HTMLElement;
      if (e.key === "/" && !e.metaKey && !e.ctrlKey && !["INPUT", "TEXTAREA", "SELECT"].includes(el.tagName) && !el.isContentEditable) {
        e.preventDefault();
        ref.current?.focus();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);
  const search = async () => {
    const q = query.trim();
    if (!q) return;
    setError(null);
    const r = await run(() => api.searchKB({ query: q, mode: "any" }));
    if (r.ok) setResult({ query: q, data: r.value });
    else setError(r.error);
  };
  // 命中按文档聚合，每篇最多列三处
  const groups = new Map<number, { title: string; hits: KBSearchResult["hits"] }>();
  for (const h of result?.data.hits ?? []) {
    const g = groups.get(h.doc) ?? { title: h.title, hits: [] };
    if (g.hits.length < 3) g.hits.push(h);
    groups.set(h.doc, g);
  }
  const named = (result?.data.docs ?? []).filter((d) => !groups.has(d.doc));
  const askInDSH = async () => {
    const text = `/kb-qa ${query.trim()}`;
    if (await copyText(text)) notify(t("knowledge.askCopied", { name: qaTitle }));
    else notify(text);
  };
  return (
    <Card className="mb-6 p-5">
      <form
        className="flex flex-wrap gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          void search();
        }}
      >
        <div className="relative min-w-0 flex-1 basis-full sm:basis-64">
          <Search size={16} className="pointer-events-none absolute top-1/2 left-3 -translate-y-1/2 text-neutral-400" aria-hidden />
          <input
            ref={ref}
            className={cx(inputCls, "h-10 pl-9 text-[15px]")}
            value={query}
            placeholder={t("knowledge.searchPlaceholder")}
            aria-label={t("knowledge.searchPlaceholder")}
            onChange={(e) => setQuery(e.target.value)}
          />
        </div>
        <Button type="submit" size="lg" disabled={busy || !query.trim()}>
          {t("knowledge.searchSubmit")}
        </Button>
        <Button type="button" variant="secondary" size="lg" className="min-w-0 flex-1 sm:flex-none" disabled={!query.trim()} onClick={() => void askInDSH()}>
          <MessageSquare size={15} /> {t("knowledge.askInDSH", { name: qaTitle })}
        </Button>
      </form>
      <p className="mt-2 text-xs text-neutral-500">
        {t("knowledge.searchHint")}{" "}
        <a href="dsh://open" className="inline-flex items-center gap-0.5 text-brand-600 hover:underline dark:text-brand-300">
          {t("knowledge.openDSH")} <ExternalLink size={11} />
        </a>
      </p>
      <ErrorNote>{error}</ErrorNote>
      {result && (
        <div className="mt-4 border-t border-neutral-100 pt-4">
          {groups.size === 0 && named.length === 0 ? (
            <Empty compact icon={SearchX}>{t("knowledge.searchNoHit")}</Empty>
          ) : (
            <ul className="space-y-3">
              {named.map((d) => (
                <li key={`d${d.doc}`}>
                  <Link to={kdocPath(d.doc)} className="font-medium text-brand-600 hover:underline dark:text-brand-300">
                    {d.title}
                  </Link>
                  <span className="ml-2 text-xs text-neutral-500">{t("knowledge.titleMatch")}</span>
                </li>
              ))}
              {[...groups.entries()].map(([doc, g]) => (
                <li key={doc}>
                  <Link to={kdocPath(doc)} className="font-medium text-brand-600 hover:underline dark:text-brand-300">
                    {g.title}
                  </Link>
                  <ul className="mt-1 space-y-1">
                    {g.hits.map((h) => (
                      <li key={`${h.from}-${h.line}`}>
                        <Link to={kdocPath(doc, { from: h.line, to: h.line })} className="block rounded px-2 py-1 text-sm text-neutral-700 hover:bg-neutral-100">
                          {h.heading && <span className="mr-2 text-xs text-neutral-500">{h.heading}</span>}
                          <Snippet text={h.snippet} />
                        </Link>
                      </li>
                    ))}
                  </ul>
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
    </Card>
  );
}

/** 一个知识库都没有时：三步引导，管理员可以直接导入示例。 */
function FirstRun({ onCreate }: { onCreate: () => void }) {
  const { t } = useTranslation();
  const steps = [
    { icon: FolderPlus, title: t("knowledge.guide.step1"), desc: t("knowledge.guide.step1Desc") },
    { icon: Upload, title: t("knowledge.guide.step2"), desc: t("knowledge.guide.step2Desc") },
    { icon: MessageSquare, title: t("knowledge.guide.step3"), desc: t("knowledge.guide.step3Desc") },
  ];
  return (
    <Card className="p-6">
      <h2 className="text-lg font-semibold text-neutral-900">{t("knowledge.guide.title")}</h2>
      <ol className="mt-4 grid gap-4 md:grid-cols-3">
        {steps.map((s, i) => (
          <li key={i} className="rounded-lg border border-neutral-200 bg-neutral-50/50 p-4">
            <div className="flex items-center gap-2 text-sm font-medium text-neutral-900">
              <span className="inline-flex h-6 w-6 items-center justify-center rounded-full bg-brand-50 text-xs font-semibold text-brand-700 dark:bg-brand-500/15 dark:text-brand-200">{i + 1}</span>
              <s.icon size={15} className="text-neutral-500" aria-hidden />
              {s.title}
            </div>
            <p className="mt-2 text-sm text-neutral-600">{s.desc}</p>
          </li>
        ))}
      </ol>
      <div className="mt-5 flex flex-wrap gap-2">
        <Button onClick={onCreate}>
          <Plus size={15} /> {t("knowledge.create")}
        </Button>
        <Link to="/go?setup=1">
          <Button variant="secondary">
            <Download size={15} /> {t("knowledge.guide.importDemo")}
          </Button>
        </Link>
      </div>
    </Card>
  );
}

export default function Knowledge() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const list = useApi(api.knowledge);
  const mine = useApi(() => api.kbQueries());
  const opts = useScopeOptions();
  const [creating, setCreating] = useState(false);
  const items = list.data?.items ?? [];
  const manage = list.data?.can_manage ?? false;
  const formats = (list.data?.formats ?? []).filter((f) => f !== ".markdown").join(" ");

  return (
    <div>
      <PageHeader
        title={t("knowledge.title")}
        actions={
          manage && (
            <Button onClick={() => setCreating(true)}>
              <Plus size={15} /> {t("knowledge.create")}
            </Button>
          )
        }
      />
      <LoadState loading={list.loading && !list.data} error={list.data ? null : list.error} onRetry={list.reload}>
        {items.length === 0 ? (
          manage ? (
            <FirstRun onCreate={() => setCreating(true)} />
          ) : (
            <Card>
              <Empty>{t("knowledge.empty")}</Empty>
            </Card>
          )
        ) : (
          <>
            {items.some((b) => b.readable && b.docs > 0) && <KnowledgeSearch qaTitle={list.data?.qa?.qaTitle || t("knowledge.qaName")} />}
            <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
              {items.map((b) => (
                <Card key={b.id} className={cx("relative flex h-full flex-col p-5", cardHover)}>
                  <div className="flex items-start gap-3">
                    <span className="inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-brand-50 text-brand-600 dark:bg-brand-500/15 dark:text-brand-300">
                      <BookOpen size={18} aria-hidden />
                    </span>
                    <div className="min-w-0 flex-1">
                      <div className="flex flex-wrap items-center gap-2">
                        {/* 整张卡片可点：链接盖住卡片，卡片里的文档链接浮在上面 */}
                        <Link to={kbPath(b.id)} className="text-base font-medium text-neutral-900 after:absolute after:inset-0 after:content-['']">
                          {b.title}
                        </Link>
                        {(b.gaps ?? 0) > 0 && <Badge tone="amber">{t("knowledge.gapsBadge", { count: b.gaps })}</Badge>}
                        {b.can_edit && !b.can_manage && <Badge tone="brand">{t("knowledge.maintainerBadge")}</Badge>}
                        {!b.readable && <Badge tone="amber">{t("knowledge.notReadable")}</Badge>}
                      </div>
                      {b.description && <p className="mt-1 line-clamp-2 text-sm text-neutral-600">{b.description}</p>}
                    </div>
                  </div>
                  {(b.recent?.length ?? 0) > 0 && (
                    <div className="mt-4">
                      <div className="text-xs font-medium text-neutral-500">{t("knowledge.recent")}</div>
                      <ul className="mt-1 space-y-0.5">
                        {b.recent!.map((d) => (
                          <li key={d.id} className="flex items-center justify-between gap-2 text-sm">
                            <Link to={kdocPath(d.id)} className="relative z-10 min-w-0 truncate text-neutral-800 hover:text-brand-600 dark:hover:text-brand-200 hover:underline">
                              {d.title}
                            </Link>
                            <span className="shrink-0 text-xs text-neutral-400">{formatDateOnly(d.updated_at)}</span>
                          </li>
                        ))}
                      </ul>
                    </div>
                  )}
                  <div className="mt-auto pt-4 text-xs text-neutral-500">
                    {t("knowledge.docs", { count: b.docs })} · {t("knowledge.scope")}：{scopeSummary(t, b.scope, opts.roleName)}
                  </div>
                </Card>
              ))}
            </div>
          </>
        )}
        {manage && list.data && (
          <div className="mt-6">
            <QASettings value={list.data.qa} onSaved={list.reload} />
          </div>
        )}
      </LoadState>
      <Card className="mt-6">
        <CardHeader title={t("knowledge.howTitle")} />
        <div className="space-y-1 px-5 pb-5 text-sm text-neutral-700">
          <p>{t("knowledge.how")}</p>
          {formats && <p className="text-neutral-500">{t("knowledge.formats", { list: formats })}</p>}
        </div>
      </Card>
      <Card className="mt-6">
        <details className="group">
          <summary className="flex cursor-pointer list-none items-center gap-1.5 rounded-lg px-5 py-3.5 text-sm font-semibold text-neutral-900 select-none [&::-webkit-details-marker]:hidden">
            <ChevronRight size={15} aria-hidden className="text-neutral-400 transition-transform group-open:rotate-90" />
            {t("knowledge.myQueries")}
          </summary>
          <LoadState loading={mine.loading && !mine.data} error={mine.data ? null : mine.error} onRetry={mine.reload}>
            <QueryTable items={(mine.data ?? []).slice(0, 30)} empty={t("knowledge.myQueriesEmpty")} bases={new Map(items.map((b) => [b.id, b.title]))} />
          </LoadState>
        </details>
      </Card>
      <CreateDialog open={creating} onOpenChange={setCreating} onCreated={(b) => navigate(kbPath(b.id))} />
    </div>
  );
}
