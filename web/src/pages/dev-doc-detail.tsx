/**
 * /docs/:slug：开发文档详情（固定路由）。
 *
 * 正文优先来自构建期生成物（源是仓库 docs/，见 scripts/docs/build.mjs），
 * 没有生成物时回退到旧静态数据；渲染见 components/docs-markdown.tsx。
 * 地址里的 #锚点 会在标题渲染出来后滚动定位；英文界面显示「正文为中文」的提示，不做机器翻译。
 */
import { Link, useParams } from "react-router";
import { useTranslation } from "react-i18next";
import { ArrowLeft, Copy, ExternalLink } from "lucide-react";
import { readMinutes, findDevDoc, sourceUrl } from "../data/dev-docs";
import { isZh } from "../i18n";
import { copyText } from "../lib/clipboard";
import { useStore } from "../lib/store";
import { DocsMarkdown } from "../components/docs-markdown";
import { Badge, Button, Card, Empty } from "../components/ui";
import { LangSwitch } from "../components/lang-switch";

export default function DevDocDetail() {
  const { t } = useTranslation();
  const { slug = "" } = useParams();
  const { notify } = useStore();
  const doc = findDevDoc(slug);
  const zh = isZh();

  /** 复制当前页地址（含 #锚点）；失败时提示，不谎报成功。 */
  const copyLink = async () => {
    if (!(await copyText(window.location.href))) return notify(t("common.copyFailed"));
    notify(t("devdocs.linkCopied"));
  };

  if (!doc) {
    return (
      <div className="max-w-4xl" data-dev-page="docs">
        <Empty
          title={t("devdocs.notFoundTitle")}
          action={
            <Link to="/docs" className="text-sm font-medium text-brand-600 hover:underline dark:text-brand-300">
              {t("devdocs.backToList")}
            </Link>
          }
        >
          {t("devdocs.notFoundHint")}
        </Empty>
      </div>
    );
  }

  return (
    <div className="max-w-4xl" data-dev-page="docs" data-dev-doc={doc.slug}>
      <div className="mb-4 flex items-center justify-between gap-3">
        <Link
          to="/docs"
          className="inline-flex items-center gap-1 rounded text-[13px] font-medium text-neutral-500 transition-colors hover:text-neutral-900"
        >
          <ArrowLeft size={14} />
          {t("devdocs.backToList")}
        </Link>
        {/* 文档正文只有中文；语言入口切换的是界面语言（英文界面会显示「正文为中文」提示）。 */}
        <LangSwitch />
      </div>

      <div className="mb-6">
        <div className="flex flex-wrap items-center gap-2">
          <Badge tone="brand">{t(`devdocs.categories.${doc.category}`)}</Badge>
          {doc.updatedAt && (
            <span className="text-xs text-neutral-500 tabular-nums">{t("devdocs.updated", { date: doc.updatedAt })}</span>
          )}
          <span className="text-xs text-neutral-500">{t("devdocs.readMinutes", { count: readMinutes(doc.body) })}</span>
          {!zh && <Badge tone="amber">{t("devdocs.zhOnly")}</Badge>}
        </div>
        <h1 className="mt-2 text-2xl leading-8 font-bold tracking-[-0.02em] text-neutral-900 sm:text-[28px] sm:leading-9">
          {zh ? doc.title.zh : doc.title.en}
        </h1>
        <p className="mt-2 max-w-3xl text-sm leading-6 text-neutral-600">{zh ? doc.summary.zh : doc.summary.en}</p>
        <div className="mt-3 flex flex-wrap items-center gap-3" data-dev-doc-actions>
          <Button variant="secondary" size="sm" onClick={copyLink}>
            <Copy size={13} />
            {t("devdocs.copyLink")}
          </Button>
          {doc.sourcePath && (
            <a
              href={sourceUrl(doc.sourcePath)}
              target="_blank"
              rel="noreferrer"
              data-dev-doc-source={doc.sourcePath}
              className="inline-flex items-center gap-1 text-sm font-medium text-brand-600 hover:underline dark:text-brand-300"
            >
              {t("devdocs.viewSource")}
              <ExternalLink size={13} />
            </a>
          )}
        </div>
      </div>

      <DocsMarkdown markdown={doc.body} />

      <Card className="mt-8 p-5">
        <div className="text-sm leading-6 text-neutral-600">{t("devdocs.disclaimer")}</div>
        <div className="mt-3 flex flex-wrap items-center gap-2">
          <Button variant="secondary" size="sm" onClick={() => window.scrollTo({ top: 0, behavior: "smooth" })}>
            {t("devdocs.backToTop")}
          </Button>
          <Link
            to="/examples"
            className="inline-flex items-center gap-1 text-sm font-medium text-brand-600 hover:underline dark:text-brand-300"
          >
            {t("devdocs.goExamples")}
            <ExternalLink size={13} />
          </Link>
        </div>
      </Card>
    </div>
  );
}
