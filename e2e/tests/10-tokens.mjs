// 统一令牌：设备码页面批准与拒绝、吊销失效、机器人令牌和按令牌统计。
import http from "node:http";
import { ADMIN_PW, BASE, api, assert, assertEq, login, newPage, shot, step, tag } from "../lib.mjs";

async function bearer(token, method, path, body) {
  const response = await fetch(BASE + path, {
    method,
    headers: { Authorization: `Bearer ${token}`, ...(body ? { "Content-Type": "application/json" } : {}) },
    body: body ? JSON.stringify(body) : undefined,
  });
  const text = await response.text();
  return { status: response.status, data: text ? JSON.parse(text) : null };
}

async function deviceStart(clientName) {
  const response = await fetch(`${BASE}/api/device/code`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ client_name: clientName, client_os: "E2E macOS", client_version: "0.2.1-rc.5" }),
  });
  assertEq(response.status, 200, "发起设备码");
  return response.json();
}

async function deviceToken(flow) {
  const response = await fetch(`${BASE}/api/device/token`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ device_code: flow.device_code }),
  });
  return { status: response.status, data: await response.json() };
}

export default async function (t) {
  const upstream = http.createServer((req, res) => {
    req.resume();
    req.on("end", () => {
      res.setHeader("Content-Type", "application/json");
      res.end(JSON.stringify({ id: "token-e2e", choices: [{ message: { role: "assistant", content: "ok" } }], usage: { prompt_tokens: 1200, completion_tokens: 800, total_tokens: 2000 } }));
    });
  });
  await new Promise((resolve) => upstream.listen(0, "127.0.0.1", resolve));
  const admin = await newPage(t);
  const employee = await newPage(t);
  const username = `token-emp-${tag}`;
  const password = "token-employee-password-1";
  const department = `E2E-${tag}`;
  const model = `token-chat-${tag}`;
  let providerId;
  try {
    await login(admin, "admin", ADMIN_PW);
    assertEq((await api(admin, "POST", "/api/admin/users", { username, password, department })).status, 201, "创建员工");
    await login(employee, username, password);

    const flow = await deviceStart(`MacBook-${tag}`);
    await employee.goto(`${BASE}/device?code=${encodeURIComponent(flow.user_code)}`);
    await employee.getByText(`MacBook-${tag}`, { exact: true }).waitFor();
    await employee.getByText("0.2.1-rc.5", { exact: true }).waitFor();
    await shot(employee, "phase-a-device-before");
    await employee.getByRole("button", { name: "授权", exact: true }).click();
    await employee.getByText("已授权", { exact: true }).waitFor();
    await shot(employee, "phase-a-device-approved");
    const granted = await deviceToken(flow);
    assertEq(granted.status, 200, "领取设备令牌");
    const token = granted.data.access_token;
    assert(token.startsWith("ee_at_"), "设备令牌前缀");
    assertEq((await deviceToken(flow)).status, 400, "设备令牌只能领取一次");
    assertEq((await bearer(token, "GET", "/v1/models")).status, 200, "新令牌读取模型");
    assertEq((await bearer(token, "GET", "/api/dsh/catalog")).status, 200, "新令牌读取 DSH 目录");
    step("设备确认页批准，新令牌可调模型目录与 DSH 目录，且只能领取一次");

    await employee.goto(`${BASE}/tokens`);
    await employee.getByRole("heading", { name: "我的令牌" }).waitFor();
    await shot(employee, "phase-a-my-tokens");
    const tokens = await api(employee, "GET", "/api/me/tokens");
    const info = tokens.data.tokens.find((item) => item.prefix === token.slice(0, 12));
    assert(info, "我的令牌里能看到设备令牌");
    assertEq((await api(admin, "POST", "/api/admin/tokens/revoke", { ids: [info.id] })).status, 200, "管理员吊销设备令牌");
    const revoked = await bearer(token, "GET", "/api/dsh/catalog");
    assertEq(revoked.status, 401, "吊销后状态");
    assertEq(revoked.data.code, "token_revoked", "吊销错误码");
    step("管理员吊销后立即返回 401 token_revoked");

    const deniedFlow = await deviceStart(`Denied-${tag}`);
    await employee.goto(`${BASE}/device`);
    await employee.getByPlaceholder("KQ7M-4XPD").fill(deniedFlow.user_code.replace("-", "").toLowerCase());
    await employee.getByRole("button", { name: "继续", exact: true }).click();
    await employee.getByText(`Denied-${tag}`, { exact: true }).waitFor();
    await employee.getByRole("button", { name: "拒绝", exact: true }).click();
    await employee.getByText("已拒绝", { exact: true }).waitFor();
    await shot(employee, "phase-a-device-denied");
    assertEq((await deviceToken(deniedFlow)).data.error, "access_denied", "拒绝后的轮询");
    step("手动确认码容忍小写与无连字符，拒绝后返回 access_denied");

    await admin.goto(`${BASE}/admin/users`);
    await admin.getByRole("button", { name: "新建用户" }).click();
    await admin.getByRole("button", { name: "机器人账号", exact: true }).click();
    await admin.getByPlaceholder("zhang.wei").fill(`ci-bot-${tag}`);
    await admin.getByLabel("显示名", { exact: true }).fill("E2E CI Bot");
    await admin.getByLabel("部门", { exact: true }).fill(department);
    await admin.locator("form select").first().selectOption({ index: 1 });
    await shot(admin, "phase-a-bot-user-form");
    await admin.getByRole("button", { name: "创建用户" }).click();
    await admin.getByPlaceholder("搜索用户名、显示名、部门或角色").fill(`ci-bot-${tag}`);
    await admin.getByText("E2E CI Bot", { exact: true }).waitFor();
    await shot(admin, "phase-a-bot-user");
    const users = await api(admin, "GET", "/api/admin/users");
    const bot = users.data.users.find((user) => user.username === `ci-bot-${tag}`);
    assertEq(bot.account_type, "bot", "机器人类型");
    const botLogin = await fetch(`${BASE}/api/auth/login`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ username: bot.username, password: "not-a-password" }) });
    assert(botLogin.status >= 400, "机器人不能会话登录");
    step("网页创建机器人并指定负责人，机器人不能登录");

    const provider = await api(admin, "POST", "/api/admin/providers", { name: `token-fake-${tag}`, base_url: `http://127.0.0.1:${upstream.address().port}`, api_key: "e2e-upstream-secret", models: [model] });
    assertEq(provider.status, 201, "创建假上游");
    providerId = provider.data.id;
    assertEq((await api(admin, "PATCH", `/api/admin/users/${bot.id}/quota`, { monthly_token_limit: 100000, allowed_models: [model], quota_expires_at: null })).status, 200, "机器人额度");
    const issued = await api(admin, "POST", "/api/admin/tokens", { user_id: bot.id, name: `CI-${tag}`, scopes: ["models", "registry:read", "hub", "connections"] });
    assertEq(issued.status, 201, "下发机器人令牌");
    assertEq(issued.data.info.client_kind, "third_party", "管理员签发默认第三方 Harness");
    const botToken = issued.data.token;
    const chat = await bearer(botToken, "POST", "/v1/chat/completions", { model, messages: [{ role: "user", content: "hello" }] });
    assertEq(chat.status, 200, "机器人调用模型");
    const stats = await api(admin, "GET", "/api/admin/stats?group_by=token");
    const stat = stats.data.token_stats.find((item) => item.name === `CI-${tag}` && item.username === bot.username);
    assert(stat && stat.account_type === "bot" && stat.tokens === 2000, "按令牌统计机器人用量：" + JSON.stringify(stats.data.token_stats));
    step("机器人令牌调用 2000 token，统计归到对应 bot 与令牌");

    await admin.goto(`${BASE}/admin/tokens`);
    await admin.getByRole("heading", { name: "令牌管理" }).waitFor();
    await admin.getByText("第三方 Harness").first().waitFor();
    await shot(admin, "phase-a-admin-tokens");
    await admin.getByPlaceholder("部门名称").fill(department);
    await admin.getByRole("button", { name: "吊销整个部门的令牌" }).click();
    await admin.getByText(new RegExp(`确定吊销「${department}」的 \\d+ 把有效令牌吗？`)).waitFor();
    await shot(admin, "phase-a-admin-tokens-bulk");
    await admin.keyboard.press("Escape");
    await admin.getByLabel("批量吊销方式", { exact: true }).selectOption("user");
    await admin.locator('button[role="combobox"]').first().click();
    await admin.getByPlaceholder("搜索…").fill(bot.username);
    await admin.locator("[cmdk-item]").first().click();
    await admin.getByText("将吊销 1 把有效令牌", { exact: true }).waitFor();
    await admin.getByRole("button", { name: "吊销此账号的令牌" }).click();
    await admin.getByRole("alertdialog").getByRole("button", { name: "吊销" }).click();
    await admin.getByRole("alertdialog").waitFor({ state: "hidden" });
    assertEq((await bearer(botToken, "GET", "/api/dsh/catalog")).status, 401, "按账号吊销后立即失效");
    await admin.getByLabel("批量吊销方式", { exact: true }).selectOption("device");
    await admin.getByPlaceholder("例如 MacBook-Pro").waitFor();
    await admin.getByText("将吊销 0 把有效令牌", { exact: true }).waitFor();
    step("管理端按账号预览并吊销，按设备模式可筛选数量");
    await admin.goto(`${BASE}/setup`);
    await admin.getByText(/setup\.sh \| sh/).first().waitFor();
    await shot(admin, "phase-a-setup");
    await admin.goto(`${BASE}/admin/audit`);
    await admin.getByText("来源", { exact: true }).first().waitFor();
    await shot(admin, "phase-a-audit-source");

    // config 范围（dsh-ee config 命令行）：服务端提示「在我的令牌里建一个带 config 范围的令牌」，页面要真的能建；
    // 列表里显示成文字，不是 tokens.scopes.config；只给能管理字段或角色的人
    await employee.goto(`${BASE}/tokens`);
    await employee.getByRole("button", { name: "生成令牌", exact: true }).click();
    await employee.getByRole("checkbox", { name: "模型", exact: true }).waitFor();
    assertEq(await employee.getByRole("checkbox", { name: "权限配置（dsh-ee config）", exact: true }).count(), 0, "普通员工看不到 config 范围");
    await admin.goto(`${BASE}/tokens`);
    await admin.getByRole("button", { name: "生成令牌", exact: true }).click();
    await admin.getByLabel("名称", { exact: true }).fill(`config-${tag}`);
    await admin.getByRole("checkbox", { name: "权限配置（dsh-ee config）", exact: true }).check();
    await admin.locator("form").getByRole("button", { name: "生成令牌", exact: true }).click();
    await admin.getByText("令牌已生成", { exact: true }).waitFor();
    const configToken = (await admin.locator("pre").filter({ hasText: /^ee_at_/ }).first().innerText()).trim();
    await admin.locator("td", { hasText: "权限配置（dsh-ee config）" }).first().waitFor();
    assertEq(await admin.getByText("tokens.scopes.config").count(), 0, "令牌列表不应出现原始翻译键");
    const exported = await fetch(`${BASE}/api/admin/config`, { headers: { Authorization: `Bearer ${configToken}` } });
    assertEq(exported.status, 200, "config 令牌能导出权限配置");
    assert((await exported.text()).startsWith("# dsh-ee"), "导出的是权限配置（YAML）");
    await shot(admin, "tokens-config-scope");
    step("能管理字段或角色的人能在「我的令牌」里建 config 令牌，列表显示文字，令牌能调 dsh-ee config 的接口");
  } finally {
    upstream.close();
    if (providerId) await api(admin, "DELETE", `/api/admin/providers/${providerId}`);
  }
}
