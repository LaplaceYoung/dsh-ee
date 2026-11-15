import assert from "node:assert/strict";
import { test } from "node:test";
import zh from "../src/i18n/zh.js";
import en from "../src/i18n/en.js";
import es from "../src/i18n/es.js";
import pt from "../src/i18n/pt.js";
import hi from "../src/i18n/hi.js";
import { DEFAULT_LANG, LANGS, detectLang, getLang, hostLocaleId, initI18n, normalizeLang, resetLang, setLang, subscribeLang, t } from "../src/i18n/index.js";
import { enterpriseHeader, enterprisePanel } from "../src/status-view.js";
import { appsSection } from "../src/apps-view.js";

const cjk = /[\u3400-\u9fff\uf900-\ufaff]/;

const createElement = (type: string, props: any, ...children: any[]) => ({ type, props, children });

/** 递归收集文案表里的所有字符串值（不含注释、键名）。 */
function values(node: unknown, out: string[] = []): string[] {
  if (typeof node === "string") out.push(node);
  else if (node && typeof node === "object") for (const value of Object.values(node)) values(value, out);
  return out;
}

test("设施固定支持 5 个语言码", () => {
  assert.deepEqual([...LANGS], ["zh-CN", "en", "es", "pt", "hi"]);
  assert.equal(DEFAULT_LANG, "zh-CN");
});

test("语言探测优先级：宿主 → 浏览器 → 环境变量 → 默认 zh", () => {
  // 宿主 locale 最优先
  assert.equal(detectLang({ host: "en-US", browser: ["zh-CN"], env: ["zh"] }), "en");
  // 宿主给了不支持的语言（日语）时继续往后找
  assert.equal(detectLang({ host: "ja-JP", browser: ["zh-TW"], env: ["en"] }), "zh-CN");
  // 没有宿主时看浏览器
  assert.equal(detectLang({ browser: ["es-ES"], env: ["en"] }), "es");
  // 浏览器也不支持时看环境变量
  assert.equal(detectLang({ browser: ["fr-FR"], env: ["pt_BR"] }), "pt");
  // C / POSIX 这类不是语言，不能当成英文
  assert.equal(detectLang({ env: ["C.UTF-8"] }), "zh-CN");
  // 什么都没有时默认中文（与插件原来的界面一致）
  assert.equal(detectLang({}), "zh-CN");
});

test("BCP 47 标签归一：zh-TW / en_US / pt-BR / hi-IN；不支持的语言返回 undefined", () => {
  assert.equal(normalizeLang("zh-TW"), "zh-CN");
  assert.equal(normalizeLang("zh_CN"), "zh-CN");
  assert.equal(normalizeLang("en_US"), "en");
  assert.equal(normalizeLang("en-GB"), "en");
  assert.equal(normalizeLang("pt-BR"), "pt");
  assert.equal(normalizeLang("hi-IN"), "hi");
  assert.equal(normalizeLang("es-419"), "es");
  assert.equal(normalizeLang("ja"), undefined);
  assert.equal(normalizeLang("de-DE"), undefined);
  assert.equal(normalizeLang("C.UTF-8"), undefined);
  assert.equal(normalizeLang(undefined), undefined);
});

test("宿主 locale 服务探测：读 getSnapshot().active；没有服务或报错时安全返回 undefined", () => {
  assert.equal(hostLocaleId({ get: (name: string) => (name === "locale" ? { getSnapshot: () => ({ active: "en" }) } : undefined) }), "en");
  assert.equal(hostLocaleId({ get: () => ({} as any) }), undefined);
  assert.equal(hostLocaleId({}), undefined);
  assert.equal(hostLocaleId(undefined), undefined);
  assert.equal(hostLocaleId({ get: () => { throw new Error("宿主没有这个服务"); } }), undefined);
});

test("initI18n 读宿主 locale 服务，并在宿主切换语言后跟随", () => {
  const listeners: Array<() => void> = [];
  let active = "en";
  const service = {
    getSnapshot: () => ({ active }),
    subscribe: (fn: () => void) => { listeners.push(fn); return () => { listeners.splice(listeners.indexOf(fn), 1); }; },
  };
  const ctx = { get: (name: string) => (name === "locale" ? service : undefined), effect: (fn: () => unknown) => { fn(); return () => {}; } };
  const emit = (id: string) => { active = id; for (const fn of [...listeners]) fn(); };
  try {
    assert.equal(initI18n(ctx), "en");
    assert.equal(getLang(), "en");
    emit("zh-TW");
    assert.equal(getLang(), "zh-CN");
    emit("es");
    assert.equal(getLang(), "es");
  } finally {
    resetLang();
  }
});

test("切换语言：setLang 归一标签，未知语言回到默认 zh", () => {
  try {
    assert.equal(setLang("en-US"), "en");
    assert.equal(getLang(), "en");
    assert.equal(setLang("zh-TW"), "zh-CN");
    assert.equal(getLang(), "zh-CN");
    assert.equal(setLang("xx"), "zh-CN");
  } finally {
    resetLang();
  }
});

test("订阅语言变化：切换时通知订阅者，退订后不再通知", () => {
  let calls = 0;
  const off = subscribeLang(() => { calls += 1; });
  try {
    setLang("en");
    assert.equal(calls, 1);
    setLang("en");
    assert.equal(calls, 1);
    off();
    setLang("zh-CN");
    assert.equal(calls, 1);
  } finally {
    resetLang();
  }
});

test("缺 key 回退：键不存在时回退键原文（任意语言）", () => {
  try {
    for (const lang of ["zh-CN", "en", "es", "pt", "hi"] as const) {
      setLang(lang);
      assert.equal((t as any)("common.thisKeyDoesNotExist"), "common.thisKeyDoesNotExist");
      assert.equal((t as any)("noSuchNamespace.key"), "noSuchNamespace.key");
    }
  } finally {
    resetLang();
  }
});

test("es / pt / hi 文案表完整：键数与中文表一致、值里没有汉字", () => {
  const zhStrings = values(zh);
  for (const [lang, table] of [
    ["es", es],
    ["pt", pt],
    ["hi", hi],
  ] as const) {
    const strings = values(table);
    assert.equal(strings.length, zhStrings.length, `${lang} 表的文案条数与中文表不一致`);
    assert.equal(strings.filter((v) => cjk.test(v)).length, 0, `${lang} 表里仍有中文`);
  }
  try {
    setLang("es");
    assert.equal(t("common.save"), "Guardar");
    setLang("pt");
    assert.equal(t("common.save"), "Salvar");
    setLang("hi");
    assert.equal(t("common.save"), "सहेजें");
  } finally {
    resetLang();
  }
});

test("中文表保留原文案：抽样断言与迁移前逐字一致", () => {
  try {
    setLang("zh-CN");
    assert.equal(t("common.login"), "登录");
    assert.equal(t("common.loading"), "加载中…");
    assert.equal(t("statusView.failureAuth"), "还没有登录企业账号，或登录已失效，请登录。");
    assert.equal(t("workbenchView.widgetBoard"), "AI 看板");
    assert.equal(t("appsView.hideSidebar"), "在左侧栏隐藏企业应用（在「查看更多」或 设置 → dsh-ee 里可以再打开）");
    assert.equal(t("kbDocRender.page", { page: 3 }), "第 3 页");
  } finally {
    resetLang();
  }
});

test("{{name}} 插值：替换参数、保留没传的参数占位符", () => {
  try {
    setLang("zh-CN");
    assert.equal(t("common.viewMoreCount", { n: 3 }), "查看更多（3）");
    assert.equal(t("appsView.moreActions", { title: "审批" }), "审批更多操作");
    assert.equal(t("statusView.failureServer"), "dsh-ee 返回错误（{{detail}}），请稍后刷新。");
    assert.equal(t("statusView.failureServer", { detail: "HTTP 500" }), "dsh-ee 返回错误（HTTP 500），请稍后刷新。");
  } finally {
    resetLang();
  }
});

test("英文文案表里没有一个汉字（防止漏翻）", () => {
  const leaked = values(en).filter((text) => cjk.test(text));
  assert.deepEqual(leaked, []);
  // 中文表反过来应有汉字
  assert.ok(values(zh).some((text) => cjk.test(text)));
});

test("视图的 en 输出无 CJK（企业面板与头部）", () => {
  const data = {
    me: { username: "zhangsan", department: "R&D", token_prefix: "ee_at_test", expires_at: "2027-01-02T00:00:00Z", admin_live: false },
    usage: { quota: { monthly_token_limit: 1000, month_tokens: 900 }, current_token: { monthly_token_limit: 200, month_tokens: 150 } },
    models: ["chat", "reasoner"],
    default_model: "chat",
    version: "1.0.8",
    latest_version: "1.0.9",
  };
  const actions = { login() {}, refresh() {}, doctor() {} };
  try {
    setLang("en");
    const panel = JSON.stringify(enterprisePanel(createElement, "Acme", { data, failed: false }, actions));
    assert.doesNotMatch(panel, cjk);
    assert.match(panel, /Account/);
    const header = JSON.stringify(enterpriseHeader(createElement, data as any, false, () => {}));
    assert.doesNotMatch(header, cjk);
    // 失败态与诊断结果也走同一个视图
    const failed = JSON.stringify(enterprisePanel(createElement, "Acme", { failed: true, failure: { reason: "unreachable", base: "http://10.0.0.5:8080", detail: "ECONNRESET" } }, actions));
    assert.doesNotMatch(failed, cjk);
  } finally {
    resetLang();
  }
});

test("视图的 en 输出无 CJK（企业应用侧栏）", () => {
  const state = {
    apps: [{ name: "app", title: "App", description: "Desc", starters: ["Question"], launch: "new", version: 1 }],
    loggedIn: true,
    statuses: { app: "need" as const },
    available: [{ name: "other", title: "Other", description: "Desc", starters: [], launch: "new", version: 1 }],
  };
  const actions = { login() {}, open() {}, fill() {}, web() {}, newSession() {}, unbind() {}, search() {} };
  try {
    setLang("en");
    const section = JSON.stringify(appsSection(createElement as any, state as any, actions as any));
    assert.doesNotMatch(section, cjk);
    assert.match(section, /Enterprise apps/);
  } finally {
    resetLang();
  }
});
