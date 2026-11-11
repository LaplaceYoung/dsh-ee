import { useState, type FormEvent } from "react";
import { api, formatTime, type Role, type User } from "../../lib/api";
import { useAuth } from "../../lib/auth";
import { useStore } from "../../lib/store";
import { useAction, useApi } from "../../lib/use-api";
import { PageLink } from "../../lib/menus";
import { useLabels } from "../../lib/labels";
import { DataTable, columnHelper, type DataColumn } from "@/components/data-table";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from "@/components/shadcn/dropdown-menu";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/shadcn/tooltip";
import { KeyRound, MoreHorizontal, ShieldCheck, Unlink, UserCheck, UserX } from "lucide-react";
import { ErrorNote, LoadState } from "../../components/api-ui";
import { Badge, Button, Card, CardHeader, cx, Field, inputBase, inputCls, PageHeader, Segmented } from "../../components/ui";
import { useTranslation } from "react-i18next";
import { useConfirm } from "@/lib/confirm";

const emptyForm = { username: "", password: "", display_name: "", department: "" };

/** 角色多选；超级管理员角色单独标出。 */
function RolePicker({ roles, value, onChange }: { roles: Role[]; value: number[]; onChange: (ids: number[]) => void }) {
  const { t } = useTranslation();
  const L = useLabels();
  return (
    <div className="flex flex-wrap gap-x-5 gap-y-2">
      {roles.map((r) => (
        <label key={r.id} className="flex items-center gap-2 text-sm text-neutral-800" title={L.roleDesc(r)}>
          <input
            type="checkbox"
            className="h-4 w-4 accent-neutral-900"
            checked={value.includes(r.id)}
            onChange={(e) => onChange(e.target.checked ? [...value, r.id] : value.filter((x) => x !== r.id))}
          />
          {L.role(r)}
          {r.is_super && <span className="text-xs text-neutral-500">{t("users.allPermissions")}</span>}
        </label>
      ))}
    </div>
  );
}

function CreateUser({ roles, users, onCreated }: { roles: Role[] | null; users: User[]; onCreated: (u: User) => void }) {
  const { t } = useTranslation();
  const [f, setF] = useState(emptyForm);
  const defaultRoles = roles?.filter((r) => r.code === "member").map((r) => r.id) ?? [];
  const [roleIds, setRoleIds] = useState<number[] | null>(null);
  const [source, setSource] = useState<"local" | "ldap">("local");
  const [accountType, setAccountType] = useState<"human" | "bot">("human");
  const [ownerId, setOwnerId] = useState("");
  const methods = useApi(api.authMethods);
  const [error, setError] = useState<string | null>(null);
  const { busy, run } = useAction();
  const set = (patch: Partial<typeof f>) => setF((x) => ({ ...x, ...patch }));
  const ldap = source === "ldap";
  const bot = accountType === "bot";

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setError(null);
    // 没动过角色就不传，由服务端给默认角色；LDAP 账号不设本地密码
    const r = await run(() =>
      api.createUser({
        ...f,
        username: f.username.trim(),
        account_type: accountType,
        ...(bot ? { password: undefined, ...(ownerId ? { owner_id: Number(ownerId) } : {}) } : {}),
        ...(ldap ? { password: undefined, auth_source: "ldap" as const } : {}),
        ...(roleIds ? { role_ids: roleIds } : {}),
      }),
    );
    if (!r.ok) return setError(r.error);
    setF(emptyForm);
    setRoleIds(null);
    onCreated(r.value);
  };

  return (
    <form onSubmit={submit} className="space-y-4 px-5 pb-5">
      <Field label={t("users.accountType")}>
        <Segmented value={accountType} onChange={setAccountType} options={[{ value: "human", label: t("users.human") }, { value: "bot", label: t("users.bot") }]} />
      </Field>
      <Field
        label={t("users.source")}
        hint={ldap ? (methods.data && !methods.data.ldap ? `${t("users.ldapCreateHint")} ${t("users.ldapOff")}` : t("users.ldapCreateHint")) : undefined}
      >
        <Segmented
          value={source}
          onChange={setSource}
          options={[
            { value: "local", label: t("users.sourceLocal") },
            { value: "ldap", label: t("users.sourceLdap") },
          ]}
        />
      </Field>
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label={t("users.username")} hint={t("users.usernameHint")}>
          <input className={inputCls} required value={f.username} onChange={(e) => set({ username: e.target.value })} placeholder="zhang.wei" />
        </Field>
        {!ldap && !bot && (
          <Field label={t("users.initialPassword")} hint={t("users.initialPasswordHint")}>
            <input className={inputCls} type="text" autoComplete="off" required value={f.password} onChange={(e) => set({ password: e.target.value })} />
          </Field>
        )}
        <Field label={t("users.displayName")}>
          <input className={inputCls} value={f.display_name} onChange={(e) => set({ display_name: e.target.value })} placeholder={t("users.displayNamePlaceholder")} />
        </Field>
        <Field label={t("users.department")}>
          <input className={inputCls} value={f.department} onChange={(e) => set({ department: e.target.value })} placeholder={t("users.departmentPlaceholder")} />
        </Field>
        {bot && (
          <Field label={t("users.owner")}>
            <select className={inputCls} value={ownerId} onChange={(e) => setOwnerId(e.target.value)}>
              <option value="">{t("users.noOwner")}</option>
              {users.filter((u) => u.account_type !== "bot").map((u) => <option key={u.id} value={u.id}>{u.display_name || u.username}</option>)}
            </select>
          </Field>
        )}
      </div>
      {roles ? (
        <fieldset>
          <legend className="mb-1.5 text-sm font-medium text-neutral-700">{t("users.roles")}</legend>
          <RolePicker roles={roles} value={roleIds ?? defaultRoles} onChange={setRoleIds} />
        </fieldset>
      ) : (
        <p className="text-sm text-neutral-600">{t("users.defaultRoleNote")}</p>
      )}
      <ErrorNote>{error}</ErrorNote>
      <Button type="submit" disabled={busy || !f.username || (!ldap && !bot && !f.password)}>
        {busy ? t("common.creating") : t("users.create")}
      </Button>
    </form>
  );
}

export default function AdminUsers() {
  const { t } = useTranslation();
  const confirm = useConfirm();
  const L = useLabels();
  const { user: me, can, refresh } = useAuth();
  const { notify } = useStore();
  const { data: users, error, loading, reload } = useApi(api.users);
  const canRoles = can("roles.manage");
  const canManage = can("users.manage");
  const roles = useApi(() => (canRoles ? api.roles() : Promise.resolve(null)));
  const { busy, run } = useAction();
  const [creating, setCreating] = useState(false);
  const [resetFor, setResetFor] = useState<number | null>(null);
  const [rolesFor, setRolesFor] = useState<{ id: number; ids: number[] } | null>(null);
  const [newPw, setNewPw] = useState("");

  const patch = async (u: User, p: Parameters<typeof api.updateUser>[1], done: string) => {
    const r = await run(() => api.updateUser(u.id, p));
    notify(r.ok ? done : r.error);
    if (r.ok) reload();
    return r.ok;
  };

  const saveRoles = async (u: User, ids: number[]) => {
    const r = await run(() => api.setUserRoles(u.id, ids));
    notify(r.ok ? t("users.rolesUpdated", { name: u.username }) : r.error);
    if (r.ok) {
      setRolesFor(null);
      reload();
      // 改的是自己时，侧边栏与权限要跟着刷新
      if (u.id === me?.id) await refresh();
    }
  };

  const col = columnHelper<User>();
  const columns: DataColumn<User>[] = [
    col.accessor((u) => u.display_name || u.username, {
      id: "user",
      header: () => t("users.colUser"),
      cell: ({ row: { original: u } }) => (
        <>
          <div className={u.disabled ? "" : "text-neutral-900"}>
            {u.display_name || u.username}
            {u.id === me?.id && <span className="ml-1.5 text-xs text-neutral-500">{t("users.you")}</span>}
          </div>
          <div className="flex items-center gap-1.5 text-xs text-neutral-600">
            {u.username}
            {u.account_type === "bot" && <Badge>{t("users.bot")}</Badge>}
            {u.auth_source === "ldap" && (
              <span title={u.external_dn ? t("users.ldapTip", { dn: u.external_dn }) : undefined}>
                <Badge tone="brand">{t("users.ldapBadge")}</Badge>
              </span>
            )}
            {u.auth_source === "oidc" && <Badge tone="brand">{t("ssoLogin.badge")}</Badge>}
          </div>
        </>
      ),
    }),
    col.accessor((u) => u.department, {
      id: "department",
      header: () => t("users.colDepartment"),
      cell: (c) => c.getValue() || "—",
    }),
    col.accessor((u) => (u.roles ?? []).map((r) => L.role(r)).join(", "), {
      id: "roles",
      header: () => t("users.colRoles"),
      cell: ({ row: { original: u } }) => (
        <div className="flex flex-wrap gap-1">
          {(u.roles ?? []).length === 0 && <span className="text-neutral-500">{t("users.unassigned")}</span>}
          {(u.roles ?? []).map((r) => (
            <Badge key={r.id} tone={r.is_super ? "brand" : "slate"}>
              {L.role(r)}
            </Badge>
          ))}
        </div>
      ),
    }),
    col.accessor((u) => u.last_login_at ?? "", {
      id: "lastLogin",
      header: () => t("users.colLastLogin"),
      cell: (c) => (c.getValue() ? formatTime(c.getValue()) : <span className="text-neutral-500">{t("users.never")}</span>),
    }),
    col.accessor((u) => u.created_at, {
      id: "created",
      header: () => t("users.colCreated"),
      cell: (c) => formatTime(c.getValue()),
    }),
    col.accessor((u) => (u.disabled ? 1 : 0), {
      id: "status",
      header: () => t("users.colStatus"),
      cell: ({ row: { original: u } }) => (u.disabled ? <Badge>{t("users.disabled")}</Badge> : <Badge tone="green">{t("users.active")}</Badge>),
    }),
    col.display({
      id: "actions",
      header: () => <span className="sr-only">{t("table.actions")}</span>,
      cell: ({ row: { original: u } }) => {
        const self = u.id === me?.id;
        if (!canRoles && !canManage) return null;
        return (
          <div className="flex justify-end">
            <DropdownMenu>
              <Tooltip>
                <TooltipTrigger asChild>
                  <DropdownMenuTrigger asChild>
                    <button
                      type="button"
                      aria-label={t("table.more")}
                      disabled={busy}
                      className="rounded-md p-1.5 text-neutral-500 hover:bg-neutral-100 hover:text-neutral-900 disabled:opacity-40"
                    >
                      <MoreHorizontal size={16} />
                    </button>
                  </DropdownMenuTrigger>
                </TooltipTrigger>
                <TooltipContent>{t("table.more")}</TooltipContent>
              </Tooltip>
              <DropdownMenuContent align="end">
                {canRoles && (
                  <DropdownMenuItem
                    disabled={!roles.data}
                    onSelect={() => {
                      setResetFor(null);
                      setRolesFor({ id: u.id, ids: (u.roles ?? []).map((r) => r.id) });
                    }}
                  >
                    <ShieldCheck /> {t("users.rolesAction")}
                  </DropdownMenuItem>
                )}
                {canManage && u.auth_source === "local" && (
                  <DropdownMenuItem
                    onSelect={() => {
                      setRolesFor(null);
                      setResetFor(u.id);
                      setNewPw("");
                    }}
                  >
                    <KeyRound /> {t("users.resetPassword")}
                  </DropdownMenuItem>
                )}
                {canManage && !self && u.auth_source === "oidc" && u.external_dn && (
                  <DropdownMenuItem
                    onSelect={async () => {
                      if (!(await confirm({ kind: "unbind", description: t("users.unbindConfirm", { name: u.username }) }))) return;
                      void patch(u, { unbind_identity: true }, t("users.unboundMsg", { name: u.username }));
                    }}
                  >
                    <Unlink /> {t("users.unbindIdentity")}
                  </DropdownMenuItem>
                )}
                {canManage && !self && (
                  <>
                    <DropdownMenuSeparator />
                    {u.disabled ? (
                      <DropdownMenuItem onSelect={() => void patch(u, { disabled: false }, t("users.enabledMsg", { name: u.username }))}>
                        <UserCheck /> {t("users.enable")}
                      </DropdownMenuItem>
                    ) : (
                      <DropdownMenuItem
                        variant="destructive"
                        onSelect={async () => {
                          if (!(await confirm({ kind: "disable", description: t("users.disableConfirm", { name: u.username }) }))) return;
                          void patch(u, { disabled: true }, t("users.disabledMsg", { name: u.username }));
                        }}
                      >
                        <UserX /> {t("users.disable")}
                      </DropdownMenuItem>
                    )}
                  </>
                )}
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
        );
      },
    }),
  ];

  return (
    <div>
      <PageHeader
        title={t("users.title")}
        desc={t("users.desc")}
        actions={
          canManage && (
            <Button size="sm" onClick={() => setCreating((v) => !v)}>
              {creating ? t("common.close") : t("users.newUser")}
            </Button>
          )
        }
      />

      {creating && (
        <Card className="mb-4">
          <CardHeader title={t("users.newUser")} />
          <CreateUser
            roles={roles.data}
            users={users ?? []}
            onCreated={(u) => {
              notify(t("users.created", { name: u.username }));
              setCreating(false);
              reload();
            }}
          />
        </Card>
      )}

      <Card>
        <CardHeader
          title={t("users.all")}
          aside={
            <>
              {users ? t("users.count", { count: users.length }) : ""}
              {canRoles && (
                <>
                  {" · "}
                  <PageLink page="admin-roles" className="font-medium text-brand-600 hover:text-brand-700 dark:text-brand-300 dark:hover:text-brand-200">
                    {t("users.manageRoles")}
                  </PageLink>
                </>
              )}
            </>
          }
        />
        <LoadState loading={loading && !users} error={error} onRetry={reload}>
          <DataTable
            data={users ?? []}
            getRowId={(u) => String(u.id)}
            columns={columns}
            searchText={(u) => `${u.username} ${u.display_name} ${u.department} ${(u.roles ?? []).map((r) => L.role(r)).join(" ")}`}
            searchPlaceholder={t("users.searchPlaceholder")}
            hideOnMobile={["department", "lastLogin", "created"]}
            alignRight={["lastLogin", "created"]}
            rowClassName={(u) => u.disabled && "text-neutral-500"}
            emptyText={t("users.none")}
            noMatchText={t("users.noMatch")}
            initialSorting={[{ id: "created", desc: false }]}
            renderExpanded={(u) =>
              resetFor === u.id ? (
                <form
                  className="flex flex-wrap items-center gap-2"
                  onSubmit={async (e) => {
                    e.preventDefault();
                    if (await patch(u, { password: newPw }, t("users.passwordReset", { name: u.username }))) {
                      setResetFor(null);
                      setNewPw("");
                    }
                  }}
                >
                  <span className="text-sm text-neutral-700">{t("users.resetPasswordFor", { name: u.username })}</span>
                  <input
                    aria-label={t("users.newPassword")}
                    className={cx(inputBase, "h-8 w-56")}
                    placeholder={t("users.newPasswordPlaceholder")}
                    autoComplete="off"
                    autoFocus
                    value={newPw}
                    onChange={(e) => setNewPw(e.target.value)}
                  />
                  <Button size="sm" type="submit" disabled={busy || !newPw}>
                    {t("users.confirmReset")}
                  </Button>
                  <Button size="sm" variant="ghost" type="button" onClick={() => setResetFor(null)}>
                    {t("common.cancel")}
                  </Button>
                </form>
              ) : rolesFor?.id === u.id && roles.data ? (
                <div className="space-y-3">
                  <div className="text-sm text-neutral-700">{t("users.rolesFor", { name: u.username })}</div>
                  <RolePicker roles={roles.data} value={rolesFor.ids} onChange={(ids) => setRolesFor({ id: u.id, ids })} />
                  <div className="flex gap-2">
                    <Button size="sm" disabled={busy} onClick={() => saveRoles(u, rolesFor.ids)}>
                      {t("users.saveRoles")}
                    </Button>
                    <Button size="sm" variant="ghost" onClick={() => setRolesFor(null)}>
                      {t("common.cancel")}
                    </Button>
                  </div>
                </div>
              ) : null
            }
          />
        </LoadState>
      </Card>
    </div>
  );
}
