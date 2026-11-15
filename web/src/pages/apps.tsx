import { useState } from "react";
import { Link, useNavigate } from "react-router";
import { Copy, Plus, LayoutGrid, GraduationCap, Star } from "lucide-react";
import { useTranslation } from "react-i18next";
import { copyText } from "../lib/clipboard";
import { api, isExpert, type HubAsset } from "../lib/api";
import { useAuth } from "../lib/auth";
import { useStore } from "../lib/store";
import { useAction, useApi } from "../lib/use-api";
import { assetPath } from "../lib/hub";
import { pluginPath } from "../components/plugin-card";
import { LoadState } from "../components/api-ui";
import { useMenus } from "../lib/menus";
import { Badge, Button, Card, cx, PageHeader, Segmented, cardHover, Empty } from "../components/ui";

type Filter = "all" | "enabled";
type Form = "all" | "expert" | "app";

/** 微应用图标：有 emoji 居中显示，没有就用标题首字；底板和插件卡片同一套圆角浅色。 */
function AppIcon({ icon, title }: { icon?: string; title: string }) {
  const letter = title.trim().charAt(0).toUpperCase() || "?";
  return (
    <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-brand-50 text-brand-600 dark:bg-brand-400/15 dark:text-brand-200">
      {icon ? <span className="text-xl leading-none">{icon}</span> : <span className="text-base font-semibold">{letter}</span>}
    </span>
  );
}

function AppCard({ a, bySlug, busy, onToggle }: { a: HubAsset; bySlug: Map<string, HubAsset>; busy: boolean; onToggle: (a: HubAsset) => void }) {
  const { t } = useTranslation();
  const { plugins, notify } = useStore();
  const { can } = useAuth();
  const deps = [
    ...(a.spec.skills ?? []).map((s) => ({ key: "s:" + s, label: bySlug.get(s)?.spec.title ?? s, to: bySlug.get(s) ? assetPath(bySlug.get(s)!.id) : null, kind: t("assetKinds.skill") })),
    ...(a.spec.connections ?? []).map((s) => ({ key: "c:" + s, label: bySlug.get(s)?.spec.title ?? s, to: bySlug.get(s) ? assetPath(bySlug.get(s)!.id) : null, kind: t("assetKinds.connection") })),
    // 插件只在管理和「造」的高级选项里出现
    ...(can("plugins.manage") ? (a.spec.plugins ?? []) : []).map((p) => ({ key: "p:" + p, label: plugins.find((x) => x.name === p)?.title ?? p, to: pluginPath(p), kind: t("hub.tabs.plugin") })),
  ];
  return (
    <Card className={cx("flex h-full flex-col p-4", cardHover)}>
      <div className="flex gap-3">
        <AppIcon icon={a.spec.icon} title={a.spec.title} />
        <div className="min-w-0">
          <Link to={assetPath(a.id)} className="block truncate font-medium text-neutral-900 hover:text-brand-600 dark:hover:text-brand-200">
            {a.spec.title}
          </Link>
          <div className="truncate font-mono text-xs text-neutral-500">/{a.slug}</div>
        </div>
      </div>
      <div className="mt-3 flex flex-wrap items-center gap-1.5">
        {isExpert(a) && (
          <Badge tone="brand">
            <GraduationCap size={12} /> {t("expertCenter.formExpert")}
          </Badge>
        )}
        {a.featured && (
          <Badge tone="amber">
            <Star size={12} /> {t("expertCenter.featured")}
          </Badge>
        )}
        {a.enabled && <Badge tone="green">{t("hub.enabledBadge")}</Badge>}
        {a.delivery === "auto" && <Badge tone="brand">{t("hub.autoBadge")}</Badge>}
        {a.category && <Badge tone="slate">{a.category}</Badge>}
        {(a.spec.members?.length ?? 0) > 0 && <Badge tone="brand">{t("expertCenter.team", { count: a.spec.members!.length + 1 })}</Badge>}
      </div>
      {isExpert(a) && (a.spec.expertise?.length ?? 0) > 0 && (
        <div className="mt-2 flex flex-wrap gap-1">
          {a.spec.expertise!.map((x) => (
            <span key={x} className="rounded-[5px] bg-brand-50 px-1.5 py-0.5 text-xs text-brand-700 dark:bg-brand-400/15 dark:text-brand-200">
              {x}
            </span>
          ))}
        </div>
      )}
      <p className="mt-2 line-clamp-2 text-sm text-neutral-700">{a.spec.description}</p>
      {(a.spec.starters?.length ?? 0) > 0 && (
        <div className="mt-2 line-clamp-1 text-xs text-neutral-600">
          {isExpert(a) ? t("expertCenter.examples") : t("apps.starters")}：{a.spec.starters!.map((q) => `「${q}」`).join(" ")}
        </div>
      )}
      {deps.length > 0 && (
        <div className="mt-3 flex flex-wrap items-center gap-1.5">
          <span className="text-xs text-neutral-600">{t("apps.dependsOn")}</span>
          {deps.map((d) =>
            d.to ? (
              <Link
                key={d.key}
                to={d.to}
                className="max-w-full truncate rounded-[5px] bg-neutral-100 px-1.5 py-0.5 text-xs text-neutral-700 transition-colors hover:bg-brand-50 hover:text-brand-600 dark:hover:bg-brand-400/15 dark:hover:text-brand-200"
              >
                {d.label}
              </Link>
            ) : (
              <span key={d.key} className="max-w-full truncate rounded-[5px] bg-neutral-100 px-1.5 py-0.5 text-xs text-neutral-500">
                {d.label}
              </span>
            ),
          )}
        </div>
      )}
      {/* 垫片：内容再满，底部信息栏上方也留出一档间距 */}
      <div aria-hidden className="min-h-3 flex-1" />
      <div className="flex items-center justify-between gap-2 border-t border-neutral-100 pt-3 text-xs text-neutral-600">
        <span className="min-w-0 truncate">{t("apps.meta", { owner: a.owner_name || a.owner || "—", department: a.department || "—", uses: a.uses })}</span>
        <div className="flex shrink-0 items-center gap-2">
          {a.spec.web && a.available && (
            <Link to={`/a/${a.slug}`} className="font-medium text-brand-600 dark:text-brand-300 hover:underline">{t("appWeb.short")}</Link>
          )}
          {a.available && a.delivery === "optional" && (
            <Button size="sm" variant={a.subscribed ? "secondary" : "primary"} disabled={busy} onClick={() => onToggle(a)}>
              {a.subscribed ? t("hub.disable") : t("hub.enable")}
            </Button>
          )}
          <button
            type="button"
            title={t("apps.copyCommand", { slug: a.slug })}
            onClick={async () => {
              if (!(await copyText(`/${a.slug}`))) return notify(t("common.copyFailed"));
              notify(t("apps.copied", { slug: a.slug }));
            }}
            className="rounded-md p-1.5 text-neutral-500 transition-colors hover:bg-neutral-100 hover:text-neutral-900"
          >
            <Copy size={14} />
          </button>
        </div>
      </div>
    </Card>
  );
}

export default function Apps() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { can, user } = useAuth();
  const { notify } = useStore();
  const { pathOf } = useMenus();
  const hub = useApi(api.hub);
  const { busy, run } = useAction();
  const [filter, setFilter] = useState<Filter>("all");
  const [form, setForm] = useState<Form>("all");
  const all = hub.data?.assets ?? [];
  const bySlug = new Map(all.map((a) => [a.slug, a]));
  // 「企业应用」只列我能用的和我自己做的；管理员看全部在「能力管理」
  const apps = all.filter((a) => a.kind === "app" && (a.available || a.owner_id === user?.id));
  const experts = apps.filter(isExpert).length;
  // 精选排在最前，其余保持后端顺序
  const list = apps
    .filter((a) => (filter === "all" || a.enabled) && (form === "all" || (form === "expert") === isExpert(a)))
    .sort((a, b) => Number(b.featured) - Number(a.featured));

  const toggle = async (a: HubAsset) => {
    const r = await run(() => (a.subscribed ? api.unsubscribe(a.id) : api.subscribe(a.id)));
    notify(r.ok ? t(a.subscribed ? "hub.disabledMsg" : "hub.enabledMsg", { title: a.spec.title }) : r.error);
    if (r.ok) hub.reload();
  };

  return (
    <div>
      <PageHeader
        title={t("apps.title")}
        actions={
          <>
            {experts > 0 && (
              <Segmented
                value={form}
                onChange={setForm}
                options={[
                  { value: "all", label: t("expertCenter.formAll") },
                  { value: "expert", label: t("expertCenter.formExperts", { count: experts }) },
                  { value: "app", label: t("expertCenter.formApps", { count: apps.length - experts }) },
                ]}
              />
            )}
            <Segmented
              value={filter}
              onChange={setFilter}
              options={[
                { value: "all", label: t("apps.filterAll", { count: apps.length }) },
                { value: "enabled", label: t("apps.filterEnabled") },
              ]}
            />
            {can("registry.publish") && (
              <>
                <Button variant="secondary" onClick={() => navigate(`${pathOf("app-new")}?form=expert`)}>
                  <GraduationCap size={15} /> {t("expertCenter.createExpert")}
                </Button>
                <Button onClick={() => navigate(pathOf("app-new"))}>
                  <Plus size={15} /> {t("apps.create")}
                </Button>
              </>
            )}
          </>
        }
      />
      <LoadState loading={hub.loading && !hub.data} error={hub.data ? null : hub.error} onRetry={hub.reload}>
        {list.length === 0 ? (
          <Card>
            <Empty icon={LayoutGrid}>{filter === "all" ? t("apps.empty") : t("apps.emptyEnabled")}</Empty>
          </Card>
        ) : (
          <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
            {list.map((a) => (
              <AppCard key={a.id} a={a} bySlug={bySlug} busy={busy} onToggle={toggle} />
            ))}
          </div>
        )}
      </LoadState>
    </div>
  );
}
