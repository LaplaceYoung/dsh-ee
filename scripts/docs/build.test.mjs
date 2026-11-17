// 站内开发文档管线（docs/ 单一事实源）的测试：
// - 生成器幂等、白名单、链接改写、坏 front matter 失败、没有 docs/ 时兜底；
// - 生成物拆分：主模块只带元数据 + 中文正文，en/es/pt/hi 各一个独立正文模块（按语言懒加载），
//   主模块用 hasBody 标记译文；llms.txt 保持不变；
// - 正文回退链（请求语言 → en → zh）、zh 同步取正文、其它语言动态 import 并按语言缓存；
// - --check 能发现生成物（含 4 个正文模块）与 docs/ 不同步；
// - 页面数据优先用生成物、生成物为空时回退旧静态数据；搜索预热后能命中译文正文。
//
// 只用 Node 内置模块；页面侧选择函数是纯 TS，Node 22.18+/24 可以按类型擦除直接 import。
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import { test } from "node:test";

const HERE = dirname(fileURLToPath(import.meta.url));
const BUILD = join(HERE, "build.mjs");
const SELECT_MODULE = pathToFileURL(join(HERE, "..", "..", "web", "src", "data", "dev-docs-select.ts")).href;

/** 译文正文的语言（中文是源语言，正文在 body / markdown 里）。 */
const BODY_LANGS = ["en", "es", "pt", "hi"];

/**
 * 已接入 en / es / pt / hi 四语正文的文档（#185 的白名单）。
 * main 上后续新增的站内文档（deepseek-tools / thinking-mode / responses-api / chatbot /
 * beta-completions）暂时只有中文正文与英文字段，属于渐进补译：不参与「四语齐全」断言，
 * 但仍要保证生成物写全五键、正文模块里没有它们的条目。
 */
const TRANSLATED_SLUGS = [
  "overview",
  "harness-quickstart",
  "anthropic-api",
  "plugin-quickstart",
  "extension-quickstart",
  "access-rules",
  "publish-precheck",
  "quality-standards",
];

/** 去掉代码围栏内容，避免把示例里的 ./ 当成相对链接（与 scripts/check-docs.mjs 的 stripCode 同思路）。 */
function stripFences(text) {
  return text.replace(/```[\s\S]*?```/g, "").replace(/~~~[\s\S]*?~~~/g, "");
}

function makeTmpRoot() {
  return mkdtempSync(join(tmpdir(), "dsh-docs-pipeline-"));
}

function writeFiles(root, files) {
  for (const [rel, content] of Object.entries(files)) {
    const file = join(root, rel);
    mkdirSync(dirname(file), { recursive: true });
    writeFileSync(file, content);
  }
}

/** 临时目录里的标准参数：自定义 docs/manifest/out/llms-out，避免碰仓库里的真实生成物。 */
function argsFor(root, extra = []) {
  return [
    "--docs-dir",
    join(root, "docs"),
    "--manifest",
    join(root, "manifest.json"),
    "--out",
    join(root, "generated", "dev-docs.ts"),
    "--llms-out",
    join(root, "generated", "llms.txt"),
    ...extra,
  ];
}

function run(args) {
  return spawnSync(process.execPath, [BUILD, ...args], { encoding: "utf8" });
}

const FRONT_MATTER_A = [
  "---",
  "slug: a-doc",
  "category: plugin",
  'title: "A 标题"',
  'title_en: "A title"',
  'summary: "A 摘要"',
  'summary_en: "A summary"',
  "tags: [一, 二]",
  "updated_at: 2026-01-02",
  "---",
  "",
  "## 小节",
  "",
  "正文 A。",
  "",
].join("\n");

test("生成幂等：同一输入两次生成同一份产物，白名单外文件不生成", () => {
  const root = makeTmpRoot();
  try {
    writeFiles(root, {
      "docs/dev/a.md": FRONT_MATTER_A,
      // 没有 front matter：slug 取文件名，标题取第一个标题，摘要取第一段，分类用 defaultCategory
      "docs/dev/b.md": "# B 的标题\n\nB 的第一段正文。\n\n## 后文\n",
      "docs/dev/c.md": "SHOULD_NOT_APPEAR：白名单里没有这篇。\n",
      "manifest.json": JSON.stringify({ defaultCategory: "start", docs: ["dev/a.md", "dev/b.md"] }),
    });
    const out = join(root, "generated", "dev-docs.ts");
    const bodyFiles = BODY_LANGS.map((lang) => join(root, "generated", `dev-docs-bodies.${lang}.ts`));
    const readAll = () => [out, ...bodyFiles].map((file) => readFileSync(file, "utf8"));
    const first = run(argsFor(root));
    assert.equal(first.status, 0, `生成失败：${first.stderr}`);
    const firstContents = readAll();
    const firstContent = firstContents[0];
    const second = run(argsFor(root));
    assert.equal(second.status, 0, `二次生成失败：${second.stderr}`);
    assert.deepEqual(readAll(), firstContents, "两次生成的 5 个生成物必须字节一致");

    assert.match(firstContent, /slug: "a-doc"/);
    // 生成物始终写全五语键：fixture 只有 zh/en，es/pt/hi 在生成时回退到 en
    assert.match(firstContent, /title: \{ zh: "A 标题", en: "A title", es: "A title", pt: "A title", hi: "A title" \}/);
    assert.match(firstContent, /tags: \["一", "二"\]/);
    assert.match(firstContent, /updated_at: "2026-01-02"/);
    assert.match(firstContent, /markdown: "## 小节\\n\\n正文 A。\\n"/);
    // 译文正文已从主生成物拆走：没有 bodies 字段；fixture 没有译文 → 不写 hasBody
    assert.ok(!firstContent.includes("bodies:"), "主生成物不应再带译文 bodies 字段");
    assert.ok(!firstContent.includes("hasBody:"), "没有译文时不应写 hasBody 标记");
    // 4 个语言正文模块始终存在（页面侧是显式 import()）：没有译文时是空对象
    for (const [index, lang] of BODY_LANGS.entries()) {
      assert.match(firstContents[index + 1], /^\/\/ 本文件由 scripts\/docs\/build\.mjs 生成/m, `${lang} 正文模块应有生成头注释`);
      assert.match(firstContents[index + 1], /export default \{\};/, `${lang} 没有译文时应生成空对象（否则 import() 解析失败）`);
    }
    // 没有 front matter 的 b.md：默认值（en 回退 zh，es/pt/hi 再回退 en）
    assert.match(firstContent, /slug: "b"/);
    assert.match(firstContent, /category: "start"/);
    assert.match(firstContent, /title: \{ zh: "B 的标题", en: "B 的标题", es: "B 的标题", pt: "B 的标题", hi: "B 的标题" \}/);
    assert.match(
      firstContent,
      /summary: \{ zh: "B 的第一段正文。", en: "B 的第一段正文。", es: "B 的第一段正文。", pt: "B 的第一段正文。", hi: "B 的第一段正文。" \}/,
    );
    assert.match(firstContent, /updated_at: ""/);
    assert.ok(firstContent.indexOf('slug: "a-doc"') < firstContent.indexOf('slug: "b"'), "顺序应与白名单一致");
    assert.ok(!firstContent.includes("SHOULD_NOT_APPEAR"), "白名单外的文件不应出现在生成物里");
    assert.match(firstContent, /export const DOCS_REPO_BLOB = "https:\/\/github\.com\/hikariming\/dsh-ee\/blob\/main";/);
    assert.match(firstContent, /source: "dev\/a\.md"/);

    // llms.txt：给 Agent 的入口，同样重复生成字节一致，且不因正文拆分而变化
    const llmsFile = join(root, "generated", "llms.txt");
    const firstLlms = readFileSync(llmsFile, "utf8");
    assert.match(firstLlms, /^# dsh-ee 开发文档/);
    assert.ok(firstLlms.includes("- [A 标题 / A title](/docs/a-doc): A 摘要"), `llms.txt 缺少文档条目：${firstLlms}`);
    assert.ok(firstLlms.includes("/tree/main/docs"), "llms.txt 缺少仓库入口");
    assert.equal(readFileSync(llmsFile, "utf8"), firstLlms, "两次生成的 llms.txt 必须字节一致");
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("五语元数据：缺译按「该语言 → en → zh」回退，生成物始终写全五键", () => {
  const root = makeTmpRoot();
  try {
    writeFiles(root, {
      // 有 es、缺 pt/hi：pt/hi 回退 en；有完整 summary 五语
      "docs/dev/multi.md": [
        "---",
        "slug: multi",
        "title: 中文标题",
        'title_en: "English title"',
        'title_es: "Título en español"',
        'summary: "中文摘要"',
        'summary_en: "English summary"',
        'summary_pt: "Resumo em português"',
        "---",
        "",
        "正文。",
        "",
      ].join("\n"),
      // 没有 en：en 回退 zh，es/pt/hi 未译也回退 en（此时即 zh）
      "docs/dev/zh-only.md": [
        "---",
        "slug: zh-only",
        "title: 只有中文",
        "summary: 只有中文摘要",
        "---",
        "",
        "正文。",
        "",
      ].join("\n"),
      "manifest.json": JSON.stringify({ docs: ["dev/multi.md", "dev/zh-only.md"] }),
    });
    const result = run(argsFor(root));
    assert.equal(result.status, 0, `生成失败：${result.stderr}`);
    const generated = readFileSync(join(root, "generated", "dev-docs.ts"), "utf8");

    assert.match(
      generated,
      /title: \{ zh: "中文标题", en: "English title", es: "Título en español", pt: "English title", hi: "English title" \}/,
      "es 用译文，缺译的 pt/hi 回退英文",
    );
    assert.match(
      generated,
      /summary: \{ zh: "中文摘要", en: "English summary", es: "English summary", pt: "Resumo em português", hi: "English summary" \}/,
      "缺译的 es/hi 回退英文，pt 用译文",
    );
    assert.match(generated, /title: \{ zh: "只有中文", en: "只有中文", es: "只有中文", pt: "只有中文", hi: "只有中文" \}/);

    // llms.txt 仍是「中文 / English」的稳定标题行，不随五语扩展改变（e2e 用它反推篇数）
    const llms = readFileSync(join(root, "generated", "llms.txt"), "utf8");
    assert.ok(llms.includes("- [中文标题 / English title](/docs/multi): 中文摘要"), `llms.txt 标题行不应变化：${llms}`);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("译文正文：按 docs/<lang>/<entry> 拆到每语言模块，缺译不报错，站内链接改写幂等", async () => {
  const root = makeTmpRoot();
  try {
    writeFiles(root, {
      "docs/dev/a.md": FRONT_MATTER_A,
      "docs/dev/b.md": "---\nslug: b\n---\n\n## 小节\n\nB 正文。\n",
      // 译文没有 front matter、首行是 H1；站内链接已写成 /docs/<slug>，
      // 这里再混一个包内相对链接，确认生成器仍会跑一次改写（对 /docs 链接是幂等的）。
      "docs/en/dev/a.md": "# A title\n\nSee [B](./b.md#小节) and [B again](/docs/b).\n",
      "docs/es/dev/a.md": "# Un título\n\nTexto en español.\n",
      "docs/pt/dev/a.md": "", // 空文件按缺译处理，不报错
      "manifest.json": JSON.stringify({ docs: ["dev/a.md", "dev/b.md"] }),
    });
    const out = join(root, "generated", "dev-docs.ts");
    const result = run(argsFor(root));
    assert.equal(result.status, 0, `缺 pt/hi 译文不应让生成失败：${result.stderr}`);
    // 覆盖率写进日志：CI 里能看出哪篇还缺哪种语言
    assert.match(result.stdout, /译文正文覆盖 en 1\/2 · es 1\/2 · pt 0\/2 · hi 0\/2/);

    // 元数据：只带 hasBody 标记，正文已拆到每语言模块
    const { GENERATED_DEV_DOCS } = await import(pathToFileURL(out).href);
    const a = GENERATED_DEV_DOCS.find((doc) => doc.slug === "a-doc");
    const b = GENERATED_DEV_DOCS.find((doc) => doc.slug === "b");
    assert.deepEqual(a.hasBody, { en: true, es: true }, "hasBody 只标实际存在的语言（空文件不算）");
    assert.equal(b.hasBody, undefined, "没有任何译文时省略 hasBody");
    assert.ok(!("bodies" in a), "主生成物不应再带 bodies 字段");

    const bodiesOf = async (lang) =>
      (await import(pathToFileURL(join(root, "generated", `dev-docs-bodies.${lang}.ts`)).href)).default;
    const en = await bodiesOf("en");
    const es = await bodiesOf("es");
    const pt = await bodiesOf("pt");
    const hi = await bodiesOf("hi");
    assert.deepEqual(Object.keys(en), ["a-doc"], "en 模块只收该语言实际有译文的篇目");
    assert.equal(pt["a-doc"], undefined, "空文件按缺译处理");
    assert.deepEqual(hi, {}, "没有任何译文的语言模块是空对象");
    assert.match(en["a-doc"], /^# A title\n/, "译文正文原样保留（首行 H1 不动）");
    assert.ok(en["a-doc"].includes("](/docs/b#小节)"), "包内相对链接应改写成站内路由");
    assert.ok(en["a-doc"].includes("](/docs/b)"), "已经是 /docs 的链接保持不变（改写幂等）");
    assert.equal(es["a-doc"], "# Un título\n\nTexto en español.\n");
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("正文回退链：请求语言 → en → zh；zh 同步取元数据，译文按语言动态 import", async () => {
  const helperUrl = new URL("../../web/src/lib/dev-docs-i18n.ts", import.meta.url).href;
  const generatedUrl = new URL("../../web/src/generated/dev-docs.ts", import.meta.url).href;
  const selectUrl = new URL("../../web/src/data/dev-docs-select.ts", import.meta.url).href;
  const { bodyLangChain, docBodyForAsync, hasBodyFor, loadDocBody, loadDocBodies, pickDocBody } = await import(helperUrl);
  const { GENERATED_DEV_DOCS } = await import(generatedUrl);
  const { fromGeneratedDoc } = await import(selectUrl);
  const docs = GENERATED_DEV_DOCS.map(fromGeneratedDoc);
  const doc = {
    slug: "fallback-probe",
    category: "start",
    title: { zh: "中文标题", en: "English title" },
    summary: { zh: "中文摘要", en: "English summary" },
    body: "# 中文正文\n",
    updatedAt: "",
    tags: [],
  };
  // 没有 hasBody（旧静态兜底数据）：任何语言都回退中文正文；徽标判断同步、不用加载 chunk
  for (const lang of BODY_LANGS) {
    assert.equal(hasBodyFor(doc, lang), false, `${lang} 没有译文标记`);
    assert.deepEqual(pickDocBody(doc, lang), { text: "# 中文正文\n", lang: "zh", fallback: true }, `${lang} 应回退中文正文`);
  }
  assert.equal(hasBodyFor(doc, "zh-CN"), true);
  assert.equal(hasBodyFor(doc, "zh-TW"), true);
  // 元数据说有译文、但 chunk 还没加载：pickDocBody 只认已加载的正文，先回退中文（搜索的兜底语义）
  const enOnly = { ...doc, hasBody: { en: true } };
  assert.deepEqual(pickDocBody(enOnly, "en"), { text: "# 中文正文\n", lang: "zh", fallback: true }, "chunk 未加载时不会假装有英文正文");
  // 已加载 en 正文：en 命中、其它语言回退 en（不是直接回中文）
  const enBodies = { en: { "fallback-probe": "# English body\n" } };
  assert.deepEqual(pickDocBody(enOnly, "en", enBodies), { text: "# English body\n", lang: "en", fallback: false });
  assert.deepEqual(pickDocBody(enOnly, "pt-BR", enBodies), { text: "# English body\n", lang: "en", fallback: true });
  assert.deepEqual(
    pickDocBody({ ...doc, hasBody: { en: true, es: true } }, "es-MX", { ...enBodies, es: { "fallback-probe": "# Cuerpo en español\n" } }),
    { text: "# Cuerpo en español\n", lang: "es", fallback: false },
  );
  // 真实生成物：正文模块能被 Node 直接动态 import（页面里 Vite 同样按语言拆 chunk）；
  // zh 在元数据里，不触发任何正文模块。
  const overview = docs.find((item) => item.slug === "overview");
  assert.deepEqual(bodyLangChain("es-ES"), ["es", "en", "zh"]);
  assert.deepEqual(bodyLangChain("en-GB"), ["en", "zh"]);
  assert.deepEqual(bodyLangChain("zh-CN"), ["zh"]);
  assert.deepEqual(await docBodyForAsync(overview, "zh-CN"), { text: overview.body, lang: "zh", fallback: false });
  const enBody = await docBodyForAsync(overview, "en");
  assert.equal(enBody.lang, "en");
  assert.equal(enBody.fallback, false);
  assert.match(enBody.text, /^# Where to start: three development paths/);
  const ptBody = await docBodyForAsync(overview, "pt-BR");
  assert.equal(ptBody.lang, "pt");
  assert.equal(ptBody.fallback, false);
  // 元数据标了有译文、但模块里没有该 slug（不新鲜/异常数据）时仍按回退链落到中文
  assert.deepEqual(await docBodyForAsync({ ...doc, hasBody: { en: true } }, "en"), { text: "# 中文正文\n", lang: "zh", fallback: true });
  // 单个语言加载器：只返回该语言实际正文，不拼中文回退；zh 返回空对象
  const en = await loadDocBodies("en-US");
  assert.equal(
    Object.keys(en).length,
    docs.filter((item) => item.hasBody?.en).length,
    "en 正文模块应收录所有标了 en 正文的文档",
  );
  assert.deepEqual(await loadDocBodies("zh"), {}, "zh 正文在元数据里，不单独成 chunk");
  assert.equal((await loadDocBody("overview", "en"))?.lang, "en");
  assert.equal(await loadDocBody("overview", "zh"), null, "zh 不走译文模块");
  assert.equal(await loadDocBody("not-a-doc", "en"), null, "没有的 slug 返回 null");
  // 同一语言重复加载走模块级 Promise 缓存（不会重复解析）
  assert.equal(await loadDocBodies("en"), en);
});

test("仓库里的译文正文：8 篇 × en/es/pt/hi 全在各自模块里，首行 H1 与标题一致，无相对链接", async () => {
  const generatedUrl = new URL("../../web/src/generated/dev-docs.ts", import.meta.url).href;
  const { GENERATED_DEV_DOCS } = await import(generatedUrl);
  const translated = GENERATED_DEV_DOCS.filter((doc) => TRANSLATED_SLUGS.includes(doc.slug));
  assert.equal(translated.length, TRANSLATED_SLUGS.length, "生成物应包含全部已接入译文的 8 篇文档");
  const slugs = translated.map((doc) => doc.slug).sort();
  const bodiesOf = async (lang) =>
    (await import(new URL(`../../web/src/generated/dev-docs-bodies.${lang}.ts`, import.meta.url).href)).default;

  for (const doc of translated) {
    assert.deepEqual(
      Object.keys(doc.hasBody ?? {}).sort(),
      [...BODY_LANGS].sort(),
      `${doc.slug} 应标记 en/es/pt/hi 四语正文（正文接入后不允许再缺译）`,
    );
    assert.ok(!("bodies" in doc), `${doc.slug} 的主生成物记录不应再带正文`);
  }
  // main 上新增的站内文档还没接入译文：不标 hasBody，正文模块里也不该有它们的条目
  for (const doc of GENERATED_DEV_DOCS.filter((item) => !TRANSLATED_SLUGS.includes(item.slug))) {
    assert.equal(doc.hasBody, undefined, `${doc.slug} 还没有译文，不应标 hasBody`);
  }
  for (const lang of BODY_LANGS) {
    const bodies = await bodiesOf(lang);
    // 译文模块只收已接入译文的文档；main 新增文档四语缺译时回退 en → zh，不写空条目
    assert.deepEqual(Object.keys(bodies).sort(), slugs, `${lang} 正文模块应含全部 8 篇已译文档`);
    for (const doc of translated) {
      const text = bodies[doc.slug];
      assert.ok(text && text.trim(), `${doc.slug} 的 ${lang} 正文不能为空`);
      // 译文的站内链接已写成 /docs/<slug>；模块里不应出现 ./ 或 ../ 相对链接
      assert.doesNotMatch(text, /\]\(\.{1,2}\//, `${doc.slug} 的 ${lang} 正文不应有相对链接`);
      assert.match(text, /^# /, `${doc.slug} 的 ${lang} 正文应以 H1 开头`);
    }
  }

  // 抽查 2 篇：译文正文首行 H1 与 front matter 里的 title_<lang> 一致（防止标题与正文错位）
  const overview = GENERATED_DEV_DOCS.find((doc) => doc.slug === "overview");
  assert.equal((await bodiesOf("en")).overview.split("\n")[0], `# ${overview.title.en}`, "overview 的英文正文首行 H1 应等于 title.en");
  const quality = GENERATED_DEV_DOCS.find((doc) => doc.slug === "quality-standards");
  assert.equal((await bodiesOf("hi"))["quality-standards"].split("\n")[0], `# ${quality.title.hi}`, "quality-standards 的印地语正文首行 H1 应等于 title.hi");

  // 源文件层面再查一遍：生成物的 link 改写会掩盖 ./x.md，所以对 32 篇译文源码直接断言（跳过代码块）
  const docsDir = join(HERE, "..", "..", "docs");
  for (const doc of translated) {
    for (const lang of BODY_LANGS) {
      const text = stripFences(readFileSync(join(docsDir, lang, doc.source), "utf8"));
      assert.doesNotMatch(text, /\]\(\.{1,2}\//, `docs/${lang}/${doc.source} 在代码块外不应有相对链接`);
    }
  }
});

test("生成物拆分：主生成物只带元数据 + 中文正文，4 个译文模块独立，llms.txt 不变", () => {
  const generatedDir = new URL("../../web/src/generated/", import.meta.url);
  const metadataFile = fileURLToPath(new URL("dev-docs.ts", generatedDir));
  const metadata = readFileSync(metadataFile, "utf8");
  assert.ok(!metadata.includes("bodies:"), "主生成物不应再有 bodies 字段");
  assert.match(metadata, /hasBody: \{ en: true, es: true, pt: true, hi: true \}/, "元数据用 hasBody 标记四语译文");
  // 体积断言：主生成物只含元数据 + 中文正文；四语正文都在 4 个按需模块里
  // 13 篇中文正文约 13 万字节；四语译文合计约 30 万字节都在按需模块里，主包不再带它们
  assert.ok(metadata.length < 160_000, `主生成物应只含元数据 + 中文正文，实际 ${metadata.length} 字节`);
  let bodiesTotal = 0;
  for (const lang of BODY_LANGS) {
    const content = readFileSync(fileURLToPath(new URL(`dev-docs-bodies.${lang}.ts`, generatedDir)), "utf8");
    bodiesTotal += content.length;
    assert.ok(content.length > 10_000, `${lang} 译文模块应有实际正文，实际 ${content.length} 字节`);
    assert.match(content, /用显式 import\(\) 按语言懒加载/, `${lang} 模块头注释应说明按语言懒加载`);
  }
  assert.ok(bodiesTotal > 200_000, `四语正文应都在按需模块里，实际合计 ${bodiesTotal} 字节`);
  // 抽查：英文 / 印地语 H1 只在各自模块里，不再出现在主生成物里
  const en = readFileSync(fileURLToPath(new URL("dev-docs-bodies.en.ts", generatedDir)), "utf8");
  const hi = readFileSync(fileURLToPath(new URL("dev-docs-bodies.hi.ts", generatedDir)), "utf8");
  const overviewEnH1 = "# Where to start: three development paths";
  const qualityHiH1 = "# मानक और जाँच: ENTRY / FLOW / BOOST";
  assert.ok(en.includes(overviewEnH1), "英文模块应含 overview 的英文 H1");
  assert.ok(hi.includes(qualityHiH1), "印地语模块应含 quality-standards 的印地语 H1");
  assert.ok(!metadata.includes(overviewEnH1), "英文正文不应再进主生成物");
  assert.ok(!metadata.includes(qualityHiH1), "印地语正文不应再进主生成物");
  // llms.txt 是给 Agent 的稳定契约：只有文档清单 + 契约入口，不因正文拆分而变化（也不含正文）
  const llms = readFileSync(new URL("../../web/public/llms.txt", import.meta.url), "utf8");
  assert.match(llms, /^# dsh-ee 开发文档/);
  assert.ok(!llms.includes(overviewEnH1), "llms.txt 不应包含译文正文");
  const listed = new Set([...llms.matchAll(/\]\(\/docs\/([a-z0-9-]+)\)/g)].map((match) => match[1]));
  const manifest = JSON.parse(readFileSync(join(HERE, "manifest.json"), "utf8"));
  assert.equal(listed.size, manifest.docs.length, "llms.txt 应列出发布白名单里的全部站内文档");
});

test("发布集内的相对链接改写成站内路由，其它链接保持原样", () => {
  const root = makeTmpRoot();
  try {
    writeFiles(root, {
      "docs/dev/a.md": [
        "---",
        "slug: a-doc",
        "title: A",
        "summary: A",
        "---",
        "",
        "[去 B](./b.md#小节) [未发布 C](./c.md) [外部](https://example.com/x) [站内绝对](/docs/keep) ![图](./pic.png)",
        "",
      ].join("\n"),
      "docs/dev/b.md": "---\nslug: b\n---\n\n## 小节\n\nB 正文。\n",
      "docs/dev/c.md": "---\nslug: c\n---\n\nC 正文（只在仓库里，不进站内）。\n",
      // 译文正文也跑同一套改写：确保按语言拆到模块后仍改写
      "docs/en/dev/a.md": "[去 B](./b.md#小节) [未发布 C](./c.md)\n",
      "manifest.json": JSON.stringify({ docs: ["dev/a.md", "dev/b.md"] }),
    });
    const result = run(argsFor(root));
    assert.equal(result.status, 0, `生成失败：${result.stderr}`);
    const generated = readFileSync(join(root, "generated", "dev-docs.ts"), "utf8");
    assert.ok(generated.includes("/docs/b#小节"), "发布集内链接应改写成 /docs/<slug>");
    assert.ok(generated.includes("](./c.md)"), "未发布文档的相对链接应保持原样");
    assert.ok(generated.includes("https://example.com/x"), "外部链接应保持原样");
    assert.ok(generated.includes("](/docs/keep)"), "站内绝对链接应保持原样");
    assert.ok(generated.includes("](./pic.png)"), "非 Markdown 的相对链接应保持原样");
    const enBodies = readFileSync(join(root, "generated", "dev-docs-bodies.en.ts"), "utf8");
    assert.ok(enBodies.includes("/docs/b#小节"), "译文正文的包内相对链接也应改写成站内路由");
    assert.ok(enBodies.includes("](./c.md)"), "译文正文里未发布文档的相对链接应保持原样");
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("坏 front matter 失败并报出文件和行号", () => {
  const cases = [
    {
      name: "缺少结束分隔线",
      file: ["---", "title: 没有结束"].join("\n"),
      expected: /a\.md: front matter 缺少结束分隔线/,
    },
    {
      name: "行格式错误",
      file: ["---", "title: ok", "这不是键值对", "---", "", "正文"].join("\n"),
      expected: /a\.md:3: front matter 行格式错误/,
    },
    {
      name: "分类不在枚举内",
      file: ["---", "slug: bad-doc", "category: 不存在的分类", "title: T", "---", "", "正文"].join("\n"),
      expected: /a\.md: 分类|category「不存在的分类」/,
    },
    {
      name: "updated_at 不是日期",
      file: ["---", "title: T", "updated_at: 2026-13-40", "---", "", "正文"].join("\n"),
      expected: /a\.md: updated_at「2026-13-40」/,
    },
  ];
  for (const item of cases) {
    const root = makeTmpRoot();
    try {
      writeFiles(root, {
        "docs/dev/a.md": item.file,
        "manifest.json": JSON.stringify({ docs: ["dev/a.md"] }),
      });
      const result = run(argsFor(root));
      assert.equal(result.status, 1, `${item.name}：应以退出码 1 失败`);
      assert.match(result.stderr, item.expected, `${item.name}：报错应指出文件与原因`);
    } finally {
      rmSync(root, { recursive: true, force: true });
    }
  }

  const root = makeTmpRoot();
  try {
    writeFiles(root, {
      "docs/dev/.keep": "",
      "manifest.json": JSON.stringify({ docs: ["dev/missing.md"] }),
    });
    const result = run(argsFor(root));
    assert.equal(result.status, 1, "白名单里的文件不存在应失败");
    assert.match(result.stderr, /missing\.md/);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("没有 docs/ 时兜底：保留已有生成物，没有生成物时写空数组 + 4 个空译文模块", () => {
  const root = makeTmpRoot();
  try {
    const out = join(root, "generated", "dev-docs.ts");
    writeFiles(root, {
      "generated/dev-docs.ts": "// SENTINEL：已有生成物\n",
      "generated/dev-docs-bodies.en.ts": "// SENTINEL：已有英文模块\n",
    });
    const kept = run(argsFor(root));
    assert.equal(kept.status, 0, `没有 docs/ 不应让构建失败：${kept.stderr}`);
    assert.match(kept.stderr, /回退旧静态数据/);
    assert.equal(readFileSync(out, "utf8"), "// SENTINEL：已有生成物\n", "已有生成物不应被覆盖");
    assert.equal(readFileSync(join(root, "generated", "dev-docs-bodies.en.ts"), "utf8"), "// SENTINEL：已有英文模块\n", "已有译文模块不应被覆盖");
    // 缺的其它语言模块补空对象（页面侧是显式 import()）
    for (const lang of BODY_LANGS.filter((item) => item !== "en")) {
      assert.match(readFileSync(join(root, "generated", `dev-docs-bodies.${lang}.ts`), "utf8"), /export default \{\};/);
    }

    const fresh = join(root, "generated-fresh", "dev-docs.ts");
    const created = run([
      "--docs-dir",
      join(root, "docs"),
      "--manifest",
      join(root, "manifest.json"),
      "--out",
      fresh,
    ]);
    assert.equal(created.status, 0, `没有 docs/ 且没有生成物时也不应失败：${created.stderr}`);
    const generated = readFileSync(fresh, "utf8");
    assert.match(generated, /GENERATED_DEV_DOCS: GeneratedDevDoc\[\] = \[\]/);
    assert.match(generated, /回退到旧静态数据|白名单为空/);
    for (const lang of BODY_LANGS) {
      assert.match(readFileSync(join(root, "generated-fresh", `dev-docs-bodies.${lang}.ts`), "utf8"), /export default \{\};/);
    }
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("--check：生成物与 docs/ 不一致时失败，重新生成后通过", () => {
  const root = makeTmpRoot();
  try {
    writeFiles(root, {
      "docs/dev/a.md": FRONT_MATTER_A,
      "manifest.json": JSON.stringify({ docs: ["dev/a.md"] }),
    });
    const args = argsFor(root);
    const out = join(root, "generated", "dev-docs.ts");

    const missing = run([...args, "--check"]);
    assert.equal(missing.status, 1, "生成物不存在时 --check 应失败");
    assert.match(missing.stderr, /生成物不存在/);

    assert.equal(run(args).status, 0);
    const fresh = run([...args, "--check"]);
    assert.equal(fresh.status, 0, `一致时 --check 应通过：${fresh.stderr}`);

    // 译文模块也是生成物：缺失 / 过期同样失败
    const bodyFile = join(root, "generated", "dev-docs-bodies.en.ts");
    rmSync(bodyFile);
    const missingBodies = run([...args, "--check"]);
    assert.equal(missingBodies.status, 1, "译文模块缺失时 --check 应失败");
    assert.match(missingBodies.stderr, /dev-docs-bodies\.en\.ts/);
    assert.equal(run(args).status, 0);
    writeFileSync(bodyFile, `${readFileSync(bodyFile, "utf8")}\n// 过期\n`);
    const staleBodies = run([...args, "--check"]);
    assert.equal(staleBodies.status, 1, "译文模块过期时 --check 应失败");
    assert.match(staleBodies.stderr, /dev-docs-bodies\.en\.ts/);
    assert.equal(run(args).status, 0);

    writeFileSync(join(root, "docs", "dev", "a.md"), `${FRONT_MATTER_A}\n追加一段。\n`);
    const stale = run([...args, "--check"]);
    assert.equal(stale.status, 1, "docs/ 改动后 --check 应失败");
    assert.match(stale.stderr, /不一致/);
    assert.match(stale.stderr, /重新生成/);

    assert.equal(run(args).status, 0);
    assert.equal(run([...args, "--check"]).status, 0);
    assert.ok(readFileSync(out, "utf8").includes("追加一段。"));
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("仓库里的生成物与 docs/ 同步（改了文档要重新生成并提交）", () => {
  const result = run(["--check"]);
  assert.equal(result.status, 0, `web/src/generated/dev-docs.ts 与 docs/ 不一致：${result.stderr}`);
});

test("页面数据优先用生成物，生成物为空时回退旧静态数据", async () => {
  const { selectDevDocs, fromGeneratedDoc, devDocsSource } = await import(SELECT_MODULE);
  const fallback = [
    {
      slug: "fallback-doc",
      category: "start",
      title: { zh: "兜底", en: "Fallback" },
      summary: { zh: "兜底摘要", en: "Fallback summary" },
      body: "# 兜底正文\n",
      updatedAt: "2026-01-01",
      tags: [],
    },
  ];
  const generated = [
    {
      slug: "generated-doc",
      category: "plugin",
      title: { zh: "生成", en: "Generated" },
      summary: { zh: "生成摘要", en: "Generated summary" },
      tags: ["x"],
      updated_at: "2026-02-03",
      source: "dev/generated-doc.md",
      markdown: "# 生成正文\n",
    },
  ];

  assert.equal(devDocsSource([]), "fallback");
  assert.equal(devDocsSource(generated), "generated");
  assert.equal(selectDevDocs([], fallback), fallback, "没有生成物时应原样使用旧静态数据");
  const mapped = fromGeneratedDoc(generated[0]);
  assert.deepEqual(mapped, {
    slug: "generated-doc",
    category: "plugin",
    title: { zh: "生成", en: "Generated" },
    summary: { zh: "生成摘要", en: "Generated summary" },
    body: "# 生成正文\n",
    updatedAt: "2026-02-03",
    tags: ["x"],
    sourcePath: "dev/generated-doc.md",
  });
  assert.ok(!("hasBody" in mapped), "生成物没有译文时不应映射出 hasBody 键");
  // 只有「哪些语言有译文」的同步标记进页面结构；正文由 lib/dev-docs-i18n.ts 按语言懒加载
  const withBodies = fromGeneratedDoc({ ...generated[0], hasBody: { en: true, es: true } });
  assert.deepEqual(withBodies.hasBody, { en: true, es: true });
  assert.ok(!("bodies" in withBodies), "译文正文不应再进页面数据结构（改为按语言懒加载）");
  const picked = selectDevDocs(generated, fallback);
  assert.equal(picked.length, 1);
  assert.equal(picked[0].slug, "generated-doc");
  assert.notEqual(picked, fallback);
});

test("llms.txt：生成文档清单与契约入口，仓库文档引用做断链检查", () => {
  const root = makeTmpRoot();
  try {
    writeFiles(root, {
      "docs/dev/a.md": FRONT_MATTER_A,
      "docs/errors.md": "# 错误码\n",
      "docs/extension-protocol.md": "# 扩展协议\n",
      "manifest.json": JSON.stringify({
        docs: ["dev/a.md"],
        repository: "https://github.com/acme/dsh-ee",
        llms: {
          title: "ACME 开发文档",
          summary: "给 Agent 的入口。",
          contracts: [
            { title: "发现文档", url: "/.well-known/dsh-ee", description: "匿名能力发现。" },
            { title: "错误码", docs: "errors.md", description: "错误索引。" },
            { title: "扩展协议", docs: "extension-protocol.md" },
          ],
        },
      }),
    });
    const result = run(argsFor(root));
    assert.equal(result.status, 0, `生成失败：${result.stderr}`);
    const llms = readFileSync(join(root, "generated", "llms.txt"), "utf8");
    assert.match(llms, /^# ACME 开发文档\n/);
    assert.ok(llms.includes("> 给 Agent 的入口。"), "应写摘要 blockquote");
    assert.ok(llms.includes("- [A 标题 / A title](/docs/a-doc): A 摘要"), "应列出站内文档");
    assert.ok(llms.includes("- [发现文档](/.well-known/dsh-ee): 匿名能力发现。"), "应保留站内契约 URL");
    assert.ok(
      llms.includes("- [错误码](https://github.com/acme/dsh-ee/blob/main/docs/errors.md): 错误索引。"),
      "仓库契约应转成固定仓库的 blob URL",
    );
    assert.ok(llms.includes("https://github.com/acme/dsh-ee/blob/main/docs/extension-protocol.md"));
    assert.ok(llms.includes("https://github.com/acme/dsh-ee/tree/main/docs"), "应带仓库文档目录入口");

    // 同一输入再生成一次，llms.txt 字节一致
    assert.equal(run(argsFor(root)).status, 0);
    assert.equal(readFileSync(join(root, "generated", "llms.txt"), "utf8"), llms);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("llms.txt：契约引用的仓库文档不存在时失败并报路径（断链检查）", () => {
  const root = makeTmpRoot();
  try {
    writeFiles(root, {
      "docs/dev/a.md": FRONT_MATTER_A,
      "manifest.json": JSON.stringify({
        docs: ["dev/a.md"],
        llms: { contracts: [{ title: "错误码", docs: "missing-errors.md" }] },
      }),
    });
    const result = run(argsFor(root));
    assert.equal(result.status, 1, "llms 引用的仓库文档不存在时应失败");
    assert.match(result.stderr, /missing-errors\.md/);
    assert.match(result.stderr, /llms/);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("--check：llms.txt 缺失或过期同样报错，重新生成后通过", () => {
  const root = makeTmpRoot();
  try {
    writeFiles(root, {
      "docs/dev/a.md": FRONT_MATTER_A,
      "manifest.json": JSON.stringify({ docs: ["dev/a.md"] }),
    });
    const args = argsFor(root);
    const llmsFile = join(root, "generated", "llms.txt");
    assert.equal(run(args).status, 0);

    writeFileSync(llmsFile, `${readFileSync(llmsFile, "utf8")}\n额外一行。\n`);
    const stale = run([...args, "--check"]);
    assert.equal(stale.status, 1, "llms.txt 过期时 --check 应失败");
    assert.match(stale.stderr, /llms\.txt/);
    assert.match(stale.stderr, /不一致/);

    rmSync(llmsFile);
    const missing = run([...args, "--check"]);
    assert.equal(missing.status, 1, "llms.txt 缺失时 --check 应失败");
    assert.match(missing.stderr, /生成物不存在：llms\.txt/);

    assert.equal(run(args).status, 0);
    assert.equal(run([...args, "--check"]).status, 0);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("中文 golden 搜索：标题/摘要/正文命中、两字词、多词全部命中", async () => {
  const generatedUrl = new URL("../../web/src/generated/dev-docs.ts", import.meta.url).href;
  const selectUrl = new URL("../../web/src/data/dev-docs-select.ts", import.meta.url).href;
  const searchUrl = new URL("../../web/src/data/dev-docs-search.ts", import.meta.url).href;
  const { GENERATED_DEV_DOCS } = await import(generatedUrl);
  const { fromGeneratedDoc } = await import(selectUrl);
  const { matchesDevDoc, registerDocBodies, searchDevDocs, docBodyText } = await import(searchUrl);
  const docs = GENERATED_DEV_DOCS.map(fromGeneratedDoc);
  // 数量以发布白名单为准（新增一篇只改 manifest + 重新生成，这里不再写死数字）。
  const docsManifest = JSON.parse(readFileSync(join(HERE, "manifest.json"), "utf8"));
  assert.equal(docs.length, docsManifest.docs.length, `生成物应有 ${docsManifest.docs.length} 篇站内文档`);

  // 标题命中：access-rules 的标题与摘要都含「权限」，应排第一
  assert.ok(matchesDevDoc(docs.find((d) => d.slug === "access-rules"), "权限"));
  assert.equal(searchDevDocs(docs, "权限")[0].doc.slug, "access-rules");
  // 正文命中（中文两字词）：设备码只出现在 harness-quickstart 正文里
  const bodyHits = searchDevDocs(docs, "设备码");
  assert.equal(bodyHits.length, 1, "「设备码」应只命中一篇");
  assert.equal(bodyHits[0].doc.slug, "harness-quickstart");
  assert.ok(bodyHits[0].snippet.includes("设备码"), `片段应包含命中词：${bodyHits[0].snippet}`);
  // 正文命中（英文/大小写不敏感）：main 新增的站内文档也有 model 字样，只断言命中集合包含 harness-quickstart
  assert.ok(searchDevDocs(docs, "MODEL").some((hit) => hit.doc.slug === "harness-quickstart"), "大写 MODEL 应命中正文");
  assert.ok(searchDevDocs(docs, "model").some((hit) => hit.doc.slug === "harness-quickstart"), "小写 model 应命中同一篇");
  assert.ok(searchDevDocs(docs, "TASL").some((hit) => hit.doc.slug === "quality-standards"));
  // 多个词要求全部命中（正文 chunk 未加载时英文界面按中文正文兜底，见下）
  const harness = docs.find((d) => d.slug === "harness-quickstart");
  assert.ok(!matchesDevDoc(harness, "device code pgvector", "en"));
  assert.equal(searchDevDocs(docs, "device code pgvector", 6, "en").length, 0);
  // 正文 chunk 未加载时英文界面按中文正文兜底（与详情页「无正文回退 zh」一致，不谎报命中英文正文）
  assert.equal(matchesDevDoc(harness, "设备码", "en"), true, "正文未加载时英文搜索按中文正文兜底");
  assert.equal(matchesDevDoc(harness, "设备码", "zh"), true);
  // 空查询：列表页显示全部，⌘K 分组不展示
  assert.ok(docs.every((doc) => matchesDevDoc(doc, "  ")));
  assert.equal(searchDevDocs(docs, "").length, 0);
  // limit 生效
  assert.equal(searchDevDocs(docs, "权限", 2).length, 2);
  // 正文已拆成按语言懒加载的 chunk：预热（primeBodies ≈ registerDocBodies）前英文搜索退回中文正文，
  // 预热后同一篇文档的不同语言拿到不同正文（缓存键仍是 slug:lang，互不覆盖），中文搜索行为不变。
  const overview = docs.find((d) => d.slug === "overview");
  assert.equal(docBodyText(overview, "en"), docBodyText(overview, "zh"), "正文 chunk 未加载时英文正文按中文兜底");
  const enBodies = (await import(new URL("../../web/src/generated/dev-docs-bodies.en.ts", import.meta.url).href)).default;
  registerDocBodies("en", enBodies);
  assert.equal(matchesDevDoc(overview, "deliverable", "en"), true, "预热英文正文后应能命中");
  assert.ok(matchesDevDoc(harness, "device code and limits", "en"), "预热英文正文后英文多词应命中");
  assert.equal(matchesDevDoc(harness, "设备码", "en"), false, "预热英文正文后英文界面不应再命中中文正文");
  assert.notEqual(docBodyText(overview, "en"), docBodyText(overview, "zh"));
  assert.match(docBodyText(overview, "en"), /deliverable/);
  assert.ok(!matchesDevDoc(overview, "deliverable", "zh"), "中文界面不应命中英文正文");
  // docBodyText 也可以显式传入已加载的正文缓存（不依赖模块级注册）
  assert.match(docBodyText(overview, "en", enBodies), /deliverable/);
});

test("五语搜索：按界面语言命中对应语言的标题/摘要，未命中语言时回退 en → zh", async () => {
  const generatedUrl = new URL("../../web/src/generated/dev-docs.ts", import.meta.url).href;
  const selectUrl = new URL("../../web/src/data/dev-docs-select.ts", import.meta.url).href;
  const searchUrl = new URL("../../web/src/data/dev-docs-search.ts", import.meta.url).href;
  const { GENERATED_DEV_DOCS } = await import(generatedUrl);
  const { fromGeneratedDoc } = await import(selectUrl);
  const { matchesDevDoc, registerDocBodies, searchDevDocs } = await import(searchUrl);
  const docs = GENERATED_DEV_DOCS.map(fromGeneratedDoc);
  // 模拟「进入 /docs 或打开 ⌘K 后 primeBodies 完成」：注册四语正文模块后正文可被搜索命中
  for (const lang of BODY_LANGS) {
    registerDocBodies(lang, (await import(new URL(`../../web/src/generated/dev-docs-bodies.${lang}.ts`, import.meta.url).href)).default);
  }

  assert.equal(searchDevDocs(docs, "compatibility", 6, "en")[0].doc.slug, "anthropic-api", "英文标题应命中");
  assert.equal(searchDevDocs(docs, "Permisos y reglas de acceso", 6, "es")[0].doc.slug, "access-rules", "es 标题应命中");
  assert.equal(searchDevDocs(docs, "Entradas de publicação", 6, "pt")[0].doc.slug, "publish-precheck", "pt 标题应命中");
  assert.equal(searchDevDocs(docs, "अनुमतियाँ", 6, "hi")[0].doc.slug, "access-rules", "hi 标题应命中");
  // 语言切换后同一个查询按该语言的标题/摘要/正文重新判定（es 标题命中，zh 不含该词）
  assert.equal(matchesDevDoc(docs.find((d) => d.slug === "access-rules"), "Permisos", "es"), true);
  assert.equal(matchesDevDoc(docs.find((d) => d.slug === "access-rules"), "Permisos", "zh"), false);
  // 正文也按界面语言：es / pt / hi 的正文词可搜到，切到中文则搜不到（标题/摘要也不含这些词）
  assert.equal(searchDevDocs(docs, "entregas", 6, "es")[0].doc.slug, "overview", "es 正文应可搜到");
  assert.equal(searchDevDocs(docs, "caminhos de desenvolvimento", 6, "pt")[0].doc.slug, "overview", "pt 正文应可搜到");
  assert.ok(searchDevDocs(docs, "विकास के तीन मार्ग", 6, "hi").some((hit) => hit.doc.slug === "overview"), "hi 正文应可搜到");
  assert.equal(searchDevDocs(docs, "entregas", 6, "zh").length, 0, "中文界面不应命中 es 正文");
  // 旧静态兜底数据只有 zh 正文（没有 hasBody / 也不在正文模块里）：es/pt/hi 回退英文标题/摘要，正文回退中文
  // 用独立 slug，避免正文纯文本缓存（slug:lang）串到真实文档上
  const legacyDoc = docs.find((d) => d.slug === "access-rules");
  const { hasBody: _ignoredHasBody, ...legacyOnly } = {
    ...legacyDoc,
    slug: "legacy-access-rules",
    title: { zh: "权限", en: "Permissions" },
    summary: { zh: "摘要", en: "Summary" },
  };
  assert.equal(matchesDevDoc(legacyOnly, "Permissions", "es"), true, "缺 es 时搜索回退英文");
  assert.equal(matchesDevDoc(legacyOnly, "权限", "pt"), true, "缺 pt 译文时正文回退中文，中文词仍可命中正文");
});

test("五语取值：pickLocalized 按 当前语言 → en → zh 回退，docLang 归一 BCP-47", async () => {
  const helperUrl = new URL("../../web/src/lib/dev-docs-i18n.ts", import.meta.url).href;
  const { DOC_LANGS, docLang, pickLocalized } = await import(helperUrl);
  assert.deepEqual([...DOC_LANGS], ["zh", "en", "es", "pt", "hi"]);
  // resolvedLanguage 是 BCP-47 标签，归一到五语键
  assert.equal(docLang("zh-CN"), "zh");
  assert.equal(docLang("zh-TW"), "zh");
  assert.equal(docLang("en-US"), "en");
  assert.equal(docLang("es-MX"), "es");
  assert.equal(docLang("pt-BR"), "pt");
  assert.equal(docLang("hi-IN"), "hi");
  assert.equal(docLang("fr-FR"), "en", "未知语言按英文处理");
  assert.equal(docLang(undefined), "en");

  const full = { zh: "中文", en: "English", es: "Español", pt: "Português", hi: "हिन्दी" };
  assert.equal(pickLocalized(full, "hi-IN"), "हिन्दी");
  assert.equal(pickLocalized(full, "en-GB"), "English");
  assert.equal(pickLocalized(full, "zh-HK"), "中文");
  // 缺译回退链：es/pt/hi → en → zh（旧静态兜底数据只有 zh/en）
  const legacy = { zh: "中文", en: "English" };
  assert.equal(pickLocalized(legacy, "es"), "English");
  assert.equal(pickLocalized(legacy, "pt-BR"), "English");
  assert.equal(pickLocalized({ zh: "中文", en: "" }, "hi"), "中文");
});

test("仓库里的五语元数据：8 篇站内文档的标题/摘要都有人工 es/pt/hi 译文", async () => {
  const generatedUrl = new URL("../../web/src/generated/dev-docs.ts", import.meta.url).href;
  const { GENERATED_DEV_DOCS } = await import(generatedUrl);
  const translated = GENERATED_DEV_DOCS.filter((doc) => TRANSLATED_SLUGS.includes(doc.slug));
  assert.equal(translated.length, 8, "生成物应包含 8 篇已接入人工译文的站内文档");
  for (const doc of translated) {
    for (const [field, label] of [
      [doc.title, "title"],
      [doc.summary, "summary"],
    ]) {
      assert.deepEqual(Object.keys(field), ["zh", "en", "es", "pt", "hi"], `${doc.slug} 的 ${label} 应写全五键`);
      for (const lang of ["zh", "en", "es", "pt", "hi"]) {
        assert.ok(field[lang] && field[lang].trim(), `${doc.slug} 的 ${label}.${lang} 不能为空`);
      }
      for (const lang of ["es", "pt", "hi"]) {
        assert.notEqual(field[lang], field.en, `${doc.slug} 的 ${label}.${lang} 应是人工译文，不能直接复用英文`);
      }
    }
  }
  // main 上新增的站内文档还没人工译文：生成物仍写全五键，es/pt/hi 按回退链落到 en（zh 缺英译时是 zh）
  for (const doc of GENERATED_DEV_DOCS.filter((item) => !TRANSLATED_SLUGS.includes(item.slug))) {
    for (const field of [doc.title, doc.summary]) {
      assert.deepEqual(Object.keys(field), ["zh", "en", "es", "pt", "hi"], `${doc.slug} 也应写全五键`);
      assert.equal(field.es, field.en, `${doc.slug} 缺 es 译文时回退 en`);
      assert.equal(field.pt, field.en, `${doc.slug} 缺 pt 译文时回退 en`);
      assert.equal(field.hi, field.en, `${doc.slug} 缺 hi 译文时回退 en`);
    }
  }
});
