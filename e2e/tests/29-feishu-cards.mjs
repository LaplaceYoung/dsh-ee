// 飞书通知与卡片：员工提交资产 → 审核人收卡 → 回调通过 → 防重复回调 → 工作台免登打开网页应用。
import http from "node:http";
import { generateKeyPairSync, randomUUID, sign as cryptoSign, webcrypto } from "node:crypto";
import { ADMIN_PW, BASE, SHOTS, api, assert, assertEq, login, newPage, shot, step, tag } from "../lib.mjs";
import path from "node:path";

const b64 = (value) => Buffer.from(value).toString("base64url");
const b64json = (value) => b64(JSON.stringify(value));
const openID = "ou-reviewer";

async function startFakeFeishu() {
  const { privateKey, publicKey } = generateKeyPairSync("rsa", { modulusLength: 2048 });
  const jwk = publicKey.export({ format: "jwk" });
  const grants = new Map();
  const server = http.createServer(async (req, res) => {
    server.messages ??= [];
    const url = new URL(req.url, "http://localhost");
    const json = (status, body) => { res.writeHead(status, { "Content-Type": "application/json" }); res.end(JSON.stringify(body)); };
    const readBody = async () => { let raw = ""; for await (const chunk of req) raw += chunk; return raw; };
    const signClaims = (claims) => b64json({ alg: "RS256", kid: "key", typ: "JWT" }) + "." + b64json(claims) + "." + cryptoSign("RSA-SHA256",
      b64json({ alg: "RS256", kid: "key", typ: "JWT" }) + "." + b64json(claims), privateKey).toString("base64url");
    if (url.pathname === "/.well-known/openid-configuration") {
      return json(200, { issuer: server.url, authorization_endpoint: `${server.url}/authorize`, token_endpoint: `${server.url}/token`, jwks_uri: `${server.url}/jwks`, userinfo_endpoint: `${server.url}/userinfo` });
    }
    if (url.pathname === "/jwks") return json(200, { keys: [{ kty: "RSA", kid: "key", alg: "RS256", n: jwk.n, e: jwk.e }] });
    if (url.pathname === "/authorize") {
      const code = randomUUID();
      grants.set(code, { challenge: url.searchParams.get("code_challenge"), claims: {
        iss: server.url, aud: url.searchParams.get("client_id"), sub: openID,
        preferred_username: "feishu.reviewer", name: "飞书审批人", email: "reviewer@example.com",
        nonce: url.searchParams.get("nonce"), iat: Math.floor(Date.now() / 1000), exp: Math.floor(Date.now() / 1000) + 300,
      } });
      const next = new URL(url.searchParams.get("redirect_uri"));
      next.searchParams.set("code", code); next.searchParams.set("state", url.searchParams.get("state"));
      res.writeHead(302, { Location: next.toString() });
      return res.end();
    }
    if (url.pathname === "/token" && req.method === "POST") {
      const form = new URLSearchParams(await readBody());
      const grant = grants.get(form.get("code"));
      if (!grant || form.get("client_secret") !== "idp-secret" || b64(new Uint8Array(await webcrypto.subtle.digest("SHA-256", new TextEncoder().encode(form.get("code_verifier"))))) !== grant.challenge) {
        return json(400, { error: "invalid_grant" });
      }
      grants.delete(form.get("code"));
      return json(200, { id_token: signClaims(grant.claims), access_token: `at-${openID}`, token_type: "Bearer" });
    }
    if (url.pathname === "/userinfo") return json(200, { sub: openID, department: "研发部" });
    if (url.pathname === "/open-apis/auth/v3/tenant_access_token/internal" && req.method === "POST") return json(200, { code: 0, tenant_access_token: "tenant-feishu" });
    if (url.pathname.includes("/departments/parentchildren")) return json(200, { code: 0, data: { items: [{ open_department_id: "dep-rd", name: "研发部" }], has_more: false } });
    if (url.pathname.includes("/users/find_by_department")) return json(200, { code: 0, data: { items: [{ open_id: openID, name: "飞书审批人", email: "reviewer@example.com", department_ids: ["dep-rd"] }], has_more: false } });
    if (url.pathname === "/open-apis/im/v1/messages" && req.method === "POST") {
      if (req.headers.authorization !== "Bearer tenant-feishu") return json(401, { error: "unauthorized" });
      const payload = JSON.parse(await readBody());
      payload.message_id = `om-${server.messages.length + 1}`;
      server.messages.push(payload);
      return json(200, { code: 0, data: { message_id: payload.message_id } });
    }
    return json(404, { error: "not_found", path: url.pathname });
  });
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  server.url = `http://127.0.0.1:${server.address().port}`;
  return server;
}

export default async function (t) {
  const fake = await startFakeFeishu();
  try {
    const admin = await newPage(t, process.env.E2E_RECORD === "1" ? {
      viewport: { width: 1280, height: 900 },
      recordVideo: { dir: process.env.E2E_RECORD_DIR || path.join(SHOTS, "feishu-card-video"), size: { width: 1280, height: 900 } },
    } : {});
    const video = process.env.E2E_RECORD === "1" ? admin.video() : null;
    await login(admin, "admin", ADMIN_PW);
    const roles = await api(admin, "GET", "/api/admin/roles");
    const superRole = roles.data.roles.find((r) => r.code === "admin");
    assert(superRole, "应存在管理员角色");
    const reviewer = await api(admin, "POST", "/api/admin/users", { username: "reviewer", display_name: "飞书审批人", department: "研发部", auth_source: "oidc" });
    assertEq(reviewer.status, 201, "创建飞书审批人");
    const grant = await api(admin, "PUT", `/api/admin/users/${reviewer.data.id}/roles`, { role_ids: [superRole.id] });
    assertEq(grant.status, 200, "设置审批人角色");
    const author = await api(admin, "POST", "/api/admin/users", { username: `card-author-${tag}`, password: "employee-password-1", display_name: "卡片作者", department: "研发部" });
    assertEq(author.status, 201, "创建普通提交人");
    const employee = await newPage(t);
    await login(employee, `card-author-${tag}`, "employee-password-1");
    const cfg = await api(admin, "PUT", "/api/admin/auth/oidc", {
      enabled: true, preset: "feishu", name: "飞书", issuer: fake.url, clientId: "dsh-ee", clientSecret: "idp-secret",
      feishuApiBase: fake.url, feishuSync: true, feishuSyncDisable: false, feishuNotify: true, feishuNotifyActions: true, autoCreate: true,
    });
    assertEq(cfg.status, 200, "开启飞书通知与卡片审批");
    let synced = await api(admin, "POST", "/api/admin/auth/feishu/sync");
    assertEq(synced.status, 200, "绑定审批人 open_id");

    const submit = await api(employee, "POST", "/api/hub", {
      kind: "skill", slug: `card-skill-${tag}`,
      spec: { title: `飞书卡片技能 ${tag}`, description: "待审批", body: "内容" },
    });
    assertEq(submit.status, 201, "员工提交待审技能");
    const assetID = submit.data.asset.id;

    const card = fake.messages.find((m) => m.msg_type === "interactive");
    assert(card, "应发出实际交互卡片");
    const document = JSON.parse(card.content);
    const buttons = document.body.components.find((c) => c.tag === "action");
    assert(buttons?.actions?.length === 2, "卡片应含通过和驳回按钮");
    assert(buttons.actions.every((button) => button.behaviors?.some((behavior) => behavior.type === "callback")), "按钮必须显式声明 callback behavior");
    const actionID = buttons.actions[0].value.action_id;
    const callback = await admin.evaluate(async ({ actionID }) => {
      const r = await fetch("/api/auth/feishu/card", { method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ type: "card.action.trigger", event_id: "evt-once", operator_id: "ou-reviewer", message_id: "om-1", action_tag: "button", action_value: { action_id: actionID, decision: "approve" } }) });
      return { status: r.status, data: await r.json() };
    }, { actionID });
    assertEq(callback.status, 200, "真实飞书回调通过审批");
    assertEq(callback.data.status, "approved", "审批结果固定为 approved");
    const published = await api(admin, "GET", `/api/hub/${assetID}`);
    assertEq(published.data.versions.find((v) => v.version === 1).status, "published", "卡片批准后版本上架");
    step("真实发送卡片，审批回调通过，版本上架");

    const replay = await admin.evaluate(async ({ actionID }) => {
      const r = await fetch("/api/auth/feishu/card", { method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ type: "card.action.trigger", event_id: "evt-replay", operator_id: "ou-reviewer", message_id: "om-1", action_tag: "button", action_value: { action_id: actionID, decision: "approve" } }) });
      return r.status;
    }, { actionID });
    assertEq(replay, 409, "同一动作不能重复审批");
    await shot(admin, "feishu-card-approved");
    step("同一动作重放返回 409，没有重复生效");

    const app = await api(admin, "POST", "/api/hub", {
      kind: "app", slug: `card-app-${tag}`,
      spec: {
        title: `飞书工作台应用 ${tag}`,
        description: "从飞书免登打开",
        persona: "x",
        web: true,
        workbench: {
          panes: [{
            id: "main", type: "panel", title: "工作台",
            widgets: [{ type: "markdown", title: "飞书工作台", text: "已通过 OIDC 从飞书登录并打开应用" }],
          }],
        },
      },
    });
    assertEq(app.status, 201, "创建网页应用");
    const link = await api(admin, "GET", `/api/admin/auth/feishu/workspace-url?next=/a/card-app-${tag}`);
    assertEq(link.status, 200, "取得工作台入口");
    await admin.goto(link.data.url);
    await admin.getByText("已通过 OIDC 从飞书登录并打开应用").waitFor();
    await shot(admin, "feishu-workbench-app");
    step("从飞书工作台入口打开 /a/<应用>，不需要重复输入 dsh-ee 密码");

    if (video) {
      await admin.context().close();
      console.log(`  · 录屏原片：${await video.path()}`);
    }
  } finally {
    await new Promise((resolve) => fake.close(resolve));
  }
}
