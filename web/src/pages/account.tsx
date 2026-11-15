import { useState, type FormEvent } from "react";
import { useNavigate } from "react-router";
import { api } from "../lib/api";
import { useAuth } from "../lib/auth";
import { useAction } from "../lib/use-api";
import { ErrorNote } from "../components/api-ui";
import { Button, Card, Field, inputCls, PageHeader, PasswordInput } from "../components/ui";
import { LangSwitch } from "../components/lang-switch";
import { ThemeToggle } from "../components/theme-toggle";
import { useTranslation } from "react-i18next";
import { useLabels } from "../lib/labels";

export default function Account() {
  const { t } = useTranslation();
  const L = useLabels();
  const { user, logout, clear } = useAuth();
  const navigate = useNavigate();
  const { busy, run } = useAction();
  const [oldPw, setOldPw] = useState("");
  const [newPw, setNewPw] = useState("");
  const [confirmPw, setConfirmPw] = useState("");
  const [error, setError] = useState<string | null>(null);

  if (!user) return null;

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setError(null);
    if (newPw !== confirmPw) return setError(t("account.mismatch"));
    const r = await run(() => api.changePassword(oldPw, newPw));
    if (!r.ok) return setError(r.error);
    // 服务端已让所有会话失效，回登录页用新密码登录
    clear();
    navigate("/login?next=%2Faccount", { replace: true });
  };

  return (
    <div className="max-w-2xl">
      <PageHeader
        title={t("account.title")}
        actions={
          <Button
            variant="secondary"
            size="sm"
            onClick={() => void logout()}
          >
            {t("account.logout")}
          </Button>
        }
      />
      <Card className="p-5">
        <dl className="grid grid-cols-[6rem_1fr] gap-y-2 text-sm">
          <dt className="text-neutral-600">{t("account.username")}</dt>
          <dd className="text-neutral-900">{user.username}</dd>
          <dt className="text-neutral-600">{t("account.displayName")}</dt>
          <dd className="text-neutral-900">{user.display_name || "—"}</dd>
          <dt className="text-neutral-600">{t("account.department")}</dt>
          <dd className="text-neutral-900">{user.department || "—"}</dd>
          {user.email && (
            <>
              <dt className="text-neutral-600">{t("account.email")}</dt>
              <dd className="text-neutral-900">{user.email}</dd>
            </>
          )}
          <dt className="text-neutral-600">{t("account.source")}</dt>
          <dd className="text-neutral-900">
            {user.auth_source === "ldap" ? t("account.sourceLdap") : user.auth_source === "oidc" ? t("ssoLogin.sourceOidc") : t("account.sourceLocal")}
          </dd>
          <dt className="text-neutral-600">{t("account.roles")}</dt>
          <dd className="text-neutral-900">{(user.roles ?? []).map((r) => L.role(r)).join(", ") || t("shell.noRoles")}</dd>
        </dl>
      </Card>

      <Card className="mt-4 p-5">
        <h2 className="text-neutral-900 text-[15px] leading-6 font-semibold">{t("account.preferences")}</h2>
        <p className="mt-2 text-sm text-neutral-600">{t("account.preferencesHint")}</p>
        <div className="mt-4 flex flex-wrap items-center gap-x-8 gap-y-3">
          <div className="flex items-center gap-2">
            <span className="text-sm text-neutral-600">{t("account.language")}</span>
            <LangSwitch />
          </div>
          <div className="flex items-center gap-2">
            <span className="text-sm text-neutral-600">{t("account.theme")}</span>
            <ThemeToggle />
          </div>
        </div>
      </Card>

      {user.auth_source !== "local" ? (
        <Card className="mt-4 p-5">
          <h2 className="text-neutral-900 text-[15px] leading-6 font-semibold">{t("account.changePassword")}</h2>
          <p className="mt-2 text-sm text-neutral-600">{user.auth_source === "oidc" ? t("ssoLogin.oidcManaged") : t("account.ldapManaged")}</p>
        </Card>
      ) : (
        <Card className="mt-4 p-5">
          <form onSubmit={submit} className="space-y-4">
            <h2 className="text-neutral-900 text-[15px] leading-6 font-semibold">{t("account.changePassword")}</h2>
            <Field label={t("account.currentPassword")}>
              <PasswordInput label={t("account.currentPassword")} className={inputCls} autoComplete="current-password" required value={oldPw} onChange={(e) => setOldPw(e.target.value)} />
            </Field>
            <Field label={t("account.newPassword")} hint={t("account.newPasswordHint")}>
              <PasswordInput label={t("account.newPassword")} className={inputCls} autoComplete="new-password" required value={newPw} onChange={(e) => setNewPw(e.target.value)} />
            </Field>
            <Field label={t("account.confirmPassword")}>
              <PasswordInput label={t("account.confirmPassword")} className={inputCls} autoComplete="new-password" required value={confirmPw} onChange={(e) => setConfirmPw(e.target.value)} />
            </Field>
            <ErrorNote>{error}</ErrorNote>
            <Button type="submit" disabled={busy || !oldPw || !newPw}>
              {busy ? t("common.submitting") : t("account.changePassword")}
            </Button>
          </form>
        </Card>
      )}
    </div>
  );
}
