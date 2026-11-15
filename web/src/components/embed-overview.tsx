import { RefreshCw } from "lucide-react";
import { useTranslation } from "react-i18next";
import { formatTimeOnly } from "@/lib/format";
import { api } from "../lib/api";
import { useApi } from "../lib/use-api";
import { Button, Notice, Skeleton, cx } from "./ui";

/** 概览里的时间只精确到分钟：管理页看的是「最近怎么样」，不需要毫秒。 */
function formatUpdatedAt(ts: string): string {
  const d = new Date(ts);
  if (Number.isNaN(d.getTime())) return ts;
  return formatTimeOnly(d);
}

/**
 * 系统设置「跨站嵌入」里的运行概览卡片（GET /api/admin/embed/stats）：
 * 活跃会话 / 今日创建 / 今日过期 / 今日吊销 / 24h 拒绝 + 活跃应用 top 10。
 * 只展示聚合数量，不含用户、IP、令牌与宿主 origin；支持手动刷新、空态与错误态。
 */
export function EmbedOverview() {
  const { t } = useTranslation();
  const stats = useApi(api.adminEmbedStats);
  const data = stats.data;
  const initialLoading = stats.loading && !data;
  const tiles = [
    { key: "active", label: t("settingsAdmin.embed.overview.activeSessions"), value: data?.active_sessions },
    { key: "created", label: t("settingsAdmin.embed.overview.createdToday"), value: data?.created_today },
    { key: "expired", label: t("settingsAdmin.embed.overview.expiredToday"), value: data?.expired_today },
    { key: "revoked", label: t("settingsAdmin.embed.overview.revokedToday"), value: data?.revoked_today },
    { key: "denied", label: t("settingsAdmin.embed.overview.denied24h"), value: data?.denied_24h },
  ];

  return (
    <div data-testid="embed-overview" className="rounded-lg border border-neutral-200/70 p-4 dark:border-white/10">
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div className="min-w-0">
          <h3 className="text-sm font-medium text-neutral-900 dark:text-neutral-100">{t("settingsAdmin.embed.overview.title")}</h3>
          <p className="mt-0.5 text-xs leading-5 text-neutral-500">{t("settingsAdmin.embed.overview.desc")}</p>
        </div>
        <Button variant="secondary" size="sm" onClick={stats.reload} disabled={stats.loading}>
          <RefreshCw size={14} className={cx(stats.loading && "animate-spin")} />
          {t("settingsAdmin.embed.overview.refresh")}
        </Button>
      </div>

      {stats.error && (
        <Notice tone="warn" className="mt-3">
          <p>{t("settingsAdmin.embed.overview.loadFailed")}</p>
          <p className="mt-0.5 font-mono text-xs break-all">{stats.error}</p>
          <Button variant="secondary" size="sm" className="mt-2" onClick={stats.reload}>
            {t("settingsAdmin.embed.overview.retry")}
          </Button>
        </Notice>
      )}

      {!stats.error || data ? (
        <>
          <div className="mt-3 grid grid-cols-2 gap-px overflow-hidden rounded-md border border-neutral-200/70 bg-neutral-200/70 sm:grid-cols-3 xl:grid-cols-5 dark:border-white/10 dark:bg-white/10">
            {tiles.map((x) => (
              <div key={x.key} className="bg-card px-3.5 py-2.5">
                <div className="truncate text-xs text-neutral-500">{x.label}</div>
                {initialLoading ? (
                  <Skeleton className="mt-1.5 h-6 w-10" />
                ) : (
                  <div className="mt-1 text-xl leading-7 font-semibold text-neutral-900 tabular-nums dark:text-neutral-100">{x.value ?? 0}</div>
                )}
              </div>
            ))}
          </div>

          <div className="mt-4">
            <div className="flex flex-wrap items-center justify-between gap-x-2 gap-y-0.5 text-xs">
              <span className="font-medium text-neutral-700 dark:text-neutral-300">{t("settingsAdmin.embed.overview.topApps")}</span>
              {data && <span className="text-neutral-400">{t("settingsAdmin.embed.overview.updatedAt", { time: formatUpdatedAt(data.generated_at) })}</span>}
            </div>
            {data && data.top_apps.length > 0 ? (
              <ul className="mt-2 space-y-1.5">
                {data.top_apps.map((a) => (
                  <li key={a.app} className="flex items-center gap-2 text-sm">
                    <span className="truncate font-mono text-xs text-neutral-700 dark:text-neutral-300">{a.app}</span>
                    <span className="ml-auto shrink-0 text-xs text-neutral-500 tabular-nums">
                      {t("settingsAdmin.embed.overview.sessions", { n: a.sessions })}
                    </span>
                  </li>
                ))}
              </ul>
            ) : (
              <p className="mt-2 text-xs text-neutral-500">
                {initialLoading ? t("settingsAdmin.embed.overview.loading") : t("settingsAdmin.embed.overview.empty")}
              </p>
            )}
          </div>

          <p className="mt-3 text-[11px] leading-4 text-neutral-400 dark:text-neutral-500">{t("settingsAdmin.embed.overview.privacy")}</p>
        </>
      ) : null}
    </div>
  );
}
