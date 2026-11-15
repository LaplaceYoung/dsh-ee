/** 可搜索的下拉选择与日期选择，替代原生 <select> / <input type="date">。 */
import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { CalendarIcon, Check, ChevronsUpDown, X } from "lucide-react";
import { enUS, zhCN } from "react-day-picker/locale";
import type { Locale } from "react-day-picker";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/shadcn/popover";
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList } from "@/components/shadcn/command";
import { Calendar } from "@/components/shadcn/calendar";
import { formatDateOnly, resolveLang, type FormatLang } from "@/lib/format";
import { cx, inputBase } from "./ui";
import { cn } from "@/lib/utils";

/**
 * 日历 locale：en / zh-CN 在首包里（静态 import），es / pt / hi 按需动态 import，
 * 不把另外三种语言的 date-fns locale 塞进首屏 bundle。
 * 动态 chunk 加载失败时回退 enUS（离线部署下最多日历是英文，不影响选择行为）。
 */
const BASE_LOCALES: Partial<Record<FormatLang, Locale>> = { en: enUS, "zh-CN": zhCN };
const EXTRA_LOCALE_LOADERS: Partial<Record<FormatLang, () => Promise<Locale>>> = {
  es: () => import("react-day-picker/locale/es").then((m) => m.es),
  pt: () => import("react-day-picker/locale/pt").then((m) => m.pt),
  hi: () => import("react-day-picker/locale/hi").then((m) => m.hi),
};

function useCalendarLocale(): Locale {
  const { i18n } = useTranslation();
  const lang = resolveLang(i18n.resolvedLanguage);
  const [loaded, setLoaded] = useState<{ lang: FormatLang; locale: Locale } | null>(null);
  useEffect(() => {
    const load = EXTRA_LOCALE_LOADERS[lang];
    if (!load) return;
    let alive = true;
    load()
      .then((locale) => {
        if (alive) setLoaded({ lang, locale });
      })
      .catch(() => {
        // 保持 enUS 兜底
      });
    return () => {
      alive = false;
    };
  }, [lang]);
  if (BASE_LOCALES[lang]) return BASE_LOCALES[lang];
  return loaded?.lang === lang ? loaded.locale : enUS;
}

export interface ComboOption {
  value: string;
  label: string;
  /** 第二行灰字，例如部门、代码。也参与搜索。 */
  hint?: string;
  /** 分组名；同组的选项挨在一起显示。 */
  group?: string;
}

/**
 * 可搜索的单选下拉。value 为空字符串表示未选；allowEmpty 时列表顶部有一项「emptyLabel」。
 */
export function Combobox({
  value,
  onChange,
  options,
  placeholder,
  searchPlaceholder,
  emptyLabel,
  allowEmpty = false,
  id,
  className,
  "aria-label": ariaLabel,
}: {
  value: string;
  onChange: (v: string) => void;
  options: ComboOption[];
  placeholder?: string;
  searchPlaceholder?: string;
  /** allowEmpty 时「不选」那一项的文字。 */
  emptyLabel?: string;
  allowEmpty?: boolean;
  id?: string;
  className?: string;
  "aria-label"?: string;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const current = options.find((o) => o.value === value);
  const groups = [...new Set(options.map((o) => o.group ?? ""))];

  const pick = (v: string) => {
    onChange(v);
    setOpen(false);
  };

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <button
          id={id}
          type="button"
          role="combobox"
          aria-expanded={open}
          aria-label={ariaLabel}
          // cn 合并冲突的类：调用处给了 w-48 之类的宽度就盖过默认的 w-full
          className={cn(inputBase, "flex w-full items-center justify-between gap-2 text-left", className)}
        >
          <span className={cx("min-w-0 truncate", !current && !(allowEmpty && value === "") && "text-neutral-500")}>
            {current ? current.label : allowEmpty && value === "" && emptyLabel ? emptyLabel : (placeholder ?? t("pickers.choose"))}
          </span>
          <ChevronsUpDown size={14} className="shrink-0 text-neutral-400" />
        </button>
      </PopoverTrigger>
      <PopoverContent className="w-(--radix-popover-trigger-width) min-w-64 p-0" align="start">
        <Command
          filter={(itemValue, search) => {
            // 按标签、提示和值一起搜，中英文都不区分大小写
            const o = options.find((x) => x.value === itemValue);
            const hay = `${o?.label ?? ""} ${o?.hint ?? ""} ${itemValue}`.toLowerCase();
            return hay.includes(search.trim().toLowerCase()) ? 1 : 0;
          }}
        >
          <CommandInput placeholder={searchPlaceholder ?? t("pickers.search")} />
          <CommandList>
            <CommandEmpty>{t("pickers.noResults")}</CommandEmpty>
            {allowEmpty && (
              <CommandGroup>
                <CommandItem value="__empty__" onSelect={() => pick("")}>
                  <Check size={14} className={cx(value === "" ? "opacity-100" : "opacity-0")} />
                  {emptyLabel ?? t("pickers.none")}
                </CommandItem>
              </CommandGroup>
            )}
            {groups.map((g) => (
              <CommandGroup key={g} heading={g || undefined}>
                {options
                  .filter((o) => (o.group ?? "") === g)
                  .map((o) => (
                    <CommandItem key={o.value} value={o.value} onSelect={() => pick(o.value)}>
                      <Check size={14} className={cx("shrink-0", value === o.value ? "opacity-100" : "opacity-0")} />
                      <span className="min-w-0">
                        <span className="block truncate">{o.label}</span>
                        {o.hint && <span className="block truncate text-xs text-neutral-500">{o.hint}</span>}
                      </span>
                    </CommandItem>
                  ))}
              </CommandGroup>
            ))}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}

const pad = (n: number) => String(n).padStart(2, "0");
/** Date → 本地日期 "YYYY-MM-DD"（与原生日期输入框的取值格式一致）。 */
const toYmd = (d: Date) => `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
const fromYmd = (s: string) => {
  const [y, m, d] = s.split("-").map(Number);
  return y && m && d ? new Date(y, m - 1, d) : undefined;
};

/**
 * 日期选择。value / onChange 用本地日期字符串 "YYYY-MM-DD"，空字符串表示未选。
 * futureOnly 时不能选今天之前的日期（过期时间这类字段）。
 */
export function DatePicker({
  value,
  onChange,
  placeholder,
  futureOnly = false,
  clearable = true,
  id,
  "aria-label": ariaLabel,
}: {
  value: string;
  onChange: (v: string) => void;
  placeholder?: string;
  futureOnly?: boolean;
  clearable?: boolean;
  id?: string;
  "aria-label"?: string;
}) {
  const { t } = useTranslation();
  const calendarLocale = useCalendarLocale();
  const [open, setOpen] = useState(false);
  const selected = value ? fromYmd(value) : undefined;
  const today = new Date();
  today.setHours(0, 0, 0, 0);
  const label = selected ? formatDateOnly(selected, undefined, { year: "numeric", month: "long", day: "numeric" }) : null;

  return (
    <div className="flex items-center gap-1">
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger asChild>
          <button
            id={id}
            type="button"
            aria-label={ariaLabel}
            className={cx(inputBase, "flex w-full items-center gap-2 text-left", !label && "text-neutral-500")}
          >
            <CalendarIcon size={14} className="shrink-0 text-neutral-500" />
            <span className="truncate">{label ?? placeholder ?? t("pickers.pickDate")}</span>
          </button>
        </PopoverTrigger>
        <PopoverContent className="w-auto p-0" align="start">
          <Calendar
            mode="single"
            locale={calendarLocale}
            selected={selected}
            defaultMonth={selected}
            disabled={futureOnly ? { before: today } : undefined}
            onSelect={(d) => {
              onChange(d ? toYmd(d) : "");
              setOpen(false);
            }}
          />
        </PopoverContent>
      </Popover>
      {clearable && value && (
        <button
          type="button"
          aria-label={t("pickers.clear")}
          title={t("pickers.clear")}
          onClick={() => onChange("")}
          className="rounded-md p-2 text-neutral-500 transition-colors hover:bg-neutral-100 hover:text-neutral-900"
        >
          <X size={14} />
        </button>
      )}
    </div>
  );
}
