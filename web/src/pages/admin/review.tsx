import { useState } from "react";
import { Download } from "lucide-react";
import { categoryLabel } from "../../lib/catalog";
import i18n from "../../i18n";
import { api, formatBytes, type AssetReviewItem, type AssetSpec, type ReviewItem, type ScopeReviewItem } from "../../lib/api";
import { KIND_ICONS, scopeSummary } from "../../lib/hub";
import { useScopeOptions } from "../../components/scope-dialog";
import { useApi, useAction } from "../../lib/use-api";
import { useScoring, useStore } from "../../lib/store";
import { LoadState } from "../../components/api-ui";
import { ScanList, scanWarnings } from "../../components/scan-list";
import { Badge, Button, Card, cx, inputCls, PageHeader, PluginIcon, ScoreBadge, Segmented, Empty } from "../../components/ui";
import { useTranslation } from "react-i18next";

function ago(iso: string) {
  const h = Math.max(1, Math.round((Date.now() - new Date(iso).getTime()) / 3_600_000));
  return h < 24 ? i18n.t("review.hoursAgo", { n: h }) : i18n.t("review.daysAgo", { n: Math.round(h / 24) });
}

const key = (v: ReviewItem) => `${v.name}@${v.version}`;

export default function AdminReview() {
  const { t } = useTranslation();
  const plugins = useApi(api.reviewQueue);
  const hub = useApi(api.hubReview);
  const assets = { ...hub, data: hub.data?.items ?? null } as ReturnType<typeof useApi<AssetReviewItem[]>>;
  const scopes = { ...hub, data: hub.data?.scopes ?? null } as ReturnType<typeof useApi<ScopeReviewItem[]>>;
  const [tab, setTab] = useState<"plugins" | "assets" | "scopes">("plugins");
  // 插件队列空时，默认看有内容的那一队
  const effective =
    tab === "plugins" && plugins.data?.length === 0
      ? (assets.data?.length ?? 0) > 0
        ? "assets"
        : (scopes.data?.length ?? 0) > 0
          ? "scopes"
          : tab
      : tab;
  return (
    <div>
      <PageHeader
        title={t("review.title")}
        desc={t("review.desc")}
        actions={
          <Segmented
            value={effective}
            onChange={setTab}
            options={[
              { value: "plugins", label: `${t("reviewAssets.plugins")} ${plugins.data?.length ?? ""}` },
              { value: "assets", label: `${t("reviewAssets.assets")} ${assets.data?.length ?? ""}` },
              { value: "scopes", label: `${t("scopeReview.tab")} ${scopes.data?.length ?? ""}` },
            ]}
          />
        }
      />
      {effective === "plugins" ? <PluginReview q={plugins} /> : effective === "assets" ? <AssetReview q={assets} /> : <ScopeReview q={scopes} />}
    </div>
  );
}

function SpecView({ spec }: { spec: AssetSpec }) {
  const { t } = useTranslation();
  return (
    <div className="space-y-3 text-sm">
      <div>
        <div className="font-medium text-neutral-900">{spec.title}</div>
        <div className="text-neutral-700">{spec.description}</div>
      </div>
      {spec.when_to_use && (
        <div>
          <span className="text-neutral-500">{t("asset.whenToUse")}：</span>
          {spec.when_to_use}
        </div>
      )}
      {(spec.body || spec.persona) && (
        <pre className="max-h-96 overflow-auto rounded-md bg-neutral-50 p-3 text-sm whitespace-pre-wrap text-neutral-800">{spec.body || spec.persona}</pre>
      )}
      {(spec.skills?.length || spec.connections?.length || spec.plugins?.length) ? (
        <div className="flex flex-wrap gap-1.5">
          {[...(spec.skills ?? []), ...(spec.connections ?? []), ...(spec.plugins ?? [])].map((x) => (
            <Badge key={x}>{x}</Badge>
          ))}
        </div>
      ) : null}
    </div>
  );
}

function AssetReview({ q }: { q: ReturnType<typeof useApi<AssetReviewItem[]>> }) {
  const { t } = useTranslation();
  const { notify, reloadPending } = useStore();
  const { busy, run } = useAction();
  const [selected, setSelected] = useState<string | null>(null);
  const [reason, setReason] = useState("");
  const queue = q.data ?? [];
  const k = (v: AssetReviewItem) => `${v.asset.id}#${v.version}`;
  const current = queue.find((v) => k(v) === selected) ?? queue[0];

  const decide = async (approve: boolean) => {
    if (!current) return;
    const r = await run(() => api.reviewAsset({ asset_id: current.asset.id, version: current.version, action: approve ? "approve" : "reject", note: reason.trim() }));
    if (!r.ok) return notify(r.error);
    notify(t(approve ? "reviewAssets.approved" : "reviewAssets.rejected", { title: current.spec.title, version: current.version }));
    setSelected(null);
    setReason("");
    q.reload();
    reloadPending();
  };

  return (
    <LoadState loading={q.loading && !q.data} error={q.error} onRetry={q.reload}>
      {queue.length === 0 ? (
        <Card><Empty>{t("reviewAssets.empty")}</Empty></Card>
      ) : (
        <div className="grid gap-6 lg:grid-cols-[340px_1fr]">
          <Card className="h-fit divide-y divide-neutral-100">
            {queue.map((v) => {
              const Icon = KIND_ICONS[v.asset.kind];
              return (
                <button
                  key={k(v)}
                  onClick={() => setSelected(k(v))}
                  className={cx("flex w-full gap-3 px-4 py-3 text-left", current && k(current) === k(v) ? "bg-neutral-100" : "hover:bg-neutral-50")}
                >
                  <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-neutral-100 text-neutral-700">
                    <Icon size={16} />
                  </span>
                  <div className="min-w-0 flex-1">
                    <div className="truncate text-sm font-medium text-neutral-800">{v.spec.title}</div>
                    <div className="truncate font-mono text-xs text-neutral-500">
                      /{v.asset.slug} · v{v.version}
                    </div>
                    <div className="mt-1 flex flex-wrap gap-1.5">
                      <Badge>{t(`assetKinds.${v.asset.kind}`)}</Badge>
                      {v.previous_version === 0 && <Badge tone="brand">{t("reviewAssets.firstVersion")}</Badge>}
                    </div>
                  </div>
                  <span className="text-xs whitespace-nowrap text-neutral-500">{ago(v.created_at)}</span>
                </button>
              );
            })}
          </Card>
          {current && (
            <Card className="p-6">
              <div className="mb-4 flex flex-wrap items-center gap-2">
                <h2 className="text-xl font-semibold text-neutral-900">{current.spec.title}</h2>
                <Badge>{t(`assetKinds.${current.asset.kind}`)}</Badge>
                <span className="text-sm text-neutral-600">
                  {t("reviewAssets.submittedBy", { name: current.publisher_name || current.publisher || "—", department: current.department || "—" })}
                </span>
              </div>
              <h3 className="mb-2 text-sm font-semibold text-neutral-900">{t("reviewAssets.current")}</h3>
              <SpecView spec={current.spec} />
              {current.previous && (
                <>
                  <h3 className="mt-6 mb-2 text-sm font-semibold text-neutral-900">
                    {t("reviewAssets.previous")} · {t("reviewAssets.compare", { n: current.previous_version })}
                  </h3>
                  <div className="opacity-75">
                    <SpecView spec={current.previous} />
                  </div>
                </>
              )}
              <div className="mt-6">
                <textarea className={cx(inputCls, "h-20 py-2")} placeholder={t("review.commentPlaceholder")} value={reason} onChange={(e) => setReason(e.target.value)} />
                <div className="mt-3 flex gap-2">
                  <Button disabled={busy} onClick={() => decide(true)}>
                    {t("review.approve")}
                  </Button>
                  <Button variant="danger" disabled={busy || !reason.trim()} onClick={() => decide(false)}>
                    {t("review.reject")}
                  </Button>
                </div>
              </div>
            </Card>
          )}
        </div>
      )}
    </LoadState>
  );
}

/** 负责人把「自己用」的能力分享出去（扩大可用范围）的申请。 */
function ScopeReview({ q }: { q: ReturnType<typeof useApi<ScopeReviewItem[]>> }) {
  const { t } = useTranslation();
  const { notify, reloadPending } = useStore();
  const opts = useScopeOptions();
  const { busy, run } = useAction();
  const [selected, setSelected] = useState<number | null>(null);
  const [reason, setReason] = useState("");
  const queue = q.data ?? [];
  const current = queue.find((v) => v.asset.id === selected) ?? queue[0];

  const decide = async (approve: boolean) => {
    if (!current) return;
    // 带上评审时看到的申请快照：期间负责人换成新申请时服务端返回 409，不会误批没看过的范围。
    const r = await run(() =>
      api.reviewScope({ asset_id: current.asset.id, scope_requested_at: current.asset.scope_requested_at ?? current.created_at ?? "", action: approve ? "approve" : "reject", note: reason.trim() }),
    );
    if (!r.ok) return notify(r.error);
    notify(t(approve ? "scopeReview.approved" : "scopeReview.rejected", { title: current.spec.title }));
    setSelected(null);
    setReason("");
    q.reload();
    reloadPending();
  };

  return (
    <LoadState loading={q.loading && !q.data} error={q.error} onRetry={q.reload}>
      {queue.length === 0 ? (
        <Card><Empty>{t("scopeReview.empty")}</Empty></Card>
      ) : (
        <div className="grid gap-6 lg:grid-cols-[340px_1fr]">
          <Card className="h-fit divide-y divide-neutral-100">
            {queue.map((v) => {
              const Icon = KIND_ICONS[v.asset.kind];
              return (
                <button
                  key={v.asset.id}
                  onClick={() => setSelected(v.asset.id)}
                  className={cx("flex w-full gap-3 px-4 py-3 text-left", current?.asset.id === v.asset.id ? "bg-neutral-100" : "hover:bg-neutral-50")}
                >
                  <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-neutral-100 text-neutral-700">
                    <Icon size={16} />
                  </span>
                  <div className="min-w-0 flex-1">
                    <div className="truncate text-sm font-medium text-neutral-800">{v.spec.title}</div>
                    <div className="truncate text-xs text-neutral-500">{scopeSummary(t, v.requested, opts.roleName)}</div>
                  </div>
                  {v.created_at && <span className="text-xs whitespace-nowrap text-neutral-500">{ago(v.created_at)}</span>}
                </button>
              );
            })}
          </Card>
          {current && (
            <Card className="p-6">
              <div className="mb-4 flex flex-wrap items-center gap-2">
                <h2 className="text-xl font-semibold text-neutral-900">{current.spec.title}</h2>
                <Badge>{t(`assetKinds.${current.asset.kind}`)}</Badge>
                <span className="text-sm text-neutral-600">
                  {t("reviewAssets.submittedBy", { name: current.publisher_name || current.publisher || "—", department: current.department || "—" })}
                </span>
              </div>
              <dl className="mb-5 grid gap-3 text-sm sm:grid-cols-2">
                <div className="rounded-lg border border-neutral-100 px-3 py-2">
                  <dt className="text-xs text-neutral-500">{t("scopeReview.from")}</dt>
                  <dd className="text-neutral-900">{scopeSummary(t, current.scope, opts.roleName)}</dd>
                </div>
                <div className="rounded-lg border border-amber-200 bg-amber-50/50 px-3 py-2 dark:border-amber-400/30 dark:bg-amber-400/10">
                  <dt className="text-xs text-neutral-500">{t("scopeReview.to")}</dt>
                  <dd className="font-medium text-neutral-900">{scopeSummary(t, current.requested, opts.roleName)}</dd>
                </div>
              </dl>
              <h3 className="mb-2 text-sm font-semibold text-neutral-900">{t("reviewAssets.previous")}</h3>
              <SpecView spec={current.spec} />
              <p className="mt-4 text-xs text-neutral-500">{t("scopeReview.hint")}</p>
              <div className="mt-4">
                <textarea className={cx(inputCls, "h-20 py-2")} placeholder={t("review.commentPlaceholder")} value={reason} onChange={(e) => setReason(e.target.value)} />
                <div className="mt-3 flex gap-2">
                  <Button disabled={busy} onClick={() => decide(true)}>
                    {t("review.approve")}
                  </Button>
                  <Button variant="danger" disabled={busy || !reason.trim()} onClick={() => decide(false)}>
                    {t("review.reject")}
                  </Button>
                </div>
              </div>
            </Card>
          )}
        </div>
      )}
    </LoadState>
  );
}

function PluginReview({ q }: { q: ReturnType<typeof useApi<ReviewItem[]>> }) {
  const { t } = useTranslation();
  const { notify, reloadPending, reloadPlugins } = useStore();
  const scoring = useScoring();
  const { data, loading, error, reload } = q;
  const { busy, run } = useAction();
  const [selected, setSelected] = useState<string | null>(null);
  const [reason, setReason] = useState("");
  const queue = data ?? [];
  const current = queue.find((v) => key(v) === selected) ?? queue[0];

  const decide = async (approve: boolean) => {
    if (!current) return;
    const r = await run(() => api.review({ name: current.name, version: current.version, action: approve ? "approve" : "reject", note: reason.trim() }));
    if (!r.ok) return notify(r.error);
    notify(approve ? t("review.approvedMsg", { name: key(current) }) : t("review.rejectedMsg", { name: key(current) }));
    setSelected(null);
    setReason("");
    reload();
    reloadPending();
    reloadPlugins();
  };

  return (
    <div>
      <LoadState loading={loading && !data} error={error} onRetry={reload}>
        {queue.length === 0 ? (
          <Card><Empty>{t("review.empty")}</Empty></Card>
        ) : (
          <div className="grid gap-6 lg:grid-cols-[340px_1fr]">
            <Card className="h-fit divide-y divide-neutral-100">
              {queue.map((v) => {
                const warns = scanWarnings(v.scan);
                return (
                  <button
                    key={key(v)}
                    onClick={() => setSelected(key(v))}
                    className={cx("flex w-full gap-3 px-4 py-3 text-left", current && key(current) === key(v) ? "bg-neutral-100" : "hover:bg-neutral-50")}
                  >
                    <PluginIcon name={v.name} icon={v.package.icon || null} size={32} />
                    <div className="min-w-0 flex-1">
                      <div className="truncate text-sm font-medium text-neutral-800">{v.package.title || v.name}</div>
                      <div className="truncate font-mono text-xs text-neutral-500">{key(v)}</div>
                      <div className="mt-1 flex flex-wrap gap-1.5">
                        {v.package.origin === "internal" ? (
                          <Badge tone="brand">{t("review.internalSubmission")}</Badge>
                        ) : v.publisher ? (
                          <Badge tone="brand">{t("registry.review.requested")}</Badge>
                        ) : (
                          <Badge>{t("review.upstreamVersion")}</Badge>
                        )}
                        {warns > 0 && <Badge tone="amber">{t("review.warnings", { count: warns })}</Badge>}
                      </div>
                    </div>
                    <span className="text-xs whitespace-nowrap text-neutral-500">{ago(v.created_at)}</span>
                  </button>
                );
              })}
            </Card>

            {current && (
              <Card className="p-6">
                <div className="flex gap-4">
                  <PluginIcon name={current.name} icon={current.package.icon || null} size={52} />
                  <div className="min-w-0">
                    <h2 className="text-xl font-semibold text-neutral-900">{current.package.title || current.name}</h2>
                    <div className="font-mono text-sm text-neutral-500">{key(current)}</div>
                    <div className="mt-2 flex flex-wrap gap-1.5">
                      {current.package.category && <Badge>{categoryLabel(current.package.category) ?? current.package.category}</Badge>}
                      {current.publisher && (
                        <Badge>
                          {current.package.origin === "internal"
                            ? t("registry.review.publishedBy", { name: current.publisher_name || current.publisher, department: current.department || "—" })
                            : t("registry.review.requestedBy", { name: current.publisher_name || current.publisher })}
                        </Badge>
                      )}
                      {current.package.origin === "upstream" && current.package.author && <Badge>{t("review.author", { name: current.package.author })}</Badge>}
                      {current.previous ? (
                        <Badge tone="slate">{t("registry.review.previous", { version: current.previous })}</Badge>
                      ) : (
                        <Badge tone="brand">{t("registry.review.firstVersion")}</Badge>
                      )}
                      {scoring.upstream && current.package.score !== null && <ScoreBadge score={current.package.score} />}
                    </div>
                  </div>
                </div>
                {current.package.description && <p className="mt-4 text-sm text-neutral-600">{current.package.description}</p>}

                <h3 className="mt-6 mb-2 text-sm font-semibold text-neutral-900">{t("review.autoChecks")}</h3>
                <ScanList items={current.scan} />

                <div className="mt-4 grid gap-3 text-sm sm:grid-cols-2">
                  <KV title={t("registry.review.dependencies")} map={current.dependencies} />
                  <KV title={t("registry.review.peers")} map={current.peer_dependencies} />
                  <KV title={t("registry.review.engines")} map={current.engines} />
                  <KV title={t("registry.review.scripts")} map={current.scripts} />
                </div>
                <div className="mt-3 flex items-center gap-3 text-xs text-neutral-500">
                  <span>
                    {formatBytes(current.size)} · <span className="font-mono">{current.integrity.slice(0, 24)}…</span>
                  </span>
                  <a href={api.tarballUrl(current.name, current.version)} className="inline-flex items-center gap-1 text-brand-600 dark:text-brand-300 hover:underline">
                    <Download size={12} /> {t("registry.review.downloadTarball")}
                  </a>
                </div>

                <div className="mt-6">
                  <textarea
                    className={cx(inputCls, "h-20 py-2")}
                    placeholder={t("review.commentPlaceholder")}
                    value={reason}
                    onChange={(e) => setReason(e.target.value)}
                  />
                  <div className="mt-3 flex gap-2">
                    <Button disabled={busy} onClick={() => decide(true)}>
                      {t("review.approve")}
                    </Button>
                    <Button variant="danger" disabled={busy || !reason.trim()} onClick={() => decide(false)}>
                      {t("review.reject")}
                    </Button>
                  </div>
                </div>
              </Card>
            )}
          </div>
        )}
      </LoadState>
    </div>
  );
}

function KV({ title, map }: { title: string; map: Record<string, string> | null }) {
  const entries = Object.entries(map ?? {});
  return (
    <div className="rounded-lg border border-neutral-100 px-3 py-2">
      <div className="mb-1 text-xs text-neutral-500">{title}</div>
      {entries.length === 0 ? (
        <div className="text-xs text-neutral-400">—</div>
      ) : (
        <ul className="space-y-0.5 font-mono text-xs text-neutral-700">
          {entries.slice(0, 12).map(([k, v]) => (
            <li key={k} className="truncate">
              {k} <span className="text-neutral-500">{v}</span>
            </li>
          ))}
          {entries.length > 12 && <li className="text-neutral-500">… +{entries.length - 12}</li>}
        </ul>
      )}
    </div>
  );
}
