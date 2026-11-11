// 端到端测试的公共部分：浏览器、登录、断言、截图。
//
// 环境变量：
//   E2E_BASE   被测服务地址，默认 http://127.0.0.1:8080（单文件服务端；开发时可指向 Vite 的 :5188）
//   ADMIN_PW   初始管理员 admin 的密码（必填）
//   CHROME     Chrome / Chromium 可执行文件；不填时依次找 playwright 缓存、系统 Chrome
//   E2E_SHOTS  截图目录，默认 e2e/shots
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { chromium } from "playwright-core";

export const BASE = (process.env.E2E_BASE || "http://127.0.0.1:8080").replace(/\/$/, "");
export const ADMIN_PW = process.env.ADMIN_PW || "";
export const SHOTS = process.env.E2E_SHOTS || path.join(path.dirname(fileURLToPath(import.meta.url)), "shots");
fs.mkdirSync(SHOTS, { recursive: true });

/** 每次运行一个短后缀，测试数据（用户名、角色、包名）带上它，库里有旧数据也不冲突。 */
export const tag = Date.now().toString(36);

function findChrome() {
  if (process.env.CHROME) return process.env.CHROME;
  const cache = path.join(os.homedir(), ".cache", "ms-playwright");
  if (fs.existsSync(cache)) {
    for (const dir of fs.readdirSync(cache).filter((d) => d.startsWith("chromium-")).sort().reverse()) {
      for (const rel of ["chrome-linux/chrome", "chrome-linux64/chrome", "chrome-mac/Chromium.app/Contents/MacOS/Chromium"]) {
        const p = path.join(cache, dir, rel);
        if (fs.existsSync(p)) return p;
      }
    }
  }
  for (const p of ["/usr/bin/google-chrome", "/usr/bin/chromium", "/usr/bin/chromium-browser"]) {
    if (fs.existsSync(p)) return p;
  }
  throw new Error("找不到 Chrome，请设置 CHROME=/path/to/chrome");
}

export class AssertionError extends Error {}

export function assert(cond, msg) {
  if (!cond) throw new AssertionError(msg);
}

export function assertEq(got, want, what) {
  if (got !== want) throw new AssertionError(`${what}：得到 ${JSON.stringify(got)}，期望 ${JSON.stringify(want)}`);
}

export const step = (s) => console.log("  ·", s);

let browser;
export async function getBrowser() {
  browser ??= await chromium.launch({ executablePath: findChrome(), args: ["--no-sandbox"] });
  return browser;
}

export async function closeBrowser() {
  await browser?.close();
  browser = undefined;
}

/**
 * 新开一个浏览器上下文。页面报错、5xx 都记下来，测试结束时由 run.mjs 判失败。
 * lang 固定界面语言（默认简体中文，选择器按中文写）。
 * recordVideo 可传 Playwright 的 {dir,size}，只给需要留存操作证据的场景打开。
 */
export async function newPage(t, { lang = "zh-CN", locale, viewport = { width: 1440, height: 1000 }, userAgent, recordVideo } = {}) {
  const b = await getBrowser();
  const ctx = await b.newContext({ viewport, locale: locale ?? lang, ...(userAgent ? { userAgent } : {}), ...(recordVideo ? { recordVideo } : {}) });
  if (lang) await ctx.addInitScript((v) => localStorage.setItem("dsh-ee:lang", v), lang);
  const page = await ctx.newPage();
  page.on("pageerror", (e) => t.errors.push("pageerror: " + e.message));
  page.on("response", (r) => r.status() >= 500 && t.errors.push(`${r.status()} ${r.request().method()} ${r.url()}`));
  t.pages.push(page);
  return page;
}

/**
 * 登录。落点要么是部署清单（/go 上的进度条），要么是分流后的页面。
 * 还没接入过 DSH 的账号会先被带去「开始使用」；其它测试不关心它，默认点掉（keepStart 时停在那里）。
 */
export async function login(page, username, password, { keepStart = false } = {}) {
  await page.goto(`${BASE}/login`);
  await page.locator("input[autocomplete=username]").fill(username);
  await page.locator("input[type=password]").fill(password);
  await page.locator("button[type=submit]").click();
  await page.waitForURL((u) => !u.pathname.startsWith("/login"));
  await Promise.race([
    page.waitForURL((u) => u.pathname !== "/go"),
    page.getByRole("progressbar").waitFor(),
  ]);
  if (!keepStart && new URL(page.url()).pathname.startsWith("/get-started")) {
    await api(page, "PUT", "/api/me/welcome", { dismissed: true });
    await page.goto(`${BASE}/go`);
    await page.waitForURL((u) => u.pathname !== "/go" && !u.pathname.startsWith("/get-started"));
  }
}

export async function logout(page) {
  await page.evaluate(() => fetch("/api/auth/logout", { method: "POST" }));
}

/** 在页面里用当前会话调接口（同源，带 cookie）。 */
export async function api(page, method, url, body) {
  return page.evaluate(
    async ([method, url, body]) => {
      const r = await fetch(url, {
        method,
        headers: body === undefined ? {} : { "Content-Type": "application/json" },
        body: body === undefined ? undefined : JSON.stringify(body),
      });
      const text = await r.text();
      let data = null;
      try {
        data = text ? JSON.parse(text) : null;
      } catch {
        data = text;
      }
      return { status: r.status, data };
    },
    [method, url, body],
  );
}

export async function shot(page, name) {
  await page.screenshot({ path: path.join(SHOTS, `${name}.png`), fullPage: true });
}

/**
 * 点侧边栏里的页面。侧边栏分「工作台」「管理后台」两套菜单，只显示当前这一套；
 * 要找的页面不在当前这套里时，先点底部的切换入口。
 */
export async function nav(page, opts) {
  const aside = page.locator("aside");
  const link = aside.getByRole("link", opts);
  if (!(await link.count())) await aside.locator("[data-nav-switch]").click();
  await link.first().click();
}
