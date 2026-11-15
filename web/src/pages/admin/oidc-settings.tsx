import { useEffect, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { copyText } from "../../lib/clipboard";
import { Check, Copy, Plus, X } from "lucide-react";
import { api, type OidcConfig, type OidcTestResult, type OidcView } from "../../lib/api";
import { useAuth } from "../../lib/auth";
import { useStore } from "../../lib/store";
import { useAction, useApi } from "../../lib/use-api";
import { useLabels } from "../../lib/labels";
import { ErrorNote, LoadState } from "../../components/api-ui";
import { Combobox } from "../../components/pickers";
import { Badge, Button, CopyBox, cx, Field, inputBase, inputCls, PasswordInput, ToggleRow } from "../../components/ui";

const PRESETS: Record<OidcConfig["preset"], Partial<OidcConfig>> = {
  keycloak: { preset: "keycloak", scopes: "profile email", usernameClaim: "preferred_username", nameClaim: "name", emailClaim: "email", departmentClaim: "department", groupsClaim: "groups" },
  authing: { preset: "authing", scopes: "profile email roles", usernameClaim: "preferred_username", nameClaim: "name", emailClaim: "email", departmentClaim: "department", groupsClaim: "roles" },
  azure: { preset: "azure", scopes: "profile email", usernameClaim: "preferred_username", nameClaim: "name", emailClaim: "email", departmentClaim: "department", groupsClaim: "groups" },
  custom: { preset: "custom" },
  feishu: { preset: "feishu", issuer: "https://accounts.feishu.cn", feishuApiBase: "https://open.feishu.cn", scopes: "profile email", usernameClaim: "preferred_username", nameClaim: "name", emailClaim: "email", departmentClaim: "department", groupsClaim: "groups" },
};

function configOf(v: OidcView): OidcConfig {
  const { clientSecretSet: _s, redirectUri: _r, ...c } = v;
  return c;
}

function SubHeading({ children }: { children: ReactNode }) {
  return <h3 className="border-t border-neutral-100 pt-5 text-sm font-semibold text-neutral-900">{children}</h3>;
}

/** 系统设置里的企业统一登录（OIDC）。Client secret 只写不读。 */
export default function OidcSettings() {
  const { t } = useTranslation();
  const { can } = useAuth();
  const { notify } = useStore();
  const L = useLabels();
  const { data, error, loading, reload } = useApi(api.oidc);
  const canRoles = can("roles.manage");
  const roles = useApi(() => (canRoles || can("users.manage") ? api.roles() : Promise.resolve(null)));
  const [saved, setSaved] = useState<OidcView | null>(null);
  const [form, setForm] = useState<OidcConfig | null>(null);
  const [secret, setSecret] = useState("");
  const [saveError, setSaveError] = useState<string | null>(null);
  const [test, setTest] = useState<OidcTestResult | null>(null);
  const save = useAction();
  const tester = useAction();
  const syncStatus = useApi(api.feishuSyncStatus);
  const synchronize = useAction();
  const workspace = useAction();
  const [workspaceNext, setWorkspaceNext] = useState("/go");
  const [workspaceLink, setWorkspaceLink] = useState("");

  useEffect(() => {
    if (!data) return;
    setSaved(data);
    setForm(configOf(data));
  }, [data]);

  if (!form || !saved) {
    return (
      <LoadState loading={loading} error={error} onRetry={reload}>
        {null}
      </LoadState>
    );
  }

  const set = (patch: Partial<OidcConfig>) => setForm((f) => (f ? { ...f, ...patch } : f));
  const dirty = JSON.stringify(form) !== JSON.stringify(configOf(saved)) || secret !== "";
  const input = () => ({ ...form, ...(secret ? { clientSecret: secret } : {}) });
  const off = !form.enabled;
  const roleOptions = (roles.data ?? []).map((r) => ({ value: String(r.id), label: L.role(r) }));
  const roleName = (id: number) => {
    const r = roles.data?.find((x) => x.id === id);
    return r ? L.role(r) : `#${id}`;
  };

  const onSave = async () => {
    setSaveError(null);
    const r = await save.run(() => api.saveOidc(input()));
    if (!r.ok) return setSaveError(r.error);
    setSaved(r.value);
    setForm(configOf(r.value));
    setSecret("");
    notify(t("oidcAdmin.saved"));
  };

  const onTest = async () => {
    setTest(null);
    const r = await tester.run(() => api.testOidc(input()));
    if (r.ok) setTest(r.value);
    else setTest({ ok: false, error: r.error });
  };

  const onSync = async () => {
    const r = await synchronize.run(api.syncFeishu);
    if (!r.ok) return setSaveError(r.error);
    syncStatus.reload();
    if (!r.value.ok) return setSaveError(r.value.error ?? t("oidcAdmin.feishu.failed"));
    notify(t("oidcAdmin.feishu.syncDone", { created: r.value.created, reconciled: r.value.reconciled, disabled: r.value.disabled.length }));
  };

  const onWorkspace = async () => {
    const r = await workspace.run(() => api.feishuWorkspaceURL(workspaceNext));
    if (!r.ok) return notify(r.error);
    setWorkspaceLink(r.value.url);
    if (!(await copyText(r.value.url))) return notify(t("common.copyFailed"));
  };

  return (
    <div className="space-y-5">
      <ToggleRow label={t("oidcAdmin.enabled")} desc={t("oidcAdmin.desc")} checked={form.enabled} onChange={(v) => set({ enabled: v })} />

      <div className={cx("space-y-5", off && "opacity-60")}>
        <div className="flex flex-wrap items-center gap-2 text-sm text-neutral-600">
          <span>{t("oidcAdmin.preset")}</span>
          {(Object.keys(PRESETS) as OidcConfig["preset"][]).map((k) => (
            <Button key={k} size="sm" variant={form.preset === k ? "primary" : "secondary"} onClick={() => set(k === "feishu" ? { ...PRESETS[k], name: t("oidcAdmin.presets.feishu") } : PRESETS[k])}>
              {t(`oidcAdmin.presets.${k}`)}
            </Button>
          ))}
        </div>
        <p className="text-xs text-neutral-500">{t(`oidcAdmin.presetHints.${form.preset || "custom"}`)}</p>

        <div className="grid gap-4 sm:grid-cols-2">
          <Field label={t("oidcAdmin.name")}>
            <input className={inputCls} value={form.name} onChange={(e) => set({ name: e.target.value })} />
          </Field>
          <Field label={t("oidcAdmin.issuer")} hint={t("oidcAdmin.issuerHint")}>
            <input className={cx(inputCls, "font-mono")} value={form.issuer} onChange={(e) => set({ issuer: e.target.value })} placeholder="https://sso.acme.com/realms/acme" />
          </Field>
          <Field label={t("oidcAdmin.clientId")}>
            <input className={cx(inputCls, "font-mono")} value={form.clientId} onChange={(e) => set({ clientId: e.target.value })} autoComplete="off" />
          </Field>
          <Field label={t("oidcAdmin.clientSecret")}>
            <PasswordInput
              label={t("oidcAdmin.clientSecret")}
              className={inputCls}
              autoComplete="new-password"
              value={secret}
              onChange={(e) => setSecret(e.target.value)}
              placeholder={saved.clientSecretSet ? t("oidcAdmin.clientSecretSet") : ""}
            />
          </Field>
        </div>
        <Field label={t("oidcAdmin.redirectUri")}>
          <div className="flex items-center gap-2">
            <code className="min-w-0 flex-1 truncate rounded-md bg-neutral-50 px-3 py-2 text-xs text-neutral-800">{saved.redirectUri}</code>
            <Button
              size="sm"
              variant="secondary"
              onClick={async () => {
                if (!(await copyText(saved.redirectUri))) return notify(t("common.copyFailed"));
                notify(t("oidcAdmin.copied"));
              }}
            >
              <Copy size={14} /> {t("oidcAdmin.copy")}
            </Button>
          </div>
        </Field>
        <Field label={t("oidcAdmin.scopes")} hint={t("oidcAdmin.scopesHint")}>
          <input className={cx(inputCls, "font-mono")} value={form.scopes} onChange={(e) => set({ scopes: e.target.value })} />
        </Field>

        <SubHeading>{t("oidcAdmin.claims")}</SubHeading>
        <p className="text-xs text-neutral-500">{t("oidcAdmin.claimsHint")}</p>
        <div className="grid gap-4 sm:grid-cols-5">
          {(["usernameClaim", "nameClaim", "emailClaim", "departmentClaim", "groupsClaim"] as const).map((k) => (
            <Field key={k} label={t(`oidcAdmin.${k}`)}>
              <input className={cx(inputCls, "font-mono")} value={form[k]} onChange={(e) => set({ [k]: e.target.value })} />
            </Field>
          ))}
        </div>

        <div>
          <div className="text-sm font-medium text-neutral-800">{t("oidcAdmin.mappings")}</div>
          <p className="mt-0.5 mb-3 text-xs text-neutral-500">{t("oidcAdmin.mappingsDesc")}</p>
          {form.roleMappings.length === 0 && <p className="mb-3 text-sm text-neutral-600">{t("ldapAdmin.noMappings")}</p>}
          <div className="space-y-2">
            {form.roleMappings.map((m, i) => (
              <div key={i} className="grid grid-cols-[minmax(0,1fr)_12rem_2.25rem] items-center gap-2">
                <input
                  aria-label={t("ldapAdmin.mappingGroup")}
                  className={cx(inputBase, "w-full font-mono")}
                  placeholder={t("ldapAdmin.mappingGroup")}
                  value={m.group}
                  disabled={!canRoles}
                  onChange={(e) => set({ roleMappings: form.roleMappings.map((x, j) => (j === i ? { ...x, group: e.target.value } : x)) })}
                />
                {canRoles && roles.data ? (
                  <Combobox
                    className="w-full"
                    aria-label={t("ldapAdmin.mappingRole")}
                    placeholder={t("ldapAdmin.mappingRole")}
                    value={m.roleId ? String(m.roleId) : ""}
                    options={roleOptions}
                    onChange={(v) => set({ roleMappings: form.roleMappings.map((x, j) => (j === i ? { ...x, roleId: Number(v) } : x)) })}
                  />
                ) : (
                  <Badge>{roleName(m.roleId)}</Badge>
                )}
                {canRoles && (
                  <Button size="sm" variant="ghost" onClick={() => set({ roleMappings: form.roleMappings.filter((_, j) => j !== i) })} aria-label={t("ldapAdmin.removeMapping")}>
                    <X size={14} />
                  </Button>
                )}
              </div>
            ))}
          </div>
          {canRoles ? (
            <Button size="sm" variant="secondary" className="mt-3" onClick={() => set({ roleMappings: [...form.roleMappings, { group: "", roleId: 0 }] })}>
              <Plus size={14} /> {t("ldapAdmin.addMapping")}
            </Button>
          ) : (
            <p className="mt-2 text-xs text-neutral-500">{t("ldapAdmin.mappingsNeedRoles")}</p>
          )}
        </div>

        <ToggleRow label={t("oidcAdmin.autoCreate")} desc={t("oidcAdmin.autoCreateDesc")} checked={form.autoCreate} onChange={(v) => set({ autoCreate: v })} />
        <ToggleRow label={t("oidcAdmin.requireEmailVerified")} desc={t("oidcAdmin.requireEmailVerifiedDesc")} checked={form.requireEmailVerified} onChange={(v) => set({ requireEmailVerified: v })} />
        {form.preset === "feishu" && (
          <section className="space-y-4" data-feishu-sync>
            <SubHeading>{t("oidcAdmin.feishu.title")}</SubHeading>
            <p className="text-xs leading-5 text-neutral-500">{t("oidcAdmin.feishu.desc")}</p>
            <Field label={t("oidcAdmin.feishu.apiBase")} hint={t("oidcAdmin.feishu.apiBaseHint")}>
              <input className={cx(inputCls, "font-mono")} value={form.feishuApiBase ?? ""} onChange={(e) => set({ feishuApiBase: e.target.value })} />
            </Field>
            <ToggleRow label={t("oidcAdmin.feishu.autoSync")} desc={t("oidcAdmin.feishu.autoSyncDesc")} checked={!!form.feishuSync} onChange={(v) => set({ feishuSync: v })} />
            <ToggleRow label={t("oidcAdmin.feishu.syncDisable")} desc={t("oidcAdmin.feishu.syncDisableDesc")} checked={!!form.feishuSyncDisable} onChange={(v) => set({ feishuSyncDisable: v })} />
            <ToggleRow label={t("oidcAdmin.feishu.notify")} desc={t("oidcAdmin.feishu.notifyDesc")} checked={!!form.feishuNotify} onChange={(v) => set({ feishuNotify: v })} />
            <ToggleRow label={t("oidcAdmin.feishu.notifyActions")} desc={t("oidcAdmin.feishu.notifyActionsDesc")} checked={!!form.feishuNotifyActions} onChange={(v) => set({ feishuNotifyActions: v })} />
            {form.feishuNotifyActions && (
              <Field label={t("oidcAdmin.feishu.verificationToken")} hint={t("oidcAdmin.feishu.verificationTokenHint")}>
                <input className={cx(inputCls, "font-mono")} autoComplete="off" value={form.feishuVerificationToken ?? ""} onChange={(e) => set({ feishuVerificationToken: e.target.value })} />
              </Field>
            )}
            {form.feishuNotifyActions && (
              <Field label={t("oidcAdmin.feishu.encryptKey")} hint={form.feishuEncryptKeySet && form.feishuEncryptKey === undefined ? t("oidcAdmin.feishu.encryptKeySetHint") : t("oidcAdmin.feishu.encryptKeyHint")}>
                <input className={cx(inputCls, "font-mono")} type="password" autoComplete="new-password" value={form.feishuEncryptKey ?? ""} placeholder={form.feishuEncryptKeySet ? "••••••••" : ""} onChange={(e) => set({ feishuEncryptKey: e.target.value })} />
              </Field>
            )}
            <Field label={t("oidcAdmin.feishu.workspaceNext")} hint={t("oidcAdmin.feishu.workspaceNextHint")}>
              <div className="flex flex-wrap gap-2">
                <input className={cx(inputCls, "min-w-56 flex-1 font-mono")} value={workspaceNext} onChange={(e) => setWorkspaceNext(e.target.value)} placeholder="/a/team-outing" aria-label={t("oidcAdmin.feishu.workspaceNext")} />
                <Button type="button" variant="secondary" disabled={workspace.busy || dirty} onClick={() => void onWorkspace()}>{t("oidcAdmin.feishu.workspaceLink")}</Button>
              </div>
            </Field>
            {workspaceLink && <CopyBox text={workspaceLink} wrap />}
            <Field label={t("oidcAdmin.feishu.callbackLink")}>
              <CopyBox text={`${window.location.origin}/api/auth/feishu/card`} wrap />
            </Field>
            <p className="text-xs leading-5 text-neutral-500">{t("oidcAdmin.feishu.workbookNote")}</p>
            <div className="flex flex-wrap items-center gap-3">
              <Button type="button" variant="secondary" disabled={dirty || synchronize.busy || !form.feishuApiBase} onClick={() => void onSync()}>
                {synchronize.busy ? t("oidcAdmin.feishu.syncing") : t("oidcAdmin.feishu.syncNow")}
              </Button>
              {dirty && <span className="text-sm text-amber-700">{t("oidcAdmin.feishu.saveFirst")}</span>}
            </div>
            {syncStatus.data?.last && (
              <div className="space-y-1 rounded-md border border-neutral-200 p-3 text-xs text-neutral-600">
                <div>{t("oidcAdmin.feishu.lastAt", { time: syncStatus.data.last.at })}</div>
                {syncStatus.data.last.ok ? (
                  <div>{t("oidcAdmin.feishu.lastResult", { remote: syncStatus.data.last.remote, created: syncStatus.data.last.created, reconciled: syncStatus.data.last.reconciled, disabled: syncStatus.data.last.disabled.length, conflicts: syncStatus.data.last.conflicts.length })}</div>
                ) : (
                  <div className="text-red-700">{syncStatus.data.last.error}</div>
                )}
                {syncStatus.data.last.conflicts.length > 0 && <div className="break-all">{t("oidcAdmin.feishu.conflicts", { users: syncStatus.data.last.conflicts.join("、") })}</div>}
                {syncStatus.data.last.disabled.length > 0 && <div className="break-all">{t("oidcAdmin.feishu.disabled", { users: syncStatus.data.last.disabled.join("、") })}</div>}
                {(syncStatus.data.last.kept?.length ?? 0) > 0 && <div className="break-all text-amber-700">{t("oidcAdmin.feishu.kept", { users: syncStatus.data.last.kept!.join("、") })}</div>}
                {syncStatus.data.last.skipped.length > 0 && <div className="break-all">{t("oidcAdmin.feishu.skipped", { count: syncStatus.data.last.skipped.length })}</div>}
              </div>
            )}
          </section>
        )}
      </div>

      <ErrorNote>{saveError}</ErrorNote>
      <div className="flex flex-wrap items-center gap-3">
        <Button onClick={onSave} disabled={save.busy || !dirty}>
          {save.busy ? t("common.submitting") : t("oidcAdmin.save")}
        </Button>
        <Button variant="secondary" onClick={onTest} disabled={tester.busy || !form.issuer}>
          {tester.busy ? t("oidcAdmin.testing") : t("oidcAdmin.test")}
        </Button>
        {dirty && <span className="text-sm text-amber-700">{t("oidcAdmin.unsaved")}</span>}
      </div>
      {test &&
        (test.ok && test.provider ? (
          <div className="space-y-1.5 rounded-md border border-neutral-200 p-4 text-sm">
            <div className="flex items-center gap-2 text-neutral-900">
              <Check size={16} className="text-emerald-600" /> {t("oidcAdmin.testOk")}
            </div>
            <dl className="grid grid-cols-[9rem_1fr] gap-x-3 gap-y-1 font-mono text-xs break-all text-neutral-700">
              <dt className="text-neutral-500">authorization</dt>
              <dd>{test.provider.authorization_endpoint}</dd>
              <dt className="text-neutral-500">token</dt>
              <dd>{test.provider.token_endpoint}</dd>
              <dt className="text-neutral-500">jwks</dt>
              <dd>{test.provider.jwks_uri}</dd>
              <dt className="text-neutral-500">userinfo</dt>
              <dd>{test.provider.userinfo_endpoint || "—"}</dd>
            </dl>
          </div>
        ) : (
          <ErrorNote>{t("oidcAdmin.testFail", { error: test.error ?? "" })}</ErrorNote>
        ))}
    </div>
  );
}
