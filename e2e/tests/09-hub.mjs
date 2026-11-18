// 能力中心：管理员写技能 → 员工在能力中心启用 → DSH 目录里出现；员工写提示词 → 审核通过；
// 接入一个 OpenAPI 系统（就用 dsh-ee 自己的 /healthz）→ 员工经 /mcp 调用；系统设置里发布 DSH 接入插件
import { ADMIN_PW, BASE, api, assert, assertEq, login, newPage, shot, step, tag } from "../lib.mjs";

/** 以 DSH 的身份（接入令牌）调接口。 */
async function dsh(token, method, path, body) {
  const r = await fetch(BASE + path, {
    method,
    headers: { Authorization: `Bearer ${token}`, ...(body ? { "Content-Type": "application/json" } : {}) },
    body: body ? JSON.stringify(body) : undefined,
  });
  const text = await r.text();
  return { status: r.status, data: text ? JSON.parse(text) : null };
}

export default async function (t) {
  const admin = await newPage(t);
  await login(admin, "admin", ADMIN_PW);
  const skill = `e2e-skill-${tag}`;
  const prompt = `e2e-prompt-${tag}`;
  const app = `e2e-app-${tag}`;
  const conn = `e2e-health-${tag}`;

  // ---- 管理员写一个技能：有审核权限，直接上架 ----
  await admin.goto(`${BASE}/assets/new?kind=skill`);
  await admin.getByLabel("标识").fill(skill);
  await admin.getByLabel("标题").fill(`E2E 报销制度 ${tag}`);
  await admin.getByLabel("说明").fill("差旅住宿标准");
  await admin.locator("textarea").last().fill("住宿每晚 1300 元以内。");
  await shot(admin, "hub-editor");
  await admin.getByRole("button", { name: "提交", exact: true }).click();
  await admin.waitForURL(/\/assets\/\d+$/);
  await admin.getByText(`/${skill} 直接调用`).waitFor();
  step("管理员写的技能直接上架，详情页写明 DSH 里怎么用");
  await shot(admin, "hub-asset-detail");

  // ---- 员工：拿接入令牌，在能力中心启用技能 ----
  const emp = `hubemp-${tag}`;
  assertEq((await api(admin, "POST", "/api/admin/users", { username: emp, password: "hub-password-123", department: "研发效能部" })).status, 201, "建员工");
  const page = await newPage(t);
  await login(page, emp, "hub-password-123");
  const tok = await api(page, "POST", "/api/me/tokens", { name: "dsh", scopes: ["models", "registry:read", "hub", "connections"] });
  assertEq(tok.status, 201, "统一接入令牌");
  const token = tok.data.token;
  const legacy = await api(page, "POST", "/api/me/npm-tokens", { name: "legacy-dsh", access: "read" });
  assertEq(legacy.status, 201, "旧 npm 令牌");
  assertEq((await dsh(legacy.data.token, "GET", "/api/dsh/catalog")).status, 200, "旧 npm 令牌读取 DSH 目录");
  let cat = await dsh(token, "GET", "/api/dsh/catalog");
  assert(!cat.data.skills.some((s) => s.name === skill), "没启用的技能不应下发");
  await page.goto(`${BASE}/hub?tab=skill`);
  const card = page.locator("div.rounded-lg", { hasText: `E2E 报销制度 ${tag}` }).last();
  await card.getByRole("button", { name: "启用" }).click();
  await card.getByText("已在 DSH 生效").waitFor();
  cat = await dsh(token, "GET", "/api/dsh/catalog");
  assert(cat.data.skills.some((s) => s.name === skill), "启用后应出现在 DSH 目录：" + JSON.stringify(cat.data));
  const body = await dsh(token, "GET", `/api/dsh/skills/${skill}`);
  assert(body.data.content.includes("1300"), "技能正文");
  step("员工在能力中心启用后，DSH 目录里有了这个技能，正文可读");
  await shot(page, "hub-skills");

  // ---- 员工写提示词 → 待审核 → 管理员在审核页通过 ----
  await page.goto(`${BASE}/assets/new?kind=prompt`);
  await page.getByLabel("标识").fill(prompt);
  await page.getByLabel("标题").fill(`E2E 周报 ${tag}`);
  await page.getByLabel("说明").fill("把本周工作整理成周报");
  await page.locator("textarea").last().fill("按项目整理本周进展。");
  // 员工新建默认「只有我自己」（不用审核）；改成全公司才进审核
  await page.getByRole("button", { name: "全公司", exact: true }).click();
  await page.getByRole("button", { name: "提交", exact: true }).click();
  await page.waitForURL(/\/assets\/\d+$/);
  await page.getByText("等待审核，通过后才会生效").waitFor();
  step("员工提交的提示词进入审核");
  await admin.goto(`${BASE}/admin/review`);
  await admin.getByRole("button", { name: /技能、提示词与微应用/ }).click();
  await admin.getByRole("button", { name: new RegExp(`E2E 周报 ${tag}`) }).click();
  await shot(admin, "hub-review");
  await admin.getByRole("button", { name: "通过并上架" }).click();
  await admin.getByText(`已通过：E2E 周报 ${tag}`).waitFor();
  cat = await dsh(token, "GET", "/api/dsh/catalog");
  const p = cat.data.skills.find((s) => s.name === prompt);
  assert(p && p.user_invocable && !p.model_invocable, "作者的提示词通过后应在 DSH 里，只给人调用：" + JSON.stringify(cat.data));
  step("审核通过后，提示词出现在作者的 DSH 里（/命令）");

  // ---- 微应用：配置图标与打开方式，详情页和列表显示图标 ----
  await admin.goto(`${BASE}/assets/new?kind=app`);
  await admin.getByLabel("标识").fill(app);
  await admin.getByLabel("标题").fill(`E2E 报销助手 ${tag}`);
  await admin.getByLabel("说明").fill("帮你判断费用能不能报销");
  await admin.getByLabel("图标（可选）").fill("🧾");
  await admin.getByRole("button", { name: "回到上次会话", exact: true }).click();
  await admin.getByLabel("人设与工作方式").fill("你是企业报销助手，先询问费用类型，再给出报销建议。");
  await admin.getByLabel("示例问题（每行一个，最多 6 个）").fill("这张发票能报吗？");
  await admin.getByRole("button", { name: "范围内的人自动生效", exact: true }).click();
  await admin.getByRole("button", { name: "提交", exact: true }).click();
  await admin.waitForURL(/\/assets\/\d+$/);
  await admin.getByText("🧾", { exact: true }).first().waitFor();
  cat = await dsh(token, "GET", "/api/dsh/catalog");
  const catalogApp = cat.data.apps.find((a) => a.name === app);
  const legacyApp = cat.data.skills.find((s) => s.name === app);
  assert(catalogApp?.icon === "🧾" && catalogApp.launch === "resume" && catalogApp.starters[0] === "这张发票能报吗？", "DSH 微应用目录：" + JSON.stringify(cat.data));
  assert(legacyApp?.title === catalogApp.title && legacyApp.kind === "app" && legacyApp.user_invocable && !legacyApp.model_invocable, "旧 skills 条目保持兼容");
  step("微应用能保存图标与打开方式，详情页显示图标，DSH 目录单独列出且保留旧 skills 条目");
  await admin.goto(`${BASE}/apps`);
  const appRow = admin.locator("div.rounded-lg", { hasText: `E2E 报销助手 ${tag}` }).last();
  await appRow.getByText("🧾", { exact: true }).waitFor();
  await shot(admin, "hub-app-icon");

  // ---- 微应用工作台：插入示例 → 服务端拒绝不在依赖里的连接 → 改成合法配置 → 预览 → 保存并下发 ----
  const wbApp = `e2e-wb-${tag}`;
  await admin.goto(`${BASE}/assets/new?kind=app`);
  await admin.getByLabel("标识").fill(wbApp);
  await admin.getByLabel("标题").fill(`E2E 工作台 ${tag}`);
  await admin.getByLabel("说明").fill("带业务面板的微应用");
  await admin.getByLabel("人设与工作方式").fill("你是演示助手。");
  await admin.getByRole("button", { name: "范围内的人自动生效", exact: true }).click();
  await admin.getByRole("button", { name: /插入示例/ }).click();
  const wbBox = admin.getByPlaceholder('{ "panes": [ … ] }');
  assert((await wbBox.inputValue()).includes("报销工作台"), "插入示例后编辑框里应有示例工作台");
  await admin.getByRole("button", { name: "提交", exact: true }).click();
  await admin.getByText("不在这个微应用的依赖连接里").waitFor();
  step("工作台的数据源连接不在依赖里时，服务端拒绝并在页面上提示");
  await wbBox.fill(JSON.stringify({ panes: [
    { type: "panel", id: "main", title: `E2E 面板 ${tag}`, widgets: [{ type: "markdown", text: "演示说明" }, { type: "board", title: "AI 看板" }] },
    { type: "web", id: "hub", title: "能力中心", url: `${BASE}/hub` },
  ] }, null, 2));
  await admin.getByRole("button", { name: "预览", exact: true }).click();
  await admin.getByText(`E2E 面板 ${tag}`, { exact: true }).waitFor();
  await shot(admin, "hub-app-workbench");
  await admin.getByRole("button", { name: "提交", exact: true }).click();
  await admin.waitForURL(/\/assets\/\d+$/);
  cat = await dsh(token, "GET", "/api/dsh/catalog");
  const wbEntry = cat.data.apps.find((a) => a.name === wbApp);
  assert(wbEntry?.workbench?.panes?.length === 2 && wbEntry.workbench.panes[1].url === `${BASE}/hub`, "DSH 目录应带上工作台：" + JSON.stringify(wbEntry));
  step("工作台配置能预览、保存，并随目录下发");

  // ---- 企业连接：OpenAPI，指向 dsh-ee 自己的 /healthz ----
  const doc = JSON.stringify({ openapi: "3.0.0", info: { title: "Health" }, paths: { "/healthz": { get: { operationId: "health", summary: "服务健康状态" } }, "/healthz/x": { post: { operationId: "poke" } } } });
  await admin.goto(`${BASE}/assets/new?kind=connection&mode=manual`);
  await admin.getByLabel("标识").fill(conn);
  await admin.getByLabel("标题").fill(`E2E 健康检查 ${tag}`);
  await admin.getByRole("button", { name: "HTTP 接口（OpenAPI）", exact: true }).click();
  await admin.getByLabel("接口文档（OpenAPI 3，JSON 或 YAML）").fill(doc);
  await admin.getByLabel("接口根地址（可选）").fill(BASE);
  await admin.getByRole("button", { name: "范围内的人自动生效", exact: true }).click();
  await admin.getByRole("button", { name: "测试连接" }).click();
  await admin.getByText("连接成功，发现 2 个工具").waitFor();
  await shot(admin, "hub-connection-editor");
  await admin.getByRole("button", { name: "提交", exact: true }).click();
  await admin.waitForURL(/\/assets\/\d+$/);
  await admin.getByRole("button", { name: "重新同步工具" }).waitFor();
  await admin.getByText("会修改数据", { exact: true }).waitFor();
  step("接入 OpenAPI 系统：试连发现 2 个工具，保存后写操作默认关闭");
  await shot(admin, "hub-connection");

  const list = await dsh(token, "POST", "/mcp", { jsonrpc: "2.0", id: 1, method: "tools/list" });
  const names = list.data.result.tools.map((x) => x.name);
  assert(names.includes(`${conn}__health`) && !names.includes(`${conn}__poke`), "员工只应看到只读工具：" + names);
  const call = await dsh(token, "POST", "/mcp", { jsonrpc: "2.0", id: 2, method: "tools/call", params: { name: `${conn}__health`, arguments: {} } });
  assert(JSON.stringify(call.data.result).includes("status"), "工具调用结果：" + JSON.stringify(call.data));
  step("员工的 DSH 经 /mcp 调到企业系统");
  await admin.goto(`${BASE}/admin/connections`);
  await admin.getByText(`E2E 健康检查 ${tag}`).waitFor();
  await shot(admin, "hub-connections");

  // ---- 系统设置：发布 DSH 接入插件 ----
  await admin.goto(`${BASE}/admin/settings`);
  await admin.getByText("生成并发布接入插件").waitFor();
  await admin.getByRole("button", { name: "立即发布" }).click();
  await admin.getByText(/\/dsh-connect@1\.0\.\d+ ·/).waitFor();
  const info = await api(page, "GET", "/api/connect");
  assert(info.data.ready && info.data.package.endsWith("/dsh-connect"), "接入信息：" + JSON.stringify(info.data));
  step(`DSH 接入插件已发布：${info.data.package}@${info.data.version}`);
  await page.goto(`${BASE}/setup`);
  await page.getByText(`接入插件 ${info.data.package}@${info.data.version} 已发布`).waitFor();
  await shot(page, "hub-setup");

  // 收尾：下架连接，免得影响别的测试
  const assets = (await api(admin, "GET", "/api/hub")).data.assets;
  for (const a of assets.filter((x) => [skill, prompt, app, conn].includes(x.slug))) {
    await api(admin, "DELETE", `/api/hub/${a.id}`);
  }
}
