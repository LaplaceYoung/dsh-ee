import { Fragment, useState } from "react";
import { ChevronDown, ChevronRight, Blocks } from "lucide-react";
import { categoryLabel } from "../../lib/catalog";
import { api, formatBytes, formatTime, type CatalogPlugin, type PackageVersion, type ReviewAction } from "../../lib/api";
import { useApi, useAction } from "../../lib/use-api";
import { useScoring, useStore } from "../../lib/store";
import type { PluginStatus } from "../../lib/types";
import { StatusBadge } from "../../components/status";
import { LoadState } from "../../components/api-ui";
import { Badge, Button, Card, cx, inputBase, PageHeader, PluginIcon, Segmented, Empty } from "../../components/ui";
import { ScopeDialog } from "../../components/scope-dialog";
import { allScope } from "../../lib/hub";
import { useTranslation } from "react-i18next";

export default function AdminPlugins() {
  const { t } = useTranslation();
  const { notify, reloadPlugins, reloadPending } = useStore();
  const scoring = useScoring();
  const { data, loading, error, reload } = useApi(api.adminPackages);
  const hub = useApi(api.hub);
  const policies = hub.data?.plugin_policies ?? {};
  const [policyFor, setPolicyFor] = useState<string | null>(null);
  const { busy, run } = useAction();
  const [filter, setFilter] = useState<"all" | PluginStatus>("all");
  const [q, setQ] = useState("");
  const [open, setOpen] = useState<string | null>(null);
  const [yanking, setYanking] = useState<{ name: string; version: string } | null>(null);
  const [reason, setReason] = useState("");
  // 改了状态后让展开的版本列表重新加载
  const [rev, setRev] = useState(0);
  const list = (data ?? []).filter(
    (p) => (filter === "all" || p.status === filter) && (!q || `${p.name} ${p.title}`.toLowerCase().includes(q.toLowerCase())),
  );

  const act = async (name: string, version: string, action: ReviewAction, note = "") => {
    const r = await run(() => api.review({ name, version, action, note }));
    if (!r.ok) return notify(r.error);
    const label = version ? `${name}@${version}` : name;
    notify(action === "unpublish" ? t("pluginsAdmin.yanked", { name: label }) : t("pluginsAdmin.restored", { name: label }));
    setYanking(null);
    setReason("");
    setRev((n) => n + 1);
    reload();
    reloadPlugins();
    reloadPending();
  };

  return (
    <div>
      <PageHeader title={t("pluginsAdmin.title")} desc={t("pluginsAdmin.desc")} />
      <div className="mb-4 flex flex-wrap items-center gap-3">
        <Segmented
          value={filter}
          onChange={setFilter}
          options={[
            { value: "all", label: t("pluginsAdmin.all") },
            { value: "published", label: t("status.published") },
            { value: "pending", label: t("status.pending") },
            { value: "unpublished", label: t("status.unpublished") },
            { value: "rejected", label: t("status.rejected") },
          ]}
        />
        <input className={cx(inputBase, "h-9 w-64")} placeholder={t("pluginsAdmin.search")} value={q} onChange={(e) => setQ(e.target.value)} />
      </div>
      <LoadState loading={loading && !data} error={error} onRetry={reload}>
        <Card className="overflow-x-auto">
          <table className="tbl max-sm:min-w-[44rem]">
            <thead>
              <tr>
                <th className="w-8 px-2 py-2.5" />
                <th className="px-4 py-2.5">{t("pluginsAdmin.colPlugin")}</th>
                <th className="px-4 py-2.5">{t("pluginsAdmin.colSource")}</th>
                <th className="px-4 py-2.5">{t("pluginsAdmin.colCategory")}</th>
                <th className="px-4 py-2.5">{t("pluginsAdmin.colVersion")}</th>
                <th className="px-4 py-2.5 text-right">{t("pluginsAdmin.colInstalls")}</th>
                {scoring.upstream && <th className="px-4 py-2.5 text-right">{t("pluginsAdmin.colScore")}</th>}
                <th className="px-4 py-2.5">{t("pluginsAdmin.colSize")}</th>
                <th className="px-4 py-2.5">{t("pluginsAdmin.colStatus")}</th>
                <th className="px-4 py-2.5" />
              </tr>
            </thead>
            <tbody className="divide-y divide-neutral-100">
              {list.length === 0 && (
                <tr>
                  <td colSpan={10}>
                    <div className="tbl-empty"><Empty icon={Blocks}>{t("registry.plugins.empty")}</Empty></div>
                  </td>
                </tr>
              )}
              {list.map((p) => (
                <Fragment key={p.name}>
                  <tr className="hover:bg-neutral-50">
                    <td className="px-2 py-2.5">
                      <button
                        type="button"
                        aria-label={t("registry.plugins.versions")}
                        className="text-neutral-500 hover:text-neutral-900"
                        onClick={() => setOpen(open === p.name ? null : p.name)}
                      >
                        {open === p.name ? <ChevronDown size={16} /> : <ChevronRight size={16} />}
                      </button>
                    </td>
                    <td className="px-4 py-2.5">
                      <div className="flex items-center gap-2.5">
                        <PluginIcon name={p.name} icon={p.icon || null} size={28} />
                        <div className="min-w-0">
                          <div className="max-w-56 truncate font-medium text-neutral-800">{p.title || p.name}</div>
                          <div className="max-w-56 truncate font-mono text-xs text-neutral-500">{p.name}</div>
                        </div>
                      </div>
                    </td>
                    <td className="px-4 py-2.5 text-neutral-500">{p.origin === "internal" ? p.department || p.owner || "—" : t("pluginsAdmin.upstream")}</td>
                    <td className="px-4 py-2.5 text-neutral-500">{categoryLabel(p.category) ?? "—"}</td>
                    <td className="px-4 py-2.5 font-mono text-xs text-neutral-600">{p.version}</td>
                    <td className="px-4 py-2.5 text-right tabular-nums">{p.installs}</td>
                    {scoring.upstream && <td className="px-4 py-2.5 text-right tabular-nums text-neutral-600">{p.score ?? "—"}</td>}
                    <td className="px-4 py-2.5 text-neutral-500">{formatBytes(p.closure_size ?? p.size)}</td>
                    <td className="px-4 py-2.5">
                      <div className="flex items-center gap-1.5">
                        <StatusBadge status={p.status} />
                        {p.pending > 0 && p.status !== "pending" && <span className="text-xs text-amber-700">{t("registry.plugins.pendingVersions", { count: p.pending })}</span>}
                        {policies[p.name]?.delivery === "auto" && <Badge tone="brand">{t("pluginPolicy.badgeAuto")}</Badge>}
                      </div>
                    </td>
                    <td className="space-x-2 px-4 py-2.5 text-right whitespace-nowrap">
                      {p.is_plugin && (
                        <Button variant="secondary" size="sm" onClick={() => setPolicyFor(p.name)}>
                          {t("pluginPolicy.action")}
                        </Button>
                      )}
                      {p.status === "published" && (
                        <Button variant="danger" size="sm" disabled={busy} onClick={() => setYanking({ name: p.name, version: "" })}>
                          {t("pluginsAdmin.yank")}
                        </Button>
                      )}
                      {p.status === "unpublished" && (
                        <Button variant="secondary" size="sm" disabled={busy} onClick={() => act(p.name, "", "restore")}>
                          {t("pluginsAdmin.restore")}
                        </Button>
                      )}
                    </td>
                  </tr>
                  {yanking?.name === p.name && (
                    <tr className="bg-red-50/40 dark:bg-red-500/10">
                      <td />
                      <td colSpan={9} className="px-4 py-3">
                        <div className="flex flex-wrap items-center gap-2">
                          <span className="text-sm text-neutral-700">
                            {yanking.version ? t("registry.plugins.yankVersion", { name: `${p.name}@${yanking.version}` }) : t("registry.plugins.yankAll", { name: p.name })}
                          </span>
                          <input
                            autoFocus
                            className={cx(inputBase, "h-8 w-72")}
                            placeholder={t("registry.plugins.reasonPlaceholder")}
                            value={reason}
                            onChange={(e) => setReason(e.target.value)}
                          />
                          <Button variant="danger" size="sm" disabled={busy || !reason.trim()} onClick={() => act(p.name, yanking.version, "unpublish", reason.trim())}>
                            {t("pluginsAdmin.yank")}
                          </Button>
                          <Button variant="secondary" size="sm" onClick={() => setYanking(null)}>
                            {t("common.cancel")}
                          </Button>
                        </div>
                      </td>
                    </tr>
                  )}
                  {open === p.name && (
                    <tr className="bg-neutral-50/60">
                      <td />
                      <td colSpan={9} className="px-4 py-3">
                        <Versions key={rev} plugin={p} busy={busy} onYank={(v) => setYanking({ name: p.name, version: v })} onRestore={(v) => act(p.name, v, "restore")} />
                      </td>
                    </tr>
                  )}
                </Fragment>
              ))}
            </tbody>
          </table>
        </Card>
      </LoadState>
      <ScopeDialog
        open={policyFor !== null}
        onOpenChange={(v) => !v && setPolicyFor(null)}
        title={t("pluginPolicy.title", { name: policyFor ?? "" })}
        description={t("pluginPolicy.note")}
        scope={(policyFor && policies[policyFor]?.scope) || allScope()}
        delivery={(policyFor && policies[policyFor]?.delivery) || "optional"}
        canDelivery
        labels={{ auto: t("pluginPolicy.auto"), optional: t("pluginPolicy.optional"), autoHint: t("pluginPolicy.autoHint"), optionalHint: t("pluginPolicy.optionalHint") }}
        onSave={async (scope, delivery) => {
          if (!policyFor) return;
          const policy = policies[policyFor];
          await api.setPluginPolicy(policyFor, { scope, delivery, ...(policy ? { updated_at: policy.updated_at } : {}) });
          notify(t("pluginPolicy.saved", { name: policyFor }));
          hub.reload();
        }}
      />
    </div>
  );
}

function Versions({ plugin, busy, onYank, onRestore }: { plugin: CatalogPlugin; busy: boolean; onYank: (v: string) => void; onRestore: (v: string) => void }) {
  const { t } = useTranslation();
  const { data, loading, error, reload } = useApi(() => api.adminPackage(plugin.name));
  return (
    <LoadState loading={loading && !data} error={error} onRetry={reload}>
      <div className="overflow-x-auto"><table className="tbl tbl-compact">
        <thead>
          <tr>
            <th className="py-1 pr-4">{t("pluginsAdmin.colVersion")}</th>
            <th className="py-1 pr-4">{t("pluginsAdmin.colStatus")}</th>
            <th className="py-1 pr-4">{t("registry.plugins.publisher")}</th>
            <th className="py-1 pr-4">{t("registry.plugins.created")}</th>
            <th className="py-1 pr-4">{t("registry.plugins.note")}</th>
            <th />
          </tr>
        </thead>
        <tbody>
          {(data?.versions ?? []).map((v: PackageVersion) => (
            <tr key={v.version} className="border-t border-neutral-100">
              <td className="py-1.5 pr-4 font-mono text-neutral-800">
                {v.version}
                {data?.dist_tags.latest === v.version && <span className="ml-1 text-neutral-500">latest</span>}
              </td>
              <td className="py-1.5 pr-4">
                <StatusBadge status={v.status} />
              </td>
              <td className="py-1.5 pr-4 text-neutral-600">{v.publisher || "—"}</td>
              <td className="py-1.5 pr-4 text-neutral-600 tabular-nums">{formatTime(v.created_at)}</td>
              <td className="max-w-64 truncate py-1.5 pr-4 text-neutral-600" title={v.review_note}>
                {v.review_note || (v.reviewer ? t("registry.plugins.reviewedBy", { name: v.reviewer }) : "")}
              </td>
              <td className="py-1.5 text-right">
                {v.status === "published" && (
                  <button type="button" disabled={busy} className="text-neutral-600 hover:text-red-700" onClick={() => onYank(v.version)}>
                    {t("pluginsAdmin.yank")}
                  </button>
                )}
                {(v.status === "unpublished" || v.status === "rejected") && (
                  <button type="button" disabled={busy} className="text-neutral-600 hover:text-neutral-900" onClick={() => onRestore(v.version)}>
                    {t("pluginsAdmin.restore")}
                  </button>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table></div>
    </LoadState>
  );
}
