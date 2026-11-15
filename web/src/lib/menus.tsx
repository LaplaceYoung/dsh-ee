/**
 * 当前用户可访问的菜单（来自后端，已按权限过滤）。路由、侧边栏和站内链接都从这里取，
 * 所以管理员改了页面的路由或名称，全站链接会跟着变。
 */
import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import { Link, type LinkProps } from "react-router";
import { api, type Menu } from "./api";
import { useAuth } from "./auth";
import { PAGE_META, type PageKey } from "./page-meta";
import { useTranslation } from "react-i18next";
import i18n, { LANGUAGES } from "../i18n";

export interface MenuNode extends Menu {
  children: Menu[];
}

interface Menus {
  menus: Menu[];
  /** 顶层项（分组带 children）。 */
  tree: MenuNode[];
  loading: boolean;
  error: string | null;
  reload: () => Promise<void>;
  /** page_key 当前配置的路由；菜单里没有时回退到默认路由。 */
  pathOf: (pageKey: string) => string;
  /** 登录后默认进入的页面：第一个可见页面。 */
  homePath: string | null;
  /** 菜单里有没有这个页面（有就说明当前用户能打开）。 */
  hasPage: (pageKey: string) => boolean;
}

const Ctx = createContext<Menus | null>(null);

const bySort = (a: Menu, b: Menu) => a.sort - b.sort || a.id - b.id;

export function buildTree(menus: Menu[]): MenuNode[] {
  const top = menus.filter((m) => m.parent_id === null).sort(bySort);
  return top.map((m) => ({ ...m, children: menus.filter((c) => c.parent_id === m.id).sort(bySort) }));
}

export function MenuProvider({ children }: { children: ReactNode }) {
  const { user } = useAuth();
  const [menus, setMenus] = useState<Menu[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const reload = useCallback(async () => {
    try {
      setMenus(await api.myMenus());
      setError(null);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  }, []);

  // 换了登录用户（或角色权限刷新后 user 对象变化）就重新拉菜单
  useEffect(() => {
    if (!user) {
      setMenus([]);
      setLoading(true);
      return;
    }
    void reload();
  }, [user, reload]);

  const value = useMemo<Menus>(() => {
    const tree = buildTree(menus);
    const pages = menus.filter((m) => m.kind === "page");
    const byKey = new Map<string, string>();
    for (const m of [...pages].sort(bySort)) if (!byKey.has(m.page_key)) byKey.set(m.page_key, m.path);
    const flatVisible = tree.flatMap((n) => (n.kind === "group" ? n.children : [n])).filter((m) => m.kind === "page" && m.visible);
    return {
      menus,
      tree,
      loading,
      error,
      reload,
      pathOf: (key) => byKey.get(key) ?? PAGE_META[key]?.defaultPath ?? "/",
      homePath: flatVisible[0]?.path ?? null,
      hasPage: (key) => byKey.has(key),
    };
  }, [menus, loading, error, reload]);

  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

export function useMenus() {
  const m = useContext(Ctx);
  if (!m) throw new Error("useMenus must be used inside MenuProvider");
  return m;
}

/**
 * 页面标题是不是「页面默认名称」：可能是中文默认值（旧数据 / zh 界面新建），
 * 也可能是当前或其它界面语言的页面名（新建时写的是当时界面语言的名称）。
 * 显示、编辑时按它判断要不要换成当前语言，避免把某一种语言写死。
 */
export function isDefaultPageTitle(pageKey: string, title: string): boolean {
  const meta = PAGE_META[pageKey];
  if (!meta) return false;
  if (title === meta.title) return true;
  return LANGUAGES.some(({ code }) => i18n.getFixedT(code)(`pages.${pageKey}` as never) === title);
}

/**
 * 菜单标题：还是默认值的页面与分组按当前语言显示；管理员改过的标题原样显示。
 */
export function useMenuTitle() {
  const { t } = useTranslation();
  return (m: Pick<Menu, "kind" | "title" | "page_key">) => {
    if (m.kind === "page" && isDefaultPageTitle(m.page_key, m.title)) return t(`pages.${m.page_key as PageKey}`);
    if (m.kind === "group" && m.title in DEFAULT_GROUPS) return t(`menuGroups.${m.title as keyof typeof DEFAULT_GROUPS}`);
    return m.title;
  };
}

const DEFAULT_GROUPS = { 用: 1, 造: 1, 接入: 1, 开发: 1, 工作台: 1, 账号与模型: 1, 管理: 1, 运营: 1, 账号与权限: 1, 模型与额度: 1, 能力与连接: 1, 分发与源: 1, 系统: 1 };

/** 按 page_key 链接站内页面；suffix 用于带查询参数等，例如 "?tab=x"。 */
export function PageLink({ page, suffix = "", ...rest }: Omit<LinkProps, "to"> & { page: string; suffix?: string }) {
  const { pathOf } = useMenus();
  return <Link to={pathOf(page) + suffix} {...rest} />;
}
