import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { test } from "node:test";
import { appsSection } from "../src/apps-view.js";
import type { CatalogApp } from "../src/skills.js";
import { checkBoardBlocks, checkForm, fillPrompt, formatValue, getPath, parseWorkbenchAddress, tableRows, toolValue, workbenchAddress, type AppWorkbench, type WorkbenchPane } from "../src/workbench.js";
import { BoardStore, insertFormRow, readWidgetData, runWorkbenchTool } from "../src/workbench-host.js";
import { boardView, missingPaneView, panelView, type PanelHandlers } from "../src/workbench-view.js";
import { openWorkbench, waitMounted, workbenchOpenIn } from "../src/workbench-open.js";
import { localeTag } from "../src/i18n/index.js";

const h = (type: string, props: any, ...children: any[]) => ({ type, props, children });

const workbench: AppWorkbench = {
  panes: [
    {
      id: "main", type: "panel", title: "报销工作台",
      widgets: [
        { type: "metric", title: "本月已报", source: { connection: "expense", tool: "summary" }, field: "total", format: "money" },
        { type: "table", title: "待审", source: { connection: "expense", tool: "pending" }, rows: "items", columns: [{ field: "id", title: "单号" }], row_actions: [{ label: "让 AI 审核", prompt: "审核 {{row.id}}" }] },
        { type: "form", title: "新建", fields: [{ name: "amount", label: "金额", kind: "number", required: true }], submit: { label: "交给 AI", prompt: "新建 {{form.amount}}" } },
        { type: "board", title: "AI 看板" },
        { type: "table", title: "写工具", source: { connection: "expense", tool: "approve" }, columns: [{ field: "id", title: "单号" }] },
      ],
    },
    { id: "erp", type: "web", title: "费控", url: "https://fk.example.com/" },
  ],
};
const app: CatalogApp = { name: "expense-helper", title: "报销助手", description: "", starters: [], launch: "new", version: 1, workbench };

test("地址能编码再还原，别的地址不认", () => {
  const address = workbenchAddress("expense-helper", "main");
  assert.equal(address, "dsh-resource://dsh-ee-workbench/expense-helper/main");
  assert.deepEqual(parseWorkbenchAddress(address), { app: "expense-helper", pane: "main" });
  assert.equal(parseWorkbenchAddress("dsh-resource://file/x"), undefined);
  assert.equal(parseWorkbenchAddress("dsh-resource://dsh-ee-workbench/only"), undefined);
});

test("点号路径、格式化与表格行", () => {
  assert.equal(getPath({ a: { b: [{ c: 3 }] } }, "a.b.0.c"), 3);
  assert.equal(getPath({ a: 1 }, "a.b"), undefined);
  assert.equal(formatValue(undefined), "—");
  assert.equal(formatValue(1234.5, "money"), "1,234.50");
  assert.equal(formatValue(0.256, "percent"), "25.6%");
  assert.equal(formatValue({ x: 1 }), '{"x":1}');
  // 纯日期按本地日历日显示，不随时区差一天
  assert.equal(formatValue("2026-11-25", "date"), new Date(2026, 10, 25).toLocaleDateString(localeTag()));
  const many = { items: Array.from({ length: 60 }, (_, i) => ({ id: i })) };
  assert.deepEqual(tableRows(many, "items").rows.length, 50);
  assert.equal(tableRows(many, "items").total, 60);
  assert.deepEqual(tableRows({ items: "x" }, "items"), { rows: [], total: 0 });
});

test("占位符替换：取值、截断、缺失为空", () => {
  assert.equal(fillPrompt("审核 {{row.id}}（{{ row.who.name }}）", { row: { id: 7, who: { name: " 张三 " } } }), "审核 7（张三）");
  assert.equal(fillPrompt("新建 {{form.amount}} {{form.none}}", { form: { amount: "12" } }), "新建 12 ");
  const long = fillPrompt("{{row.text}}", { row: { text: "长".repeat(600) } });
  assert.equal(long.length, 501);
  assert.ok(long.endsWith("…"));
});

test("表单必填与数字校验", () => {
  const fields = workbench.panes[0].widgets![2].fields!;
  assert.equal(checkForm(fields, {}), "请填写「金额」");
  assert.equal(checkForm(fields, { amount: "abc" }), "「金额」要填数字");
  assert.equal(checkForm(fields, { amount: "12.5" }), "");
  assert.equal(checkForm([{ name: "level", label: "级别", kind: "select", options: ["A"] }], { level: "B" }), "「级别」请从选项里选");
});

test("工具结果：structuredContent、JSON 文字、普通文字与错误", () => {
  assert.deepEqual(toolValue({ structuredContent: { total: 1 } }), { total: 1 });
  assert.deepEqual(toolValue({ content: [{ type: "text", text: '{"total":2}' }] }), { total: 2 });
  assert.equal(toolValue({ content: [{ type: "text", text: "hello" }] }), "hello");
  assert.throws(() => toolValue({ isError: true, content: [{ type: "text", text: "没权限" }] }), /没权限/);
});

function fakeTools(tools: Record<string, { readOnly: boolean; result?: any }>) {
  const calls: { name: string; args: unknown }[] = [];
  return {
    calls,
    hasTool: (name: string) => name in tools,
    isReadOnlyTool: (name: string) => tools[name]?.readOnly === true,
    callTool: async (name: string, args: unknown) => {
      calls.push({ name, args });
      return tools[name].result;
    },
  };
}

test("取数只调只读工具：没权限显示无权限，写工具拒绝", async () => {
  const tools = fakeTools({
    expense__summary: { readOnly: true, result: { content: [{ type: "text", text: '{"total":3200}' }] } },
    expense__approve: { readOnly: false },
  });
  assert.deepEqual(await readWidgetData([app], tools, { app: "expense-helper", pane: "main", widget: 0 }), { value: { total: 3200 } });
  assert.deepEqual(await readWidgetData([app], tools, { app: "expense-helper", pane: "main", widget: 1 }), { forbidden: true });
  assert.match((await readWidgetData([app], tools, { app: "expense-helper", pane: "main", widget: 4 }) as any).error, /不是只读工具/);
  assert.match((await readWidgetData([app], tools, { app: "expense-helper", pane: "main", widget: 3 }) as any).error, /目录里没有这个组件/);
  assert.match((await readWidgetData([app], tools, { app: "other", pane: "main", widget: 0 }) as any).error, /目录里没有这个组件/);
  assert.deepEqual(tools.calls.map((call) => call.name), ["expense__summary"]);
});

test("看板参数校验", () => {
  assert.match(checkBoardBlocks([]).error!, /非空数组/);
  assert.match(checkBoardBlocks(Array.from({ length: 21 }, () => ({ type: "text", text: "x" }))).error!, /最多 20 块/);
  assert.match(checkBoardBlocks([{ type: "html", text: "<b>" }]).error!, /只能是 text、list、table 或 citations/);
  assert.match(checkBoardBlocks([{ type: "text", text: "长".repeat(12000) }]).error!, /32 KB/);
  assert.deepEqual(checkBoardBlocks([{ type: "table", columns: ["单号"], rows: [[1]] }]).blocks, [{ type: "table", columns: ["单号"], rows: [["1"]] }]);
});

test("ee_workbench 写进调用它的会话，拿不到会话时用绑定的会话", async () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "dsh-ee-board-"));
  const boards = new BoardStore(dir);
  const deps = { boards, readApps: async () => [app], boundSession: () => "bound-session" };
  const blocks = [{ type: "text", text: "已审核 3 张" }];
  assert.equal(await runWorkbenchTool(deps, { action: "board", app: "expense-helper", blocks }, { agent: { session: { id: "s/1" } } }), "已更新工作台看板");
  assert.deepEqual(boards.read("expense-helper", "s/1")?.blocks, blocks);
  await runWorkbenchTool(deps, { action: "board", app: "expense-helper", blocks }, undefined);
  assert.ok(boards.read("expense-helper", "bound-session"));
  await assert.rejects(runWorkbenchTool(deps, { action: "board", app: "nope", blocks }), /没有微应用 nope/);
  await assert.rejects(runWorkbenchTool({ ...deps, readApps: async () => [{ ...app, workbench: { panes: [workbench.panes[1]] } }] }, { action: "board", app: "expense-helper", blocks }), /没有 AI 看板/);
  await assert.rejects(runWorkbenchTool(deps, { action: "board", app: "expense-helper", blocks: [] }), /非空数组/);
  assert.equal(boards.read("../etc", "x"), undefined);
  assert.deepEqual(fs.readdirSync(dir).filter((name) => name.endsWith(".tmp")), []);
  fs.rmSync(dir, { recursive: true, force: true });
});

function handlers(sending = new Set<string>()): PanelHandlers & { sent: string[] } {
  const sent: string[] = [];
  return {
    sent, sending,
    send: (key, action, data) => sent.push(`${key}=${fillPrompt(action.prompt, data)}`),
    refresh: () => {},
    formValues: () => ({}),
    setFormValue: () => {},
    formError: () => "",
    setFormError: () => {},
  };
}

function find(node: any, predicate: (node: any) => boolean): any[] {
  if (!node || typeof node !== "object") return [];
  const own = predicate(node) ? [node] : [];
  return own.concat(...(node.children ?? []).flat(Infinity).map((child: any) => find(child, predicate)));
}

test("面板渲染：加载、无权限、出错、数据与行按钮", () => {
  const pane = workbench.panes[0];
  const loading = JSON.stringify(panelView(h, pane, { data: {} }, handlers()));
  assert.match(loading, /加载中/);
  const states = JSON.stringify(panelView(h, pane, { data: { 0: { status: "forbidden" }, 1: { status: "error", error: "超时" } } }, handlers()));
  assert.match(states, /无权限/);
  assert.match(states, /取数失败：超时/);
  const hs = handlers();
  const ready = panelView(h, pane, { data: { 0: { status: "ready", value: { total: 3200 } }, 1: { status: "ready", value: { items: [{ id: "A1" }] } } } }, hs);
  const text = JSON.stringify(ready);
  assert.match(text, /3,200.00/);
  assert.match(text, /A1/);
  const rowButton = find(ready, (node) => node.type === "button" && node.children.includes("让 AI 审核"))[0];
  rowButton.props.onClick();
  assert.deepEqual(hs.sent, ["1:row:0:0=审核 A1"]);
});

test("发送中的按钮不能再点；表单必填不通过时不发送", () => {
  const pane = workbench.panes[0];
  const busy = panelView(h, pane, { data: { 1: { status: "ready", value: { items: [{ id: "A1" }] } } } }, handlers(new Set(["1:row:0:0"])));
  assert.equal(find(busy, (node) => node.type === "button" && node.children.includes("让 AI 审核"))[0].props.disabled, true);
  let error = "";
  const hs = { ...handlers(), setFormError: (_index: number, value: string) => { error = value; } };
  const form = panelView(h, pane, { data: {} }, hs);
  find(form, (node) => node.type === "button" && node.children.includes("交给 AI"))[0].props.onClick();
  assert.equal(error, "请填写「金额」");
  assert.deepEqual(hs.sent, []);
});

test("看板只显示文字；没有内容时提示", () => {
  assert.match(JSON.stringify(boardView(h, { type: "board" }, null)), /AI 处理完以后/);
  const board = boardView(h, { type: "board" }, { app: "a", sessionId: "s", updatedAt: "2026-11-05T00:00:00Z", blocks: [{ type: "text", text: "<b>粗体</b>" }, { type: "list", items: ["一"] }] });
  assert.equal(find(board, (node) => node.type === "b").length, 0);
  assert.match(JSON.stringify(board), /<b>粗体<\/b>/);
  assert.match(JSON.stringify(missingPaneView(h)), /已经不可用/);
});

test("应用菜单只在有工作台时显示「打开工作台」", () => {
  const actions = { login() {}, open() {}, fill() {}, web() {}, newSession() {}, unbind() {}, search() {}, openWorkbench() {} };
  const plain: CatalogApp = { ...app, name: "plain", workbench: undefined };
  const openMenu = JSON.stringify(appsSection(h, { apps: [app], loggedIn: true, menuFor: app.name }, actions));
  assert.equal(openMenu.split("打开工作台").length - 1, 1);
  const closedMenu = JSON.stringify(appsSection(h, { apps: [plain], loggedIn: true, menuFor: plain.name }, actions));
  assert.equal(closedMenu.split("打开工作台").length - 1, 0);
});

function fakeSidebar(mounted: string, tabs: any[] = []) {
  const opened: any[] = [];
  let listener: (() => void) | undefined;
  let current = mounted;
  return {
    opened,
    switchTo: (id: string) => { current = id; listener?.(); },
    sidebarRight: {
      mounted: { getSnapshot: () => current, subscribe: (fn: () => void) => { listener = fn; return () => { listener = undefined; }; } },
      openTabs: { getSnapshot: () => tabs },
      openResource: (address: string, options: any) => opened.push(["resource", address, options]),
      openTab: (kind: string, options: any) => opened.push(["tab", kind, options]),
    },
  };
}

test("打开工作台：等会话切过去，第一个窗格占主区，第二个另开一栏", async () => {
  const fake = fakeSidebar("old");
  const pending = openWorkbench({ sidebarRight: fake.sidebarRight }, app, "new");
  fake.switchTo("new");
  assert.equal(await pending, true);
  assert.deepEqual(fake.opened, [
    ["resource", "dsh-resource://dsh-ee-workbench/expense-helper/main", {}],
    ["tab", "browser", { preferNewPane: true, params: { url: "https://fk.example.com/" } }],
  ]);
});

test("已经打开过就不重复打开，除非手动要求；没有工作台或会话没切过去时不打开", async () => {
  const tabs = [{ sessionId: "s1", contentId: workbenchAddress("expense-helper", "main") }];
  const fake = fakeSidebar("s1", tabs);
  assert.equal(workbenchOpenIn(fake.sidebarRight, "s1", "expense-helper"), true);
  assert.equal(workbenchOpenIn(fake.sidebarRight, "s2", "expense-helper"), false);
  assert.equal(await openWorkbench({ sidebarRight: fake.sidebarRight }, app, "s1"), false);
  assert.equal(await openWorkbench({ sidebarRight: fake.sidebarRight }, app, "s1", true), true);
  assert.equal(fake.opened.length, 2);
  assert.equal(await openWorkbench({ sidebarRight: fake.sidebarRight }, { ...app, workbench: undefined }, "s1"), false);
  assert.equal(await waitMounted(fakeSidebar("other").sidebarRight, "s1", 20), false);
});

test("某个窗格打不开时其余照常打开", async () => {
  const fake = fakeSidebar("s1");
  fake.sidebarRight.openTab = () => { throw new Error("unknown kind browser"); };
  const webFirst: CatalogApp = { ...app, workbench: { panes: [workbench.panes[1], workbench.panes[0]] } };
  assert.equal(await openWorkbench({ sidebarRight: fake.sidebarRight }, webFirst, "s1"), true);
  assert.deepEqual(fake.opened.map((item: any) => item[1]), ["dsh-resource://dsh-ee-workbench/expense-helper/main"]);
});

test("没有原文窗格时，看板上的出处是 dsh-ee 网页链接，带版本和高亮行", async () => {
  const { kbDocUrl } = await import("../src/workbench.js");
  const { citationsView } = await import("../src/workbench-view.js");
  assert.equal(kbDocUrl("https://ee.example.com", { doc: 12, version: 3, from: 5, to: 12 }), "https://ee.example.com/knowledge/docs/12?v=3#L5-12");
  assert.equal(kbDocUrl("https://ee.example.com", { doc: 7, from: 4, to: 4 }), "https://ee.example.com/knowledge/docs/7#L4");
  assert.equal(kbDocUrl("", { doc: 7, from: 4, to: 4 }), undefined);
  const h = (type: any, props: any, ...children: any[]) => ({ type, props: props ?? {}, children });
  const view = citationsView(h, [{ doc: 12, version: 3, from: 5, to: 12, quote: "住宿 550 元", title: "差旅费管理办法" }], 0, undefined, (c) => kbDocUrl("https://ee.example.com", c));
  const text = JSON.stringify(view);
  // 带上摘句，网页的阅读视图里能精确标出来
  assert.match(text, /"type":"a","props":\{"href":"https:\/\/ee\.example\.com\/knowledge\/docs\/12\?v=3&q=%E4%BD%8F%E5%AE%BF\+550\+%E5%85%83#L5-12","target":"_blank"/);
});

// ---- 工作台组件升级：图表、清单、详情卡、四种状态 ----

test("大数字、变化比例、图表序列与清单条目", async () => {
  const { compactNumber, formatDelta, chartSeries, checklistItems } = await import("../src/workbench.js");
  assert.equal(compactNumber(123456), "12.3 万");
  assert.equal(compactNumber(-250000000), "-2.5 亿");
  assert.equal(compactNumber(980), "980");
  assert.equal(formatValue(15000, "compact"), "1.5 万");
  assert.deepEqual(formatDelta(0.123), { text: "↑12.3%", direction: "up" });
  assert.deepEqual(formatDelta("-0.05"), { text: "↓5%", direction: "down" });
  assert.deepEqual(formatDelta(0), { text: "持平", direction: "flat" });
  assert.equal(formatDelta("abc"), undefined);
  const data = chartSeries({ items: [{ m: "5月", a: 10, b: "x" }, { m: "6月", a: "20", b: 3 }] }, { rows: "items", x: "m", y: [{ field: "a", title: "金额" }, { field: "b", title: "单数" }] });
  assert.deepEqual(data.labels, ["5月", "6月"]);
  assert.deepEqual(data.series.map((s) => s.values), [[10, 20], [0, 3]]);
  assert.deepEqual(checklistItems({ items: [{ id: "laptop", text: "领电脑" }] }), [{ id: "laptop", text: "领电脑" }]);
  assert.deepEqual(checklistItems({ rows: "items", item_field: "name", id_field: "id" }, { items: [{ id: 7, name: "签合同" }, { id: 7, name: "重复" }, { name: "没编号" }] }), [{ id: "7", text: "签合同" }, { id: "没编号", text: "没编号" }]);
});

const upgraded: WorkbenchPane = {
  id: "main", type: "panel", title: "新组件",
  widgets: [
    { type: "heading", title: "本月" },
    { type: "metric", title: "已报", source: { connection: "expense", tool: "summary" }, field: "total", format: "compact", delta_field: "change" },
    { type: "chart", title: "趋势", chart: "line", source: { connection: "expense", tool: "trend" }, rows: "items", x: "m", y: [{ field: "a", title: "金额" }, { field: "b", title: "单数" }] },
    { type: "chart", title: "构成", chart: "pie", source: { connection: "expense", tool: "trend" }, rows: "items", x: "m", y: [{ field: "a", title: "金额" }] },
    { type: "checklist", title: "入职清单", items: [{ id: "laptop", text: "领电脑" }, { id: "badge", text: "办工牌" }] },
    { type: "table", title: "单据", source: { connection: "expense", tool: "list" }, rows: "items", columns: [{ field: "id", title: "单号" }], detail: [{ field: "reason", title: "事由" }], empty: { text: "这个月还没有报销单", action: { label: "新建一张", prompt: "帮我新建一张报销单" } } },
  ],
};

test("新组件渲染：分组标题、指标变化、折线与饼图、清单进度、详情卡展开", () => {
  const hs = handlers();
  let toggled = "";
  let expandedRow: number | undefined;
  const view = panelView(h, upgraded, {
    data: {
      1: { status: "ready", value: { total: 123456, change: 0.08 } },
      2: { status: "ready", value: { items: [{ m: "5月", a: 10, b: 1 }, { m: "6月", a: 30, b: 2 }] } },
      3: { status: "ready", value: { items: [{ m: "差旅", a: 3 }, { m: "餐费", a: 1 }] } },
      5: { status: "ready", value: { items: [{ id: "B1", reason: "去上海见客户" }] } },
    },
  }, { ...hs, checked: () => ["badge"], toggleCheck: (_i, id) => { toggled = id; }, expandedRow: () => expandedRow, toggleRow: (_i, r) => { expandedRow = r; } });
  const text = JSON.stringify(view);
  assert.equal(find(view, (node) => node.type === "h3")[0].children[0], "本月");
  assert.match(text, /12\.3 万/);
  assert.match(text, /↑8%/);
  assert.equal(find(view, (node) => node.type === "polyline").length, 2);
  assert.equal(find(view, (node) => node.type === "path").length, 2);
  assert.match(text, /差旅：3（75%）/);
  assert.match(text, /1\/2/);
  const boxes = find(view, (node) => node.type === "input" && node.props.type === "checkbox");
  assert.deepEqual(boxes.map((node) => node.props.checked), [false, true]);
  boxes[0].props.onChange();
  assert.equal(toggled, "laptop");
  assert.equal(find(view, (node) => node.type === "dl").length, 0);
  find(view, (node) => node.type === "button" && node.props["aria-label"] === "展开详情")[0].props.onClick({ stopPropagation() {} });
  assert.equal(expandedRow, 0);
  const open = panelView(h, upgraded, { data: { 5: { status: "ready", value: { items: [{ id: "B1", reason: "去上海见客户" }] } } } }, { ...hs, expandedRow: (i) => (i === 5 ? 0 : undefined) });
  assert.match(JSON.stringify(find(open, (node) => node.type === "dl")), /去上海见客户/);
});

test("四种状态：骨架屏、空状态带建议操作、无权限带申请链接、出错可重试", () => {
  const table = upgraded.widgets![5];
  const one: WorkbenchPane = { ...upgraded, widgets: [table] };
  const states = (state: any, extra: Partial<PanelHandlers> = {}) => panelView(h, one, { data: { 0: state } }, { ...handlers(), ...extra });
  const loading = states(undefined);
  assert.equal(find(loading, (node) => node.props?.["data-state"] === "loading").length, 1);
  const hs = handlers();
  const empty = panelView(h, one, { data: { 0: { status: "ready", value: { items: [] } } } }, hs);
  assert.match(JSON.stringify(empty), /这个月还没有报销单/);
  find(empty, (node) => node.type === "button" && node.children.includes("新建一张"))[0].props.onClick();
  assert.deepEqual(hs.sent, ["0:empty=帮我新建一张报销单"]);
  const forbidden = states({ status: "forbidden" }, { requestHref: (c) => `https://ee.example.com/connections/${c}` });
  assert.match(JSON.stringify(forbidden), /你还没有「expense」里「list」的使用权限/);
  assert.equal(find(forbidden, (node) => node.type === "a")[0].props.href, "https://ee.example.com/connections/expense");
  let retried = -1;
  const error = states({ status: "error", error: "超时" }, { refresh: (i) => { retried = i; } });
  assert.match(JSON.stringify(error), /取数失败：超时/);
  find(error, (node) => node.type === "button" && node.children.includes("重试"))[0].props.onClick();
  assert.equal(retried, 0);
});

test("ee_workbench 更新清单：按标题找清单、只认配置里的编号、按会话保存", async () => {
  const { ChecklistStore } = await import("../src/workbench-host.js");
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "dsh-ee-checklist-"));
  const checklists = new ChecklistStore(dir);
  const withList: CatalogApp = { ...app, name: "onboarding", workbench: { panes: [upgraded] } };
  const deps = { boards: new BoardStore(dir), checklists, readApps: async () => [withList], boundSession: () => "bound" };
  const exec = { agent: { session: { id: "s1" } } };
  assert.equal(await runWorkbenchTool(deps, { action: "checklist", app: "onboarding", done: ["laptop"] }, exec), "已更新清单「入职清单」：完成 1/2");
  assert.deepEqual(checklists.read("onboarding", "s1")?.lists, { "main:4": ["laptop"] });
  await runWorkbenchTool(deps, { action: "checklist", app: "onboarding", title: "入职清单", done: ["badge"], undone: ["laptop"] }, exec);
  assert.deepEqual(checklists.read("onboarding", "s1")?.lists["main:4"], ["badge"]);
  await assert.rejects(runWorkbenchTool(deps, { action: "checklist", app: "onboarding", done: ["nope"] }, exec), /没有条目 nope/);
  await assert.rejects(runWorkbenchTool(deps, { action: "checklist", app: "onboarding", title: "别的", done: ["badge"] }, exec), /没有标题是「别的」的清单/);
  await assert.rejects(runWorkbenchTool(deps, { action: "checklist", app: "onboarding" }, exec), /至少填一个/);
  await assert.rejects(runWorkbenchTool(deps, { action: "checklist", app: "expense-helper", done: ["x"] }, exec), /没有微应用/);
  await assert.rejects(runWorkbenchTool({ ...deps, readApps: async () => [app] }, { action: "checklist", app: "expense-helper", done: ["x"] }, exec), /没有清单/);
  // 拿不到调用方会话时用绑定的会话；别的会话不受影响
  await runWorkbenchTool(deps, { action: "checklist", app: "onboarding", done: ["laptop"] });
  assert.deepEqual(checklists.read("onboarding", "bound")?.lists["main:4"], ["laptop"]);
  assert.deepEqual(checklists.read("onboarding", "s1")?.lists["main:4"], ["badge"]);
  assert.throws(() => checklists.update("onboarding", "s1", "../x", ["a"], []), /清单编号不合法/);
  fs.rmSync(dir, { recursive: true, force: true });
});

test("表单提交到应用自带的表：只带表单里的字段，经 app__write 插入；没配表或出错给原因", async () => {
  const app = {
    name: "outing", title: "团建报名", description: "", starters: [], launch: "new", delivery: "auto", version: 1,
    workbench: { panes: [{ id: "main", type: "panel", title: "报名", widgets: [
      { type: "form", title: "报名", fields: [{ name: "session", label: "场次", kind: "select", options: ["上午"] }, { name: "diet", label: "忌口", kind: "text" }], submit: { label: "报名", prompt: "", table: "signups" } },
      { type: "form", title: "问 AI", fields: [{ name: "q", label: "问题", kind: "text" }], submit: { label: "问", prompt: "{{form.q}}" } },
    ] }] },
  } as unknown as CatalogApp;
  const ok = fakeTools({ app__write: { readOnly: false, result: { content: [{ type: "text", text: "{\"inserted\":1}" }] } } });
  assert.deepEqual(await insertFormRow([app], ok, { app: "outing", pane: "main", widget: 0, values: { session: "上午", diet: "", owner: "冒名" } }), { ok: true });
  assert.deepEqual(ok.calls, [{ name: "app__write", args: { action: "insert", app: "outing", table: "signups", rows: [{ session: "上午" }] } }]);
  assert.deepEqual(await insertFormRow([app], ok, { app: "outing", pane: "main", widget: 1, values: { q: "x" } }), { error: "这个表单没有配置提交到表" });
  const bad = fakeTools({ app__write: { readOnly: false, result: { isError: true, content: [{ type: "text", text: "字段「场次」必填" }] } } });
  assert.deepEqual(await insertFormRow([app], bad, { app: "outing", pane: "main", widget: 0, values: {} }), { error: "字段「场次」必填" });
});

// ---- 仪表盘版式与写表的行按钮 ----

const rowsPane: WorkbenchPane = {
  id: "todo", type: "panel", title: "待办",
  widgets: [
    { type: "metric", title: "待办", source: { connection: "app", tool: "rows", table: "todos" }, field: "total" },
    { type: "metric", title: "已完成", source: { connection: "app", tool: "rows", table: "todos" }, field: "total" },
    { type: "metric", title: "逾期", source: { connection: "app", tool: "rows", table: "todos" }, field: "total" },
    { type: "table", title: "我的待办", width: "wide", source: { connection: "app", tool: "rows", table: "todos" }, rows: "items",
      columns: [{ field: "title", title: "事项" }, { field: "status", title: "状态", format: "status" }],
      row_actions: [{ label: "完成", prompt: "", table: "todos", set: { status: "已完成" } }, { label: "删除", prompt: "", table: "todos", delete: true }] },
  ],
};

test("仪表盘：没写宽度的连续指标平分一行，状态列显示成带颜色的标签", async () => {
  const { widgetWidths } = await import("../src/workbench-view.js");
  assert.deepEqual(widgetWidths(rowsPane.widgets!), ["third", "third", "third", "wide"]);
  assert.deepEqual(widgetWidths([...rowsPane.widgets!.slice(0, 3), rowsPane.widgets![0], rowsPane.widgets![0]]), ["quarter", "quarter", "quarter", "quarter", "full"]);
  const view = panelView(h, rowsPane, { data: { 3: { status: "ready", value: { items: [{ id: 7, title: "领电脑", status: "已完成" }, { id: 8, title: "交材料", status: "已驳回" }] } } } }, handlers());
  const cells = find(view, (node) => node.props?.["data-status"]);
  assert.deepEqual(cells.map((node) => node.props["data-status"]), ["good", "bad"]);
  assert.equal(find(view, (node) => node.props?.className?.includes("dshee-w-wide")).length, 1);
});

test("删除按钮点过一次显示「确认删除」", () => {
  const view = panelView(h, rowsPane, { data: { 3: { status: "ready", value: { items: [{ id: 7, title: "领电脑", status: "待办" }] } } } }, { ...handlers(), armed: (key) => key === "3:row:0:1" });
  assert.equal(find(view, (node) => node.type === "button" && node.children.includes("确认删除")).length, 1);
});

test("写表的行按钮：按目录里的配置改这一行；删除先拿确认码再删；不是写表的按钮拒绝", async () => {
  const { writeTableRow } = await import("../src/workbench-host.js");
  const apps: CatalogApp[] = [{ ...app, name: "todo-app", workbench: { panes: [rowsPane] } }];
  const calls: any[] = [];
  const tools = {
    hasTool: () => true,
    isReadOnlyTool: () => false,
    callTool: async (name: string, args: any) => {
      calls.push([name, args]);
      const value = args.action === "delete" && !args.confirm ? { confirm_required: true, confirm: "c-1" } : { ok: true };
      return { content: [{ type: "text", text: JSON.stringify(value) }] };
    },
  };
  assert.deepEqual(await writeTableRow(apps, tools, { app: "todo-app", pane: "todo", widget: 3, action: 0, id: 7 }), { ok: true });
  assert.deepEqual(calls[0], ["app__write", { action: "update", app: "todo-app", table: "todos", id: 7, row: { status: "已完成" } }]);
  assert.deepEqual(await writeTableRow(apps, tools, { app: "todo-app", pane: "todo", widget: 3, action: 1, id: 8 }), { ok: true });
  assert.deepEqual(calls.slice(1).map((c) => c[1].confirm), [undefined, "c-1"]);
  assert.deepEqual(await writeTableRow(apps, tools, { app: "todo-app", pane: "todo", widget: 0, action: 0, id: 7 }), { error: "这个按钮不能改表" });
});

test("多个业务面板在同一栏里开成标签页，最后切回第一个", async () => {
  const fake = fakeSidebar("s1");
  const two: CatalogApp = { ...app, workbench: { panes: [workbench.panes[0], { id: "plans", type: "panel", title: "计划", widgets: [{ type: "markdown", text: "x" }] }] } };
  assert.equal(await openWorkbench({ sidebarRight: fake.sidebarRight }, two, "s1"), true);
  assert.deepEqual(fake.opened, [
    ["resource", "dsh-resource://dsh-ee-workbench/expense-helper/main", {}],
    ["resource", "dsh-resource://dsh-ee-workbench/expense-helper/plans", {}],
    ["resource", "dsh-resource://dsh-ee-workbench/expense-helper/main", {}],
  ]);
});
