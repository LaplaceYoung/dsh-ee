import assert from "node:assert/strict";
import { test } from "node:test";
import { centerPanel, filterItems, sortItems, tabCount, type CenterViewActions, type CenterViewState } from "../src/center-view.js";
import { centerData } from "../src/skills.js";

const h = (type: string, props: any, ...children: any[]) => ({ type, props, children });
const calls: string[] = [];
const actions: CenterViewActions = {
  login: () => calls.push("login"),
  refresh: () => calls.push("refresh"),
  setTab: (t) => calls.push(`tab:${t}`),
  search: () => {},
  setCategory: (id) => calls.push(`cat:${id}`),
  toggleOpen: (n) => calls.push(`open:${n}`),
  summon: (a) => calls.push(`summon:${a.name}`),
  startWith: (a, ex) => calls.push(`start:${a.name}:${ex}`),
  toggle: (kind, name, enabled) => calls.push(`${enabled ? "on" : "off"}:${kind}:${name}`),
  web: (url) => calls.push(`web:${url}`),
};

const catalog = {
  apps: [
    { name: "expense-expert", title: "报销专家", description: "核对报销", starters: ["这张发票能报吗"], launch: "new", version: 1, asset_id: 1, delivery: "auto", form: "expert", expertise: ["报销"], category_id: 9, featured: true },
    { name: "plain-app", title: "普通应用", description: "x", starters: [], launch: "new", version: 1, asset_id: 2, delivery: "auto" },
  ],
  available_apps: [{ name: "legal-expert", title: "法务专家", description: "合同", starters: [], launch: "new", version: 1, asset_id: 3, delivery: "optional", form: "expert" }],
  skills: [
    { name: "invoice-check", title: "发票核对", kind: "skill", description: "核对", version: 1, asset_id: 4, delivery: "optional" },
    { name: "workbench", title: "系统技能", kind: "app", description: "", version: 1 },
    { name: "kb-sys", title: "系统知识技能", kind: "skill", description: "", version: 1 },
  ],
  available_skills: [{ name: "weekly", title: "周报", kind: "prompt", description: "写周报", version: 1, asset_id: 5, delivery: "optional" }],
  connections: [
    { name: "erp", title: "ERP", description: "财务系统", asset_id: 6, delivery: "optional", enabled: false, tools: 3, writes: true, auth: "ready" },
    { name: "oa", title: "OA", description: "审批", asset_id: 7, delivery: "auto", enabled: true, tools: 2, writes: false, auth: "needs_auth", auth_mode: "user_secret" },
  ],
  categories: [{ id: 9, group: "app", name: "财务", sort: 0 }, { id: 10, group: "connection", name: "办公", sort: 0 }],
};

test("centerData 只挑专家、带 asset_id 的技能，并拼好网页链接", () => {
  const d = centerData(catalog, "https://ee.example.com");
  assert.deepEqual(d.experts.map((x) => x.name), ["expense-expert"]);
  assert.deepEqual(d.availableExperts.map((x) => x.name), ["legal-expert"]);
  assert.deepEqual(d.skills.map((k) => `${k.name}:${k.enabled}`), ["invoice-check:true", "weekly:false"]);
  assert.equal(d.connections[1].detail_url, "https://ee.example.com/assets/7");
  assert.equal(d.legacy, false);
  // 旧服务端：没有 connections 字段，只显示专家
  assert.equal(centerData({ apps: catalog.apps }, "").legacy, true);
});

test("排序：精选在前，其次已生效；过滤看搜索词、擅长领域与分类", () => {
  const items = [
    { name: "a", title: "甲", description: "", enabled: false },
    { name: "b", title: "乙", description: "", enabled: true },
    { name: "c", title: "丙", description: "", featured: true, category_id: 1, expertise: ["税务"] },
  ];
  assert.deepEqual(sortItems(items).map((x) => x.name), ["c", "b", "a"]);
  assert.deepEqual(filterItems(items, "税务", 0).map((x) => x.name), ["c"]);
  assert.deepEqual(filterItems(items, "", 1).map((x) => x.name), ["c"]);
});

test("未登录只显示登录", () => {
  const view = JSON.stringify(centerPanel(h, { loggedIn: false, tab: "experts", query: "", category: 0 }, actions));
  assert.match(view, /登录企业账号后/);
  assert.doesNotMatch(view, /"召唤"/);
  assert.doesNotMatch(view, /center-expert/);
});

test("专家页：已生效的能召唤，没启用的先启用；展开后任务示例可直接开始", () => {
  const data = centerData(catalog, "https://ee.example.com");
  const base: CenterViewState = { data, loggedIn: true, tab: "experts", query: "", category: 0 };
  const text = JSON.stringify(centerPanel(h, base, actions));
  assert.match(text, /报销专家/);
  assert.match(text, /法务专家/);
  assert.doesNotMatch(text, /普通应用/);
  assert.match(text, /精选/);
  assert.match(text, /专家 2/);
  assert.match(text, /财务/); // 分类筛选
  const open = JSON.stringify(centerPanel(h, { ...base, open: "expense-expert" }, actions));
  assert.match(open, /任务示例/);
  assert.match(open, /这张发票能报吗/);
  assert.equal(tabCount(data, "connections"), 2);
});

test("连接器页：待授权的显示「去授权」，自选的能启用", () => {
  const data = centerData(catalog, "https://ee.example.com");
  const text = JSON.stringify(centerPanel(h, { data, loggedIn: true, tab: "connections", query: "", category: 0 }, actions));
  assert.match(text, /待授权/);
  assert.match(text, /去授权/);
  assert.match(text, /有 1 个连接器要先授权才能用/);
  assert.match(text, /可读写/);
  assert.match(text, /3 个工具/);
});

test("旧服务端只有专家页", () => {
  const data = centerData({ apps: catalog.apps }, "");
  const text = JSON.stringify(centerPanel(h, { data, loggedIn: true, tab: "connections", query: "", category: 0 }, actions));
  assert.doesNotMatch(text, /连接器 /);
  assert.match(text, /报销专家/);
});
