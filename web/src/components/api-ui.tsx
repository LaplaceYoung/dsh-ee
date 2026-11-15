/** 接真实接口的页面共用的小组件：加载 / 出错状态、key 状态、用量条。 */
import type { ReactNode } from "react";
import { formatTokens, keyState, type ApiKey } from "../lib/api";
import { formatNumber } from "../lib/format";
import { CircleAlert } from "lucide-react";
import { Badge, Button, cx, Notice, Spinner } from "./ui";
import { useTranslation } from "react-i18next";
import { Tip } from "@/components/tip";

export function LoadState({ loading, error, onRetry, children }: { loading: boolean; error: string | null; onRetry: () => void; children: ReactNode }) {
  const { t } = useTranslation();
  if (error) {
    return (
      <div className="px-5 py-6">
        <Notice tone="danger">
          <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-2">
            <span className="min-w-0 break-words">{t("common.loadFailed", { error })}</span>
            <Button variant="secondary" size="sm" onClick={onRetry}>
              {t("common.retry")}
            </Button>
          </div>
        </Notice>
      </div>
    );
  }
  if (loading) {
    return (
      <div role="status" className="flex items-center gap-2 px-5 py-8 text-sm text-neutral-500">
        <Spinner className="text-neutral-400" />
        {t("common.loading")}
      </div>
    );
  }
  return children;
}

export function ErrorNote({ children }: { children: ReactNode }) {
  if (!children) return null;
  return (
    <p role="alert" className="flex gap-2 rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm leading-6 text-red-800 dark:border-red-400/25 dark:bg-red-500/10 dark:text-red-300">
      <CircleAlert size={16} aria-hidden className="mt-1 shrink-0 text-red-600 dark:text-red-300" />
      <span className="min-w-0 flex-1 break-words">{children}</span>
    </p>
  );
}

export function KeyStateBadge({ k }: { k: ApiKey }) {
  const { t } = useTranslation();
  const s = keyState(k);
  if (s === "revoked") return <Badge>{t("keys.state.revoked")}</Badge>;
  if (s === "expired") return <Badge tone="amber">{t("keys.state.expired")}</Badge>;
  return <Badge tone="green">{t("keys.state.active")}</Badge>;
}

/** 单个 key 的本月用量（额度按用户共享，所以这里不显示上限）。 */
export function TokensCell({ tokens }: { tokens: number }) {
  return <span className="tabular-nums">{formatTokens(tokens)}</span>;
}

/** 用户级额度条：null = 不限额，0 = 未分配，>0 = 本月上限。 */
export function QuotaBar({ used, limit }: { used: number; limit: number | null }) {
  const { t } = useTranslation();
  if (limit === 0) {
    return <Badge tone="amber">{t("keys.quota.unassigned")}</Badge>;
  }
  if (limit === null) {
    return (
      <span className="tabular-nums">
        {formatTokens(used)}
        <span className="text-neutral-500"> / {t("common.unlimited")}</span>
      </span>
    );
  }
  const pct = Math.min(100, (used / limit) * 100);
  const over = used >= limit;
  return (
    <Tip label={t("keys.quota.exact", { used: formatNumber(used), limit: formatNumber(limit), pct: pct.toFixed(1) })}>
    <div tabIndex={0} className="min-w-32 cursor-help">
      <div className="text-right tabular-nums">
        {formatTokens(used)}
        <span className="text-neutral-500"> / {formatTokens(limit)}</span>
      </div>
      <div className="mt-1 h-1.5 overflow-hidden rounded-full bg-neutral-100">
        <div
          className={cx("h-full rounded-full", over || pct >= 90 ? "bg-red-600" : pct >= 70 ? "bg-amber-500" : "bg-brand-500")}
          style={{ width: `${pct}%` }}
        />
      </div>
      {over && <div className="mt-0.5 text-right text-xs text-red-700">{t("keys.quota.exhausted")}</div>}
    </div>
    </Tip>
  );
}

export function ModelsCell({ models }: { models: string[] | null }) {
  const { t } = useTranslation();
  if (!models || models.length === 0) return <span className="text-neutral-600">{t("common.allModels")}</span>;
  return <span className="text-neutral-800">{models.join(", ")}</span>;
}
