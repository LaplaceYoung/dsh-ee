import { useState } from "react";
import { useTranslation } from "react-i18next";
import { KeyRound, RefreshCw, ShieldCheck, Trash2 } from "lucide-react";
import { api, formatTime, type AssetSpec, type MyCredential, type UserAuth } from "../lib/api";
import { useAction, useApi } from "../lib/use-api";
import { useStore } from "../lib/store";
import { useConfirm } from "../lib/confirm";
import { Badge, Button, Card, CardHeader, CopyBox, cx, Empty, Field, inputCls, Notice, PasswordInput, Segmented } from "./ui";

// 按员工本人的身份调用企业系统（docs/plugins.md §7）：连接编辑页的设置、员工自己的凭据、管理员看到的名单和签名密钥。

const textareaCls = "w-full rounded-md border border-neutral-300 bg-card px-3 py-2 text-sm outline-none placeholder:text-neutral-500 focus:border-brand-500 focus:ring-2 focus:ring-brand-100";
const CLAIMS = ["dept", "roles", "idp_sub"] as const;

/** 连接编辑页里「员工身份」这一段。 */
export function UserAuthFields({ spec, set, slug }: { spec: AssetSpec; set: (p: Partial<AssetSpec>) => void; slug: string }) {
  const { t } = useTranslation();
  const ua = spec.user_auth;
  const mode = ua?.mode ?? "none";
  const info = useApi(api.eeJwtInfo);
  const patch = (p: Partial<UserAuth>) => set({ user_auth: { ...(ua ?? { mode: "ee_jwt" }), ...p } });
  return (
    <>
      <Field label={t("identity.mode")} hint={t("identity.modeHint")}>
        <Segmented
          value={mode}
          onChange={(v) => set({ user_auth: v === "none" ? undefined : { mode: v, header: ua?.header } })}
          options={[
            { value: "none", label: t("identity.none") },
            { value: "ee_jwt", label: t("identity.eeJwt") },
            { value: "user_secret", label: t("identity.userSecret") },
          ]}
        />
      </Field>
      {ua && (
        <div className="space-y-4 rounded-lg border border-neutral-200 bg-neutral-50/60 p-4">
          <p className="text-[13px] leading-relaxed text-neutral-600">{t(ua.mode === "ee_jwt" ? "identity.eeJwtDesc" : "identity.userSecretDesc")}</p>
          <div className="grid gap-4 sm:grid-cols-2">
            <Field label={t("identity.header")} hint={t("identity.headerHint")}>
              <input className={cx(inputCls, "font-mono")} value={ua.header ?? ""} onChange={(e) => patch({ header: e.target.value })} placeholder="Authorization: Bearer …" />
            </Field>
            {ua.mode === "ee_jwt" && (
              <Field label={t("identity.audience")} hint={t("identity.audienceHint")}>
                <input className={cx(inputCls, "font-mono")} value={ua.audience ?? ""} onChange={(e) => patch({ audience: e.target.value })} placeholder={slug || "oa"} />
              </Field>
            )}
          </div>
          {ua.mode === "ee_jwt" ? (
            <>
              <Field label={t("identity.claims")} hint={t("identity.claimsHint")}>
                <div className="flex flex-wrap gap-x-5 gap-y-2">
                  {CLAIMS.map((c) => {
                    const on = ua.claims?.includes(c) ?? false;
                    return (
                      <label key={c} className="flex items-center gap-2 text-sm text-neutral-800">
                        <input
                          type="checkbox"
                          className="size-4 accent-brand-600"
                          checked={on}
                          onChange={() => patch({ claims: on ? (ua.claims ?? []).filter((x) => x !== c) : [...(ua.claims ?? []), c] })}
                        />
                        {t(`identity.claim.${c}`)}
                      </label>
                    );
                  })}
                </div>
              </Field>
              {info.data && (
                <div className="grid gap-3 sm:grid-cols-2">
                  <div>
                    <div className="mb-1 text-xs text-neutral-500">{t("identity.issuer")}</div>
                    <CopyBox text={info.data.issuer} wrap />
                  </div>
                  <div>
                    <div className="mb-1 text-xs text-neutral-500">{t("identity.jwks")}</div>
                    <CopyBox text={info.data.jwks_url} wrap />
                  </div>
                </div>
              )}
            </>
          ) : (
            <Field label={t("identity.hint")} hint={t("identity.hintHint")}>
              <textarea className={cx(textareaCls, "h-16")} value={ua.hint ?? ""} onChange={(e) => patch({ hint: e.target.value })} placeholder={t("identity.hintPlaceholder")} />
            </Field>
          )}
        </div>
      )}
    </>
  );
}

/** 连接详情页上员工看到的「我的凭据」。 */
export function MyCredentialCard({ assetId, cred, reload }: { assetId: number; cred: MyCredential; reload: () => void }) {
  const { t } = useTranslation();
  const { notify } = useStore();
  const { busy, run } = useAction();
  const confirm = useConfirm();
  const [secret, setSecret] = useState("");
  if (cred.mode === "ee_jwt") {
    return (
      <Card>
        <CardHeader title={t("identity.mineTitle")} aside={<Badge tone="green">{t("identity.asYou")}</Badge>} />
        <p className="flex items-start gap-2 px-5 pb-5 text-sm text-neutral-600">
          <ShieldCheck size={16} className="mt-0.5 shrink-0 text-neutral-400" />
          {t("identity.mineEeJwt")}
        </p>
      </Card>
    );
  }
  const save = async () => {
    const r = await run(() => api.setMyCredential(assetId, secret.trim()));
    notify(r.ok ? t("identity.saved") : r.error);
    if (r.ok) {
      setSecret("");
      reload();
    }
  };
  return (
    <Card>
      <CardHeader
        title={t("identity.mineTitle")}
        aside={cred.filled ? <Badge tone="green">{t("identity.filled", { tail: cred.tail })}</Badge> : <Badge tone="amber">{t("identity.notFilled")}</Badge>}
      />
      <div className="space-y-3 px-5 pb-5">
        <p className="text-sm text-neutral-600">{t("identity.mineSecret")}</p>
        {cred.hint && <Notice>{cred.hint}</Notice>}
        <div className="flex flex-wrap items-start gap-2">
          <div className="min-w-0 flex-1 basis-64">
            <PasswordInput
              label={t("identity.mineTitle")}
              className={inputCls}
              autoComplete="new-password"
              value={secret}
              onChange={(e) => setSecret(e.target.value)}
              placeholder={cred.filled ? t("identity.replacePlaceholder") : t("identity.fillPlaceholder")}
            />
          </div>
          <Button disabled={busy || !secret.trim()} onClick={() => void save()}>
            <KeyRound size={14} /> {cred.filled ? t("identity.replace") : t("identity.save")}
          </Button>
          {cred.filled && (
            <Button
              variant="secondary"
              disabled={busy}
              onClick={async () => {
                if (!(await confirm({ kind: "delete", description: t("identity.deleteConfirm") }))) return;
                const r = await run(() => api.deleteMyCredential(assetId));
                notify(r.ok ? t("identity.deleted") : r.error);
                if (r.ok) reload();
              }}
            >
              <Trash2 size={14} /> {t("identity.delete")}
            </Button>
          )}
        </div>
        {cred.filled && cred.updated_at && <p className="text-xs text-neutral-500">{t("identity.updatedAt", { time: formatTime(cred.updated_at) })}</p>}
      </div>
    </Card>
  );
}

/** 连接详情页上管理员看到的：哪些员工填了自己的凭据，可以逐个收回。看不到凭据内容。 */
export function CredentialHolders({ assetId }: { assetId: number }) {
  const { t } = useTranslation();
  const { notify } = useStore();
  const { busy, run } = useAction();
  const confirm = useConfirm();
  const list = useApi(() => api.connCredentials(assetId));
  const holders = list.data ?? [];
  return (
    <Card>
      <CardHeader title={t("identity.holdersTitle")} aside={<span className="text-xs text-neutral-500">{t("identity.holdersCount", { count: holders.length })}</span>} />
      {holders.length === 0 ? (
        <Empty compact title={t("identity.holdersEmpty")}>{t("identity.holdersEmptyDesc")}</Empty>
      ) : (
        <div className="overflow-x-auto">
          <table className="tbl">
            <tbody>
              {holders.map((h) => (
                <tr key={h.user_id} className="border-t border-neutral-100">
                  <td className="px-5 py-2">
                    <div className="text-neutral-900">{h.display_name || h.username}</div>
                    <div className="text-xs text-neutral-500">{[h.username, h.department].filter(Boolean).join(" · ")}</div>
                  </td>
                  <td className="px-5 py-2 whitespace-nowrap text-neutral-600 tabular-nums">{formatTime(h.updated_at)}</td>
                  <td className="px-5 py-2 text-right">
                    <Button
                      size="sm"
                      variant="secondary"
                      disabled={busy}
                      onClick={async () => {
                        if (!(await confirm({ kind: "revoke", description: t("identity.revokeConfirm", { name: h.display_name || h.username }) }))) return;
                        const r = await run(() => api.revokeConnCredential(assetId, h.user_id));
                        notify(r.ok ? t("identity.revoked", { name: h.display_name || h.username }) : r.error);
                        if (r.ok) list.reload();
                      }}
                    >
                      {t("identity.revoke")}
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </Card>
  );
}

/** 企业连接页上的 ee_jwt 签名密钥：签发方、JWKS 地址、钥匙列表、轮换。 */
export function EeJwtKeysCard() {
  const { t } = useTranslation();
  const { notify } = useStore();
  const { busy, run } = useAction();
  const confirm = useConfirm();
  const info = useApi(api.eeJwtInfo);
  if (!info.data) return null;
  const d = info.data;
  return (
    <Card className="mt-6">
      <CardHeader
        title={t("identity.keysTitle")}
        aside={
          <Button
            size="sm"
            variant="secondary"
            disabled={busy}
            onClick={async () => {
              if (!(await confirm({ kind: "rotate", description: t("identity.rotateConfirm", { minutes: Math.round(d.rotate_delay_seconds / 60) }) }))) return;
              const r = await run(api.rotateEeJwt);
              notify(r.ok ? t("identity.rotated") : r.error);
              if (r.ok) info.reload();
            }}
          >
            <RefreshCw size={14} /> {t("identity.rotate")}
          </Button>
        }
      />
      <div className="space-y-4 px-5 pb-5">
        <p className="text-sm text-neutral-600">{t("identity.keysDesc", { minutes: Math.round(d.lifetime_seconds / 60) })}</p>
        <div className="grid gap-3 sm:grid-cols-2">
          <div>
            <div className="mb-1 text-xs text-neutral-500">{t("identity.issuer")}</div>
            <CopyBox text={d.issuer} wrap />
          </div>
          <div>
            <div className="mb-1 text-xs text-neutral-500">{t("identity.jwks")}</div>
            <CopyBox text={d.jwks_url} wrap />
          </div>
        </div>
        <ul className="space-y-1.5 text-sm">
          {d.keys.map((k) => (
            <li key={k.kid} className="flex flex-wrap items-center gap-x-3 gap-y-1">
              <span className="font-mono text-xs text-neutral-800">{k.kid}</span>
              {k.state === "active" && <Badge tone="green">{t("identity.keyActive")}</Badge>}
              {k.state === "pending" && <Badge tone="amber">{t("identity.keyPending", { time: formatTime(k.active_at) })}</Badge>}
              {k.state === "retired" && <Badge>{t("identity.keyRetired")}</Badge>}
              <span className="text-xs text-neutral-500">{t("identity.keyCreated", { time: formatTime(k.created_at) })}</span>
            </li>
          ))}
        </ul>
      </div>
    </Card>
  );
}
