import assert from "node:assert/strict";
import { test } from "node:test";
import { askNote } from "../src/vote-note.js";

test("👎 的说明：填了就带上，取消就不投，宿主不支持 prompt 时照常投", () => {
  assert.equal(askNote({ prompt: () => "答非所问" }, "哪里不好？"), "答非所问");
  assert.equal(askNote({ prompt: () => "" }, "哪里不好？"), "");
  // 浏览器里点「取消」：prompt 返回 null，这一票不投
  assert.equal(askNote({ prompt: () => null }, "哪里不好？"), null);
  // DSH Desktop（Electron）：window.prompt 直接抛错，原来整个投票都失败、按钮没反应
  assert.equal(askNote({ prompt: () => { throw new Error("prompt() is and will not be supported."); } }, "哪里不好？"), "");
  // 没有 window（或没有 prompt）：不问，直接投
  assert.equal(askNote(undefined, "哪里不好？"), "");
  assert.equal(askNote({}, "哪里不好？"), "");
});
