import { KeyRound } from "lucide-react";
import { useState } from "react";
import { useStore } from "../../lib/store";
import { formatNumber } from "../../lib/format";
import type { Settings } from "../../lib/types";
import { api, formatBytes, formatTime } from "../../lib/api";
import { useApi, useAction } from "../../lib/use-api";
import { useConfirm } from "../../lib/confirm";
import { LoadState } from "../../components/api-ui";
import { Badge, Button, Card, CardHeader, CopyBox, cx, Field, inputBase, inputCls, PageHeader, Segmented, Stat, StatStrip, Toggle, Empty } from "../../components/ui";
import { Trans, useTranslation } from "react-i18next";
import { PageLink } from "../../lib/menus";

const upstreamLabel: Record<Settings["upstream"]["registry"], string> = {
  npmmirror: "registry.npmmirror.com",
  npmjs: "registry.npmjs.org",
  custom: "custom",
};

export default function AdminRegistry() {
  const { t } = useTranslation();
  const { settings: s, updateSettings, baseUrl, notify } = useStore();
  const overview = useApi(api.registryOverview);
  const { busy, run } = useAction();
  const [newScope, setNewScope] = useState("");

  const setReg = (patch: Partial<Settings["registry"]>) => updateSettings((c) => ({ ...c, registry: { ...c.registry, ...patch } }));
  const setUp = (patch: Partial<Settings["upstream"]>) => updateSettings((c) => ({ ...c, upstream: { ...c.upstream, ...patch } }));

  const registryUrl = `${baseUrl}/npm/`;
  const indexUrl = `${baseUrl}/index/dsh-index.json`;
  const scopes = [s.scope, ...s.registry.extraScopes];
  const offline = s.upstream.mode === "offline";
  const upstreamTarget = s.upstream.registry === "custom" ? s.upstream.customRegistry || t("registryAdmin.noAddress") : upstreamLabel[s.upstream.registry];
  const st = overview.data?.stats;

  const routes: { match: string; target: string; tone: "brand" | "amber" | "slate"; rule: string; scope?: string }[] = [
    ...scopes.map((sc, i) => ({
      match: `${sc}/*`,
      target: t("registryAdmin.hosted"),
      tone: "brand" as const,
      rule: t("registryAdmin.hostedRule"),
      scope: i === 0 ? undefined : sc,
    })),
    {
      match: t("registryAdmin.everythingElse"),
      target: offline ? t("registryAdmin.noUpstream") : upstreamTarget,
      tone: offline ? "amber" : "slate",
      rule: offline ? t("registryAdmin.offlineRule") : s.registry.proxyMode === "cache" ? t("registryAdmin.cacheRule") : t("registryAdmin.preloadedRule"),
    },
  ];

  const addScope = () => {
    const v = newScope.trim();
    if (!/^@[a-z0-9][a-z0-9-._~]*$/.test(v)) return notify(t("registryAdmin.scopeFormat"));
    if (scopes.includes(v)) return notify(t("registryAdmin.scopeExists", { scope: v }));
    setReg({ extraScopes: [...s.registry.extraScopes, v] });
    setNewScope("");
    notify(t("registryAdmin.scopeAdded", { scope: v }));
  };

  const npmrc = [`registry=${registryUrl}`, ...scopes.map((sc) => `${sc}:registry=${registryUrl}`), `${registryUrl.replace(/^https?:/, "")}:_authToken=\${DSH_EE_TOKEN}`].join("\n");

  return (
    <div className="max-w-5xl">
      <PageHeader
        title={t("registryAdmin.title")}
        desc={t("registryAdmin.desc", { url: registryUrl })}
        actions={
          <Button
            variant="secondary"
            size="sm"
            disabled={busy}
            onClick={async () => {
              const r = await run(api.checkRegistry);
              if (!r.ok) return notify(r.error);
              const c = r.value;
              notify(c.ok ? t("registry.admin.checkOk", { upstream: c.upstream, ms: c.latency_ms }) : t("registry.admin.checkFail", { msg: c.message ?? c.status }));
            }}
          >
            {t("registryAdmin.testConnectivity")}
          </Button>
        }
      />

      <LoadState loading={overview.loading && !overview.data} error={overview.error} onRetry={overview.reload}>
        <StatStrip cols={4}>
          <Stat label={t("registryAdmin.hostedPackages")} value={st?.hosted_packages ?? 0} hint={scopes.join(", ")} />
          <Stat label={t("registryAdmin.cachedPackages")} value={formatNumber(st?.upstream_packages ?? 0)} hint={t("registry.admin.upstreamPlugins", { count: st?.upstream_plugins ?? 0 })} />
          <Stat label={t("registryAdmin.storage")} value={formatBytes(st?.storage_bytes ?? 0)} hint={t("registry.admin.versions", { count: st?.versions ?? 0 })} />
          <Stat label={t("registry.admin.downloads30d")} value={formatNumber(st?.downloads_30d ?? 0)} hint={t("registry.admin.pending", { count: st?.pending ?? 0 })} />
        </StatStrip>
      </LoadState>

      <Card className="mt-4">
        <CardHeader title={t("registryAdmin.routes")} aside={t("registryAdmin.routesOrder")} />
        <div className="overflow-x-auto">
          <table className="tbl max-sm:min-w-[36rem]">
            <thead>
              <tr>
                <th className="px-5 py-2">{t("registryAdmin.colPackage")}</th>
                <th className="px-5 py-2">{t("registryAdmin.colTarget")}</th>
                <th className="px-5 py-2">{t("registryAdmin.colRule")}</th>
                <th className="px-5 py-2" />
              </tr>
            </thead>
            <tbody>
              {routes.map((r) => (
                <tr key={r.match} className="border-t border-neutral-100">
                  <td className="px-5 py-3 font-mono whitespace-nowrap text-neutral-900">{r.match}</td>
                  <td className="px-5 py-3 whitespace-nowrap">
                    <Badge tone={r.tone}>{r.target}</Badge>
                  </td>
                  <td className="px-5 py-3 text-neutral-700">{r.rule}</td>
                  <td className="px-5 py-3 text-right">
                    {r.scope && (
                      <button
                        type="button"
                        className="text-sm text-neutral-600 hover:text-red-700"
                        onClick={() => setReg({ extraScopes: s.registry.extraScopes.filter((x) => x !== r.scope) })}
                      >
                        {t("registryAdmin.remove")}
                      </button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <div className="flex flex-wrap items-center gap-2 border-t border-neutral-100 px-5 py-4">
          <input
            aria-label={t("registryAdmin.newScope")}
            className={cx(inputBase, "w-48 font-mono")}
            placeholder="@acme-data"
            value={newScope}
            onChange={(e) => setNewScope(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && addScope()}
          />
          <Button variant="secondary" size="sm" onClick={addScope}>
            {t("registryAdmin.addScope")}
          </Button>
          <span className="text-sm text-neutral-600">{t("registryAdmin.mainScope", { scope: s.scope })}</span>
        </div>
      </Card>

      <div className="mt-4 grid gap-4 lg:grid-cols-2">
        <Card className="p-5">
          <h2 className="text-neutral-900 text-[15px] leading-6 font-semibold">{t("registryAdmin.upstream")}</h2>
          <div className="mt-4 space-y-5">
            <Field label={t("registryAdmin.npmUpstream")} hint={offline ? t("registryAdmin.offlineHint") : undefined}>
              <div className="flex flex-col gap-2">
                <select
                  className={cx(inputBase, "w-56")}
                  value={s.upstream.registry}
                  disabled={offline}
                  onChange={(e) => setUp({ registry: e.target.value as Settings["upstream"]["registry"] })}
                >
                  <option value="npmmirror">{t("registryAdmin.npmmirror")}</option>
                  <option value="npmjs">registry.npmjs.org</option>
                  <option value="custom">{t("registryAdmin.customNexus")}</option>
                </select>
                {s.upstream.registry === "custom" && (
                  <input
                    className={cx(inputCls, "font-mono")}
                    placeholder="https://nexus.acme.internal/repository/npm/"
                    value={s.upstream.customRegistry}
                    disabled={offline}
                    onChange={(e) => setUp({ customRegistry: e.target.value })}
                  />
                )}
              </div>
            </Field>
            <Field label={t("registryAdmin.outsideScope")} hint={s.registry.proxyMode === "cache" ? t("registryAdmin.cacheHint") : t("registryAdmin.preloadedHint")}>
              <Segmented
                value={s.registry.proxyMode}
                onChange={(v) => setReg({ proxyMode: v })}
                options={[
                  { value: "preloaded", label: t("registryAdmin.preloadedOnly") },
                  { value: "cache", label: t("registryAdmin.cacheOnDemand") },
                ]}
              />
            </Field>
          </div>
        </Card>

        <Card className="p-5">
          <h2 className="text-neutral-900 text-[15px] leading-6 font-semibold">{t("registryAdmin.clientConfig")}</h2>
          <p className="mt-1 text-sm text-neutral-600">
            <Trans i18nKey="registryAdmin.clientConfigDesc" components={{ link: <PageLink page="setup" className="mx-1 font-medium text-brand-600 hover:text-brand-700 dark:text-brand-300 dark:hover:text-brand-200" /> }} />
          </p>
          <div className="mt-4 space-y-3">
            <CopyBox label="~/.npmrc" text={npmrc} />
            <CopyBox label={t("registry.admin.indexUrl")} text={indexUrl} />
          </div>
        </Card>
      </div>

      <TokensCard requireToken={s.registry.requireToken} onRequireToken={(v) => setReg({ requireToken: v })} />
    </div>
  );
}

function TokensCard({ requireToken, onRequireToken }: { requireToken: boolean; onRequireToken: (v: boolean) => void }) {
  const { t } = useTranslation();
  const { notify } = useStore();
  const confirm = useConfirm();
  const { data, loading, error, reload } = useApi(api.allNpmTokens);
  const { run } = useAction();
  return (
    <Card className="mt-4">
      <CardHeader title={t("registryAdmin.tokens")} aside={t("registry.admin.tokensCount", { count: data?.length ?? 0 })} />
      <div className="flex items-start justify-between gap-6 px-5 pb-4">
        <div>
          <div className="text-sm text-neutral-900">{t("registryAdmin.requireToken")}</div>
          <div className="mt-0.5 text-sm text-neutral-600">{t("registryAdmin.requireTokenHint")}</div>
        </div>
        <Toggle checked={requireToken} onChange={onRequireToken} />
      </div>
      <LoadState loading={loading && !data} error={error} onRetry={reload}>
        <div className="overflow-x-auto">
          <table className="tbl max-sm:min-w-[36rem]">
            <thead>
              <tr>
                <th className="px-5 py-2">{t("registryAdmin.colName")}</th>
                <th className="px-5 py-2">{t("registryAdmin.colOwner")}</th>
                <th className="px-5 py-2">{t("registryAdmin.colAccess")}</th>
                <th className="px-5 py-2 text-right">{t("registryAdmin.colCreated")}</th>
                <th className="px-5 py-2 text-right">{t("registryAdmin.colLastUsed")}</th>
                <th className="px-5 py-2" />
              </tr>
            </thead>
            <tbody>
              {(data ?? []).length === 0 && (
                <tr className="border-t border-neutral-100">
                  <td colSpan={6}>
                    <div className="tbl-empty"><Empty compact icon={KeyRound}>{t("registry.admin.noTokens")}</Empty></div>
                  </td>
                </tr>
              )}
              {(data ?? []).map((tk) => (
                <tr key={tk.id} className="border-t border-neutral-100">
                  <td className="px-5 py-3 text-neutral-900">
                    {tk.name} <span className="font-mono text-xs text-neutral-500">{tk.prefix}…</span>
                  </td>
                  <td className="px-5 py-3 text-neutral-700">{tk.username}</td>
                  <td className="px-5 py-3">
                    <Badge tone={tk.access === "publish" ? "amber" : "slate"}>{tk.access === "publish" ? t("registryAdmin.readPublish") : t("registryAdmin.readOnly")}</Badge>
                  </td>
                  <td className="px-5 py-3 text-right text-neutral-700 tabular-nums">{formatTime(tk.created_at)}</td>
                  <td className="px-5 py-3 text-right text-neutral-700 tabular-nums">{tk.last_used_at ? formatTime(tk.last_used_at) : t("registry.setup.never")}</td>
                  <td className="px-5 py-3 text-right">
                    <button
                      type="button"
                      className="text-sm text-neutral-600 hover:text-red-700"
                      onClick={async () => {
                        if (!(await confirm({ kind: "revoke", description: t("registry.admin.revokeConfirm", { name: tk.name, user: tk.username }) }))) return;
                        const r = await run(() => api.revokeNpmToken(tk.id));
                        if (!r.ok) return notify(r.error);
                        notify(t("registryAdmin.tokenRevoked", { name: tk.name }));
                        reload();
                      }}
                    >
                      {t("registryAdmin.revoke")}
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </LoadState>
      <div className="border-t border-neutral-100 px-5 py-4 text-sm text-neutral-600">
        <Trans i18nKey="registry.admin.tokensHint" components={{ link: <PageLink page="setup" className="font-medium text-brand-600 hover:text-brand-700 dark:text-brand-300 dark:hover:text-brand-200" /> }} />
      </div>
    </Card>
  );
}
