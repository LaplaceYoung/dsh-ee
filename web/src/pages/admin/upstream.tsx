import { useEffect, useMemo, useRef, useState } from "react";
import { FileArchive, Loader2, PackageOpen, RefreshCw, ShieldCheck, WifiOff, CloudOff, TriangleAlert } from "lucide-react";
import { categoryIds, categoryLabel } from "../../lib/catalog";
import { formatNumber } from "../../lib/format";
import { useScoring, useStore } from "../../lib/store";
import { api, formatBytes, formatTime, type ImportResult, type SyncJob } from "../../lib/api";
import { useApi, useAction } from "../../lib/use-api";
import { LoadState } from "../../components/api-ui";
import { StatusBadge } from "../../components/status";
import { Badge, Button, Card, cx, Field, inputCls, PageHeader, Segmented, Toggle, buttonCls, Empty } from "../../components/ui";
import { useTranslation } from "react-i18next";
import { DatePicker } from "@/components/pickers";

type RankBy = "stars" | "score";

const packageName = /^(@[A-Za-z0-9][A-Za-z0-9._~-]*\/)?[A-Za-z0-9_][A-Za-z0-9._~-]*$/;
function isPackageName(name: string): boolean {
  return packageName.test(name) && !name.includes("..");
}

export default function AdminUpstream() {
  const { t } = useTranslation();
  const { settings } = useStore();
  const [tab, setTab] = useState<"online" | "offline">(settings.upstream.mode);
  return (
    <div>
      <PageHeader
        title={t("upstreamAdmin.title")}
        desc={t("upstreamAdmin.desc")}
        actions={
          <Segmented
            value={tab}
            onChange={setTab}
            options={[
              { value: "online", label: t("upstreamAdmin.online") },
              { value: "offline", label: t("upstreamAdmin.offline") },
            ]}
          />
        }
      />
      {tab === "online" ? <Online /> : <Offline />}
    </div>
  );
}

/** 轮询一个任务直到结束。 */
function useJob(onFinish?: () => void) {
  const [job, setJob] = useState<SyncJob | null>(null);
  const finish = useRef(onFinish);
  finish.current = onFinish;
  useEffect(() => {
    if (!job || job.status !== "running") return;
    const timer = setTimeout(() => {
      api.job(job.id).then((j) => {
        setJob(j);
        if (j.status !== "running") finish.current?.();
      }, () => {});
    }, 1200);
    return () => clearTimeout(timer);
  }, [job]);
  return [job, setJob] as const;
}

function Online() {
  const { t } = useTranslation();
  const { settings, notify, reloadPending, reloadPlugins } = useStore();
  const scoring = useScoring();
  const feed = useApi(() => api.upstreamFeed());
  const jobs = useApi(api.jobs);
  const { busy, run } = useAction();
  const [rankByPref, setRankBy] = useState<RankBy>("score");
  const [topN, setTopN] = useState(60);
  const [hideMirrored, setHideMirrored] = useState(true);
  const [cats, setCats] = useState<string[]>([]);
  const [minScore, setMinScore] = useState(scoring.minPreloadScore);
  const [picked, setPicked] = useState<Set<string>>(new Set());
  const [publish, setPublish] = useState(false);
  const [manual, setManual] = useState("");
  const [job, setJob] = useJob(() => {
    feed.reload();
    jobs.reload();
    reloadPending();
    reloadPlugins();
  });

  const rankBy: RankBy = scoring.upstream ? rankByPref : "stars";
  const all = feed.data?.plugins ?? [];
  const ranked = useMemo(
    () =>
      [...all]
        .filter((c) => !cats.length || cats.includes(c.category ?? ""))
        .filter((c) => !scoring.upstream || (c.score ?? 0) >= minScore)
        .sort((a, b) => (rankBy === "score" ? (b.score ?? 0) - (a.score ?? 0) : (b.stars ?? 0) - (a.stars ?? 0)))
        .slice(0, topN),
    [all, cats, scoring.upstream, minScore, rankBy, topN],
  );
  const fresh = ranked.filter((c) => !c.local_version);
  const manualEntries = useMemo(
    () => Array.from(new Set(manual.split(/[\s,]+/).map((s) => s.trim()).filter(Boolean))),
    [manual],
  );
  const manualNames = useMemo(() => manualEntries.filter(isPackageName), [manualEntries]);
  const manualInvalid = manualEntries.length - manualNames.length;
  // 手动输入包名时不要默认把“未入库的全部”也带上；只同步勾选的和手动输入的。
  const selected = picked.size ? ranked.filter((c) => picked.has(c.name)) : manualEntries.length ? [] : fresh;
  const syncNames = useMemo(() => Array.from(new Set([...selected.map((c) => c.name), ...manualNames])), [selected, manualNames]);

  const toggle = (name: string) =>
    setPicked((s) => {
      const n = new Set(s);
      if (n.has(name)) n.delete(name);
      else n.add(name);
      return n;
    });

  const start = async () => {
    if (syncNames.length > 500) return notify(t("upstreamAdmin.tooMany"));
    const r = await run(() => api.syncUpstream(syncNames, publish));
    if (!r.ok) return notify(r.error);
    setJob(r.value);
    setPicked(new Set());
    setManual("");
  };

  const checkUpdates = async () => {
    const r = await run(api.checkUpdates);
    if (!r.ok) return notify(r.error);
    if (r.value) setJob(r.value);
    else notify(t("registry.upstream.noUpdates"));
  };

  return (
    <div className="grid gap-6 xl:grid-cols-[320px_1fr]">
      <div className="space-y-4">
        <Card className="space-y-4 p-5">
          <div className="font-medium text-neutral-800">{t("upstreamAdmin.rules")}</div>
          <Field label={t("upstreamAdmin.rankBy")} hint={!scoring.upstream ? t("upstreamAdmin.scoringOff") : undefined}>
            <Segmented
              value={rankBy}
              onChange={setRankBy}
              options={[
                { value: "stars", label: "GitHub Star" },
                ...(scoring.upstream ? [{ value: "score" as const, label: t("upstreamAdmin.dshfindScore") }] : []),
              ]}
            />
          </Field>
          <Field label={t("upstreamAdmin.topN", { n: topN })}>
            <input type="range" min={10} max={300} step={10} value={topN} onChange={(e) => setTopN(+e.target.value)} className="w-full accent-brand-500" />
          </Field>
          {scoring.upstream && (
            <Field label={t("upstreamAdmin.minScore", { n: minScore })}>
              <input type="range" min={0} max={90} step={5} value={minScore} onChange={(e) => setMinScore(+e.target.value)} className="w-full accent-brand-500" />
            </Field>
          )}
          <Field label={t("upstreamAdmin.categories")}>
            <div className="flex flex-wrap gap-1.5">
              {categoryIds.map((id) => (
                <button
                  key={id}
                  onClick={() => setCats((c) => (c.includes(id) ? c.filter((x) => x !== id) : [...c, id]))}
                  className={cx("rounded-md border px-2 py-1 text-xs", cats.includes(id) ? "border-brand-300 bg-brand-50 text-brand-700" : "border-neutral-200 text-neutral-600")}
                >
                  {categoryLabel(id)}
                </button>
              ))}
            </div>
          </Field>
          <div className="flex items-center justify-between text-sm">
            <span className="text-neutral-700">{t("upstreamAdmin.hideMirrored")}</span>
            <Toggle checked={hideMirrored} onChange={setHideMirrored} />
          </div>
          <div className="flex items-center justify-between gap-3 text-sm">
            <span className="text-neutral-700">{t("registry.upstream.publishDirectly")}</span>
            <Toggle checked={publish} onChange={setPublish} />
          </div>
          {publish && <p className="text-xs text-amber-700">{t("registry.upstream.publishDirectlyHint")}</p>}
        </Card>

        <Card className="space-y-2 p-5 text-sm">
          <div className="font-medium text-neutral-800">{t("upstreamAdmin.config")}</div>
          <div className="flex justify-between text-neutral-600">
            <span>{t("upstreamAdmin.catalog")}</span>
            <span>{settings.upstream.dshfindFeed ? "dshfind.com" : t("upstreamAdmin.notEnabled")}</span>
          </div>
          <div className="flex justify-between text-neutral-600">
            <span>{t("upstreamAdmin.npmSource")}</span>
            <span className="truncate pl-3">{settings.upstream.registry === "custom" ? settings.upstream.customRegistry || t("upstreamAdmin.custom") : settings.upstream.registry}</span>
          </div>
          <div className="flex justify-between text-neutral-600">
            <span>{t("upstreamAdmin.newVersions")}</span>
            <span>{{ auto: t("upstreamAdmin.syncAuto"), review: t("upstreamAdmin.syncReview"), off: t("upstreamAdmin.syncOff") }[settings.upstream.autoSync]}</span>
          </div>
          <Button variant="secondary" size="sm" className="mt-2 w-full" disabled={busy || settings.upstream.autoSync === "off"} onClick={checkUpdates}>
            {t("registry.upstream.checkUpdates")}
          </Button>
          <p className="pt-1 text-xs text-neutral-500">{t("registry.upstream.closureNote")}</p>
        </Card>
      </div>

      <div className="space-y-4">
        {job && <JobCard job={job} />}
        <Card className="space-y-3 p-4">
          <Field label={t("upstreamAdmin.manualNames")} hint={t("upstreamAdmin.manualNamesHint")}>
            <textarea
              className={cx(inputCls, "h-20 py-2 font-mono")}
              value={manual}
              onChange={(e) => setManual(e.target.value)}
              placeholder={"dsh-clinic\n@perrylink/dsh-claim-check"}
            />
          </Field>
          {manualInvalid > 0 && <p className="text-xs text-amber-700">{t("upstreamAdmin.manualInvalid", { count: manualInvalid })}</p>}
        </Card>
        <Card className="flex flex-wrap items-center gap-4 p-4">
          <div className="text-sm">
            <span className="font-medium text-neutral-800">
              {t("upstreamAdmin.willSync", { count: syncNames.length })}
              {picked.size && manualEntries.length
                ? t("upstreamAdmin.manualAndPasted")
                : picked.size
                  ? t("upstreamAdmin.manual")
                  : manualEntries.length
                    ? t("upstreamAdmin.manualOnly")
                    : t("upstreamAdmin.allUnmirrored")}
            </span>
            <span className="ml-2 text-neutral-500">{t("registry.upstream.feedCount", { count: all.length, at: feed.data?.fetched_at ? formatTime(feed.data.fetched_at) : "—" })}</span>
          </div>
          <div className="ml-auto flex flex-wrap items-center gap-2">
            <Button variant="secondary" size="sm" disabled={feed.loading} onClick={() => api.upstreamFeed(true).then(() => feed.reload())}>
              <RefreshCw size={14} /> {t("registry.upstream.refreshFeed")}
            </Button>
            <Button disabled={!syncNames.length || busy || job?.status === "running"} onClick={start}>
              {t("upstreamAdmin.startSync")}
            </Button>
          </div>
        </Card>

        <LoadState loading={feed.loading && !feed.data} error={feed.error} onRetry={feed.reload}>
          {feed.data?.disabled || feed.data?.error ? (
            <Card>
              <Empty icon={feed.data.disabled ? CloudOff : TriangleAlert}>
                {feed.data.disabled ? t("registry.upstream.feedDisabled") : t("registry.upstream.feedError", { msg: feed.data.error })}
              </Empty>
            </Card>
          ) : (
            <Card className="overflow-x-auto">
              <table className="tbl max-sm:min-w-[36rem]">
                <thead>
                  <tr>
                    <th className="w-10 px-4 py-2.5" />
                    <th className="px-4 py-2.5">{t("upstreamAdmin.colPlugin")}</th>
                    <th className="px-4 py-2.5">{t("upstreamAdmin.colCategory")}</th>
                    <th className="px-4 py-2.5 text-right">Star</th>
                    {scoring.upstream && <th className="px-4 py-2.5 text-right">{t("upstreamAdmin.colScore")}</th>}
                    <th className="px-4 py-2.5">{t("upstreamAdmin.colDeps")}</th>
                    <th className="px-4 py-2.5">{t("upstreamAdmin.colStatus")}</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-neutral-100">
                  {ranked
                    .filter((c) => !hideMirrored || !c.local_version)
                    .map((c) => (
                      <tr key={c.name} className={cx(c.local_version && "text-neutral-500")}>
                        <td className="px-4 py-2">
                          <input
                            type="checkbox"
                            aria-label={c.name}
                            checked={picked.has(c.name)}
                            onChange={() => toggle(c.name)}
                            className="accent-brand-500"
                          />
                        </td>
                        <td className="px-4 py-2">
                          <div className="font-medium">{c.title || c.name}</div>
                          <div className="font-mono text-xs text-neutral-500">
                            {c.name}
                            {c.version && <span>@{c.version}</span>}
                          </div>
                        </td>
                        <td className="px-4 py-2">{categoryLabel(c.category ?? "") ?? "—"}</td>
                        <td className="px-4 py-2 text-right tabular-nums">{formatNumber(c.stars ?? 0)}</td>
                        {scoring.upstream && <td className="px-4 py-2 text-right tabular-nums">{c.score ?? "—"}</td>}
                        <td className="px-4 py-2 text-xs">
                          {c.dep_count !== null ? t("upstreamAdmin.depsValue", { count: c.dep_count, size: formatBytes(c.closure_size ?? 0) }) : "—"}
                          {c.offline_ready === false && (
                            <span className="ml-1 inline-flex items-center gap-0.5 text-amber-600">
                              <WifiOff size={11} /> {t("upstreamAdmin.needsInternet")}
                            </span>
                          )}
                        </td>
                        <td className="px-4 py-2">
                          {c.local_status ? (
                            <span className="inline-flex items-center gap-1.5">
                              <StatusBadge status={c.local_status} />
                              {c.version && c.local_version !== c.version && <Badge tone="amber">{t("registry.upstream.newer", { version: c.version })}</Badge>}
                            </span>
                          ) : (
                            <Badge>{t("upstreamAdmin.notMirrored")}</Badge>
                          )}
                        </td>
                      </tr>
                    ))}
                </tbody>
              </table>
            </Card>
          )}
        </LoadState>
        <JobHistory jobs={jobs.data ?? []} onPick={setJob} />
      </div>
    </div>
  );
}

function JobCard({ job }: { job: SyncJob }) {
  const { t } = useTranslation();
  const pct = job.total ? Math.round((job.done / job.total) * 100) : 0;
  const running = job.status === "running";
  return (
    <Card className="p-4">
      <div className="flex items-center gap-3 text-sm">
        {running && <Loader2 size={16} className="animate-spin text-brand-500" />}
        <span className="font-medium text-neutral-800">{t(`registry.jobs.kind.${job.kind}`)}</span>
        <JobStatus job={job} />
        <span className="text-neutral-500">
          {job.done} / {job.total} · {formatBytes(job.bytes)}
        </span>
      </div>
      {running && (
        <div className="mt-2 h-2 rounded-full bg-neutral-100">
          <div className="h-2 rounded-full bg-brand-500 transition-all" style={{ width: `${Math.max(pct, 3)}%` }} />
        </div>
      )}
      {job.log && <pre className="mt-3 max-h-56 overflow-auto rounded-md bg-neutral-50 p-3 font-mono text-xs whitespace-pre-wrap text-neutral-700">{job.log}</pre>}
    </Card>
  );
}

function JobStatus({ job }: { job: SyncJob }) {
  const { t } = useTranslation();
  const tone = { running: "brand", succeeded: "green", partial: "amber", failed: "red", interrupted: "slate" } as const;
  return <Badge tone={tone[job.status]}>{t(`registry.jobs.status.${job.status}`)}</Badge>;
}

function JobHistory({ jobs, onPick, kinds }: { jobs: SyncJob[]; onPick: (j: SyncJob) => void; kinds?: SyncJob["kind"][] }) {
  const { t } = useTranslation();
  const list = jobs.filter((j) => !kinds || kinds.includes(j.kind));
  if (list.length === 0) return null;
  return (
    <Card>
      <div className="border-b border-neutral-100 px-5 py-3 text-[15px] leading-6 font-semibold text-neutral-900">{t("upstreamAdmin.history")}</div>
      <div className="overflow-x-auto"><table className="tbl max-sm:min-w-[36rem]">
        <thead>
          <tr>
            <th className="px-5 py-2">{t("registry.jobs.colKind")}</th>
            <th className="px-5 py-2">{t("upstreamAdmin.colResult")}</th>
            <th className="px-5 py-2">{t("registry.jobs.colCount")}</th>
            <th className="px-5 py-2">{t("upstreamAdmin.colSize")}</th>
            <th className="px-5 py-2">{t("registry.jobs.colStarted")}</th>
            <th className="px-5 py-2" />
          </tr>
        </thead>
        <tbody className="divide-y divide-neutral-100">
          {list.slice(0, 10).map((j) => (
            <tr key={j.id}>
              <td className="px-5 py-2.5">{t(`registry.jobs.kind.${j.kind}`)}</td>
              <td className="px-5 py-2.5">
                <JobStatus job={j} />
              </td>
              <td className="px-5 py-2.5 tabular-nums">
                {j.done - j.failed} / {j.total}
              </td>
              <td className="px-5 py-2.5">{formatBytes(j.bytes)}</td>
              <td className="px-5 py-2.5 text-neutral-500 tabular-nums">{formatTime(j.created_at)}</td>
              <td className="px-5 py-2.5 text-right">
                <button type="button" className="text-sm text-neutral-600 hover:text-neutral-900" onClick={() => onPick(j)}>
                  {t("registry.jobs.viewLog")}
                </button>
              </td>
            </tr>
          ))}
        </tbody>
      </table></div>
    </Card>
  );
}

function Offline() {
  const { t } = useTranslation();
  const { notify, reloadPending, reloadPlugins } = useStore();
  const jobs = useApi(api.jobs);
  const { busy, run } = useAction();
  const [file, setFile] = useState<File | null>(null);
  const [exportSince, setExportSince] = useState("");
  const [result, setResult] = useState<ImportResult | null>(null);
  const [job, setJob] = useState<SyncJob | null>(null);

  const doImport = async () => {
    if (!file) return;
    const r = await run(() => api.importBundle(file));
    if (!r.ok) return notify(r.error);
    setJob(r.value.job);
    if (r.value.error) notify(r.value.error);
    if (r.value.result) {
      setResult(r.value.result);
      notify(t("upstreamAdmin.imported"));
    }
    setFile(null);
    jobs.reload();
    reloadPending();
    reloadPlugins();
  };

  return (
    <div className="space-y-6">
      <div className="grid gap-6 lg:grid-cols-2">
        <Card className="p-6">
          <div className="mb-1 flex items-center gap-2 font-medium text-neutral-800">
            <PackageOpen size={18} /> {t("upstreamAdmin.importSeed")}
          </div>
          <p className="mb-4 text-sm text-neutral-500">{t("registry.upstream.importDesc")}</p>
          <label className="flex cursor-pointer flex-col items-center rounded-xl border-2 border-dashed border-neutral-200 px-6 py-8 text-center hover:border-brand-300">
            <FileArchive size={28} className="text-brand-500" />
            <span className="mt-2 text-sm font-medium text-neutral-700">{file ? `${file.name} · ${formatBytes(file.size)}` : t("upstreamAdmin.chooseFile")}</span>
            <input type="file" accept=".tar" className="hidden" onChange={(e) => setFile(e.target.files?.[0] ?? null)} />
          </label>
          <Button className="mt-4" disabled={!file || busy} onClick={doImport}>
            {busy ? t("registry.upstream.importing") : t("upstreamAdmin.startImport")}
          </Button>
          {result && (
            <p className="mt-3 text-sm text-neutral-700">
              {t("registry.upstream.importResult", { added: result.added, pending: result.pending, existing: result.existing, skipped: result.skipped.length })}
            </p>
          )}
        </Card>

        <Card className="p-6">
          <div className="mb-1 flex items-center gap-2 font-medium text-neutral-800">
            <ShieldCheck size={18} /> {t("upstreamAdmin.exportDelta")}
          </div>
          <p className="mb-4 text-sm text-neutral-500">{t("upstreamAdmin.exportDesc")}</p>
          <Field label={t("upstreamAdmin.exportSince")} hint={t("registry.upstream.exportAllHint")}>
            <DatePicker value={exportSince} onChange={setExportSince} />
          </Field>
          <a
            href={api.exportUrl(exportSince || undefined)}
            className={buttonCls("secondary", "md", "mt-4")}
          >
            {t("upstreamAdmin.generate")}
          </a>
        </Card>
      </div>
      {job && <JobCard job={job} />}
      <JobHistory jobs={jobs.data ?? []} onPick={setJob} kinds={["import"]} />
    </div>
  );
}
