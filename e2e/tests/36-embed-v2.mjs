// 跨站嵌入协议 v2（增量）：hello/supports、deeplink（消费 mint next）、navigate（pushState 不重载）、
// navigate-request / open-external（iframe 请求宿主跳转）、logout/logged-out（只撤嵌入会话）。
// 宿主页在 http://localhost:8099，iframe 在 127.0.0.1（双 origin，无 cookie）。
import http from "node:http";
import { ADMIN_PW, BASE, api, assert, assertEq, login, newPage, step, tag } from "../lib.mjs";

const HOST_PORT = Number(process.env.E2E_EMBED_HOST_PORT || 8099);
const HOST_ORIGIN = `http://localhost:${HOST_PORT}`;
const SUPPORTS = ["connect-v1", "navigate", "navigate-request", "open-external", "logout", "deeplink", "theme", "resize", "lang"];

async function startHost(token, slug, deeplink) {
  const page = `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><title>v2 宿主</title></head>
<body style="margin:0;font-family:system-ui">
<iframe id="ee-frame" style="width:900px;height:600px;border:0;display:block"></iframe>
<script>
const EE = ${JSON.stringify(BASE)};
const SLUG = ${JSON.stringify(slug)};
const DEEPLINK = ${JSON.stringify(deeplink)};
const TARGET = new URL(EE).origin;
const state = { hello: null, ready: null, session: null, deeplink: null, loggedOut: null, errors: [],
  authRequests: [], navigateRequests: [], openExternal: [], resizes: [], themes: [], mints: 0, loads: 0, lastMintError: null };
window.__embedState = state;
const frame = document.getElementById("ee-frame");
frame.addEventListener("load", () => { state.loads++; });
let connecting = false;
async function mintAndConnect() {
  if (connecting) return;
  connecting = true;
  try {
    const r = await fetch("/mint", { method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ app: SLUG, next: DEEPLINK }) });
    const data = await r.json();
    state.mints++;
    if (!r.ok) { state.lastMintError = data; return; }
    frame.contentWindow.postMessage({ v: 2, type: "dsh-ee:connect", code: data.code }, TARGET);
  } catch (e) {
    state.lastMintError = String(e);
  } finally {
    connecting = false;
  }
}
window.addEventListener("message", (e) => {
  if (e.origin !== TARGET || e.source !== frame.contentWindow) return;
  const m = e.data;
  if (!m || (m.v !== 1 && m.v !== 2)) return;
  if (m.type === "dsh-ee:hello") state.hello = m;
  if (m.type === "dsh-ee:ready") { state.ready = m; void mintAndConnect(); }
  if (m.type === "dsh-ee:auth-required") {
    state.authRequests.push(m.reason);
    if (m.reason === "expired") void mintAndConnect();
  }
  if (m.type === "dsh-ee:session") state.session = m;
  if (m.type === "dsh-ee:deeplink") state.deeplink = m;
  if (m.type === "dsh-ee:logged-out") state.loggedOut = m;
  if (m.type === "dsh-ee:navigate-request") state.navigateRequests.push(m.path);
  if (m.type === "dsh-ee:open-external") state.openExternal.push(m.url);
  if (m.type === "dsh-ee:resize") state.resizes.push(m);
  if (m.type === "dsh-ee:theme") state.themes.push(m);
  if (m.type === "dsh-ee:error") state.errors.push(m);
});
window.__embedNavigate = (path) => frame.contentWindow.postMessage({ v: 2, type: "dsh-ee:navigate", path }, TARGET);
window.__embedTheme = (theme) => frame.contentWindow.postMessage({ v: 1, type: "dsh-ee:theme", theme }, TARGET);
window.__embedLogout = () => frame.contentWindow.postMessage({ v: 2, type: "dsh-ee:logout" }, TARGET);
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
      const params = body ? JSON.parse(body) : {};
      const r = await fetch(`${BASE}/api/embed/sessions`, {
        method: "POST",
        headers: { "Content-Type": "application/json", Authorization: `Bearer ${token}` },
        body: JSON.stringify({ app: params.app || slug, host_origin: HOST_ORIGIN, ...(params.next ? { next: params.next } : {}) }),
      });
      const data = await r.text();
      res.writeHead(r.status, { "Content-Type": "application/json" });
      res.end(data);
      return;
    }
    res.writeHead(404).end();
  });
  await new Promise((resolve) => server.listen(HOST_PORT, resolve));
  return { server, close: () => new Promise((resolve) => server.close(resolve)) };
}

export default async function (t) {
  const admin = await newPage(t);
  await login(admin, "admin", ADMIN_PW);
  const slug = `embed-v2-${tag}`;
  const spec = {
    title: `嵌入 v2 ${tag}`,
    description: "e2e 协议 v2",
    persona: "你是嵌入测试助手。",
    icon: "🧭",
    web: true,
    tables: [{ name: "notes", title: "备注", visibility: "own", fields: [{ name: "text", title: "内容", kind: "text" }] }],
    workbench: {
      panes: [
        { id: "main", type: "panel", title: "主页", widgets: [{ type: "heading", title: "主面板内容" }] },
        { id: "side", type: "panel", title: "侧栏", widgets: [{ type: "heading", title: "侧栏内容" }] },
      ],
    },
  };
  assertEq((await api(admin, "POST", "/api/hub", { kind: "app", slug, delivery: "auto", spec })).status, 201, "建 v2 嵌入应用");
  const tok = await api(admin, "POST", "/api/me/tokens", { name: `embed-v2-${tag}`, scopes: ["agent", "hub"] });
  assertEq(tok.status, 201, "签发宿主后端令牌");

  const deeplink = `/a/${slug}?tab=side`;
  const host = await startHost(tok.data.token, slug, deeplink);
  try {
    const guest = await newPage(t);
    await guest.goto(`${HOST_ORIGIN}/`);
    await guest.waitForFunction(() => window.__embedState?.session != null, null, { timeout: 25000 });
    const state = await guest.evaluate(() => window.__embedState);

    // hello/supports：协议 v2 能力声明（增量，ready 仍保留）
    assert(state.hello, "宿主应收到 hello");
    assertEq(state.hello.protocol, 2, "hello.protocol");
    for (const s of SUPPORTS) assert(state.hello.supports?.includes(s), "hello.supports 应包含 " + s + "：" + JSON.stringify(state.hello.supports));
    assert(state.ready?.supports?.includes("connect-v1"), "ready 仍带 connect-v1");
    assert(state.ready?.supports?.includes("lang"), "ready 声明 lang");
    assertEq(state.hello.locale, "zh-CN", "hello.locale");
    assertEq(state.ready?.locale, "zh-CN", "ready.locale");
    step("v2 握手：hello/supports（9 项，含 lang）+ ready 兼容 v1 并带 locale");

    // fit=auto / 视口 / 超时：hello 与 ready 带 viewport，hello 声明 10s 握手 + 8s 宿主兜底；
    // resize 报内容高度（上限 4000）。宿主按 8s 显示手动兜底、iframe 10s 自行失败，两者对齐。
    assertEq(state.hello.viewport?.mobile, false, "hello.viewport.mobile（桌面）");
    assertEq(state.ready.viewport?.mobile, false, "ready.viewport.mobile（桌面）");
    assertEq(state.hello.timeouts?.handshake_ms, 10000, "hello.timeouts.handshake_ms");
    assertEq(state.hello.timeouts?.host_fallback_ms, 8000, "hello.timeouts.host_fallback_ms");
    await guest.waitForFunction(() => (window.__embedState?.resizes ?? []).length > 0, null, { timeout: 10000 });
    const resize = await guest.evaluate(() => window.__embedState.resizes.at(-1));
    assert(typeof resize?.height === "number" && resize.height > 0, "resize.height：" + JSON.stringify(resize));
    assert(resize.height <= 4000, "resize.height 上限 4000：" + resize.height);
    assert(typeof resize?.width === "number" && resize.width > 0, "resize.width：" + JSON.stringify(resize));
    assertEq(resize?.mobile, false, "resize.mobile（桌面）");
    step(`fit=auto：resize 报内容高度 ${resize.height}px（上限 4000），宿主 8s/iframe 10s 超时声明一致`);

    // 主题：URL theme=light 生效；宿主 dsh-ee:theme 仍可覆盖（只改 DOM）。
    assertEq(state.hello.theme, "light", "hello.theme（URL theme=light）");
    await guest.evaluate(() => window.__embedTheme("dark"));
    await guest.waitForFunction(() => window.__embedState?.themes?.at(-1)?.theme === "dark", null, { timeout: 5000 });
    await guest.frameLocator("#ee-frame").locator("html.dark").waitFor({ timeout: 5000 });
    await guest.evaluate(() => window.__embedTheme("light"));
    await guest.waitForFunction(() => window.__embedState?.themes?.at(-1)?.theme === "light", null, { timeout: 5000 });
    step("theme：URL theme=light 首屏生效，宿主 dsh-ee:theme 覆盖后 iframe 只改 DOM 并回发确认");

    // 移动端：窄屏 iframe 显示常驻「在浏览器打开」兜底，内容保留、不自动顶层打开。
    const hostURL = guest.url();
    await guest.evaluate(() => {
      document.getElementById("ee-frame").style.width = "390px";
    });
    await guest.waitForFunction(
      () => (window.__embedState?.resizes ?? []).some((r) => r.mobile === true),
      null,
      { timeout: 10000 },
    );
    const frameMobile = guest.frameLocator("#ee-frame");
    await frameMobile.locator("[data-embed-mobile]").waitFor({ timeout: 10000 });
    await frameMobile.getByRole("link", { name: "在浏览器打开" }).waitFor();
    await frameMobile.getByRole("heading", { name: /主面板内容|侧栏内容/ }).first().waitFor();
    assertEq(guest.url(), hostURL, "移动端不自动在 iframe 里顶层跳转（宿主地址不变）");
    await guest.evaluate(() => {
      document.getElementById("ee-frame").style.width = "900px";
    });
    step("移动端（<640px）：resize.mobile=true + 常驻「在浏览器打开」，内容保留且不自动顶层打开");

    // deeplink：mint 时带的 next 在 exchange 后被消费（pushState，不整页刷新）
    const frameLoc = guest.frameLocator("#ee-frame");
    await frameLoc.getByRole("heading", { name: "侧栏内容" }).waitFor({ timeout: 15000 });
    const frame0 = guest.frames().find((f) => f.url().includes(`/a/${slug}`));
    assert(frame0 && frame0.url().includes("tab=side"), "deeplink 应把 iframe 切到 ?tab=side：" + (frame0 && frame0.url()));
    assertEq(state.loads, 1, "deeplink 不能整页刷新 iframe");
    step("deeplink：消费 mint 的 next（?tab=side），pushState 不刷新");

    // navigate：宿主切到本应用另一个页签；不重载、内存 token 不丢（后续接口仍成功）
    await guest.evaluate((path) => window.__embedNavigate(path), `/a/${slug}?tab=main`);
    await frameLoc.getByRole("heading", { name: "主面板内容" }).waitFor({ timeout: 10000 });
    const frame1 = guest.frames().find((f) => f.url().includes(`/a/${slug}`));
    assert(frame1 && frame1.url().includes("tab=main"), "navigate 后 URL 应带 tab=main：" + (frame1 && frame1.url()));
    assertEq(state.loads, 1, "navigate 不能整页刷新 iframe");
    // 内存 Bearer 还在：再打开「数据」页签会请求 /api/apps/.../data，应成功（无 401）
    await frameLoc.getByRole("button", { name: "数据", exact: true }).click();
    await frameLoc.getByText("这个应用自带的数据表").waitFor({ timeout: 10000 });
    step("navigate：只允许本应用路径，pushState 切换且保留内存 Bearer");

    // navigate-request / open-external：iframe 里点击站外链接，交给宿主确认
    const frame = guest.frames().find((f) => f.url().includes(`/a/${slug}`));
    await frame.evaluate(() => {
      const mk = (href, id, text) => {
        const a = document.createElement("a");
        a.href = href;
        a.id = id;
        a.textContent = text;
        a.style.cssText = "display:block;padding:8px";
        document.body.appendChild(a);
      };
      mk("/connections/needs-approval", "ee-navreq", "申请连接");
      mk("dsh://open", "ee-external", "打开 DSH");
    });
    await frame.click("#ee-navreq");
    await guest.waitForFunction(() => window.__embedState.navigateRequests.length > 0, null, { timeout: 5000 });
    assertEq((await guest.evaluate(() => window.__embedState.navigateRequests))[0], "/connections/needs-approval", "navigate-request 带站内路径");
    await frame.click("#ee-external");
    await guest.waitForFunction(() => window.__embedState.openExternal.length > 0, null, { timeout: 5000 });
    assertEq((await guest.evaluate(() => window.__embedState.openExternal))[0], "dsh://open", "open-external 带登记 scheme");
    step("navigate-request / open-external：iframe 只发起请求，由宿主确认打开");

    // logout / logged-out：只撤销本次嵌入会话
    await guest.evaluate(() => window.__embedLogout());
    await guest.waitForFunction(() => window.__embedState.loggedOut != null, null, { timeout: 10000 });
    const out = await guest.evaluate(() => window.__embedState.loggedOut);
    assertEq(out.revoked, true, "嵌入会话应被撤销（cookie 会话不存在）");
    // 撤销后 iframe 内显示退出态，不会跳登录页
    await frameLoc.getByText(/已退出本次嵌入会话/).waitFor({ timeout: 10000 });
    const pathAfter = await guest.evaluate(() => document.getElementById("ee-frame").src);
    assert(pathAfter.includes(`/a/${slug}`), "logout 后 iframe 不跳登录页");
    step("logout：撤销嵌入会话并回 logged-out(revoked=true)，不碰网页会话、不跳登录");
  } finally {
    await host.close();
    const list = await api(admin, "GET", "/api/hub");
    const asset = list.data.assets.find((x) => x.slug === slug);
    if (asset) await api(admin, "DELETE", `/api/hub/${asset.id}`);
  }
}
