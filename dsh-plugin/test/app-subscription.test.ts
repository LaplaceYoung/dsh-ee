import assert from "node:assert/strict";
import { test } from "node:test";
import { setAppSubscription, setSubscription } from "../src/app-subscription.js";

function fakeHttp(status: number, body: any) {
  const calls: string[] = [];
  return {
    calls,
    http: {
      fetch: async (path: string, init: any) => {
        calls.push(`${init?.method} ${path}`);
        return { ok: status < 300, status, json: async () => body } as any;
      },
    } as any,
  };
}

test("启用、停用调对应接口，失败时带上服务端的错误", async () => {
  const ok = fakeHttp(200, { revision: "r" });
  assert.deepEqual(await setAppSubscription(ok.http, "expense-pick", true), { ok: true });
  assert.deepEqual(await setAppSubscription(ok.http, "expense-pick", false), { ok: true });
  assert.deepEqual(ok.calls, ["POST /api/dsh/apps/expense-pick/subscription", "DELETE /api/dsh/apps/expense-pick/subscription"]);
  const bad = fakeHttp(400, { error: "这个微应用对你默认生效，不需要启用，也不能停用" });
  assert.deepEqual(await setAppSubscription(bad.http, "always-on", false), { ok: false, error: "这个微应用对你默认生效，不需要启用，也不能停用" });
  const invalid = fakeHttp(200, {});
  assert.deepEqual(await setAppSubscription(invalid.http, "../x", true), { ok: false, error: "微应用名不合法" });
  assert.deepEqual(invalid.calls, []);
});

test("技能与连接走各自的接口，类型不对直接拒绝", async () => {
  const ok = fakeHttp(200, {});
  assert.deepEqual(await setSubscription(ok.http, "skills", "invoice-check", true), { ok: true });
  assert.deepEqual(await setSubscription(ok.http, "connections", "erp", false), { ok: true });
  assert.deepEqual(ok.calls, ["POST /api/dsh/skills/invoice-check/subscription", "DELETE /api/dsh/connections/erp/subscription"]);
  assert.deepEqual(await setSubscription(ok.http, "plugins" as any, "x", true), { ok: false, error: "能力类型不合法" });
  assert.deepEqual(await setSubscription(ok.http, "connections", "Bad Name", true), { ok: false, error: "连接名不合法" });
  assert.equal(ok.calls.length, 2);
});
