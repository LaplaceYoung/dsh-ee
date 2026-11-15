// 示例公司「星河科技」：部署清单里一键导入 → 演示员工登录能用 → 统计页有数据 → 一键清除，用户自己的内容一条不少。
import { ADMIN_PW, BASE, api, assert, assertEq, login, newPage, shot, step, tag } from "../lib.mjs";

export default async function (t) {
  const admin = await newPage(t);
  await login(admin, "admin", ADMIN_PW);
  // 别的测试可能导入过：先清干净
  await api(admin, "DELETE", "/api/admin/demo");

  await admin.goto(`${BASE}/go?setup=1`);
  await admin.getByRole("heading", { name: "导入示例公司，马上体验" }).waitFor();
  await shot(admin, "demo-hero");
  const started = Date.now();
  await admin.getByRole("button", { name: "导入示例公司" }).click();
  await admin.getByText("导入后，示例应用会出现在全员的 DSH 里").waitFor();
  await admin.locator("div.rounded-md", { hasText: "导入后，示例应用" }).getByRole("button", { name: "导入示例公司" }).click();
  await admin.getByText("演示员工账号（密码只显示这一次").waitFor({ timeout: 60_000 });
  const seconds = (Date.now() - started) / 1000;
  assert(seconds < 60, `导入应在一分钟内完成，实际 ${seconds}s`);
  await admin.getByText("接下来：").waitFor();
  await shot(admin, "demo-imported");
  step(`部署清单顶部「导入示例公司」，${seconds.toFixed(1)} 秒导入完成，给出账号与下一步`);

  const text = await admin.locator("body").innerText();
  const m = /周航（销售部 · 客户经理）\s+(demo\.zhou\.hang)\s+(demo-\S+)/.exec(text);
  assert(m, "应列出周航的账号和密码：" + text.slice(0, 500));

  // ---- 演示员工登录 ----
  const emp = await newPage(t);
  await login(emp, m[1], m[2]);
  await emp.goto(`${BASE}/apps`);
  await emp.getByText("出差报销助手").first().waitFor();
  await emp.goto(`${BASE}/build`);
  const row = emp.locator("li", { hasText: "我的客户周报" });
  await row.getByText("自己用", { exact: true }).waitFor();
  await emp.goto(`${BASE}/build/materials`);
  for (const kb of ["差旅与报销制度（示例）", "产品资料（示例）", "IT 排障手册（示例）"]) await emp.getByText(kb).first().waitFor();
  await shot(emp, "demo-employee-materials");
  step("用演示员工周航登录：企业应用、自己用的「我的客户周报」、各部门知识库都在");

  // ---- 统计页一打开就有数据 ----
  const stats = await api(admin, "GET", "/api/admin/stats?days=30&tz=480");
  assert(stats.data.totals.calls > 100 && stats.data.totals.active_users >= 5, "运营数据应有 30 天的模型调用：" + JSON.stringify(stats.data.totals));
  await admin.goto(`${BASE}/admin/stats`);
  await admin.waitForLoadState("networkidle");
  await shot(admin, "demo-stats");
  step("运营数据有 30 天的模型调用与活跃人数");

  // ---- 一键清除：用户自己的内容不受影响 ----
  const own = await api(admin, "POST", "/api/hub", { kind: "prompt", slug: `own-${tag}`, spec: { title: `我自己的 ${tag}`, description: "用户自己建的", body: "写日报" } });
  assertEq(own.status, 201, "用户自己建的提示词");
  await admin.goto(`${BASE}/admin/settings`);
  await admin.getByRole("button", { name: "清除示例数据" }).click();
  await admin.getByRole("alertdialog").getByRole("button", { name: "删除" }).click();
  await admin.getByText(/已清除：\d+ 项能力、6 个知识库、10 个演示员工/).first().waitFor({ timeout: 60_000 });
  step("系统设置「清除示例数据」：确认后删掉示例的能力、知识库与演示员工");
  const hub = await api(admin, "GET", "/api/hub");
  assert(hub.data.assets.some((a) => a.slug === `own-${tag}`), "用户自己的提示词应还在");
  assert(!hub.data.assets.some((a) => ["demo-crm", "travel-expense", "zhouhang-weekly", "renewal-advisor"].includes(a.slug)), "示例能力应删掉");
  const back = await newPage(t);
  await back.goto(`${BASE}/login`);
  await back.locator("input[autocomplete=username]").fill(m[1]);
  await back.locator("input[type=password]").fill(m[2]);
  await back.locator("button[type=submit]").click();
  await back.getByRole("alert").waitFor();
  step("清除后演示员工不能再登录，用户自己建的内容还在");
  await api(admin, "DELETE", `/api/hub/${own.data.asset.id}`);
}
