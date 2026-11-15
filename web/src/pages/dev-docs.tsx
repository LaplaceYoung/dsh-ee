/**
 * /docs：开发文档目录（固定路由，不随菜单 path 变化）。
 *
 * 内容优先来自构建期生成物 web/src/generated/dev-docs.ts（源是仓库 docs/，
 * 见 scripts/docs/build.mjs）；没有生成物时回退到旧静态数据，点开进入 /docs/:slug 阅读。
 * 目录支持按分类筛选 + 客户端搜索（标题/摘要/标签/正文，中文两字词直接子串匹配），
 * 搜索逻辑在 data/dev-docs-search.ts，⌘K 的「文档」分组共用同一套；不依赖任何后端搜索。
 * 侧栏与工作台卡片的入口也指向这里，但即使管理员改了菜单，URL 仍保持不变。
 */
import { useMemo, useState } from "react";
import { Link } from "react-router";
import { useTranslation } from "react-i18next";
import { ArrowRight, Search, X } from "lucide-react";
import { DOC_CATEGORIES, DEV_DOCS, readMinutes, type DocCategory } from "../data/dev-docs";
import { matchesDevDoc } from "../data/dev-docs-search";
import { isZh } from "../i18n";
import { Badge, Card, cardHover, cx, Empty, Notice, PageHeader, Segmented } from "../components/ui";
import { LangSwitch } from "../components/lang-switch";

type Filter = "all" | DocCategory;

export default function DevDocs() {
  const { t } = useTranslation();
  const [filter, setFilter] = useState<Filter>("all");
  const [query, setQuery] = useState("");
  const zh = isZh();
  const docs = useMemo(
    () => DEV_DOCS.filter((doc) => (filter === "all" || doc.category === filter) && matchesDevDoc(doc, query)),
    [filter, query],
  );

  return (
    <div className="max-w-4xl" data-dev-page="docs">
      {/* 文档语言与界面语言是同一个入口：切换后界面与文档列表标题/摘要一起换语言。 */}
      <PageHeader title={t("devdocs.title")} desc={t("devdocs.desc")} actions={<LangSwitch />} />

      <div className="mb-5 flex flex-col gap-3 sm:flex-row sm:flex-wrap sm:items-center">
        <div className="relative w-full sm:max-w-xs">
          <Search size={15} aria-hidden className="pointer-events-none absolute top-1/2 left-3 -translate-y-1/2 text-neutral-400" />
          <input
            type="search"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder={t("devdocs.searchPlaceholder")}
            aria-label={t("devdocs.searchPlaceholder")}
            className="h-9 w-full rounded-md border border-neutral-300 bg-card pr-9 pl-8 text-sm text-neutral-900 shadow-control outline-none transition-[border-color,box-shadow] duration-150 placeholder:text-neutral-400 hover:border-neutral-400/70 focus:border-brand-500 focus:ring-[3px] focus:ring-brand-500/20 dark:border-white/15 dark:placeholder:text-neutral-500"
          />
          {query && (
            <button
              type="button"
              aria-label={t("devdocs.clearSearch")}
              onClick={() => setQuery("")}
              className="absolute top-1/2 right-1.5 flex h-7 w-7 -translate-y-1/2 items-center justify-center rounded-md text-neutral-400 transition-colors hover:bg-neutral-100 hover:text-neutral-700 dark:hover:bg-white/10"
            >
              <X size={14} />
            </button>
          )}
        </div>
        <Segmented
          value={filter}
          onChange={setFilter}
          options={[
            { value: "all", label: t("devdocs.allCategories") },
            ...DOC_CATEGORIES.map((c) => ({ value: c, label: t(`devdocs.categories.${c}`) })),
          ]}
        />
      </div>

      {docs.length === 0 ? (
        <Empty icon={Search} title={t("devdocs.searchEmpty")} action={<button type="button" className="text-sm font-medium text-brand-600 hover:underline dark:text-brand-300" onClick={() => { setQuery(""); setFilter("all"); }}>{t("devdocs.clearSearch")}</button>} />
      ) : (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          {docs.map((doc) => (
            <Link
              key={doc.slug}
              to={`/docs/${doc.slug}`}
              data-dev-doc={doc.slug}
              className={cx("group block rounded-lg", cardHover)}
            >
              <Card className="flex h-full flex-col p-5">
                <div className="flex flex-wrap items-center gap-2">
                  <Badge tone="brand">{t(`devdocs.categories.${doc.category}`)}</Badge>
                  {doc.updatedAt && (
                    <span className="text-xs text-neutral-500 tabular-nums">{t("devdocs.updated", { date: doc.updatedAt })}</span>
                  )}
                </div>
                <h2 className="mt-2.5 text-[15px] leading-6 font-semibold text-neutral-900 group-hover:text-brand-700 dark:group-hover:text-brand-300">
                  {zh ? doc.title.zh : doc.title.en}
                </h2>
                <p className="mt-1 line-clamp-3 text-sm leading-6 text-neutral-600">{zh ? doc.summary.zh : doc.summary.en}</p>
                <div className="mt-auto flex items-center gap-1.5 pt-3 text-xs text-neutral-500">
                  <span>{t("devdocs.readMinutes", { count: readMinutes(doc.body) })}</span>
                  {!zh && <Badge tone="amber">{t("devdocs.zhOnly")}</Badge>}
                  <ArrowRight size={14} className="ml-auto text-neutral-300 transition-transform group-hover:translate-x-0.5 group-hover:text-brand-500" />
                </div>
              </Card>
            </Link>
          ))}
        </div>
      )}

      <Notice className="mt-6" tone="info">
        {t("devdocs.disclaimer")}
      </Notice>
    </div>
  );
}
