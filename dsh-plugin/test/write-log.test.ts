import assert from "node:assert/strict";
import { test } from "node:test";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { WriteLog, writeOf, writesBetween } from "../src/write-log.js";
import { McpBridge } from "../src/bridge.js";
import { normalizeConfig } from "../src/config.js";
import { groupTurnWrites, turnWritesView } from "../src/turn-writes-view.js";

test("writeOf: read-only tools, annotated actions and connection writes", () => {
  assert.equal(writeOf({ name: "kb__search", annotations: { readOnlyHint: true } }, {}), undefined);
  const hub = { name: "ee_hub", _meta: { dshee: { actions: { create: { e: "write", zh: "新建能力" }, delete: { e: "danger", zh: "删除能力" } } } } };
  assert.equal(writeOf(hub, { action: "list" }), undefined, "没标注的 action 是只读");
  assert.deepEqual(writeOf(hub, { action: "create" }), { label: "新建能力" });
  assert.equal(writeOf(hub, { action: "delete" }), undefined, "危险操作的预览一步不算");
  assert.deepEqual(writeOf(hub, { action: "delete", confirm: true }), { label: "删除能力" });
  assert.deepEqual(
    writeOf({ name: "demo-itsm__create_ticket", description: "【IT 工单】新建", annotations: { readOnlyHint: false }, _meta: { dshee: { asset: 7, system: "IT 工单" } } }, {}),
    { label: "create_ticket", system: "IT 工单", asset: 7 },
  );
  assert.deepEqual(writeOf({ name: "crm__update", description: "【CRM】改客户" }, {}), { label: "update", system: "CRM" }, "旧服务端从说明里取系统名");
});

test("WriteLog keeps records per session and filters by turn window", () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "dsh-ee-writes-"));
  const log = new WriteLog(dir);
  log.record("s1", { at: 100, tool: "a__x", label: "x", ok: true });
  log.record("s1", { at: 200, tool: "a__y", label: "y", ok: false });
  log.record("weird/../id", { at: 150, tool: "a__z", label: "z", ok: true });
  assert.deepEqual(log.between("s1", 150, 250).map((item) => item.label), ["y"]);
  assert.deepEqual(log.between("s1", 0, 1000).map((item) => item.label), ["x", "y"]);
  assert.deepEqual(log.between("weird/../id", 0, 1000).map((item) => item.label), ["z"]);
  assert.ok(fs.readdirSync(dir).every((name) => /^[A-Za-z0-9_-]+\.json$/.test(name)), "会话 id 不安全时用哈希做文件名");
  assert.deepEqual(log.between("nope", 0, 1000), []);
  assert.deepEqual(writesBetween([{ at: 5, tool: "t", label: "t", ok: true }], 1), [{ at: 5, tool: "t", label: "t", ok: true }]);
});

test("bridge reports writes with the calling session, skips reads and calls without a session", async () => {
  const registered = new Map<string, any>();
  const ctx = { tools: { register(tool: any) { registered.set(tool.name, tool); return () => registered.delete(tool.name); } } };
  const tools = [
    { name: "itsm__list", description: "【IT 工单】列表", annotations: { readOnlyHint: true } },
    { name: "itsm__create", description: "【IT 工单】新建", annotations: { readOnlyHint: false }, _meta: { dshee: { asset: 3, system: "IT 工单" } } },
  ];
  let fail = false;
  const http = {
    async fetch(_path: string, init: RequestInit) {
      if (!init.body) return new Response(null, { status: 500 });
      const body = JSON.parse(String(init.body));
      if (body.method === "initialize") return new Response(JSON.stringify({ result: {} }), { headers: { "mcp-session-id": "s" } });
      if (body.method === "tools/list") return new Response(JSON.stringify({ result: { tools } }));
      if (body.method === "tools/call") return new Response(JSON.stringify({ result: { content: [{ type: "text", text: "ok" }], isError: fail } }));
      throw new Error("unexpected");
    },
  };
  const bridge = new McpBridge(ctx as any, normalizeConfig({ base: "http://localhost", serverName: "acme" }), http as any);
  const seen: any[] = [];
  bridge.onWrite = (sessionId, record) => seen.push({ sessionId, ...record });
  await bridge.sync();
  const exec = { agent: { session: { id: "sess-1" } } };
  await registered.get("mcp__acme__itsm__list").execute({}, exec);
  await registered.get("mcp__acme__itsm__create").execute({ title: "VPN" }, exec);
  await registered.get("mcp__acme__itsm__create").execute({ title: "no session" });
  fail = true;
  await registered.get("mcp__acme__itsm__create").execute({ title: "bad" }, exec);
  bridge.stop();
  assert.equal(seen.length, 2);
  assert.equal(seen[0].sessionId, "sess-1");
  assert.equal(seen[0].label, "create");
  assert.equal(seen[0].system, "IT 工单");
  assert.equal(seen[0].asset, 3);
  assert.equal(seen[0].ok, true);
  assert.equal(seen[1].ok, false);
});

test("turn writes group by system with web links and render nothing when empty", () => {
  const groups = groupTurnWrites({
    base: "https://ee.example.com",
    writes: [
      { at: 1, tool: "itsm__create", label: "create", system: "IT 工单", asset: 3, ok: true },
      { at: 2, tool: "itsm__create", label: "create", system: "IT 工单", asset: 3, ok: true },
      { at: 3, tool: "itsm__close", label: "close", system: "IT 工单", asset: 3, ok: false },
      { at: 4, tool: "ee_hub", label: "新建能力", ok: true },
    ],
  });
  assert.deepEqual(groups, [
    { system: "IT 工单", url: "https://ee.example.com/assets/3", ops: [{ label: "create", ok: 2, failed: 0 }, { label: "close", ok: 0, failed: 1 }] },
    { system: "dsh-ee", ops: [{ label: "新建能力", ok: 1, failed: 0 }] },
  ]);
  const h = (type: any, props: any, ...children: any[]) => ({ type, props, children });
  assert.equal(turnWritesView(h, [], () => {}), null);
  const opened: string[] = [];
  const view: any = turnWritesView(h, groups, (url) => opened.push(url));
  const text = JSON.stringify(view);
  assert.match(text, /有未成功的/);
  assert.match(text, /create ×2/);
  assert.match(text, /close 未成功/);
  const link = view.children[1].children[2];
  link.props.onClick({ preventDefault() {} });
  assert.deepEqual(opened, ["https://ee.example.com/assets/3"]);
});
