// 微应用同站 iframe 嵌入 v0：embed 模式隐藏返回/管理/导出等入口；postMessage 上报
// ready/resize/error/theme；宿主 -> iframe 的 theme 只改 DOM；未登录不在 iframe 里跳登录。
// 宿主页用同源页面（/go、/login）注入 iframe：同源天然满足 frame-ancestors 'self'，
// 配置了 DSH_EE_EMBED_ORIGINS 后 /a/* 的 CSP 追加来源由 Go 单测覆盖。
import { ADMIN_PW, BASE, api, assert, assertEq, login, newPage, step, tag } from "../lib.mjs";

const iframeSrc = (slug, extra = "") => `${BASE}/a/${slug}?embed=1&host=${encodeURIComponent(BASE)}${extra}`;
const messages = (page) => page.evaluate(() => (window.__embedMsgs ?? []).map((m) => m.data));

/** 把 iframe 注入当前页面，并开始收集宿主收到的 postMessage。 */
async function openHost(page, url, id = "embed-frame") {
  await page.evaluate(
    ([url, id]) => {
      window.__embedMsgs = [];
      if (!window.__embedListening) {
        window.__embedListening = true;
        window.addEventListener("message", (e) => window.__embedMsgs.push({ origin: e.origin, data: e.data }));
      }
      const f = document.createElement("iframe");
      f.id = id;
      f.src = url;
      f.style.cssText = "width: 900px; height: 600px; border: 0; display: block;";
      document.body.appendChild(f);
      window.__embedFrame = f;
    },
    [url, id],
  );
}

async function waitMsg(page, type, timeout = 20000) {
  await page.waitForFunction((t) => (window.__embedMsgs ?? []).some((m) => m.data?.type === t), type, { timeout });
  const all = await messages(page);
  return all.filter((m) => m?.type === type).at(-1);
}

export default async function (t) {
  const admin = await newPage(t);
  await login(admin, "admin", ADMIN_PW);
  const slug = `embed-${tag}`;
  const spec = {
    title: `嵌入版 ${tag}`,
    description: "e2e 嵌入模式",
    persona: "你是嵌入测试助手。",
    icon: "🧩",
    web: true,
    tables: [{ name: "notes", title: "备注", visibility: "own", fields: [{ name: "text", title: "内容", kind: "text" }] }],
    workbench: { panes: [{ id: "main", type: "panel", title: "主页", widgets: [{ type: "heading", title: "嵌入内容" }] }] },
  };
  assertEq((await api(admin, "POST", "/api/hub", { kind: "app", slug, delivery: "auto", spec })).status, 201, "建嵌入应用");

  // CSP 按路径收口：/a/* 可追加 DSH_EE_EMBED_ORIGINS 里的来源；其它 HTML 永远只允许同站。
  // 未设置 DSH_EE_CSP 时默认 enforce：这里同时带内置基线策略与 frame-ancestors。
  const csp = async (path) => (await fetch(`${BASE}${path}`)).headers.get("content-security-policy");
  const configured = (process.env.DSH_EE_EMBED_ORIGINS || "").split(",").map((x) => x.trim()).filter(Boolean);
  const appCSP = await csp(`/a/${slug}`);
  assert(appCSP.startsWith("default-src 'self';"), "微应用页应带默认基线策略：" + appCSP);
  assert(appCSP.includes("frame-ancestors 'self'"), "微应用页 CSP 至少允许同站：" + appCSP);
  if (configured.length) {
    for (const origin of configured) assert(appCSP.includes(origin), "/a/* 应放行配置的来源 " + origin + "：" + appCSP);
  }
  const adminCSP = await csp("/admin/whatever");
  assert(adminCSP.endsWith("frame-ancestors 'self'"), "管理页只允许同站：" + adminCSP);
  for (const origin of configured) assert(!adminCSP.includes(origin), "管理页不能放行配置的来源 " + origin + "：" + adminCSP);
  step("CSP 收口：/a/* 带配置来源，/admin 始终只允许同站（路径差异见 Go 单测）");

  // ---- 宿主页注入 iframe：ready + 主题 + 隐藏入口 + 页签保留 ----
  await admin.goto(`${BASE}/go`);
  await openHost(admin, iframeSrc(slug, "&fit=auto&theme=dark"));
  const hello = await waitMsg(admin, "dsh-ee:hello");
  assertEq(hello?.protocol, 2, "hello.protocol");
  assertEq(hello?.viewport?.mobile, false, "hello 带视口（桌面上不是移动端）");
  assertEq(hello?.timeouts?.handshake_ms, 10000, "hello 声明 iframe 握手超时 10s");
  assertEq(hello?.timeouts?.host_fallback_ms, 8000, "hello 声明宿主兜底 8s");
  const ready = await waitMsg(admin, "dsh-ee:ready");
  assertEq(ready?.v, 1, "协议版本");
  assertEq(ready?.app?.slug, slug, "ready 带应用标识");
  assertEq(ready?.theme, "dark", "ready 带初始主题");
  assertEq(ready?.viewport?.mobile, false, "ready 带视口（桌面上不是移动端）");
  assert(ready?.supports?.includes("connect-v1"), "ready 声明 connect-v1：" + JSON.stringify(ready));
  const session = await waitMsg(admin, "dsh-ee:session");
  assertEq(session?.limited, false, "同站 cookie 场景 session.limited 为 false");

  const frame = admin.frameLocator("#embed-frame");
  await frame.getByRole("heading", { name: `嵌入版 ${tag}` }).waitFor();
  await frame.getByRole("button", { name: "数据", exact: true }).waitFor();
  assertEq(await frame.getByRole("link", { name: "返回企业应用" }).count(), 0, "嵌入模式隐藏返回");
  assertEq(await frame.getByRole("link", { name: "管理", exact: true }).count(), 0, "嵌入模式隐藏管理入口");
  await admin.waitForFunction(() => window.__embedFrame?.contentDocument?.documentElement?.classList.contains("dark") === true, null, { timeout: 5000 });
  step("宿主收到 ready；标题/页签保留，返回与管理入口隐藏；初始深色主题生效");

  // 数据页签里的 CSV 导出入口也必须隐藏（管理员平时会看到）
  await frame.getByRole("button", { name: "数据", exact: true }).click();
  await frame.getByText("备注").first().waitFor();
  assertEq(await frame.getByRole("link", { name: "导出 CSV" }).count(), 0, "嵌入模式隐藏导出");

  // ---- fit=auto：上报的是内容高度，不是 iframe 视口高度；header 不 sticky、页面不再 min-h-screen ----
  // 把 iframe 拉到 2000px 高：旧实现上报 documentElement.scrollHeight 会得到 2000，新实现仍是内容高度。
  await admin.evaluate(() => {
    window.__embedFrame.style.height = "2000px";
  });
  await admin.waitForTimeout(300); // 等 iframe 的 window resize 后再上报一轮
  const resize = await waitMsg(admin, "dsh-ee:resize");
  assert(typeof resize?.height === "number" && resize.height > 0, "resize 带内容高度：" + JSON.stringify(resize));
  assert(resize.height <= 4000, "resize 高度有上限：" + resize.height);
  assert(resize.height < 1500, "resize 报内容高度而不是 iframe 视口高度：" + resize.height);
  assert(typeof resize?.width === "number" && resize.width > 0, "resize 带宽度");
  assertEq(resize?.mobile, false, "resize 带移动端判定");
  const layout = await admin.evaluate(() => {
    const doc = window.__embedFrame.contentDocument;
    const header = doc.querySelector("header");
    const page = doc.getElementById("root")?.firstElementChild;
    return {
      headerPosition: header ? getComputedStyle(header).position : "missing",
      pageClass: typeof page?.className === "string" ? page.className : "",
      pageMinHeight: page ? getComputedStyle(page).minHeight : "missing",
    };
  });
  assert(layout.headerPosition !== "sticky", "fit=auto 嵌入时 header 不吸顶，实际：" + layout.headerPosition);
  assert(!layout.pageClass.includes("min-h-screen"), "fit=auto 嵌入时页面不用 min-h-screen：" + layout.pageClass);
  assert(layout.pageMinHeight !== "100vh", "fit=auto 嵌入时页面 min-height 不再等于视口：" + layout.pageMinHeight);
  await admin.evaluate(() => {
    window.__embedFrame.style.height = "600px";
  });
  step(`fit=auto 上报内容高度 ${resize.height}px（上限 4000），header 不吸顶、页面由内容撑开`);

  // ---- 宿主 -> iframe 切主题，iframe 回发实际主题 ----
  await admin.evaluate(() => window.__embedFrame.contentWindow.postMessage({ v: 1, type: "dsh-ee:theme", theme: "light" }, location.origin));
  await admin.waitForFunction(() => window.__embedFrame?.contentDocument?.documentElement?.classList.contains("dark") === false, null, { timeout: 5000 });
  await admin.waitForFunction(() => (window.__embedMsgs ?? []).some((m) => m.data?.type === "dsh-ee:theme" && m.data?.theme === "light"), null, { timeout: 5000 });
  step("宿主发 dsh-ee:theme 切浅色：iframe 只改 DOM，并回发 dsh-ee:theme 确认");

  // ---- 移动端：窄屏 iframe 显示常驻「在浏览器打开」兜底，内容不隐藏、不自动顶层跳转 ----
  const hostURL = admin.url();
  await admin.evaluate(() => {
    window.__embedFrame.style.width = "390px";
  });
  await admin.waitForFunction(
    () => (window.__embedMsgs ?? []).some((m) => m.data?.type === "dsh-ee:resize" && m.data?.mobile === true),
    null,
    { timeout: 10000 },
  );
  const frameMobile = admin.frameLocator("#embed-frame");
  await frameMobile.locator("[data-embed-mobile]").waitFor({ timeout: 10000 });
  await frameMobile.getByRole("link", { name: "在浏览器打开" }).waitFor();
  await frameMobile.getByRole("heading", { name: `嵌入版 ${tag}` }).waitFor();
  assertEq(admin.url(), hostURL, "移动端不自动在 iframe 里顶层跳转（宿主地址不变）");
  await admin.evaluate(() => {
    window.__embedFrame.style.width = "900px";
  });
  await admin.waitForFunction(
    () => window.__embedFrame?.contentDocument?.querySelector("[data-embed-mobile]") == null,
    null,
    { timeout: 10000 },
  );
  step("移动端（<640px）：显示常驻「在浏览器打开」+ resize.mobile=true，内容保留且不自动顶层打开");

  // ---- theme=system：跟随 prefers-color-scheme（不写 localStorage），宿主 theme 消息仍可覆盖 ----
  await admin.evaluate(() => localStorage.removeItem("dsh-ee:theme"));
  await admin.emulateMedia({ colorScheme: "dark" });
  await admin.evaluate((url) => {
    window.__embedMsgs = [];
    const f = document.createElement("iframe");
    f.id = "embed-frame-system";
    f.src = url;
    f.style.cssText = "width: 900px; height: 600px; border: 0; display: block;";
    document.body.appendChild(f);
    window.__embedFrame = f;
  }, iframeSrc(slug, "&theme=system"));
  const sysReady = await waitMsg(admin, "dsh-ee:ready");
  assertEq(sysReady?.theme, "dark", "theme=system 首屏跟随系统深色");
  await admin.waitForFunction(
    () => document.getElementById("embed-frame-system")?.contentDocument?.documentElement?.classList.contains("dark") === true,
    null,
    { timeout: 10000 },
  );
  const storedTheme = await admin.evaluate(() => document.getElementById("embed-frame-system").contentWindow.localStorage.getItem("dsh-ee:theme"));
  assertEq(storedTheme, null, "嵌入页不写 localStorage 主题（theme=system）");
  await admin.emulateMedia({ colorScheme: "light" });
  await admin.waitForFunction(
    () => document.getElementById("embed-frame-system")?.contentDocument?.documentElement?.classList.contains("dark") === false,
    null,
    { timeout: 10000 },
  );
  await admin.waitForFunction(
    () => (window.__embedMsgs ?? []).some((m) => m.data?.type === "dsh-ee:theme" && m.data?.theme === "light"),
    null,
    { timeout: 10000 },
  );
  // 宿主显式指定深色后，系统在深浅之间切换不应覆盖宿主选择。
  await admin.evaluate(() => {
    document.getElementById("embed-frame-system").contentWindow.postMessage({ v: 1, type: "dsh-ee:theme", theme: "dark" }, location.origin);
  });
  await admin.waitForFunction(
    () => document.getElementById("embed-frame-system")?.contentDocument?.documentElement?.classList.contains("dark") === true,
    null,
    { timeout: 10000 },
  );
  await admin.emulateMedia({ colorScheme: "light" });
  await admin.emulateMedia({ colorScheme: "dark" });
  await admin.emulateMedia({ colorScheme: "light" });
  await admin.waitForTimeout(300);
  const overrideKept = await admin.evaluate(
    () => document.getElementById("embed-frame-system")?.contentDocument?.documentElement?.classList.contains("dark") === true,
  );
  assert(overrideKept, "宿主 theme 消息覆盖 theme=system 后，系统主题变化不应回退");
  await admin.emulateMedia({ colorScheme: null });
  step("theme=system：跟随 prefers-color-scheme 且不写 localStorage；宿主 theme 消息覆盖优先");

  // ---- 应用不存在 / 不在可用范围：error forbidden ----
  await admin.evaluate((url) => {
    window.__embedMsgs = [];
    const f = document.createElement("iframe");
    f.id = "embed-frame2";
    f.src = url;
    document.body.appendChild(f);
  }, iframeSrc(`missing-${tag}`));
  const forbidden = await waitMsg(admin, "dsh-ee:error");
  assertEq(forbidden?.code, "forbidden", "应用不存在/没权限应回 forbidden");
  assertEq(forbidden?.v, 1, "错误消息带协议版本");
  assertEq(typeof forbidden?.message, "string", "错误消息带可读信息");
  assertEq(forbidden?.stage, "load", "错误带 stage（页面/接口加载阶段）");
  step("应用不存在/没权限：宿主收到 dsh-ee:error {code: forbidden, stage: load}");

  // ---- 未登录：不在 iframe 里跳登录，先发 auth-required 请宿主握手，并常驻「在浏览器打开」兜底 ----
  const guest = await newPage(t);
  await guest.goto(`${BASE}/login`);
  await openHost(guest, iframeSrc(slug), "embed-frame-guest");
  const authReq = await waitMsg(guest, "dsh-ee:auth-required");
  assertEq(authReq?.reason, "missing", "未登录应回 auth-required(missing)");
  const iframePath = await guest.evaluate(() => window.__embedFrame.contentWindow.location.pathname);
  assert(iframePath.startsWith("/a/"), "iframe 里不应跳登录页，实际路径：" + iframePath);
  await guest.frameLocator("#embed-frame-guest").getByRole("link", { name: "在浏览器打开" }).waitFor();
  step("未登录：宿主收到 dsh-ee:auth-required(missing) 去安排握手，iframe 停在 /a/*，页面有「在浏览器打开」兜底");

  const list = await api(admin, "GET", "/api/hub");
  const asset = list.data.assets.find((x) => x.slug === slug);
  if (asset) await api(admin, "DELETE", `/api/hub/${asset.id}`);
}
