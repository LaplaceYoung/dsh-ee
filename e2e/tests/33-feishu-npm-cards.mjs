// NPM 插件审核卡：真实 OIDC 与 npm CLI 发布 → 审核网页 → 本地卡片按钮 → 回调改写版本状态。
// 卡片页面是明确标注的本地 Feishu 兼容验证界面，不冒充生产 Feishu 租户。
import http from "node:http";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { execFile } from "node:child_process";
import { promisify } from "node:util";
import { generateKeyPairSync, randomUUID, sign as signCrypto, webcrypto } from "node:crypto";
import { ADMIN_PW, BASE, api, assert, assertEq, login, newPage, shot, step, tag } from "../lib.mjs";

const run = promisify(execFile);
const b64 = (v) => Buffer.from(v).toString("base64url");
const b64json = (v) => b64(JSON.stringify(v));
const requestToken = "vt-npm-card";

async function fakeFeishuServer() {
  const { privateKey, publicKey } = generateKeyPairSync("rsa", { modulusLength: 2048 });
  const jwk = publicKey.export({ format: "jwk" });
  const grants = new Map();
  const server = http.createServer(async (req, res) => {
    server.messages ??= [];
    server.loginAs ??= "ou-reviewer";
    const profiles = {
      "ou-reviewer": { name: "飞书审批人", email: "reviewer@example.com" },
      "ou-author": { name: "插件作者", email: "npm.publisher@example.com" },
    };
    const current = () => profiles[server.loginAs];
    const url = new URL(req.url, "http://localhost");
    const json = (status, body) => { res.writeHead(status, { "Content-Type": "application/json" }); res.end(JSON.stringify(body)); };
    const text = (status, body) => { res.writeHead(status, { "Content-Type": "text/html; charset=utf-8" }); res.end(body); };
    const readBody = async () => { let raw = ""; for await (const chunk of req) raw += chunk; return raw; };
    if (url.pathname === "/.well-known/openid-configuration") {
      return json(200, { issuer: server.url, authorization_endpoint: `${server.url}/authorize`, token_endpoint: `${server.url}/token`, jwks_uri: `${server.url}/jwks`, userinfo_endpoint: `${server.url}/userinfo` });
    }
    if (url.pathname === "/jwks") return json(200, { keys: [{ kty: "RSA", kid: "npm-card", alg: "RS256", n: jwk.n, e: jwk.e }] });
    if (url.pathname === "/authorize") {
      const code = randomUUID();
      grants.set(code, { challenge: url.searchParams.get("code_challenge"), nonce: url.searchParams.get("nonce") });
      const next = new URL(url.searchParams.get("redirect_uri"));
      next.searchParams.set("code", code); next.searchParams.set("state", url.searchParams.get("state"));
      res.writeHead(302, { Location: next.toString() });
      return res.end();
    }
    if (url.pathname === "/token") {
      const form = new URLSearchParams(await readBody());
      const grant = grants.get(form.get("code"));
      const src = form.get("code_verifier");
      const hash = b64(new Uint8Array(await webcrypto.subtle.digest("SHA-256", new TextEncoder().encode(src))));
      if (!grant || form.get("client_secret") !== "idp-secret" || hash !== grant.challenge) return json(400, { error: "invalid_grant" });
      const payload = { iss: server.url, aud: "dsh-ee", sub: server.loginAs, preferred_username: current().email, name: current().name, email: current().email, nonce: grant.nonce, iat: Math.floor(Date.now() / 1000), exp: Math.floor(Date.now() / 1000) + 300 };
      const header = b64json({ alg: "RS256", kid: "npm-card", typ: "JWT" });
      const idToken = header + "." + b64json(payload) + "." + signCrypto("RSA-SHA256", header + "." + b64json(payload), privateKey).toString("base64url");
      grants.delete(form.get("code"));
      return json(200, { id_token: idToken, access_token: server.loginAs, token_type: "Bearer" });
    }
    if (url.pathname === "/userinfo") return json(200, { sub: server.loginAs, department: "研发部" });
    if (url.pathname === "/open-apis/auth/v3/tenant_access_token/internal") return json(200, { code: 0, tenant_access_token: "tenant-npm-test" });
    if (url.pathname.includes("/departments/parentchildren")) return json(200, { code: 0, data: { items: [{ open_department_id: "dep-rd", name: "研发部" }], has_more: false } });
    if (url.pathname.includes("/users/find_by_department")) {
      return json(200, { code: 0, data: { items: Object.entries(profiles).map(([open_id, p]) => ({ open_id, ...p, department_ids: ["dep-rd"] })), has_more: false } });
    }
    if (url.pathname === "/open-apis/im/v1/messages" && req.method === "POST") {
      if (req.headers.authorization !== "Bearer tenant-npm-test") return json(401, { error: "unauthorized" });
      const payload = JSON.parse(await readBody());
      payload.message_id = `om-${server.messages.length + 1}`;
      server.messages.push(payload);
      return json(200, { code: 0, data: { message_id: payload.message_id } });
    }
    // 本地可操作的卡片预览。实际发出的 doc 被读取，而不是另外伪造一个包名或 action id。
    if (url.pathname.startsWith("/fake/cards/")) {
      const escape = (v) => String(v).replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[c]);
      const id = url.pathname.split("/")[3];
      const card = server.messages.find((m) => m.message_id === id && m.msg_type === "interactive");
      if (!card) return text(404, "<h1>卡片不存在</h1>");
      const doc = JSON.parse(card.content);
      const actions = doc.body.components.find((c) => c.tag === "action")?.actions || [];
      const title = doc.header.title.content;
      const detail = doc.body.components.find((c) => c.tag === "markdown")?.content || "";
      if (req.method === "POST" && url.pathname.endsWith("/act")) {
        const form = new URLSearchParams(await readBody());
        const decision = form.get("decision");
        const button = actions.find((a) => a.value.decision === decision);
        if (!button || form.get("action_id") !== button.value.action_id) return text(400, "<h1>无效的按钮</h1>");
        const callback = await fetch(`${BASE}/api/auth/feishu/card`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({
          schema: "2.0", header: { event_id: randomUUID(), token: requestToken, event_type: "card.action.trigger", app_id: "dsh-ee" },
          event: { operator: { open_id: card.receive_id }, action: { tag: "button", value: { action_id: button.value.action_id, decision } }, context: { open_message_id: card.message_id } },
        }) });
        const result = await callback.json();
        const toast = result.toast || { type: callback.ok ? "info" : "error", content: result.error || "无法处理" };
        const color = toast.type === "success" ? "#067647" : toast.type === "info" ? "#175cd3" : "#b42318";
        const next = `<script>setTimeout(()=>{document.body.innerHTML='<main style="font:20px sans-serif;max-width:780px;margin:40px auto"><h2 style="color:${color}">${escape(toast.type)}</h2><p>${escape(toast.content)}</p><small>来自 dsh-ee 的实际回调结果 · 本地验证</small></main>';},50)</script>`;
        return text(200, next);
      }
      const buttons = actions.map((a) => `<form method="post" action="/fake/cards/${encodeURIComponent(id)}/act" style="display:inline"><input type="hidden" name="action_id" value="${escape(a.value.action_id)}"><button name="decision" value="${escape(a.value.decision)}" style="padding:10px 28px;margin:14px;border:0;border-radius:6px;color:white;background:${a.value.decision === "approve" ? "#1570ef" : "#d92d20"}">${escape(a.value.decision === "approve" ? "通过" : "驳回")}</button></form>`).join("");
      return text(200, `<!doctype html><meta charset="utf-8"><main style="font:16px sans-serif;max-width:780px;margin:36px auto;border:1px solid #d0d5dd;border-radius:12px;padding:24px"><h1>${escape(title)}</h1><pre style="font:16px sans-serif;white-space:pre-wrap">${escape(detail)}</pre><div>${buttons}</div><small>本地 Feishu 兼容验证卡片 · 点击触发真实 dsh-ee 回调，不代表生产租户已验收</small></main>`);
    }
    return json(404, { error: "not_found", path: url.pathname });
  });
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  server.url = `http://127.0.0.1:${server.address().port}`;
  return server;
}

export default async function (t) {
  const fake = await fakeFeishuServer();
  let work = "";
  const recording = process.env.E2E_RECORD === "1";
  const recordDir = process.env.E2E_RECORD_DIR || path.join(os.tmpdir(), "dsh-ee-record");
  if (recording) fs.mkdirSync(recordDir, { recursive: true });
  try {
    const admin = await newPage(t);
    await login(admin, "admin", ADMIN_PW);
    const pubRole = await api(admin, "POST", "/api/admin/roles", { code: "npm-card-publisher", name: "NPM 发布者", permissions: ["registry.publish"] });
    assertEq(pubRole.status, 201, "创建普通发布者角色");
    // 审核人需要审核管理权及查看目录/卡片链接；仍不是超级管理员，也不能代作者发布。
    const reviewRole = await api(admin, "POST", "/api/admin/roles", { code: "npm-card-reviewer", name: "NPM 审批人", permissions: ["plugins.manage", "portal.view"] });
    assertEq(reviewRole.status, 201, "创建插件审批角色");
    const reviewerAccount = await api(admin, "POST", "/api/admin/users", { username: "reviewer", display_name: "飞书审批人", auth_source: "oidc" });
    assertEq(reviewerAccount.status, 201, "预建审批人");
    assertEq((await api(admin, "PUT", `/api/admin/users/${reviewerAccount.data.id}/roles`, { role_ids: [reviewRole.data.id] })).status, 200, "给审批人实际权限");
    const configured = await api(admin, "PUT", "/api/admin/auth/oidc", {
      enabled: true, preset: "feishu", name: "飞书", issuer: fake.url, clientId: "dsh-ee", clientSecret: "idp-secret",
      feishuApiBase: fake.url, feishuSync: true, feishuSyncDisable: false, feishuNotify: true, feishuNotifyActions: true,
      feishuVerificationToken: requestToken, autoCreate: true,
    });
    assertEq(configured.status, 200, "启用飞书通知和卡片");
    assertEq((await api(admin, "POST", "/api/admin/auth/feishu/sync")).status, 200, "同步目录与身份");
    const settings = await api(admin, "GET", "/api/settings");
    settings.data.scope = "@acme";
    settings.data.baseUrl = BASE;
    assertEq((await api(admin, "PUT", "/api/admin/settings", settings.data)).status, 200, "限定企业 package scope");

    const reviewer = await newPage(t, recording ? { viewport: { width: 1440, height: 1000 }, recordVideo: { dir: recordDir, size: { width: 1440, height: 1000 } } } : {});
    const video = recording ? reviewer.video() : null;
    const dwell = () => recording ? reviewer.waitForTimeout(750) : Promise.resolve();
    await reviewer.goto(`${BASE}/login`);
    fake.loginAs = "ou-reviewer";
    await reviewer.getByRole("link", { name: "用飞书登录" }).click();
    await reviewer.waitForURL((url) => url.origin === BASE && !url.pathname.startsWith("/login") && !url.pathname.startsWith("/api/auth/oidc"));
    await reviewer.goto(`${BASE}/admin/review`);
    await reviewer.getByRole("heading", { name: /审核/ }).first().waitFor();

    const publisher = await newPage(t);
    await publisher.goto(`${BASE}/login`);
    fake.loginAs = "ou-author";
    await publisher.getByRole("link", { name: "用飞书登录" }).click();
    await publisher.waitForURL((url) => url.origin === BASE && !url.pathname.startsWith("/login") && !url.pathname.startsWith("/api/auth/oidc"));
    const userList = await api(admin, "GET", "/api/admin/users");
    const author = userList.data.users.find((u) => u.username === "npm.publisher");
    assert(author, "目录与 OIDC 应复用同一个 npm.publisher 身份");
    assertEq((await api(admin, "PUT", `/api/admin/users/${author.id}/roles`, { role_ids: [pubRole.data.id] })).status, 200, "授予真实的普通发布权限");
    const tokenResult = await api(publisher, "POST", "/api/me/npm-tokens", { name: `feishu-card-cli-${tag}`, access: "publish" });
    assertEq(tokenResult.status, 201, "创建只用于发布的令牌");
    const publishToken = tokenResult.data.token;

    const packageName = `@acme/feishu-card-${tag}`;
    const latestCard = () => [...fake.messages].reverse().find((m) => m.msg_type === "interactive");
    const publish = async (version) => {
      const cmd = process.platform === "win32" ? "npm.cmd" : "npm";
      const npmrc = path.join(work, "npmrc");
      const host = new URL(BASE).host;
      fs.writeFileSync(npmrc, `@acme:registry=${BASE}/npm/\n//${host}/npm/:_authToken=${publishToken}\n`, { mode: 0o600 });
      fs.writeFileSync(path.join(work, "package.json"), JSON.stringify({ name: packageName, version, description: "NPM 卡片 E2E", "dsh.bundle": "index.js", license: "MIT" }, null, 2));
      fs.writeFileSync(path.join(work, "index.js"), "module.exports = {};\n");
      await run(cmd, ["publish", "--registry", `${BASE}/npm`, "--loglevel", "error"], { cwd: work, timeout: 45000, env: { ...process.env, NPM_CONFIG_USERCONFIG: npmrc, HOME: work, NPM_CONFIG_UPDATE_NOTIFIER: "false" }, windowsHide: true });
      const card = latestCard();
      assert(card, `${version} 应真正发出待审卡片`);
      const doc = JSON.parse(card.content);
      const actions = doc.body.components.find((c) => c.tag === "action")?.actions;
      assertEq(actions?.length, 2, "卡片包含通过和驳回");
      assert(actions.every((button) => button.behaviors?.some((behavior) => behavior.type === "callback")), "按钮必须声明真实 callback behavior");
      assert(JSON.stringify(doc).includes(`${packageName}@${version}`), "卡片标题对应准确的包名与版本");
      assert(!JSON.stringify(doc).includes("idp-secret"), "卡片不得带应用密钥");
      return { card, doc };
    };

    work = fs.mkdtempSync(path.join(os.tmpdir(), "feishu-npm-card-"));
    const first = await publish("1.0.0");
    await reviewer.reload();
    await reviewer.getByText(packageName, { exact: true }).first().waitFor();
    await reviewer.getByRole("button", { name: "通过并上架" }).first().waitFor();
    await shot(reviewer, "feishu-npm-pending");
    await dwell();
    step("真实 npm CLI 发布 + OIDC 发布者，飞书收到待审卡，网页审核队列确实出现该版本");

    await reviewer.goto(`${fake.url}/fake/cards/${first.card.message_id}`);
    await reviewer.getByRole("heading", { name: `${packageName}@1.0.0 · 插件待审核` }).waitFor();
    await shot(reviewer, "feishu-npm-card");
    await dwell();
    await reviewer.getByRole("button", { name: "通过", exact: true }).click();
    await reviewer.getByText("已通过", { exact: false }).first().waitFor();
    await dwell();
    // api() 复用页面同源会话；卡片预览在本地 Feishu 域名，读取后台状态前先回到 dsh-ee。
    await reviewer.goto(`${BASE}/admin/review`);
    const detail = await api(reviewer, "GET", `/api/admin/packages/${encodeURIComponent(packageName)}`);
    assertEq(detail.status, 200, "批准后读取插件详情");
    assert(Array.isArray(detail.data.versions), "详情应包含版本列表：" + JSON.stringify(detail.data));
    assertEq(detail.data.versions.find((v) => v.version === "1.0.0").status, "published", "卡片按钮批准的是库存中的这个 NPM 版本");
    await reviewer.goto(`${BASE}/plugins/${encodeURIComponent(packageName)}`);
    await reviewer.getByText(/已上架|已发布/, { exact: true }).first().waitFor();
    await shot(reviewer, "feishu-npm-approved");
    step("真实点击本地卡片按钮，发送 Feishu schema 2.0 回调，版本上架并在网页可见");

    await reviewer.goto(`${fake.url}/fake/cards/${first.card.message_id}`);
    await reviewer.getByRole("button", { name: "通过", exact: true }).click();
    await reviewer.getByText("已经处理过", { exact: false }).waitFor();
    step("再次点同一卡片只给出已处理提示，不会重新审批");

    const second = await publish("1.1.0");
    await reviewer.goto(`${fake.url}/fake/cards/${second.card.message_id}`);
    await dwell();
    await reviewer.getByRole("button", { name: "驳回", exact: true }).click();
    await reviewer.getByText("已驳回", { exact: false }).first().waitFor();
    await dwell();
    await reviewer.goto(`${BASE}/admin/review`);
    const rejected = await api(reviewer, "GET", `/api/admin/packages/${encodeURIComponent(packageName)}`);
    assertEq(rejected.status, 200, "驳回后读取插件详情");
    assert(Array.isArray(rejected.data.versions), "驳回后详情应包含版本列表：" + JSON.stringify(rejected.data));
    assertEq(rejected.data.versions.find((v) => v.version === "1.1.0").status, "rejected", "驳回状态与实际版本绑定");
    const authorResults = fake.messages.filter((m) => m.msg_type === "text" && m.receive_id === "ou-author" && m.content.includes(packageName));
    assert(authorResults.some((m) => m.content.includes("已通过") && m.content.includes("1.0.0")), "发布者应收到实际通过结果");
    assert(authorResults.some((m) => m.content.includes("已驳回") && m.content.includes("1.1.0")), "发布者应收到实际驳回结果");
    step("第二版卡片驳回，发布者同时收到通过和驳回的结果通知");

    if (video) {
      await reviewer.context().close();
      await video.saveAs(path.join(recordDir, "feishu-npm-review.webm"));
    }
  } finally {
    if (work) fs.rmSync(work, { recursive: true, force: true });
    await new Promise((resolve) => fake.close(resolve));
  }
}
