/**
 * 工作台的「开发」双卡：开发文档与开发样例的发现入口。
 *
 * 位置在权限申请卡之后；只放 3 条摘要，阅读在 /docs 与 /examples 两个固定页面里完成。
 * 权限用 portal.view（工作台本身就要求它）；没有权限时整段不渲染。
 */
import { Link } from "react-router";
import { useTranslation } from "react-i18next";
import { ArrowRight, BookMarked, FlaskConical } from "lucide-react";
import { useAuth } from "../lib/auth";
import { DEV_DOCS } from "../data/dev-docs";
import { DEV_EXAMPLES } from "../data/dev-examples";
import { pickLocalized } from "../lib/dev-docs-i18n";
import { isZh } from "../i18n";
import { Card, CardHeader } from "./ui";

const rowCls = "group flex items-center gap-3 border-t border-neutral-100 py-2.5 first:border-t-0 first:pt-1 dark:border-white/[0.07]";

export function DevCards({ className }: { className?: string }) {
  const { t, i18n } = useTranslation();
  const { can } = useAuth();
  if (!can("portal.view")) return null;
  const zh = isZh();
  const docs = DEV_DOCS.slice(0, 3);
  const examples = DEV_EXAMPLES.slice(0, 3);
  const moreCls = "font-medium text-brand-600 hover:text-brand-700 dark:text-brand-300 dark:hover:text-brand-200";

  return (
    <section aria-label={t("devdocs.cardsTitle")} className={className} data-dev-block="">
      {/* 移动端单列，sm 起两列等高 */}
      <div className="grid grid-cols-[minmax(0,1fr)] gap-4 sm:grid-cols-2">
        <Card className="flex flex-col" data-dev-card="docs">
          <CardHeader
            title={
              <span className="inline-flex items-center gap-2">
                <BookMarked size={16} className="text-brand-500" />
                {t("devdocs.cardDocs")}
              </span>
            }
            aside={
              <Link to="/docs" className={moreCls}>
                {t("devdocs.cardDocsMore", { count: DEV_DOCS.length })}
              </Link>
            }
          />
          <ul className="px-5 pb-4">
            {docs.map((doc) => (
              <li key={doc.slug}>
                <Link to={`/docs/${doc.slug}`} data-dev-doc={doc.slug} className={rowCls}>
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-sm font-medium text-neutral-900 group-hover:text-brand-700 dark:group-hover:text-brand-300">
                      {pickLocalized(doc.title, i18n.resolvedLanguage)}
                    </span>
                    <span className="mt-0.5 block truncate text-xs text-neutral-500">{pickLocalized(doc.summary, i18n.resolvedLanguage)}</span>
                  </span>
                  <ArrowRight size={14} className="shrink-0 text-neutral-300 transition-transform group-hover:translate-x-0.5 group-hover:text-brand-500" />
                </Link>
              </li>
            ))}
          </ul>
        </Card>

        <Card className="flex flex-col" data-dev-card="examples">
          <CardHeader
            title={
              <span className="inline-flex items-center gap-2">
                <FlaskConical size={16} className="text-brand-500" />
                {t("devdocs.cardExamples")}
              </span>
            }
            aside={
              <Link to="/examples" className={moreCls}>
                {t("devdocs.cardExamplesMore", { count: DEV_EXAMPLES.length })}
              </Link>
            }
          />
          <ul className="px-5 pb-4">
            {examples.map((example) => (
              <li key={example.id}>
                <Link to={`/examples/${example.id}`} data-dev-example={example.id} className={rowCls}>
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-sm font-medium text-neutral-900 group-hover:text-brand-700 dark:group-hover:text-brand-300">
                      {zh ? example.title.zh : example.title.en}
                    </span>
                    <span className="mt-0.5 block truncate text-xs text-neutral-500">
                      {t(`devdocs.difficultyLevels.${example.difficulty}`)} · {t(`devdocs.surfaceNames.${example.surfaces[0]}`)}
                    </span>
                  </span>
                  <ArrowRight size={14} className="shrink-0 text-neutral-300 transition-transform group-hover:translate-x-0.5 group-hover:text-brand-500" />
                </Link>
              </li>
            ))}
          </ul>
        </Card>
      </div>
    </section>
  );
}
