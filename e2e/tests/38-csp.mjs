// 可配置 CSP（DSH_EE_CSP）：未设置时默认 enforce（内置基线策略）；report 只报告、off 显式关闭回滚。
// 隔离模式起三个独立实例（默认 / report / off），用浏览器确认默认策略下内联主题脚本的哈希正确、
// 页面没有 CSP 违规、同站嵌入页（/a/*）仍能正常工作；非隔离模式跳过（需要独立实例）。
import { ADMIN_PW, api, assert, assertEq, newPage, step, tag } from "../lib.mjs";
import { findFreePort, isManaged, startManagedServer } from "../server.mjs";

const scriptHash = /script-src 'self' 'sha256-[A-Za-z0-9+/=]+'/;
const cspViolation = /content security policy|refused to (load|execute|apply|frame|connect)/i;

/** 把页面上的 CSP 违规记进 t.errors（run.mjs 会判失败）。 */
function watchCSP(t, page) {
  page.on("console", (msg) => {
    if (msg.type() === "error" && cspViolation.test(msg.text())) t.errors.push("CSP 违规：" + msg.text());
  });
}

async function headers(base, path) {
  const resp = await fetch(base + path);
  return { status: resp.status, headers: resp.headers };
}

export default async function (t) {
  if (!isManaged()) {
    step("非隔离模式：跳过可配置 CSP 的独立实例用例（需要 E2E_MANAGED=1）");
    return;
  }
  const embedOrigins = (process.env.DSH_EE_EMBED_ORIGINS || "")
    .split(",")
    .map((s) => s.trim())
    .filter(Boolean);

  // 默认实例：把 DSH_EE_CSP 显式设成空串，覆盖从外部继承的同名变量，验证「未设置 = 默认 enforce」。
  const fallback = await startManagedServer({ port: await findFreePort(), env: { DSH_EE_CSP: "" } });
  const report = await startManagedServer({
    port: await findFreePort(),
    env: { DSH_EE_CSP: "report", DSH_EE_CSP_REPORT_URI: "https://reports.example.com/csp" },
  });
  const off = await startManagedServer({ port: await findFreePort(), env: { DSH_EE_CSP: "off" } });
  try {
    // ---- 默认（enforce）：响应头 ----
    const loginPage = await headers(fallback.url, "/login");
    const policy = loginPage.headers.get("content-security-policy") || "";
    assert(policy.startsWith("default-src 'self';"), "默认应下发完整策略：" + policy);
    assert(scriptHash.test(policy), "script-src 应带 index.html 内联脚本的 sha256 哈希：" + policy);
    assert(!/script-src[^;]*unsafe-inline/.test(policy), "script-src 不能用 unsafe-inline 兜底：" + policy);
    assert(policy.includes("object-src 'none'"), "默认策略应禁用 object/embed：" + policy);
    assert(/img-src[^;]*blob:/.test(policy), "img-src 应放行 blob:（品牌设置里本地裁剪预览用）：" + policy);
    assert(policy.endsWith("frame-src 'self'; frame-ancestors 'self'"), "非嵌入页只能允许同站嵌入：" + policy);
    assertEq(loginPage.headers.get("content-security-policy-report-only"), null, "默认模式不应有 Report-Only");

    const appPage = await headers(fallback.url, "/a/some-app");
    const appPolicy = appPage.headers.get("content-security-policy") || "";
    assert(appPolicy.startsWith("default-src 'self';"), "默认策略同样下发到嵌入页：" + appPolicy);
    assert(appPolicy.includes("frame-ancestors 'self'"), "嵌入页至少允许同站：" + appPolicy);
    for (const origin of embedOrigins) assert(appPolicy.includes(origin), "嵌入页应合并配置的来源 " + origin + "：" + appPolicy);
    const adminPolicy = (await headers(fallback.url, "/admin/menus")).headers.get("content-security-policy") || "";
    assert(adminPolicy.endsWith("frame-ancestors 'self'"), "管理页不能放行嵌入来源：" + adminPolicy);

    const health = await headers(fallback.url, "/healthz");
    for (const key of ["content-security-policy", "content-security-policy-report-only"]) {
      assertEq(health.headers.get(key), null, "非 HTML 接口不应带 " + key);
    }
    step("默认（enforce）：HTML 带完整策略 + 合并后的 frame-ancestors；JSON 接口不带");

    // ---- 默认（enforce）：浏览器里内联脚本能执行、页面无违规、嵌入页正常 ----
    const admin = await newPage(t);
    watchCSP(t, admin);
    await admin.addInitScript(() => {
      try {
        localStorage.setItem("dsh-ee:theme", "dark");
      } catch {
        /* 隐私模式 */
      }
    });
    await admin.goto(`${fallback.url}/login`);
    await admin.locator("input[autocomplete=username]").waitFor({ timeout: 15000 });
    assertEq(
      await admin.evaluate(() => document.documentElement.classList.contains("dark")),
      true,
      "首屏主题内联脚本应被哈希放行（默认 enforce 下也能执行）",
    );
    await admin.locator("input[autocomplete=username]").fill("admin");
    await admin.locator("input[type=password]").fill(ADMIN_PW);
    await admin.locator("button[type=submit]").click();
    await admin.waitForURL((u) => !u.pathname.startsWith("/login"), { timeout: 20000 });

    const slug = `csp-${tag}`;
    const spec = {
      title: `CSP ${tag}`,
      description: "csp e2e",
      persona: "你是 CSP 测试助手。",
      icon: "🛡️",
      web: true,
      tables: [{ name: "notes", title: "备注", visibility: "own", fields: [{ name: "text", title: "内容", kind: "text" }] }],
      workbench: { panes: [{ id: "main", type: "panel", title: "主页", widgets: [{ type: "heading", title: "CSP 嵌入内容" }] }] },
    };
    assertEq((await api(admin, "POST", "/api/hub", { kind: "app", slug, delivery: "auto", spec })).status, 201, "建嵌入应用");

    await admin.goto(`${fallback.url}/go`);
    const iframeURL = `${fallback.url}/a/${slug}?embed=1&host=${encodeURIComponent(fallback.url)}`;
    await admin.evaluate((url) => {
      const f = document.createElement("iframe");
      f.id = "csp-frame";
      f.src = url;
      f.style.cssText = "width: 900px; height: 600px; border: 0; display: block;";
      document.body.appendChild(f);
    }, iframeURL);
    const frame = admin.frameLocator("#csp-frame");
    await frame.getByRole("heading", { name: `CSP ${tag}` }).waitFor({ timeout: 20000 });
    step("默认：首屏主题脚本执行、页面无 CSP 违规、同站嵌入 iframe 正常渲染");

    // ---- report：只报告不阻断，frame-ancestors 仍强制执行 ----
    const reportLogin = await headers(report.url, "/login");
    assertEq(
      reportLogin.headers.get("content-security-policy"),
      "frame-ancestors 'self'",
      "report 模式的 Content-Security-Policy 仍保留嵌入收口",
    );
    const reportOnly = reportLogin.headers.get("content-security-policy-report-only") || "";
    assert(reportOnly.startsWith("default-src 'self';"), "report 模式应下发 Report-Only 完整策略：" + reportOnly);
    assert(scriptHash.test(reportOnly), "Report-Only 的 script-src 应带内联脚本哈希：" + reportOnly);
    assert(
      reportOnly.includes("report-uri https://reports.example.com/csp"),
      "Report-Only 应带配置的 report-uri：" + reportOnly,
    );

    const reportPage = await newPage(t);
    watchCSP(t, reportPage);
    await reportPage.goto(`${report.url}/login`);
    await reportPage.locator("input[autocomplete=username]").waitFor({ timeout: 15000 });
    step("report：Report-Only 策略 + report-uri，页面正常渲染且无违规");

    // ---- off：显式关闭完整策略，只保留 frame-ancestors（升级后的回滚路径）----
    const offLogin = await headers(off.url, "/login");
    assertEq(offLogin.headers.get("content-security-policy"), "frame-ancestors 'self'", "off 模式只保留 frame-ancestors");
    assertEq(offLogin.headers.get("content-security-policy-report-only"), null, "off 模式不应有 Report-Only");
    const offPage = await newPage(t);
    await offPage.goto(`${off.url}/login`);
    await offPage.locator("input[autocomplete=username]").waitFor({ timeout: 15000 });
    step("off：显式关闭完整策略，只保留 frame-ancestors（升级回滚路径可用）");
  } finally {
    await Promise.all([fallback.stop(), report.stop(), off.stop()]);
  }
}
