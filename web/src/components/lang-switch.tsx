import { Globe } from "lucide-react";
import { useTranslation } from "react-i18next";
import { LANGUAGES } from "../i18n";
import { cx } from "./ui";

/**
 * 语言切换：选择会记在 localStorage，下次打开沿用。
 *
 * compact：移动端顶栏/落地页手机端用的收起形式（Globe 图标 + 更窄的下拉），
 * 375px 宽下仍可用；桌面端保持默认样式。
 */
export function LangSwitch({ className, compact = false }: { className?: string; compact?: boolean }) {
  const { t, i18n } = useTranslation();
  const current = LANGUAGES.find((l) => i18n.resolvedLanguage === l.code)?.code ?? "zh-CN";
  return (
    <label className={cx("inline-flex shrink-0 items-center gap-1.5 text-sm text-neutral-600", className)}>
      <Globe size={compact ? 17 : 15} aria-hidden className="shrink-0" />
      <span className="sr-only">{t("shell.language")}</span>
      <select
        value={current}
        onChange={(e) => void i18n.changeLanguage(e.target.value)}
        className={cx(
          "cursor-pointer bg-transparent py-1 pr-1 text-sm text-neutral-700 outline-none hover:text-neutral-900",
          compact && "max-w-[5.5rem] text-[13px]",
        )}
      >
        {LANGUAGES.map((l) => (
          <option key={l.code} value={l.code}>
            {l.label}
          </option>
        ))}
      </select>
    </label>
  );
}
