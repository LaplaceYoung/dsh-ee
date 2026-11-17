import assert from "node:assert/strict";
import { test } from "node:test";
import { inject } from "../src/index.js";

// 旧版 DSH 的加载器对 required: false 的依赖也会等它出现：声明一个宿主没有的服务，整个插件就停在 pending，
// 登录与目录接口全部 404。只声明所有在用的 DSH 版本都提供的服务；新服务（如 mcpManager）用 ctx.get 按需取。
test("inject 只声明各版本 DSH 都有的服务", () => {
  assert.deepEqual(Object.keys(inject).sort(), ["commands", "credentials", "pluginManager", "skills", "tools"]);
});
