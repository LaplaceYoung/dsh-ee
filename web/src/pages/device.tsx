import { CircleCheck, CircleSlash } from "lucide-react";
import { useEffect, useRef, useState, type FormEvent } from "react";
import { useSearchParams } from "react-router";
import { useTranslation } from "react-i18next";
import { api, formatTime } from "../lib/api";
import { useAction, useApi } from "../lib/use-api";
import { ErrorNote, LoadState } from "../components/api-ui";
import { useStore } from "../lib/store";
import { Badge, Button, Card, Field, inputCls, PageHeader, Notice, Empty } from "../components/ui";

export default function DevicePage() {
  const { t } = useTranslation();
  const [params, setParams] = useSearchParams();
  const code = params.get("code") ?? "";
  const { data, error, loading, reload } = useApi(() => (code ? api.deviceInfo(code) : Promise.resolve(null)));
  const { busy, run } = useAction();
  const { notify } = useStore();
  const [manual, setManual] = useState("");
  const [done, setDone] = useState<"approved" | "denied" | null>(null);
  const initial = useRef(true);
  useEffect(() => {
    if (initial.current) {
      initial.current = false;
      return;
    }
    reload();
  }, [code, reload]);

  const decide = async (approve: boolean) => {
    const r = await run(() => api.approveDevice(code, approve));
    if (r.ok) setDone(approve ? "approved" : "denied");
    else notify(r.error);
  };

  const submit = (e: FormEvent) => {
    e.preventDefault();
    setDone(null);
    setParams({ code: manual.trim().toUpperCase() });
  };

  return (
    <div className="mx-auto max-w-2xl py-8">
      <PageHeader title={t("device.title")} desc={t("device.desc")} />
      {done ? (
        <Card>
          <Empty icon={done === "approved" ? CircleCheck : CircleSlash} title={<Badge tone={done === "approved" ? "green" : "amber"}>{t(`device.${done}`)}</Badge>}>
            <span className="text-neutral-700">{t(`device.${done}Hint`)}</span>
          </Empty>
        </Card>
      ) : (
        <Card>
          <div className="p-6">
            <LoadState loading={loading && code.length > 0} error={error} onRetry={reload}>
              {data && (
                <div className="space-y-5">
                  <div><h2 className="text-lg font-semibold text-neutral-900">{t("device.authorizeTitle")}</h2><p className="mt-1 text-sm text-neutral-600">{t("device.authorizeDesc")}</p></div>
                  <dl className="grid gap-4 rounded-lg border border-neutral-200/80 bg-neutral-50/60 p-4 text-sm sm:grid-cols-2 dark:border-white/[0.07]">
                    <div><dt className="text-neutral-500">{t("device.code")}</dt><dd className="mt-1 font-mono text-lg font-semibold tracking-wider text-neutral-900">{data.user_code}</dd></div>
                    <div><dt className="text-neutral-500">{t("device.expires")}</dt><dd className="mt-1">{formatTime(data.expires_at)}</dd></div>
                    <div><dt className="text-neutral-500">{t("device.name")}</dt><dd className="mt-1">{data.client_name || "—"}</dd></div>
                    <div><dt className="text-neutral-500">{t("device.os")}</dt><dd className="mt-1">{data.client_os || "—"}</dd></div>
                    <div><dt className="text-neutral-500">{t("device.version")}</dt><dd className="mt-1">{data.client_version || "—"}</dd></div>
                    <div><dt className="text-neutral-500">{t("device.ip")}</dt><dd className="mt-1 font-mono">{data.client_ip || "—"}</dd></div>
                  </dl>
                  <div><h3 className="mb-2 text-sm font-semibold text-neutral-900">{t("device.scopes")}</h3><div className="flex flex-wrap gap-2">{data.scopes.map((scope) => <Badge key={scope}>{t(`tokens.scopes.${scope}`)}</Badge>)}</div></div>
                  {data.replace_existing && <Notice tone="warn">{t("device.replaceNote")}</Notice>}
                  <div className="flex flex-wrap justify-end gap-2 border-t border-neutral-100 pt-5"><Button variant="secondary" disabled={busy} onClick={() => decide(false)}>{t("device.deny")}</Button><Button disabled={busy} onClick={() => decide(true)}>{t("device.approve")}</Button></div>
                </div>
              )}
            </LoadState>
            {!code && (
              <form onSubmit={submit} className="space-y-4">
                <Field label={t("device.inputCode")} hint={t("device.inputHint")}><input className={inputCls} required value={manual} onChange={(e) => setManual(e.target.value)} placeholder="KQ7M-4XPD" /></Field>
                <Button type="submit">{t("device.continue")}</Button>
              </form>
            )}
          </div>
        </Card>
      )}
      <ErrorNote>{error}</ErrorNote>
    </div>
  );
}
