import { useState } from "react";
import { Link, useNavigate } from "react-router";
import { Copy, Plus, LayoutGrid } from "lucide-react";
import { useTranslation } from "react-i18next";
import { copyText } from "../lib/clipboard";
import { api, type HubAsset, type Scope } from "../lib/api";
import { useAuth } from "../lib/auth";
import { useStore } from "../lib/store";
import { useApi } from "../lib/use-api";
import { assetPath, buildState, KIND_ICONS, scopeSummary, type BuildState } from "../lib/hub";
import { PageLink, useMenus } from "../lib/menus";
import { ImportAppButton } from "../components/app-import";
import { LoadState } from "../components/api-ui";
import { ScopeDialog, useScopeOptions } from "../components/scope-dialog";
import { Badge, Button, Card, PageHeader, Segmented, buttonCls, Empty } from "../components/ui";

const STATE_TONE: Record<BuildState, "green" | "brand" | "amber" | "red" | "slate"> = {
  self: "slate",
  shared: "green",
  sharing: "amber",
  pending: "amber",
  rejected: "red",
  unpublished: "red",
};

const STATE_LABEL = {
  self: "build.stateSelf",
  shared: "build.stateShared",
  sharing: "build.stateSharing",
  pending: "build.statePending",
  rejected: "build.stateRejected",
  unpublished: "build.stateUnpublished",
} as const;

export function BuildStateBadge({ a }: { a: HubAsset }) {
  const { t } = useTranslation();
  const { user } = useAuth();
  const s = buildState(a, user?.id);
  return <Badge tone={STATE_TONE[s]}>{t(STATE_LABEL[s])}</Badge>;
}

function Row({ a, roleName, onShare }: { a: HubAsset; roleName: (id: number) => string; onShare: (a: HubAsset) => void }) {
  const { t } = useTranslation();
  const { notify } = useStore();
  const navigate = useNavigate();
  const Icon = KIND_ICONS[a.kind];
  return (
    <li className="flex flex-wrap items-center gap-x-4 gap-y-3 border-t border-neutral-100 px-5 py-4 first:border-t-0">
      <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-brand-50 text-brand-600 dark:bg-brand-400/15 dark:text-brand-200">
        {a.kind === "app" && a.spec.icon ? <span className="text-xl leading-none">{a.spec.icon}</span> : <Icon size={18} />}
      </span>
      {/* basis：窄屏放不下时，右边的操作按钮整组换到下一行，而不是把标题挤成一列字 */}
      <div className="min-w-0 flex-1 basis-52">
        <div className="flex flex-wrap items-center gap-2">
          <Link to={assetPath(a.id)} className="font-medium text-neutral-900 hover:text-brand-600 dark:hover:text-brand-200">
            {a.spec.title}
          </Link>
          <span className="font-mono text-xs text-neutral-500">/{a.slug}</span>
          <BuildStateBadge a={a} />
          {a.live_version > 0 && a.pending > 0 && <Badge tone="amber">{t("build.stateNewVersion")}</Badge>}
        </div>
        <div className="mt-0.5 truncate text-sm text-neutral-600">{a.spec.description}</div>
        {a.pending_scope && <div className="mt-1 text-xs text-amber-700">{t("build.sharingHint", { scope: scopeSummary(t, a.pending_scope as Scope, roleName) })}</div>}
        {!a.pending_scope && a.scope_note && <div className="mt-1 text-xs text-red-700">{t("build.shareRejected", { note: a.scope_note })}</div>}
      </div>
      <span className="hidden shrink-0 text-sm text-neutral-600 tabular-nums md:block">{t("build.uses", { uses: a.uses, users: a.users })}</span>
      <div className="flex shrink-0 items-center gap-2 max-sm:pl-14">
        <Button size="sm" variant="secondary" onClick={() => navigate(`/assets/${a.id}/edit`)}>
          {t("build.edit")}
        </Button>
        <Button size="sm" variant="secondary" onClick={() => onShare(a)}>
          {t("build.share")}
        </Button>
        <button
          type="button"
          title={t("build.useInDsh", { slug: a.slug })}
          aria-label={t("apps.copyCommand", { slug: a.slug })}
          onClick={async () => {
            if (!(await copyText(`/${a.slug}`))) return notify(t("common.copyFailed"));
            notify(t("apps.copied", { slug: a.slug }));
          }}
          className="rounded-md p-1.5 text-neutral-500 transition-colors hover:bg-neutral-100 hover:text-neutral-900"
        >
          <Copy size={14} />
        </button>
      </div>
    </li>
  );
}

/** 「造 → 我的应用」：自己做的应用（以及技能、提示词）、它们的状态与分享。 */
export default function Build() {
  const { t } = useTranslation();
  const { user, can } = useAuth();
  const { notify } = useStore();
  const { pathOf } = useMenus();
  const navigate = useNavigate();
  const hub = useApi(api.hub);
  const opts = useScopeOptions();
  const [tab, setTab] = useState<"apps" | "others">("apps");
  const [sharing, setSharing] = useState<HubAsset | null>(null);
  const mine = (hub.data?.assets ?? []).filter((a) => a.owner_id === user?.id && a.kind !== "connection");
  const apps = mine.filter((a) => a.kind === "app");
  const others = mine.filter((a) => a.kind !== "app");
  const list = tab === "apps" ? apps : others;

  return (
    <div>
      <PageHeader
        title={t("build.title")}
        desc={t("build.desc")}
        actions={
          can("registry.publish") && (
            <>
              <ImportAppButton onDone={hub.reload} />
              <Button variant="secondary" onClick={() => navigate(pathOf("templates"))}>
                <Copy size={15} /> {t("build.fromTemplate")}
              </Button>
              <Button onClick={() => navigate(pathOf("app-new"))}>
                <Plus size={15} /> {t("build.newApp")}
              </Button>
            </>
          )
        }
      />
      <div className="mb-4">
        <Segmented
          value={tab}
          onChange={setTab}
          options={[
            { value: "apps", label: t("build.tabApps", { count: apps.length }) },
            { value: "others", label: t("build.tabOthers", { count: others.length }) },
          ]}
        />
      </div>
      <LoadState loading={hub.loading && !hub.data} error={hub.data ? null : hub.error} onRetry={hub.reload}>
        {list.length === 0 ? (
          <Card>
            <Empty
              icon={LayoutGrid}
              action={
                tab === "apps" &&
                can("registry.publish") && (
                  <>
                    <PageLink page="templates" className={buttonCls("secondary")}>
                      {t("build.fromTemplate")}
                    </PageLink>
                    <PageLink page="app-new" className={buttonCls("primary")}>
                      {t("build.newApp")}
                    </PageLink>
                  </>
                )
              }
            >
              <p>{tab === "apps" ? t("build.empty") : t("build.emptyOthers")}</p>
            </Empty>
          </Card>
        ) : (
          <Card>
            <ul>
              {list.map((a) => (
                <Row key={a.id} a={a} roleName={opts.roleName} onShare={setSharing} />
              ))}
            </ul>
          </Card>
        )}
      </LoadState>
      {sharing && (
        <ScopeDialog
          open
          onOpenChange={(v) => !v && setSharing(null)}
          title={t("build.shareTitle", { title: sharing.spec.title })}
          description={t("build.shareDesc")}
          scope={sharing.pending_scope ?? sharing.scope}
          delivery={sharing.delivery}
          canDelivery={sharing.can_manage}
          selfId={user?.id}
          labels={{ auto: t("asset.deliveryAuto"), optional: t("asset.deliveryOptional"), autoHint: t("editor.deliveryAutoHint"), optionalHint: t("editor.deliveryOptionalHint") }}
          onSave={async (scope, delivery) => {
            const res = await api.patchAsset(sharing.id, sharing.can_manage ? { scope, delivery, updated_at: sharing.updated_at } : { scope, updated_at: sharing.updated_at });
            notify(res.pending_scope ? t("build.shareSubmitted") : t("build.shareSaved"));
            hub.reload();
          }}
        />
      )}
    </div>
  );
}
