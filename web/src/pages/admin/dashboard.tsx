import { useState } from "react";
import { Link } from "react-router";
import { ArrowRight, FileText } from "lucide-react";
import { kindOf } from "../../lib/catalog";
import { formatNumber } from "../../lib/format";
import { useStore } from "../../lib/store";
import { useAuth } from "../../lib/auth";
import type { Plugin, PluginKind } from "../../lib/types";
import { api, formatTime, formatTokens, toPlugin } from "../../lib/api";
import { useApi } from "../../lib/use-api";
import { auditAction, auditActor, auditTone } from "../../lib/audit";
import { platformLabel } from "../../lib/labels";
import { BarList, DailyBars } from "../../components/charts";
import { pluginPath } from "../../components/plugin-card";
import { LoadState } from "../../components/api-ui";
import { Badge, Card, CardHeader, PageHeader, Segmented, buttonCls } from "../../components/ui";
import { PageLink } from "../../lib/menus";
import { useTranslation } from "react-i18next";

const kindOrder: PluginKind[] = ["skill", "connector", "knowledge", "plugin"];

/** 链路面板：标题、领头数字、一句说明，下面放明细。 */
function ChainPanel({
  step,
  lead,
  unit,
  note,
  children,
}: {
  step: string;
  lead: string;
  unit: string;
  note: React.ReactNode;
  children: React.ReactNode;
}) {
  return (
    <Card className="flex flex-col p-5">
      <h2 className="text-neutral-900 text-[15px] leading-6 font-semibold">{step}</h2>
      <div className="mt-3 flex items-baseline gap-1.5">
        <span className="text-3xl font-medium tracking-tight text-neutral-900 tabular-nums">{lead}</span>
        <span className="text-sm text-neutral-600">{unit}</span>
      </div>
      <p className="mt-1 text-sm text-neutral-600">{note}</p>
      <div className="mt-5 flex-1">{children}</div>
    </Card>
  );
}

function KeyValue({ rows }: { rows: [string, React.ReactNode][] }) {
  return (
    <dl className="divide-y divide-neutral-100 text-sm">
      {rows.map(([k, v]) => (
        <div key={k} className="flex items-baseline justify-between py-2 first:pt-0">
          <dt className="text-neutral-600">{k}</dt>
          <dd className="text-neutral-900 tabular-nums">{v}</dd>
        </div>
      ))}
    </dl>
  );
}

const DAYS = 14;

export default function AdminDashboard() {
  const { t } = useTranslation();
  const { settings } = useStore();
  const { can } = useAuth();
  const canAudit = can("audit.view");
  const [usageGroup, setUsageGroup] = useState<"user" | "token">("user");
  const stats = useApi(() => api.stats(DAYS, "token"));
  const audit = useApi(() => (canAudit ? api.audit({ limit: 6 }) : Promise.resolve([])));
  // 全部状态的能力（能力市场的列表只有已上架的，算不出待审核、驳回与下架）
  const packages = useApi(api.adminPackages);
  const s = stats.data;
  const plugins = (packages.data ?? []).map(toPlugin);
  const pendingVersions = (packages.data ?? []).reduce((n, p) => n + p.pending, 0);

  const approved = plugins.filter((p) => p.status === "published");
  const count = (st: string) => plugins.filter((p) => p.status === st).length;
  const byKind = kindOrder.map((k) => ({ label: t(`kinds.${k}`), value: approved.filter((p) => kindOf(p) === k).length }));
  const internal = approved.filter((p) => p.origin === "internal").length;
  const dates = s?.daily.map((d) => d.date) ?? [];

  return (
    <div>
      <PageHeader
        title={t("dashboard.title")}
        desc={[settings.orgName, t("dashboard.desc")].filter(Boolean).join(" · ")}
        actions={
          <Link
            to="/report/monthly"
            target="_blank"
            className={buttonCls("secondary")}
          >
            <FileText size={15} /> {t("report.export")}
          </Link>
        }
      />

      <div className="grid grid-cols-[minmax(0,1fr)] gap-4 lg:grid-cols-3">
        <ChainPanel
          step={t("dashboard.assets")}
          lead={formatNumber(approved.length)}
          unit={t("dashboard.published")}
          note={t("dashboard.assetsNote", { internal, upstream: approved.length - internal })}
        >
          <BarList rows={byKind} highlight={(l) => l !== t("kinds.plugin")} />
        </ChainPanel>

        <ChainPanel
          step={t("dashboard.governance")}
          lead={String(pendingVersions)}
          unit={t("dashboard.pendingUnit")}
          note={
            <PageLink page="admin-review" className="inline-flex items-center gap-1 font-medium text-brand-600 hover:text-brand-700 dark:text-brand-300 dark:hover:text-brand-200">
              {t("dashboard.goReview")} <ArrowRight size={14} />
            </PageLink>
          }
        >
          <KeyValue
            rows={[
              [t("dashboard.approved"), approved.length],
              [t("dashboard.rejected"), count("rejected")],
              [t("dashboard.yanked"), count("unpublished")],
              [t("dashboard.admission"), settings.admissionMode === "allowlist" ? t("dashboard.allowlist") : t("dashboard.openBlocklist")],
              [t("dashboard.upstreamNew"), { auto: t("dashboard.syncAuto"), review: t("dashboard.syncReview"), off: t("dashboard.syncOff") }[settings.upstream.autoSync]],
            ]}
          />
        </ChainPanel>

        <ChainPanel
          step={t("dashboard.endpoints")}
          lead={s ? formatNumber(s.totals.active_users) : "—"}
          unit={t("dashboard.activeUnit")}
          note={s ? t("dashboard.endpointsNote", { users: s.totals.users }) : ""}
        >
          <LoadState loading={stats.loading && !s} error={s ? null : stats.error} onRetry={stats.reload}>
            {s && s.platforms.length > 0 ? (
              <>
                <BarList
                  rows={s.platforms.slice(0, 6).map((p) => ({
                    label: platformLabel(p.os, p.arch, t("stats.unknownPlatform")),
                    value: p.users,
                  }))}
                />
                <p className="mt-3 text-xs text-neutral-500">{t("dashboard.platformsHint")}</p>
              </>
            ) : (
              <p className="text-sm text-neutral-600">{t("dashboard.noPlatforms")}</p>
            )}
          </LoadState>
        </ChainPanel>
      </div>

      <div className="mt-4 grid grid-cols-[minmax(0,1fr)] gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader title={t("dashboard.dailyInstalls")} aside={t("dashboard.last14")} />
          <div className="px-5 pb-5">
            <LoadState loading={stats.loading && !s} error={s ? null : stats.error} onRetry={stats.reload}>
              {s && (
                <>
                  <p className="mb-6 text-sm text-neutral-600">
                    {t("dashboard.installsSummary", { installs: formatNumber(s.totals.installs), deps: formatNumber(s.totals.downloads - s.totals.installs) })}
                  </p>
                  <DailyBars values={s.daily.map((d) => d.installs)} dates={dates} unit={t("dashboard.installsUnit")} height="h-44" />
                </>
              )}
            </LoadState>
          </div>
        </Card>
        <Card>
          <CardHeader title={t("dashboard.dailyCalls")} aside={t("dashboard.last14")} />
          <div className="px-5 pb-5">
            <LoadState loading={stats.loading && !s} error={s ? null : stats.error} onRetry={stats.reload}>
              {s && (
                <>
                  <p className="mb-6 text-sm text-neutral-600">
                    {t("dashboard.callsSummary", {
                      calls: formatNumber(s.totals.calls),
                      tokens: formatTokens(s.totals.tokens),
                      errors: formatNumber(s.totals.call_errors),
                    })}
                  </p>
                  <DailyBars values={s.daily.map((d) => d.calls)} dates={dates} unit={t("dashboard.callsUnit")} height="h-44" />
                </>
              )}
            </LoadState>
          </div>
        </Card>
      </div>

      <div className="mt-4 grid grid-cols-[minmax(0,1fr)] gap-4 lg:grid-cols-[minmax(0,1fr)_380px]">
        <Card>
          <CardHeader
            title={t("dashboard.topPlugins")}
            aside={
              <PageLink page="admin-stats" className="font-medium text-brand-600 hover:text-brand-700 dark:text-brand-300 dark:hover:text-brand-200">
                {t("dashboard.last14")}
              </PageLink>
            }
          />
          {s && s.top_plugins.length === 0 ? (
            <p className="px-5 pb-5 text-sm text-neutral-600">{t("dashboard.noInstalls")}</p>
          ) : (
            <div className="overflow-x-auto">
              <table className="tbl">
                <thead>
                  <tr>
                    <th className="px-5 py-2">{t("dashboard.colName")}</th>
                    <th className="px-5 py-2 max-sm:hidden">{t("dashboard.colSource")}</th>
                    <th className="px-5 py-2 text-right">{t("dashboard.colInstalls")}</th>
                    <th className="px-5 py-2 text-right">{t("dashboard.colUsers")}</th>
                  </tr>
                </thead>
                <tbody>
                  {(s?.top_plugins ?? []).slice(0, 6).map((p) => (
                    <tr key={p.name} className="border-t border-neutral-100">
                      <td className="px-5 py-2.5">
                        <Link to={pluginPath(p.name)} className="text-neutral-900 hover:underline">
                          {p.title}
                        </Link>
                        <div className="font-mono text-xs text-neutral-600">{p.name}</div>
                      </td>
                      <td className="px-5 py-2.5 text-neutral-600 max-sm:hidden">{p.origin === "hosted" ? t("dashboard.internal") : t("dashboard.upstream")}</td>
                      <td className="px-5 py-2.5 text-right text-neutral-900 tabular-nums">{formatNumber(p.installs)}</td>
                      <td className="px-5 py-2.5 text-right text-neutral-900 tabular-nums">{formatNumber(p.users)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </Card>

        <Card>
          <CardHeader
            title={t("dashboard.tokenUsage")}
            aside={<Segmented value={usageGroup} onChange={setUsageGroup} options={[{ value: "user", label: t("dashboard.byUser") }, { value: "token", label: t("dashboard.byToken") }]} />}
          />
          {usageGroup === "user" ? (
            <ul className="px-5 pb-3 text-sm">
              {(s?.top_models ?? []).slice(0, 6).map((m) => <li key={m.model} className="flex justify-between border-t border-neutral-100 py-2 first:border-t-0"><span className="font-mono">{m.model}</span><span className="tabular-nums">{formatTokens(m.tokens)}</span></li>)}
            </ul>
          ) : (
            <ul className="px-5 pb-3 text-sm">
              {(s?.token_stats ?? []).slice(0, 10).map((token) => (
                <li key={token.id} className="flex items-center justify-between gap-3 border-t border-neutral-100 py-2 first:border-t-0">
                  <span>{token.name}{token.account_type === "bot" && <Badge>{t("users.bot")}</Badge>}<span className="ml-2 text-xs text-neutral-500">{token.username}</span></span>
                  <span className="tabular-nums">{formatTokens(token.tokens)}</span>
                </li>
              ))}
              {s?.token_stats?.length === 0 && <li className="text-neutral-600">{t("dashboard.noInstalls")}</li>}
            </ul>
          )}
        </Card>

        <div className="flex flex-col gap-4">
          {canAudit && (
            <Card>
              <CardHeader
                title={t("dashboard.auditLog")}
                aside={
                  <PageLink page="admin-audit" className="font-medium text-brand-600 hover:text-brand-700 dark:text-brand-300 dark:hover:text-brand-200">
                    {t("dashboard.allAudit")}
                  </PageLink>
                }
              />
              <LoadState loading={audit.loading && !audit.data} error={audit.data ? null : audit.error} onRetry={audit.reload}>
                <ul className="px-5 pb-3">
                  {audit.data?.length === 0 && <li className="pb-2 text-sm text-neutral-600">{t("dashboard.noAudit")}</li>}
                  {audit.data?.map((e) => (
                    <li key={e.id} className="flex gap-3 border-t border-neutral-100 py-2.5 first:border-t-0 first:pt-0">
                      <div className="min-w-0 flex-1 text-sm">
                        <div className="flex items-center gap-1.5">
                          <span className="truncate text-neutral-900">
                            {auditActor(t, e)}
                            {e.target && ` · ${e.target}`}
                          </span>
                          <span className="ml-auto">
                            <Badge tone={auditTone(e.action)}>{auditAction(t, e.action)}</Badge>
                          </span>
                        </div>
                        <div className="truncate text-xs text-neutral-600 tabular-nums">{formatTime(e.created_at)}</div>
                      </div>
                    </li>
                  ))}
                </ul>
              </LoadState>
            </Card>
          )}
          <PendingCard plugins={plugins} total={pendingVersions} />
        </div>
      </div>
    </div>
  );
}

/** 有待审核版本的能力（包括已上架能力的新版本）。total 是待审核的版本数。 */
function PendingCard({ plugins, total }: { plugins: Plugin[]; total: number }) {
  const { t } = useTranslation();
  const pending = plugins.filter((p) => (p.pending ?? 0) > 0);
  return (
    <Card>
      <CardHeader
        title={t("dashboard.pendingReview")}
        aside={
          <PageLink page="admin-review" className="font-medium text-brand-600 hover:text-brand-700 dark:text-brand-300 dark:hover:text-brand-200">
            {t("dashboard.pendingCount", { count: total })}
          </PageLink>
        }
      />
      <ul className="px-5 pb-3">
        {pending.length === 0 && <li className="pb-2 text-sm text-neutral-600">{t("dashboard.noPending")}</li>}
        {pending.slice(0, 5).map((p) => (
          <li key={p.name} className="border-t border-neutral-100 py-2.5 first:border-t-0 first:pt-0">
            <div className="flex items-center gap-2 text-sm">
              <span className="truncate text-neutral-900">{p.title}</span>
              <span className="ml-auto">
                <Badge tone={p.origin === "internal" ? "brand" : "slate"}>{p.origin === "internal" ? t("dashboard.colleagueSubmission") : t("dashboard.upstreamVersion")}</Badge>
              </span>
            </div>
            <div className="truncate text-xs text-neutral-600">
              {p.origin === "internal" ? `${p.author} · ${p.department ?? ""}` : t("dashboard.upstreamMeta", { version: p.version })} · {t(`kinds.${kindOf(p)}`)}
            </div>
          </li>
        ))}
      </ul>
    </Card>
  );
}
