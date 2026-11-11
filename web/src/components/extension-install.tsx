import { useRef, useState } from "react";
import { useNavigate } from "react-router";
import { useTranslation } from "react-i18next";
import { Upload } from "lucide-react";
import { api, type ExtensionCheckResult, type ExtensionInstallField } from "../lib/api";
import { useAction } from "../lib/use-api";
import { useStore } from "../lib/store";
import { assetPath } from "../lib/hub";
import { Badge, Button, CopyBox, Field, inputCls, Notice, PasswordInput, ToggleRow } from "./ui";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/shadcn/dialog";

async function zipBase64(file: File) {
  const bytes = new Uint8Array(await file.arrayBuffer());
  let binary = "";
  for (let offset = 0; offset < bytes.length; offset += 0x8000) {
    binary += String.fromCharCode(...bytes.subarray(offset, offset + 0x8000));
  }
  return btoa(binary);
}

function defaultConfig(fields: ExtensionInstallField[]) {
  const config: Record<string, unknown> = {};
  for (const field of fields) {
    if (field.default !== undefined && !field.secret) config[field.key] = field.default;
    if (field.type === "boolean" && config[field.key] === undefined) config[field.key] = false;
  }
  return config;
}

function fieldValue(config: Record<string, unknown>, field: ExtensionInstallField) {
  const current = config[field.key];
  if (typeof current === "object" && current !== null) return JSON.stringify(current);
  return String(current ?? "").trim();
}

export function ExtensionInstallDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const { t } = useTranslation();
  const { notify } = useStore();
  const navigate = useNavigate();
  const fileInput = useRef<HTMLInputElement>(null);
  const { busy, run } = useAction();
  const [checking, setChecking] = useState(false);
  const [archive, setArchive] = useState("");
  const [result, setResult] = useState<ExtensionCheckResult | null>(null);
  const [server, setServer] = useState("");
  const [slug, setSlug] = useState("");
  const [config, setConfig] = useState<Record<string, unknown>>({});
  const [sharedSecret, setSharedSecret] = useState("");
  const [error, setError] = useState<string | null>(null);

  const check = async (file?: File) => {
    if (!file) return;
    setError(null);
    setResult(null);
    if (file.size > 8 * 1024 * 1024) {
      setError(t("extensionInstall.fileTooLarge"));
      return;
    }
    setChecking(true);
    try {
      const [encoded, preview] = await Promise.all([zipBase64(file), api.checkExtension(file)]);
      setArchive(encoded);
      setResult(preview);
      setServer(preview.servers?.[0] ?? "");
      setSlug((preview.identity?.split("@")[0].split("/").pop()) ?? "");
      setConfig(defaultConfig(preview.install_fields ?? []));
      setSharedSecret("");
      if (fileInput.current) fileInput.current.value = "";
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setChecking(false);
    }
  };

  const install = async () => {
    if (!result?.ok) return;
    const r = await run(() => api.installExtension({ archive, server, slug, config, ...(sharedSecret ? { shared_secret: sharedSecret } : {}) }));
    if (!r.ok) return setError(r.error);
    if (r.value.sync_error) notify(t("extensionInstall.installedWithWarning", { error: r.value.sync_error }));
    else notify(t("extensionInstall.installed", { id: r.value.extension.id, count: r.value.tools ?? 0 }));
    onOpenChange(false);
    setResult(null);
    setArchive("");
    navigate(assetPath(r.value.asset.id));
  };

  return (
    <Dialog open={open} onOpenChange={(value) => { if (!busy) onOpenChange(value); setError(null); }}>
      <DialogContent className="max-h-[88vh] overflow-y-auto sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>{t("extensionInstall.title")}</DialogTitle>
          <DialogDescription>{t("extensionInstall.desc")}</DialogDescription>
        </DialogHeader>
        <div className="space-y-5 py-3">
          <div className="flex items-center gap-3">
            <input ref={fileInput} type="file" accept=".zip,application/zip" className="hidden" onChange={(e) => void check(e.target.files?.[0])} />
            <Button type="button" variant="secondary" disabled={checking || busy} onClick={() => fileInput.current?.click()}>
              <Upload size={15} /> {checking ? t("extensionInstall.checking") : t("extensionInstall.choose")}
            </Button>
            {result?.identity && <Badge tone={result.ok ? "green" : "red"}>{result.identity}</Badge>}
          </div>
          {error && <Notice tone="danger">{error}</Notice>}
          {result && result.errors > 0 && <Notice tone="danger">{t("extensionInstall.errors", { count: result.errors })}</Notice>}
          {result && result.ok && result.warnings > 0 && <Notice tone="warn">{t("extensionInstall.warnings", { count: result.warnings })}
          </Notice>}

          {result?.identity && (
            <section className="space-y-3 rounded-lg border border-neutral-200 p-4">
              <div className="flex flex-wrap items-center gap-2">
                <h3 className="text-sm font-semibold text-neutral-900">{t("extensionInstall.identity")}</h3>
                <span className="font-mono text-xs text-neutral-700">{result.identity}</span>
              </div>
              {result.digest && <CopyBox text={result.digest} />}
              <div className="grid gap-3 sm:grid-cols-2">
                <div className="text-xs text-neutral-500">{t("extensionInstall.digest")}</div><div />
              </div>
              <Field label={`${t("extensionInstall.connectionSlug")}（${t("extensionInstall.required")}）`} hint={t("extensionInstall.slugHint")}>
                <input className={inputCls} value={slug} onChange={(e) => setSlug(e.target.value)} required />
              </Field>
              <Field label={t("extensionInstall.server")} hint={t("extensionInstall.serverHint")}>
                <select className={inputCls} value={server} onChange={(e) => setServer(e.target.value)}>
                  {(result.servers ?? []).map((url) => <option key={url} value={url}>{url}</option>)}
                </select>
              </Field>
              <details>
                <summary className="cursor-pointer text-sm font-medium text-neutral-900">{t("extensionInstall.tools", { count: result.tools?.length ?? 0 })}</summary>
                <ul className="mt-3 space-y-2">
                  {(result.tools ?? []).map((tool) => (
                    <li key={tool.name} className="border-t border-neutral-100 pt-2 text-sm first:border-t-0 first:pt-0">
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="font-mono text-xs text-neutral-900">{tool.name}</span>
                        <Badge tone={tool.readOnly ? "green" : "amber"}>{tool.readOnly ? t("extensionInstall.readOnly") : t("extensionInstall.write")}</Badge>
                        {tool.method && <span className="text-xs text-neutral-500">{tool.method} {tool.path}</span>}
                      </div>
                      {(tool.fixed?.length ?? 0) > 0 && <div className="mt-1 text-xs text-neutral-500">{t("extensionInstall.fixedParams")}：{tool.fixed!.map((p) => p.name).join("、")}</div>}
                      {(tool.keep?.length ?? 0) > 0 && <div className="mt-1 text-xs text-neutral-500">{t("extensionInstall.keepFields", { fields: tool.keep!.join("、") })}</div>}
                    </li>
                  ))}
                </ul>
              </details>
            </section>
          )}

          {result?.ok && (
            <section className="space-y-4">
              <h3 className="text-sm font-semibold text-neutral-900">{t("extensionInstall.configTitle")}</h3>
              <p className="text-xs text-neutral-500">{t("extensionInstall.optionalDefault")}</p>
              {(result.install_fields ?? []).map((field) => (
                <InstallField key={field.key} field={field} value={config[field.key]} onChange={(value) => setConfig((current) => ({ ...current, [field.key]: value }))} />
              ))}
              {result.credential?.sharedHeader && (
                <Field label={`${t("extensionInstall.sharedSecret", { header: result.credential.sharedHeader })}（${t("extensionInstall.required")}）`} hint={t("extensionInstall.sharedSecretHint")}>
                  <PasswordInput label={t("extensionInstall.sharedSecret", { header: result.credential.sharedHeader })} className={inputCls} value={sharedSecret} onChange={(e) => setSharedSecret(e.target.value)} autoComplete="new-password" required />
                </Field>
              )}
            </section>
          )}
        </div>
        <DialogFooter>
          <Button type="button" variant="secondary" disabled={busy} onClick={() => onOpenChange(false)}>{t("common.cancel")}</Button>
          <Button type="button" disabled={busy || !result?.ok || !server || !slug || (result.credential?.sharedHeader !== undefined && !sharedSecret)} onClick={() => void install()}>
            {busy ? t("extensionInstall.installing") : t("extensionInstall.install")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function InstallField({ field, value, onChange }: { field: ExtensionInstallField; value: unknown; onChange: (value: unknown) => void }) {
  const { t } = useTranslation();
  const label = (field.label || field.key) + (field.required ? `（${t("extensionInstall.required")}）` : "");
  const common = { label, hint: field.help };
  if (field.type === "boolean") {
    return <ToggleRow label={label} desc={field.help} checked={value === true} onChange={onChange} />;
  }
  if (field.type === "select") {
    const index = Math.max(0, (field.options ?? []).findIndex((opt) => JSON.stringify(opt.value) === JSON.stringify(value)));
    return (
      <Field {...common}>
        <select className={inputCls} value={index} onChange={(e) => {
          const option = field.options?.[Number(e.target.value)];
          if (option) onChange(option.value);
        }}>
          {(field.options ?? []).map((opt, i) => <option key={`${field.key}-${i}`} value={i}>{opt.label}</option>)}
        </select>
      </Field>
    );
  }
  if (field.type === "array" || field.type === "object") {
    return (
      <Field {...common}>
        <textarea className={`${inputCls} h-24 font-mono text-xs`} value={fieldValue({ [field.key]: value }, field)} onChange={(e) => {
          try {
            onChange(JSON.parse(e.target.value));
          } catch {
            onChange(e.target.value);
          }
        }} required={field.required} />
      </Field>
    );
  }
  if (field.secret) {
    return (
      <Field {...common}>
        <PasswordInput label={label} className={inputCls} value={fieldValue({ [field.key]: value }, field)} onChange={(e) => onChange(e.target.value)} autoComplete="new-password" required={field.required} />
      </Field>
    );
  }
  return (
    <Field {...common}>
      <input className={inputCls} type={field.type === "number" ? "number" : "text"} value={fieldValue({ [field.key]: value }, field)} onChange={(e) => onChange(e.target.value)} required={field.required} />
    </Field>
  );
}
