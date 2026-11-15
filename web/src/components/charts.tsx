import { useState } from "react";
import { formatDateOnly, formatNumber } from "@/lib/format";
import { cx } from "./ui";

/** 接口给的 YYYY-MM-DD → 轴上的「月/日」，顺序与分隔符跟界面语言（en 11/14、es 14/11）。 */
function dayLabel(date: string) {
  const [y, m, d] = date.split("-").map(Number);
  if (!y || !m || !d) return date;
  // 用本地日期构造，避免 new Date("YYYY-MM-DD") 按 UTC 解析后在东半球以外差一天。
  return formatDateOnly(new Date(y, m - 1, d), undefined, { month: "numeric", day: "numeric" });
}

/** 刻度取整到 max 之上；全为 0 时给一个最小刻度，柱子贴着基线。 */
function niceTop(max: number) {
  if (max <= 0) return 4;
  const step = 10 ** Math.floor(Math.log10(max));
  return Math.ceil(max / step) * step;
}

/**
 * 逐日单序列柱状图：一个色相、无图例，只标峰值与最后一天，悬停看逐日值。
 * 背后两条淡网格线，刻度取整到 max 之上。dates 与 values 一一对应（YYYY-MM-DD）。
 */
export function DailyBars({
  values,
  dates,
  unit,
  height = "h-56",
  format = (v: number) => formatNumber(v),
}: {
  values: number[];
  dates: string[];
  unit: string;
  height?: string;
  format?: (v: number) => string;
}) {
  const [hover, setHover] = useState<number | null>(null);
  const raw = Math.max(0, ...values);
  const top = niceTop(raw);
  const peak = raw > 0 ? values.indexOf(raw) : -1;
  const last = values.length - 1;
  const pct = (v: number) => `${(v / top) * 100}%`;
  return (
    <div className="pl-10">
      <div className="flex gap-3">
        <div className={cx("relative flex-1", height)}>
          {[0.5, 1].map((f) => (
            <div key={f} className="absolute inset-x-0 border-t border-dashed border-neutral-200" style={{ bottom: `${f * 100}%` }}>
              <span className="absolute -top-2.5 right-full mr-2 text-xs text-neutral-500 tabular-nums">{format(Math.round(top * f))}</span>
            </div>
          ))}
          <div className="absolute inset-0 flex items-end gap-[3px] border-b border-neutral-300">
            {values.map((v, i) => (
              <div
                key={dates[i] ?? i}
                className="relative flex h-full flex-1 items-end"
                onMouseEnter={() => setHover(i)}
                onMouseLeave={() => setHover(null)}
              >
                <div
                  className={cx("w-full rounded-t-[3px] transition-colors", hover === i ? "bg-brand-700" : "bg-brand-500")}
                  style={{ height: pct(v) }}
                />
                {(i === peak || (i === last && v > 0)) && hover === null && (
                  <span className="absolute left-1/2 -translate-x-1/2 text-xs text-neutral-700 tabular-nums" style={{ bottom: `calc(${pct(v)} + 2px)` }}>
                    {format(v)}
                  </span>
                )}
                {hover === i && (
                  <div
                    className="on-dark absolute left-1/2 z-10 -translate-x-1/2 -translate-y-full rounded bg-neutral-900 px-2 py-1 text-xs whitespace-nowrap text-white shadow-md"
                    style={{ bottom: `calc(${pct(v)} + 6px)` }}
                  >
                    {dates[i] && dayLabel(dates[i])} · {format(v)} {unit}
                  </div>
                )}
              </div>
            ))}
          </div>
        </div>
      </div>
      {dates.length > 0 && (
        <div className="mt-1.5 flex justify-between pl-0 text-xs text-neutral-500">
          <span>{dayLabel(dates[0])}</span>
          <span>{dayLabel(dates[dates.length - 1])}</span>
        </div>
      )}
    </div>
  );
}

/** 单值排行条：一个颜色、无底轨，数值紧跟条尾。 */
export function BarList({ rows, highlight }: { rows: { label: string; value: number; hint?: string }[]; highlight?: (label: string) => boolean }) {
  const max = Math.max(...rows.map((r) => r.value), 1);
  return (
    <ul className="space-y-2.5">
      {rows.map((r) => (
        <li key={r.label}>
          <div className="flex items-baseline justify-between text-sm">
            <span className="text-neutral-800">
              {r.label}
              {r.hint && <span className="ml-1.5 text-xs text-neutral-500">{r.hint}</span>}
            </span>
            <span className="text-neutral-900 tabular-nums">{formatNumber(r.value)}</span>
          </div>
          <div className="mt-1 h-1.5 rounded-full bg-neutral-100">
            <div
              className={cx("h-full rounded-full", highlight && !highlight(r.label) ? "bg-neutral-300" : "bg-brand-500")}
              style={{ width: `${(r.value / max) * 100}%` }}
            />
          </div>
        </li>
      ))}
    </ul>
  );
}
