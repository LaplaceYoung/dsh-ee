import { useRef, useState } from "react";
import { useNavigate } from "react-router";
import { useTranslation } from "react-i18next";
import { PackageOpen } from "lucide-react";
import { api, type ExtAdminParam, type ExtPackage, type ExtPlan } from "../lib/api";
import { useAction } from "../lib/use-api";
import { ErrorNote } from "./api-ui";
import { Badge, Button, Field, Notice, PasswordInput, Segmented, cx, inputCls } from "./ui";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/shadcn/dialog";

const MANIFEST = "dsh-ee.extension.json";

/** 把选中的文件读成上传格式：一个 zip 包，或清单和接口文档几个文本文件。 */
async function readPackage(files: File[]): Promise<ExtPackage> {
  const zip = files.find((f) => f.name.toLowerCase().endsWith(".zip"));
  if (zip) {
    const url = await new Promise<string>((resolve, reject) => {
      const r = new FileReader();
      r.onload = () => resolve(String(r.result));
      r.onerror = () => reject(r.error);
      r.readAsDataURL(zip);
    });
    return { zip: url.slice(url.indexOf(",") + 1) };
  }
  const out: Record<string, string> = {};
  for (const f of files) out[f.name] = await f.text();
  return { files: out };
}

/** 管理员设置的参数在输入框里怎么显示：字符串去掉引号，其余按 JSON。 */
function shown(v: unknown) {
  if (v === undefined || v === null) return "";
  return typeof v === "string" ? v : JSON.stringify(v);
}

function parsed(p: ExtAdminParam, text: string): unknown {
  if (p.type === "number" || p.type === "integer") return text.trim() === "" ? undefined : Number(text);
  if (p.type === "boolean") return text === "true";
  if (p.type === "array" || p.type === "object") {
    try {
      return JSON.parse(text);
    } catch {
      return text;
    }
  }
  return text;
}

/**
 * 「安装扩展」按钮（docs/plugins.md §9.4，第 1 批）：选声明式扩展的 zip 包或清单 + 接口文档，
 * 先检查并列出安装计划（会发往哪些地址、用谁的身份、哪些是写操作、要设置的参数、升级时变了什么），
 * 管理员看过、填好之后再装成企业连接。
 */
export function InstallExtensionButton() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const input = useRef<HTMLInputElement>(null);
  const { busy, run } = useAction();
  const [pkg, setPkg] = useState<ExtPackage | null>(null);
  const [plan, setPlan] = useState<ExtPlan | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [slug, setSlug] = useState("");
  const [url, setUrl] = useState("");
  const [authType, setAuthType] = useState<"none" | "bearer" | "header">("bearer");
  const [authHeader, setAuthHeader] = useState("X-API-Key");
  const [secret, setSecret] = useState("");
  const [admin, setAdmin] = useState<Record<string, string>>({});
  const [confirm, setConfirm] = useState(false);

  const close = () => {
    setPkg(null);
    setPlan(null);
    setError(null);
  };

  const pick = async (files: File[]) => {
    setError(null);
    let p: ExtPackage;
    try {
      p = await readPackage(files);
    } catch {
      return setError(t("extensions.readFailed"));
    }
    if ("files" in p && !p.files[MANIFEST]) return setError(t("extensions.needManifest", { file: MANIFEST }));
    const r = await run(() => api.checkExtension(p));
    if (!r.ok) return setError(r.error);
    const pl = r.value;
    setPkg(p);
    setPlan(pl);
    setSlug(pl.slug);
    setUrl(pl.existing?.url ?? (pl.check.servers?.length === 1 ? pl.check.servers[0] : ""));
    setSecret("");
    setConfirm(false);
    const init: Record<string, string> = {};
    for (const ap of pl.admin_params) init[ap.key] = shown(pl.admin?.[ap.key] ?? ap.default);
    setAdmin(init);
  };

  const install = async () => {
    if (!pkg || !plan) return;
    const values: Record<string, unknown> = {};
    for (const ap of plan.admin_params) values[ap.key] = parsed(ap, admin[ap.key] ?? "");
    const shared = plan.check.credential?.mode === "shared";
    const r = await run(() =>
      api.installExtension(pkg, {
        slug: plan.existing ? undefined : slug.trim(),
        url: url.trim() || undefined,
        auth: shared && !plan.existing ? { type: authType, header: authType === "header" ? authHeader.trim() : undefined } : undefined,
        secret: secret.trim() ? secret.trim() : undefined,
        admin: values,
        confirm,
      }),
    );
    if (!r.ok) return setError(r.error);
    close();
    navigate(`/assets/${r.value.asset.id}`);
  };

  const check = plan?.check;
  const cred = check?.credential;
  const needsSecret = cred?.mode === "shared" || !!cred?.sharedHeader;
  const writes = (check?.tools ?? []).filter((x) => !x.readOnly).length;
  const problems = (check?.results ?? []).filter((d) => d.severity !== "info");
  const blocked = !check?.ok || (plan!.sensitive.length > 0 && !confirm);

  return (
    <>
      <input
        ref={input}
        type="file"
        multiple
        accept=".zip,.json,.yaml,.yml"
        className="hidden"
        aria-label={t("extensions.install")}
        onChange={(e) => {
          const fs = Array.from(e.target.files ?? []);
          if (fs.length) void pick(fs);
          e.target.value = "";
        }}
      />
      <Button variant="secondary" disabled={busy} onClick={() => input.current?.click()}>
        <PackageOpen size={15} /> {t("extensions.install")}
      </Button>

      <Dialog open={!!error && !plan} onOpenChange={(v) => !v && setError(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("extensions.checkFailed")}</DialogTitle>
            <DialogDescription>{t("extensions.pickHint", { file: MANIFEST })}</DialogDescription>
          </DialogHeader>
          <ErrorNote>{error}</ErrorNote>
        </DialogContent>
      </Dialog>

      <Dialog open={!!plan} onOpenChange={(v) => !v && close()}>
        <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-2xl" data-extension-plan>
          {plan && check && (
            <>
              <DialogHeader>
                <DialogTitle>{plan.existing ? t("extensions.upgradeTitle", { name: plan.name || plan.slug }) : t("extensions.installTitle", { name: plan.name || plan.slug })}</DialogTitle>
                <DialogDescription>
                  <span className="font-mono">{check.identity ?? "—"}</span>
                  {check.digest && <span className="ml-2 font-mono text-xs text-neutral-500">#{check.digest.slice(0, 12)}</span>}
                  {plan.existing && <span className="ml-2">{t("extensions.upgradeFrom", { version: plan.existing.version, slug: plan.existing.slug })}</span>}
                </DialogDescription>
              </DialogHeader>
              <div className="space-y-4 text-sm">
                {plan.description && <p className="text-neutral-700">{plan.description}</p>}

                {problems.length > 0 && (
                  <Notice tone={check.ok ? "warn" : "danger"}>
                    <p className="font-medium">{check.ok ? t("extensions.warnings", { count: check.warnings }) : t("extensions.errors", { count: check.errors })}</p>
                    <ul className="mt-1 space-y-1">
                      {problems.map((d, i) => (
                        <li key={i}>
                          {d.message}
                          {d.fix && <span className="block text-xs opacity-80">{d.fix}</span>}
                        </li>
                      ))}
                    </ul>
                  </Notice>
                )}

                {check.ok && (
                  <>
                    <dl className="grid grid-cols-[8rem_1fr] gap-x-3 gap-y-2">
                      <dt className="text-neutral-500">{t("extensions.servers")}</dt>
                      <dd className="font-mono text-xs break-all">{(check.servers ?? []).join("\n") || "—"}</dd>
                      <dt className="text-neutral-500">{t("extensions.credential")}</dt>
                      <dd>{cred ? t(`extensions.mode.${cred.mode}`) : t("extensions.mode.none")}</dd>
                      <dt className="text-neutral-500">{t("extensions.tools")}</dt>
                      <dd>{t("extensions.toolCount", { count: check.tools?.length ?? 0, writes })}</dd>
                    </dl>

                    <ul className="divide-y divide-neutral-100 rounded-md border border-neutral-200">
                      {(check.tools ?? []).map((x) => (
                        <li key={x.name} className="flex flex-wrap items-center gap-2 px-3 py-2">
                          <span className="font-mono text-neutral-900">{x.name}</span>
                          {x.method && <span className="font-mono text-xs text-neutral-500">{x.method} {x.path}</span>}
                          <span className="ml-auto">{x.readOnly ? <Badge tone="green">{t("extensions.readOnly")}</Badge> : <Badge tone="amber">{t("extensions.write")}</Badge>}</span>
                        </li>
                      ))}
                    </ul>
                    {writes > 0 && <p className="text-xs text-neutral-500">{t("connections.writeOff")}</p>}

                    {plan.changes.length > 0 && plan.existing && (
                      <div>
                        <p className="mb-1 font-medium text-neutral-900">{t("extensions.changes")}</p>
                        <ul className="list-disc space-y-0.5 pl-5 text-neutral-700">
                          {plan.changes.map((c) => (
                            <li key={c}>{c}</li>
                          ))}
                        </ul>
                      </div>
                    )}

                    {!plan.existing && (
                      <Field label={t("extensions.slug")} hint={t("extensions.slugHint", { slug: slug || "…" })}>
                        <input className={cx(inputCls, "font-mono")} value={slug} onChange={(e) => setSlug(e.target.value)} />
                      </Field>
                    )}
                    {(check.servers?.length ?? 0) > 1 && (
                      <Field label={t("connections.apiBase")} hint={t("extensions.urlHint")}>
                        <select className={inputCls} value={url} onChange={(e) => setUrl(e.target.value)}>
                          <option value="">{t("extensions.urlFromDoc")}</option>
                          {check.servers!.map((s) => (
                            <option key={s} value={s}>
                              {s}
                            </option>
                          ))}
                        </select>
                      </Field>
                    )}

                    {plan.admin_params.map((ap) => (
                      <Field key={ap.key} label={ap.label || ap.key} hint={t("extensions.adminHint", { tools: ap.tools.join("、") })}>
                        {ap.enum?.length ? (
                          <select className={inputCls} value={admin[ap.key] ?? ""} onChange={(e) => setAdmin({ ...admin, [ap.key]: e.target.value })}>
                            {ap.enum.map((o) => (
                              <option key={shown(o)} value={shown(o)}>
                                {shown(o)}
                              </option>
                            ))}
                          </select>
                        ) : (
                          <input className={inputCls} value={admin[ap.key] ?? ""} onChange={(e) => setAdmin({ ...admin, [ap.key]: e.target.value })} />
                        )}
                      </Field>
                    ))}

                    {needsSecret && (
                      <>
                        {cred?.mode === "shared" && !plan.existing && (
                          <Field label={t("connections.auth")}>
                            <Segmented
                              value={authType}
                              onChange={(v) => setAuthType(v)}
                              options={[
                                { value: "none", label: t("connections.authNone") },
                                { value: "bearer", label: t("connections.authBearer") },
                                { value: "header", label: t("connections.authHeader") },
                              ]}
                            />
                          </Field>
                        )}
                        {cred?.mode === "shared" && !plan.existing && authType === "header" && (
                          <Field label={t("connections.authHeaderName")}>
                            <input className={cx(inputCls, "font-mono")} value={authHeader} onChange={(e) => setAuthHeader(e.target.value)} />
                          </Field>
                        )}
                        {!(cred?.mode === "shared" && !plan.existing && authType === "none") && (
                          <Field label={t("connections.secret")} hint={plan.existing ? t("extensions.secretKeep") : t("connections.secretHint")}>
                            <PasswordInput label={t("connections.secret")} className={inputCls} autoComplete="new-password" value={secret} onChange={(e) => setSecret(e.target.value)} />
                          </Field>
                        )}
                      </>
                    )}
                    {(cred?.mode === "ee_jwt" || cred?.mode === "user_secret") && <Notice>{t(`extensions.identity.${cred.mode}`)}</Notice>}

                    {plan.sensitive.length > 0 && (
                      <Notice tone="warn">
                        <p className="font-medium">{t("extensions.sensitive")}</p>
                        <ul className="mt-1 list-disc space-y-0.5 pl-5">
                          {plan.sensitive.map((c) => (
                            <li key={c}>{c}</li>
                          ))}
                        </ul>
                        <label className="mt-2 flex items-center gap-2">
                          <input type="checkbox" checked={confirm} onChange={(e) => setConfirm(e.target.checked)} />
                          {t("extensions.confirm")}
                        </label>
                      </Notice>
                    )}
                  </>
                )}
                {error && <ErrorNote>{error}</ErrorNote>}
              </div>
              <DialogFooter>
                <Button variant="secondary" onClick={close}>
                  {t("extensions.cancel")}
                </Button>
                <Button disabled={busy || blocked} onClick={() => void install()}>
                  {plan.existing ? t("extensions.doUpgrade") : t("extensions.doInstall")}
                </Button>
              </DialogFooter>
            </>
          )}
        </DialogContent>
      </Dialog>
    </>
  );
}
