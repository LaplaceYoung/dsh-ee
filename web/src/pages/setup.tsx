import { useState } from "react";
import { KeyRound } from "lucide-react";
import { useStore } from "../lib/store";
import { useAuth } from "../lib/auth";
import { api, formatTime } from "../lib/api";
import { useApi, useAction } from "../lib/use-api";
import { useConfirm } from "../lib/confirm";
import { pluginAddCommand } from "../lib/install";
import { LoadState } from "../components/api-ui";
import { Badge, Button, Card, CardHeader, CopyBox, cx, inputBase, PageHeader, Segmented, Notice } from "../components/ui";
import { useTranslation } from "react-i18next";

type Mode = "unix" | "windows" | "manual";

export default function Setup() {
  const { t } = useTranslation();
  const { settings, baseUrl, plugins } = useStore();
  const [mode, setMode] = useState<Mode>("unix");
  // 刚创建的令牌只在本页显示这一次，下面的命令会自动填进去
  const [token, setToken] = useState<string | null>(null);
  const shown = token ?? t("registry.setup.tokenPlaceholder");
  const indexUrl = `${baseUrl}/index/dsh-index.json`;
  const registry = `${baseUrl}/npm/`;
  const registryKey = registry.replace(/^https?:/, "");
  const scopes = [settings.scope, ...settings.registry.extraScopes];
  const example = plugins.find((p) => p.origin === "internal")?.name ?? plugins[0]?.name ?? `${settings.scope}/hello`;
  const connect = useApi(api.connectInfo);
  const ci = connect.data;

  return (
    <div className="max-w-4xl">
      <PageHeader title={t("setup.title")} desc={t("setup.desc")} />

      <Tokens onCreated={setToken} />

      {ci && ci.enabled && (
        <Card className="mt-6 p-5">
          <div className="flex flex-wrap items-center gap-2">
            <h2 className="text-neutral-900 text-[15px] leading-6 font-semibold">{t("setupConnect.title")}</h2>
            {ci.ready ? <Badge tone="green">{t("setupConnect.ready", { pkg: ci.package, version: ci.version })}</Badge> : <Badge tone="amber">{t("hub.notLiveBadge")}</Badge>}
          </div>
          <p className="mt-1 text-sm text-neutral-600">{ci.ready ? t("setupConnect.desc", { pkg: ci.package }) : t("setupConnect.notReady")}</p>
          <p className="mt-2 text-xs text-neutral-500">
            {t("setupConnect.needs")} {t("setupConnect.tokenNote")}
          </p>
          {ci.ready && ci.tarball_url && (
            <div className="mt-4">
              <p className="mb-2 text-sm text-neutral-600">
                <span className="font-medium text-neutral-800">{t("setupConnect.tarballTitle")}</span> {t("setupConnect.tarballDesc")}
              </p>
              <CopyBox text={ci.tarball_url} />
            </div>
          )}
        </Card>
      )}

      <Card className="mt-6 p-5 text-sm text-neutral-700">
        <h2 className="mb-2 text-neutral-900 text-[15px] leading-6 font-semibold">{t("setup.deviceLoginTitle")}</h2>
        <p>{t("setup.deviceLoginDesc")}</p>
        <div className="mt-2 font-mono text-xs text-neutral-600">/ee-login · /ee-status</div>
      </Card>

      <div className="mt-6">
        <Segmented
          value={mode}
          onChange={setMode}
          options={[
            { value: "unix", label: "macOS / Linux" },
            { value: "windows", label: "Windows" },
            { value: "manual", label: t("setup.manual") },
          ]}
        />
      </div>

      <div className="mt-4 space-y-4">
        {mode === "unix" && (
          <>
            <CopyBox label={t("setup.runInTerminal")} text={`curl -fsSL ${baseUrl}/setup.sh | sh`} />
            <ScriptExplain index={indexUrl} />
          </>
        )}
        {mode === "windows" && (
          <>
            <CopyBox label={t("setup.runInPowerShell")} text={`irm ${baseUrl}/setup.ps1 | iex`} />
            <ScriptExplain index={indexUrl} />
          </>
        )}
        {mode === "manual" && (
          <div className="space-y-5">
            <Step n={1} title={t("setup.step1")}>
              <p className="mb-2 text-sm text-neutral-500">{t("setup.step1Desc")}</p>
              <CopyBox text={indexUrl} />
            </Step>
            <Step n={2} title={t("setup.step2")}>
              <p className="mb-2 text-sm text-neutral-500">{t("setup.step2Desc")}</p>
              <CopyBox text={[`registry=${registry}`, `${registryKey}:_authToken=${shown}`, ...scopes.map((sc) => `${sc}:registry=${registry}`)].join("\n")} />
            </Step>
            {baseUrl.startsWith("https:") && (
              <Step n={3} title={t("setup.step3")}>
                <p className="mb-2 text-sm text-neutral-500">{t("registry.setup.caDesc")}</p>
                <CopyBox text={`echo "cafile=$HOME/.dsh-ee-ca.pem" >> ~/.npmrc\nexport NODE_EXTRA_CA_CERTS=$HOME/.dsh-ee-ca.pem   ${t("setup.shellComment")}`} />
              </Step>
            )}
            {ci?.ready && (
              <Step n={baseUrl.startsWith("https:") ? 4 : 3} title={t("setupConnect.title")}>
                <p className="mb-2 text-sm text-neutral-500">{t("setupConnect.manualEnv")}</p>
                <CopyBox text={`${ci.token_env}=${shown}`} />
                <p className="mt-3 mb-2 text-sm text-neutral-500">{t("setupConnect.manualAdd")}</p>
                <CopyBox text={`dsh plugin --profile ${ci.profile} add ${ci.package}`} />
                <p className="mt-3 text-sm text-neutral-500">{t("setupConnect.manualRestart")}</p>
              </Step>
            )}
            <Step n={(baseUrl.startsWith("https:") ? 4 : 3) + (ci?.ready ? 1 : 0)} title={t("setup.step4")}>
              <CopyBox text={pluginAddCommand(example, ci?.profile)} />
            </Step>
          </div>
        )}
      </div>

      <Notice tone="warn" className="mt-8">{t("setup.coexist")}</Notice>
    </div>
  );
}

function Tokens({ onCreated }: { onCreated: (token: string) => void }) {
  const { t } = useTranslation();
  const { notify } = useStore();
  const { can } = useAuth();
  const confirm = useConfirm();
  const { data, loading, error, reload } = useApi(api.myNpmTokens);
  const { busy, run } = useAction();
  const [name, setName] = useState("");
  const [access, setAccess] = useState<"read" | "publish">("read");
  const [issued, setIssued] = useState<string | null>(null);
  const canPublish = can("registry.publish");

  const create = async () => {
    const r = await run(() => api.createNpmToken({ name: name.trim(), access }));
    if (!r.ok) return notify(r.error);
    setIssued(r.value.token);
    onCreated(r.value.token);
    setName("");
    reload();
  };

  return (
    <Card>
      <CardHeader
        title={
          <span className="inline-flex items-center gap-2">
            <KeyRound size={16} className="text-brand-500" /> {t("setup.tokenTitle")}
          </span>
        }
        aside={t("registry.setup.tokenCount", { count: data?.length ?? 0 })}
      />
      <p className="px-5 text-sm text-neutral-600">{t("setup.tokenDesc")}</p>
      <LoadState loading={loading && !data} error={error} onRetry={reload}>
        {(data ?? []).length > 0 && (
          <div className="overflow-x-auto"><table className="tbl mt-3 max-sm:min-w-[32rem]">
            <thead>
              <tr>
                <th className="px-5 py-2">{t("registryAdmin.colName")}</th>
                <th className="px-5 py-2">{t("registryAdmin.colAccess")}</th>
                <th className="px-5 py-2">{t("registry.setup.prefix")}</th>
                <th className="px-5 py-2 text-right">{t("registryAdmin.colLastUsed")}</th>
                <th className="px-5 py-2" />
              </tr>
            </thead>
            <tbody>
              {(data ?? []).map((tk) => (
                <tr key={tk.id} className="border-t border-neutral-100">
                  <td className="px-5 py-2.5 text-neutral-900">{tk.name}</td>
                  <td className="px-5 py-2.5">
                    <Badge tone={tk.access === "publish" ? "amber" : "slate"}>{tk.access === "publish" ? t("registryAdmin.readPublish") : t("registryAdmin.readOnly")}</Badge>
                  </td>
                  <td className="px-5 py-2.5 font-mono text-xs text-neutral-600">{tk.prefix}…</td>
                  <td className="px-5 py-2.5 text-right text-neutral-600 tabular-nums">{tk.last_used_at ? formatTime(tk.last_used_at) : t("registry.setup.never")}</td>
                  <td className="px-5 py-2.5 text-right">
                    <button
                      type="button"
                      className="text-sm text-neutral-600 hover:text-red-700"
                      onClick={async () => {
                        if (!(await confirm({ kind: "revoke", description: t("registry.setup.revokeConfirm", { name: tk.name }) }))) return;
                        const r = await run(() => api.revokeMyNpmToken(tk.id));
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
          </table></div>
        )}
      </LoadState>
      <div className="mt-3 border-t border-neutral-100 px-5 py-4">
        <div className="flex flex-wrap items-center gap-2">
          <input
            aria-label={t("registryAdmin.tokenName")}
            className={cx(inputBase, "w-56")}
            placeholder={t("registry.setup.tokenNamePlaceholder")}
            value={name}
            onChange={(e) => setName(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && name.trim() && create()}
          />
          {canPublish && (
            <Segmented
              value={access}
              onChange={setAccess}
              options={[
                { value: "read", label: t("registryAdmin.readOnly") },
                { value: "publish", label: t("registryAdmin.readPublish") },
              ]}
            />
          )}
          <Button size="sm" disabled={busy || !name.trim()} onClick={create}>
            {t("registryAdmin.newToken")}
          </Button>
        </div>
        {issued && (
          <div className="mt-4">
            <p className="mb-2 text-sm text-neutral-700">{t("registry.setup.tokenOnce")}</p>
            <CopyBox text={issued} />
          </div>
        )}
      </div>
    </Card>
  );
}

function ScriptExplain({ index }: { index: string }) {
  const { t } = useTranslation();
  return (
    <Card className="p-5 text-sm text-neutral-600">
      <div className="mb-2 font-medium text-neutral-800">{t("setup.scriptTitle")}</div>
      <ol className="list-decimal space-y-1 pl-5">
        <li>{t("registry.setup.scriptNpmrc")}</li>
        <li>{t("setup.scriptCheck")}</li>
        <li>
          {t("registry.setup.scriptIndex")} <code className="font-mono text-xs text-neutral-800">{index}</code>
        </li>
      </ol>
    </Card>
  );
}

function Step({ n, title, children }: { n: number; title: string; children: React.ReactNode }) {
  return (
    <div className="flex gap-4">
      <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-brand-500 text-sm font-medium text-white">{n}</span>
      <div className="min-w-0 flex-1">
        <div className="mb-2 font-medium text-neutral-800">{title}</div>
        {children}
      </div>
    </div>
  );
}
