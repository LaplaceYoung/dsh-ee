// 扩展签名信任根管理页（docs/plugin-signing.md §7、#134 的后端接口）：
// 空态 → 粘贴公钥 PEM 新增 → 重复添加报错 → 换下 → 撤销（保留历史）→ 已撤销不能再操作 / 不能再加回来；
// 员工没有权限：菜单里没有这一页，接口 403。
import crypto from "node:crypto";
import { ADMIN_PW, BASE, api, assert, assertEq, login, nav, newPage, shot, step, tag } from "../lib.mjs";

/** 生成一把 Ed25519 公钥 PEM；key id 由服务端从公钥算出，测试不自己拼。 */
function ed25519PublicKeyPEM() {
  const { publicKey } = crypto.generateKeyPairSync("ed25519");
  return publicKey.export({ type: "spki", format: "pem" }).toString();
}

export default async function (t) {
  const page = await newPage(t);
  await login(page, "admin", ADMIN_PW);

  // 侧边栏「能力与连接 → 扩展签名」能进来（菜单是迁移 / 默认菜单写的）。
  // 先等菜单加载完再 nav：刚 goto 时侧边栏还是空的，nav 会误切到「工作台」那套。
  await page.goto(`${BASE}/admin`);
  await page.locator("aside").getByRole("link", { name: "扩展签名" }).first().waitFor();
  await nav(page, { name: "扩展签名" });
  await page.waitForURL((u) => u.pathname === "/admin/signing-keys");
  await page.getByRole("heading", { name: "扩展签名信任根" }).waitFor();
  await page.getByText("还没有受信任的扩展签名公钥").waitFor();
  assertEq((await api(page, "GET", "/api/admin/signing-keys")).data.count, 0, "初始信任根应为空");
  step("从管理后台菜单进入信任根页，空态");

  // 私钥不能贴：客户端先挡住，不调用接口
  await page.getByTestId("signing-key-pem").fill("-----BEGIN PRIVATE KEY-----\nAAAA\n-----END PRIVATE KEY-----");
  await page.getByTestId("signing-key-add").click();
  await page.getByText("这是私钥，不要粘贴").waitFor();
  assertEq((await api(page, "GET", "/api/admin/signing-keys")).data.count, 0, "贴私钥不应创建信任根");
  step("粘贴私钥被客户端拒绝");

  // 新增公钥：列表出现使用中的信任根（key id / 用途 / 来源 / 备注 / 生效时间）
  const pem = ed25519PublicKeyPEM();
  const note = `e2e 发布密钥 ${tag}`;
  await page.getByTestId("signing-key-pem").fill(pem);
  await page.getByTestId("signing-key-note").fill(note);
  await page.getByTestId("signing-key-add").click();
  await page.getByText(note).first().waitFor();
  const created = (await api(page, "GET", "/api/admin/signing-keys")).data;
  assertEq(created.count, 1, "新增后应有一条信任根");
  const key = created.keys[0];
  assertEq(key.state, "active", "新加的公钥状态");
  assertEq(key.purpose, "extension", "用途");
  assertEq(key.note, note, "备注");
  assert(!created.require_signature, "默认不强制验签");
  const row = page.getByTestId(`signing-key-${key.key_id}`);
  await row.getByText("使用中").waitFor();
  await row.getByText("管理页").waitFor();
  await row.getByText("扩展签名").waitFor();
  step(`新增信任根 ${key.key_id}，列表显示使用中 / 管理页 / 扩展签名`);

  // 同一把公钥重复添加：服务端 409，页面显示错误
  await page.getByTestId("signing-key-pem").fill(pem);
  await page.getByTestId("signing-key-add").click();
  await page.getByText(/已经加过了/).waitFor();
  step("重复添加被拒绝并显示错误");

  // 换下：确认对话框 → 状态变成已换下，记录还在（历史签名仍认）
  await row.getByRole("button", { name: "换下" }).click();
  await page.getByRole("alertdialog").getByRole("button", { name: "换下" }).click();
  await row.getByText("已换下").waitFor();
  let after = (await api(page, "GET", "/api/admin/signing-keys")).data;
  assertEq(after.count, 1, "换下不删记录");
  assertEq(after.keys[0].state, "retired", "换下后的状态");
  assert(!!after.keys[0].retired_at, "换下时间应记录");
  step("换下后状态为已换下，记录保留");

  // 撤销：确认 → 已撤销；撤销后这一行没有操作按钮
  await row.getByRole("button", { name: "撤销" }).click();
  await page.getByRole("alertdialog").getByRole("button", { name: "撤销" }).click();
  await row.getByText("已撤销").waitFor();
  after = (await api(page, "GET", "/api/admin/signing-keys")).data;
  assertEq(after.keys[0].state, "revoked", "撤销后的状态");
  assert(!!after.keys[0].revoked_at, "撤销时间应记录");
  assertEq(await row.getByRole("button").count(), 0, "已撤销的信任根不应再有操作按钮");
  step("撤销后状态为已撤销，操作按钮消失");

  // 撤销过的公钥不能重新加回来（服务端拒绝，页面显示错误）
  await page.getByTestId("signing-key-pem").fill(pem);
  await page.getByTestId("signing-key-add").click();
  await page.getByText(/撤销过的公钥不能重新加/).first().waitFor();
  assertEq((await api(page, "GET", "/api/admin/signing-keys")).data.count, 1, "不能重新加回来");
  step("撤销过的公钥不能重新加回来");
  await shot(page, "signing-keys");

  // 员工：菜单里没有这一页，直接访问是「没有权限」，接口 403
  const uname = `sign.${tag}`;
  const made = await api(page, "POST", "/api/admin/users", { username: uname, display_name: "E2E 签名只读", password: "qa-password-12" });
  assertEq(made.status, 201, "创建员工账号");
  const member = await newPage(t);
  await login(member, uname, "qa-password-12");
  const denied = await api(member, "GET", "/api/admin/signing-keys");
  assertEq(denied.status, 403, "员工读信任根应 403");
  await member.goto(`${BASE}/admin/signing-keys`);
  await member.getByText("页面不存在，或你没有访问权限").waitFor();
  step("员工没有信任根页和接口权限");
}
