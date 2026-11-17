/**
 * 全局搜索（⌘K / Ctrl+K）：搜页面（来自菜单）、开发文档（标题+摘要+标签+正文，见 data/dev-docs-search.ts）
 * 和页面里的设置项（lib/search-index.ts）。
 * 选中设置项会打开那个页面并定位、高亮那一节（见 useHashFocus）；选中「文档」进 /docs/:slug。
 */
import { useEffect, useState } from "react";
import { useNavigate } from "react-router";
import { useTranslation } from "react-i18next";
import { FileText, Settings2 } from "lucide-react";
import { useCommandState } from "cmdk";
import { DEV_DOCS } from "../data/dev-docs";
import { primeBodies, searchDevDocs } from "../data/dev-docs-search";
import { docLang, pickLocalized } from "../lib/dev-docs-i18n";
import { CommandDialog, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList } from "@/components/shadcn/command";
import { useMenus, useMenuTitle } from "../lib/menus";
import { MenuIcon } from "../lib/icons";
import { PAGE_META } from "../lib/page-meta";
import { SEARCH_ITEMS } from "../lib/search-index";

/** 全局快捷键：⌘K / Ctrl+K 打开或关上搜索。 */
export function useSearchShortcut(toggle: () => void) {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && !e.altKey && e.key.toLowerCase() === "k") {
        e.preventDefault();
        toggle();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [toggle]);
}

/**
 * ⌘K 里的「文档」分组：用站内文档的全文搜索（标题/摘要/标签/正文），
 * 只渲染当前查询命中的文档；keywords 传入查询本身，保证 cmdk 自己的过滤不会把命中的条目藏掉。
 * 打开面板时预热当前语言的正文 chunk：预热前正文按中文兜底，预热完成后重算，英文等正文也能命中。
 */
function DocsGroup({ onSelect }: { onSelect: (slug: string) => void }) {
  const { t, i18n } = useTranslation();
  const search = useCommandState((state) => state.search);
  const lang = docLang(i18n.resolvedLanguage);
  const [, setBodiesVersion] = useState(0);
  useEffect(() => {
    if (lang === "zh") return;
    let alive = true;
    primeBodies(lang).then((ok) => {
      if (alive && ok) setBodiesVersion((version) => version + 1);
    });
    return () => {
      alive = false;
    };
  }, [lang]);
  const hits = searchDevDocs(DEV_DOCS, search, 5, lang);
  if (!search.trim() || hits.length === 0) return null;
  return (
    <CommandGroup heading={t("nav.docs")}>
      {hits.map(({ doc, snippet }) => (
        <CommandItem
          key={`doc-${doc.slug}`}
          value={`docs ${doc.slug} ${pickLocalized(doc.title, lang)}`}
          keywords={[search]}
          onSelect={() => onSelect(doc.slug)}
          className="[&>svg:last-child]:hidden"
        >
          <FileText size={16} strokeWidth={1.6} className="shrink-0 text-neutral-400" />
          <span className="shrink-0 truncate">{pickLocalized(doc.title, lang)}</span>
          <span className="ml-auto min-w-0 truncate pl-3 text-xs text-neutral-500">
            {t(`devdocs.categories.${doc.category}`)} · {snippet}
          </span>
        </CommandItem>
      ))}
    </CommandGroup>
  );
}

export function CommandPalette({ open, onOpenChange }: { open: boolean; onOpenChange: (v: boolean) => void }) {
  const { t, i18n } = useTranslation();
  const { tree, pathOf, hasPage } = useMenus();
  const menuTitle = useMenuTitle();
  const navigate = useNavigate();
  const go = (to: string) => {
    onOpenChange(false);
    navigate(to);
  };

  const pages = tree.flatMap((n) =>
    n.kind === "group" ? (n.visible ? n.children.filter((c) => c.visible).map((c) => ({ m: c, group: menuTitle(n) })) : []) : n.visible ? [{ m: n, group: "" }] : [],
  ).filter(({ m }) => m.kind === "page");
  const desc = (key: string) => (i18n.exists(`pageDesc.${key}`) ? t(`pageDesc.${key}` as never) : PAGE_META[key]?.description ?? "");
  const items = SEARCH_ITEMS.filter((it) => it.page === "account" || hasPage(it.page));

  return (
    <CommandDialog open={open} onOpenChange={onOpenChange} title={t("nav.searchTitle")} description={t("nav.searchPlaceholder")} className="sm:max-w-xl">
      <CommandInput placeholder={t("nav.searchPlaceholder")} />
      <CommandList className="max-h-[60vh]">
        <CommandEmpty>{t("nav.empty")}</CommandEmpty>
        <CommandGroup heading={t("nav.pages")}>
          {pages.map(({ m, group }) => {
            const title = menuTitle(m);
            return (
              <CommandItem
                key={`page-${m.id}`}
                value={`page ${m.id} ${title}`}
                keywords={[group, desc(m.page_key), m.path, PAGE_META[m.page_key]?.title ?? ""]}
                onSelect={() => go(m.path)}
                className="[&>svg:last-child]:hidden"
              >
                <MenuIcon name={m.icon} className="text-neutral-400" />
                <span className="truncate">{title}</span>
                <span className="ml-auto truncate pl-3 text-xs text-neutral-500">{group}</span>
              </CommandItem>
            );
          })}
        </CommandGroup>
        <DocsGroup onSelect={(slug) => go(`/docs/${slug}`)} />
        <CommandGroup heading={t("nav.settings")}>
          {items.map((it) => {
            const title = t(`searchItems.${it.id}` as never);
            const pageTitle = it.page === "account" ? "" : t(`pages.${it.page}` as never);
            const to = (it.page === "account" ? "/account" : pathOf(it.page)) + (it.page === "account" ? "" : `#${it.id}`);
            return (
              <CommandItem key={it.id} value={`setting ${it.id} ${title}`} keywords={[...it.keywords, pageTitle]} onSelect={() => go(to)} className="[&>svg:last-child]:hidden">
                <Settings2 size={16} strokeWidth={1.6} className="text-neutral-400" />
                <span className="truncate">{title}</span>
                <span className="ml-auto truncate pl-3 text-xs text-neutral-500">{pageTitle}</span>
              </CommandItem>
            );
          })}
        </CommandGroup>
      </CommandList>
    </CommandDialog>
  );
}

/**
 * 地址带 #id 时，等那一节渲染出来（页面数据是异步加载的），滚到中间并闪一下高亮。
 */
export function useHashFocus(pathname: string, hash: string) {
  useEffect(() => {
    const id = decodeURIComponent(hash.replace(/^#/, ""));
    if (!id) return;
    let stopped = false;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const started = Date.now();
    const tryFocus = () => {
      if (stopped) return;
      const el = document.getElementById(id);
      if (!el) {
        if (Date.now() - started < 5000) timer = setTimeout(tryFocus, 120);
        return;
      }
      el.scrollIntoView({ block: "center", behavior: "smooth" });
      el.setAttribute("data-flash", "");
      timer = setTimeout(() => el.removeAttribute("data-flash"), 2400);
    };
    timer = setTimeout(tryFocus, 60);
    return () => {
      stopped = true;
      if (timer) clearTimeout(timer);
    };
  }, [pathname, hash]);
}
