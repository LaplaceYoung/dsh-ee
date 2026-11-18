import { useEffect, useMemo, useRef, type ReactNode } from "react";
import { Link } from "react-router";
import { useTranslation } from "react-i18next";
import { BookOpen, Cable, Database, Globe, LayoutDashboard, MessageSquare, Puzzle, Table2 } from "lucide-react";
import { api, type AppDep, type ConnectionTool, type HubAsset, type WorkbenchWidget } from "../lib/api";
import { useApi } from "../lib/use-api";
import { assetPath } from "../lib/hub";
import { pluginPath } from "./plugin-card";
import { Badge, Card, cx } from "./ui";

type ToolInfo = { tools: ConnectionTool[] | null; id?: number };

const statusTone: Record<AppDep["status"], "green" | "slate" | "amber" | "red"> = { ok: "green", off: "slate", partial: "amber", missing: "red", gone: "red" };

/** 工作台里所有组件，连同所在窗格的标题。 */
function allWidgets(a: HubAsset): { pane: string; w: WorkbenchWidget }[] {
  return (a.spec.workbench?.panes ?? []).flatMap((p) => (p.widgets ?? []).map((w) => ({ pane: p.title, w })));
}

/** 组件写入了哪张自带表（表单提交、快捷按钮）。 */
function writesTable(w: WorkbenchWidget): string | undefined {
  return w.submit?.table ?? w.actions?.find((x) => x.table)?.table;
}

/** 读出依赖连接各自的接口：按当前员工的权限，看不到的记为 null。 */
function useConnectionTools(a: HubAsset, deps: AppDep[] | undefined, bySlug: Map<string, HubAsset>) {
  const slugs = a.spec.connections ?? [];
  const key = slugs.join(",");
  const q = useApi<Record<string, ToolInfo>>(async () => {
      const out: Record<string, ToolInfo> = {};
      await Promise.all(
        slugs.map(async (slug) => {
          const id = deps?.find((d) => d.kind === "connection" && d.slug === slug)?.id ?? bySlug.get(slug)?.id;
          if (!id) return void (out[slug] = { tools: null });
          try {
            out[slug] = { id, tools: (await api.hubAsset(id)).tools ?? [] };
          } catch {
            out[slug] = { id, tools: null };
          }
        }),
      );
      return out;
  });
  // 能力列表晚一步到时才知道连接的 id，到了再读一次
  const { reload } = q;
  const first = useRef(true);
  useEffect(() => {
    if (first.current) return void (first.current = false);
    reload();
  }, [key, bySlug.size, reload]);
  return q;
}

function GroupLabel({ icon, children }: { icon: ReactNode; children: ReactNode }) {
  return (
    <div className="mb-2 flex items-center gap-1.5 text-[11px] font-semibold tracking-wide text-[var(--hub-label)] uppercase">
      {icon} {children}
    </div>
  );
}

/** 落地页枢纽图风格的小瓦片：彩色图标块 + 标题 + 类型，可挂小字内容。 */
function Tile({ icon, bg, kind, title, to, aside, children }: { icon: ReactNode; bg: string; kind: string; title: ReactNode; to?: string | null; aside?: ReactNode; children?: ReactNode }) {
  const body = (
    <>
      <span className="flex items-center gap-2.5">
        <span className={cx("flex h-7 w-7 shrink-0 items-center justify-center rounded-md text-white", bg)}>{icon}</span>
        <span className="min-w-0 flex-1">
          <span className="block truncate text-[13px] font-semibold text-[var(--hub-chip-ink)]">{title}</span>
          <span className="block truncate text-[11px] text-[var(--hub-label)]">{kind}</span>
        </span>
        {aside}
      </span>
      {children}
    </>
  );
  const cls = cx(
    "block w-full rounded-lg bg-[var(--hub-tile)] px-2.5 py-2 text-left ring-1 ring-[var(--hub-tile-ring)] transition",
    to && "hover:-translate-y-0.5 hover:bg-[var(--hub-tile-hover)] hover:shadow-[0_10px_24px_-10px_rgba(50,50,93,0.35)] hover:ring-[var(--hub-line)]",
  );
  return to ? <Link to={to} className={cls}>{body}</Link> : <div className={cls}>{body}</div>;
}

/** 两组之间下行的流动虚线：dash-flow 滚动，中点压一个品牌色小标签。 */
function Wire({ label }: { label: string }) {
  return (
    <div className="relative my-0.5 flex justify-center" aria-hidden>
      <svg width="16" height="52" viewBox="0 0 16 52" fill="none">
        <path d="M8 2v38" stroke="var(--hub-line)" strokeWidth="1.5" strokeLinecap="round" className="dash-flow" />
        <path d="M3 38l5 8 5-8" stroke="var(--hub-line)" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
      </svg>
      <span className="absolute top-1/2 -translate-y-1/2 rounded-md bg-[#4d6bfe] px-2 py-0.5 text-[10px] font-medium whitespace-nowrap text-white shadow-[0_6px_16px_-6px_rgba(77,107,254,0.6)]">
        {label}
      </span>
    </div>
  );
}

const emptyBox = "rounded-lg border border-dashed border-[var(--hub-placeholder)] px-3 py-2.5 text-xs text-[var(--hub-label)]";

/**
 * 应用详情里的「应用全景」：落地页枢纽图风格——左边数据来源（企业系统及其接口、知识库、
 * 中台数据集、自带表），中间应用本身（人设、技能、插件），右边提供什么（对话入口、工作台、网页版），
 * 下面是能操作哪些数据的清单。配色读 --hub-*（.lp 作用域），连线用 dash-flow。
 */
export function AppMap({ a, deps, bySlug }: { a: HubAsset; deps?: AppDep[]; bySlug: Map<string, HubAsset> }) {
  const { t } = useTranslation();
  const tools = useConnectionTools(a, deps, bySlug);
  const widgets = useMemo(() => allWidgets(a), [a]);
  const dep = (kind: AppDep["kind"], slug: string) => deps?.find((d) => d.kind === kind && d.slug === slug);
  const statusBadge = (d?: AppDep) => d && d.status !== "ok" ? <Badge tone={statusTone[d.status]}>{t(`appMap.status.${d.status}`)}</Badge> : null;

  // 工作台直接调用了哪些接口：connection/tool
  const usedTools = new Set(widgets.flatMap(({ w }) => (w.source?.connection && w.source.tool ? [`${w.source.connection}/${w.source.tool}`] : [])));
  const datasets = [...new Set(widgets.flatMap(({ w }) => (w.source?.dataset ? [w.source.dataset] : [])))];
  const conns = a.spec.connections ?? [];
  const kbs = a.spec.knowledge ?? [];
  const tables = a.spec.tables ?? [];
  const skills = a.spec.skills ?? [];
  const plugins = a.spec.plugins ?? [];
  const toolCount = conns.reduce((n, c) => n + (tools.data?.[c]?.tools?.length ?? 0), 0);
  const visibility = (v?: string) => t(`appData.visibility.${(v ?? "own") as "own"}`);

  // 能操作哪些数据
  type Op = { key: string; target: ReactNode; where: string; can: string[]; how: string[]; write?: boolean };
  const ops: Op[] = [];
  for (const c of conns) {
    const list = tools.data?.[c]?.tools;
    const title = bySlug.get(c)?.spec.title ?? dep("connection", c)?.title ?? c;
    for (const tool of list ?? []) {
      if (!tool.enabled) continue;
      const inWb = usedTools.has(`${c}/${tool.name}`);
      ops.push({
        key: `${c}/${tool.name}`,
        target: <span className="font-mono text-xs">{tool.name}</span>,
        where: title,
        can: [tool.read_only ? t("appMap.can.query") : t("appMap.can.write")],
        how: tool.read_only ? [t("appMap.how.chat"), ...(inWb ? [t("appMap.how.workbench")] : [])] : [t("appMap.how.chatApprove")],
        write: !tool.read_only,
      });
    }
  }
  for (const k of kbs) {
    ops.push({ key: "kb:" + k, target: dep("knowledge", k)?.title ?? k, where: t("appMap.knowledge"), can: [t("appMap.can.kbRead")], how: [t("appMap.how.chat")] });
  }
  for (const ds of datasets) {
    ops.push({ key: "ds:" + ds, target: <span className="font-mono text-xs">{ds}</span>, where: t("appMap.dataset"), can: [t("appMap.can.datasetRead")], how: [t("appMap.how.workbench")] });
  }
  for (const tb of tables) {
    const hasForm = widgets.some(({ w }) => writesTable(w) === tb.name);
    ops.push({
      key: "tb:" + tb.name,
      target: tb.title,
      where: t("appMap.ownTable"),
      can: [t("appMap.can.tableRead", { visibility: visibility(tb.visibility) }), t("appMap.can.tableInsert"), t("appMap.can.tableEdit")],
      how: [t("appMap.how.chat"), ...(hasForm ? [t("appMap.how.form")] : []), ...(a.spec.web ? [t("appMap.how.web")] : [])],
      write: true,
    });
  }

  const hasSources = conns.length + kbs.length + datasets.length + tables.length > 0;

  return (
    <Card className="p-5" data-app-map>
      <div className="mb-4">
        <h2 className="font-medium text-neutral-900">{t("appMap.title")}</h2>
        <p className="mt-1 text-sm text-neutral-600">{t("appMap.desc")}</p>
        <div className="mt-3 flex flex-wrap gap-1.5">
          <Badge>{t("appMap.summary.systems", { count: conns.length })}</Badge>
          {conns.length > 0 && <Badge>{t("appMap.summary.tools", { count: toolCount })}</Badge>}
          <Badge>{t("appMap.summary.kb", { count: kbs.length })}</Badge>
          <Badge>{t("appMap.summary.tables", { count: tables.length })}</Badge>
          <Badge>{t("appMap.summary.widgets", { count: widgets.length })}</Badge>
        </div>
      </div>

      {/* 调用链路：落地页枢纽图风格——点阵底 + 光晕，左右瓦片经流动虚线汇入中间的渐变应用块 */}
      <div
        className="lp relative overflow-hidden rounded-xl ring-1 ring-[var(--hub-panel-ring)]"
        style={{
          background:
            "radial-gradient(60% 50% at 50% 75%, var(--hub-glow1), transparent 70%), radial-gradient(45% 40% at 85% 10%, rgb(14 165 233 / 0.10), transparent 70%), var(--hub-base)",
        }}
      >
        <div aria-hidden className="lp-hero-dots pointer-events-none absolute inset-0" />
        <div className="relative p-4 sm:p-5">
          {/* 数据从哪来 */}
          <div className="min-w-0">
            <GroupLabel icon={<Database size={12} />}>{t("appMap.sources")}</GroupLabel>
            <div className="grid items-start gap-2" style={{ gridTemplateColumns: "repeat(auto-fill, minmax(190px, 1fr))" }}>
              {!hasSources && <p className={emptyBox}>{t("appMap.noSources")}</p>}
              {conns.map((c) => {
                const d = dep("connection", c);
                const x = bySlug.get(c);
                const info = tools.data?.[c];
                const list = (info?.tools ?? []).filter((tool) => tool.enabled);
                const shown = list.slice(0, 4);
                return (
                  <Tile key={c} icon={<Cable size={14} />} bg="bg-[#0ea5e9]" kind={t("appMap.system")} title={x?.spec.title ?? d?.title ?? c} to={x ? assetPath(x.id) : d?.id ? assetPath(d.id) : null} aside={statusBadge(d)}>
                    {tools.loading && !tools.data ? (
                      <p className="mt-1.5 pl-[38px] text-[11px] text-[var(--hub-label)]">{t("appMap.loading")}</p>
                    ) : info?.tools == null ? (
                      <p className="mt-1.5 pl-[38px] text-[11px] text-[var(--hub-label)]">{t("appMap.toolsHidden")}</p>
                    ) : list.length === 0 ? (
                      <p className="mt-1.5 pl-[38px] text-[11px] text-[var(--hub-label)]">{t("appMap.noTools")}</p>
                    ) : (
                      <ul className="mt-1.5 flex flex-wrap gap-1 pl-[38px]">
                        {shown.map((tool) => (
                          <li key={tool.name} className="inline-flex items-center gap-1 rounded bg-[var(--hub-chip)] px-1.5 py-0.5" title={tool.description}>
                            <span className={cx("h-1.5 w-1.5 shrink-0 rounded-full", tool.read_only ? "bg-emerald-500" : "bg-amber-500")} />
                            <span className="font-mono text-[10px] text-[var(--hub-chip-ink)]">{tool.name}</span>
                            <span className="text-[9px] text-[var(--hub-label)]">{tool.read_only ? t("appMap.read") : t("appMap.write")}</span>
                          </li>
                        ))}
                        {list.length > shown.length && (
                          <li className="inline-flex items-center px-1 text-[10px] text-[var(--hub-label)]">+{list.length - shown.length}</li>
                        )}
                      </ul>
                    )}
                  </Tile>
                );
              })}
              {kbs.map((k) => {
                const d = dep("knowledge", k);
                return <Tile key={k} icon={<BookOpen size={14} />} bg="bg-[#10b981]" kind={t("appMap.knowledge")} title={d?.title ?? k} to={d?.id ? `/knowledge/${d.id}` : null} aside={statusBadge(d)} />;
              })}
              {datasets.map((ds) => (
                <Tile key={ds} icon={<Database size={14} />} bg="bg-[#4d6bfe]" kind={t("appMap.dataset")} title={<span className="font-mono">{ds}</span>} />
              ))}
              {tables.map((tb) => (
                <Tile key={tb.name} icon={<Table2 size={14} />} bg="bg-[#f97316]" kind={t("appMap.ownTable")} title={tb.title} aside={<Badge>{visibility(tb.visibility)}</Badge>}>
                  <div className="mt-1.5 flex flex-wrap gap-1 pl-[38px]">
                    {tb.fields.filter((f) => !f.hidden).map((f) => (
                      <span key={f.name} className="rounded bg-[var(--hub-chip)] px-1.5 py-0.5 text-[10px] text-[var(--hub-chip-ink)]">{f.title}</span>
                    ))}
                  </div>
                </Tile>
              ))}
            </div>
          </div>

          <Wire label={t("appMap.wireIn")} />

          {/* 应用本身：枢纽图中间的渐变方块，限宽居中 */}
          <div className="mx-auto w-full max-w-[340px] min-w-0">
            <div className="scene-card relative rounded-2xl bg-[#4d6bfe] p-4 text-white shadow-[0_18px_36px_-16px_rgba(45,66,200,0.55)] ring-1 ring-white/20" style={{ "--d": "150ms" } as React.CSSProperties}>
              <div className="flex items-center gap-2.5">
                <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-white text-[#3b5bdb]">
                  {a.spec.icon ? <span className="text-lg leading-none">{a.spec.icon}</span> : <Puzzle size={18} />}
                </span>
                <div className="min-w-0">
                  <div className="truncate text-[15px] font-bold tracking-tight">{a.spec.title}</div>
                  <div className="truncate font-mono text-[11px] text-white/75">/{a.slug}</div>
                </div>
              </div>
              <p className="mt-2 line-clamp-3 text-[11px] leading-5 text-white/80">{(a.spec.persona ?? "").replace(/[#*_`]/g, "").trim() || t("appMap.noPersona")}</p>
            </div>
            <div className="mt-2 flex flex-col gap-2">
              {skills.map((s) => {
                const x = bySlug.get(s);
                const d = dep("skill", s);
                return <Tile key={s} icon={<BookOpen size={14} />} bg="bg-[#4d6bfe]" kind={t("appMap.skill")} title={x?.spec.title ?? d?.title ?? s} to={x ? assetPath(x.id) : null} aside={statusBadge(d)} />;
              })}
              {plugins.map((p) => (
                <Tile key={p} icon={<Puzzle size={14} />} bg="bg-[#10b981]" kind={t("appMap.plugin")} title={<span className="font-mono">{p}</span>} to={pluginPath(p)} aside={statusBadge(dep("plugin", p))} />
              ))}
            </div>
          </div>

          <Wire label={t("appMap.wireOut")} />

          {/* 提供什么 */}
          <div className="min-w-0">
            <GroupLabel icon={<LayoutDashboard size={12} />}>{t("appMap.provides")}</GroupLabel>
            <div className="grid items-start gap-2" style={{ gridTemplateColumns: "repeat(auto-fill, minmax(190px, 1fr))" }}>
              <Tile
                icon={<MessageSquare size={14} />}
                bg="bg-[#4d6bfe]"
                kind={t("appMap.chat")}
                title={t("appMap.chatDesc", { slug: a.slug })}
                aside={(a.spec.starters?.length ?? 0) > 0 ? <span className="shrink-0 text-[10px] text-[var(--hub-label)]">{t("appMap.starters", { count: a.spec.starters!.length })}</span> : undefined}
              />
              {(a.spec.workbench?.panes ?? []).map((p) => {
                const counts = new Map<WorkbenchWidget["type"], number>();
                const writes = new Set<string>();
                for (const w of p.widgets ?? []) {
                  if (w.type === "heading" || w.type === "markdown") continue;
                  counts.set(w.type, (counts.get(w.type) ?? 0) + 1);
                  const wt = writesTable(w);
                  if (wt) writes.add(tables.find((x) => x.name === wt)?.title ?? wt);
                }
                return (
                  <Tile key={p.id} icon={<LayoutDashboard size={14} />} bg="bg-[#0ea5e9]" kind={t("appMap.workbench")} title={t("appMap.pane", { title: p.title })}>
                    <div className="mt-1 pl-[38px] text-[11px] leading-5 text-[var(--hub-label)]">
                      {[...counts].map(([type, n]) => `${t(`appWorkbench.widget.${type}`)}×${n}`).join(" · ")}
                      {writes.size > 0 && <span className="text-amber-700 dark:text-amber-300"> · {t("appMap.writesTo", { table: [...writes].join("、") })}</span>}
                    </div>
                  </Tile>
                );
              })}
              {a.spec.web && <Tile icon={<Globe size={14} />} bg="bg-[#10b981]" kind={t("appMap.web")} title={t("appMap.webDesc", { slug: a.slug })} to={`/a/${a.slug}`} />}
              {widgets.length === 0 && !a.spec.web && <p className={emptyBox}>{t("appMap.noProvides")}</p>}
            </div>
          </div>
        </div>
      </div>

      <div className="mt-5">
        <h3 className="text-sm font-medium text-neutral-900">{t("appMap.opsTitle")}</h3>
        <p className="mt-0.5 mb-2 text-xs text-neutral-600">{t("appMap.opsDesc")}</p>
        {ops.length === 0 ? (
          <p className="text-sm text-neutral-500">{t("appMap.noOps")}</p>
        ) : (
          <div className="overflow-x-auto rounded-md border border-neutral-200">
            <table className="w-full text-xs">
              <thead className="bg-neutral-50 text-left text-[11px] text-neutral-500 dark:bg-neutral-700/30">
                <tr>
                  <th className="px-2.5 py-1.5 font-medium">{t("appMap.col.target")}</th>
                  <th className="px-2.5 py-1.5 font-medium">{t("appMap.col.where")}</th>
                  <th className="px-2.5 py-1.5 font-medium">{t("appMap.col.can")}</th>
                  <th className="px-2.5 py-1.5 font-medium">{t("appMap.col.how")}</th>
                </tr>
              </thead>
              <tbody>
                {ops.map((o) => (
                  <tr key={o.key} className="border-t border-neutral-100 dark:border-neutral-700/60">
                    <td className="px-2.5 py-1.5 text-neutral-900">{o.target}</td>
                    <td className="px-2.5 py-1.5 text-neutral-700">{o.where}</td>
                    <td className="px-2.5 py-1.5">
                      <div className="flex flex-wrap gap-1">
                        {o.can.map((c) => <Badge key={c} tone={o.write ? "amber" : "slate"}>{c}</Badge>)}
                      </div>
                    </td>
                    <td className="px-2.5 py-1.5 text-neutral-600">{o.how.join(" · ")}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </Card>
  );
}
