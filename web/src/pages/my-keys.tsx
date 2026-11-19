import { useEffect, useState, type FormEvent } from "react";
import { useSearchParams } from "react-router";
import { useTranslation } from "react-i18next";
import { api, formatTime, formatTokens, type TokenClientKind, type TokenScope, type TokenView } from "../lib/api";
import { useAction, useApi } from "../lib/use-api";
import { useStore } from "../lib/store";
import { useAuth } from "../lib/auth";
import { useConfirm } from "@/lib/confirm";
import { ErrorNote, LoadState } from "../components/api-ui";
import { Badge, Button, Card, CardHeader, CopyBox, Field, inputCls, PageHeader, PasswordInput, Empty } from "../components/ui";

const scopeOptions: TokenScope[] = ["models", "registry:read", "registry:publish", "hub", "connections", "agent"];

// 员工自己就有的权限点；账号有这之外的权限点才算有管理权限，才能开管理模式
const employeePerms = ["portal.view", "llm.use", "keys.self", "registry.use", "registry.publish"];

function stateOf(token: TokenView) {
  if (token.revoked_at) return "revoked";
  if (token.expires_at && new Date(token.expires_at) <= new Date()) return "expired";
  return "active";
}

export default function MyTokens() {
  const { t } = useTranslation();
  const confirm = useConfirm();
  const { notify, settings } = useStore();
  const { data, error, loading, reload } = useApi(api.myTokens);
  const { busy, run } = useAction();
  const { user, can } = useAuth();
  // config（dsh-ee config 命令行：导出、试算、提交权限配置变更）只给能管理字段或角色的人，和服务端签发时的检查一致
  const scopeChoices: TokenScope[] = can("fields.manage", "roles.manage") ? [...scopeOptions, "config"] : scopeOptions;
  const canAdmin = !!user && (user.is_super || user.permissions.some((p) => !employeePerms.includes(p)));
  const [params, setParams] = useSearchParams();
  const [elevating, setElevating] = useState<string | null>(null);
  const [password, setPassword] = useState("");
  const [hours, setHours] = useState<1 | 8>(1);
  const [elevateError, setElevateError] = useState<string | null>(null);
  const [creating, setCreating] = useState(false);
  const [issued, setIssued] = useState<string | null>(null);
  const [name, setName] = useState("");
  const [scopes, setScopes] = useState<TokenScope[]>(["models", "registry:read", "hub", "connections"]);
  // 默认按第三方 Harness 处理：写操作始终人工确认；DSH 用户可显式选 dsh
  const [clientKind, setClientKind] = useState<TokenClientKind>("third_party");
  const [limit, setLimit] = useState("");
  const [days, setDays] = useState("");
  const [formError, setFormError] = useState<string | null>(null);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setFormError(null);
    const r = await run(() => api.createMyToken({
      name: name.trim(), scopes, client_kind: clientKind,
      ...(limit ? { monthly_token_limit: Number(limit) } : {}),
      ...(days ? { expires_in_days: Number(days) } : {}),
    }));
    if (!r.ok) return setFormError(r.error);
    setIssued(r.value.token);
    setCreating(false);
    setName("");
    setClientKind("third_party");
    reload();
  };

  // Agent 给的链接形如 /tokens?elevate=at:12：直接打开这把令牌的开启表单
  const wanted = params.get("elevate");
  useEffect(() => {
    if (wanted && data?.some((token) => token.id === wanted && stateOf(token) === "active")) setElevating(wanted);
  }, [wanted, data]);

  const closeElevate = () => {
    setElevating(null);
    setPassword("");
    setElevateError(null);
    if (params.has("elevate")) setParams((cur) => { cur.delete("elevate"); return cur; }, { replace: true });
  };

  const elevate = async (e: FormEvent) => {
    e.preventDefault();
    if (!elevating) return;
    setElevateError(null);
    const r = await run(() => api.elevateToken(elevating, password, hours));
    if (!r.ok) return setElevateError(r.error);
    notify(t("tokens.adminOn", { time: formatTime(r.value.admin_until) }));
    closeElevate();
    reload();
  };

  const endAdmin = async (token: TokenView) => {
    const r = await run(() => api.endElevation(token.id));
    notify(r.ok ? t("tokens.adminEnded") : r.error);
    if (r.ok) reload();
  };

  const revoke = async (token: TokenView) => {
    if (!(await confirm({ kind: "revoke", description: t("tokens.revokeConfirm", { name: token.name }) }))) return;
    const r = await run(() => api.revokeMyToken(token.id));
    notify(r.ok ? t("tokens.revoked", { name: token.name }) : r.error);
    if (r.ok) reload();
  };

  return (
    <div>
      <PageHeader
        title={t("tokens.myTitle")}
        desc={t("tokens.myDesc")}
        actions={settings.connect.allowManualTokens && <Button size="sm" onClick={() => setCreating((v) => !v)}>{creating ? t("common.close") : t("tokens.create")}</Button>}
      />
      {issued && <Card className="mb-4 p-5"><h2 className="mb-2 text-[15px] leading-6 font-semibold text-neutral-900">{t("tokens.issuedTitle")}</h2><p className="mb-3 text-sm text-neutral-600">{t("tokens.issuedHint")}</p><CopyBox text={issued} /></Card>}
      {creating && (
        <Card className="mb-4"><CardHeader title={t("tokens.create")} />
          <form onSubmit={submit} className="space-y-4 px-5 pb-5">
            <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
              <Field label={t("tokens.name")}><input className={inputCls} required value={name} onChange={(e) => setName(e.target.value)} /></Field>
              <Field label={t("tokens.clientKind")}>
                <select aria-label={t("tokens.clientKind")} className={inputCls} value={clientKind} onChange={(e) => setClientKind(e.target.value as TokenClientKind)}>
                  {(["dsh", "third_party"] as const).map((kind) => <option key={kind} value={kind}>{t(`tokens.clientKinds.${kind}`)}</option>)}
                </select>
              </Field>
              <Field label={t("tokens.limit")}><input className={inputCls} type="number" min={0} value={limit} onChange={(e) => setLimit(e.target.value)} placeholder={t("tokens.unlimited")} /></Field>
              <Field label={t("tokens.days")}><input className={inputCls} type="number" min={0} max={3650} value={days} onChange={(e) => setDays(e.target.value)} placeholder={t("tokens.noExpiry")} /></Field>
            </div>
            <p className="-mt-2 text-xs leading-5 text-neutral-600">{t(`tokens.clientKindHints.${clientKind}`)}</p>
            <div className="flex flex-wrap gap-4 text-sm">
              {scopeChoices.map((scope) => <label key={scope} className="flex items-center gap-1.5"><input type="checkbox" checked={scopes.includes(scope)} onChange={(e) => setScopes((cur) => e.target.checked ? [...cur, scope] : cur.filter((x) => x !== scope))} />{t(`tokens.scopes.${scope}`)}</label>)}
            </div>
            <ErrorNote>{formError}</ErrorNote>
            <Button type="submit" disabled={busy || !name.trim() || scopes.length === 0}>{t("tokens.create")}</Button>
          </form>
        </Card>
      )}
      {elevating && (
        <Card className="mb-4"><CardHeader title={t("tokens.adminTitle")} />
          <form onSubmit={elevate} className="space-y-4 px-5 pb-5">
            <p className="text-sm text-neutral-600">{t("tokens.adminModeDesc", { name: data?.find((token) => token.id === elevating)?.name ?? elevating })}</p>
            <div className="grid gap-4 sm:grid-cols-2">
              <Field label={t("tokens.adminPassword")}><PasswordInput label={t("tokens.adminPassword")} className={inputCls} autoComplete="current-password" required autoFocus value={password} onChange={(e) => setPassword(e.target.value)} /></Field>
              <Field label={t("tokens.adminDuration")}>
                <div className="flex gap-4 pt-2 text-sm">
                  {([1, 8] as const).map((h) => <label key={h} className="flex items-center gap-1.5"><input type="radio" name="hours" checked={hours === h} onChange={() => setHours(h)} />{t("tokens.adminHours", { count: h })}</label>)}
                </div>
              </Field>
            </div>
            <ErrorNote>{elevateError}</ErrorNote>
            <div className="flex gap-3"><Button type="submit" disabled={busy || !password}>{t("tokens.adminConfirm")}</Button><Button type="button" variant="secondary" onClick={closeElevate}>{t("common.close")}</Button></div>
          </form>
        </Card>
      )}
      <Card>
        <CardHeader title={t("tokens.list")} aside={data ? t("tokens.count", { count: data.length }) : undefined} />
        <LoadState loading={loading && !data} error={error} onRetry={reload}>
          <div className="overflow-x-auto">
            <table className="tbl max-sm:min-w-[40rem]">
              <thead><tr><th className="px-5 py-2">{t("tokens.name")}</th><th className="px-5 py-2">{t("tokens.source")}</th><th className="px-5 py-2">{t("tokens.clientKind")}</th><th className="px-5 py-2">{t("tokens.scopesTitle")}</th><th className="px-5 py-2 text-right">{t("tokens.usage")}</th><th className="px-5 py-2 text-right">{t("tokens.lastUsed")}</th><th className="px-5 py-2">{t("tokens.status")}</th><th className="px-5 py-2" /></tr></thead>
              <tbody>
                {(data ?? []).map((token) => {
                  const state = stateOf(token);
                  return <tr key={token.id} className="border-t border-neutral-100 align-top">
                    <td className="px-5 py-3"><div className="text-neutral-900">{token.name}</div><div className="font-mono text-xs text-neutral-600">{token.prefix}…</div>{token.client_name && <div className="text-xs text-neutral-600">{token.client_name}</div>}</td>
                    <td className="px-5 py-3 text-neutral-600">{t(`tokens.sources.${token.source}`)}</td>
                    <td className="px-5 py-3"><Badge tone={token.client_kind === "dsh" ? "brand" : "slate"}>{t(`tokens.clientKinds.${token.client_kind ?? "third_party"}`)}</Badge></td>
                    <td className="max-w-72 px-5 py-3 text-xs text-neutral-600">{token.scopes.map((scope) => t(`tokens.scopes.${scope}`)).join(" · ") || "—"}</td>
                    <td className="px-5 py-3 text-right tabular-nums">{formatTokens(token.month_used)}{token.monthly_token_limit !== null && ` / ${formatTokens(token.monthly_token_limit)}`}</td>
                    <td className="px-5 py-3 text-right tabular-nums text-neutral-600">{formatTime(token.last_used_at)}</td>
                    <td className="px-5 py-3"><Badge tone={state === "active" ? "green" : state === "expired" ? "amber" : "slate"}>{t(`tokens.states.${state}`)}</Badge></td>
                    <td className="px-5 py-3 text-right whitespace-nowrap">
                      {state === "active" && token.kind === "access" && canAdmin && (token.admin_until
                        ? <><Badge tone="amber">{t("tokens.adminUntil", { time: formatTime(token.admin_until) })}</Badge> <button type="button" className="mr-3 text-sm text-neutral-600 hover:text-neutral-900" onClick={() => endAdmin(token)}>{t("tokens.adminEnd")}</button></>
                        : <button type="button" className="mr-3 text-sm text-neutral-600 hover:text-neutral-900" onClick={() => { setElevating(token.id); setElevateError(null); }}>{t("tokens.adminStart")}</button>)}
                      {state === "active" && <button type="button" className="text-sm text-neutral-600 hover:text-red-700" onClick={() => revoke(token)}>{t("tokens.revoke")}</button>}
                    </td>
                  </tr>;
                })}
              </tbody>
            </table>
            {data?.length === 0 && <Empty compact>{t("tokens.empty")}</Empty>}
          </div>
        </LoadState>
      </Card>
    </div>
  );
}
