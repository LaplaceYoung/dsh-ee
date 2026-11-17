/**
 * 正文语言徽标：当前界面语言还没有正文、正文回退到中文时，提示「正文为中文」。
 *
 * 正文按语言懒加载（见 lib/dev-docs-i18n.ts）：列表页只读同步的 hasBody 元数据（不加载 chunk）；
 * 详情页在正文加载完成后把实际拿到的语言（bodyLang）传进来，避免元数据与 chunk 内容不一致时误报。
 * 有请求语言的正文时不渲染任何东西。
 *
 * 只处理「回退到中文」这一种情况，复用既有的 devdocs.zhOnly key；**不新增 i18n key**：
 * es / pt / hi 语言包由独立 PR 提供，这里如果引用新 key 会让两个 PR 互相依赖。
 * 「es 界面回退英文正文」理论上存在（某篇缺 es 译文但有 en 译文），本 rebase 接入译文的 8 篇
 * 四语正文齐全，不会触发；真出现时不显示徽标，正文仍按回退链正常渲染。
 */
import { useTranslation } from "react-i18next";
import type { DevDoc, DocLang } from "../data/dev-docs-types";
import { docLang, fallbackBodyLang, hasBodyFor } from "../lib/dev-docs-i18n";
import { Badge } from "./ui";

export function DevDocBodyBadge({ doc, lang, bodyLang }: { doc: DevDoc; lang: string; bodyLang?: DocLang }) {
  const { t } = useTranslation();
  const requested = docLang(lang);
  const actual = bodyLang ?? (hasBodyFor(doc, requested) ? requested : fallbackBodyLang(doc, requested));
  if (actual !== "zh" || requested === "zh") return null;
  return <Badge tone="amber">{t("devdocs.zhOnly")}</Badge>;
}
