// 多语言：按浏览器语言自动选择、切换后记住，所有页面都没有漏翻成原始 key。
//
// 「支持哪些语言」「哪些前缀算未翻译的 key」都从源码动态读取，加语言不用改本脚本：
// - 语言注册表：web/src/i18n/langs.ts（唯一事实源）；
// - key 前缀：web/src/i18n/zh/*.ts 里的一级命名空间（新增命名空间自动纳入检测）；
// - 浏览器语言映射：计划中的 es/pt/hi 在语言包上线前按 fallback 规则落英文，
//   注册后断言自动变成「映射到该语言」，避免语言包 PR 与脚本改动互相卡顺序。
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { ADMIN_PW, BASE, assert, assertEq, login, newPage, step } from "../lib.mjs";

const WEB_SRC = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../../web/src");

/** 语言注册表以 langs.ts 为唯一事实源：加语言只改那里，本脚本跟着自动生效。 */
const LANGS_FILE = path.join(WEB_SRC, "i18n", "langs.ts");
const LANGS_SOURCE = fs.readFileSync(LANGS_FILE, "utf8");
// 只取 `export const LANGUAGES = [...] as const;` 里的 code，避免以后注释里的示例被算进来。
const LANGS_BLOCK = /export const LANGUAGES = \[([\s\S]*?)\] as const;/.exec(LANGS_SOURCE)?.[1] ?? "";
const LANGUAGES = [...LANGS_BLOCK.matchAll(/code:\s*"([^"]+)"/g)].map((m) => m[1]);
assert(LANGUAGES.length > 0, "没能从 langs.ts 的 LANGUAGES 解析出语言列表");
const hasLanguage = (code) => LANGUAGES.includes(code);

/**
 * 从 zh 资源（结构源）提取一级命名空间，拼出「模块名.键名」的未翻译检测正则。
 * 以前这里写死了一份 52 个的列表，漏掉了 table/embed/pickers/requests 等分组；
 * 现在按 zh/*.ts 动态生成，新增命名空间自动覆盖。
 */
function zhNamespaces() {
  const dir = path.join(WEB_SRC, "i18n", "zh");
  const names = new Set();
  for (const file of fs.readdirSync(dir).filter((f) => f.endsWith(".ts") && f !== "index.ts")) {
    const source = fs.readFileSync(path.join(dir, file), "utf8");
    for (const m of source.matchAll(/^ {2}([A-Za-z][A-Za-z0-9_]*):\s*\{/gm)) names.add(m[1]);
  }
  // index.ts 里直接合并（不是 ...spread）的命名空间，例如 sharedCredentials。
  const index = fs.readFileSync(path.join(dir, "index.ts"), "utf8");
  for (const m of index.matchAll(/^ {2}([A-Za-z][A-Za-z0-9_]*),\s*$/gm)) names.add(m[1]);
  return [...names].sort();
}

const NAMESPACES = zhNamespaces();
assert(NAMESPACES.length > 20, `从 zh 资源解析出的命名空间太少（${NAMESPACES.length} 个），检查解析逻辑`);
// i18n 模块名.键名 的样子；排除域名、文件名
const keyLike = new RegExp(`\\b(${NAMESPACES.join("|")})\\.[a-zA-Z][\\w.-]*\\b`, "g");
// 测试数据：用户名、包名等带 lib.mjs 的随机后缀（Date.now().toString(36)，8 位）
const notKey = (h) => /^[a-z]+\.(?=[a-z0-9]*\d)[a-z0-9]{8}$/.test(h) || /\.(com|org|net|io|internal|json|pem|tgz|sh|ps1|zst|npmrc|mjs|js|ts|md|yml|yaml|csv|txt|zip|tar|gz)\b/.test(h) || h.startsWith("keys.self") || h.startsWith("plugin.");

const pages = ["/workbench", "/apps", "/hub", "/hub?tab=skill", "/hub?tab=connection", "/dsh", "/setup", "/publish", "/docs", "/docs/overview", "/docs/harness-quickstart", "/examples", "/examples/dsh-claim-check", "/tokens", "/device", "/account",
  "/admin", "/admin/stats", "/admin/audit", "/admin/users", "/admin/roles", "/admin/menus", "/admin/providers", "/admin/quotas", "/admin/tokens",
  "/admin/plugins", "/admin/connections", "/admin/signing-keys", "/admin/review", "/admin/registry", "/admin/upstream", "/admin/distribution", "/admin/settings"];

/**
 * 浏览器语言 → 期望的界面语言。目标语言还没注册时按 fallback 规则落英文（fallbackLng = en），
 * 注册后自动改成断言映射到该语言。
 */
const localeCases = [
  ["zh-CN", "zh-CN"],
  ["zh-TW", "zh-CN"],
  ["en-US", "en"],
  ["en-GB", "en"],
  ["es-MX", "es"],
  ["pt-BR", "pt"],
  ["hi-IN", "hi"],
  ["ja-JP", "en"],
];

export default async function (t) {
  assert(hasLanguage("en"), "fallbackLng 的 en 必须在 langs.ts 的 LANGUAGES 里注册");
  for (const [locale, target] of localeCases) {
    const want = hasLanguage(target) ? target : "en";
    const p = await newPage(t, { lang: null, locale });
    await p.goto(`${BASE}/login`);
    await p.waitForTimeout(300);
    assertEq(await p.evaluate(() => document.documentElement.lang), want, `浏览器语言 ${locale} 应归一化到 ${want}`);
  }
  step(`浏览器语言按注册表归一（当前注册：${LANGUAGES.join(", ")}；未注册语言回落 en）`);

  // 页面扫描仍然只跑 zh/en：新语言的页面扫描放到语言包 PR 里加，避免本脚本一改就漂移。
  for (const lang of ["zh-CN", "en"].filter(hasLanguage)) {
    const page = await newPage(t, { lang });
    await login(page, "admin", ADMIN_PW);
    const raw = [];
    for (const path of pages) {
      await page.goto(BASE + path);
      await page.waitForTimeout(300);
      // data-raw：原样展示的数据（如审计日志里的权限点代码）；code/pre：文档与样例里的
      // 代码片段（例如 `registry.publish` 权限点、build.mjs），都不是界面文案。
      const text = await page.evaluate(() => {
        let text = document.body.innerText;
        for (const el of document.querySelectorAll("[data-raw], code, pre")) {
          // innerText 与 textContent 在代码块换行/空格上可能不一致，两种形态都从扫描文本里去掉。
          for (const raw of [el.innerText, el.textContent]) {
            if (raw) text = text.split(raw).join(" ");
          }
        }
        return (
          text +
          " " +
          [...document.querySelectorAll("[placeholder],[aria-label],[title]")]
            .map((e) => [e.getAttribute("placeholder"), e.getAttribute("aria-label"), e.getAttribute("title")].join(" "))
            .join(" ")
        );
      });
      const hits = [...new Set(text.match(keyLike) ?? [])].filter((h) => !notKey(h));
      if (hits.length) raw.push(`${path}: ${hits.join(", ")}`);
    }
    assert(raw.length === 0, `[${lang}] 页面上出现了未翻译的 key：\n    ${raw.join("\n    ")}`);
    step(`[${lang}] ${pages.length} 个页面没有原始 key（检测 ${NAMESPACES.length} 个命名空间）`);
  }

  const page = await newPage(t, { lang: null, locale: "en-US" });
  await login(page, "admin", ADMIN_PW);
  await page.goto(`${BASE}/workbench`);
  await page.locator("aside select").selectOption("zh-CN");
  await page.reload();
  await page.waitForTimeout(300);
  assertEq(await page.evaluate(() => document.documentElement.lang), "zh-CN", "切换后刷新");
  assertEq(await page.evaluate(() => localStorage.getItem("dsh-ee:lang")), "zh-CN", "记住的语言");
  step("切换到中文后刷新仍是中文");
}
