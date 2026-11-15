// 站内开发文档管线（docs/ 单一事实源）的测试：
// - 生成器幂等、白名单、链接改写、坏 front matter 失败、没有 docs/ 时兜底；
// - --check 能发现生成物与 docs/ 不同步；
// - 页面数据优先用生成物、生成物为空时回退旧静态数据。
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
    const first = run(argsFor(root));
    assert.equal(first.status, 0, `生成失败：${first.stderr}`);
    const firstContent = readFileSync(out, "utf8");
    const second = run(argsFor(root));
    assert.equal(second.status, 0, `二次生成失败：${second.stderr}`);
    assert.equal(readFileSync(out, "utf8"), firstContent, "两次生成必须字节一致");

    assert.match(firstContent, /slug: "a-doc"/);
    assert.match(firstContent, /title: \{ zh: "A 标题", en: "A title" \}/);
    assert.match(firstContent, /tags: \["一", "二"\]/);
    assert.match(firstContent, /updated_at: "2026-01-02"/);
    assert.match(firstContent, /markdown: "## 小节\\n\\n正文 A。\\n"/);
    // 没有 front matter 的 b.md：默认值
    assert.match(firstContent, /slug: "b"/);
    assert.match(firstContent, /category: "start"/);
    assert.match(firstContent, /title: \{ zh: "B 的标题", en: "B 的标题" \}/);
    assert.match(firstContent, /summary: \{ zh: "B 的第一段正文。", en: "B 的第一段正文。" \}/);
    assert.match(firstContent, /updated_at: ""/);
    assert.ok(firstContent.indexOf('slug: "a-doc"') < firstContent.indexOf('slug: "b"'), "顺序应与白名单一致");
    assert.ok(!firstContent.includes("SHOULD_NOT_APPEAR"), "白名单外的文件不应出现在生成物里");
    assert.match(firstContent, /export const DOCS_REPO_BLOB = "https:\/\/github\.com\/hikariming\/dsh-ee\/blob\/main";/);
    assert.match(firstContent, /source: "dev\/a\.md"/);

    // llms.txt：给 Agent 的入口，同样重复生成字节一致
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

test("没有 docs/ 时兜底：保留已有生成物，没有生成物时写空数组让页面回退", () => {
  const root = makeTmpRoot();
  try {
    const out = join(root, "generated", "dev-docs.ts");
    writeFiles(root, { "generated/dev-docs.ts": "// SENTINEL：已有生成物\n" });
    const kept = run(argsFor(root));
    assert.equal(kept.status, 0, `没有 docs/ 不应让构建失败：${kept.stderr}`);
    assert.match(kept.stderr, /回退旧静态数据/);
    assert.equal(readFileSync(out, "utf8"), "// SENTINEL：已有生成物\n", "已有生成物不应被覆盖");

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
  const { matchesDevDoc, searchDevDocs } = await import(searchUrl);
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
  // 正文命中（英文/大小写不敏感）
  assert.equal(searchDevDocs(docs, "MODEL")[0].doc.slug, "harness-quickstart");
  assert.ok(searchDevDocs(docs, "TASL").some((hit) => hit.doc.slug === "quality-standards"));
  // 多个词要求全部命中
  assert.ok(matchesDevDoc(docs.find((d) => d.slug === "harness-quickstart"), "设备码 and limits"));
  assert.ok(!matchesDevDoc(docs.find((d) => d.slug === "harness-quickstart"), "设备码 pgvector"));
  assert.equal(searchDevDocs(docs, "设备码 pgvector").length, 0);
  // 空查询：列表页显示全部，⌘K 分组不展示
  assert.ok(docs.every((doc) => matchesDevDoc(doc, "  ")));
  assert.equal(searchDevDocs(docs, "").length, 0);
  // limit 生效
  assert.equal(searchDevDocs(docs, "权限", 2).length, 2);
});
