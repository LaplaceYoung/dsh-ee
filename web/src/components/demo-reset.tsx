import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { formatTimeOnly } from "@/lib/format";

/** 演示站距下次数据重置还剩多久，例如「下次重置：还剩 2 小时 15 分（18:00）」。每 30 秒刷新一次。 */
export function DemoResetCountdown({ at }: { at: string }) {
  const { t } = useTranslation();
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const id = window.setInterval(() => setNow(Date.now()), 30_000);
    return () => window.clearInterval(id);
  }, []);
  const target = new Date(at);
  if (Number.isNaN(target.getTime())) return null;
  const mins = Math.ceil((target.getTime() - now) / 60_000);
  if (mins <= 1) return <>{t("shell.demoNext.soon")}</>;
  const h = Math.floor(mins / 60);
  const m = mins % 60;
  const left = h > 0 ? t("shell.demoNext.hm", { h, m }) : t("shell.demoNext.m", { m });
  const time = formatTimeOnly(target);
  return <>{t("shell.demoNext.left", { left, time })}</>;
}
