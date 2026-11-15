import { useEffect, useRef, useState, type ReactNode } from "react";
import { Link } from "react-router";
import { useTranslation } from "react-i18next";
import { api, formatBytes, formatTokens } from "../../lib/api";
import { formatNumber } from "../../lib/format";
import { useApi } from "../../lib/use-api";
import { platformLabel } from "../../lib/labels";
import { Card, CardHeader, PageHeader, Segmented, Stat, StatStrip } from "../../components/ui";
import { DailyBars } from "../../components/charts";
import { LoadState } from "../../components/api-ui";
import { pluginPath } from "../../components/plugin-card";

const RANGES = ["7", "14", "30", "90"] as const;

/** 简单的数据表：第一列左对齐，其余右对齐的数字列。 */
function NumTable({ head, rows, empty }: { head: string[]; rows: { key: string; cells: ReactNode[] }[]; empty: string }) {
  if (rows.length === 0) return <p className="px-5 pb-5 text-sm text-neutral-600">{empty}</p>;
  return (
    <div className="overflow-x-auto">
      <table className="tbl">
        <thead>
          <tr>
            {head.map((h, i) => (
              <th key={h} className={i === 0 ? "px-5 py-2" : "px-5 py-2 text-right"}>
                {h}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((r) => (
            <tr key={r.key} className="border-t border-neutral-100">
              {r.cells.map((c, i) => (
                <td key={i} className={i === 0 ? "px-5 py-2.5" : "px-5 py-2.5 text-right text-neutral-900 tabular-nums"}>
                  {c}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

export default function AdminStats() {
  const { t } = useTranslation();
  const [days, setDays] = useState<(typeof RANGES)[number]>("14");
  const { data: s, error, loading, reload } = useApi(() => api.stats(Number(days)));
  // 切换时间范围后重新加载（首次加载由 useApi 自己发起）
  const first = useRef(true);
  useEffect(() => {
    if (first.current) {
      first.current = false;
      return;
    }
    reload();
  }, [days, reload]);

  const range = t("stats.days", { n: days });
  const dates = s?.daily.map((d) => d.date) ?? [];
  const errPct = s && s.totals.calls > 0 ? ((s.totals.call_errors / s.totals.calls) * 100).toFixed(1) : "0";
  const storage = (origin: "hosted" | "upstream") => s?.storage.find((x) => x.origin === origin) ?? { bytes: 0, versions: 0 };

  return (
    <div>
      <PageHeader
        title={t("stats.title")}
        desc={t("stats.desc")}
        actions={<Segmented value={days} onChange={setDays} options={RANGES.map((d) => ({ value: d, label: t("stats.days", { n: d }) }))} />}
      />
      <LoadState loading={loading && !s} error={s ? null : error} onRetry={reload}>
        {s && (
          <div className={loading ? "opacity-60 transition-opacity" : "transition-opacity"}>
            <StatStrip>
              <Stat label={t("stats.installs")} value={formatNumber(s.totals.installs)} hint={t("stats.installsHint", { deps: formatNumber(s.totals.downloads - s.totals.installs) })} />
              <Stat label={t("stats.activeUsers")} value={formatNumber(s.totals.active_users)} hint={t("stats.activeHint", { users: s.totals.users })} />
              <Stat label={t("stats.calls")} value={formatNumber(s.totals.calls)} hint={t("stats.callsHint", { errors: s.totals.call_errors, pct: errPct })} />
              <Stat label={t("stats.tokens")} value={formatTokens(s.totals.tokens)} hint={t("stats.tokensHint")} />
            </StatStrip>

            <div className="mt-4 grid grid-cols-[minmax(0,1fr)] gap-4 lg:grid-cols-2">
              <Card>
                <CardHeader title={t("stats.dailyInstalls")} aside={range} />
                <div className="px-5 pb-5">
                  <DailyBars values={s.daily.map((d) => d.installs)} dates={dates} unit={t("stats.installsUnit")} height="h-48" />
                </div>
              </Card>
              <Card>
                <CardHeader title={t("stats.dailyTokens")} aside={range} />
                <div className="px-5 pb-5">
                  <DailyBars values={s.daily.map((d) => d.tokens)} dates={dates} unit={t("stats.tokensUnit")} height="h-48" format={formatTokens} />
                </div>
              </Card>
            </div>

            <Card className="mt-4">
              <CardHeader title={t("stats.departments")} aside={range} />
              <NumTable
                head={[t("stats.colDepartment"), t("stats.colAccounts"), t("stats.colActive"), t("stats.colInstalls"), t("stats.colCalls"), t("stats.colTokens")]}
                empty={t("stats.empty")}
                rows={[...s.departments]
                  .sort((a, b) => b.active_users - a.active_users || b.tokens - a.tokens || b.users - a.users)
                  .map((d) => ({
                    key: d.department,
                    cells: [
                      d.department ? <span className="text-neutral-900">{d.department}</span> : <span className="text-neutral-600">{t("stats.noDepartment")}</span>,
                      formatNumber(d.users),
                      formatNumber(d.active_users),
                      formatNumber(d.installs),
                      formatNumber(d.calls),
                      formatTokens(d.tokens),
                    ],
                  }))}
              />
            </Card>

            <div className="mt-4 grid grid-cols-[minmax(0,1fr)] gap-4 lg:grid-cols-2">
              <Card>
                <CardHeader title={t("stats.topPlugins")} aside={range} />
                <NumTable
                  head={[t("stats.colPlugin"), t("stats.colInstalls"), t("stats.colUsers")]}
                  empty={t("stats.empty")}
                  rows={s.top_plugins.map((p) => ({
                    key: p.name,
                    cells: [
                      <>
                        <Link to={pluginPath(p.name)} className="text-neutral-900 hover:underline">
                          {p.title}
                        </Link>
                        <div className="text-xs text-neutral-600">
                          <span className="font-mono">{p.name}</span> · {p.origin === "hosted" ? t("stats.internal") : t("stats.upstream")}
                        </div>
                      </>,
                      formatNumber(p.installs),
                      formatNumber(p.users),
                    ],
                  }))}
                />
              </Card>
              <Card>
                <CardHeader title={t("stats.topModels")} aside={range} />
                <NumTable
                  head={[t("stats.colModel"), t("stats.colCalls"), t("stats.colUsers"), t("stats.colTokens")]}
                  empty={t("stats.empty")}
                  rows={s.top_models.map((m) => ({
                    key: m.model,
                    cells: [<span className="font-mono text-neutral-900">{m.model}</span>, formatNumber(m.calls), formatNumber(m.users), formatTokens(m.tokens)],
                  }))}
                />
              </Card>
            </div>

            <div className="mt-4 grid grid-cols-[minmax(0,1fr)] gap-4 lg:grid-cols-2">
              <Card>
                <CardHeader title={t("stats.platforms")} aside={range} />
                <NumTable
                  head={[t("stats.colPlatform"), t("stats.colUsers"), t("stats.colInstalls")]}
                  empty={t("stats.empty")}
                  rows={s.platforms.map((p) => ({
                    key: `${p.os}/${p.arch}`,
                    cells: [<span className="text-neutral-900">{platformLabel(p.os, p.arch, t("stats.unknownPlatform"))}</span>, formatNumber(p.users), formatNumber(p.installs)],
                  }))}
                />
              </Card>
              <Card>
                <CardHeader title={t("stats.storage")} />
                <NumTable
                  head={["", t("stats.colSize")]}
                  empty={t("stats.empty")}
                  rows={(["hosted", "upstream"] as const).map((o) => ({
                    key: o,
                    cells: [
                      <>
                        <div className="text-neutral-900">{o === "hosted" ? t("stats.storageHosted") : t("stats.storageUpstream")}</div>
                        <div className="text-xs text-neutral-600">{t("stats.storageVersions", { count: storage(o).versions })}</div>
                      </>,
                      formatBytes(storage(o).bytes),
                    ],
                  }))}
                />
              </Card>
            </div>
          </div>
        )}
      </LoadState>
    </div>
  );
}
