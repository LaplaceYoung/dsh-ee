// 👎 时的说明输入，单独成文件：不依赖 react，好测。

/**
 * 👎 时问一句「哪里不好」。返回填的内容；点「取消」返回 null（不投票）。
 * DSH Desktop 是 Electron，渲染进程里的 window.prompt 会直接抛错（Electron 不支持 prompt）：
 * 这时照常记下 👎，只是不带说明，不能让整个投票失败。
 */
export function askNote(win: { prompt?: (message: string) => string | null } | undefined, message: string): string | null {
  if (typeof win?.prompt !== "function") return "";
  try {
    return win.prompt(message);
  } catch {
    return "";
  }
}
