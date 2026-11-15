// 专家 · 技能 · 连接器（docs/expert-center.md）：
// 企业应用页的专家筛选 → 管理页建分类 → 归类并设精选 → 禁用策略挡住一个员工 → 权限查询指出是哪条策略；
// 员工没有管理权限：接口 403。
import { ADMIN_PW, BASE, api, assert, assertEq, login, newPage, step, tag } from "../lib.mjs";

export default async function (t) {
  const page = await newPage(t);
  await login(page, "admin", ADMIN_PW);

  const slug = `e2e-expert-${tag}`;
  const title = `报销专家${tag}`;
  const created = await api(page, "POST", "/api/hub", {
    kind: "app", slug, delivery: "auto",
    spec: { title, description: "核对报销单", persona: "你是报销专家。", form: "expert", expertise: ["报销", "差旅"], method: "先核对发票。", starters: ["这张发票能报吗"] },
  });
  assertEq(created.status, 201, "建专家");
  const expertID = created.data.asset.id;
  const skill = await api(page, "POST", "/api/hub", { kind: "skill", slug: `e2e-skill-${tag}`, delivery: "auto", spec: { title: `发票核对${tag}`, description: "核对", body: "步骤" } });
  assertEq(skill.status, 201, "建技能");
  const skillID = skill.data.asset.id;
  const user = await api(page, "POST", "/api/admin/users", { username: `e2e-bob-${tag}`, password: "e2e-bob-password-1", display_name: `Bob${tag}`, department: "财务部" });
  assertEq(user.status, 201, "建员工");
  step("准备专家、技能与员工");

  // 企业应用页：专家筛选，卡片带「专家」与擅长领域
  await page.goto(`${BASE}/apps`);
  await page.getByRole("button", { name: /^专家 \d+$/ }).click();
  await page.getByRole("link", { name: title }).waitFor();
  await page.getByText("差旅", { exact: true }).first().waitFor();
  await page.getByRole("button", { name: "新建专家" }).waitFor();
  step("企业应用页按专家筛选，卡片显示擅长领域");

  // 管理页：建分类
  await page.goto(`${BASE}/admin/expert-center`);
  await page.getByRole("heading", { name: "专家与连接器策略" }).waitFor();
  const catName = `e2e${tag}`.slice(0, 32);
  await page.getByPlaceholder("例如：财务").fill(catName);
  await page.getByRole("button", { name: "新建分类" }).click();
  await page.getByText(`已新建分类「${catName}」`).waitFor();
  const cats = (await api(page, "GET", "/api/admin/hub/categories")).data.categories;
  const cat = cats.find((c) => c.name === catName);
  assert(cat && cat.group === "app", "分类应建在「专家与应用」组");
  step("新建分类");

  // 精选与归类：选分类、点星
  await page.getByRole("tab", { name: "精选与归类" }).click();
  const row = page.locator("tr").filter({ hasText: slug });
  await row.getByRole("combobox", { name: "分类" }).selectOption(String(cat.id));
  await page.getByText("已保存").first().waitFor();
  await row.getByRole("button", { name: "精选" }).click();
  await row.locator('button[aria-pressed="true"]').waitFor();
  const asset = (await api(page, "GET", `/api/hub/${expertID}`)).data.asset;
  assertEq(asset.category_id, cat.id, "归类");
  assertEq(asset.category, catName, "分类文本同步");
  assert(asset.featured, "精选");
  step("归类并设为精选");

  // 禁用策略（接口建）→ 权限查询页指出原因与策略编号
  const block = await api(page, "POST", "/api/admin/hub/blocks", { scope: { all: false, departments: [], roles: [], users: [user.data.id] }, asset_ids: [skillID], note: `e2e ${tag}` });
  assertEq(block.status, 201, "建禁用策略");
  await page.getByRole("tab", { name: "禁用策略" }).click();
  await page.getByText(`e2e ${tag}`).waitFor();
  await page.getByRole("tab", { name: "权限查询" }).click();
  await page.getByRole("combobox", { name: "选择员工" }).click();
  await page.keyboard.type(`e2e-bob-${tag}`);
  await page.keyboard.press("Enter");
  const skillRow = page.locator("tr").filter({ hasText: `e2e-skill-${tag}` });
  await skillRow.getByText("被禁用策略挡住").waitFor();
  await skillRow.getByText(`#${block.data.id}`).waitFor();
  const expertRow = page.locator("tr").filter({ hasText: slug });
  await expertRow.getByText("已生效").waitFor();
  step("权限查询：技能被禁用策略挡住，专家已生效");

  // 清理禁用策略：界面上删
  await page.getByRole("tab", { name: "禁用策略" }).click();
  const blockRow = page.locator("tr").filter({ hasText: `e2e ${tag}` });
  await blockRow.getByRole("button", { name: "删除" }).click();
  await page.getByRole("alertdialog").getByRole("button", { name: "删除" }).click();
  await page.getByText("已删除禁用策略").waitFor();
  const blocks = (await api(page, "GET", "/api/admin/hub/blocks")).data.blocks;
  assert(!blocks.some((b) => b.id === block.data.id), "禁用策略应已删除");
  step("删除禁用策略");

  // 员工没有管理权限
  const emp = await newPage(t);
  await login(emp, `e2e-bob-${tag}`, "e2e-bob-password-1");
  assertEq((await api(emp, "GET", "/api/admin/hub/categories")).status, 403, "员工不能管分类");
  assertEq((await api(emp, "GET", `/api/admin/hub/effective?user=${user.data.id}`)).status, 403, "员工不能查权限");
  step("员工访问管理接口被拒");
}
