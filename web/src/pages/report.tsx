import { useEffect, useState, type ReactNode } from "react";
import { Link, useSearchParams } from "react-router";
import { ArrowLeft, Printer } from "lucide-react";
import { useTranslation } from "react-i18next";
import { api, formatTime, formatTokens, type MonthlyReport, type ReportParams } from "../lib/api";
import { formatNumber } from "../lib/format";
import { useAction, useApi } from "../lib/use-api";
import { useStore } from "../lib/store";
import { BarList, DailyBars } from "../components/charts";
import { ErrorNote, LoadState } from "../components/api-ui";
import { Button, cx, Field, inputBase } from "../components/ui";

/** 上个月（报告默认看上一个完整的月）。 */
function lastMonth() {
  const d = new Date();
  d.setDate(1);
  d.setMonth(d.getMonth() - 1);
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}`;
}

const pct = (a: number, b: number) => (b > 0 ? `${Math.round((a / b) * 100)}%` : "—");

function Kpi({ label, value, hint, sub }: { label: string; value: ReactNode; hint?: ReactNode; sub?: ReactNode }) {
  return (
    <div className="rounded-lg border border-neutral-200 bg-card p-4 break-inside-avoid">
      <div className="text-xs text-neutral-500">{label}</div>
      <div className="mt-1 text-2xl font-semibold text-neutral-900 tabular-nums">{value}</div>
      {hint && <div className="mt-1 text-xs text-neutral-600">{hint}</div>}
      {sub && <div className="mt-0.5 text-xs text-neutral-500">{sub}</div>}
    </div>
  );
}

function Block({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="mt-6 break-inside-avoid">
      <h2 className="mb-3 text-base font-semibold text-neutral-900">{title}</h2>
      {children}
    </section>
  );
}

const th = "border-b border-neutral-200 py-2 pr-3 text-left text-xs font-medium text-neutral-500";
const td = "border-b border-neutral-100 py-2 pr-3 text-sm text-neutral-800";

/** 估算参数：管理员按公司实际调整（打印时不显示表单，只在估算方法里写出取值）。 */
function ParamsForm({ value, onSaved }: { value: ReportParams; onSaved: () => void }) {
  const { t } = useTranslation();
  const { notify } = useStore();
  const [p, setP] = useState(value);
  const [error, setError] = useState<string | null>(null);
  const { busy, run } = useAction();
  useEffect(() => setP(value), [value]);
  const num = (k: keyof ReportParams, label: string) => (
    <Field label={label}>
      <input className={cx(inputBase, "w-32")} type="number" min={0} step="any" value={p[k]} onChange={(e) => setP({ ...p, [k]: Number(e.target.value) })} />
    </Field>
  );
  return (
    <div className="mt-8 rounded-lg border border-dashed border-neutral-300 p-4 print:hidden">
      <h2 className="mb-3 text-sm font-semibold text-neutral-900">{t("report.params")}</h2>
      <div className="flex flex-wrap items-end gap-4">
        {num("minutesPerAppRun", t("report.paramApp"))}
        {num("minutesPerToolCall", t("report.paramTool"))}
        {num("minutesPerKbHit", t("report.paramKb"))}
        {num("hourlyCost", t("report.paramCost"))}
        <Button
          variant="secondary"
          disabled={busy}
          onClick={async () => {
            setError(null);
            const r = await run(() => api.saveReportParams(p));
            if (!r.ok) return setError(r.error);
            notify(t("report.paramsSaved"));
            onSaved();
          }}
        >
          {t("report.saveParams")}
        </Button>
      </div>
      <ErrorNote>{error}</ErrorNote>
    </div>
  );
}

function ReportBody({ d }: { d: MonthlyReport }) {
  const { t } = useTranslation();
  const r = d.report;
  const dates = r.days.map((x) => x.date);
  return (
    <>
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        <Kpi
          label={t("report.kpiActive")}
          value={pct(r.active_users, r.users)}
          hint={t("report.kpiActiveHint", { active: r.active_users, users: r.users })}
          sub={t("report.vsLast", { value: pct(d.previous.active_users, d.previous.users) })}
        />
        <Kpi
          label={t("report.kpiApps")}
          value={formatNumber(r.app_runs)}
          hint={t("report.kpiAppsHint", { users: r.app_users })}
          sub={t("report.vsLast", { value: formatNumber(d.previous.app_runs) })}
        />
        <Kpi
          label={t("report.kpiSaved")}
          value={t("report.hours", { n: d.estimate.hours })}
          hint={d.params.hourlyCost > 0 ? t("report.kpiSavedHint", { cost: formatNumber(d.estimate.cost) }) : t("report.kpiSavedHintNoCost")}
        />
        <Kpi
          label={t("report.kpiSatisfaction")}
          value={d.satisfaction === null ? "—" : `${Math.round(d.satisfaction * 100)}%`}
          hint={d.satisfaction === null ? t("report.kpiSatisfactionNone") : undefined}
        />
      </div>

      <Block title={t("report.trend")}>
        <div className="grid gap-6 lg:grid-cols-2">
          <div>
            <div className="mb-2 text-xs text-neutral-500">{t("report.trendActive")}</div>
            <DailyBars values={r.days.map((x) => x.active_users)} dates={dates} unit={t("report.trendActive")} height="h-40" />
          </div>
          <div>
            <div className="mb-2 text-xs text-neutral-500">{t("report.trendRuns")}</div>
            <DailyBars values={r.days.map((x) => x.app_runs)} dates={dates} unit={t("report.trendRuns")} height="h-40" />
          </div>
        </div>
      </Block>

      <Block title={t("report.apps")}>
        {r.apps.length === 0 ? (
          <p className="text-sm text-neutral-600">{t("report.appsEmpty")}</p>
        ) : (
          <div className="overflow-x-auto"><table className="w-full">
            <thead>
              <tr>
                <th className={th}>{t("report.colApp")}</th>
                <th className={th}>{t("report.colDept")}</th>
                <th className={cx(th, "text-right")}>{t("report.colRuns")}</th>
                <th className={cx(th, "text-right")}>{t("report.colUsers")}</th>
              </tr>
            </thead>
            <tbody>
              {r.apps.map((a) => (
                <tr key={a.id}>
                  <td className={td}>
                    {a.title} <span className="font-mono text-xs text-neutral-500">/{a.slug}</span>
                  </td>
                  <td className={td}>{a.department || "—"}</td>
                  <td className={cx(td, "text-right tabular-nums")}>{formatNumber(a.runs)}</td>
                  <td className={cx(td, "text-right tabular-nums")}>{a.users}</td>
                </tr>
              ))}
            </tbody>
          </table></div>
        )}
        {r.capabilities.length > 0 && (
          <div className="mt-4">
            <div className="mb-2 text-xs text-neutral-500">{t("report.capabilities")}</div>
            <BarList rows={r.capabilities.map((c) => ({ label: c.title, value: c.runs, hint: `/${c.slug}` }))} />
          </div>
        )}
      </Block>

      <Block title={t("report.questions")}>
        {r.questions.length === 0 ? (
          <p className="text-sm text-neutral-600">{t("report.questionsEmpty")}</p>
        ) : (
          <div className="overflow-x-auto"><table className="w-full">
            <thead>
              <tr>
                <th className={th}>{t("report.colQuestion")}</th>
                <th className={cx(th, "text-right")}>{t("report.colAsked")}</th>
                <th className={cx(th, "text-right")}>{t("report.colAskers")}</th>
                <th className={cx(th, "text-right")}>{t("report.colMisses")}</th>
              </tr>
            </thead>
            <tbody>
              {r.questions.map((q) => (
                <tr key={q.query}>
                  <td className={td}>{q.query}</td>
                  <td className={cx(td, "text-right tabular-nums")}>{q.count}</td>
                  <td className={cx(td, "text-right tabular-nums")}>{q.users}</td>
                  <td className={cx(td, "text-right tabular-nums", q.misses > 0 && "text-amber-700")}>{q.misses}</td>
                </tr>
              ))}
            </tbody>
          </table></div>
        )}
      </Block>

      <Block title={t("report.departments")}>
        <div className="overflow-x-auto"><table className="w-full">
          <thead>
            <tr>
              <th className={th}>{t("report.colDepartment")}</th>
              <th className={cx(th, "text-right")}>{t("report.colActive")}</th>
              <th className={cx(th, "text-right")}>{t("report.colRuns")}</th>
              <th className={cx(th, "text-right")}>{t("report.colTokens")}</th>
            </tr>
          </thead>
          <tbody>
            {r.departments.map((x) => (
              <tr key={x.department}>
                <td className={td}>{x.department || t("report.noDepartment")}</td>
                <td className={cx(td, "text-right tabular-nums")}>
                  {x.active_users} / {x.users} <span className="text-xs text-neutral-500">{pct(x.active_users, x.users)}</span>
                </td>
                <td className={cx(td, "text-right tabular-nums")}>{formatNumber(x.app_runs)}</td>
                <td className={cx(td, "text-right tabular-nums")}>{formatTokens(x.tokens)}</td>
              </tr>
            ))}
          </tbody>
        </table></div>
      </Block>

      <Block title={t("report.usage")}>
        <ul className="list-disc space-y-1 pl-5 text-sm text-neutral-700">
          <li>{t("report.toolCalls", { n: r.tool_calls, errors: r.tool_errors })}</li>
          <li>{t("report.kbSearches", { n: r.kb_searches, hits: r.kb_hits })}</li>
          <li>{t("report.skills", { n: r.skill_runs })}</li>
          <li>{t("report.model", { n: r.model_calls, tokens: formatTokens(r.tokens) })}</li>
        </ul>
      </Block>

      <Block title={t("report.method")}>
        <p className="text-sm text-neutral-700">
          {t("report.methodText", { app: d.params.minutesPerAppRun, tool: d.params.minutesPerToolCall, kb: d.params.minutesPerKbHit })}
        </p>
        <p className="mt-1 text-sm text-neutral-700">{t("report.methodBreakdown", { app: d.estimate.app_hours, tool: d.estimate.tool_hours, kb: d.estimate.kb_hours })}</p>
        <p className="mt-2 text-xs text-neutral-500">{t("report.activeNote")}</p>
        <p className="mt-1 text-xs text-neutral-500">{t("report.satisfactionNote")}</p>
      </Block>
    </>
  );
}

/** 月度 AI 落地报告：可打印的网页，浏览器「另存为 PDF」即可，不引入 PDF 库。 */
export default function Report() {
  const { t } = useTranslation();
  const [params, setParams] = useSearchParams();
  const month = /^\d{4}-\d{2}$/.test(params.get("month") ?? "") ? params.get("month")! : lastMonth();
  const rep = useApi(() => api.monthlyReport(month));
  const { reload } = rep;
  useEffect(() => reload(), [month, reload]);
  const d = rep.data;

  return (
    <div className="min-h-screen bg-neutral-50 print:bg-white">
      <div className="mx-auto max-w-4xl px-6 py-8 print:max-w-none print:px-0 print:py-0">
        <div className="mb-6 flex flex-wrap items-center gap-3 print:hidden">
          <Link to="/admin" className="inline-flex items-center gap-1 text-sm text-neutral-600 hover:text-neutral-900">
            <ArrowLeft size={14} /> {t("report.back")}
          </Link>
          <label className="ml-auto flex items-center gap-2 text-sm text-neutral-700">
            {t("report.month")}
            <input type="month" className={cx(inputBase, "w-40")} value={month} onChange={(e) => e.target.value && setParams({ month: e.target.value }, { replace: true })} />
          </label>
          <Button onClick={() => window.print()} disabled={!d}>
            <Printer size={15} /> {t("report.print")}
          </Button>
        </div>
        <header className="mb-6 border-b border-neutral-200 pb-4">
          <h1 className="text-2xl font-bold tracking-tight text-neutral-900">{t("report.title")}</h1>
          <p className="mt-1 text-sm text-neutral-600">{t("report.subtitle", { org: d?.org_name || "dsh-ee", month })}</p>
          {d && <p className="mt-0.5 text-xs text-neutral-500">{t("report.generated", { time: formatTime(d.generated_at) })}</p>}
        </header>
        <LoadState loading={rep.loading && !d} error={d ? null : rep.error} onRetry={rep.reload}>
          {d && (
            <>
              <ReportBody d={d} />
              <ParamsForm value={d.params} onSaved={rep.reload} />
            </>
          )}
        </LoadState>
      </div>
    </div>
  );
}
