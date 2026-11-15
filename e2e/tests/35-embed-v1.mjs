// 跨站嵌入 v1：宿主页在 http://localhost:8099（与 dsh-ee 的 127.0.0.1 不同站），
// iframe 里没有 cookie；宿主后端用每用户 ee_at_ 令牌 mint 一次性 code，iframe exchange 出
// ee_emb_ 内存 Bearer 后取数。断言：握手消息、无 Set-Cookie、跨站可读内容、失败码。
//
// 需要服务端开启跨站嵌入并放行宿主来源（CI 的 e2e job 与本地跑法都要设置）：
//   DSH_EE_EMBED_CROSS_SITE=1 DSH_EE_EMBED_ORIGINS=http://localhost:8099
import http from "node:http";
import { ADMIN_PW, BASE, api, assert, assertEq, login, newPage, step, tag } from "../lib.mjs";

const HOST_PORT = Number(process.env.E2E_EMBED_HOST_PORT || 8099);
const HOST_ORIGIN = `http://localhost:${HOST_PORT}`;

/** 宿主页面 + 宿主后端（token 只在 Node 侧，不进浏览器）。 */
async function startHost(token, slug) {
  const page = `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><title>嵌入宿主</title></head>
<body style="margin:0;font-family:system-ui">
<iframe id="ee-frame" style="width:900px;height:600px;border:0;display:block"></iframe>
<script>
const EE = ${JSON.stringify(BASE)};
const SLUG = ${JSON.stringify(slug)};
const TARGET = new URL(EE).origin;
const state = { ready: null, session: null, errors: [], authRequests: [], mints: 0, lastMintError: null };
window.__embedState = state;
const frame = document.getElementById("ee-frame");
let connecting = false;
async function mintAndConnect() {
  if (connecting) return;
  connecting = true;
  try {
    const r = await fetch("/mint", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ app: SLUG }) });
    const data = await r.json();
    state.mints++;
    if (!r.ok) { state.lastMintError = data; return; }
    frame.contentWindow.postMessage({ v: 1, type: "dsh-ee:connect", code: data.code }, TARGET);
  } catch (e) {
    state.lastMintError = String(e);
  } finally {
    connecting = false;
  }
}
window.addEventListener("message", (e) => {
  if (e.origin !== TARGET || e.source !== frame.contentWindow) return;
  const m = e.data;
  if (!m || m.v !== 1) return;
  if (m.type === "dsh-ee:ready") { state.ready = m; void mintAndConnect(); }
  if (m.type === "dsh-ee:auth-required") {
    state.authRequests.push(m.reason);
    // 只有过期才重新 mint（模拟宿主只重试一次）
    if (m.reason === "expired") void mintAndConnect();
  }
  if (m.type === "dsh-ee:session") state.session = m;
  if (m.type === "dsh-ee:error") state.errors.push(m);
});
frame.src = EE + "/a/" + SLUG + "?embed=1&host=" + encodeURIComponent(location.origin) + "&fit=auto&theme=light";
</script></body></html>`;
  const server = http.createServer(async (req, res) => {
    if (req.method === "GET" && (req.url === "/" || req.url?.startsWith("/?"))) {
      res.writeHead(200, { "Content-Type": "text/html; charset=utf-8", "Cache-Control": "no-store" });
      res.end(page);
      return;
    }
    if (req.method === "POST" && req.url === "/mint") {
      let body = "";
      for await (const chunk of req) body += chunk;
      const app = body ? JSON.parse(body).app : slug;
      const r = await fetch(`${BASE}/api/embed/sessions`, {
        method: "POST",
        headers: { "Content-Type": "application/json", Authorization: `Bearer ${token}` },
        body: JSON.stringify({ app, host_origin: HOST_ORIGIN }),
      });
      const data = await r.text();
      res.writeHead(r.status, { "Content-Type": "application/json" });
      res.end(data);
      return;
    }
    res.writeHead(404).end();
  });
  // 不指定 host：Node 会监听 ::（含 IPv4）；浏览器访问 http://localhost:8099 时 IPv4/IPv6 都能连上。
  await new Promise((resolve) => server.listen(HOST_PORT, resolve));
  return { server, close: () => new Promise((resolve) => server.close(resolve)) };
}

export default async function (t) {
  const admin = await newPage(t);
  await login(admin, "admin", ADMIN_PW);
  const slug = `embed-v1-${tag}`;
  const spec = {
    title: `跨站嵌入 ${tag}`,
    description: "e2e 跨站嵌入",
    persona: "你是嵌入测试助手。",
    icon: "🌐",
    web: true,
    tables: [{ name: "notes", title: "备注", visibility: "own", fields: [{ name: "text", title: "内容", kind: "text" }] }],
    workbench: { panes: [{ id: "main", type: "panel", title: "主页", widgets: [{ type: "heading", title: "跨站内容" }] }] },
  };
  assertEq((await api(admin, "POST", "/api/hub", { kind: "app", slug, delivery: "auto", spec })).status, 201, "建跨站嵌入应用");

  // 每用户 ee_at_ 令牌（只在宿主后端用，浏览器拿不到）
  const tok = await api(admin, "POST", "/api/me/tokens", { name: `embed-${tag}`, scopes: ["agent", "hub"] });
  assertEq(tok.status, 201, "签发宿主后端令牌");
  const hostToken = tok.data.token;

  // 服务端配置没开跨站 / 没放行来源时，mint 应该被拒绝（防止 e2e 环境漏配后误判通过）
  const plain = async (path, body, bearer) => {
    const r = await fetch(`${BASE}${path}`, {
      method: "POST",
      headers: { "Content-Type": "application/json", ...(bearer ? { Authorization: `Bearer ${bearer}` } : {}) },
      body: JSON.stringify(body),
    });
    return { status: r.status, data: await r.json().catch(() => null), headers: r.headers };
  };
  const badOrigin = await plain("/api/embed/sessions", { app: slug, host_origin: "https://evil.example.com" }, hostToken);
  assertEq(badOrigin.status, 403, "来源不在白名单应 403：" + JSON.stringify(badOrigin.data));
  assertEq(badOrigin.data?.code, "origin_not_allowed", "来源错误码");

  const host = await startHost(hostToken, slug);
  try {
    // 无登录态的全新浏览器上下文：iframe 里没有任何 dsh-ee cookie
    const guest = await newPage(t);
    const exchangeResponses = [];
    guest.on("response", (r) => {
      if (r.url().includes("/api/auth/embed/exchange")) exchangeResponses.push(r);
    });
    await guest.goto(`${HOST_ORIGIN}/`);
    await guest.waitForFunction(() => window.__embedState?.session != null, null, { timeout: 25000 });
    const state = await guest.evaluate(() => window.__embedState);

    // 握手：ready 声明 connect-v1；没 cookie 时发 auth-required；宿主 mint 一次后 session
    assert(state.ready, "宿主应收到 ready");
    assertEq(state.ready.app?.slug, slug, "ready 带应用标识");
    assert(state.ready.supports?.includes("connect-v1"), "ready 声明 connect-v1：" + JSON.stringify(state.ready));
    assertEq(state.ready.locale, "zh-CN", "ready 带当前界面语言 locale");
    assert(state.authRequests.includes("missing"), "没有 cookie 时应发 auth-required(missing)：" + JSON.stringify(state.authRequests));
    assertEq(state.session.limited, true, "嵌入会话是 limited");
    assert(state.session.user?.display_name, "session 带用户展示名");
    assert(state.mints >= 1, "宿主后端 mint 过 code");
    step("跨站握手：ready(connect-v1) → auth-required → 宿主 mint code → connect → session(limited)");

    // exchange 响应不能 Set-Cookie（内存 Bearer，不依赖三方 cookie）
    assertEq(exchangeResponses.length >= 1, true, "iframe 调过 exchange");
    for (const r of exchangeResponses) {
      assertEq(r.headers()["set-cookie"] ?? "", "", "exchange 不能 Set-Cookie");
    }
    const cookies = (await guest.context().cookies()).filter((c) => c.domain.includes("127.0.0.1"));
    assertEq(cookies.length, 0, "浏览器里不应有 dsh-ee cookie：" + JSON.stringify(cookies));

    // iframe 是跨站（host 在 localhost，应用在 127.0.0.1），内容取数正常
    const frame = guest.frameLocator("#ee-frame");
    await frame.getByRole("heading", { name: `跨站嵌入 ${tag}` }).waitFor({ timeout: 15000 });
    await frame.getByRole("button", { name: "数据", exact: true }).click();
    await frame.getByText("备注").first().waitFor({ timeout: 10000 });
    const iframeURL = await guest.evaluate(() => document.getElementById("ee-frame").src);
    assert(iframeURL.startsWith(BASE), "iframe 指向 dsh-ee：" + iframeURL);
    step("跨站 iframe（localhost → 127.0.0.1）不需要 cookie 也能取数");

    // 一次性 code：同一个 code 第二次 exchange 失败
    const mint = await plain("/api/embed/sessions", { app: slug, host_origin: HOST_ORIGIN }, hostToken);
    assertEq(mint.status, 200, "mint 成功：" + JSON.stringify(mint.data));
    const first = await plain("/api/auth/embed/exchange", { code: mint.data.code });
    assertEq(first.status, 200, "exchange 成功");
    assertEq(first.headers.get("set-cookie") ?? "", "", "exchange 响应没有 Set-Cookie");
    const second = await plain("/api/auth/embed/exchange", { code: mint.data.code });
    assertEq(second.status, 400, "code 只能用一次");
    assertEq(second.data?.code, "embed_code_invalid", "重复 code 错误码");

    // 路由白名单：嵌入会话不能碰管理接口
    const embedToken = first.data.token;
    const blocked = await fetch(`${BASE}/api/admin/users`, { headers: { Authorization: `Bearer ${embedToken}` } });
    assertEq(blocked.status, 403, "白名单外的接口应 403");
    assertEq((await blocked.json()).code, "embed_scope", "白名单错误码");
    const me = await fetch(`${BASE}/api/auth/me`, { headers: { Authorization: `Bearer ${embedToken}` } });
    const meBody = await me.json();
    assertEq(meBody.limited, true, "嵌入会话 me.limited");
    assertEq(meBody.embed?.app, slug, "me.embed.app");
    assertEq(meBody.embed?.host, HOST_ORIGIN, "me.embed.host");

    // logout：撤销后同一个 token 立即 401（只撤嵌入会话）
    const logout = await fetch(`${BASE}/api/auth/embed/logout`, { method: "POST", headers: { Authorization: `Bearer ${embedToken}` } });
    assertEq(logout.status, 204, "嵌入会话 logout 204");
    const after = await fetch(`${BASE}/api/auth/me`, { headers: { Authorization: `Bearer ${embedToken}` } });
    assertEq(after.status, 401, "logout 后原 token 应 401");
    step("exchange 不 Set-Cookie；code 一次性；白名单外 403 embed_scope；logout 后 401");
  } finally {
    await host.close();
    const list = await api(admin, "GET", "/api/hub");
    const asset = list.data.assets.find((x) => x.slug === slug);
    if (asset) await api(admin, "DELETE", `/api/hub/${asset.id}`);
  }
}
