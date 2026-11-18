import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Check, Copy, ExternalLink, Lightbulb, Repeat, Sparkles } from "lucide-react";
import { useStore } from "../lib/store";
import { PageLink } from "../lib/menus";
import { Pills } from "./app-agent-guide";
import { Button, Card, cx, Field } from "./ui";

const textareaCls = "w-full rounded-md border border-neutral-300 bg-card px-3 py-2 text-sm outline-none placeholder:text-neutral-500 focus:border-brand-500 focus:ring-2 focus:ring-brand-100";
const EXAMPLES = ["travel", "crm", "ticket", "hr"] as const;
const WAYS = ["openapi", "mcp", "unsure"] as const;
const AUTHS = ["none", "bearer", "header", "unsure"] as const;

/**
 * 「新建连接 → 让 AI 帮我做」：管理员描述要接的系统、接入方式与认证，页面拼出一段提示词，
 * 复制到 DSH 里发送；AI 用 ee_hub 的 create（kind: connection）建好连接，凭据回页面补填。
 */
export function ConnAgentGuide({ onManual }: { onManual: () => void }) {
  const { t } = useTranslation();
  const { notify } = useStore();
  const [what, setWhat] = useState("");
  const [way, setWay] = useState<string>("unsure");
  const [url, setUrl] = useState("");
  const [doc, setDoc] = useState("");
  const [auth, setAuth] = useState<string>("unsure");
  const [header, setHeader] = useState("");
  const [copied, setCopied] = useState(false);

  const prompt = useMemo(() => {
    if (!what.trim()) return "";
    const P = (k: string, o?: Record<string, string>) => t(`connAgent.prompt.${k}` as "connAgent.prompt.head", o);
    const lines = [P("head"), "", P("what") + what.trim()];
    if (way === "openapi") lines.push(P("wayOpenapi"));
    else if (way === "mcp") lines.push(P("wayMcp"));
    else lines.push(P("wayUnsure"));
    if (url.trim()) lines.push(P("url", { url: url.trim() }));
    if (doc.trim()) lines.push("", P("doc", { doc: doc.trim() }));
    if (auth !== "unsure") {
      const h = auth === "header" && header.trim() ? `（${header.trim()}）` : "";
      lines.push(P("auth", { auth: t(`connAgent.auth.${auth}` as "connAgent.auth.none"), header: h }));
    }
    lines.push("", P("steps", { origin: window.location.origin }));
    return lines.join("\n");
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [what, way, url, doc, auth, header, t]);

  const copy = () => {
    if (!prompt) return;
    void navigator.clipboard?.writeText(prompt);
    setCopied(true);
    notify(t("connAgent.copied"));
    setTimeout(() => setCopied(false), 1500);
  };

  const steps = t("connAgent.steps", { returnObjects: true }) as string[];
  const iterate = t("connAgent.iterate", { returnObjects: true }) as string[];
  const tips = t("connAgent.tips", { returnObjects: true }) as string[];

  return (
    <div className="grid items-start gap-6 lg:grid-cols-[minmax(0,1fr)_minmax(0,380px)]" data-conn-agent-guide>
      <div className="space-y-4">
        <div className="flex gap-3 rounded-lg border border-brand-200 bg-brand-50/60 px-4 py-3 text-sm text-neutral-800 dark:border-brand-400/30 dark:bg-brand-400/10 dark:text-neutral-200">
          <Sparkles size={18} className="mt-0.5 shrink-0 text-brand-600 dark:text-brand-300" />
          <p>{t("connAgent.intro")}</p>
        </div>

        <Card className="space-y-5 p-6">
          <h2 className="font-semibold text-neutral-900">{t("connAgent.describeTitle")}</h2>
          <Field label={t("connAgent.describeLabel")}>
            <textarea className={cx(textareaCls, "h-24")} value={what} placeholder={t("connAgent.describePlaceholder")} onChange={(e) => setWhat(e.target.value)} />
            <div className="mt-2 flex flex-wrap items-center gap-1.5 text-xs text-neutral-600">
              <span>{t("connAgent.examples")}</span>
              {EXAMPLES.map((k) => (
                <button
                  key={k}
                  type="button"
                  onClick={() => {
                    setWhat(t(`connAgent.example.${k}.text`));
                    setWay(k === "ticket" ? "mcp" : "openapi");
                    setAuth(k === "crm" || k === "ticket" ? "bearer" : "unsure");
                  }}
                  className="rounded-full bg-neutral-100 px-2.5 py-1 text-neutral-700 transition hover:bg-brand-50 hover:text-brand-700 dark:bg-neutral-700/40 dark:text-neutral-200 dark:hover:bg-brand-400/15"
                >
                  {t(`connAgent.example.${k}.label`)}
                </button>
              ))}
            </div>
          </Field>
          <Field label={t("connAgent.wayLabel")}>
            <Pills options={WAYS.map((w) => ({ value: w, label: t(`connAgent.way.${w}`) }))} values={[way]} onChange={(v) => setWay(v[0] ?? "unsure")} empty="" />
          </Field>
          <Field label={t("connAgent.urlLabel")}>
            <input className={cx(textareaCls, "h-9 py-0")} value={url} placeholder={t("connAgent.urlPlaceholder")} onChange={(e) => setUrl(e.target.value)} />
          </Field>
          {way === "openapi" && (
            <Field label={t("connAgent.specLabel")}>
              <textarea className={cx(textareaCls, "h-28 font-mono text-xs")} value={doc} placeholder={t("connAgent.specPlaceholder")} onChange={(e) => setDoc(e.target.value)} />
            </Field>
          )}
          <Field label={t("connAgent.authLabel")}>
            <Pills options={AUTHS.map((a) => ({ value: a, label: t(`connAgent.auth.${a}`) }))} values={[auth]} onChange={(v) => setAuth(v[0] ?? "unsure")} empty="" />
          </Field>
          {auth === "header" && (
            <Field label={t("connAgent.headerLabel")}>
              <input className={cx(textareaCls, "h-9 py-0")} value={header} placeholder={t("connAgent.headerPlaceholder")} onChange={(e) => setHeader(e.target.value)} />
            </Field>
          )}
          <p className="rounded-md bg-amber-50 px-3 py-2 text-xs leading-5 text-amber-800 dark:bg-amber-400/10 dark:text-amber-200">{t("connAgent.secretNote")}</p>
        </Card>

        <Card className="space-y-3 p-6">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <h2 className="font-semibold text-neutral-900">{t("connAgent.promptTitle")}</h2>
            <div className="flex gap-2">
              <a href="dsh://open" className="inline-flex h-8 items-center gap-1 rounded-md px-2.5 text-sm text-neutral-700 hover:bg-neutral-100 dark:text-neutral-200 dark:hover:bg-neutral-700/50">
                <ExternalLink size={14} /> {t("connAgent.openDsh")}
              </a>
              <Button size="sm" disabled={!prompt} onClick={copy}>
                {copied ? <Check size={14} /> : <Copy size={14} />} {t("connAgent.copy")}
              </Button>
            </div>
          </div>
          {prompt ? (
            <pre
              aria-label={t("connAgent.promptLabel")}
              className="max-h-96 overflow-auto rounded-md bg-neutral-900 px-4 py-3 font-mono text-[13px] leading-relaxed whitespace-pre-wrap break-words text-neutral-100"
            >
              {prompt}
            </pre>
          ) : (
            <p className="rounded-md border border-dashed border-neutral-300 px-4 py-8 text-center text-sm text-neutral-500">{t("connAgent.emptyDesc")}</p>
          )}
          <p className="text-xs text-neutral-500">
            {t("connAgent.manualHint")}{" "}
            <button type="button" onClick={onManual} className="text-brand-600 hover:underline">{t("appAgent.tabManual")} →</button>
          </p>
        </Card>
      </div>

      <aside className="space-y-4 lg:sticky lg:top-4">
        <Card className="p-5">
          <h3 className="mb-3 font-medium text-neutral-900">{t("connAgent.stepsTitle")}</h3>
          <ol className="space-y-3">
            {steps.map((s, i) => (
              <li key={i} className="flex gap-3 text-sm text-neutral-700">
                <span className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-brand-50 text-xs font-semibold text-brand-600 dark:bg-brand-400/15 dark:text-brand-200">{i + 1}</span>
                <span className="pt-0.5">{s}</span>
              </li>
            ))}
          </ol>
          <div className="mt-4 flex flex-wrap items-center gap-x-3 gap-y-1 border-t border-neutral-100 pt-3 text-sm dark:border-neutral-700/60">
            <span className="text-neutral-600">{t("connAgent.notReady")}</span>
            <PageLink page="setup" className="font-medium text-brand-600 hover:underline">{t("connAgent.goSetup")} →</PageLink>
            <PageLink page="admin-connections" className="font-medium text-brand-600 hover:underline">{t("connAgent.goConns")} →</PageLink>
          </div>
        </Card>
        <Card className="p-5">
          <h3 className="mb-1 flex items-center gap-2 font-medium text-neutral-900"><Repeat size={15} /> {t("connAgent.iterateTitle")}</h3>
          <p className="mb-2 text-sm text-neutral-600">{t("connAgent.iterateDesc")}</p>
          <ul className="space-y-1.5">
            {iterate.map((s) => (
              <li key={s} className="rounded-md bg-neutral-50 px-3 py-1.5 font-mono text-xs text-neutral-700 dark:bg-neutral-700/30 dark:text-neutral-200">{s}</li>
            ))}
          </ul>
        </Card>
        <Card className="p-5">
          <h3 className="mb-2 flex items-center gap-2 font-medium text-neutral-900"><Lightbulb size={15} /> {t("connAgent.tipsTitle")}</h3>
          <ul className="list-disc space-y-1.5 pl-5 text-sm text-neutral-700">
            {tips.map((s) => <li key={s}>{s}</li>)}
          </ul>
        </Card>
      </aside>
    </div>
  );
}
