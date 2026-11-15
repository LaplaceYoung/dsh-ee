import assert from "node:assert/strict";
import { test } from "node:test";
import { createLauncher, sendUnknownNotice, singleFlight } from "../src/app-launcher.js";
import type { CatalogApp } from "../src/skills.js";

const app: CatalogApp = { name: "expense", title: "报销助手", description: "", starters: ["这张发票能报吗？"], launch: "new", version: 1 };

function fakeHost(options: { send?: (text: string) => Promise<any>; current?: string } = {}) {
  const sent: string[] = [];
  const ctx: any = {
    get: () => ({ openSession: async () => undefined }),
    workspaces: { initializeDefault: async () => ({ workspaceId: "w" }) },
    sessions: {
      create: async () => "new-session",
      list: { getSnapshot: () => ({ byId: { [options.current ?? "blank"]: { id: options.current ?? "blank", retainedBy: { mainView: 1 } } } }) },
      using: async (_id: string, _source: any, fn: any) => fn({ ready: Promise.resolve({ session: {} }) }),
    },
    conversation: {
      sendSession: async (_session: any, text: string) => {
        sent.push(text);
        return options.send ? options.send(text) : { kind: "success" };
      },
    },
  };
  return { ctx, sent };
}

test("首屏启动在当前会话发消息后记下绑定", async () => {
  const { ctx, sent } = fakeHost({ current: "hero" });
  const bound: string[] = [];
  const opened: string[] = [];
  const launcher = createLauncher(ctx, {
    bind: async (target, id) => { bound.push(`${target.name}:${id}`); },
    afterLaunch: (target, id) => { opened.push(`${target.name}:${id}`); },
  });
  assert.equal(await launcher.run(() => launcher.launchHero(app), async () => "fallback"), "");
  assert.deepEqual(sent, ["/expense\n这张发票能报吗？"]);
  assert.deepEqual(bound, ["expense:hero"]);
  assert.deepEqual(opened, ["expense:hero"]);
});

test("发送失败时只提示，不再填进输入框", async () => {
  const { ctx, sent } = fakeHost({ send: async () => ({ kind: "error", text: "超时" }) });
  let filled = 0;
  const launcher = createLauncher(ctx, { bind: async () => undefined });
  const notice = await launcher.run(() => launcher.launchNew(app), async () => { filled++; return "已填进输入框"; });
  assert.equal(notice, sendUnknownNotice());
  assert.equal(filled, 0);
  assert.equal(sent.length, 1);
});

test("发送之前出错时退回填入", async () => {
  const { ctx, sent } = fakeHost();
  ctx.sessions.create = async () => { throw new Error("不能新建"); };
  let filled = 0;
  const launcher = createLauncher(ctx, { bind: async () => undefined });
  const notice = await launcher.run(() => launcher.launchNew(app), async () => { filled++; return "已填进输入框"; });
  assert.equal(notice, "已填进输入框");
  assert.equal(filled, 1);
  assert.equal(sent.length, 0);
});

test("绑定或附加动作失败不影响启动", async () => {
  const { ctx, sent } = fakeHost();
  const launcher = createLauncher(ctx, {
    bind: async () => { throw new Error("写不进去"); },
    afterLaunch: async () => { throw new Error("工作台打不开"); },
  });
  assert.equal(await launcher.run(() => launcher.launchNew(app, "选中的问题"), async () => "fallback"), "");
  assert.deepEqual(sent, ["/expense\n选中的问题"]);
});

test("回到已绑定会话只切换，不发消息；切不过去就新建", async () => {
  const { ctx, sent } = fakeHost();
  const launcher = createLauncher(ctx, { bind: async () => undefined });
  await launcher.resume(app, "old");
  assert.equal(sent.length, 0);
  ctx.get = () => ({ openSession: async (id: string) => { if (id === "old") throw new Error("已删除"); } });
  await launcher.resume(app, "old");
  assert.deepEqual(sent, ["/expense"]);
});

test("并发刷新只跑一次，结束后可以再跑", async () => {
  let calls = 0;
  let release!: () => void;
  const refresh = singleFlight(() => { calls++; return new Promise<void>((resolve) => { release = resolve; }); });
  const all = Promise.all([refresh(), refresh(), refresh()]);
  assert.equal(calls, 1);
  release();
  await all;
  const next = refresh();
  assert.equal(calls, 2);
  release();
  await next;
});
