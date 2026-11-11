// 多份命名共享凭据：浏览器新增/替换/删除/选择；服务端真实 OpenAPI 调用使用所选凭据。
import http from "node:http";
import { ADMIN_PW, BASE, api, assert, assertEq, login, newPage, shot, step, tag } from "../lib.mjs";

export default async function (t) {
  const seen = [];
  let video = null;
  let recordingContext = null;
  const upstream = http.createServer((req, res) => {
    seen.push(req.headers["x-credential"] || "");
    res.writeHead(200, { "content-type": "application/json" });
    res.end(JSON.stringify({ ok: true }));
  });
  await new Promise((resolve) => upstream.listen(0, "127.0.0.1", resolve));
  try {
    const { port } = upstream.address();
    const page = await newPage(t, { recordVideo: process.env.E2E_RECORD === "1" ? { dir: process.env.E2E_RECORD_DIR || ".e2e-record", size: { width: 1440, height: 1000 } } : undefined });
    if (process.env.E2E_RECORD === "1") {
      video = page.video();
      recordingContext = page.context();
    }
    await login(page, "admin", ADMIN_PW);
    const slug = `named-${tag}`;
    const doc = JSON.stringify({ openapi: "3.0.0", info: { title: "Named shared" }, servers: [{ url: `http://127.0.0.1:${port}` }], paths: { "/lookup": { get: { operationId: "lookup", summary: "检查选择哪一份凭据" } } } });
    const created = await api(page, "POST", "/api/hub", { kind: "connection", slug, delivery: "auto", secret: "legacy-secret",
      spec: { title: `多份凭据 ${tag}`, type: "openapi", openapi: doc, auth: { type: "header", header: "X-Credential" } } });
    assertEq(created.status, 201, "创建共享凭据连接");
    const id = created.data.asset.id;
    const token = await api(page, "POST", "/api/me/tokens", { name: `named-token/${tag}`, scopes: ["connections"] });
    assertEq(token.status, 201, "创建连接调用令牌");
    async function call() {
      const r = await fetch(`${BASE}/mcp`, { method: "POST", headers: { authorization: `Bearer ${token.data.token}`, "content-type": "application/json", accept: "application/json, text/event-stream" }, body: JSON.stringify({ jsonrpc: "2.0", id: 1, method: "tools/call", params: { name: `${slug}__lookup`, arguments: {} } }) });
      const text = await r.text();
      assert(r.status === 200 && !text.includes("isError\":true"), "命名凭据的工具调用必须成功");
    }
    async function edit() {
      await page.goto(`${BASE}/assets/${id}/edit`);
      await page.getByTestId("named-shared-credentials").waitFor();
      await page.getByTestId("shared-credential-list").waitFor();
    }
    async function add(name, secret) {
      await page.getByRole("button", { name: "新增命名凭据" }).click();
      await page.getByLabel("名称", { exact: true }).fill(name);
      await page.getByLabel("密钥", { exact: true }).fill(secret);
      await page.getByRole("button", { name: "保存凭据", exact: true }).click();
      await page.getByTestId(`shared-credential-${name}`).waitFor();
      await page.locator("[data-testid='shared-credential-form']").waitFor({ state: "hidden" });
    }
    async function saveVersion() {
      await page.getByRole("button", { name: /^(保存|提交)$/ }).click();
      await page.waitForURL(new RegExp(`/assets/${id}$`));
      await edit();
    }

    await call();
    assertEq(seen[0], "legacy-secret", "老连接默认密钥先能真实调用");
    // 分别以一份原连接密钥和两份命名凭据真实跑调用。
    await edit();
    assertEq(await page.getByTestId("shared-credential-select").inputValue(), "", "老连接保留原默认");
    await add("prod", "prod-secret-1");
    await add("test", "test-secret-1");
    await shot(page, "named-credentials-list");
    await call();
    assertEq(seen[1], "legacy-secret", "添加命名凭据不静默更改默认");
    step("新增两份命名凭据；未切换时仍使用原连接默认");

    await page.getByTestId("shared-credential-select").selectOption("prod");
    await page.getByText(/已选择 prod.*保存这个版本/).waitFor();
    await saveVersion();
    await call();
    assertEq(seen[2], "prod-secret-1", "保存选择后使用生产凭据");
    await page.goto(`${BASE}/assets/${id}`);
    await page.locator("table").getByText("prod", { exact: true }).waitFor();
    await shot(page, "named-credential-audit");
    await edit();
    step("下拉选生产并保存版本，真实 HTTP 请求读取到生产密钥；调用记录只显示名称");

    await page.getByRole("button", { name: "替换密钥 prod" }).click();
    assertEq(await page.getByLabel("名称", { exact: true }).inputValue(), "prod", "替换表单标题保留名字");
    await page.getByLabel("密钥", { exact: true }).fill("prod-secret-2");
    await page.getByRole("button", { name: "保存凭据", exact: true }).click();
    await page.locator("[data-testid='shared-credential-form']").waitFor({ state: "hidden" });
    await shot(page, "named-credentials-rotate");
    await call();
    assertEq(seen[3], "prod-secret-2", "替换生产凭据即时生效");
    step("替换密钥不需要重新输入旧密钥，下一次真实调用使用新值");

    await page.getByTestId("shared-credential-select").selectOption("test");
    await saveVersion();
    await call();
    assertEq(seen[4], "test-secret-1", "切到测试凭据");
    page.once("dialog", (dialog) => dialog.accept());
    await page.getByRole("button", { name: "删除 test", exact: true }).click();
    await page.getByText(/正在使用命名凭据 test.*请先改选/).waitFor();
    step("真实切换到测试后拒绝删除使用中的凭据，不退回生产或默认");

    await page.getByTestId("shared-credential-select").selectOption("");
    await saveVersion();
    await call();
    assertEq(seen[5], "legacy-secret", "保存别名选择后退回默认");
    await page.getByTestId("shared-credential-list").waitFor();
    page.once("dialog", (dialog) => dialog.accept());
    await page.getByRole("button", { name: "删除 test", exact: true }).click();
    await page.getByTestId("shared-credential-test").waitFor({ state: "hidden" });
    page.once("dialog", (dialog) => dialog.accept());
    await page.getByRole("button", { name: "删除 prod", exact: true }).click();
    await page.getByTestId("shared-credential-prod").waitFor({ state: "hidden" });
    await call();
    assertEq(seen[6], "legacy-secret", "删除所有命名凭据不改变旧默认行为");
    await shot(page, "named-credentials-default");
    step("保存默认后可删除两份命名凭据；旧连接行为不受影响");
  } finally {
    await new Promise((resolve) => upstream.close(resolve));
    if (recordingContext) await recordingContext.close();
    if (video) await video.saveAs(`${process.env.E2E_RECORD_DIR || ".e2e-record"}/30-named-shared-credentials.webm`);
  }
}
