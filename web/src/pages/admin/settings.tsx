import { useEffect, useState, type ReactNode } from "react";
import { Link } from "react-router";
import { useStore } from "../../lib/store";
import type { Settings } from "../../lib/types";
import { Button, Card, cx, Field, inputBase, inputCls, PageHeader, Segmented, ToggleRow } from "../../components/ui";
import { DemoSettings } from "../../components/demo-import";
import ConnectSettings from "./connect-settings";
import { WriteConfirmSettings } from "../../components/write-confirm-settings";
import { EmbedSettings } from "../../components/embed-settings";
import { PageLink, useMenus } from "../../lib/menus";
import { useConfirm } from "../../lib/confirm";
import { useAuth } from "../../lib/auth";
import { Trans, useTranslation } from "react-i18next";
import { LANGUAGE_OPTIONS } from "../../lib/languages";

/** 设置页的一节。id 用于页内目录与全局搜索定位（lib/search-index.ts）。 */
export function Section({ id, title, desc, children }: { id?: string; title: string; desc?: string; children: ReactNode }) {
  return (
    <Card id={id} className="scroll-mt-6 p-6">
      <div className="mb-5">
        <h2 className="text-neutral-900 text-[15px] leading-6 font-semibold">{title}</h2>
        {desc && <p className="mt-0.5 text-sm text-neutral-500">{desc}</p>}
      </div>
      <div className="space-y-5">{children}</div>
    </Card>
  );
}

export default function AdminSettings() {
  const { t } = useTranslation();
  const { settings: s, updateSettings, resetSettings, notify } = useStore();
  const confirm = useConfirm();
  const set = <K extends keyof Settings>(key: K, patch: Partial<Settings[K]> | Settings[K]) =>
    updateSettings((cur) => ({
      ...cur,
      [key]: typeof patch === "object" && patch !== null ? { ...(cur[key] as object), ...patch } : patch,
    }));

  return (
    <div className="max-w-5xl">
      <PageHeader
        title={t("settingsAdmin.title")}
        desc={t("settingsAdmin.desc")}
        actions={
          <Button
            variant="secondary"
            size="sm"
            onClick={async () => {
              if (!(await confirm({ kind: "reset", description: t("settingsAdmin.resetConfirm") }))) return;
              resetSettings();
              notify(t("settingsAdmin.resetDone"));
            }}
          >
            {t("settingsAdmin.reset")}
          </Button>
        }
      />
      <div className="lg:grid lg:grid-cols-[11rem_minmax(0,1fr)] lg:gap-8">
      <SettingsToc />
      <div className="max-w-3xl space-y-6">
        <Section id="settings-basics" title={t("settingsAdmin.basics")}>
          <div className="grid gap-4 sm:grid-cols-2">
            <Field label={t("settingsAdmin.orgName")}>
              <input className={inputCls} value={s.orgName} onChange={(e) => set("orgName", e.target.value)} />
            </Field>
            <Field label={t("settingsAdmin.baseUrl")}>
              <input className={cx(inputCls, "font-mono")} value={s.baseUrl} onChange={(e) => set("baseUrl", e.target.value)} />
            </Field>
          </div>
          <Field label={t("settingsAdmin.defaultLanguage")} hint={t("settingsAdmin.defaultLanguageHint")}>
            <select
              className={cx(inputCls, "w-56")}
              value={s.defaultLanguage}
              onChange={(e) => set("defaultLanguage", e.target.value as Settings["defaultLanguage"])}
            >
              {LANGUAGE_OPTIONS.map((l) => (
                <option key={l.code} value={l.code}>
                  {l.label}
                </option>
              ))}
            </select>
          </Field>
          <Field label={t("settingsAdmin.mainScope")} hint={t("settingsAdmin.mainScopeHint")}>
            <input className={cx(inputBase, "w-48 font-mono")} value={s.scope} onChange={(e) => set("scope", e.target.value)} />
          </Field>
        </Section>

        <Section id="settings-scoring" title={t("settingsAdmin.scoring")} desc={t("settingsAdmin.scoringDesc")}>
          <ToggleRow
            label={t("settingsAdmin.enableScoring")}
            desc={t("settingsAdmin.enableScoringDesc")}
            checked={s.scoring.enabled}
            onChange={(v) => set("scoring", { enabled: v })}
          />
          <div className="space-y-5 border-l-2 border-neutral-100 pl-5">
            <ToggleRow
              label={t("settingsAdmin.showUpstreamScore")}
              desc={t("settingsAdmin.showUpstreamScoreDesc")}
              checked={s.scoring.showUpstreamScore}
              disabled={!s.scoring.enabled}
              onChange={(v) => set("scoring", { showUpstreamScore: v })}
            />
            <ToggleRow
              label={t("settingsAdmin.allowRating")}
              desc={t("settingsAdmin.allowRatingDesc")}
              checked={s.scoring.allowEmployeeRating}
              disabled={!s.scoring.enabled}
              onChange={(v) => set("scoring", { allowEmployeeRating: v })}
            />
            <div className={cx(!(s.scoring.enabled && s.scoring.showUpstreamScore) && "pointer-events-none opacity-50")}>
              <Field label={t("settingsAdmin.minPreload", { n: s.scoring.minPreloadScore })}>
                <input
                  type="range"
                  min={0}
                  max={90}
                  step={5}
                  value={s.scoring.minPreloadScore}
                  onChange={(e) => set("scoring", { minPreloadScore: +e.target.value })}
                  className="w-full accent-brand-500"
                />
              </Field>
            </div>
          </div>
        </Section>

        <Section id="settings-admission" title={t("settingsAdmin.admission")}>
          <Field
            label={t("settingsAdmin.admissionMode")}
            hint={
              s.admissionMode === "allowlist"
                ? t("settingsAdmin.allowlistHint")
                : t("settingsAdmin.openHint")
            }
          >
            <Segmented
              value={s.admissionMode}
              onChange={(v) => set("admissionMode", v)}
              options={[
                { value: "allowlist", label: t("settingsAdmin.allowlist") },
                { value: "open", label: t("settingsAdmin.open") },
              ]}
            />
          </Field>
        </Section>

        <Section id="settings-upstream" title={t("settingsAdmin.upstream")}>
          <Field label={t("settingsAdmin.network")}>
            <Segmented
              value={s.upstream.mode}
              onChange={(v) => set("upstream", { mode: v })}
              options={[
                { value: "online", label: t("settingsAdmin.online") },
                { value: "offline", label: t("settingsAdmin.offline") },
              ]}
            />
          </Field>
          <ToggleRow
            label={t("settingsAdmin.feed")}
            desc={t("settingsAdmin.feedDesc")}
            checked={s.upstream.dshfindFeed}
            disabled={s.upstream.mode === "offline"}
            onChange={(v) => set("upstream", { dshfindFeed: v })}
          />
          <p className="text-sm text-neutral-600">
            <Trans i18nKey="settingsAdmin.registryNote" components={{ link: <PageLink page="admin-registry" className="mx-1 font-medium text-brand-600 hover:text-brand-700 dark:text-brand-300 dark:hover:text-brand-200" /> }} />
          </p>
          <Field label={t("settingsAdmin.newVersion")}>
            <Segmented
              value={s.upstream.autoSync}
              onChange={(v) => set("upstream", { autoSync: v })}
              options={[
                { value: "review", label: t("settingsAdmin.syncReview") },
                { value: "auto", label: t("settingsAdmin.autoPublish") },
                { value: "off", label: t("settingsAdmin.noSync") },
              ]}
            />
          </Field>
        </Section>

        <Section id="settings-distribution" title={t("settingsAdmin.distribution")}>
          <ToggleRow
            label={t("settingsAdmin.offerDownload")}
            desc={t("settingsAdmin.offerDownloadDesc")}
            checked={s.distribution.enabled}
            onChange={(v) => set("distribution", { enabled: v })}
          />
        </Section>

        <Section id="settings-connect" title={t("connectAdmin.title")}>
          <ConnectSettings />
        </Section>

        <Section id="settings-write-confirm" title={t("writeConfirm.title")} desc={t("writeConfirm.desc")}>
          <WriteConfirmSettings />
        </Section>

        <Section id="settings-embed" title={t("settingsAdmin.embed.title")} desc={t("settingsAdmin.embed.desc")}>
          <EmbedSettings />
        </Section>

        <Section id="settings-demo" title={t("demo.title")} desc={t("demo.desc")}>
          <DemoSettings />
        </Section>
      </div>
      </div>
    </div>
  );
}

const TOC: { id: string; label: string }[] = [
  { id: "settings-basics", label: "settingsAdmin.basics" },
  { id: "settings-scoring", label: "settingsAdmin.scoring" },
  { id: "settings-admission", label: "settingsAdmin.admission" },
  { id: "settings-upstream", label: "settingsAdmin.upstream" },
  { id: "settings-distribution", label: "settingsAdmin.distribution" },
  { id: "settings-connect", label: "connectAdmin.title" },
  { id: "settings-write-confirm", label: "writeConfirm.title" },
  { id: "settings-embed", label: "settingsAdmin.embed.title" },
  { id: "settings-demo", label: "demo.title" },
];

/** 左侧的本页目录：滚动时高亮当前一节；下面是从这里拆出去的页面。 */
function SettingsToc() {
  const { t } = useTranslation();
  const { can } = useAuth();
  const { hasPage } = useMenus();
  const [current, setCurrent] = useState(TOC[0].id);
  useEffect(() => {
    const els = TOC.map((x) => document.getElementById(x.id)).filter((x): x is HTMLElement => !!x);
    if (!els.length || typeof IntersectionObserver === "undefined") return;
    const seen = new Map<string, boolean>();
    const io = new IntersectionObserver(
      (entries) => {
        for (const e of entries) seen.set(e.target.id, e.isIntersecting);
        const first = TOC.find((x) => seen.get(x.id));
        if (first) setCurrent(first.id);
      },
      { rootMargin: "-10% 0px -55% 0px" },
    );
    els.forEach((el) => io.observe(el));
    return () => io.disconnect();
  }, []);
  // 从系统设置拆出去的页面；扩展签名主要挂在「企业连接」旁边，这里也给只有 settings.manage 的管理员一个入口。
  const related = [
    ...(["admin-brand", "admin-auth"] as const).filter((k) => hasPage(k)),
    ...(can("settings.manage") ? (["admin-signing-keys"] as const) : []),
  ];
  return (
    <nav aria-label={t("nav.toc")} className="mb-6 lg:sticky lg:top-8 lg:mb-0 lg:self-start">
      <div className="px-2 pb-1.5 text-xs font-semibold tracking-wide text-neutral-400">{t("nav.toc")}</div>
      <div className="flex flex-wrap gap-1 lg:flex-col lg:gap-0.5">
        {TOC.map((x) => (
          <a
            key={x.id}
            href={`#${x.id}`}
            onClick={(e) => {
              e.preventDefault();
              document.getElementById(x.id)?.scrollIntoView({ behavior: "smooth", block: "start" });
              history.replaceState(null, "", `#${x.id}`);
              setCurrent(x.id);
            }}
            className={cx(
              "rounded-md px-2 py-1 text-sm transition",
              current === x.id ? "bg-brand-50 font-medium text-brand-600 dark:bg-brand-500/15 dark:text-brand-200" : "text-neutral-600 hover:bg-neutral-100/70 hover:text-neutral-900",
            )}
          >
            {t(x.label as never)}
          </a>
        ))}
      </div>
      {related.length > 0 && (
        <>
          <div className="mt-5 px-2 pb-1.5 text-xs font-semibold tracking-wide text-neutral-400">{t("nav.related")}</div>
          <div className="flex flex-wrap gap-1 lg:flex-col lg:gap-0.5">
            {related.map((k) => (
              <PageLink key={k} page={k} className="rounded-md px-2 py-1 text-sm text-neutral-600 hover:bg-neutral-100/70 hover:text-neutral-900">
                {t(`pages.${k}`)} →
              </PageLink>
            ))}
          </div>
        </>
      )}
    </nav>
  );
}