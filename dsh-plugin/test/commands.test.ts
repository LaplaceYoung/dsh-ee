import assert from "node:assert/strict";
import { test } from "node:test";
import { registerCommands } from "../src/commands.js";
import { normalizeConfig } from "../src/config.js";

test("并发 /ee-login 只申请一个设备码", async () => {
  const original = globalThis.fetch;
  let codes = 0;
  let tokens = 0;
  globalThis.fetch = (async (input: any) => {
    const url = String(input);
    if (url.endsWith("/api/device/code")) {
      codes++;
      await new Promise((resolve) => setTimeout(resolve, 40));
      return new Response(JSON.stringify({
        device_code: "device-1",
        user_code: "ABCD-EFGH",
        verification_uri: "http://ee/device",
        verification_uri_complete: "http://ee/device?code=ABCD-EFGH",
        interval: 1,
      }), { headers: { "content-type": "application/json" } });
    }
    if (url.endsWith("/api/device/token")) {
      tokens++;
      return new Response(JSON.stringify({ access_token: "ee_at_test", scopes: [], user: { username: "u", department: "d" }, expires_at: "" }), { headers: { "content-type": "application/json" } });
    }
    throw new Error("unexpected " + url);
  }) as typeof fetch;
  try {
    const ctx = { get: () => undefined } as any;
    const tm = { set: async () => undefined, syncNpmrc: async () => undefined } as any;
    const handle = registerCommands(ctx, normalizeConfig({ base: "http://ee" }), tm);
    const [first, second] = await Promise.all([handle.login("ABCD-EFGH"), handle.login("ABCD-EFGH")]);
    assert.equal(first.kind, "success");
    assert.equal(second.kind, "success");
    assert.match(second.text, /正在发起/);
    assert.equal(codes, 1, "并发登录只申请一个设备码");
    assert.equal(tokens, 1);
  } finally {
    globalThis.fetch = original;
  }
});
