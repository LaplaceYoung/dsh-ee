// 开发文档 / 开发样例（PR-B）：固定路由 /docs、/examples，工作台「开发」双卡，侧栏「开发」组。
// 页面内容全部是前端静态数据，不需要额外接口；这里同时验证表格、标题锚点、移动端单列与中英切换。
import { ADMIN_PW, BASE, assert, assertEq, login, newPage, shot, step } from "../lib.mjs";

export default async function (t) {
  const page = await newPage(t);
  await login(page, "admin", ADMIN_PW);

  // ---- 侧栏：「开发」组在「接入」之后，两项都进得去 ----
  const aside = page.locator("aside");
  const group = aside.getByText("开发", { exact: true });
  await group.waitFor();
  const jieru = aside.getByText("接入", { exact: true });
  const [gb, jb] = await Promise.all([group.boundingBox(), jieru.boundingBox()]);
  assert(gb && jb && gb.y > jb.y, "「开发」组应排在「接入」组之后");
  await aside.getByRole("link", { name: "开发文档" }).waitFor();
  await aside.getByRole("link", { name: "开发样例" }).waitFor();
  step("侧栏新增「开发」组（开发文档 / 开发样例），排在「接入」之后");

  // ---- /docs：目录 + 分类筛选 + 详情（表格与标题锚点）----
  // 站内文档数量从构建期生成的 /llms.txt 反推：新增一篇 docs/dev 文档后这里不用跟着改数字。
  const llmsRes = await page.request.get(`${BASE}/llms.txt`);
  assertEq(llmsRes.status(), 200, "/llms.txt 状态");
  const llmsText = await llmsRes.text();
  const docTotal = new Set([...llmsText.matchAll(/\]\(\/docs\/([a-z0-9-]+)\)/g)].map((m) => m[1])).size;
  assert(docTotal >= 7, `llms.txt 应列出站内文档，实际 ${docTotal} 篇`);
  await page.goto(`${BASE}/docs`);
  await page.getByRole("heading", { level: 1, name: "开发文档" }).waitFor();
  const docCards = page.locator("[data-dev-doc]");
  assertEq(await docCards.count(), docTotal, "开发文档数量");
  await page.getByRole("button", { name: "DSH 插件" }).click();
  assertEq(await page.locator("a[data-dev-doc]").count(), 1, "按分类筛选后的文档数");
  await page.getByRole("button", { name: "全部" }).click();
  // 客户端搜索：标题/摘要/标签/正文全覆盖，中文两字词直接子串；清除后恢复全部
  const search = page.getByRole("searchbox");
  await search.fill("设备码"); // 只出现在 harness-quickstart 正文里，验证正文搜索
  assertEq(await page.locator("a[data-dev-doc]").count(), 1, "搜索「设备码」后的文档数");
  assertEq(await page.locator('a[data-dev-doc="harness-quickstart"]').count(), 1, "正文命中应出现 harness-quickstart");
  await search.fill("TASL"); // overview 与 quality-standards 正文都有
  assertEq(await page.locator("a[data-dev-doc]").count(), 2, "搜索「TASL」后的文档数");
  await page.getByRole("button", { name: "清除搜索" }).click();
  assertEq(await page.locator("a[data-dev-doc]").count(), docTotal, "清除搜索后恢复全部文档");
  step("/docs 支持标题+正文搜索（中文两字词）与分类筛选叠加");
  await shot(page, "devhub-docs-list");

  // /llms.txt 的正文断言（状态码/正文在上面已经取过）
  assert(llmsText.startsWith("# dsh-ee 开发文档"), "llms.txt 应以 H1 标题开头");
  for (const needle of ["/docs/overview", "/docs/harness-quickstart", "/docs/chatbot", "/v1/messages", "/anthropic/v1/messages", "/.well-known/dsh-ee", "/v1/models", "/mcp", "docs/errors.md", "docs/extension-protocol.md"]) {
    assert(llmsText.includes(needle), `/llms.txt 缺少「${needle}」`);
  }
  step("/llms.txt 可匿名读取，含文档清单与发现/模型/MCP/错误码入口");

  await page.locator('a[data-dev-doc="overview"]').click();
  await page.waitForURL(/\/docs\/overview$/);
  // 详情页小入口：复制链接 + 查看仓库源文件
  const sourceHref = await page.locator("[data-dev-doc-source]").getAttribute("href");
  assert(sourceHref?.endsWith("/blob/main/docs/dev/overview.md"), `查看源文件应指向仓库 docs/dev/overview.md，实际 ${sourceHref}`);
  await page.getByRole("button", { name: "复制链接" }).click();
  await page.getByText(/链接已复制|复制失败/).first().waitFor();
  await shot(page, "devhub-doc-actions");
  step("详情页有「复制链接」与「在 GitHub 查看源文件」入口，源文件指向 docs/dev");
  // 正文里的站内链接用 router 跳转，不刷新整页
  await page.getByRole("link", { name: /写 DSH 插件并发布到企业源/ }).click();
  await page.waitForURL(/\/docs\/plugin-quickstart$/);
  await page.getByRole("heading", { level: 1, name: /写 DSH 插件并发布到企业源/ }).waitFor();
  assert((await page.locator("[data-dev-markdown] .tbl").count()) >= 2, "文档里应有表格");
  const heading = page.locator("[data-dev-markdown] h2[id]").first();
  await heading.waitFor();
  const hid = await heading.getAttribute("id");
  assert(hid, "标题应有 GitHub 同款 id");
  await heading.locator('a[href^="#"]').click();
  await page.waitForTimeout(200);
  assert(page.url().endsWith(`#${encodeURI(hid)}`) || page.url().endsWith(`#${hid}`), `点标题锚点后地址应带 #${hid}，实际 ${page.url()}`);
  await shot(page, "devhub-doc-detail");
  step("/docs 列表可分类筛选，详情页支持表格与 /docs/:slug#标题锚点 深链");

  // Chatbot 文档：新增站内页要有正文（配置、服务端工具与能力声明字段），且 llms.txt 已收录
  await page.goto(`${BASE}/docs/chatbot`);
  await page.getByRole("heading", { level: 1, name: "Chatbot 网关选项与透传" }).waitFor();
  const chatbotText = await page.locator('[data-dev-page="docs"]').innerText();
  for (const needle of ["DSH_EE_SAFETY_CHECK", "DSH_EE_CHAT_TITLE", "deepseek_web_search", "deepseek_read_url", "safety_check", "server_tool_calls", "chatbot"]) {
    assert(chatbotText.includes(needle), `Chatbot 文档缺少「${needle}」`);
  }
  step("/docs/chatbot 渲染成功：Chatbot 配置、服务端工具与能力声明字段可读");

  // ---- 内容事实（review）：清单示例、命令、Node/primer 与预检语义必须和代码一致 ----
  await page.goto(`${BASE}/docs/extension-quickstart`);
  const extText = await page.locator('[data-dev-page="docs"]').innerText();
  for (const needle of ['"id": "acme/oa"', '"kind": "declarative"', '"openapi": "openapi.yaml"', '"servers"', "--id acme/my-ext", "端侧", "内置"]) {
    assert(extText.includes(needle), `扩展文档缺少「${needle}」`);
  }
  for (const bad of ['"endpoints"', '"schema": "dsh-ee.extension/1"', "打成 .tgz"]) {
    assert(!extText.includes(bad), `扩展文档不应出现「${bad}」`);
  }
  assert(extText.includes("打成 zip"), "extension pack 产物应写 zip");
  await page.goto(`${BASE}/docs/plugin-quickstart`);
  const quickText = await page.locator('[data-dev-page="docs"]').innerText();
  assert(quickText.includes("24.9") && quickText.includes("dsh plugin --profile web add dsh-primer"), "Node 门槛与 dsh-primer 安装步骤应对齐");
  await page.goto(`${BASE}/docs/publish-precheck`);
  const preText = await page.locator('[data-dev-page="docs"]').innerText();
  for (const needle of ["白名单模式", "开放模式", "不是拒收", "提醒"]) assert(preText.includes(needle), `预检文档缺少「${needle}」`);
  await page.goto(`${BASE}/docs/harness-quickstart`);
  const harnessText = await page.locator('[data-dev-page="docs"]').innerText();
  for (const needle of ["ee_at_", "/.well-known/dsh-ee", "/v1/models", "/mcp", "写操作", "确认"]) {
    assert(harnessText.includes(needle), `Harness 文档缺少「${needle}」`);
  }
  await page.goto(`${BASE}/docs/quality-standards`);
  const stdText = await page.locator('[data-dev-page="docs"]').innerText();
  for (const needle of ["不在服务端内置执行 checker", "Change Notice", "引用或改编", "合并后可用"]) assert(stdText.includes(needle), `规范文档缺少「${needle}」`);
  assert(!stdText.includes("不复制规范正文"), "规范文档不应再写「不复制规范正文」（与 #119/#121 改编矛盾）");
  step("站内文档与代码事实一致：清单 schema / init --id / pack=zip / Node≥24.9 / 预检模式 / 规范改编许可");

  // ---- ⌘K「文档」分组：正文命中也能搜到，选中进 /docs/:slug ----
  await page.keyboard.press("Control+k");
  const palette = page.getByRole("dialog");
  await palette.waitFor();
  const paletteInput = palette.getByRole("combobox");
  await paletteInput.fill("设备码");
  const docOption = palette.getByRole("option", { name: /接第三方 Harness \/ 自研客户端/ });
  await docOption.waitFor();
  await shot(page, "devhub-doc-palette");
  await docOption.click();
  await page.waitForURL(/\/docs\/harness-quickstart$/);
  step("⌘K 新增「文档」分组：标题/摘要/正文命中，选中进 /docs/:slug");

  // ---- 固定路由不依赖菜单：直接打开 /docs/:slug 也能渲染；未知 slug 显示空态 ----
  await page.goto(`${BASE}/docs/access-rules`);
  await page.getByRole("heading", { level: 1, name: /权限与访问规则速查/ }).waitFor();
  await page.goto(`${BASE}/docs/not-a-real-doc`);
  await page.getByText("找不到这篇文档").waitFor();
  await page.goto(`${BASE}/examples/not-a-real-example`);
  await page.getByText("找不到这个样例").waitFor();
  step("直接访问 /docs/:slug 固定路由可用（不依赖菜单里的 path），未知 slug 有空态");

  // ---- /examples：8 个样例、安装命令、固定版本、来源与许可 ----
  await page.goto(`${BASE}/examples`);
  await page.getByRole("heading", { level: 1, name: "开发样例" }).waitFor();
  const examples = page.locator("[data-dev-example]");
  assertEq(await examples.count(), 8, "开发样例数量");
  const installTexts = await page.locator("[data-dev-example]").getByText(/dsh plugin --profile \w+ add/).count();
  assertEq(installTexts, 8, "每个样例都应带安装命令");
  const listText = await page.locator('[data-dev-page="examples"]').innerText();
  assert(!/认证|排名/.test(listText), "样例页不应出现「认证」「排名」字样");
  assert(listText.includes("dsh-test-drive@0.3.25"), "dsh-test-drive 应钉 npm 最新 0.3.25");
  assert(listText.includes("@perrylink/dsh-research-core@0.1.1"), "claim-check 的 peer 依赖也要固定版本");
  await shot(page, "devhub-examples-list");
  await page.locator('[data-dev-example="dsh-claim-check"]').getByRole("link", { name: /证据逐字核对/ }).click();
  await page.waitForURL(/\/examples\/dsh-claim-check$/);
  await page.getByRole("heading", { level: 1, name: /证据逐字核对/ }).waitFor();
  const detail = page.locator('[data-dev-page="examples"]');
  await detail.getByText("@v0.1.2").waitFor();
  await detail.getByRole("link", { name: "Apache-2.0" }).waitFor();
  await detail.getByText(/未复制上游 README/).waitFor();
  const walk = detail.locator("[data-dev-walkthrough]");
  await walk.waitFor();
  const walkHref = await walk.getByRole("link", { name: /打开完整走读/ }).getAttribute("href");
  assert(walkHref?.includes("docs/examples-walkthrough.md#1-dsh-claim-check"), `完整走读锚点不对：${walkHref}`);
  await walk.getByText(/合并后可用/).waitFor();
  await shot(page, "devhub-example-detail");
  step("/examples 8 个样例带固定版本安装命令，详情页有来源 repo@tag 与 Apache-2.0 许可");

  // ---- 工作台：「开发」双卡在权限申请卡之后，各 3 条摘要 ----
  await page.goto(`${BASE}/workbench`);
  const block = page.locator("[data-dev-block]");
  await block.waitFor();
  assertEq(await block.locator('[data-dev-card="docs"] a[data-dev-doc]').count(), 3, "文档卡条数");
  assertEq(await block.locator('[data-dev-card="examples"] a[data-dev-example]').count(), 3, "样例卡条数");
  const blockBox = await block.boundingBox();
  const reqCard = page.locator("[data-access-requests]");
  if (await reqCard.count()) {
    const reqBox = await reqCard.first().boundingBox();
    assert(blockBox && reqBox && blockBox.y > reqBox.y, "「开发」双卡应位于权限申请卡之后");
  }
  await shot(page, "devhub-workbench");
  await block.getByRole("link", { name: new RegExp(`全部 ${docTotal} 篇`) }).click();
  await page.waitForURL(/\/docs$/);
  step("工作台在权限申请卡之后显示「开发」双卡，各 3 条摘要并可进全部");

  // ---- 移动端：两卡单列堆叠 ----
  const mobile = await newPage(t, { viewport: { width: 390, height: 844 } });
  await login(mobile, "admin", ADMIN_PW);
  await mobile.goto(`${BASE}/workbench`);
  const mobileCards = mobile.locator("[data-dev-block] [data-dev-card]");
  await mobileCards.first().waitFor();
  const [b1, b2] = await Promise.all([mobileCards.nth(0).boundingBox(), mobileCards.nth(1).boundingBox()]);
  assert(b1 && b2 && b1.x === b2.x && b2.y > b1.y, "窄屏下两张卡应单列堆叠");
  step("移动端「开发」双卡单列堆叠");

  // ---- 英文界面：页名、侧栏与内容标题都跟着语言走 ----
  const en = await newPage(t, { lang: "en" });
  await login(en, "admin", ADMIN_PW);
  await en.goto(`${BASE}/docs`);
  await en.getByRole("heading", { level: 1, name: "Developer docs" }).waitFor();
  await en.locator("aside").getByRole("link", { name: "Developer docs" }).waitFor();
  await en.locator("aside").getByRole("link", { name: "Examples" }).waitFor();
  await en.getByText("Body in Chinese").first().waitFor();
  await en.goto(`${BASE}/examples`);
  await en.getByRole("heading", { level: 1, name: "Examples" }).waitFor();
  step("英文界面：页名、侧栏「Develop」组与文档语言提示都正确");
}
