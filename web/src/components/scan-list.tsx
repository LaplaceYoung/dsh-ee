import { AlertTriangle, CheckCircle2, XCircle } from "lucide-react";
import { useTranslation } from "react-i18next";
import type { ScanItem } from "../lib/api";
import { formatBytes } from "../lib/api";

/** 自动检查结果：服务端给 key / level / params，这里按当前语言渲染成一句话。 */
export function ScanList({ items }: { items: ScanItem[] | null }) {
  const { t: typed } = useTranslation();
  const t = typed as unknown as DynT;
  if (!items?.length) return <p className="text-sm text-neutral-500">{typed("registry.scan.none")}</p>;
  return (
    <ul className="divide-y divide-neutral-100 rounded-lg border border-neutral-100">
      {items.map((s) => (
        <li key={s.key} className="flex items-start gap-3 px-4 py-2.5 text-sm">
          {s.level === "pass" && <CheckCircle2 size={17} className="mt-0.5 shrink-0 text-emerald-500" />}
          {s.level === "warn" && <AlertTriangle size={17} className="mt-0.5 shrink-0 text-amber-500" />}
          {s.level === "fail" && <XCircle size={17} className="mt-0.5 shrink-0 text-red-500" />}
          <span className="w-28 shrink-0 font-medium text-neutral-700">{t(`registry.scan.${s.key}.label`, { defaultValue: s.key })}</span>
          <span className="min-w-0 break-words text-neutral-600">{scanDetail(t, s)}</span>
        </li>
      ))}
    </ul>
  );
}

/** 检查项的键来自服务端，是运行时字符串，绕开编译期的键检查（缺键时退回 defaultValue）。 */
type DynT = (key: string, opts?: Record<string, unknown>) => string;

function scanDetail(t: DynT, s: ScanItem): string {
  const p = s.params ?? {};
  switch (s.key) {
    case "bundle":
      return s.level === "pass" ? t("registry.scan.bundle.pass", { path: p.path }) : t(`registry.scan.bundle.${p.reason}`, { path: p.path });
    case "size":
      return t(`registry.scan.size.${s.level}`, { size: formatBytes(Number(p.size)), unpacked: formatBytes(Number(p.unpacked)), files: p.files });
    case "patch":
      if (s.level === "pass") return t("registry.scan.patch.pass", { path: p.path, rows: p.rows });
      return t(`registry.scan.patch.${s.level}`, {
        path: p.path,
        reason: t(`registry.scan.patch.reason.${p.reason}`, { ...p, defaultValue: p.reason }),
      });
    default:
      return t(`registry.scan.${s.key}.${s.level}`, { ...p, defaultValue: JSON.stringify(p) });
  }
}

export function scanWarnings(items: ScanItem[] | null) {
  return (items ?? []).filter((s) => s.level !== "pass").length;
}
