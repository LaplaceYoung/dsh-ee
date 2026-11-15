import { useEffect, useState } from "react";
import { Link } from "react-router";
import { Check, Cpu, FileSearch, ShieldCheck, X } from "lucide-react";
import { WhaleMark } from "../components/whale-mark";
import { auditLog, clientPlatforms, microApps } from "../mock/data";
import { PageLink } from "../lib/menus";
import { LangSwitch } from "../components/lang-switch";
import { ThemeToggle } from "../components/theme-toggle";
import { LandingHeroScene } from "../components/landing-hero-scene";
import { Backdrop, Globe, HoverArrow } from "../components/landing-graphics";
import { FeatureGrid } from "../components/landing-features";
import { cx } from "../components/ui";
import { useTranslation } from "react-i18next";
import { api } from "../lib/api";
import { useApi } from "../lib/use-api";

/** 进入视口时依次浮现；脚本没跑或用户要求减少动效时内容直接可见。 */
function useReveal() {
  useEffect(() => {
    if (matchMedia("(prefers-reduced-motion: reduce)").matches) return;
    const els = document.querySelectorAll<HTMLElement>("[data-reveal]");
    els.forEach((el) => el.classList.add("reveal-pending"));
    const io = new IntersectionObserver(
      (entries) =>
        entries.forEach((e) => {
          if (!e.isIntersecting) return;
          e.target.classList.add("reveal-in");
          io.unobserve(e.target);
        }),
      { threshold: 0.12 },
    );
    els.forEach((el) => io.observe(el));
    return () => io.disconnect();
  }, []);
}

/** 顶栏在首屏渐变上是透明的，滚动后换成半透明底色。 */
function useScrolled() {
  const [scrolled, setScrolled] = useState(false);
  useEffect(() => {
    const on = () => setScrolled(window.scrollY > 12);
    on();
    window.addEventListener("scroll", on, { passive: true });
    return () => window.removeEventListener("scroll", on);
  }, []);
  return scrolled;
}

const delay = (ms: number) => ({ "--d": `${ms}ms` }) as React.CSSProperties;

type Tone = "accent" | "navy" | "ghost" | "ghost-dark";

const tones: Record<Tone, string> = {
  accent: "rounded-full bg-lp-accent px-4 py-2 text-white hover:bg-lp-accent-hover dark:text-[#0a2540]",
  navy: "rounded-full bg-[#0a2540] px-4 py-2 text-white hover:bg-[#1a3a5f]",
  ghost: "py-2 text-lp-accent hover:text-lp-ink",
  "ghost-dark": "py-2 text-[#0a2540] hover:opacity-70",
};

/** Stripe 式按钮：站内路由走 Link，锚点走 a。 */
function Pill({ to, href, tone = "accent", className, children }: { to?: string; href?: string; tone?: Tone; className?: string; children: React.ReactNode }) {
  const cls = cx("inline-flex items-center text-[15px] font-semibold whitespace-nowrap transition-colors", tones[tone], className);
  return to ? (
    <Link to={to} className={cls}>
      {children}
      <HoverArrow />
    </Link>
  ) : (
    <a href={href} className={cls}>
      {children}
      <HoverArrow />
    </a>
  );
}

/** 章节抬头：品牌色小标题 + 大标题 + 说明。 */
function SectionHead({ label, title, desc, dark, id }: { label: string; title: React.ReactNode; desc?: string; dark?: boolean; id?: string }) {
  return (
    <div className="max-w-3xl">
      <div data-reveal className={cx("text-[17px] font-semibold", dark ? "text-[#80e9ff]" : "text-lp-accent")}>
        {label}
      </div>
      <h2
        id={id}
        data-reveal
        style={delay(60)}
        className={cx(
          "mt-3 text-[clamp(1.9rem,3.6vw,2.75rem)] leading-[1.15] font-bold tracking-[-0.02em]",
          dark ? "text-white" : "text-lp-ink",
        )}
      >
        {title}
      </h2>
      {desc && (
        <p data-reveal style={delay(120)} className={cx("mt-5 text-lg leading-relaxed", dark ? "text-[#adbdcc]" : "text-lp-body")}>
          {desc}
        </p>
      )}
    </div>
  );
}

/** 特性小标题：左侧一道品牌色竖线，Stripe 的招牌写法。 */
function FeatureTitle({ children, dark }: { children: React.ReactNode; dark?: boolean }) {
  return (
    <h3
      className={cx(
        "-ml-4 border-l-2 pl-[14px] text-[15px] leading-6 font-semibold sm:-ml-5 sm:pl-[18px]",
        dark ? "border-[#80e9ff] text-white" : "border-lp-accent text-lp-ink",
      )}
    >
      {children}
    </h3>
  );
}

/** 工作台里的微应用列表，按产品真实样式画的界面片段。 */
function WorkbenchFragment() {
  const { t } = useTranslation();
  const apps = [...microApps].sort((a, b) => b.runs - a.runs).slice(0, 4);
  return (
    <div data-ui className="grid overflow-hidden rounded-xl bg-lp-card text-sm shadow-lp sm:grid-cols-[150px_1fr]">
      <div className="hidden flex-col gap-0.5 bg-lp-bg-alt p-3 sm:flex">
        <div className="flex items-center gap-2 px-2 pb-3 font-semibold text-lp-ink">
          <WhaleMark size={20} className="text-lp-accent" />
          dsh-ee
        </div>
        {(["overview", "apps", "market", "setup"] as const).map((n) => (
          <div
            key={n}
            className={cx(
              "rounded-md px-2 py-1.5",
              n === "apps" ? "bg-lp-card font-medium text-lp-accent shadow-[0_1px_3px_rgba(50,50,93,0.12)]" : "text-lp-body",
            )}
          >
            {t(`landing.fragment.nav.${n}`)}
          </div>
        ))}
      </div>
      <div className="p-4">
        <div className="text-base font-semibold text-lp-ink">{t("landing.fragment.apps")}</div>
        <ul className="mt-2">
          {apps.map((m) => (
            <li key={m.id} className="flex items-center gap-3 border-t border-lp-line py-3 first:border-t-0">
              <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-lp-accent-soft text-sm font-semibold text-lp-accent">
                {m.title.slice(0, 1)}
              </span>
              <div className="min-w-0 flex-1">
                <div className="font-medium text-lp-ink">{m.title}</div>
                <div className="truncate text-xs text-lp-muted">
                  {t("landing.fragment.runs", { department: m.department, runs: m.runs.toLocaleString() })}
                </div>
              </div>
              <span className="shrink-0 rounded-full bg-lp-accent-soft px-3 py-1 text-xs font-semibold text-lp-accent">
                {t("landing.fragment.openInDsh")}
              </span>
            </li>
          ))}
        </ul>
      </div>
    </div>
  );
}

/** 审计日志与终端环境的界面片段，放在深色色带上。 */
function AuditFragment() {
  const { t } = useTranslation();
  const total = clientPlatforms.reduce((a, c) => a + c.count, 0);
  return (
    <div data-ui className="overflow-hidden rounded-xl bg-[#0a2540]/75 text-sm shadow-[0_30px_60px_-20px_rgba(0,0,0,0.5)] ring-1 ring-white/10 backdrop-blur-md dark:bg-[#050b16]/75">
      <div className="flex items-baseline justify-between px-5 pt-4 pb-2">
        <span className="font-semibold text-white">{t("landing.fragment.auditLog")}</span>
        <span className="text-xs text-[#8193a8]">{t("landing.fragment.today")}</span>
      </div>
      <ul className="px-5">
        {auditLog.slice(0, 5).map((e) => (
          <li key={e.at + e.target} className="flex gap-3 border-t border-white/[0.08] py-2.5 first:border-t-0">
            <span className="w-10 shrink-0 font-mono text-xs text-[#8193a8] tabular-nums">{e.at}</span>
            <div className="min-w-0 flex-1">
              <div className="truncate text-[#e6edf5]">
                {e.user} <span className="text-[#80e9ff]">{t(`audit.${e.action}`)}</span> {e.target}
              </div>
              <div className="truncate text-xs text-[#8193a8]">{e.client}</div>
            </div>
          </li>
        ))}
      </ul>
      <div className="mt-2 border-t border-white/[0.08] bg-black/10 px-5 py-4">
        <div className="mb-2 flex justify-between text-xs text-[#8193a8]">
          <span>{t("landing.fragment.os")}</span>
          <span className="tabular-nums">{t("landing.fragment.devices", { n: total.toLocaleString() })}</span>
        </div>
        <div className="flex h-2 gap-0.5 overflow-hidden rounded-full">
          {clientPlatforms.map((c) => (
            <div key={c.os} className={c.xinchuang ? "bg-[#80e9ff]" : "bg-white/20"} style={{ width: `${(c.count / total) * 100}%` }} />
          ))}
        </div>
        <div className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-[#adbdcc]">
          {clientPlatforms.map((c) => (
            <span key={c.os} className="inline-flex items-center gap-1.5">
              <span className={cx("h-1.5 w-1.5 rounded-full", c.xinchuang ? "bg-[#80e9ff]" : "bg-white/30")} />
              {c.os} <span className="tabular-nums">{c.count}</span>
            </span>
          ))}
        </div>
      </div>
    </div>
  );
}

const controls = [
  { key: "xinchuang", icon: Cpu },
  { key: "control", icon: ShieldCheck },
  { key: "audit", icon: FileSearch },
] as const;

const steps = ["deploy", "capture", "review", "use"] as const;

const whyPoints = ["knowledge", "trust", "trace", "reuse"] as const;

/** 对比：员工各自用 AI 工具 vs 经 dsh-ee 统一下发。 */
function Compare() {
  const { t } = useTranslation();
  return (
    <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
      <div data-reveal className="rounded-xl bg-lp-card p-7 shadow-lp-sm sm:p-9">
        <div className="text-sm font-semibold text-lp-muted">{t("landing.why.before.tag")}</div>
        <h3 className="mt-1 text-2xl font-bold tracking-[-0.01em] text-lp-ink">{t("landing.why.before.title")}</h3>
        <ul className="mt-6 space-y-4">
          {whyPoints.map((k) => (
            <li key={k} className="flex gap-3 text-lp-body">
              <span className="mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-lp-bg-alt text-lp-muted">
                <X size={12} strokeWidth={3} />
              </span>
              {t(`landing.why.before.${k}`)}
            </li>
          ))}
        </ul>
      </div>
      <div data-reveal style={delay(100)} className="relative overflow-hidden rounded-xl bg-[#0a2540] p-7 text-white shadow-lp sm:p-9 dark:bg-[#132c4d]">
        <div aria-hidden className="lp-mesh absolute inset-x-0 top-0 h-1.5" />
        <div className="text-sm font-semibold text-[#80e9ff]">{t("landing.why.after.tag")}</div>
        <h3 className="mt-1 text-2xl font-bold tracking-[-0.01em]">{t("landing.why.after.title")}</h3>
        <ul className="mt-6 space-y-4">
          {whyPoints.map((k) => (
            <li key={k} className="flex gap-3 text-[#c4d3e3]">
              <span className="mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-[#80e9ff] text-[#0a2540]">
                <Check size={12} strokeWidth={3} />
              </span>
              {t(`landing.why.after.${k}`)}
            </li>
          ))}
        </ul>
      </div>
    </div>
  );
}

const shell = "mx-auto max-w-[1160px] px-5 sm:px-8";

/** 章节里的竖向虚线参考线。 */
function Guides() {
  return (
    <div aria-hidden className="pointer-events-none absolute inset-0">
      <div className={cx(shell, "lp-guides h-full opacity-70")} />
    </div>
  );
}

/** 演示站首页的介绍视频：按界面语言挑一个，没有就用另一个。正常部署没有。 */
function useDemoVideo() {
  const { i18n } = useTranslation();
  const videos = useApi(api.authMethods).data?.demo?.videos ?? [];
  if (!videos.length) return null;
  const want = i18n.language.startsWith("zh") ? "zh" : "en";
  return `/demo/intro/${videos.includes(want) ? want : videos[0]}`;
}

export default function Landing() {
  const { t } = useTranslation();
  const onTop = !useScrolled();
  const video = useDemoVideo();
  useReveal();
  return (
    <div className="lp relative min-h-screen overflow-x-clip">
      <header
        className={cx(
          "sticky top-0 z-20 transition-[background-color,box-shadow] duration-200",
          onTop ? "bg-transparent" : "bg-lp-bg/85 shadow-[0_1px_0_var(--lp-line)] backdrop-blur-md",
        )}
      >
        <div className={cx(shell, "flex h-16 items-center gap-4 sm:gap-8")}>
          <Link to="/" className={cx("flex shrink-0 items-center gap-2 text-lg font-bold tracking-tight whitespace-nowrap text-lp-ink")}>
            <WhaleMark size={30} className="text-lp-accent" />
            dsh-ee
          </Link>
          <nav className={cx("hidden gap-7 text-[15px] font-semibold text-lp-ink lg:flex")}>
            {(["how", "why", "workbench", "control", "deploy"] as const).map((k) => (
              <a key={k} href={`#${k}`} className="transition-opacity hover:opacity-60">
                {t(`landing.nav.${k}`)}
              </a>
            ))}
          </nav>
          <nav className="ml-auto flex items-center gap-4">
            {/* 手机端用收起形式，桌面端保持完整下拉：两个入口都在，最窄 375px 也能切换语言。 */}
            <LangSwitch compact className="rounded-full px-2 sm:hidden" />
            <LangSwitch
              className={cx("hidden rounded-full px-2.5 sm:inline-flex", onTop && "bg-white/70 backdrop-blur dark:bg-white/10 dark:[&_select]:text-white/90")}
            />
            <ThemeToggle className={cx("rounded-full!", onTop && "bg-white/70 backdrop-blur dark:bg-white/10")} />
          </nav>
          <span className="hidden sm:block">
            <Pill to="/go" tone="navy" className="py-1.5! dark:bg-white! dark:text-[#0a2540]!">
              {t("landing.enter")}
            </Pill>
          </span>
        </div>
      </header>

      <main className="relative">
        {/* 首屏 */}
        <section className="lp-hero-bg relative -mt-16 overflow-hidden border-b border-lp-line pt-16">
          <div aria-hidden className="lp-hero-dots pointer-events-none absolute inset-0" />
          <div aria-hidden className="pointer-events-none absolute inset-0">
            <div className={cx(shell, "h-full")}>
              <div className="h-full w-px -translate-x-4 bg-[linear-gradient(to_bottom,transparent,var(--hub-guide)_20%,var(--hub-guide)_80%,transparent)] sm:-translate-x-6" />
            </div>
          </div>
          <div className={cx(shell, "relative flex flex-col gap-5 pt-6 pb-6 sm:pt-8 lg:flex-row lg:items-end lg:justify-between lg:gap-10 lg:pt-10")}>
            <div data-reveal className="max-w-2xl">
              <h1 className="text-[clamp(1.35rem,2.2vw,1.85rem)] leading-tight font-semibold tracking-[-0.015em] text-lp-ink">{t("landing.heroLine1")}</h1>
              <p className="mt-2 text-[15px] leading-relaxed text-lp-muted">{t("landing.heroLead")}</p>
            </div>
            <div data-reveal className="flex shrink-0 flex-wrap items-center gap-x-6 gap-y-3" style={delay(90)}>
              {video && <Pill to="/login">{t("landing.tryDemo")}</Pill>}
              <Pill href="#how" tone="ghost">
                {t("landing.seeHow")}
              </Pill>
            </div>
          </div>
          <div className="relative pb-10">
            {video ? (
              <div className={cx(shell, "pt-4")}>
                {/* 浏览器只允许静音的视频自动播放；视频本身也没有声音 */}
                <video
                  key={video}
                  src={video}
                  className="mx-auto block w-full max-w-5xl rounded-xl bg-white shadow-lp ring-1 ring-lp-line"
                  aria-label={t("landing.videoLabel")}
                  autoPlay
                  muted
                  loop
                  playsInline
                  controls
                  preload="metadata"
                />
              </div>
            ) : (
              <LandingHeroScene />
            )}
          </div>
        </section>

        <section id="how" aria-labelledby="how-title" className="relative scroll-mt-16 pt-12 pb-32">
          <Guides />
          <div className={cx(shell, "relative")}>
            <SectionHead id="how-title" label={t("landing.nav.how")} title={t("landing.howLine")} />
            <div className="mt-14">
              <FeatureGrid />
            </div>
          </div>
        </section>

        <section id="why" aria-labelledby="why-title" className="relative scroll-mt-16 border-y border-lp-line bg-lp-bg-alt py-28">
          <div className={shell}>
            <div className="mb-14 grid grid-cols-1 gap-6 lg:grid-cols-[1.4fr_1fr] lg:items-end">
              <SectionHead id="why-title" label={t("landing.why.eyebrow")} title={t("landing.why.title")} />
              <p data-reveal style={delay(80)} className="text-lg leading-relaxed text-lp-body">
                {t("landing.why.desc")}
              </p>
            </div>
            <Compare />
          </div>
        </section>

        <section id="workbench" className="relative scroll-mt-16 py-28">
          <Guides />
          <div className={cx(shell, "relative grid grid-cols-1 gap-14 lg:grid-cols-[1fr_1.15fr] lg:items-center")}>
            <div>
              <SectionHead
                label={t("landing.nav.workbench")}
                title={
                  <>
                    {t("landing.wbLine1")}
                    <br />
                    {t("landing.wbLine2")}
                  </>
                }
                desc={t("landing.wbDesc")}
              />
              <dl data-reveal className="mt-10 grid grid-cols-1 gap-8 pl-4 sm:grid-cols-2 sm:pl-5" style={delay(160)}>
                <div>
                  <dt>
                    <FeatureTitle>{t("landing.toDsh")}</FeatureTitle>
                  </dt>
                  <dd className="mt-2 text-[15px] leading-relaxed text-lp-body">{t("landing.toDshDesc")}</dd>
                </div>
                <div>
                  <dt>
                    <FeatureTitle>{t("landing.toEe")}</FeatureTitle>
                  </dt>
                  <dd className="mt-2 text-[15px] leading-relaxed text-lp-body">{t("landing.toEeDesc")}</dd>
                </div>
              </dl>
            </div>
            <div data-reveal style={delay(120)}>
              <Backdrop>
                <WorkbenchFragment />
              </Backdrop>
            </div>
          </div>
        </section>

        <section id="control" className="relative scroll-mt-16 overflow-hidden bg-[#0a2540] py-28 dark:bg-lp-navy">
          <div className={cx(shell, "relative grid grid-cols-1 gap-14 lg:grid-cols-2")}>
            <div>
              <SectionHead
                dark
                label={t("landing.nav.control")}
                title={
                  <>
                    {t("landing.controlLine1")}
                    <br />
                    {t("landing.controlLine2")}
                  </>
                }
              />
              <div className="mt-12 grid grid-cols-1 gap-9 pl-4 sm:pl-5">
                {controls.map(({ key, icon: Icon }, j) => (
                  <div key={key} data-reveal style={delay(80 + j * 80)}>
                    <Icon size={22} className="mb-3 text-[#80e9ff]" />
                    <FeatureTitle dark>{t(`landing.controls.${key}.title`)}</FeatureTitle>
                    <p className="mt-2 max-w-lg text-[15px] leading-relaxed text-[#adbdcc]">{t(`landing.controls.${key}.desc`)}</p>
                  </div>
                ))}
              </div>
            </div>
            <div data-reveal className="relative" style={delay(120)}>
              <Globe className="pointer-events-none mx-auto -mt-6 w-full max-w-[560px] lg:-mt-16" />
              <div className="relative -mt-40 sm:-mt-56 lg:ml-10">
                <AuditFragment />
              </div>
            </div>
          </div>
        </section>

        <section id="deploy" className="relative scroll-mt-16 py-28">
          <Guides />
          <div className={cx(shell, "relative")}>
            <SectionHead
              label={t("landing.nav.deploy")}
              title={
                <>
                  {t("landing.deployLine1")}
                  {t("landing.deployLine2")}
                </>
              }
            />
            <ol className="mt-14 grid grid-cols-1 gap-x-8 gap-y-10 pl-4 sm:grid-cols-2 sm:pl-5 lg:grid-cols-4">
              {steps.map((st, j) => (
                <li key={st} data-reveal style={delay(j * 70)}>
                  <div className="mb-3 font-mono text-sm text-lp-muted">{String(j + 1).padStart(2, "0")}</div>
                  <FeatureTitle>{t(`landing.steps.${st}.title`)}</FeatureTitle>
                  <p className="mt-2 text-[15px] leading-relaxed text-lp-body">{t(`landing.steps.${st}.desc`)}</p>
                </li>
              ))}
            </ol>
            <div data-reveal className="mt-16 grid grid-cols-1 gap-8 lg:grid-cols-[1.4fr_1fr] lg:items-center" style={delay(120)}>
              <Backdrop>
                <div className="overflow-hidden rounded-xl bg-[#0a2540] shadow-lp dark:bg-[#132c4d]">
                  <div className="flex items-center gap-1.5 border-b border-white/10 px-4 py-3" aria-hidden>
                    <span className="h-2.5 w-2.5 rounded-full bg-white/20" />
                    <span className="h-2.5 w-2.5 rounded-full bg-white/20" />
                    <span className="h-2.5 w-2.5 rounded-full bg-white/20" />
                  </div>
                  <pre className="overflow-x-auto p-6 font-mono text-sm leading-7 text-[#c4d3e3]">
                    <code>
                      <span className="text-[#8193a8]">{t("landing.codeServer")}</span>
                      {"\n"}
                      <span className="text-[#80e9ff]">docker</span> compose up <span className="text-[#ffcc80]">-d</span>
                      {"\n\n"}
                      <span className="text-[#8193a8]">{t("landing.codeClient")}</span>
                      {"\n"}
                      <span className="text-[#80e9ff]">curl</span> <span className="text-[#ffcc80]">-fsSL</span> https://dsh.acme.internal/setup.sh | <span className="text-[#80e9ff]">sh</span>
                      {"\n\n"}
                      <span className="text-[#8193a8]">{t("landing.codeUse")}</span>
                      {"\n"}
                      <span className="text-[#80e9ff]">dsh</span> plugin add <span className="text-[#6ee7b7]">@acme/skill-expense-policy</span>
                    </code>
                  </pre>
                </div>
              </Backdrop>
              <p className="text-[15px] leading-relaxed text-lp-body">{t("landing.authNote")}</p>
            </div>
          </div>
        </section>
      </main>

      <footer className="relative bg-lp-bg-alt">
        <div className={cx(shell, "py-24")}>
          <h2 className="max-w-3xl text-[clamp(1.9rem,3.6vw,2.75rem)] leading-[1.15] font-bold tracking-[-0.02em] text-lp-ink">{t("landing.closing")}</h2>
          <div className="mt-8 flex flex-wrap items-center gap-x-6 gap-y-3">
            <Pill to="/go">{t("landing.enter")}</Pill>
            <PageLink page="market" className="inline-flex items-center py-2 text-[15px] font-semibold text-lp-accent hover:text-lp-ink">
              {t("landing.footerMarket")}
              <HoverArrow />
            </PageLink>
          </div>
        </div>
        <div className="border-t border-lp-line">
          <div className={cx(shell, "flex flex-wrap items-center justify-between gap-4 py-8 text-sm text-lp-muted")}>
            <span className="flex items-center gap-2 font-bold text-lp-ink">
              <WhaleMark size={24} className="text-lp-accent" />
              dsh-ee
            </span>
            <span>{t("landing.footerNote")}</span>
            <span className="flex gap-5">
              <PageLink page="market" className="hover:text-lp-ink">{t("landing.footerMarket")}</PageLink>
              <PageLink page="admin-dashboard" className="hover:text-lp-ink">{t("landing.footerAdmin")}</PageLink>
            </span>
          </div>
        </div>
      </footer>
    </div>
  );
}
