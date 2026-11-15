import { useCallback, useEffect, useState } from "react";
import { NavLink, Outlet, Link, useLocation, useNavigate } from "react-router";
import { ExternalLink, LogOut, Menu as MenuGlyph, Search, X } from "lucide-react";
import { WhaleMark } from "./whale-mark";
import { useStore } from "../lib/store";
import { useAuth } from "../lib/auth";
import { useMenuTitle, useMenus } from "../lib/menus";
import { LangSwitch } from "./lang-switch";
import { ThemeToggle } from "./theme-toggle";
import { useLabels } from "../lib/labels";
import { MenuIcon } from "../lib/icons";
import { api, type Menu } from "../lib/api";
import { useApi } from "../lib/use-api";
import { cx, Notice } from "./ui";
import { useTranslation } from "react-i18next";
import { Tip } from "@/components/tip";
import { CommandPalette, useHashFocus, useSearchShortcut } from "./command-palette";
import { entryOf, rememberSpace, rememberedSpace, spaceOfPath, splitSpaces, type NavSpace } from "../lib/nav-space";
import { DemoBar } from "./demo-site";


/** 个别页面在侧边栏上的附加状态（计数、已关闭、按设置隐藏），按 page_key 挂接。 */
function useNavExtras() {
  const { pendingCount: pending, settings } = useStore();
  return (pageKey: string): { count?: number; off?: boolean; hidden?: boolean } => {
    switch (pageKey) {
      case "admin-review":
        return { count: pending };
      case "admin-distribution":
        return { off: !settings.distribution.enabled };
      case "dsh-download":
        return { hidden: !settings.distribution.enabled };
      default:
        return {};
    }
  };
}

function NavEntry({ m, onNavigate }: { m: Menu; onNavigate: () => void }) {
  const extras = useNavExtras()(m.page_key);
  const { t } = useTranslation();
  const title = useMenuTitle()(m);
  if (extras.hidden) return null;
  const cls = "group flex h-[34px] items-center gap-2.5 rounded-md px-3 text-sm font-medium transition-colors lg:h-8";
  const idle = "text-neutral-600 hover:bg-neutral-100 hover:text-neutral-900";
  if (m.kind === "link") {
    return (
      <a href={m.path} target="_blank" rel="noreferrer" onClick={onNavigate} className={cx(cls, idle)}>
        <MenuIcon name={m.icon || "ExternalLink"} className="shrink-0 text-neutral-400 group-hover:text-neutral-600" />
        <span className="truncate">{title}</span>
        <ExternalLink size={12} className="ml-auto shrink-0 text-neutral-400" />
      </a>
    );
  }
  return (
    <NavLink
      to={m.path}
      end
      onClick={onNavigate}
      className={({ isActive }) =>
        cx(cls, isActive ? "active bg-brand-50 text-brand-600 dark:bg-brand-500/15 dark:text-brand-200" : idle)
      }
    >
      <MenuIcon name={m.icon} className="shrink-0 text-neutral-400 transition-colors group-hover:text-neutral-600 group-[.active]:text-brand-500 dark:group-[.active]:text-brand-300" />
      <span className="truncate">{title}</span>
      {extras.off && <span className="shrink-0 rounded bg-neutral-100 px-1 text-[11px] leading-4 font-normal text-neutral-500">{t("shell.off")}</span>}
      {!!extras.count && (
        <span className="ml-auto min-w-5 shrink-0 rounded-full bg-brand-500 px-1.5 text-center text-[11px] leading-5 font-semibold text-white tabular-nums">{extras.count}</span>
      )}
    </NavLink>
  );
}

/** 当前显示哪一套菜单：当前页面属于哪套就是哪套，不在菜单里的页面沿用上一次的。 */
function useNavSpace() {
  const { tree } = useMenus();
  const { pathname } = useLocation();
  const spaces = splitSpaces(tree);
  const matched = spaceOfPath(spaces, pathname);
  let space: NavSpace = matched ?? rememberedSpace();
  if (!spaces[space].length) space = space === "admin" ? "work" : "admin";
  useEffect(() => {
    if (matched) rememberSpace(matched, pathname);
  }, [matched, pathname]);
  return { spaces, space };
}

/** 「工作台 / 管理后台」切换；两套里有一套是空的（例如普通员工）就不显示。 */
function SpaceSwitch({ onNavigate }: { onNavigate: () => void }) {
  const { spaces, space } = useNavSpace();
  const navigate = useNavigate();
  const { t } = useTranslation();
  if (!spaces.work.length || !spaces.admin.length) return null;
  return (
    <div className="mx-1 mb-5 grid grid-cols-2 rounded-lg bg-neutral-100 p-0.5 text-sm ring-1 ring-neutral-200/60 ring-inset" role="tablist" aria-label={t("nav.admin")}>
      {(["work", "admin"] as const).map((s) => {
        const active = s === space;
        return (
          <button
            key={s}
            type="button"
            role="tab"
            aria-selected={active}
            {...(active ? {} : { "data-nav-switch": "" })}
            onClick={() => {
              if (active) return;
              const to = entryOf(spaces, s);
              if (to) {
                navigate(to);
                onNavigate();
              }
            }}
            className={cx(
              "rounded-md py-1 font-medium transition-[background-color,color,box-shadow] duration-150",
              active ? "bg-card text-neutral-900 shadow-control ring-1 ring-neutral-200/70 dark:ring-white/10" : "text-neutral-500 hover:text-neutral-900",
            )}
          >
            {t(s === "work" ? "nav.work" : "nav.admin")}
          </button>
        );
      })}
    </div>
  );
}

function SearchButton({ onOpen }: { onOpen: () => void }) {
  const { t } = useTranslation();
  const mac = typeof navigator !== "undefined" && /Mac|iPhone|iPad/.test(navigator.platform);
  return (
    <button
      type="button"
      onClick={onOpen}
      className="mx-1 mb-3 flex h-8 items-center gap-2 rounded-md border border-neutral-200 bg-neutral-50/60 px-2.5 text-left text-sm text-neutral-500 transition-colors hover:border-neutral-300 hover:bg-card hover:text-neutral-800 dark:border-white/10"
    >
      <Search size={14} />
      <span className="flex-1 truncate">{t("nav.search")}</span>
      <kbd className="rounded border border-neutral-200 bg-card px-1 text-[11px] leading-4 text-neutral-500 dark:border-white/10">{mac ? "⌘K" : "Ctrl K"}</kbd>
    </button>
  );
}

function Sidebar({ onNavigate, onSearch }: { onNavigate: () => void; onSearch: () => void }) {
  const { settings } = useStore();
  const { user, logout } = useAuth();
  const { loading, error, reload } = useMenus();
  const { spaces, space } = useNavSpace();
  const tree = spaces[space];
  const { t } = useTranslation();
  const menuTitle = useMenuTitle();
  const L = useLabels();
  return (
    <div className="flex min-h-full flex-col px-3 py-4">
      <Link to="/" className="mb-4 flex items-center gap-2.5 rounded-md px-3 py-1" onClick={onNavigate}>
        <WhaleMark size={30} className="shrink-0 text-brand-500" />
        <span className="min-w-0 leading-tight">
          <span className="block text-[15px] font-bold tracking-[-0.01em] text-neutral-900">dsh-ee</span>
          <span className="block truncate text-xs text-neutral-500">{settings.orgName}</span>
        </span>
      </Link>
      <SearchButton onOpen={onSearch} />
      <SpaceSwitch onNavigate={onNavigate} />
      <div className="flex flex-col gap-5 pb-6">
        {loading && <div className="px-3 text-sm text-neutral-500">{t("shell.loadingMenus")}</div>}
        {error && (
          <button type="button" onClick={() => void reload()} className="px-3 text-left text-sm text-red-700 hover:underline">
            {t("shell.menusFailed")}
          </button>
        )}
        {tree.map((n) =>
          n.kind === "group" ? (
            n.children.length ? (
              <div key={n.id}>
                <div className="px-3 pb-1.5 text-xs font-semibold tracking-[0.04em] text-neutral-500/80">{menuTitle(n)}</div>
                <nav className="flex flex-col gap-0.5">
                  {n.children
                    .map((c) => (
                      <NavEntry key={`${c.id}-${c.page_key}`} m={c} onNavigate={onNavigate} />
                    ))}
                </nav>
              </div>
            ) : null
          ) : (
            <nav key={n.id} className="flex flex-col">
              <NavEntry key={`${n.id}-${n.page_key}`} m={n} onNavigate={onNavigate} />
            </nav>
          ),
        )}
      </div>
      <div className="mt-auto flex items-center justify-between border-t border-neutral-200/70 px-3 pt-3 dark:border-white/[0.07]">
        <LangSwitch />
        <ThemeToggle />
      </div>
      {user && (
        <div className="flex items-center gap-1 pt-2">
          <Link
            to="/account"
            onClick={onNavigate}
            className="flex min-w-0 flex-1 items-center gap-2.5 rounded-md px-3 py-1.5 text-sm transition-colors hover:bg-neutral-100"
          >
            <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-gradient-to-br from-[#73c8ff] via-brand-500 to-[#6d5dfc] text-xs font-semibold text-white shadow-control">
              {(user.display_name || user.username).slice(0, 1).toUpperCase()}
            </span>
            <span className="min-w-0 leading-tight">
              <span className="block truncate font-medium text-neutral-900">{user.display_name || user.username}</span>
              <span className="block truncate text-xs text-neutral-600">
                {[user.department, (user.roles ?? []).map((r) => L.role(r)).join(", ")].filter(Boolean).join(" · ")}
              </span>
            </span>
          </Link>
          <Tip label={t("shell.logout")} side="top">
            <button
              type="button"
              aria-label={t("shell.logout")}
              onClick={() => void logout()}
              className="rounded-md p-2 text-neutral-400 transition-colors hover:bg-neutral-100 hover:text-neutral-900"
            >
              <LogOut size={16} />
            </button>
          </Tip>
        </div>
      )}
    </div>
  );
}

/** 工作台与管理后台共用的外壳：左侧导航，窄屏收起为顶栏 + 抽屉。 */
export function AppLayout() {
  const { settings } = useStore();
  const { user } = useAuth();
  // 公开演示站：每页顶上提醒共用账号、会重置
  const demo = useApi(api.authMethods).data?.demo ?? null;
  const { search } = useLocation();
  const [open, setOpen] = useState(false);
  const { t } = useTranslation();
  const { pathname, hash } = useLocation();
  const [searching, setSearching] = useState(false);
  const toggleSearch = useCallback(() => setSearching((v) => !v), []);
  useSearchShortcut(toggleSearch);
  useHashFocus(pathname, hash);
  // 用块体：新版浏览器的 scrollTo 会返回 Promise，被当成 effect 的清理函数会报错
  useEffect(() => {
    window.scrollTo(0, 0);
  }, [pathname]);

  return (
    <div className="app-canvas min-h-screen bg-neutral-50 lg:flex">
      <aside className="sticky top-0 hidden h-screen w-60 shrink-0 overflow-y-auto border-r border-neutral-200/80 bg-card lg:block dark:border-white/[0.07]">
        <Sidebar onNavigate={() => {}} onSearch={() => setSearching(true)} />
      </aside>

      <header className="sticky top-0 z-30 flex h-12 items-center gap-2.5 border-b border-neutral-200/80 bg-card/90 px-3 backdrop-blur lg:hidden dark:border-white/[0.07]">
        <button
          type="button"
          aria-label={open ? t("shell.closeNav") : t("shell.openNav")}
          onClick={() => setOpen((v) => !v)}
          className="rounded-md p-1.5 text-neutral-700 transition-colors hover:bg-neutral-100"
        >
          {open ? <X size={20} /> : <MenuGlyph size={20} />}
        </button>
        <WhaleMark size={22} className="shrink-0 text-brand-500" />
        <span className="text-sm font-bold text-neutral-900">dsh-ee</span>
        <span className="min-w-0 truncate text-sm text-neutral-500">{settings.orgName}</span>
        {/* 窄屏顶栏的语言入口：Globe + 原生下拉，375px 宽下仍可用（与搜索按钮同一行）。 */}
        <LangSwitch compact className="ml-auto -mr-1" />
        <button type="button" aria-label={t("nav.search")} onClick={() => setSearching(true)} className="shrink-0 rounded-md p-1.5 text-neutral-600 transition-colors hover:bg-neutral-100">
          <Search size={18} />
        </button>
      </header>
      <CommandPalette open={searching} onOpenChange={setSearching} />
      {open && (
        <div className="fixed inset-x-0 top-12 bottom-0 z-20 animate-in overflow-y-auto bg-card duration-150 fade-in-0 slide-in-from-top-1 lg:hidden">
          <Sidebar onNavigate={() => setOpen(false)} onSearch={() => { setOpen(false); setSearching(true); }} />
        </div>
      )}

      <div className="min-w-0 flex-1">
        <main className="mx-auto max-w-[1240px] px-4 py-6 sm:px-8 lg:px-10 lg:py-9">
          {demo && <DemoBar demo={demo} className="mb-5" />}
          {user?.limited && (
            <Notice role="status" className="mb-5">
              <span className="mr-3">{t(user.reauth ? "shell.limited.admin" : "shell.limited.employee")}</span>
              <Link className="font-medium whitespace-nowrap text-brand-600 underline-offset-2 hover:underline dark:text-brand-300" to={`/login?reauth=1&next=${encodeURIComponent(pathname + search)}`}>
                {t("shell.limited.action")}
              </Link>
            </Notice>
          )}
          <Outlet />
        </main>
        <footer className="mx-auto max-w-[1240px] px-4 pb-8 text-xs text-neutral-500 sm:px-8 lg:px-10" />
      </div>
    </div>
  );
}
