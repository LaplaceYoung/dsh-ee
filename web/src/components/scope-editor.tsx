import { useState, type KeyboardEvent } from "react";
import { X } from "lucide-react";
import { useTranslation } from "react-i18next";
import type { RoleRef, Scope, User } from "../lib/api";
import { useLabels } from "../lib/labels";
import { Combobox } from "./pickers";
import { cx, inputBase, Segmented } from "./ui";
import { isSelfOnly, selfScope } from "../lib/hub";

function Chip({ label, onRemove }: { label: string; onRemove: () => void }) {
  const { t } = useTranslation();
  return (
    <span className="inline-flex items-center gap-1 rounded bg-neutral-100 px-2 py-0.5 text-sm text-neutral-800">
      {label}
      <button type="button" onClick={onRemove} aria-label={t("scopeEditor.remove", { name: label })} className="text-neutral-500 hover:text-neutral-900">
        <X size={12} />
      </button>
    </span>
  );
}

/**
 * 可用范围：全公司，或部门 / 角色 / 指定人的并集。
 * 拿不到角色或用户列表（没有相应权限）时只显示部门，部门可以直接输入。
 * 给了 selfId 时多一个「只有我自己」：只有本人的范围不用审核（「自己用」）。
 */
export function ScopeEditor({
  value,
  onChange,
  roles,
  users,
  departments = [],
  selfId,
  noneHint,
}: {
  value: Scope;
  onChange: (s: Scope) => void;
  roles?: RoleRef[] | null;
  users?: User[] | null;
  departments?: string[];
  selfId?: number;
  /** 一个人都没选时的提示；默认是能力可用范围的说法，别的场景（如禁用策略）传自己的 */
  noneHint?: string;
}) {
  const { t } = useTranslation();
  const L = useLabels();
  const [dept, setDept] = useState("");
  const set = (p: Partial<Scope>) => onChange({ ...value, ...p });
  const addDept = () => {
    const d = dept.trim();
    if (d && !value.departments.includes(d)) set({ departments: [...value.departments, d] });
    setDept("");
  };
  const onKey = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "Enter") {
      e.preventDefault();
      addDept();
    }
  };
  const nobody = !value.all && value.departments.length + value.roles.length + value.users.length === 0;
  const userName = (id: number) => {
    if (id === selfId) return t("selfScope.self");
    const u = users?.find((x) => x.id === id);
    return u ? u.display_name || u.username : `#${id}`;
  };
  // 「指定范围」里一开始可能只有自己，要记住用户点的是哪一项
  const [custom, setCustom] = useState(false);
  const mode = value.all ? "all" : selfId && isSelfOnly(value, selfId) && !custom ? "self" : "custom";

  return (
    <div className="space-y-3">
      <Segmented
        value={mode}
        onChange={(v) => {
          setCustom(v === "custom");
          if (v === "self" && selfId) onChange(selfScope(selfId));
          else set({ all: v === "all" });
        }}
        options={[
          ...(selfId ? [{ value: "self" as const, label: t("selfScope.self") }] : []),
          { value: "all" as const, label: t("scopeEditor.all") },
          { value: "custom" as const, label: t("scopeEditor.custom") },
        ]}
      />
      {selfId && <p className="text-xs text-neutral-600">{mode === "self" ? t("selfScope.selfHint") : t("selfScope.reviewHint")}</p>}
      {mode === "custom" && (
        <div className="space-y-3 border-l-2 border-neutral-100 pl-4">
          <div>
            <div className="mb-1.5 text-sm font-medium text-neutral-700">{t("scopeEditor.departments")}</div>
            <div className="flex flex-wrap items-center gap-1.5">
              {value.departments.map((d) => (
                <Chip key={d} label={d} onRemove={() => set({ departments: value.departments.filter((x) => x !== d) })} />
              ))}
              <input
                className={cx(inputBase, "h-8 w-48")}
                list="scope-departments"
                value={dept}
                placeholder={t("scopeEditor.departmentsHint")}
                aria-label={t("scopeEditor.departments")}
                onChange={(e) => setDept(e.target.value)}
                onKeyDown={onKey}
                onBlur={addDept}
              />
              <datalist id="scope-departments">
                {departments.filter((d) => !value.departments.includes(d)).map((d) => (
                  <option key={d} value={d} />
                ))}
              </datalist>
            </div>
          </div>
          {roles && roles.length > 0 && (
            <div>
              <div className="mb-1.5 text-sm font-medium text-neutral-700">{t("scopeEditor.roles")}</div>
              <div className="flex flex-wrap gap-x-5 gap-y-2">
                {roles.map((r) => (
                  <label key={r.id} className="flex items-center gap-2 text-sm text-neutral-800">
                    <input
                      type="checkbox"
                      className="h-4 w-4 accent-neutral-900"
                      checked={value.roles.includes(r.id)}
                      onChange={(e) => set({ roles: e.target.checked ? [...value.roles, r.id] : value.roles.filter((x) => x !== r.id) })}
                    />
                    {L.role(r)}
                  </label>
                ))}
              </div>
            </div>
          )}
          {users && (
            <div>
              <div className="mb-1.5 text-sm font-medium text-neutral-700">{t("scopeEditor.users")}</div>
              <div className="flex flex-wrap items-center gap-1.5">
                {value.users.map((id) => (
                  <Chip key={id} label={userName(id)} onRemove={() => set({ users: value.users.filter((x) => x !== id) })} />
                ))}
                <Combobox
                  className="w-56"
                  value=""
                  aria-label={t("scopeEditor.users")}
                  placeholder={t("editor.pick")}
                  options={users
                    .filter((u) => !value.users.includes(u.id) && !u.disabled)
                    .map((u) => ({ value: String(u.id), label: u.display_name || u.username, hint: [u.username, u.department].filter(Boolean).join(" · ") }))}
                  onChange={(v) => v && set({ users: [...value.users, Number(v)] })}
                />
              </div>
            </div>
          )}
          {nobody && <p className="text-xs text-amber-700">{noneHint ?? t("scopeEditor.none")}</p>}
        </div>
      )}
    </div>
  );
}
