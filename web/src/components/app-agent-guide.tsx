import { useMemo, useState } from "react";
import { Link } from "react-router";
import { useTranslation } from "react-i18next";
import { copyText } from "../lib/clipboard";
import { Check, Copy, ExternalLink, Lightbulb, Repeat, Sparkles } from "lucide-react";
import { api } from "../lib/api";
import { useApi } from "../lib/use-api";
import { useStore } from "../lib/store";
import { PageLink } from "../lib/menus";
import { Button, Card, cx, Field, ToggleRow } from "./ui";

const textareaCls = "w-full rounded-md border border-neutral-300 bg-card px-3 py-2 text-sm outline-none placeholder:text-neutral-500 focus:border-brand-500 focus:ring-2 focus:ring-brand-100";
const EXAMPLES = ["signup", "visit", "expense", "ledger"] as const;
const EXPERT_EXAMPLES = ["contract", "tax", "onboarding", "team"] as const;

/** 可点选的小标签：选项不多，直接平铺比下拉好挑。 */
function Pills({ options, values, onChange, empty }: { options: { value: string; label: string }[]; values: string[]; onChange: (v: string[]) => void; empty: string }) {
  if (options.length === 0) return <p className="text-sm text-neutral-500">{empty}</p>;
  return (
    <div className="flex flex-wrap gap-1.5">
      {options.map((o) => {
        const on = values.includes(o.value);
        return (
          <button
            key={o.value}
            type="button"
            aria-pressed={on}
            onClick={() => onChange(on ? values.filter((x) => x !== o.value) : [...values, o.value])}
            className={cx(
              "inline-flex items-center gap-1 rounded-full border px-2.5 py-1 text-sm transition",
              on
                ? "border-brand-500 bg-brand-50 text-brand-700 dark:bg-brand-400/15 dark:text-brand-200"
                : "border-neutral-200 text-neutral-700 hover:border-neutral-300 hover:bg-neutral-50",
            )}
          >
            {on && <Check size={12} />}
            {o.label}
          </button>
        );
      })}
    </div>
  );
}

/**
 * 「新建应用 → 让 AI 帮我做」：员工描述需求，页面拼出一段提示词，复制到 DSH 里发送。
 * AI 那边靠的是系统技能 dsh-ee-app-design 和 ee_hub 的 create（for_me），见 server/internal/api/agent_tools.go。
 */
/** expert：做专家（docs/expert-center.md）——示例、提示词换成专家的，没有数据表和网页版。 */
export function AppAgentGuide({ onManual, expert = false }: { onManual: () => void; expert?: boolean }) {
  const { t } = useTranslation();
  const { notify } = useStore();
  const hub = useApi(api.hub);
  const kb = useApi(api.knowledge);
  const [what, setWhat] = useState("");
  const [users, setUsers] = useState("");
  const [conns, setConns] = useState<string[]>([]);
  const [kbs, setKbs] = useState<string[]>([]);
  const [tables, setTables] = useState(false);
  const [web, setWeb] = useState(false);
  const [copied, setCopied] = useState(false);

  const connOptions = useMemo(
    () => (hub.data?.assets ?? []).filter((a) => a.kind === "connection" && a.available).map((a) => ({ value: a.slug, label: a.spec.title })),
    [hub.data],
  );
  const kbOptions = useMemo(() => (kb.data?.items ?? []).filter((b) => b.readable).map((b) => ({ value: b.slug, label: b.title })), [kb.data]);
  const label = (opts: { value: string; label: string }[], v: string) => `${opts.find((o) => o.value === v)?.label ?? v}（${v}）`;

  const prompt = useMemo(() => {
    if (!what.trim()) return "";
    const P = (k: string, o?: Record<string, string>) =>
      expert && (k === "head" || k === "steps" || k === "what")
        ? t(`expertCenter.agent.prompt.${k}` as "expertCenter.agent.prompt.head", o)
        : t(`appAgent.prompt.${k}` as "appAgent.prompt.head", o);
    const lines = [P("head"), "", P("what") + what.trim()];
    if (users.trim()) lines.push(P("users") + users.trim());
    lines.push(conns.length ? P("connections", { list: conns.map((c) => label(connOptions, c)).join("、") }) : P("connectionsAuto"));
    if (kbs.length) lines.push(P("knowledge", { list: kbs.map((k) => label(kbOptions, k)).join("、") }));
    if (tables && !expert) lines.push(P("tables"));
    if (web && !expert) lines.push(P("web"));
    lines.push("", P("steps", { tables: tables ? P("tablesSuffix") : "", origin: window.location.origin }));
    return lines.join("\n");
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [what, users, conns, kbs, tables, web, connOptions, kbOptions, t, expert]);

  const copy = async () => {
    if (!prompt) return;
    if (!(await copyText(prompt))) return notify(t("common.copyFailed"));
    setCopied(true);
    notify(t("appAgent.copied"));
    setTimeout(() => setCopied(false), 1500);
  };

  const steps = t(expert ? "expertCenter.agent.steps" : "appAgent.steps", { returnObjects: true }) as string[];
  const iterate = t(expert ? "expertCenter.agent.iterate" : "appAgent.iterate", { returnObjects: true }) as string[];
  const tips = t("appAgent.tips", { returnObjects: true }) as string[];

  return (
    <div className="grid items-start gap-6 lg:grid-cols-[minmax(0,1fr)_minmax(0,380px)]" data-app-agent-guide>
      <div className="space-y-4">
        <div className="flex gap-3 rounded-lg border border-brand-200/80 bg-brand-50/70 px-4 py-3 text-sm leading-6 text-neutral-800 dark:border-brand-400/25 dark:bg-brand-500/10">
          <Sparkles size={18} className="mt-0.5 shrink-0 text-brand-600 dark:text-brand-300" />
          <p>{expert ? t("expertCenter.agent.intro") : t("appAgent.intro")}</p>
        </div>

        <Card className="space-y-5 p-6">
          <h2 className="text-neutral-900 text-[15px] leading-6 font-semibold">{expert ? t("expertCenter.agent.describeTitle") : t("appAgent.describeTitle")}</h2>
          <Field label={t("appAgent.describeLabel")}>
            <textarea className={cx(textareaCls, "h-24")} value={what} placeholder={expert ? t("expertCenter.agent.describePlaceholder") : t("appAgent.describePlaceholder")} onChange={(e) => setWhat(e.target.value)} />
            <div className="mt-2 flex flex-wrap items-center gap-1.5 text-xs text-neutral-600">
              <span>{t("appAgent.examples")}</span>
              {expert && EXPERT_EXAMPLES.map((k) => (
                <button
                  key={k}
                  type="button"
                  onClick={() => setWhat(t(`expertCenter.agent.example.${k}.text`))}
                  className="rounded-full bg-neutral-100 px-2.5 py-1 text-neutral-700 transition hover:bg-brand-50 hover:text-brand-700 dark:hover:bg-brand-400/15"
                >
                  {t(`expertCenter.agent.example.${k}.label`)}
                </button>
              ))}
              {!expert && EXAMPLES.map((k) => (
                <button
                  key={k}
                  type="button"
                  onClick={() => {
                    setWhat(t(`appAgent.example.${k}.text`));
                    if (k === "signup" || k === "ledger") setTables(true);
                  }}
                  className="rounded-full bg-neutral-100 px-2.5 py-1 text-neutral-700 transition hover:bg-brand-50 hover:text-brand-700 dark:hover:bg-brand-400/15"
                >
                  {t(`appAgent.example.${k}.label`)}
                </button>
              ))}
            </div>
          </Field>
          <Field label={t("appAgent.usersLabel")}>
            <input className={cx(textareaCls, "h-9 py-0")} value={users} placeholder={t("appAgent.usersPlaceholder")} onChange={(e) => setUsers(e.target.value)} />
          </Field>
          <Field label={t("appAgent.connectionsLabel")} hint={t("appAgent.connectionsHint")}>
            <Pills options={connOptions} values={conns} onChange={setConns} empty={t("appAgent.noConnections")} />
          </Field>
          <Field label={t("appAgent.knowledgeLabel")}>
            <Pills options={kbOptions} values={kbs} onChange={setKbs} empty={t("appAgent.noKnowledge")} />
          </Field>
          {!expert && (
            <>
              <ToggleRow label={t("appAgent.needTables")} desc={t("appAgent.needTablesDesc")} checked={tables} onChange={setTables} />
              <ToggleRow label={t("appAgent.needWeb")} desc={t("appAgent.needWebDesc")} checked={web} onChange={setWeb} />
            </>
          )}
        </Card>

        <Card className="space-y-3 p-6">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <h2 className="text-neutral-900 text-[15px] leading-6 font-semibold">{t("appAgent.promptTitle")}</h2>
            <div className="flex gap-2">
              <a href="dsh://open" className="inline-flex h-8 items-center gap-1 rounded-md px-2.5 text-sm text-neutral-700 hover:bg-neutral-100">
                <ExternalLink size={14} /> {t("appAgent.openDsh")}
              </a>
              <Button size="sm" disabled={!prompt} onClick={copy}>
                {copied ? <Check size={14} /> : <Copy size={14} />} {t("appAgent.copy")}
              </Button>
            </div>
          </div>
          {prompt ? (
            <pre
              aria-label={t("appAgent.promptLabel")}
              className="max-h-96 overflow-auto rounded-md bg-neutral-900 px-4 py-3 font-mono text-[13px] leading-relaxed whitespace-pre-wrap break-words text-neutral-100"
            >
              {prompt}
            </pre>
          ) : (
            <p className="rounded-md border border-dashed border-neutral-300 bg-neutral-50/60 px-4 py-8 text-center text-sm text-neutral-500">{t("appAgent.emptyDesc")}</p>
          )}
          <p className="text-xs text-neutral-500">
            {t("appAgent.manualHint")}{" "}
            <button type="button" onClick={onManual} className="text-brand-600 dark:text-brand-300 hover:underline">{t("appAgent.tabManual")} →</button>
          </p>
        </Card>
      </div>

      <aside className="space-y-4 lg:sticky lg:top-4">
        <Card className="p-5">
          <h3 className="mb-3 text-neutral-900 text-[15px] font-semibold">{t("appAgent.stepsTitle")}</h3>
          <ol className="space-y-3">
            {steps.map((s, i) => (
              <li key={i} className="flex gap-3 text-sm text-neutral-700">
                <span className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-brand-50 text-xs font-semibold text-brand-600 dark:bg-brand-400/15 dark:text-brand-200">{i + 1}</span>
                <span className="pt-0.5">{s}</span>
              </li>
            ))}
          </ol>
          <div className="mt-4 flex flex-wrap items-center gap-x-3 gap-y-1 border-t border-neutral-100 pt-3 text-sm">
            <span className="text-neutral-600">{t("appAgent.notReady")}</span>
            <Link to="/get-started" className="font-medium text-brand-600 dark:text-brand-300 hover:underline">{t("appAgent.goSetup")} →</Link>
            <PageLink page="build" className="font-medium text-brand-600 dark:text-brand-300 hover:underline">{t("appAgent.goBuild")} →</PageLink>
          </div>
        </Card>
        <Card className="p-5">
          <h3 className="mb-1 flex items-center gap-2 text-neutral-900 text-[15px] font-semibold"><Repeat size={15} /> {t("appAgent.iterateTitle")}</h3>
          <p className="mb-2 text-sm text-neutral-600">{t("appAgent.iterateDesc")}</p>
          <ul className="space-y-1.5">
            {iterate.map((s) => (
              <li key={s} className="rounded-md bg-neutral-50 px-3 py-1.5 font-mono text-xs text-neutral-700">{s}</li>
            ))}
          </ul>
        </Card>
        <Card className="p-5">
          <h3 className="mb-2 flex items-center gap-2 text-neutral-900 text-[15px] font-semibold"><Lightbulb size={15} /> {t("appAgent.tipsTitle")}</h3>
          <ul className="list-disc space-y-1.5 pl-5 text-sm text-neutral-700">
            {tips.map((s) => <li key={s}>{s}</li>)}
          </ul>
        </Card>
      </aside>
    </div>
  );
}
