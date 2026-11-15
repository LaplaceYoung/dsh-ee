import { useEffect, useState, type CSSProperties } from "react";
import { useTranslation } from "react-i18next";
import { ArrowUp, Clock, Folder, LayoutGrid, Plus, Puzzle, ChevronRight } from "lucide-react";
import { api } from "../../lib/api";
import { useStore } from "../../lib/store";
import type { BrandColor, BrandColorKey, BrandLogoShape, Settings } from "../../lib/types";
import { Button, cx, Field, inputBase, inputCls, Segmented, ToggleRow } from "../../components/ui";
import { ImageCropDialog, needsCrop } from "../../components/image-crop";

type Brand = Settings["brand"];
type TextKey = keyof (typeof import("../../i18n/zh/config"))["default"]["brandAdmin"];
type Mode = "light" | "dark";

const COLOR_KEYS: BrandColorKey[] = ["accent", "accentText", "accentSoft", "sidebar", "sidebarActive", "background", "input"];

const COLOR_LABELS: Record<BrandColorKey, [TextKey, TextKey]> = {
  accent: ["colorAccent", "colorAccentDesc"],
  accentText: ["colorAccentText", "colorAccentTextDesc"],
  accentSoft: ["colorAccentSoft", "colorAccentSoftDesc"],
  sidebar: ["colorSidebar", "colorSidebarDesc"],
  sidebarActive: ["colorSidebarActive", "colorSidebarActiveDesc"],
  background: ["colorBackground", "colorBackgroundDesc"],
  input: ["colorInput", "colorInputDesc"],
};

/** DSH 默认配色的近似值，只用于预览和色块的初始值。 */
const DSH_DEFAULTS: Record<BrandColorKey, BrandColor> = {
  accent: { light: "#0f1115", dark: "#f0f2f5" },
  accentText: { light: "#ffffff", dark: "#0f1115" },
  accentSoft: { light: "#e4ecfd", dark: "#1d2a48" },
  sidebar: { light: "#f5f6f8", dark: "#1c1e23" },
  sidebarActive: { light: "#e8eaee", dark: "#2c2f36" },
  background: { light: "#ffffff", dark: "#131419" },
  input: { light: "#ffffff", dark: "#24262c" },
};

type Colors = Partial<Record<BrandColorKey, BrandColor>>;

const PRESETS: { id: string; label: TextKey; colors: Colors }[] = [
  { id: "", label: "presetDefault", colors: {} },
  {
    id: "star", label: "presetStar", colors: {
      accent: { light: "#2563eb", dark: "#60a5fa" },
      accentSoft: { light: "#dbeafe", dark: "#1e3a8a" },
      sidebar: { light: "#f3f5f9", dark: "" },
      sidebarActive: { light: "#e5eaf3", dark: "" },
    },
  },
  {
    id: "forest", label: "presetForest", colors: {
      accent: { light: "#15803d", dark: "#4ade80" },
      accentSoft: { light: "#dcfce7", dark: "#14532d" },
      sidebar: { light: "#f2f6f3", dark: "" },
      sidebarActive: { light: "#e1ebe4", dark: "" },
    },
  },
  {
    id: "sunset", label: "presetSunset", colors: {
      accent: { light: "#ea580c", dark: "#fb923c" },
      accentSoft: { light: "#ffedd5", dark: "#7c2d12" },
      sidebar: { light: "#faf6f2", dark: "" },
      sidebarActive: { light: "#f2e6db", dark: "" },
    },
  },
  {
    id: "red", label: "presetRed", colors: {
      accent: { light: "#c8102e", dark: "#f87171" },
      accentSoft: { light: "#fee2e2", dark: "#7f1d1d" },
      sidebar: { light: "#faf5f5", dark: "" },
      sidebarActive: { light: "#f2e2e2", dark: "" },
    },
  },
  {
    id: "violet", label: "presetViolet", colors: {
      accent: { light: "#7c3aed", dark: "#a78bfa" },
      accentSoft: { light: "#ede9fe", dark: "#4c1d95" },
      sidebar: { light: "#f6f5fb", dark: "" },
      sidebarActive: { light: "#e8e4f5", dark: "" },
    },
  },
  {
    id: "graphite", label: "presetGraphite", colors: {
      accent: { light: "#27272a", dark: "#e4e4e7" },
      accentSoft: { light: "#ececee", dark: "#3f3f46" },
      sidebar: { light: "#f4f4f5", dark: "#18181b" },
      sidebarActive: { light: "#e4e4e7", dark: "#27272a" },
    },
  },
];

function validColor(v: string) {
  if (v === "") return true;
  if (/url|expression|["';{}\\<>]/i.test(v)) return false;
  return typeof CSS !== "undefined" && CSS.supports ? CSS.supports("color", v) : /^#[0-9a-f]{3,8}$/i.test(v);
}

/** 色块选择器只认 #rrggbb；其余写法（rgb()、颜色名）只在文字框里改。 */
function hex6(v: string) {
  if (/^#[0-9a-f]{6}$/i.test(v)) return v.toLowerCase();
  const m = /^#([0-9a-f])([0-9a-f])([0-9a-f])$/i.exec(v);
  return m ? `#${m[1]}${m[1]}${m[2]}${m[2]}${m[3]}${m[3]}`.toLowerCase() : "";
}

function colorOf(colors: Colors, key: BrandColorKey, mode: Mode) {
  return colors[key]?.[mode] || DSH_DEFAULTS[key][mode];
}

function ColorInput({ label, value, fallback, onChange }: { label: string; value: string; fallback: string; onChange: (v: string) => void }) {
  const { t } = useTranslation();
  const [draft, setDraft] = useState(value);
  useEffect(() => setDraft(value), [value]);
  const ok = validColor(draft.trim());
  return (
    <div className="flex items-center gap-1.5">
      <input
        type="color"
        aria-label={label}
        className="h-8 w-8 shrink-0 cursor-pointer rounded border border-neutral-300 bg-card p-0.5"
        value={hex6(value) || hex6(fallback) || "#000000"}
        onChange={(e) => onChange(e.target.value)}
      />
      <input
        aria-label={`${label} CSS`}
        className={cx(inputBase, "h-8 w-28 font-mono text-xs", !ok && "border-red-400 focus:border-red-500")}
        placeholder={t("brandAdmin.useDefault")}
        value={draft}
        title={ok ? undefined : t("brandAdmin.invalidColor")}
        onChange={(e) => {
          const v = e.target.value;
          setDraft(v);
          if (validColor(v.trim())) onChange(v.trim());
        }}
        onBlur={() => setDraft(value)}
      />
    </div>
  );
}

function ColorRow({ k, colors, setColor }: { k: BrandColorKey; colors: Colors; setColor: (k: BrandColorKey, mode: Mode, v: string) => void }) {
  const { t } = useTranslation();
  const [name, desc] = COLOR_LABELS[k];
  const label = t(`brandAdmin.${name}`);
  const cur = colors[k];
  return (
    <div className="grid grid-cols-1 items-center gap-2 py-2 sm:grid-cols-[1fr_auto_auto_auto]" data-brand-color={k}>
      <div className="min-w-0">
        <div className="text-sm font-medium text-neutral-800">{label}</div>
        <div className="text-xs text-neutral-500">{t(`brandAdmin.${desc}`)}</div>
      </div>
      <ColorInput label={`${label} ${t("brandAdmin.light")}`} value={cur?.light ?? ""} fallback={DSH_DEFAULTS[k].light} onChange={(v) => setColor(k, "light", v)} />
      <ColorInput label={`${label} ${t("brandAdmin.dark")}`} value={cur?.dark ?? ""} fallback={DSH_DEFAULTS[k].dark} onChange={(v) => setColor(k, "dark", v)} />
      <button
        type="button"
        className={cx("text-xs text-neutral-500 hover:text-neutral-800", !cur && "invisible")}
        onClick={() => { setColor(k, "light", ""); setColor(k, "dark", ""); }}
      >
        {t("brandAdmin.clear")}
      </button>
    </div>
  );
}

function logoStyle(shape: BrandLogoShape, size: number): CSSProperties {
  if (shape === "plain") return { width: size, height: size, objectFit: "contain" };
  return { width: size, height: size, objectFit: "cover", borderRadius: shape === "rounded" ? Math.round(size * 0.22) : "50%" };
}

function LogoMark({ brand, fallbackName, size }: { brand: Brand; fallbackName: string; size: number }) {
  const { t } = useTranslation();
  if (brand.logoRev) {
    return <img src={`/api/dsh/brand/logo?rev=${encodeURIComponent(brand.logoRev)}`} alt="" style={logoStyle(brand.logoShape, size)} />;
  }
  return (
    <span
      className="inline-flex shrink-0 items-center justify-center bg-neutral-200 font-semibold text-neutral-600"
      style={{ ...logoStyle(brand.logoShape === "plain" ? "rounded" : brand.logoShape, size), fontSize: Math.round(size * 0.42) }}
    >
      {(fallbackName || t("brandAdmin.logoFallback")).charAt(0).toUpperCase()}
    </span>
  );
}

/** DSH 首屏的示意图：侧栏 + 首屏标题 + 输入卡片，按当前配色上色。 */
function DshPreview({ brand, orgName, mode }: { brand: Brand; orgName: string; mode: Mode }) {
  const { t } = useTranslation();
  const c = (k: BrandColorKey) => colorOf(brand.theme.colors, k, mode);
  const dark = mode === "dark";
  const text = dark ? "#e6e8ec" : "#16181d";
  const muted = dark ? "#8b9099" : "#6b7280";
  const name = brand.name || orgName || "DSH";
  const border = dark ? "#ffffff14" : "#0000000f";
  return (
    <div className="flex h-80 overflow-hidden rounded-xl text-[11px] shadow-card ring-1 ring-neutral-200" style={{ background: c("background"), color: text }} data-testid={`brand-preview-${mode}`}>
      <div className="flex w-44 shrink-0 flex-col gap-1 p-3" style={{ background: c("sidebar"), borderRight: `1px solid ${border}` }}>
        <div className="mb-2 flex items-center gap-2 text-sm font-semibold">
          <LogoMark brand={brand} fallbackName={name} size={22} />
          <span className="truncate">{name}</span>
        </div>
        <div className="mb-2 flex items-center justify-center gap-1.5 rounded-lg py-1.5 font-medium" style={{ background: c("input"), border: `1px solid ${border}` }}>
          <span className="inline-flex h-3.5 w-3.5 items-center justify-center rounded-full" style={{ background: c("accent"), color: c("accentText") }}><Plus size={10} strokeWidth={3} /></span>
          {t("brandAdmin.mockNewChat")}
        </div>
        {([[Puzzle, "mockPlugins"], [Clock, "mockTasks"], [LayoutGrid, "mockMore"]] as const).map(([I, key]) => (
          <div key={key} className="flex items-center gap-2 rounded-md px-2 py-1"><I size={12} />{t(`brandAdmin.${key}`)}</div>
        ))}
        <div className="mt-2 px-2 text-[10px]" style={{ color: muted }}>{t("brandAdmin.mockWorkspace")}</div>
        <div className="flex items-center gap-2 px-2 py-1"><Folder size={12} />{name}</div>
        <div className="rounded-md px-2 py-1 pl-6" style={{ background: c("sidebarActive") }}>{t("brandAdmin.mockChat1")}</div>
        <div className="px-2 py-1 pl-6">{t("brandAdmin.mockChat2")}</div>
      </div>
      <div className="flex min-w-0 flex-1 flex-col items-center justify-center gap-4 px-6">
        <div className="flex items-center gap-2.5 text-lg font-semibold">
          <LogoMark brand={brand} fallbackName={name} size={brand.title ? 36 : 26} />
          <span className="truncate">{brand.title || t("brandAdmin.mockDefaultTitle")}</span>
        </div>
        <div className="w-full max-w-sm rounded-2xl p-3" style={{ background: c("input"), border: `1px solid ${border}`, boxShadow: dark ? "none" : "0 2px 10px #0000000d" }}>
          <div className="h-8" style={{ color: muted }}>{t("brandAdmin.mockPlaceholder")}</div>
          <div className="flex items-center justify-between">
            <span className="inline-flex h-5 w-5 items-center justify-center rounded-full" style={{ border: `1px solid ${border}` }}><Plus size={11} /></span>
            <span className="inline-flex h-6 w-6 items-center justify-center rounded-full" style={{ background: c("accent"), color: c("accentText") }}><ArrowUp size={13} strokeWidth={2.5} /></span>
          </div>
        </div>
        <div className="flex gap-2">
          <span className="rounded-full px-2 py-0.5" style={{ background: c("accentSoft"), color: c("accent") }}>{t("brandAdmin.mockChip1")}</span>
          <span className="rounded-full px-2 py-0.5" style={{ background: c("accentSoft"), color: c("accent") }}>{t("brandAdmin.mockChip2")}</span>
        </div>
      </div>
    </div>
  );
}

function TokensEditor({ brand, set }: { brand: Brand; set: (patch: Partial<Brand>) => void }) {
  const { t } = useTranslation();
  const text = (tokens: Record<string, BrandColor>) => (Object.keys(tokens).length ? JSON.stringify(tokens, null, 2) : "");
  const [draft, setDraft] = useState(text(brand.theme.tokens));
  const [err, setErr] = useState("");
  useEffect(() => setDraft(text(brand.theme.tokens)), [brand.theme.tokens]);
  const commit = () => {
    try {
      const parsed = draft.trim() ? JSON.parse(draft) : {};
      if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) throw new Error("{ }");
      setErr("");
      set({ theme: { ...brand.theme, preset: brand.theme.preset && "custom", tokens: parsed } });
    } catch (e) {
      setErr(t("brandAdmin.advancedInvalid", { msg: e instanceof Error ? e.message : String(e) }));
    }
  };
  return (
    <details className="group rounded-lg border border-neutral-200 px-4 py-3" open={Object.keys(brand.theme.tokens).length > 0}>
      <summary className="flex cursor-pointer list-none items-center gap-1.5 text-sm font-medium text-neutral-700 select-none [&::-webkit-details-marker]:hidden"><ChevronRight size={14} aria-hidden className="text-neutral-400 transition-transform group-open:rotate-90" />{t("brandAdmin.advanced")}</summary>
      <p className="mt-2 text-xs text-neutral-500">{t("brandAdmin.advancedDesc")}</p>
      <textarea
        aria-label={t("brandAdmin.advanced")}
        className={cx(inputCls, "mt-2 h-32 py-2 font-mono text-xs")}
        placeholder={'{\n  "--dsw-alias-label-primary": { "light": "#0f172a", "dark": "" }\n}'}
        value={draft}
        onChange={(e) => setDraft(e.target.value)}
        onBlur={commit}
      />
      {err && <p className="mt-1 text-xs text-red-600">{err}</p>}
    </details>
  );
}

export default function BrandSettings() {
  const { t } = useTranslation();
  const { settings: s, updateSettings, notify } = useStore();
  const [busy, setBusy] = useState(false);
  const [mode, setMode] = useState<Mode>("light");
  const brand = s.brand;
  const set = (patch: Partial<Brand>) => updateSettings((cur) => ({ ...cur, brand: { ...cur.brand, ...patch } }));
  const setColor = (k: BrandColorKey, m: Mode, v: string) =>
    updateSettings((cur) => {
      const colors = { ...cur.brand.theme.colors };
      const next = { light: "", dark: "", ...colors[k], [m]: v };
      if (next.light || next.dark) colors[k] = next;
      else delete colors[k];
      return { ...cur, brand: { ...cur.brand, theme: { ...cur.brand.theme, preset: "custom", colors } } };
    });
  const [cropFile, setCropFile] = useState<File | null>(null);
  const upload = async (file?: File) => {
    if (!file) return;
    setBusy(true);
    try {
      const result = await api.uploadBrandLogo(file);
      set({ logoRev: result.logo_rev });
      notify(t("brandAdmin.uploaded"));
    } catch (e) {
      notify(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
      setCropFile(null);
    }
  };
  const pick = (file?: File) => {
    // SVG 是矢量不用裁；位图先裁成正方形 PNG 再上传，不再要求原图必须是正方形
    if (!file) return;
    if (needsCrop(file)) setCropFile(file);
    else void upload(file);
  };
  const preset = brand.theme.preset;
  return (
    <div className="space-y-5">
      <div id="brand-basics" className="scroll-mt-6 space-y-5">
      <ToggleRow label={t("brandAdmin.enabled")} desc={t("brandAdmin.enabledDesc")} checked={brand.enabled} onChange={(v) => set({ enabled: v })} />
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label={t("brandAdmin.name")} hint={t("brandAdmin.nameHint")}>
          <input className={inputCls} maxLength={24} value={brand.name} onChange={(e) => set({ name: e.target.value })} />
        </Field>
        <Field label={t("brandAdmin.heroTitle")} hint={t("brandAdmin.heroTitleHint")}>
          <input className={inputCls} maxLength={32} value={brand.title} placeholder={brand.name ? `${brand.name}…` : ""} onChange={(e) => set({ title: e.target.value })} />
        </Field>
      </div>
      </div>
      <div id="brand-logo" className="flex scroll-mt-6 flex-wrap items-start gap-5">
        <div className="flex h-16 w-16 items-center justify-center">
          <LogoMark brand={brand} fallbackName={brand.name || s.orgName || "DSH"} size={64} />
        </div>
        <div className="min-w-0 flex-1 basis-56 space-y-3">
          <Field label={t("brandAdmin.logo")} hint={t("brandAdmin.logoHint")}>
            <input aria-label={t("brandAdmin.logo")} type="file" accept="image/*" disabled={busy} onChange={(e) => { pick(e.target.files?.[0]); e.target.value = ""; }} />
          </Field>
          <ImageCropDialog file={cropFile} busy={busy} onDone={(f) => void upload(f)} onCancel={() => setCropFile(null)} />
          <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
            <span className="text-sm font-medium text-neutral-700">{t("brandAdmin.logoShape")}</span>
            <Segmented<BrandLogoShape>
              value={brand.logoShape || "circle"}
              onChange={(v) => set({ logoShape: v })}
              options={[
                { value: "circle", label: t("brandAdmin.shapeCircle") },
                { value: "rounded", label: t("brandAdmin.shapeRounded") },
                { value: "plain", label: t("brandAdmin.shapePlain") },
              ]}
            />
          </div>
        </div>
      </div>
      {busy && <Button size="sm" disabled>{t("brandAdmin.uploading")}</Button>}

      <div id="brand-theme" className="scroll-mt-6 space-y-3 border-t border-neutral-200 pt-5">
        <div>
          <div className="text-sm font-semibold text-neutral-900">{t("brandAdmin.themeTitle")}</div>
          <p className="mt-0.5 text-xs text-neutral-500">{t("brandAdmin.themeDesc")}</p>
        </div>
        <div className="flex flex-wrap gap-2">
          {PRESETS.map((p) => {
            const active = preset === p.id && (p.id !== "" || Object.keys(brand.theme.colors).length === 0);
            return (
              <button
                key={p.id || "default"}
                type="button"
                data-preset={p.id || "default"}
                aria-pressed={active}
                onClick={() => set({ theme: { ...brand.theme, preset: p.id, colors: structuredClone(p.colors) } })}
                className={cx(
                  "flex items-center gap-2 rounded-full border px-3 py-1.5 text-sm transition",
                  active ? "border-neutral-900 bg-neutral-900 text-white" : "border-neutral-200 text-neutral-700 hover:border-neutral-400",
                )}
              >
                <span className="h-3.5 w-3.5 rounded-full ring-1 ring-black/10" style={{ background: colorOf(p.colors, "accent", "light") }} />
                {t(`brandAdmin.${p.label}`)}
              </button>
            );
          })}
          {preset === "custom" && <span className="self-center text-xs text-neutral-500">· {t("brandAdmin.custom")}</span>}
        </div>
        <div className="divide-y divide-neutral-100">
          <div className="hidden grid-cols-[1fr_auto_auto_auto] gap-2 pb-1 text-xs text-neutral-500 sm:grid">
            <span />
            <span className="w-[9.5rem]">{t("brandAdmin.light")}</span>
            <span className="w-[9.5rem]">{t("brandAdmin.dark")}</span>
            <span className="invisible text-xs">{t("brandAdmin.clear")}</span>
          </div>
          {COLOR_KEYS.map((k) => <ColorRow key={k} k={k} colors={brand.theme.colors} setColor={setColor} />)}
        </div>
        <TokensEditor brand={brand} set={set} />
      </div>

      <div id="brand-preview" className="scroll-mt-6 space-y-2 border-t border-neutral-200 pt-5">
        <div className="flex items-center justify-between">
          <span className="text-sm font-semibold text-neutral-900">{t("brandAdmin.preview")}</span>
          <Segmented<Mode> value={mode} onChange={setMode} options={[{ value: "light", label: t("brandAdmin.previewLight") }, { value: "dark", label: t("brandAdmin.previewDark") }]} />
        </div>
        <DshPreview brand={brand} orgName={s.orgName} mode={mode} />
        <p className="text-xs text-neutral-500">{t("brandAdmin.previewNote")}</p>
      </div>
      <p className="rounded-lg bg-neutral-50 px-3 py-2 text-xs text-neutral-600">{t("brandAdmin.agentTip")}</p>
      <p className={cx("text-xs text-neutral-500", !brand.enabled && "opacity-60")}>{t("brandAdmin.runtimeNote")}</p>
    </div>
  );
}
