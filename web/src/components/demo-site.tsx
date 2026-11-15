import { useState } from "react";
import { Link } from "react-router";
import { useTranslation } from "react-i18next";
import { ArrowRight, MessageSquareText } from "lucide-react";
import type { DemoPersona, DemoSite } from "../lib/api";
import { useAuth } from "../lib/auth";
import { CopyBox, Notice, Spinner, cx } from "./ui";
import { DemoResetCountdown } from "./demo-reset";

// 公开演示站专用的界面：登录页的场景卡片、「用自己的 DSH 接上来」、每页顶上的身份条。正常部署不渲染。

/** 登录页的场景卡片：点一下以这位示例员工的身份登录，打开 TA 常用的应用。 */
export function DemoPersonas({ personas, busy, onPick }: { personas: DemoPersona[]; busy: string | null; onPick: (p: DemoPersona) => void }) {
  const { t } = useTranslation();
  return (
    <div className="grid gap-3 sm:grid-cols-2">
      {personas.map((p) => (
        <button
          key={p.username}
          type="button"
          disabled={!!busy}
          onClick={() => onPick(p)}
          data-persona={p.username}
          className="group flex flex-col rounded-xl bg-card p-5 text-left shadow-card ring-1 ring-neutral-200/80 transition hover:-translate-y-0.5 hover:shadow-modal hover:ring-brand-300 disabled:opacity-60 dark:ring-white/10"
        >
          <div className="flex items-center gap-3">
            <span className="flex size-10 shrink-0 items-center justify-center rounded-full bg-gradient-to-br from-brand-400 to-brand-600 text-base font-semibold text-white">
              {p.display_name.slice(-2)}
            </span>
            <div className="min-w-0">
              <p className="truncate text-[15px] font-semibold text-neutral-900">{p.display_name}</p>
              <p className="truncate text-xs text-neutral-500">
                {p.department} · {p.title}
              </p>
            </div>
          </div>
          <p className="mt-4 text-xs font-medium tracking-wide text-brand-600 dark:text-brand-300">{t("login.demo.uses", { app: p.app_title })}</p>
          <p className="mt-1.5 flex gap-2 text-sm leading-6 text-neutral-700">
            <MessageSquareText size={16} aria-hidden className="mt-1 shrink-0 text-neutral-400" />
            <span>「{p.try}」</span>
          </p>
          <span className="mt-4 inline-flex items-center gap-1 text-sm font-medium text-brand-600 dark:text-brand-300">
            {busy === p.username ? <Spinner size={14} /> : null}
            {t("login.demo.enterAs", { name: p.display_name })}
            <ArrowRight size={14} aria-hidden className="transition-transform group-hover:translate-x-0.5" />
          </span>
        </button>
      ))}
    </div>
  );
}

/** 已经装了 DSH 的访客：用自己的模型 key，把演示站接到自己的 DSH 上，真的问一问。 */
export function DemoConnectDsh({ className }: { className?: string }) {
  const { t } = useTranslation();
  const base = window.location.origin;
  const [os, setOs] = useState<"unix" | "win">(() => (navigator.userAgent.includes("Windows") ? "win" : "unix"));
  return (
    <div className={cx("rounded-xl bg-card p-5 shadow-card ring-1 ring-neutral-200/80 dark:ring-white/10", className)}>
      <h2 className="text-[15px] font-semibold text-neutral-900">{t("login.demo.dsh.title")}</h2>
      <p className="mt-1 text-sm leading-6 text-neutral-600">{t("login.demo.dsh.intro")}</p>
      <ol className="mt-3 space-y-3 text-sm leading-6 text-neutral-700">
        <li>
          <span className="font-medium">1. </span>
          {t("login.demo.dsh.step1")}
          <span className="ml-2 inline-flex gap-1 text-xs">
            {(["unix", "win"] as const).map((k) => (
              <button
                key={k}
                type="button"
                onClick={() => setOs(k)}
                className={cx("rounded px-1.5 py-0.5", os === k ? "bg-neutral-900 text-neutral-50" : "text-neutral-500 hover:bg-neutral-100")}
              >
                {t(k === "unix" ? "login.demo.dsh.unix" : "login.demo.dsh.win")}
              </button>
            ))}
          </span>
          <div className="mt-2">
            <CopyBox wrap text={os === "unix" ? `curl -fsSL ${base}/setup.sh | sh` : `irm ${base}/setup.ps1 | iex`} />
          </div>
        </li>
        <li>
          <span className="font-medium">2. </span>
          {t("login.demo.dsh.step2")}
        </li>
        <li>
          <span className="font-medium">3. </span>
          {t("login.demo.dsh.step3")}
        </li>
      </ol>
      <p className="mt-3 text-xs leading-5 text-neutral-500">{t("login.demo.dsh.note")}</p>
    </div>
  );
}

/** 每页顶上的演示站提示：会重置；以示例员工身份登录时，说明你是谁、可以换人或看管理员视角。 */
export function DemoBar({ demo, className }: { demo: DemoSite; className?: string }) {
  const { t } = useTranslation();
  const { user, logout } = useAuth();
  const persona = demo.personas?.find((p) => p.username === user?.username);
  const switchTo = async () => {
    await logout();
    window.location.assign("/login");
  };
  return (
    <Notice role="status" className={className}>
      {persona ? (
        <>
          <span className="mr-2">{t("shell.demoPersona.you", { name: persona.display_name, dept: persona.department, title: persona.title })}</span>
          <span className="mr-2">{t("shell.demoPersona.tryInDsh", { app: persona.app, q: persona.try })}</span>
          <button type="button" onClick={switchTo} className="mr-3 font-medium whitespace-nowrap text-brand-600 underline-offset-2 hover:underline dark:text-brand-300">
            {t("shell.demoPersona.switch")}
          </button>
          <Link to="/go" className="font-medium whitespace-nowrap text-brand-600 underline-offset-2 hover:underline dark:text-brand-300">
            {t("shell.demoPersona.workbench")}
          </Link>
        </>
      ) : (
        <>{demo.reset ? t("shell.demoReset", { reset: demo.reset }) : t("shell.demo")}</>
      )}
      {demo.next_reset && (
        <>
          {" "}
          <span className="font-medium">
            <DemoResetCountdown at={demo.next_reset} />
          </span>
        </>
      )}
    </Notice>
  );
}
