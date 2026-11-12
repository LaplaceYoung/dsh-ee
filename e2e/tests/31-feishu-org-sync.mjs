// 飞书：管理员配置与手动同步 → 登录页真实跳转 → 授权码回调 → 同步账号复用。
// 本机假 IdP 实现发现配置、JWKS、授权码+PKCE、tenant token 与通讯录，便于在没有企业测试账号时重复验证。
import http from "node:http";
import { generateKeyPairSync, randomUUID, sign as cryptoSign, webcrypto } from "node:crypto";
import { ADMIN_PW, BASE, SHOTS, api, assert, assertEq, login, logout, newPage, shot, step } from "../lib.mjs";
import path from "node:path";

const b64 = (value) => Buffer.from(value).toString("base64url");
const b64json = (value) => b64(JSON.stringify(value));

async function fakeFeishu() {
  const { privateKey, publicKey } = generateKeyPairSync("rsa", { modulusLength: 2048 });
  const publicJwk = publicKey.export({ format: "jwk" });
  const grants = new Map();
  const state = { department: { open_department_id: "dep-rd", name: "研发部" }, users: [{ open_id: "ou-1", name: "飞书员工", email: "feishu.user@example.com", department_ids: ["dep-rd"] }] };
  const sign = (claims) => {
    const data = b64json({ alg: "RS256", kid: "feishu-key", typ: "JWT" }) + "." + b64json(claims);
    return data + "." + cryptoSign("RSA-SHA256", data, privateKey).toString("base64url");
  };
  const validSecret = "idp-secret";
  const server = http.createServer(async (req, res) => {
    const url = new URL(req.url, "http://localhost");
    const json = (status, value) => {
      res.writeHead(status, { "Content-Type": "application/json" });
      res.end(JSON.stringify(value));
    };
    const body = async () => {
      let text = "";
      for await (const chunk of req) text += chunk;
      return new URLSearchParams(text);
    };
    if (url.pathname === "/.well-known/openid-configuration") {
      return json(200, { issuer: server.url, authorization_endpoint: server.url + "/authorize", token_endpoint: server.url + "/token", jwks_uri: server.url + "/jwks", userinfo_endpoint: server.url + "/userinfo" });
    }
    if (url.pathname === "/jwks") {
      return json(200, { keys: [{ kty: "RSA", kid: "feishu-key", use: "sig", alg: "RS256", n: publicJwk.n, e: publicJwk.e }] });
    }
    if (url.pathname === "/authorize") {
      const code = randomUUID();
      grants.set(code, {
        challenge: url.searchParams.get("code_challenge"),
        claims: {
          iss: server.url, aud: url.searchParams.get("client_id"), sub: "ou-1",
          preferred_username: "feishu.user@example.com", name: "飞书员工", email: "feishu.user@example.com",
          nonce: url.searchParams.get("nonce"), iat: Math.floor(Date.now() / 1000), exp: Math.floor(Date.now() / 1000) + 300,
        },
      });
      const callback = new URL(url.searchParams.get("redirect_uri"));
      callback.searchParams.set("code", code);
      callback.searchParams.set("state", url.searchParams.get("state"));
      res.writeHead(302, { Location: callback.toString() });
      return res.end();
    }
    if (url.pathname === "/token" && req.method === "POST") {
      const form = await body();
      const grant = grants.get(form.get("code"));
      if (!grant || form.get("client_secret") !== validSecret || b64(new Uint8Array(await webcrypto.subtle.digest("SHA-256", new TextEncoder().encode(form.get("code_verifier"))))) !== grant.challenge) {
        return json(400, { error: "invalid_grant" });
      }
      grants.delete(form.get("code"));
      return json(200, { id_token: sign(grant.claims), access_token: "at-ou-1", token_type: "Bearer" });
    }
    if (url.pathname === "/userinfo") {
      return json(200, { sub: "ou-1", department: "研发部" });
    }
    if (url.pathname === "/open-apis/auth/v3/tenant_access_token/internal" && req.method === "POST") {
      return json(200, { code: 0, tenant_access_token: "tenant-feishu" });
    }
    if (url.pathname.startsWith("/open-apis/contact/")) {
      if (req.headers.authorization !== "Bearer tenant-feishu") return json(401, { error: "unauthorized" });
      if (url.pathname.includes("parentchildren")) return json(200, { code: 0, data: { items: [state.department], has_more: false } });
      if (url.pathname.includes("find_by_department")) return json(200, { code: 0, data: { items: state.users, has_more: false } });
    }
    return json(404, { error: "not found", path: url.pathname });
  });
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  server.url = `http://127.0.0.1:${server.address().port}`;
  return server;
}

export default async function (t) {
  const idp = await fakeFeishu();
  try {
    const admin = await newPage(t, process.env.E2E_RECORD === "1" ? {
      viewport: { width: 1280, height: 900 },
      recordVideo: { dir: process.env.E2E_RECORD_DIR || path.join(SHOTS, "feishu-video"), size: { width: 1280, height: 900 } },
    } : {});
    const video = process.env.E2E_RECORD === "1" ? admin.video() : null;
    await login(admin, "admin", ADMIN_PW);
    await admin.goto(`${BASE}/admin/auth`);
    await admin.getByRole("button", { name: "飞书", exact: true }).click();
    const oidc = admin.locator("#auth-oidc");
    await oidc.getByRole("switch").first().click();
    await oidc.getByLabel("Issuer").fill(idp.url);
    await oidc.getByLabel("Client ID").fill("dsh-ee");
    await oidc.getByLabel("Client Secret").fill("idp-secret");
    await oidc.getByLabel("通讯录 API 地址").fill(idp.url);
    await oidc.getByRole("button", { name: "保存统一登录配置" }).click();
    await admin.getByText("已保存统一登录配置").waitFor();
    let rawConfig = await api(admin, "GET", "/api/admin/auth/oidc");
    assert(!JSON.stringify(rawConfig.data).includes("idp-secret"), "不能回显 App Secret");
    await shot(admin, "feishu-admin-config");
    step("管理员选择飞书预设，填写 OIDC、App ID / Secret 和通讯录地址，secret 不回显");

    await oidc.getByRole("button", { name: "立即同步" }).click();
    await admin.getByText("飞书组织已同步：新建 1 · 核对 0 · 禁用 0").waitFor();
    await shot(admin, "feishu-org-sync");
    step("手动同步使飞书员工和研发部获取到本地账号");

    await logout(admin);
    await admin.goto(`${BASE}/login`);
    await admin.getByRole("link", { name: "用飞书登录" }).click();
    await admin.waitForURL((url) => url.pathname !== "/login" && !url.pathname.startsWith("/api/auth/oidc"));
    const me = await api(admin, "GET", "/api/auth/me");
    assertEq(me.data.external_dn, "feishu:ou-1", "OIDC 与通讯录使用同一外部身份");
    await api(admin, "PUT", "/api/me/welcome", { dismissed: true });
    await admin.goto(`${BASE}/workbench`);
    await admin.getByText("飞书员工").first().waitFor();
    await shot(admin, "feishu-login-complete");
    step("从登录页跳飞书授权再回调，登录复用同步创建的账号");

    if (video) {
      await admin.context().close();
      console.log(`  · 录屏原片：${await video.path()}`);
    }
  } finally {
    await new Promise((resolve) => idp.close(resolve));
  }
}
