// 声明式扩展：上传 ZIP → 预检 → 配置共享凭据 → 安装 → 元数据 → 启用写工具 → 实际 MCP 调用。
// 清单先固定类目为 vpn、优先级从高；即使 MCP 请求伪造 category/priority，也以服务端补齐的值为准。
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { spawnSync } from "node:child_process";
import { ADMIN_PW, BASE, SHOTS, api, assert, assertEq, login, newPage, shot, step, tag } from "../lib.mjs";

function zipExtension(manifest, document) {
  const python = `
import base64, io, zipfile
manifest = base64.b64decode(__import__('os').environ['EE_MANIFEST'])
document = base64.b64decode(__import__('os').environ['EE_DOCUMENT'])
with zipfile.ZipFile('/tmp/declarative-extension.zip', 'w', zipfile.ZIP_DEFLATED) as z:
    z.writestr('dsh-ee.extension.json', manifest)
    z.writestr('openapi.yaml', document)
`;
  const built = spawnSync("python3", ["-c", python], { env: { ...process.env, EE_MANIFEST: Buffer.from(manifest).toString("base64"), EE_DOCUMENT: Buffer.from(document).toString("base64") } });
  assertEq(built.status, 0, "生成扩展 ZIP");
  const file = "/tmp/declarative-extension.zip";
  const data = fs.readFileSync(file);
  fs.rmSync(file, { force: true });
  return data;
}

async function rpc(page, token, method, params) {
  return page.evaluate(async ({ token, method, params }) => {
    const r = await fetch("/mcp", {
      method: "POST",
      headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
      body: JSON.stringify({ jsonrpc: "2.0", id: 1, method, params }),
    });
    return { status: r.status, data: await r.json() };
  }, { token, method, params });
}

export default async function (t) {
  const admin = await newPage(t, process.env.E2E_RECORD === "1" ? {
    viewport: { width: 1280, height: 900 },
    recordVideo: { dir: process.env.E2E_RECORD_DIR || path.join(SHOTS, "declarative-video"), size: { width: 1280, height: 900 } },
  } : {});
  const video = process.env.E2E_RECORD === "1" ? admin.video() : null;
  await login(admin, "admin", ADMIN_PW);
  const extensionID = `community/demo-${tag}`;
  const manifest = JSON.stringify({
    id: extensionID, version: "1.0.0", name: `IT 工单（声明式安装测试）`, description: "安装与真实调用验收",
    kind: "declarative", openapi: "openapi.yaml", servers: [BASE],
    credential: { mode: "user_secret", hint: "在测试系统生成自己的 token", header: "X-User", shared: { type: "header", header: "X-Demo-Key" } },
    config: [{ key: "tenant", type: "secret", label: "租户密钥", required: true }],
    tools: [
      { name: "list_tickets", readOnly: true, keep: ["items"] },
      { name: "create_ticket", params: [
        { name: "body.category", label: "故障分类", fill: "app", default: "vpn", enum: ["vpn", "mail"] },
        { name: "body.priority", label: "优先级", type: "string", fill: "admin", default: "中", options: ["中", "高"] },
      ] },
    ],
  });
  const document = `openapi: 3.0.3
info: {title: 声明式安装测试, version: 1.0.0}
servers: [{url: '${BASE}'}]
paths:
  /demo/itsm/tickets:
    get: {operationId: list_tickets}
    post:
      operationId: create_ticket
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [title, requester]
              properties:
                title: {type: string}
                category: {type: string}
                requester: {type: string}
                priority: {type: string}
                detail: {type: string}
`;
  const archive = zipExtension(manifest, document);

  await admin.goto(`${BASE}/admin/connections`);
  await admin.getByRole("button", { name: "安装声明式扩展" }).click();
  const dialog = admin.getByRole("dialog");
  await dialog.locator("input[type=file]").setInputFiles({ name: "demo-extension.zip", mimeType: "application/zip", buffer: archive });
  await dialog.getByText(`${extensionID}@1.0.0`).first().waitFor();
  await dialog.getByLabel("请求根地址").selectOption(BASE);
  await dialog.getByLabel(/连接标识/).fill("demo-itsm");
  await dialog.getByLabel(/租户密钥/).fill("e2e-config-secret");
  await dialog.getByLabel(/共享凭据/).fill("e2e-demo-secret");
  await dialog.getByLabel("优先级").selectOption({ label: "高" });
  await shot(admin, "declarative-extension-check");
  await dialog.getByRole("button", { name: "确认安装" }).click();
  await admin.waitForURL(/\/assets\/\d+$/);
  await admin.getByText("IT 工单（声明式安装测试）").first().waitFor();
  await admin.getByText(`${extensionID} · v1.0.0`).waitFor();
  step("上传 ZIP 预检，确认服务器、secret 配置与共享凭据，安装并显示扩展身份/digest");

  const assetID = Number(new URL(admin.url()).pathname.split("/").pop());
  const createRow = admin.locator("li", { hasText: "create_ticket" });
  const toggle = createRow.getByRole("switch");
  assertEq(await toggle.getAttribute("aria-checked"), "false", "写工具默认停用");
  await toggle.click();
  await admin.waitForFunction((el) => el.getAttribute("aria-checked") === "true", await toggle.elementHandle());
  step("管理员显式打开写工具，服务端状态已保存");
  await shot(admin, "declarative-extension-installed");
  step("写工具默认停用，由管理员显式启用");

  const personal = await api(admin, "PUT", `/api/me/connections/${assetID}/credential`, { secret: "e2e-employee-secret" });
  assertEq(personal.status, 204, "保存员工个人凭据");
  const issued = await api(admin, "POST", "/api/me/tokens", { name: "declarative-e2e", scopes: ["connections"] });
  assertEq(issued.status, 201, "生成调用令牌");
  const token = issued.data.token;
  const listed = await rpc(admin, token, "tools/list", null);
  const tools = listed.data.result.tools;
  const writeTool = tools.find((tool) => tool.name === "demo-itsm__create_ticket");
  assert(writeTool, "启用后应能看到 create_ticket");
  const bodyProps = writeTool.inputSchema.properties.body.properties;
  assert(!("category" in bodyProps) && !("priority" in bodyProps), "fill:app/admin 不能暴露给模型");
  assert("title" in bodyProps && "requester" in bodyProps, "模型仍可填写标题与报修人");
  step("Agent 的 JSON Schema 已删掉固定参数，保留模型可填字段");

  const created = await rpc(admin, token, "tools/call", {
    name: "demo-itsm__create_ticket",
    arguments: { body: { title: `声明式工单 ${tag}`, requester: "郑小雨", category: "mail", priority: "低", detail: "验证安装后的真实调用" } },
  });
  assertEq(created.status, 200, "调用 create_ticket");
  const text = JSON.stringify(created.data.result);
  assert(text.includes("VPN 连不上") && text.includes("高"), `固定参数应覆盖模型伪造值：${text}`);
  assert(!text.includes("mail"), `模型伪造的 category 不能生效：${text}`);
  const listedResult = await rpc(admin, token, "tools/call", { name: "demo-itsm__list_tickets", arguments: {} });
  const ticketsText = JSON.stringify(listedResult.data.result);
  const firstText = listedResult.data.result.content?.[0]?.text ?? "";
  const filtered = JSON.parse(firstText);
  assertEq(Object.keys(filtered).join(","), "items", "keep 应只保留 items，不能丢掉列表或留下其他根字段");
  step("MCP 实际创建工单：category 固定为 VPN，优先级由管理员改为高，keep 只保留 items 根字段");

  await shot(admin, "declarative-extension-runtime");
  const installation = await api(admin, "GET", `/api/hub/${assetID}`);
  assert(!JSON.stringify(installation.data).includes("e2e-config-secret"), "详情接口不得回显安装 secret");
  await api(admin, "DELETE", `/api/hub/${assetID}`);
  if (video) {
    await admin.context().close();
    console.log(`  · 录屏原片：${await video.path()}`);
  }
}
