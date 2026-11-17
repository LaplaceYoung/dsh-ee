import assert from "node:assert/strict";
import { test } from "node:test";
import { appIcon, appsGallery, appsSection, heroAppsMenu, heroStrip, inProgressItems, inProgressView, startText, stripSlots } from "../src/apps-view.js";
import type { CatalogApp } from "../src/skills.js";

const createElement = (type: string, props: any, ...children: any[]) => ({ type, props, children });
const actions = { login() {}, open() {}, fill() {}, web() {}, newSession() {}, unbind() {}, search() {} };
const app = (n: number): CatalogApp => ({ name: `app-${n}`, title: `应用${n}`, description: "说明", starters: [`问题${n}`], launch: "new", version: 1 });

test("空列表隐藏入口，未登录显示登录", () => {
  assert.equal(appsSection(createElement, { apps: [], loggedIn: true }, actions), null);
  const login = appsSection(createElement, { apps: [], loggedIn: false }, actions);
  assert.match(JSON.stringify(login), /登录后查看企业应用/);
  assert.equal(heroAppsMenu(createElement, [], () => {}), null);
});

test("侧栏最多三项，「查看更多」打开应用网格（带搜索）", () => {
  const apps = Array.from({ length: 10 }, (_, i) => app(i));
  const folded = appsSection(createElement, { apps, loggedIn: true }, actions);
  const text = JSON.stringify(folded);
  assert.match(text, /应用0/);
  assert.match(text, /应用2/);
  assert.doesNotMatch(text, /应用3/);
  assert.match(text, /查看更多（10）/);
  assert.doesNotMatch(text, /搜索企业应用/);
  const panel = JSON.stringify(appsSection(createElement, { apps, loggedIn: true, limit: 6 }, actions));
  assert.match(panel, /应用5/);
  assert.doesNotMatch(panel, /应用6/);
  const gallery = appsSection(createElement, { apps, loggedIn: true, galleryOpen: true }, actions);
  const galleryText = JSON.stringify(gallery);
  assert.match(galleryText, /apps-gallery/);
  assert.match(galleryText, /应用9/);
  assert.match(galleryText, /搜索企业应用/);
  assert.match(galleryText, /repeat\(auto-fill/);
  const empty = appsSection(createElement, { apps, loggedIn: true, galleryOpen: true, query: "不存在" }, actions);
  assert.match(JSON.stringify(empty), /没有匹配的应用/);
});

test("「企业应用」区块可收起：收起后只剩标题行，展开恢复列表", () => {
  const folded = appsSection(createElement, { apps: [app(1), app(2)], loggedIn: true, collapsed: true }, actions);
  const text = JSON.stringify(folded);
  assert.match(text, /企业应用/);
  assert.match(text, /（2）/);
  assert.doesNotMatch(text, /应用1/);
  assert.doesNotMatch(text, /查看全部/);
  const open = appsSection(createElement, { apps: [app(1), app(2)], loggedIn: true }, actions);
  assert.match(JSON.stringify(open), /应用1/);
});

test("startText 只有命令或命令加示例问题", () => {
  assert.equal(startText(app(1)), "/app-1");
  assert.equal(startText(app(1), "查一下"), "/app-1\n查一下");
});

test("没有图标时使用标题首字，首屏菜单打开后最多八项", () => {
  const icon = appIcon(createElement, app(1));
  assert.equal(icon.children[0], "应");
  const closed = heroAppsMenu(createElement, Array.from({ length: 12 }, (_, i) => app(i)), () => {});
  const closedText = JSON.stringify(closed);
  assert.match(closedText, /企业应用/);
  assert.doesNotMatch(closedText, /问题0/);
  const menu = heroAppsMenu(createElement, Array.from({ length: 12 }, (_, i) => app(i)), () => {}, "", () => {}, "", true, () => {});
  const text = JSON.stringify(menu);
  assert.match(text, /企业应用/);
  assert.match(text, /问题7/);
  assert.doesNotMatch(text, /问题8/);
});

test("状态点 need 黄、busy 蓝、done 灰，菜单支持重开与解绑", () => {
  const view = appsSection(createElement, { apps: [app(1)], loggedIn: true, statuses: { "app-1": "done" }, menuFor: "app-1", menuAnchor: { up: true, right: 8, bottom: 64 } }, actions);
  const text = JSON.stringify(view);
  assert.match(text, /本轮结束/);
  assert.match(text, /#737373/);
  assert.match(text, /新开一个会话/);
  assert.match(text, /解除绑定/);
  assert.doesNotMatch(text, /成功/);
  const closed = appsSection(createElement, { apps: [app(1)], loggedIn: true }, actions);
  const closedText = JSON.stringify(closed);
  assert.doesNotMatch(closedText, /新开一个会话/);
  assert.doesNotMatch(closedText, /app-menu-backdrop/);
});

test("首屏降级提示实际渲染在菜单里", () => {
  const menu = heroAppsMenu(createElement, [app(1)], () => {}, "", () => {}, "已复制 /app-1", true, () => {});
  assert.match(JSON.stringify(menu), /已复制 \/app-1/);
});

test("「⋯」菜单列出示例问题，点一条带着它开始", () => {
  const started: string[] = [];
  const withStarters = { ...app(1), starters: ["问题一", "问".repeat(300), "", "三", "四", "五", "六", "七"] };
  const section = appsSection(createElement, { apps: [withStarters], loggedIn: true, menuFor: withStarters.name }, { ...actions, startWith: (_app: CatalogApp, starter: string) => started.push(starter) });
  const buttons: any[] = [];
  const walk = (node: any) => {
    if (!node || typeof node !== "object") return;
    if (node.type === "button" && node.props?.title && withStarters.starters.includes(node.props.title)) buttons.push(node);
    for (const child of (node.children ?? []).flat(Infinity)) walk(child);
  };
  walk(section);
  assert.equal(buttons.length, 6);
  assert.match(JSON.stringify(section), /从示例问题开始/);
  buttons[0].props.onClick();
  assert.deepEqual(started, ["问题一"]);
  const plain = appsSection(createElement, { apps: [{ ...app(2), starters: [] }], loggedIn: true, menuFor: "app-2" }, { ...actions, startWith: () => {} });
  assert.doesNotMatch(JSON.stringify(plain), /从示例问题开始/);
});

test("「进行中」只列待你决定和工作中，待你决定在前，点击只切换", () => {
  const apps = [app(1), app(2), app(3), app(4)];
  const bindings = { "app-1": { sessionId: "s1" }, "app-2": { sessionId: "s2" }, "app-3": { sessionId: "s3" } };
  const statuses: Record<string, any> = { s1: "busy", s2: "need", s3: "done" };
  const items = inProgressItems(apps, bindings, (id) => statuses[id]);
  assert.deepEqual(items.map((item) => `${item.app.name}:${item.status}`), ["app-2:need", "app-1:busy"]);
  const opened: string[] = [];
  const view = inProgressView(createElement, items, (item) => opened.push(item.sessionId));
  assert.match(JSON.stringify(view), /进行中/);
  view.children[1].children[0].props.onClick();
  assert.deepEqual(opened, ["s2"]);
  assert.equal(inProgressView(createElement, [], () => {}), null);
});

test("可启用：没有就不显示，侧栏只有入口，网格里点启用", () => {
  const enabled: string[] = [];
  const withEnable = { ...actions, enable: (target: CatalogApp) => enabled.push(target.name), toggleAvailable() {} };
  assert.doesNotMatch(JSON.stringify(appsSection(createElement, { apps: [app(1)], available: [], loggedIn: true }, withEnable)), /可启用/);
  const folded = appsSection(createElement, { apps: [], available: [app(7)], loggedIn: true }, withEnable);
  assert.match(JSON.stringify(folded), /可启用（1）/);
  assert.doesNotMatch(JSON.stringify(folded), /应用7/);
  const open = appsSection(createElement, { apps: [], available: [app(7)], galleryOpen: true, loggedIn: true }, withEnable);
  const found: any[] = [];
  const walk = (node: any) => {
    if (!node || typeof node !== "object") return;
    if (node.type === "button" && node.children.includes("启用")) found.push(node);
    for (const child of (node.children ?? []).flat(Infinity)) walk(child);
  };
  walk(open);
  found[0].props.onClick();
  assert.deepEqual(enabled, ["app-7"]);
});

test("只有自选的应用能停用，要点两次", () => {
  const calls: string[] = [];
  const withDisable = { ...actions, disable: (target: CatalogApp, confirmed: boolean) => calls.push(`${target.name}:${confirmed}`) };
  const optional = { ...app(1), delivery: "optional" as const };
  const auto = { ...app(2), delivery: "auto" as const };
  const first = JSON.stringify(appsSection(createElement, { apps: [optional, auto], loggedIn: true, menuFor: "app-1" }, withDisable));
  assert.equal(first.split('"停用"').length - 1, 1);
  const buttons: any[] = [];
  const walk = (node: any) => {
    if (!node || typeof node !== "object") return;
    if (node.type === "button" && (node.children.includes("停用") || node.children.includes("确认停用"))) buttons.push(node);
    for (const child of (node.children ?? []).flat(Infinity)) walk(child);
  };
  walk(appsSection(createElement, { apps: [optional], loggedIn: true, menuFor: "app-1" }, withDisable));
  buttons[0].props.onClick();
  buttons.length = 0;
  walk(appsSection(createElement, { apps: [optional], loggedIn: true, confirmDisable: "app-1", menuFor: "app-1" }, withDisable));
  assert.equal(buttons[0].children[0], "确认停用");
  buttons[0].props.onClick();
  assert.deepEqual(calls, ["app-1:false", "app-1:true"]);
});

test("缺权限的应用：能打开网页详情时「部分能力不可用」是按钮，点了去详情页申请", () => {
  const missing = { ...app(1), missing: ["crm"], detail_url: "https://ee.example.com/assets/7" };
  const find = (node: any, pred: (n: any) => boolean): any => {
    if (!node || typeof node !== "object") return undefined;
    if (Array.isArray(node)) {
      for (const c of node) {
        const hit = find(c, pred);
        if (hit) return hit;
      }
      return undefined;
    }
    if (pred(node)) return node;
    for (const c of node.children ?? []) {
      const hit = find(c, pred);
      if (hit) return hit;
    }
    return undefined;
  };
  const opened: string[] = [];
  const view = appsSection(createElement, { apps: [missing], loggedIn: true }, { ...actions, details: (a: CatalogApp) => opened.push(a.detail_url!) });
  const button = find(view, (n) => n.props?.["data-dsh-ee-missing"] === "app-1");
  assert.equal(button?.type, "button");
  assert.match(JSON.stringify(button.children), /申请/);
  button.props.onClick({ stopPropagation() {} });
  assert.deepEqual(opened, ["https://ee.example.com/assets/7"]);
  // 旧的宿主 / 没有详情地址：仍是提示文字
  const plain = appsSection(createElement, { apps: [{ ...missing, detail_url: undefined }], loggedIn: true }, actions);
  assert.equal(find(plain, (n) => n.props?.["data-dsh-ee-missing"]), undefined);
  assert.match(JSON.stringify(plain), /部分能力不可用/);
});

test("侧栏可以隐藏：只有左侧栏那份有「隐藏」，网格底部能再打开", () => {
  const hidden: boolean[] = [];
  const withHide = { ...actions, setSidebarHidden: (value: boolean) => hidden.push(value) };
  const find = (node: any, pred: (n: any) => boolean, out: any[] = []): any[] => {
    if (!node || typeof node !== "object") return out;
    if (pred(node)) out.push(node);
    for (const c of (node.children ?? []).flat(Infinity)) find(c, pred, out);
    return out;
  };
  const sidebar = appsSection(createElement, { apps: [app(1)], loggedIn: true, hideable: true }, withHide);
  find(sidebar, (n) => n.type === "button" && n.children.includes("隐藏"))[0].props.onClick();
  assert.deepEqual(hidden, [true]);
  assert.equal(find(appsSection(createElement, { apps: [app(1)], loggedIn: true }, withHide), (n) => n.type === "button" && n.children.includes("隐藏")).length, 0);
  const gallery = appsSection(createElement, { apps: [app(1)], loggedIn: true, galleryOpen: true, sidebarHidden: true }, withHide);
  const box = find(gallery, (n) => n.type === "input" && n.props?.type === "checkbox")[0];
  assert.equal(box.props.checked, false);
  box.props.onChange();
  assert.deepEqual(hidden, [true, false]);
});

test("首屏横排：按宽度放几张卡，放不下的进「查看更多」；量到位置之前不显示", () => {
  const apps = Array.from({ length: 9 }, (_, i) => app(i));
  assert.equal(heroStrip(createElement, apps, undefined, () => {}, () => {}), null);
  assert.equal(heroStrip(createElement, apps, { left: 0, top: 0, width: 900, maxHeight: 40 }, () => {}, () => {}), null);
  assert.equal(stripSlots(900), 4);
  assert.equal(stripSlots(1120), 6);
  assert.equal(stripSlots(300), 2);
  const opened: string[] = [];
  let more = 0;
  const strip = heroStrip(createElement, apps, { left: 10, top: 400, width: 900, maxHeight: 300 }, (a) => opened.push(a.name), () => { more++; });
  const text = JSON.stringify(strip);
  assert.match(text, /应用2/);
  assert.doesNotMatch(text, /应用3/);
  assert.match(text, /查看更多（9）/);
  const buttons: any[] = [];
  const walk = (node: any) => {
    if (!node || typeof node !== "object") return;
    if (node.type === "button") buttons.push(node);
    for (const c of (node.children ?? []).flat(Infinity)) walk(c);
  };
  walk(strip);
  buttons[0].props.onClick();
  buttons[buttons.length - 1].props.onClick();
  assert.deepEqual(opened, ["app-0"]);
  assert.equal(more, 1);
  const few = JSON.stringify(heroStrip(createElement, apps.slice(0, 3), { left: 0, top: 0, width: 900, maxHeight: 300 }, () => {}, () => {}));
  assert.doesNotMatch(few, /查看更多/);
});

test("「查看更多」浮层按 应用 / 技能 / 连接器 分标签", () => {
  const center = {
    experts: [], availableExperts: [], categories: [], legacy: false,
    skills: [
      { name: "weekly-report", title: "周报助手", kind: "skill" as const, description: "写周报", asset_id: 1, delivery: "optional" as const, enabled: false, detail_url: "https://ee/assets/1" },
      { name: "translate", title: "翻译", kind: "prompt" as const, description: "中英互译", asset_id: 2, delivery: "auto" as const, enabled: true, featured: true },
    ],
    connections: [
      { name: "crm", title: "CRM", description: "客户系统", asset_id: 3, delivery: "optional" as const, enabled: true, tools: 4, writes: true, auth: "needs_auth" as const, detail_url: "https://ee/assets/3" },
    ],
  };
  const calls: any[] = [];
  const withTabs = { ...actions, setTab: (tab: string) => calls.push(["tab", tab]), toggleCapability: (...args: any[]) => calls.push(["toggle", ...args]), openUrl: (url: string) => calls.push(["url", url]) };
  // 没有目录（或旧服务端）时不画标签栏
  assert.doesNotMatch(JSON.stringify(appsSection(createElement, { apps: [app(1)], loggedIn: true, galleryOpen: true }, withTabs)), /gallery-tabs/);
  assert.doesNotMatch(JSON.stringify(appsSection(createElement, { apps: [app(1)], loggedIn: true, galleryOpen: true, center: { ...center, legacy: true } }, withTabs)), /gallery-tabs/);

  const appsTab = JSON.stringify(appsSection(createElement, { apps: [app(1)], loggedIn: true, galleryOpen: true, center }, withTabs));
  assert.match(appsTab, /gallery-tabs/);
  assert.match(appsTab, /应用1/);
  assert.doesNotMatch(appsTab, /周报助手/);

  const skillsTab = JSON.stringify(appsGallery(createElement, { apps: [app(1)], loggedIn: true, galleryOpen: true, center, tab: "skills" }, withTabs, [app(1)]));
  assert.match(skillsTab, /2 个技能/);
  assert.doesNotMatch(skillsTab, /应用1/);
  // 精选排在前面
  assert.ok(skillsTab.indexOf("翻译") < skillsTab.indexOf("周报助手"));
  assert.match(skillsTab, /\/translate/);
  assert.match(skillsTab, /默认生效/);

  const filtered = JSON.stringify(appsSection(createElement, { apps: [app(1)], loggedIn: true, galleryOpen: true, center, tab: "skills", query: "周报" }, withTabs));
  assert.match(filtered, /周报助手/);
  assert.doesNotMatch(filtered, /中英互译/);

  const connTab = appsSection(createElement, { apps: [app(1)], loggedIn: true, galleryOpen: true, center, tab: "connections" }, withTabs);
  const connText = JSON.stringify(connTab);
  assert.match(connText, /CRM/);
  assert.match(connText, /待授权/);
  assert.match(connText, /4 个工具/);
  assert.match(connText, /有 1 个连接器要先授权/);

  // 点标签、启用、去授权都走到动作上
  const find = (node: any, pred: (n: any) => boolean): any => {
    if (!node || typeof node !== "object") return undefined;
    if (Array.isArray(node)) { for (const c of node) { const hit = find(c, pred); if (hit) return hit; } return undefined; }
    if (pred(node)) return node;
    return find(node.children, pred);
  };
  find(connTab, (n) => n.props?.["data-dsh-ee-tab"] === "skills").props.onClick();
  find(connTab, (n) => n.type === "button" && n.children?.[0] === "去授权").props.onClick();
  find(connTab, (n) => n.type === "button" && n.children?.[0] === "停用").props.onClick();
  assert.deepEqual(calls, [["tab", "skills"], ["url", "https://ee/assets/3"], ["toggle", "connections", "crm", false, "CRM"]]);
});
