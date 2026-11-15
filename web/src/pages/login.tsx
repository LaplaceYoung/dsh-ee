import { useEffect, useState, type FormEvent } from "react";
import { Link, Navigate, useNavigate, useSearchParams } from "react-router";
import { WhaleMark } from "../components/whale-mark";
import { useAuth } from "../lib/auth";
import { api } from "../lib/api";
import { useApi } from "../lib/use-api";
import { Button, buttonCls, inputCls, Notice, PasswordInput, Spinner } from "../components/ui";
import { ErrorNote } from "../components/api-ui";
import { LangSwitch } from "../components/lang-switch";
import { ThemeToggle } from "../components/theme-toggle";
import { DemoResetCountdown } from "../components/demo-reset";
import { DemoConnectDsh, DemoPersonas } from "../components/demo-site";
import type { DemoPersona } from "../lib/api";
import { useTranslation } from "react-i18next";

/** 只允许站内相对路径，防止 next 参数被拿来做开放重定向。 */
function safeNext(raw: string | null) {
  return raw && raw.startsWith("/") && !raw.startsWith("//") ? raw : "/go";
}

export default function Login() {
  const { t } = useTranslation();
  const { user, loading, error: bootError, login, logout, refresh } = useAuth();
  const [params] = useSearchParams();
  const navigate = useNavigate();
  const next = safeNext(params.get("next"));
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [signingOut, setSigningOut] = useState(false);
  const [error, setError] = useState<string | null>(null);
  // 演示站：点了哪张场景卡片、登录后去哪
  const [picking, setPicking] = useState<string | null>(null);
  const [pickError, setPickError] = useState<string | null>(null);
  const [dest, setDest] = useState<string | null>(null);
  const methods = useApi(api.authMethods);
  const ldap = !!methods.data?.ldap;
  const sso = methods.data?.oidc ?? null;
  const demo = methods.data?.demo ?? null;
  // 企业统一登录失败时，回调把原因放在 oidc_error 里带回来
  const ssoError = params.get("oidc_error");
  // 从 DSH 自动登录的会话只带员工权限：带 reauth=1 进来时照常显示登录表单
  const reauth = params.get("reauth") === "1" && !!user?.limited;
  const handoffExpired = params.get("handoff") === "expired";
  // 浏览器里已经登录着别的账号：handoff 不会替换会话，只把人送回登录页（见 /api/auth/handoff）。
  // 这时不能自动跳走，否则提示没机会显示；也不自动登出，由用户点「退出当前账号」。
  const handoffOtherSession = params.get("handoff") === "other-session";
  useEffect(() => {
    if (reauth && user) setUsername((current) => current || user.username);
  }, [reauth, user]);

  if (!loading && user && !reauth && !handoffOtherSession) return <Navigate to={dest ?? next} replace />;

  const signOut = async () => {
    setSigningOut(true);
    try {
      await logout();
    } finally {
      setSigningOut(false);
    }
  };

  const pick = async (p: DemoPersona) => {
    setPicking(p.username);
    setPickError(null);
    try {
      await api.demoLogin(p.username);
      // 从设备授权等页面跳过来的（带 next），登录后回到原处；否则直接打开这个人常用的应用
      setDest(params.get("next") ? next : `/a/${encodeURIComponent(p.app)}`);
      await refresh();
    } catch (err) {
      setPickError(err instanceof Error ? err.message : String(err));
    } finally {
      setPicking(null);
    }
  };
  const personas = demo?.personas ?? [];

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      await login(username.trim(), password);
      navigate(next, { replace: true });
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="auth-bg relative flex min-h-screen flex-col items-center justify-center overflow-hidden px-4 py-10">
      <div aria-hidden className="auth-stripe pointer-events-none absolute inset-x-0 top-0 h-[52vh] min-h-80 dark:opacity-80" />
      <Link to="/" className="relative mb-7 flex items-center gap-2.5 rounded-md text-white">
        <WhaleMark size={36} />
        <span className="text-xl font-bold tracking-[-0.01em]">dsh-ee</span>
      </Link>
      {personas.length > 0 && (
        <section className="relative mb-10 w-full max-w-3xl">
          <div className="mb-6 text-center text-white">
            <h1 className="text-2xl leading-9 font-bold tracking-[-0.02em] sm:text-[28px]">{t("login.demo.heroTitle")}</h1>
            <p className="mx-auto mt-2 max-w-2xl text-sm leading-6 text-white/85">{t("login.demo.heroSubtitle")}</p>
          </div>
          {pickError && (
            <div className="mb-3">
              <ErrorNote>{pickError}</ErrorNote>
            </div>
          )}
          <DemoPersonas personas={personas} busy={picking} onPick={pick} />
          <DemoConnectDsh className="mt-3" />
          <p className="mt-8 text-center text-sm font-medium text-neutral-600 dark:text-neutral-400">{t("login.demo.adminDivider")}</p>
        </section>
      )}
      <form onSubmit={submit} className="relative w-full max-w-[400px] rounded-xl bg-card p-7 shadow-modal sm:p-8">
        <h1 className="text-[22px] leading-7 font-bold tracking-[-0.02em] text-neutral-900">{t("login.title")}</h1>
        <p className="mt-1.5 text-sm leading-6 text-neutral-600">{ldap ? t("login.ldapHint") : t("login.subtitle")}</p>
        {(reauth || handoffExpired || handoffOtherSession) && (
          <Notice role="status" className="mt-4">
            {t(reauth ? "login.reauth" : handoffOtherSession ? "login.handoffOtherSession" : "login.handoffExpired")}
            {handoffOtherSession && user && (
              <div className="mt-2">
                <Button type="button" size="sm" variant="secondary" disabled={signingOut} onClick={() => void signOut()}>
                  {t("shell.logout")}
                </Button>
              </div>
            )}
          </Notice>
        )}
        {demo && (
          <Notice role="status" className="mt-4">
            <p className="font-medium">{t("login.demo.title")}</p>
            <p className="mt-1 font-mono text-[13px]">{t("login.demo.account", { username: demo.username, password: demo.password })}</p>
            {demo.reset && <p className="mt-1">{t("login.demo.reset", { reset: demo.reset })}</p>}
            {demo.next_reset && (
              <p className="mt-1 font-medium">
                <DemoResetCountdown at={demo.next_reset} />
              </p>
            )}
            <Button
              type="button"
              size="sm"
              variant="secondary"
              className="mt-2"
              onClick={() => {
                setUsername(demo.username);
                setPassword(demo.password);
              }}
            >
              {t("login.demo.fill")}
            </Button>
          </Notice>
        )}
        {sso && (
          <>
            <a href={`/api/auth/oidc/start?next=${encodeURIComponent(next)}`} className={buttonCls("primary", "lg", "mt-6 w-full")}>
              {t("ssoLogin.button", { name: sso.name })}
            </a>
            <div className="mt-5 flex items-center gap-3 text-xs text-neutral-500">
              <span className="h-px flex-1 bg-neutral-200" />
              {t("ssoLogin.or")}
              <span className="h-px flex-1 bg-neutral-200" />
            </div>
          </>
        )}
        <div className={sso ? "mt-4 space-y-4" : "mt-6 space-y-4"}>
          <label className="block">
            <span className="mb-1.5 block text-[13px] font-medium text-neutral-700">{t("login.username")}</span>
            <input
              className={inputCls}
              autoComplete="username"
              autoFocus
              required
              value={username}
              onChange={(e) => setUsername(e.target.value)}
            />
          </label>
          <label className="block">
            <span className="mb-1.5 block text-[13px] font-medium text-neutral-700">{t("login.password")}</span>
            <PasswordInput
              label={t("login.password")}
              className={inputCls}
              autoComplete="current-password"
              required
              value={password}
              onChange={(e) => setPassword(e.target.value)}
            />
          </label>
        </div>
        {(error ?? bootError ?? ssoError) && (
          <div className="mt-4">
            <ErrorNote>{error ?? bootError ?? t("ssoLogin.failed", { error: ssoError })}</ErrorNote>
          </div>
        )}
        <Button type="submit" size="lg" variant={sso ? "secondary" : "primary"} className="mt-6 w-full" disabled={busy || !username || !password}>
          {busy && <Spinner />}
          {busy ? t("login.submitting") : t("login.submit")}
        </Button>
      </form>
      <p className="relative mt-6 text-center text-[13px] text-neutral-500">{ldap ? t("login.ldapForgot") : t("login.forgot")}</p>
      <div className="relative mt-3 flex items-center justify-center gap-2">
        <LangSwitch />
        <ThemeToggle />
      </div>
    </div>
  );
}
