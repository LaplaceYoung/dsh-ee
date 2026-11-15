import { useState } from "react";
import { useTranslation } from "react-i18next";
import { api, type DemoGuideStep, type DemoImport, type DemoScenario } from "../lib/api";
import { useAuth } from "../lib/auth";
import { useConfirm } from "../lib/confirm";
import { useStore } from "../lib/store";
import { useApi } from "../lib/use-api";
import { Badge, Button, CopyBox, cx } from "./ui";

/** 导入完成后的下一步：用哪个演示员工登录、怎么接 DSH、先试哪个应用。 */
function NextSteps({ guide }: { guide: DemoGuideStep[] }) {
  const { t } = useTranslation();
  return (
    <div className="mt-4 rounded-md border border-brand-200/80 bg-brand-50/70 p-3.5 dark:border-brand-400/25 dark:bg-brand-500/10">
      <p className="font-medium text-neutral-900">{t("demo.tryIt")}</p>
      <ol className="mt-1 list-decimal space-y-1 pl-5 text-neutral-700">
        <li>{t("demo.next1")}</li>
        <li>
          {t("demo.next2")}
          <ul className="mt-1 list-disc space-y-0.5 pl-5">
            {guide.map((g) => (
              <li key={g.username + g.app}>
                {t("demo.guideLine", { name: g.display_name, department: g.department, title: g.title, app: g.app_title, try: `「${g.try}」` })}
              </li>
            ))}
          </ul>
        </li>
        <li>{t("demo.next3")}</li>
      </ol>
    </div>
  );
}

/** 挑一个示例场景：示例公司，或几个人的小团队。只有一个场景时不显示。 */
function ScenarioPicker({ scenarios, value, onChange, disabled }: { scenarios: DemoScenario[]; value: string; onChange: (id: string) => void; disabled: boolean }) {
  const { t } = useTranslation();
  return (
    <div role="radiogroup" aria-label={t("demo.scenario.pick")} className="mb-3">
      <p className="mb-2 font-medium text-neutral-900">{t("demo.scenario.pick")}</p>
      <div className="grid gap-2.5 sm:grid-cols-2">
        {scenarios.map((sc) => (
          <button
            key={sc.id}
            type="button"
            role="radio"
            aria-checked={value === sc.id}
            disabled={disabled}
            data-scenario={sc.id}
            onClick={() => onChange(sc.id)}
            className={cx(
              "rounded-lg border bg-card p-3.5 text-left transition disabled:opacity-60",
              value === sc.id ? "border-brand-500 ring-2 ring-brand-200 dark:ring-brand-400/30" : "border-neutral-200 hover:border-brand-300 dark:border-white/10",
            )}
          >
            <span className="flex flex-wrap items-center gap-2">
              <span className="font-semibold text-neutral-900">{t(`demo.scenario.names.${sc.id}.title`, { defaultValue: sc.title })}</span>
              <Badge tone="slate">{t(`demo.scenario.kind.${sc.kind}`)}</Badge>
              {sc.imported && <Badge tone="green">{t("demo.imported")}</Badge>}
            </span>
            <span className="mt-1.5 block leading-6 text-neutral-600">{t(`demo.scenario.names.${sc.id}.summary`, { defaultValue: sc.summary })}</span>
            <span className="mt-1.5 block text-xs text-neutral-500">{t("demo.scenario.counts", { people: sc.people, kbases: sc.kbases, assets: sc.assets })}</span>
          </button>
        ))}
      </div>
    </div>
  );
}

/** 「示例公司」：超级管理员一键导入一个示例场景（公司「星河科技」或小团队），试完一键清除。部署清单（/go）的醒目卡片和清单里的一步都用它。 */
export function DemoImportStep({ done, onImported }: { done: boolean; onImported: () => void }) {
  const { t } = useTranslation();
  const { user } = useAuth();
  const { notify } = useStore();
  const confirm = useConfirm();
  const status = useApi(() => (user?.is_super ? api.demoStatus() : Promise.resolve(null)));
  const [busy, setBusy] = useState<"" | "import" | "clear">("");
  const [asking, setAsking] = useState(false);
  const [result, setResult] = useState<DemoImport | null>(null);
  const [error, setError] = useState("");
  const [picked, setPicked] = useState("");
  const scenarios = status.data?.scenarios ?? [];
  // 默认选第一个还没导入的场景
  const scenario = scenarios.find((s) => s.id === picked) ?? scenarios.find((s) => !s.imported) ?? scenarios[0];

  const run = async () => {
    setAsking(false);
    setBusy("import");
    setError("");
    try {
      setResult(await api.importDemo(scenario?.id));
      onImported();
      status.reload();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy("");
    }
  };

  const clear = async () => {
    if (!(await confirm({ kind: "delete", description: t("demo.clearConfirm") }))) return;
    setBusy("clear");
    setError("");
    try {
      const r = await api.clearDemo();
      notify(t("demo.cleared", r));
      setResult(null);
      onImported();
      status.reload();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy("");
    }
  };

  if (!user?.is_super) return done ? null : <p className="mt-2 text-sm text-neutral-500">{t("demo.superOnly")}</p>;
  const present = status.data?.present ?? done;
  const canImport = scenario ? !scenario.imported : !done;
  const team = scenario?.kind === "team";
  return (
    <div className="mt-3 max-w-3xl text-sm">
      {scenarios.length > 1 && !asking && (
        <ScenarioPicker
          scenarios={scenarios}
          value={scenario?.id ?? ""}
          disabled={!!busy}
          onChange={(id) => {
            setPicked(id);
            setResult(null);
            setError("");
          }}
        />
      )}
      {result && (
        <ul className="flex flex-wrap gap-1.5">
          {result.items.map((it) => (
            <li key={`${it.kind}:${it.slug}`}>
              <Badge tone={it.result === "failed" ? "red" : "green"}>
                {t(`demo.kinds.${it.kind}`)} · {it.title}
                {it.result === "failed" && ` · ${t("demo.failed")}：${it.error}`}
              </Badge>
            </li>
          ))}
        </ul>
      )}
      {result && result.accounts.length > 0 && (
        <div className="mt-4">
          <p className="mb-2 font-medium text-neutral-900">{t("demo.accounts")}</p>
          <CopyBox wrap text={result.accounts.map((a) => `${a.display_name}（${a.department} · ${a.title}）  ${a.username}  ${a.password}`).join("\n")} />
        </div>
      )}
      {(result || (scenario ? scenario.imported : done && status.data)) && <NextSteps guide={result?.guide ?? scenario?.guide ?? status.data?.guide ?? []} />}
      {error && <p className="mt-3 text-red-600">{error}</p>}
      <div className="mt-3 flex flex-wrap gap-2">
        {canImport && !result && !asking && (
          <Button onClick={() => setAsking(true)} disabled={!!busy}>
            {busy === "import" ? t("demo.importing") : t(team ? "demo.importTeam" : "demo.import")}
          </Button>
        )}
        {present && !asking && (
          <Button size="sm" variant="secondary" onClick={() => void clear()} disabled={!!busy}>
            {busy === "clear" ? t("demo.clearing") : t("demo.clear")}
          </Button>
        )}
      </div>
      {asking && (
        <div className="rounded-md border border-amber-200 bg-amber-50 p-3.5 dark:border-amber-400/25 dark:bg-amber-500/10">
          <p className="text-neutral-800">{t("demo.confirm", { people: scenario?.people ?? 10 })}</p>
          <div className="mt-3 flex gap-2">
            <Button size="sm" onClick={() => void run()}>{t(team ? "demo.importTeam" : "demo.import")}</Button>
            <Button size="sm" variant="secondary" onClick={() => setAsking(false)}>{t("common.cancel")}</Button>
          </div>
        </div>
      )}
    </div>
  );
}

/** 部署清单顶部最显眼的入口：还没导入示例公司时显示。 */
export function DemoCompanyHero({ onImported }: { onImported: () => void }) {
  const { t } = useTranslation();
  return (
    <section className="onboarding-reveal mt-8 overflow-hidden rounded-2xl border border-brand-200 bg-gradient-to-br from-brand-50 via-card to-card p-6 shadow-card sm:p-7 dark:border-brand-400/30 dark:from-brand-500/15">
      <h2 className="text-xl font-bold tracking-tight text-neutral-900">{t("demo.heroTitle")}</h2>
      <p className="mt-2 max-w-3xl text-sm leading-6 text-neutral-600">{t("demo.heroDesc")}</p>
      <DemoImportStep done={false} onImported={onImported} />
    </section>
  );
}

/** 系统设置里的「示例公司」：随时导入，或转正式使用前清除。 */
export function DemoSettings() {
  const { user } = useAuth();
  const status = useApi(() => (user?.is_super ? api.demoStatus() : Promise.resolve(null)));
  if (!user?.is_super || !status.data) return null;
  return <DemoImportStep key={String(status.data.imported)} done={status.data.imported} onImported={status.reload} />;
}
