import { useEffect, useMemo, useState, type FormEvent } from "react";
import { ArrowDown, ArrowUp, ExternalLink, MousePointerClick } from "lucide-react";
import { api, type Menu, type MenuInput, type MenuKind, type Permission } from "../../lib/api";
import { useStore } from "../../lib/store";
import { useAction, useApi } from "../../lib/use-api";
import { buildTree, isDefaultPageTitle, useMenuTitle, useMenus } from "../../lib/menus";
import { PAGE_META, type PageKey } from "../../lib/page-meta";
import { useLabels } from "../../lib/labels";
import { ICONS, MenuIcon } from "../../lib/icons";
import { ErrorNote, LoadState } from "../../components/api-ui";
import { Badge, Button, Card, CardHeader, cx, Field, inputCls, PageHeader, Segmented, Toggle, Empty } from "../../components/ui";
import { useTranslation } from "react-i18next";
import { useConfirm } from "@/lib/confirm";
import { Combobox } from "@/components/pickers";
import { Tip } from "@/components/tip";

type Editing = { mode: "edit"; menu: Menu } | { mode: "new"; kind: MenuKind };


/** 只取可编辑字段：后端拒绝未知字段，不能把 id、updated_at 一起发回去。 */
function toInput(m: MenuInput): MenuInput {
  const { parent_id, kind, title, path, page_key, icon, permission, sort, visible } = m;
  return { parent_id, kind, title, path, page_key, icon, permission, sort, visible };
}

function blank(kind: MenuKind): MenuInput {
  return { parent_id: null, kind, title: "", path: kind === "link" ? "https://" : "/", page_key: "", icon: kind === "link" ? "ExternalLink" : "", permission: "", sort: 9999, visible: true };
}

function MenuForm({
  editing,
  groups,
  permissions,
  onDone,
}: {
  editing: Editing;
  groups: Menu[];
  permissions: Permission[];
  onDone: (deleted?: boolean) => void;
}) {
  const { t } = useTranslation();
  const confirm = useConfirm();
  const L = useLabels();
  const menuTitle = useMenuTitle();
  const { notify } = useStore();
  /**
   * 默认标题在编辑框里显示成当前界面语言：新建页面时写进后端的也是当前语言，
   * 不再固定写中文默认值；显示时 `useMenuTitle` 认得任一语言的默认页面名。
   */
  function withLocalizedTitle(input: MenuInput): MenuInput {
    return input.kind === "page" && isDefaultPageTitle(input.page_key, input.title)
      ? { ...input, title: t(`pages.${input.page_key as PageKey}`) }
      : input;
  }
  const initial: MenuInput = withLocalizedTitle(editing.mode === "edit" ? toInput(editing.menu) : blank(editing.kind));
  const [m, setM] = useState<MenuInput>(initial);
  const [error, setError] = useState<string | null>(null);
  const { busy, run } = useAction();
  const set = (patch: Partial<MenuInput>) => setM((x) => ({ ...x, ...patch }));
  const key = editing.mode === "edit" ? `e${editing.menu.id}` : `n${editing.kind}`;

  useEffect(() => {
    setM(withLocalizedTitle(editing.mode === "edit" ? toInput(editing.menu) : blank(editing.kind)));
    setError(null);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key]);

  const pageKeys = Object.keys(PAGE_META);
  const unknownKey = m.kind === "page" && m.page_key && !(m.page_key in PAGE_META);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setError(null);
    const r = await run(() => (editing.mode === "edit" ? api.updateMenu(editing.menu.id, toInput(m)) : api.createMenu(toInput(m))));
    if (!r.ok) return setError(r.error);
    notify(editing.mode === "edit" ? t("menusAdmin.saved", { title: m.title }) : t("menusAdmin.created", { title: m.title }));
    onDone();
  };

  const remove = async () => {
    if (editing.mode !== "edit") return;
    if (!(await confirm({ kind: "delete", description: t("menusAdmin.deleteConfirm", { title: editing.menu.title }) }))) return;
    const r = await run(() => api.deleteMenu(editing.menu.id));
    if (!r.ok) return setError(r.error);
    notify(t("menusAdmin.deleted", { title: editing.menu.title }));
    onDone(true);
  };

  return (
    <Card>
      <CardHeader title={t(editing.mode === "edit" ? "menusAdmin.editTitle" : "menusAdmin.newTitle", { kind: t(`menusAdmin.kinds.${m.kind}`) })} />
      <form onSubmit={submit} className="space-y-4 px-5 pb-5">
        {m.kind !== "group" && editing.mode === "edit" && (
          <Field label={t("menusAdmin.type")}>
            <Segmented
              value={m.kind}
              onChange={(kind) => set({ kind, ...(kind === "link" ? { path: "https://", page_key: "" } : { path: "/" }) })}
              options={[
                { value: "page", label: t("menusAdmin.internalPage") },
                { value: "link", label: t("menusAdmin.externalLink") },
              ]}
            />
          </Field>
        )}
        <Field label={t("menusAdmin.title")} hint={t("menusAdmin.titleHint")}>
          <input className={inputCls} required maxLength={64} value={m.title} onChange={(e) => set({ title: e.target.value })} />
        </Field>

        {m.kind === "page" && (
          <>
            <Field label={t("menusAdmin.component")} hint={m.page_key && PAGE_META[m.page_key] ? t(`pageDesc.${m.page_key as PageKey}`) : t("menusAdmin.componentHint")}>
              <Combobox
                value={m.page_key}
                placeholder={t("menusAdmin.choosePage")}
                options={[
                  ...(unknownKey ? [{ value: m.page_key, label: t("menusAdmin.unregistered", { key: m.page_key }) }] : []),
                  ...pageKeys.map((k) => ({ value: k, label: t(`pages.${k as PageKey}`), hint: k })),
                ]}
                onChange={(k) => {
                  const meta = PAGE_META[k];
                  // 新建时顺手带出默认标题与路由；标题按当前界面语言，避免把中文写进后端
                  const useDefaultTitle = !!meta && (m.title.trim() === "" || isDefaultPageTitle(m.page_key, m.title));
                  set({
                    page_key: k,
                    ...(useDefaultTitle ? { title: t(`pages.${k as PageKey}`) } : {}),
                    ...(meta && (m.path === "/" || !m.path) ? { path: meta.defaultPath } : {}),
                  });
                }}
              />
            </Field>
            <Field label={t("menusAdmin.route")} hint={t("menusAdmin.routeHint")}>
              <input className={cx(inputCls, "font-mono")} required value={m.path} onChange={(e) => set({ path: e.target.value })} />
            </Field>
          </>
        )}
        {m.kind === "link" && (
          <Field label={t("menusAdmin.linkUrl")} hint={t("menusAdmin.linkHint")}>
            <input className={cx(inputCls, "font-mono")} required value={m.path} onChange={(e) => set({ path: e.target.value })} />
          </Field>
        )}

        {m.kind !== "group" && (
          <Field label={t("menusAdmin.group")}>
            <select
              className={inputCls}
              value={m.parent_id ?? ""}
              onChange={(e) => set({ parent_id: e.target.value ? Number(e.target.value) : null })}
            >
              <option value="">{t("menusAdmin.topLevel")}</option>
              {groups.map((g) => (
                <option key={g.id} value={g.id}>
                  {menuTitle(g)}
                </option>
              ))}
            </select>
          </Field>
        )}

        <Field label={t("menusAdmin.access")} hint={m.kind === "group" ? t("menusAdmin.accessGroupHint") : t("menusAdmin.accessPageHint")}>
          <Combobox
            value={m.permission}
            onChange={(permission) => set({ permission })}
            allowEmpty
            emptyLabel={t("menusAdmin.loginOnly")}
            options={permissions.map((p) => ({ value: p.code, label: L.perm(p), hint: p.code, group: L.group(p.group) }))}
          />
        </Field>

        {m.kind !== "group" && (
          <fieldset>
            <legend className="mb-1.5 text-sm font-medium text-neutral-700">{t("menusAdmin.icon")}</legend>
            <div className="flex flex-wrap gap-1">
              {Object.keys(ICONS).map((name) => (
                <button
                  key={name}
                  type="button"
                  title={name}
                  aria-label={name}
                  aria-pressed={m.icon === name}
                  onClick={() => set({ icon: name })}
                  className={cx(
                    "flex h-8 w-8 items-center justify-center rounded-md",
                    m.icon === name ? "bg-brand-500 text-white" : "text-neutral-600 hover:bg-neutral-100",
                  )}
                >
                  <MenuIcon name={name} />
                </button>
              ))}
            </div>
          </fieldset>
        )}

        <label className="flex items-center justify-between gap-4 text-sm">
          <span>
            <span className="block text-neutral-800">{t("menusAdmin.showInSidebar")}</span>
            <span className="block text-xs text-neutral-600">{t("menusAdmin.showInSidebarHint")}</span>
          </span>
          <Toggle checked={m.visible} onChange={(visible) => set({ visible })} />
        </label>

        <ErrorNote>{error}</ErrorNote>
        <div className="flex gap-2">
          <Button type="submit" disabled={busy || !m.title.trim()}>
            {busy ? t("common.saving") : t("common.save")}
          </Button>
          {editing.mode === "edit" && (
            <Button type="button" variant="danger" disabled={busy} onClick={remove}>
              {t("common.delete")}
            </Button>
          )}
        </div>
      </form>
    </Card>
  );
}

function Row({
  m,
  first,
  last,
  selected,
  permissionName,
  onSelect,
  onMove,
  indent,
}: {
  m: Menu;
  first: boolean;
  last: boolean;
  selected: boolean;
  permissionName: (code: string) => string;
  onSelect: () => void;
  onMove: (dir: -1 | 1) => void;
  indent?: boolean;
}) {
  const { t } = useTranslation();
  const menuTitle = useMenuTitle();
  return (
    <div className={cx("group flex items-center gap-2 rounded-md py-1.5 pr-1", indent ? "pl-7" : "pl-2", selected ? "bg-neutral-100" : "hover:bg-neutral-50")}>
      <button type="button" onClick={onSelect} className="flex min-w-0 flex-1 items-center gap-2.5 text-left">
        {m.kind === "group" ? (
          <span className="text-sm font-medium text-neutral-900">{menuTitle(m)}</span>
        ) : (
          <>
            <MenuIcon name={m.icon || (m.kind === "link" ? "ExternalLink" : "")} className="shrink-0 text-neutral-500" />
            <span className="min-w-0">
              <span className="flex items-center gap-1.5 text-sm text-neutral-900">
                {menuTitle(m)}
                {m.kind === "link" && <ExternalLink size={12} className="text-neutral-400" />}
              </span>
              <span className="block truncate font-mono text-xs text-neutral-500">{m.path}</span>
            </span>
          </>
        )}
        <span className="ml-auto flex shrink-0 flex-wrap justify-end gap-1">
          {!m.visible && <Badge tone="amber">{t("menusAdmin.hidden")}</Badge>}
          {m.kind === "page" && !(m.page_key in PAGE_META) && <Badge tone="red">{t("menusAdmin.unregisteredBadge")}</Badge>}
          {m.permission ? <Badge>{permissionName(m.permission)}</Badge> : m.kind !== "group" && <Badge tone="green">{t("menusAdmin.loginOnly")}</Badge>}
        </span>
      </button>
      <span className="flex shrink-0 gap-0.5">
        <Tip label={t("menusAdmin.moveUp")}>
          <button type="button" aria-label={t("menusAdmin.moveUp")} disabled={first} onClick={() => onMove(-1)} className="rounded p-1 text-neutral-500 hover:bg-neutral-200 disabled:opacity-30">
            <ArrowUp size={14} />
          </button>
        </Tip>
        <Tip label={t("menusAdmin.moveDown")}>
          <button type="button" aria-label={t("menusAdmin.moveDown")} disabled={last} onClick={() => onMove(1)} className="rounded p-1 text-neutral-500 hover:bg-neutral-200 disabled:opacity-30">
            <ArrowDown size={14} />
          </button>
        </Tip>
      </span>
    </div>
  );
}

export default function AdminMenus() {
  const { t } = useTranslation();
  const confirm = useConfirm();
  const L = useLabels();
  const { notify } = useStore();
  const all = useApi(api.menus);
  const perms = useApi(api.permissions);
  const { reload: reloadSidebar } = useMenus();
  const [editing, setEditing] = useState<Editing | null>(null);
  const { busy, run } = useAction();
  const menus = all.data ?? [];
  const tree = useMemo(() => buildTree(menus), [menus]);
  const groups = menus.filter((m) => m.kind === "group").sort((a, b) => a.sort - b.sort);
  const permissionName = (code: string) => {
    const p = perms.data?.find((x) => x.code === code);
    return p ? L.perm(p) : code;
  };

  const refreshAll = () => {
    all.reload();
    void reloadSidebar();
  };

  const move = async (siblings: Menu[], index: number, dir: -1 | 1) => {
    const list = [...siblings];
    const [item] = list.splice(index, 1);
    list.splice(index + dir, 0, item);
    const r = await run(() => api.reorderMenus(list.map((m, i) => ({ id: m.id, parent_id: m.parent_id, sort: (i + 1) * 10 }))));
    if (!r.ok) notify(r.error);
    refreshAll();
  };

  const reset = async () => {
    if (!(await confirm({ kind: "reset", description: t("menusAdmin.resetConfirm") }))) return;
    const r = await run(() => api.resetMenus());
    notify(r.ok ? t("menusAdmin.resetDone") : r.error);
    setEditing(null);
    refreshAll();
  };

  const selectedId = editing?.mode === "edit" ? editing.menu.id : null;

  return (
    <div>
      <PageHeader
        title={t("menusAdmin.pageTitle")}
        desc={t("menusAdmin.pageDesc")}
        actions={
          <>
            <Button size="sm" variant="secondary" onClick={() => setEditing({ mode: "new", kind: "group" })}>
              {t("menusAdmin.newGroup")}
            </Button>
            <Button size="sm" variant="secondary" onClick={() => setEditing({ mode: "new", kind: "link" })}>
              {t("menusAdmin.newLink")}
            </Button>
            <Button size="sm" onClick={() => setEditing({ mode: "new", kind: "page" })}>
              {t("menusAdmin.newPage")}
            </Button>
          </>
        }
      />
      <LoadState loading={(all.loading && !all.data) || (perms.loading && !perms.data)} error={all.error ?? perms.error} onRetry={() => (all.reload(), perms.reload())}>
        <div className="grid items-start gap-4 lg:grid-cols-[minmax(0,1fr)_420px]">
          <Card>
            <CardHeader
              title={t("menusAdmin.structure")}
              aside={
                <button type="button" disabled={busy} onClick={reset} className="text-sm text-neutral-600 hover:text-neutral-900 disabled:opacity-50">
                  {t("menusAdmin.reset")}
                </button>
              }
            />
            <div className="space-y-3 px-3 pb-4">
              {tree.map((n, i) => (
                <div key={n.id}>
                  <Row
                    m={n}
                    first={i === 0}
                    last={i === tree.length - 1}
                    selected={selectedId === n.id}
                    permissionName={permissionName}
                    onSelect={() => setEditing({ mode: "edit", menu: n })}
                    onMove={(d) => move(tree, i, d)}
                  />
                  {n.kind === "group" &&
                    (n.children.length === 0 ? (
                      <p className="py-1.5 pl-7 text-xs text-neutral-500">{t("menusAdmin.emptyGroup")}</p>
                    ) : (
                      n.children.map((c, j) => (
                        <Row
                          key={c.id}
                          m={c}
                          indent
                          first={j === 0}
                          last={j === n.children.length - 1}
                          selected={selectedId === c.id}
                          permissionName={permissionName}
                          onSelect={() => setEditing({ mode: "edit", menu: c })}
                          onMove={(d) => move(n.children, j, d)}
                        />
                      ))
                    ))}
                </div>
              ))}
            </div>
          </Card>
          <div className="lg:sticky lg:top-6">
            {editing && perms.data ? (
              <MenuForm
                editing={editing}
                groups={groups}
                permissions={perms.data}
                onDone={(deleted) => {
                  if (deleted) setEditing(null);
                  refreshAll();
                  if (editing.mode === "new") setEditing(null);
                }}
              />
            ) : (
              <Card><Empty icon={MousePointerClick}>{t("menusAdmin.pickHint")}</Empty></Card>
            )}
          </div>
        </div>
      </LoadState>
    </div>
  );
}
