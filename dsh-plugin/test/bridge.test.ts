import assert from "node:assert/strict";
import { getEventListeners } from "node:events";
import { test } from "node:test";
import { listChangedCounter, McpBridge } from "../src/bridge.js";
import { normalizeConfig } from "../src/config.js";

test("bridge registers initial tools before start resolves and removes tools from list_changed", async () => {
  const registered = new Map<string, any>();
  const ctx = {
    tools: {
      register(tool: any) {
        registered.set(tool.name, tool);
        return () => registered.delete(tool.name);
      },
    },
  };
  let tools = [{ name: "lookup", description: "Lookup", inputSchema: { type: "object" } }];
  const http = {
    async fetch(_path: string, init: RequestInit) {
      if (!init.body) return new Response(null);
      const body = JSON.parse(String(init.body));
      if (body.method === "initialize") return new Response(JSON.stringify({ result: {} }), { headers: { "mcp-session-id": "s1" } });
      if (body.method === "tools/list") return new Response(JSON.stringify({ result: { tools } }));
      if (body.method === "tools/call") return new Response(JSON.stringify({ result: { content: [{ type: "text", text: "ok" }] } }));
      throw new Error("unexpected");
    },
  };
  const bridge = new McpBridge(ctx as any, normalizeConfig({ base: "http://localhost", serverName: "acme" }), http as any);
  await bridge.start();
  assert.ok(registered.has("mcp__acme__lookup"));
  tools = [];
  await bridge.sync();
  assert.equal(registered.size, 0);
  bridge.stop();
});

test("bridge survives an unreachable server at start and reconnects the event stream", async () => {
  // 回归：启动时连不上 dsh-ee 不能抛错；长连接断开后要重连并补一次同步
  const registered = new Map<string, any>();
  const ctx = { tools: { register(tool: any) { registered.set(tool.name, tool); return () => registered.delete(tool.name); } } };
  let online = false;
  let streams = 0;
  const http = {
    async fetch(_path: string, init: RequestInit) {
      if (!online) throw new TypeError("fetch failed");
      if (!init.body) {
        streams++;
        // 立即结束的事件流，模拟 dsh-ee 重启断开
        return new Response(new ReadableStream({ start(c) { c.close(); } }));
      }
      const body = JSON.parse(String(init.body));
      if (body.method === "initialize") return new Response(JSON.stringify({ result: {} }), { headers: { "mcp-session-id": "s1" } });
      if (body.method === "tools/list") return new Response(JSON.stringify({ result: { tools: [{ name: "lookup" }] } }));
      throw new Error("unexpected");
    },
  };
  const bridge = new McpBridge(ctx as any, normalizeConfig({ base: "http://localhost", serverName: "acme" }), http as any);
  await bridge.start();
  assert.equal(registered.size, 0);
  online = true;
  const deadline = Date.now() + 10_000;
  while (streams < 2 && Date.now() < deadline) await new Promise((r) => setTimeout(r, 50));
  assert.ok(streams >= 2, `事件流应在断开后重连，实际 ${streams} 次`);
  assert.ok(registered.has("mcp__acme__lookup"), "重连后应补同步出工具");
  bridge.stop();
});

test("bridge gives up after one retry when /mcp always 404 and surfaces the error", async () => {
  // 回归：/mcp 一直 404（地址/代理填错）时不能无限递归重发。
  let posts = 0;
  const http = {
    async fetch(_path: string, init: RequestInit) {
      if (!init.body) return new Response(null);
      posts++;
      return new Response("not found", { status: 404 });
    },
  };
  const ctx = { tools: { register() { return () => undefined; } } };
  const bridge = new McpBridge(ctx as any, normalizeConfig({ base: "http://localhost", serverName: "acme" }), http as any);
  await bridge.start();
  bridge.stop();
  assert.equal(posts, 2, "initialize 原始请求 + 一次清会话重试");
  assert.match(bridge.status.error, /404/);
});

test("event stream syncs only on tools/list_changed, not on keepalive comments", async () => {
  // 回归：服务端每 25 秒发一行 `: ping` 保活，以前每收到一块数据就全量 tools/list 一次
  const ctx = { tools: { register() { return () => undefined; } } };
  let lists = 0;
  let stream!: ReadableStreamDefaultController<Uint8Array>;
  const events = new ReadableStream<Uint8Array>({ start(c) { stream = c; } });
  const http = {
    async fetch(_path: string, init: RequestInit) {
      if (!init.body) return new Response(events);
      const body = JSON.parse(String(init.body));
      if (body.method === "initialize") return new Response(JSON.stringify({ result: {} }), { headers: { "mcp-session-id": "s1" } });
      if (body.method === "tools/list") { lists++; return new Response(JSON.stringify({ result: { tools: [] } })); }
      throw new Error("unexpected");
    },
  };
  const bridge = new McpBridge(ctx as any, normalizeConfig({ base: "http://localhost", serverName: "acme" }), http as any);
  await bridge.start();
  const send = async (text: string) => { stream.enqueue(new TextEncoder().encode(text)); await new Promise((r) => setTimeout(r, 30)); };
  await send(": dsh-ee\n\n");
  const base = lists; // 启动一次 + 连上后补一次
  await send(": ping\n\n");
  await send(": ping\n\n");
  assert.equal(lists, base, "保活不该触发同步");
  await send('event: message\ndata: {"jsonrpc":"2.0","method":"notifications/tools/list_changed"}\n\n');
  assert.equal(lists, base + 1, "list_changed 要同步一次");
  bridge.stop();
  stream.close();
});

test("listChangedCounter handles split chunks, CRLF and unrelated events", () => {
  const enc = (text: string) => new TextEncoder().encode(text);
  const events = listChangedCounter();
  assert.equal(events.push(enc(": ping\n\n")), 0);
  assert.equal(events.push(enc('event: message\ndata: {"jsonrpc":"2.0","meth')), 0);
  assert.equal(events.push(enc('od":"notifications/tools/list_changed"}\n')), 0);
  assert.equal(events.push(enc("\n: ping\n\n")), 1);
  assert.equal(events.push(enc('data: {"method":"notifications/progress"}\r\n\r\ndata: {"method":"notifications/tools/list_changed"}\r\n\r\n')), 1);
  assert.equal(events.push(enc("data: not json\n\n")), 0);
});

test("sync re-registers a tool whose definition changed and leaves unchanged ones alone", async () => {
  // 回归：以前只按名字增删，说明 / 参数 / 只读标记变了，在线的 DSH 一直用旧定义
  const registered = new Map<string, any>();
  let registrations = 0;
  const ctx = { tools: { register(tool: any) { registrations++; registered.set(tool.name, tool); return () => registered.delete(tool.name); } } };
  let tools: any[] = [
    { name: "data__dataset", description: "可用的数据集：\n- sales", annotations: { readOnlyHint: true } },
    { name: "lookup", description: "Lookup" },
  ];
  const http = {
    async fetch(_path: string, init: RequestInit) {
      if (!init.body) return new Response(null);
      const body = JSON.parse(String(init.body));
      if (body.method === "initialize") return new Response(JSON.stringify({ result: {} }), { headers: { "mcp-session-id": "s1" } });
      if (body.method === "tools/list") return new Response(JSON.stringify({ result: { tools } }));
      throw new Error("unexpected");
    },
  };
  const bridge = new McpBridge(ctx as any, normalizeConfig({ base: "http://localhost", serverName: "acme" }), http as any);
  await bridge.sync();
  await bridge.sync();
  assert.equal(registrations, 2, "定义没变不该重新注册");
  tools = [{ ...tools[0], description: "可用的数据集：\n- sales\n- hr", annotations: { readOnlyHint: false } }, tools[1]];
  await bridge.sync();
  assert.equal(registrations, 3, "只重新注册定义变了的那个");
  assert.equal(registered.get("mcp__acme__data__dataset").description, "可用的数据集：\n- sales\n- hr");
  assert.equal(bridge.isReadOnlyTool("data__dataset"), false, "只读标记要跟着更新");
  assert.ok(bridge.hasTool("lookup"));
});


test("退避等待不会累积 abort 监听器", async () => {
  const ctx = { tools: { register() { return () => undefined; } } };
  let calls = 0;
  const http = {
    async fetch() {
      calls++;
      throw new TypeError("offline");
    },
  };
  const bridge = new McpBridge(ctx as any, normalizeConfig({ base: "http://localhost", serverName: "acme" }), http as any);
  await bridge.start();
  const signal = (bridge as any).controller.signal as AbortSignal;
  const deadline = Date.now() + 5_000;
  while (calls < 5 && Date.now() < deadline) {
    bridge.wake();
    await new Promise((resolve) => setTimeout(resolve, 30));
  }
  assert.ok(calls >= 5, `应至少重试 5 次，实际 ${calls}`);
  assert.ok(getEventListeners(signal, "abort").length <= 1, `abort 监听器应被摘掉，实际 ${getEventListeners(signal, "abort").length}`);
  bridge.stop();
});
