// 管理端嵌入设置（E1）+ 运行概览（E5）：设置页分区可见、保存后 CSP 立即生效、非管理员看不到；
// env 只读 + DB 覆盖的展示；运行概览的空态/刷新/错误态，以及真实会话、拒绝、注销后的聚合数字。
// 概览相关用例需要服务端开启跨站嵌入（CI e2e 已设 DSH_EE_EMBED_CROSS_SITE=1）。
import { ADMIN_PW, BASE, api, assert, assertEq, login, newPage, step, tag } from "../lib.mjs";

const envOrigins = (process.env.DSH_EE_EMBED_ORIGINS || "").split(",").map((x) => x.trim()).filter(Boolean);

/** 宿主后端直连 dsh-ee 的两个嵌入接口：每用户令牌与一次性 code 都不进浏览器页面。 */
async function mint(token, body) {
  const r = await fetch(`${BASE}/api/embed/sessions`, {
    method: "POST",
    headers: { "Content-Type": "application/json", Authorization: `Bearer ${token}` },
    body: JSON.stringify(body),
  });
  return { status: r.status, data: await r.json().catch(() => null) };
}

async function exchange(code) {
  const r = await fetch(`${BASE}/api/auth/embed/exchange`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ code }),
  });
  return { status: r.status, data: await r.json().catch(() => null) };
}

export default async function (t) {
  const admin = await newPage(t);
  await login(admin, "admin", ADMIN_PW);

  // ---- 设置页分区与 env 基线 ----
  await admin.goto(`${BASE}/admin/settings#settings-embed`);
  const section = admin.locator("#settings-embed");
  await section.waitFor({ timeout: 15000 });
  await section.getByText("跨站嵌入", { exact: true }).waitFor();
  await admin.waitForTimeout(800); // 等 settings、embed-settings 与运行概览三个请求回来
  for (const origin of envOrigins) {
    assert((await section.innerText()).includes(origin), "设置页应展示部署环境来源 " + origin);
  }
  step("系统设置里的「跨站嵌入」分区可见，并展示只读的部署来源：" + envOrigins.join(", "));

  // ---- 运行概览：空态、刷新按钮、接口失败的错误态与重试 ----
  const overview = section.getByTestId("embed-overview");
  await overview.waitFor({ timeout: 10000 });
  assert((await overview.innerText()).includes("运行概览"), "概览卡片应显示标题");
  await overview.getByText("暂无活跃的嵌入会话。").waitFor({ timeout: 10000 });
  const refreshBtn = overview.getByRole("button", { name: "刷新", exact: true });
  assert(await refreshBtn.isVisible(), "概览应有刷新按钮");
  // 拦掉接口再刷新：错误态要显示「读取运行概览失败」与重试按钮，不能一直转圈。
  await admin.route("**/api/admin/embed/stats**", (route) => route.abort());
  await refreshBtn.click();
  await overview.getByText("读取运行概览失败").waitFor({ timeout: 10000 });
  await admin.unroute("**/api/admin/embed/stats**");
  await overview.getByRole("button", { name: "重试", exact: true }).click();
  await overview.getByText("暂无活跃的嵌入会话。").waitFor({ timeout: 10000 });
  step("运行概览：空态、刷新按钮、接口失败的错误态与重试都正常");

  // ---- 追加来源：保存后 CSP 立即生效（/a/* 加来源；/admin 仍 'self'） ----
  const dynamicOrigin = "https://dynamic.example.com";
  const originsArea = section.locator("textarea").first();
  await originsArea.fill(dynamicOrigin);
  await section.getByText("跨站嵌入", { exact: true }).click(); // blur 触发保存
  await admin.waitForTimeout(2200); // StoreProvider 600ms 合并保存 + 服务端返回
  const csp = async (path) => (await fetch(`${BASE}${path}`)).headers.get("content-security-policy");
  const appCSP = await csp("/a/some-app");
  assert(appCSP.includes(dynamicOrigin), "保存后 /a/* 的 CSP 应立即包含新来源：" + appCSP);
  for (const origin of envOrigins) assert(appCSP.includes(origin), "env 来源仍在 CSP 里：" + origin);
  // 未设置 DSH_EE_CSP 时默认 enforce：管理页带基线策略，但仍只允许同站嵌入。
  const adminCSP = await csp("/admin/menus");
  assert(
    adminCSP.startsWith("default-src 'self';") && adminCSP.endsWith("frame-ancestors 'self'") && !adminCSP.includes(dynamicOrigin),
    "管理页不能放行嵌入来源：" + adminCSP,
  );
  const saved = await api(admin, "GET", "/api/admin/embed-settings");
  assertEq(saved.status, 200, "读嵌入有效配置");
  assert(saved.data.effective.allowedOrigins.includes(dynamicOrigin), "有效来源应包含 DB 追加值：" + JSON.stringify(saved.data.effective.allowedOrigins));
  step("管理页追加来源保存后：CSP 立即带上新来源，/admin 仍只允许同站");

  // ---- 会话 TTL（分钟）：保存后 mint 使用 DB 值 ----
  const ttlInput = section.locator("input[type=number]").first();
  await ttlInput.fill("90");
  await section.getByText("跨站嵌入", { exact: true }).click();
  await admin.waitForTimeout(2200);
  const settings = await api(admin, "GET", "/api/settings");
  assertEq(settings.status, 200, "读设置");
  assertEq(settings.data.embed?.sessionMinutes, 90, "DB 里应保存 sessionMinutes=90");
  const effective = await api(admin, "GET", "/api/admin/embed-settings");
  assertEq(effective.data.effective.sessionSeconds, 90 * 60, "有效 TTL 应为 90 分钟");
  step("会话有效期改为 90 分钟：保存成功，有效配置立即变化");

  // ---- 非管理员看不到这一段 ----
  const user = `embed-settings-ui-${tag}`;
  assertEq((await api(admin, "POST", "/api/admin/users", { username: user, password: "employee-password-1", department: "研发部" })).status, 201, "建员工");
  const emp = await newPage(t);
  await login(emp, user, "employee-password-1");
  await emp.goto(`${BASE}/admin/settings#settings-embed`);
  await emp.waitForTimeout(800);
  assertEq(await emp.locator("#settings-embed").count(), 0, "非管理员不应看到嵌入设置分区");
  const empSettings = await api(emp, "GET", "/api/settings");
  assertEq(empSettings.data.embed?.allowedOrigins?.length ?? 0, 0, "非管理员读设置时嵌入段应隐藏");
  assertEq((await api(emp, "GET", "/api/admin/embed/stats")).status, 403, "非管理员不能读嵌入运行概览");
  step("非管理员：设置页没有嵌入分区，/api/settings 不返回来源，运行概览接口 403");

  // ---- 运行概览：真实会话 / 拒绝 / 注销的聚合 ----
  const slug = `embed-admin-${tag}`;
  const spec = {
    title: `嵌入概览 ${tag}`,
    description: "e2e 嵌入运行概览",
    persona: "你是嵌入测试助手。",
    icon: "📊",
    web: true,
    tables: [{ name: "notes", title: "备注", visibility: "own", fields: [{ name: "text", title: "内容", kind: "text" }] }],
    workbench: { panes: [{ id: "main", type: "panel", title: "主页", widgets: [{ type: "heading", title: "概览" }] }] },
  };
  assertEq((await api(admin, "POST", "/api/hub", { kind: "app", slug, delivery: "auto", spec })).status, 201, "建概览用应用");
  const tok = await api(admin, "POST", "/api/me/tokens", { name: `embed-admin-${tag}`, scopes: ["agent", "hub"] });
  assertEq(tok.status, 201, "签发宿主后端令牌");
  const hostToken = tok.data.token;

  // dynamicOrigin 已在上面保存进数据库允许来源；两次 mint+exchange 建立两个会话。
  const m1 = await mint(hostToken, { app: slug, host_origin: dynamicOrigin });
  assertEq(m1.status, 200, "mint 应成功（e2e 需要 DSH_EE_EMBED_CROSS_SITE=1）：" + JSON.stringify(m1.data));
  const x1 = await exchange(m1.data.code);
  assertEq(x1.status, 200, "exchange 应成功：" + JSON.stringify(x1.data));
  const m2 = await mint(hostToken, { app: slug, host_origin: dynamicOrigin });
  assertEq(m2.status, 200, "第二次 mint 应成功");
  const x2 = await exchange(m2.data.code);
  assertEq(x2.status, 200, "第二次 exchange 应成功");
  // 一次被拒绝的 mint：24h 拒绝计数 +1（审计 embed.deny）。
  const denied = await mint(hostToken, { app: slug, host_origin: "https://evil.example.com" });
  assertEq(denied.status, 403, "非白名单来源 mint 应 403");
  // 注销一个会话：今日吊销 +1，活跃会话回到 1。
  const logoutResp = await fetch(`${BASE}/api/auth/embed/logout`, {
    method: "POST",
    headers: { "Content-Type": "application/json", Authorization: `Bearer ${x2.data.token}` },
    body: "{}",
  });
  assertEq(logoutResp.status, 204, "注销嵌入会话应 204");

  const tz = await admin.evaluate(() => -new Date().getTimezoneOffset());
  const refreshed = admin.waitForResponse((r) => r.url().includes("/api/admin/embed/stats") && r.status() === 200);
  await overview.getByRole("button", { name: "刷新", exact: true }).click();
  await refreshed;
  await overview.getByText(slug).waitFor({ timeout: 10000 });
  assert((await overview.innerText()).includes("1 个会话"), "top apps 应显示该应用 1 个活跃会话");

  const stats = await api(admin, "GET", `/api/admin/embed/stats?tz=${tz}`);
  assertEq(stats.status, 200, "读嵌入运行概览");
  assertEq(stats.data.active_sessions, 1, "注销一个后活跃会话应为 1");
  assert(stats.data.created_today >= 2, "今日创建应至少 2：" + stats.data.created_today);
  assert(stats.data.revoked_today >= 1, "今日吊销应至少 1：" + stats.data.revoked_today);
  assert(stats.data.denied_24h >= 1, "24h 拒绝应至少 1：" + stats.data.denied_24h);
  assertEq(stats.data.top_apps?.[0]?.app, slug, "top apps 第一名应是该应用");
  assertEq(stats.data.top_apps?.[0]?.sessions, 1, "top apps 活跃数应为 1");
  const raw = JSON.stringify(stats.data);
  for (const secret of [hostToken, x1.data.token, x2.data.token, "evil.example.com"]) {
    assert(!raw.includes(secret), "运行概览不应泄露 " + secret.slice(0, 12) + "…");
  }
  step("运行概览：活跃/今日创建/今日吊销/24h 拒绝/top apps 聚合正确，且不含令牌与宿主来源");
}
