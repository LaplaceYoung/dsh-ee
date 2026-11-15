import { useState, type ButtonHTMLAttributes, type HTMLAttributes, type InputHTMLAttributes, type ReactNode } from "react";
import { Check, CircleAlert, CircleCheck, Copy, Eye, EyeOff, Inbox, Info, Loader2, Star, TriangleAlert, type LucideIcon } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Tip } from "@/components/tip";
import { copyText } from "@/lib/clipboard";
import { useStore } from "@/lib/store";
import { cn } from "@/lib/utils";
import { formatDateOnly, formatDecimal, formatNumber } from "@/lib/format";

export function cx(...xs: (string | false | null | undefined)[]) {
  return xs.filter(Boolean).join(" ");
}

type Variant = "primary" | "secondary" | "ghost" | "danger";
type Size = "sm" | "md" | "lg";

const buttonBase =
  "inline-flex shrink-0 items-center justify-center gap-1.5 rounded-md font-semibold whitespace-nowrap select-none transition-[background-color,border-color,color,box-shadow,transform] duration-150 active:translate-y-px disabled:cursor-not-allowed disabled:opacity-50 disabled:shadow-none [&_svg]:shrink-0";
const buttonSizes: Record<Size, string> = {
  sm: "h-8 px-3 text-[13px]",
  md: "h-9 px-4 text-sm",
  lg: "h-10 px-5 text-sm",
};
const buttonVariants: Record<Variant, string> = {
  primary: "bg-brand-500 text-white shadow-primary hover:bg-brand-600 active:bg-brand-600",
  secondary:
    "border border-neutral-200 bg-card text-neutral-800 shadow-control hover:border-neutral-300 hover:bg-neutral-50 hover:text-neutral-900 dark:border-white/10 dark:hover:border-white/20",
  ghost: "text-neutral-600 hover:bg-neutral-100 hover:text-neutral-900",
  danger:
    "border border-red-200 bg-card text-red-600 shadow-control hover:border-red-300 hover:bg-red-50 dark:border-red-500/30 dark:text-red-400 dark:hover:bg-red-500/10",
};

/** 按钮的类名：给 <Link> / <a> 这类不是 <button> 的元素套同一套按钮样式。 */
export function buttonCls(variant: Variant = "primary", size: Size = "md", className?: string) {
  // cn 会合并冲突的 Tailwind 类：调用处传的 h-7、rounded-full、w-full 能稳定盖过默认值
  return cn(buttonBase, buttonSizes[size], buttonVariants[variant], className);
}

export function Button({
  variant = "primary",
  size = "md",
  className,
  ...rest
}: ButtonHTMLAttributes<HTMLButtonElement> & { variant?: Variant; size?: Size }) {
  return <button className={buttonCls(variant, size, className)} {...rest} />;
}

const tones = {
  slate: "bg-neutral-100 text-neutral-700 ring-neutral-300/45",
  brand: "bg-brand-50 text-brand-700 ring-brand-200/80 dark:bg-brand-500/15 dark:text-brand-200 dark:ring-brand-400/25",
  green: "bg-emerald-50 text-emerald-700 ring-emerald-200/80 dark:bg-emerald-500/15 dark:text-emerald-300 dark:ring-emerald-400/25",
  amber: "bg-amber-50 text-amber-800 ring-amber-200/80 dark:bg-amber-500/15 dark:text-amber-300 dark:ring-amber-400/25",
  red: "bg-red-50 text-red-700 ring-red-200/80 dark:bg-red-500/15 dark:text-red-300 dark:ring-red-400/25",
};

export type Tone = keyof typeof tones;

export function Badge({ tone = "slate", children }: { tone?: Tone; children: ReactNode }) {
  return (
    <span
      className={cx(
        "inline-flex flex-none items-center gap-1 rounded-[5px] px-1.5 py-0.5 text-xs leading-4 font-medium whitespace-nowrap ring-1 ring-inset",
        tones[tone],
      )}
    >
      {children}
    </span>
  );
}

export function Card({ className, children, ...rest }: { className?: string; children: ReactNode } & Omit<HTMLAttributes<HTMLDivElement>, "className" | "children">) {
  return (
    <div {...rest} className={cn("rounded-lg border border-neutral-200/80 bg-card shadow-card dark:border-white/[0.07]", className)}>
      {children}
    </div>
  );
}

/** 详情页左上角的「← 返回某某」链接。 */
export const backLinkCls = "mb-4 inline-flex items-center gap-1 rounded text-[13px] font-medium text-neutral-500 transition-colors hover:text-neutral-900";

/** 可点的卡片（列表里的应用、知识库、插件）：悬停时抬起一点并加深边框。 */
export const cardHover =
  "transition-[box-shadow,border-color,transform] duration-200 hover:-translate-y-px hover:border-neutral-300 hover:shadow-card-lg dark:hover:border-white/15";

export function Toggle({
  checked,
  onChange,
  disabled,
}: {
  checked: boolean;
  onChange: (v: boolean) => void;
  disabled?: boolean;
}) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      disabled={disabled}
      onClick={() => onChange(!checked)}
      className={cx(
        "relative inline-flex h-6 w-11 shrink-0 items-center rounded-full shadow-[inset_0_1px_2px_rgba(10,37,64,0.12)] transition-colors duration-200 disabled:opacity-40",
        checked ? "bg-brand-500" : "bg-neutral-300",
      )}
    >
      <span
        className={cx(
          "inline-block h-5 w-5 rounded-full bg-white shadow-[0_1px_2px_rgba(10,37,64,0.25),0_0_0_0.5px_rgba(10,37,64,0.06)] transition-transform duration-200",
          checked ? "translate-x-[22px]" : "translate-x-0.5",
        )}
      />
    </button>
  );
}

/** 设置项：左边标题与说明，右边开关。 */
export function ToggleRow({
  label,
  desc,
  checked,
  onChange,
  disabled,
}: {
  label: string;
  desc?: string;
  checked: boolean;
  onChange: (v: boolean) => void;
  disabled?: boolean;
}) {
  return (
    <div className={cx("flex items-start justify-between gap-6", disabled && "opacity-50")}>
      <div className="min-w-0">
        <div className="text-sm font-medium text-neutral-800">{label}</div>
        {desc && <div className="mt-0.5 text-[13px] leading-5 text-neutral-500">{desc}</div>}
      </div>
      <Toggle checked={checked} onChange={onChange} disabled={disabled} />
    </div>
  );
}

export function Segmented<T extends string>({
  value,
  options,
  onChange,
}: {
  value: T;
  options: { value: T; label: string }[];
  onChange: (v: T) => void;
}) {
  return (
    <div className="inline-flex max-w-full flex-wrap rounded-lg bg-neutral-100 p-0.5 ring-1 ring-neutral-200/60 ring-inset">
      {options.map((o) => (
        <button
          key={o.value}
          type="button"
          aria-pressed={value === o.value}
          onClick={() => onChange(o.value)}
          className={cx(
            "rounded-md px-3 py-1 text-sm whitespace-nowrap transition-[background-color,color,box-shadow] duration-150",
            value === o.value ? "bg-card font-medium text-neutral-900 shadow-control ring-1 ring-neutral-200/70 dark:ring-white/10" : "text-neutral-600 hover:text-neutral-900",
          )}
        >
          {o.label}
        </button>
      ))}
    </div>
  );
}

/** 下划线页签：tablist 容器与单个 tab 的类名（role 与 aria-selected 由使用处写）。 */
export const tabListCls = "flex gap-5 overflow-x-auto border-b border-neutral-200";
export function tabCls(active: boolean) {
  return cx(
    "-mb-px inline-flex items-center gap-1.5 border-b-2 px-0.5 py-2.5 text-sm whitespace-nowrap transition-colors",
    active ? "border-brand-500 font-medium text-brand-600 dark:text-brand-300" : "border-transparent text-neutral-600 hover:border-neutral-300 hover:text-neutral-900",
  );
}

export function CopyBox({ text, label, wrap }: { text: string; label?: string; wrap?: boolean }) {
  const [done, setDone] = useState(false);
  const { t } = useTranslation();
  const { notify } = useStore();
  return (
    <div className="group on-dark relative rounded-lg bg-neutral-900 text-neutral-100 shadow-card ring-1 ring-white/5 ring-inset">
      {label && <div className="border-b border-white/10 px-4 py-1.5 text-xs text-neutral-400">{label}</div>}
      <pre className={cx("px-4 py-3 pr-12 font-mono text-[13px] leading-relaxed", wrap ? "whitespace-pre-wrap break-words" : "overflow-x-auto")}>{text}</pre>
      <Tip label={done ? t("common.copied") : t("common.copy")}>
      <button
        type="button"
        aria-label={t("common.copy")}
        onClick={async () => {
          if (!(await copyText(text))) return notify(t("common.copyFailed"));
          setDone(true);
          setTimeout(() => setDone(false), 1200);
        }}
        className={cx("absolute right-2 bottom-2 rounded-md p-1.5 transition-colors hover:bg-white/10 hover:text-white", done ? "text-emerald-400" : "text-neutral-400")}
      >
        {done ? <Check size={16} /> : <Copy size={16} />}
      </button>
      </Tip>
    </div>
  );
}

/** 转圈：按钮里、加载态里用。 */
export function Spinner({ size = 16, className }: { size?: number; className?: string }) {
  return <Loader2 size={size} aria-hidden className={cx("animate-spin", className)} />;
}

/** 占位骨架：加载时先画出大致的形状。 */
export function Skeleton({ className }: { className?: string }) {
  return <div aria-hidden className={cx("skeleton", className)} />;
}

/**
 * 空状态：一个圆形图标、一句说明、可选的下一步按钮。
 * 说明文字就是原来页面上的那句话（children），放在卡片里或直接放在页面上都可以。
 */
export function Empty({
  icon: Icon = Inbox,
  title,
  children,
  action,
  compact,
  className,
}: {
  icon?: LucideIcon;
  title?: ReactNode;
  children?: ReactNode;
  action?: ReactNode;
  /** 紧凑：放在小卡片或表格下面，上下留白减半。 */
  compact?: boolean;
  className?: string;
}) {
  return (
    <div className={cx("flex flex-col items-center justify-center px-6 text-center", compact ? "py-7" : "py-12", className)}>
      <span
        aria-hidden
        className={cx(
          "flex items-center justify-center rounded-full bg-neutral-100 text-neutral-400 ring-1 ring-neutral-200/70 ring-inset",
          compact ? "mb-2.5 h-9 w-9" : "mb-3.5 h-11 w-11",
        )}
      >
        <Icon size={compact ? 16 : 20} strokeWidth={1.6} />
      </span>
      {title && <div className="text-sm font-semibold text-neutral-900">{title}</div>}
      {children && <div className={cx("max-w-md text-sm leading-6 text-neutral-500", !!title && "mt-1")}>{children}</div>}
      {action && <div className="mt-4 flex flex-wrap justify-center gap-2">{action}</div>}
    </div>
  );
}

const noticeTones = {
  info: "border-brand-200/80 bg-brand-50/70 text-neutral-800 dark:border-brand-400/25 dark:bg-brand-500/10 [&>svg]:text-brand-500 dark:[&>svg]:text-brand-300",
  success: "border-emerald-200 bg-emerald-50 text-emerald-900 dark:border-emerald-400/25 dark:bg-emerald-500/10 dark:text-emerald-200 [&>svg]:text-emerald-600 dark:[&>svg]:text-emerald-300",
  warn: "border-amber-200 bg-amber-50 text-amber-900 dark:border-amber-400/25 dark:bg-amber-500/10 dark:text-amber-200 [&>svg]:text-amber-600 dark:[&>svg]:text-amber-300",
  danger: "border-red-200 bg-red-50 text-red-800 dark:border-red-400/25 dark:bg-red-500/10 dark:text-red-300 [&>svg]:text-red-600 dark:[&>svg]:text-red-300",
};
const noticeIcons = { info: Info, success: CircleCheck, warn: TriangleAlert, danger: CircleAlert };

/** 提示条：说明、成功、警告、出错四种语气，左侧带图标；内容可以是一句话，也可以带链接或按钮。 */
export function Notice({
  tone = "info",
  icon = true,
  className,
  children,
  ...rest
}: { tone?: keyof typeof noticeTones; icon?: boolean; className?: string; children: ReactNode } & Omit<HTMLAttributes<HTMLDivElement>, "className" | "children">) {
  const Icon = noticeIcons[tone];
  return (
    <div {...rest} className={cn("flex gap-2.5 rounded-md border px-3.5 py-2.5 text-sm leading-6", noticeTones[tone], className)}>
      {icon && <Icon size={16} aria-hidden className="mt-1 shrink-0" />}
      <div className="min-w-0 flex-1">{children}</div>
    </div>
  );
}

/** 内网通常访问不到 GitHub 头像，加载失败就退回首字母色块（正式版应由服务端镜像图标）。 */
export function PluginIcon({ name, icon, size = 40 }: { name: string; icon: string | null; size?: number }) {
  const [broken, setBroken] = useState(false);
  const letter = name.replace(/^@[^/]+\//, "").replace(/^dsh-?/i, "").charAt(0).toUpperCase() || "?";
  if (icon && !broken) {
    // 首字母垫在图片下面：远程图还没加载完时也不会是一个空白方块。
    return (
      <span className="relative inline-flex shrink-0" style={{ width: size, height: size }}>
        <span
          aria-hidden
          className="absolute inset-0 flex items-center justify-center rounded-md bg-neutral-100 font-semibold text-neutral-600"
          style={{ fontSize: size * 0.42 }}
        >
          {letter}
        </span>
        <img
          src={icon}
          alt=""
          width={size}
          height={size}
          onError={() => setBroken(true)}
          className="relative rounded-md object-cover"
          style={{ width: size, height: size }}
        />
      </span>
    );
  }
  return (
    <div
      className="flex shrink-0 items-center justify-center rounded-md bg-neutral-100 font-semibold text-neutral-600"
      style={{ width: size, height: size, fontSize: size * 0.42 }}
    >
      {letter}
    </div>
  );
}

export function ScoreBadge({ score }: { score: number }) {
  const { t } = useTranslation();
  const tone = score >= 80 ? "green" : score >= 60 ? "brand" : "amber";
  return <Badge tone={tone}>{t("plugin.score", { score })}</Badge>;
}

export function Rating({ value, count }: { value: number; count?: number }) {
  return (
    <span className="inline-flex items-center gap-0.5 text-xs text-amber-600">
      <Star size={12} className="fill-amber-400 text-amber-400" />
      {value.toFixed(1)}
      {count !== undefined && <span className="text-neutral-500">（{count}）</span>}
    </span>
  );
}

/** 单个指标：标签在上、数值在下；多个指标放进 StatStrip 共用一个容器，用细分隔线分栏。 */
export function Stat({ label, value, hint }: { label: string; value: ReactNode; hint?: ReactNode; icon?: ReactNode }) {
  return (
    <div className="min-w-0 px-5 py-4">
      <div className="truncate text-[13px] font-medium text-neutral-500">{label}</div>
      <div className="mt-1.5 text-[26px] leading-8 font-semibold tracking-[-0.02em] text-neutral-900 tabular-nums">{value}</div>
      {hint && <div className="mt-1 text-xs text-neutral-500">{hint}</div>}
    </div>
  );
}

export function StatStrip({ children, cols = 4 }: { children: ReactNode; cols?: 2 | 3 | 4 }) {
  const grid = { 2: "sm:grid-cols-2", 3: "sm:grid-cols-3", 4: "sm:grid-cols-2 xl:grid-cols-4" }[cols];
  return (
    <Card className={cx("grid divide-neutral-200/80 max-sm:divide-y sm:divide-x dark:divide-white/[0.07]", grid)}>{children}</Card>
  );
}

/** 卡片标题行：左侧标题，右侧可放时间范围或「查看全部」。 */
export function CardHeader({ title, aside }: { title: ReactNode; aside?: ReactNode }) {
  return (
    <div className="flex flex-wrap items-center justify-between gap-x-3 gap-y-1 px-5 pt-4 pb-3">
      <h2 className="text-[15px] leading-6 font-semibold text-neutral-900">{title}</h2>
      {aside && <div className="text-sm text-neutral-600">{aside}</div>}
    </div>
  );
}

/** 页面标题：大标题、一句说明，右侧放这一页的主操作；窄屏时操作换到标题下面。 */
export function PageHeader({ title, desc, actions }: { title: string; desc?: ReactNode; actions?: ReactNode }) {
  return (
    <div className="mb-6 flex flex-wrap items-start justify-between gap-x-6 gap-y-3">
      <div className="min-w-0 flex-1 basis-72">
        <h1 className="text-2xl leading-8 font-bold tracking-[-0.02em] text-neutral-900 sm:text-[28px] sm:leading-9">{title}</h1>
        {desc && <p className="mt-1.5 max-w-3xl text-sm leading-6 text-neutral-600">{desc}</p>}
      </div>
      {actions && <div className="flex flex-wrap items-center gap-2 sm:pt-0.5">{actions}</div>}
    </div>
  );
}

export function Field({ label, hint, children }: { label: string; hint?: ReactNode; children: ReactNode }) {
  return (
    <label className="block">
      <div className="mb-1.5 text-[13px] font-medium text-neutral-700">{label}</div>
      {children}
      {hint && <div className="mt-1.5 text-xs leading-5 text-neutral-500">{hint}</div>}
    </label>
  );
}

/** 不带宽度的输入框样式；需要定宽时用它，避免 w-full 与 w-40 这类类名互相覆盖。 */
export const inputBase =
  "h-9 rounded-md border border-neutral-300 bg-card px-3 text-sm text-neutral-900 shadow-control outline-none transition-[border-color,box-shadow] duration-150 placeholder:text-neutral-400 hover:border-neutral-400/70 focus:border-brand-500 focus:ring-[3px] focus:ring-brand-500/20 disabled:pointer-events-none disabled:cursor-not-allowed disabled:bg-neutral-50 disabled:text-neutral-500 disabled:shadow-none dark:border-white/15 dark:placeholder:text-neutral-500 dark:hover:border-white/25 dark:focus:border-brand-400";

export const inputCls = `${inputBase} w-full`;

export function formatKb(kb: number) {
  return kb >= 1024 ? `${formatDecimal(kb / 1024, 1)} MB` : `${formatNumber(kb)} KB`;
}

/** Unix 秒级时间戳 → 本地日期（按界面语言）。 */
export function formatDate(ts: number) {
  return formatDateOnly(new Date(ts * 1000), undefined, { year: "numeric", month: "2-digit", day: "2-digit" });
}

/**
 * 密码 / 密钥输入框：右侧一个眼睛图标，点击在明文与掩码之间切换。
 * 样式与普通 input 一致（传入 inputCls 或 inputBase），宽度由 containerClassName 控制
 * ——固定宽度的地方（例如 LDAP 试连的窄输入框）要一起传，否则图标会飘到右侧。
 */
export function PasswordInput({
  label,
  className,
  containerClassName,
  ...props
}: Omit<InputHTMLAttributes<HTMLInputElement>, "type"> & { label?: string; containerClassName?: string }) {
  const { t } = useTranslation();
  const [visible, setVisible] = useState(false);
  // 按钮的文案刻意不含「密码 / password」：否则它会被 getByLabel('密码') 之类的
  // 标签匹配当成第二个「密码」控件，读屏时也会和输入框撞名。
  const toggleLabel = visible ? t("common.hideSecret") : t("common.revealSecret");
  return (
    <div className={cx("relative", containerClassName ?? "w-full")}>
      <input {...props} aria-label={label} type={visible ? "text" : "password"} className={cx(className, "pr-10")} />
      <Tip label={toggleLabel} side="top">
        <button
          type="button"
          aria-label={toggleLabel}
          aria-pressed={visible}
          onClick={() => setVisible((v) => !v)}
          className="absolute top-1/2 right-1 flex h-7 w-7 -translate-y-1/2 items-center justify-center rounded-md text-neutral-500 transition-colors hover:bg-neutral-100 hover:text-neutral-900"
        >
          {visible ? <EyeOff size={15} aria-hidden /> : <Eye size={15} aria-hidden />}
        </button>
      </Tip>
    </div>
  );
}
