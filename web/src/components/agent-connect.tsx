import { useEffect, useState } from "react";
import { Check, Loader2 } from "lucide-react";
import { useTranslation } from "react-i18next";
import { ApiError, api, formatTime, type GetStarted, type GetStartedStatus } from "../lib/api";
import { useAction } from "../lib/use-api";
import { PageLink } from "../lib/menus";
import { Button, CopyBox, cx, inputCls, PasswordInput, Segmented, Notice, Spinner } from "./ui";

type Path = "prompt" | "manual";

/**
 * 「开始使用」的 Agent 接入：生成接入码和一段提示词，用户贴进 DSH；DSH 用 /ee-login <接入码> 发起的
 * 登录会出现在这里，就地确认。管理员接着在这里开管理模式，再把第二段话贴回 DSH，让 Agent 接着配。
 */
export function AgentConnect({ admin, onConnected }: { admin: boolean; onConnected?: () => void }) {
  const { t } = useTranslation();
  const { busy, run } = useAction();
  const [info, setInfo] = useState<GetStarted | null>(null);
  const [status, setStatus] = useState<GetStartedStatus | null>(null);
  const [expired, setExpired] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [path, setPath] = useState<Path>("prompt");
  const [password, setPassword] = useState("");
  const [hours, setHours] = useState<1 | 8>(1);

  const issue = async () => {
    setError(null);
    const r = await run(() => api.getStarted(admin));
    if (!r.ok) return setError(r.error);
    setInfo(r.value);
    setStatus(null);
    setExpired(false);
  };

  useEffect(() => {
    void issue();
  }, [admin]);

  // 轮询接入进度；接入完成且（管理员）管理模式已开后就不用再问了
  const settled = status?.stage === "connected" && (!admin || status.admin_live);
  useEffect(() => {
    if (!info || expired || settled) return;
    let stop = false;
    const tick = async () => {
      try {
        const st = await api.getStartedStatus(info.code);
        if (!stop) setStatus(st);
      } catch (error) {
        // 只有「接入码不存在/不属于当前用户」才是过期；5xx/网络抖动等下一轮再试。
        if (!stop && error instanceof ApiError && error.status === 404) setExpired(true);
      }
    };
    void tick();
    const timer = setInterval(() => void tick(), 2500);
    return () => {
      stop = true;
      clearInterval(timer);
    };
  }, [info?.code, expired, settled]);

  const connected = status?.stage === "connected";
  useEffect(() => {
    if (connected) onConnected?.();
  }, [connected]);

  const decide = async (approve: boolean) => {
    if (!status?.user_code) return;
    setError(null);
    const r = await run(() => api.approveDevice(status.user_code!, approve));
    if (!r.ok) setError(r.error);
  };

  const elevate = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!status?.token_id) return;
    setError(null);
    const r = await run(() => api.elevateToken(`at:${status.token_id}`, password, hours));
    if (!r.ok) return setError(r.error);
    setPassword("");
    setStatus({ ...status, admin_live: true });
  };

  if (!info) {
    return (
      <div className="space-y-3 text-sm">
        {error ? <Notice tone="danger">{error}</Notice> : <div role="status" className="flex items-center gap-2 text-neutral-500"><Spinner className="text-neutral-400" />{t("common.loading")}</div>}
        {error && (
          <Button variant="secondary" disabled={busy} onClick={() => void issue()}>
            {t("getStarted.retry")}
          </Button>
        )}
      </div>
    );
  }

  const stage = status?.stage ?? "waiting";

  return (
    <div className="space-y-5">
      <section className="space-y-3">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h3 className="text-sm font-semibold text-neutral-900">{t("getStarted.step1")}</h3>
          <Segmented<Path>
            value={path}
            onChange={setPath}
            options={[
              { value: "prompt", label: t("getStarted.pathPrompt") },
              { value: "manual", label: t("getStarted.pathManual") },
            ]}
          />
        </div>
        {path === "prompt" ? (
          <>
            <p className="text-sm text-neutral-600">{t("getStarted.promptDesc")}</p>
            <CopyBox text={info.prompt} wrap />
          </>
        ) : (
          <ol className="list-decimal space-y-3 pl-5 text-sm text-neutral-700">
            <li className="space-y-2">
              <div>{t("getStarted.manualAdd")}</div>
              <CopyBox text={info.tarball_url} />
            </li>
            <li>{t("getStarted.manualRestart")}</li>
            <li className="space-y-2">
              <div>{t("getStarted.manualLogin")}</div>
              <CopyBox text={info.login_command} />
            </li>
          </ol>
        )}
        <p className="text-xs text-neutral-500">
          {t("getStarted.noDsh")}{" "}
          <PageLink page="dsh-download" className="text-brand-700 hover:underline dark:text-brand-300">
            {t("getStarted.download")}
          </PageLink>
        </p>
      </section>

      <section className="space-y-3 border-t border-neutral-100 pt-4">
        <h3 className="text-sm font-semibold text-neutral-900">{t("getStarted.step2")}</h3>
        {expired ? (
          <div className="flex flex-wrap items-center gap-3 text-sm text-amber-800 dark:text-amber-200">
            {t("getStarted.expired")}
            <Button size="sm" variant="secondary" disabled={busy} onClick={() => void issue()}>
              {t("getStarted.regenerate")}
            </Button>
          </div>
        ) : stage === "waiting" ? (
          <div className="flex flex-wrap items-center gap-2 text-sm text-neutral-600">
            <Loader2 size={15} className="animate-spin" />
            {t("getStarted.waiting", { code: info.code, time: formatTime(info.expires_at) })}
          </div>
        ) : stage === "pending" ? (
          <div className="space-y-3 rounded-md border border-brand-200/80 bg-brand-50/70 px-4 py-3 text-sm dark:border-brand-400/25 dark:bg-brand-500/10">
            <div className="text-neutral-900">{t("getStarted.pending", { client: status?.client_name || "DSH", os: status?.client_os ?? "", ip: status?.client_ip ?? "" })}</div>
            <div className="font-mono text-lg tracking-widest text-neutral-900">{status?.user_code}</div>
            <div className="text-xs text-neutral-600">{t("getStarted.pendingHint")}</div>
            <div className="flex gap-2">
              <Button disabled={busy} onClick={() => void decide(true)}>
                {t("getStarted.approve")}
              </Button>
              <Button variant="secondary" disabled={busy} onClick={() => void decide(false)}>
                {t("getStarted.deny")}
              </Button>
            </div>
          </div>
        ) : stage === "approved" ? (
          <div className="flex items-center gap-2 text-sm text-neutral-600">
            <Loader2 size={15} className="animate-spin" />
            {t("getStarted.approved")}
          </div>
        ) : stage === "denied" ? (
          <div className="text-sm text-amber-800 dark:text-amber-200">{t("getStarted.denied", { cmd: info.login_command })}</div>
        ) : (
          <Done text={t("getStarted.connected", { client: status?.client_name || "DSH" })} />
        )}
      </section>

      {admin && connected && (
        <section className="space-y-3 border-t border-neutral-100 pt-4">
          <h3 className="text-sm font-semibold text-neutral-900">{t("getStarted.step3Admin")}</h3>
          {status?.admin_live ? (
            <Done text={t("getStarted.adminOn")} />
          ) : (
            <form className="flex flex-wrap items-center gap-2" onSubmit={(e) => void elevate(e)}>
              <p className="w-full text-sm text-neutral-600">{t("getStarted.adminDesc")}</p>
              <PasswordInput label={t("getStarted.password")} placeholder={t("getStarted.password")} className={inputCls} containerClassName="w-60" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="current-password" />
              <select className={cx(inputCls, "!w-28")} value={hours} onChange={(e) => setHours(Number(e.target.value) as 1 | 8)}>
                <option value={1}>{t("getStarted.hours", { n: 1 })}</option>
                <option value={8}>{t("getStarted.hours", { n: 8 })}</option>
              </select>
              <Button type="submit" disabled={busy || !password}>
                {t("getStarted.adminOnButton")}
              </Button>
            </form>
          )}
        </section>
      )}

      {connected && (!admin || status?.admin_live) && (
        <section className="space-y-3 border-t border-neutral-100 pt-4">
          <h3 className="text-sm font-semibold text-neutral-900">{t(admin ? "getStarted.step4Admin" : "getStarted.step3")}</h3>
          <p className="text-sm text-neutral-600">{t(admin ? "getStarted.nextDescAdmin" : "getStarted.nextDesc")}</p>
          <CopyBox text={info.next_prompt} wrap />
        </section>
      )}

      {error && <Notice tone="danger">{error}</Notice>}
    </div>
  );
}

function Done({ text }: { text: string }) {
  return (
    <div className="flex items-center gap-2 text-sm text-emerald-700 dark:text-emerald-300">
      <Check size={16} />
      {text}
    </div>
  );
}
