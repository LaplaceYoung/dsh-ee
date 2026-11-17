/**
 * /docs/:slug：开发文档详情（固定路由）。
 *
 * 正文优先来自构建期生成物（源是仓库 docs/，见 scripts/docs/build.mjs）。
 * 中文正文在元数据里（同步渲染）；en / es / pt / hi 的正文按语言拆在独立 chunk 里，
 * 本页用 docBodyForAsync 懒加载（见 lib/dev-docs-i18n.ts），加载中显示轻量骨架，
 * 加载失败回退中文正文并把实际语言交给徽标（不谎报已有译文）。请求语言没有正文时
 * 按「请求语言 → en → zh」回退，标题旁显示正文语言徽标（不做机器翻译）。
 * 地址里的 #锚点 会在标题渲染出来后滚动定位；标题、摘要与正文都按界面语言取。
 */
import { useEffect, useState } from "react";
import { Link, useParams } from "react-router";
import { useTranslation } from "react-i18next";
import { ArrowLeft, Copy, ExternalLink } from "lucide-react";
import { readMinutes, findDevDoc, sourceUrl } from "../data/dev-docs";
import { docBodyForAsync, docLang, pickLocalized, type DocBody } from "../lib/dev-docs-i18n";
import { copyText } from "../lib/clipboard";
import { useStore } from "../lib/store";
import { DocsMarkdown } from "../components/docs-markdown";
import { DevDocBodyBadge } from "../components/dev-doc-body-badge";
import { Badge, Button, Card, Empty } from "../components/ui";
import { LangSwitch } from "../components/lang-switch";

export default function DevDocDetail() {
  const { t, i18n } = useTranslation();
  const { slug = "" } = useParams();
  const { notify } = useStore();
  const doc = findDevDoc(slug);
  const lang = docLang(i18n.resolvedLanguage);
  // zh 是源语言：正文就在元数据里，初始同步渲染，避免多一次骨架闪烁；其它语言等 chunk
  const [body, setBody] = useState<DocBody | null>(() =>
    doc && lang === "zh" ? { text: doc.body, lang: "zh", fallback: false } : null,
  );
  const [bodyFailed, setBodyFailed] = useState(false);

  useEffect(() => {
    if (!doc) return;
    let cancelled = false;
    setBodyFailed(false);
    if (lang === "zh") {
      setBody({ text: doc.body, lang: "zh", fallback: false });
      return;
    }
    // 切语言 / 换 slug 时先回到骨架，避免把上一种语言的正文当成新语言的
    setBody(null);
    docBodyForAsync(doc, lang)
      .then((next) => {
        if (!cancelled) setBody(next);
      })
      .catch(() => {
        // chunk 加载失败：正文回退中文（可用性优先），徽标据实际语言提示
        if (cancelled) return;
        setBodyFailed(true);
        setBody({ text: doc.body, lang: "zh", fallback: true });
      });
    return () => {
      cancelled = true;
    };
  }, [doc, lang]);

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
    <div
      className="max-w-4xl"
      data-dev-page="docs"
      data-dev-doc={doc.slug}
      data-dev-doc-body-state={body ? (bodyFailed ? "error" : "ready") : "loading"}
    >
      <div className="mb-4 flex items-center justify-between gap-3">
        <Link
          to="/docs"
          className="inline-flex items-center gap-1 rounded text-[13px] font-medium text-neutral-500 transition-colors hover:text-neutral-900"
        >
          <ArrowLeft size={14} />
          {t("devdocs.backToList")}
        </Link>
        {/* 文档标题/摘要与正文都按界面语言取；语言入口切换的是界面语言（正文按「请求语言 → en → zh」回退）。 */}
        <LangSwitch />
      </div>

      <div className="mb-6">
        <div className="flex flex-wrap items-center gap-2">
          <Badge tone="brand">{t(`devdocs.categories.${doc.category}`)}</Badge>
          {doc.updatedAt && (
            <span className="text-xs text-neutral-500 tabular-nums">{t("devdocs.updated", { date: doc.updatedAt })}</span>
          )}
          {body && <span className="text-xs text-neutral-500">{t("devdocs.readMinutes", { count: readMinutes(body.text, body.lang) })}</span>}
          {body && <DevDocBodyBadge doc={doc} lang={lang} bodyLang={body.lang} />}
        </div>
        <h1 className="mt-2 text-2xl leading-8 font-bold tracking-[-0.02em] text-neutral-900 sm:text-[28px] sm:leading-9">
          {pickLocalized(doc.title, lang)}
        </h1>
        <p className="mt-2 max-w-3xl text-sm leading-6 text-neutral-600">{pickLocalized(doc.summary, lang)}</p>
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

      {body ? (
        <DocsMarkdown markdown={body.text} />
      ) : (
        // 轻量骨架：标题/摘要已经同步显示，这里只占正文位置，避免整页空白（复用 common.loading）
        <div role="status" aria-live="polite" data-dev-doc-loading className="space-y-3 py-4">
          <div className="h-4 w-2/3 animate-pulse rounded bg-neutral-200/70 dark:bg-white/10" />
          <div className="h-4 w-full animate-pulse rounded bg-neutral-200/70 dark:bg-white/10" />
          <div className="h-4 w-5/6 animate-pulse rounded bg-neutral-200/70 dark:bg-white/10" />
          <div className="text-xs text-neutral-500">{t("common.loading")}</div>
        </div>
      )}

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
