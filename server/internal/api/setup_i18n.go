package api

import (
	"net/http"
	"strconv"
	"strings"
)

// setup.sh 是多语言脚本：提示文案统一走 msg_* 函数，语言在生成时决定，或在员工机器上按
// LANG 自动判断。命令、路径、退出码与原来完全一致，只有输出文字随语言变化。
//
// 语言优先级：
//  1. ?lang=zh|en（landing / 文档可以直接指定）；
//  2. Accept-Language（浏览器下载脚本时会带）；
//  3. 都没有时留空：curl | sh 拿不到浏览器语言，脚本在运行端按 LC_ALL/LANG 自动选，
//     非 zh* 用英文（POSIX 环境通常没有 LANG，英文是更安全的默认）。

func setupLang(r *http.Request) string {
	if v := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("lang"))); v != "" {
		switch {
		case v == "zh" || strings.HasPrefix(v, "zh-") || strings.HasPrefix(v, "zh_"):
			return "zh"
		case v == "en" || strings.HasPrefix(v, "en-") || strings.HasPrefix(v, "en_"):
			return "en"
		default:
			return ""
		}
	}
	for _, part := range strings.Split(r.Header.Get("Accept-Language"), ",") {
		tag := strings.ToLower(strings.TrimSpace(strings.SplitN(part, ";", 2)[0]))
		switch {
		case strings.HasPrefix(tag, "zh"):
			return "zh"
		case strings.HasPrefix(tag, "en"):
			return "en"
		}
	}
	return ""
}

// setupShellHeader 是 setup.sh 的 shebang 与说明注释。注释不是给员工看的提示，但让脚本文本
// 在 en 下保持全英文，测试与阅读都更直观。
func setupShellHeader(lang string) string {
	comment := "# dsh-ee setup script: signs in through the browser by default; --token keeps CI and legacy flows working."
	if lang == "zh" {
		comment = "# dsh-ee 接入脚本：默认通过浏览器设备登录；--token 保留 CI 与旧流程。"
	}
	return "#!/bin/sh\n" + comment + "\n"
}

// setupDesktopComment 是 setup.sh 里 macOS Desktop 分支的说明注释。
func setupDesktopComment(lang string) string {
	if lang == "zh" {
		return "# macOS 已安装 Desktop 时，使用它自带的 Node 与 dsh，不要求系统安装 npm / pnpm。"
	}
	return "# When Desktop is installed on macOS, use its bundled Node and dsh; no system npm / pnpm needed."
}

// setupShellPrelude 选定语言并定义 msg_* 输出函数。
//   - 生成时已确定语言：直接赋值，只带一种语言的文案；
//   - 未指定：脚本运行端按 LANG 选（zh* → 中文，其它 → 英文）。
func setupShellPrelude(lang string) string {
	switch lang {
	case "zh", "en":
		return "SETUP_LANG=" + lang + "\n" + setupShellMessages(lang)
	default:
		return `case "${LC_ALL:-${LANG:-}}" in
  zh*) SETUP_LANG=zh ;;
  *) SETUP_LANG=en ;;
esac
if [ "$SETUP_LANG" = "zh" ]; then
` + setupShellMessagesZh + `else
` + setupShellMessagesEn + `fi
`
	}
}

func setupShellMessages(lang string) string {
	if lang == "en" {
		return setupShellMessagesEn
	}
	return setupShellMessagesZh
}

// setupShellMessagesZh / setupShellMessagesEn 定义同名的一组 msg_* 函数：二者必须一一对应。
// 函数参数用位置参数传入，保证 ${PENDING_FILE} 这类变量在调用时才展开。
const setupShellMessagesZh = `msg_wrote_npmrc() { printf '✔ 已写入 %s（只对 DSH 生效，不改 ~/.npmrc）\n' "$1"; }
msg_old_global_npmrc() { printf '提示：~/.npmrc 里还有旧版接入脚本写的 dsh-ee 配置，DSH 已不再需要；如果它影响你本地开发，可以删掉含 %s 的行和 _authToken 那行。\n' "$1"; }
msg_usage() { printf '用法：sh -s -- --token <令牌>\n' >&2; }
msg_unknown_arg() { printf '未知参数：%s；设备登录请不要传参数，手动接入请用 --token <令牌>\n' "$1" >&2; }
msg_no_device_login() { printf '当前环境不能进行浏览器设备登录，请使用 --token <令牌> 或设置 DSH_EE_TOKEN。\n' >&2; }
msg_no_node() { printf '没有找到 Node.js / npm：请先安装 Node.js（DSH 自带的 Node 也可以，把它加到 PATH）。\n' >&2; }
msg_bad_token_format() { printf '令牌格式不对\n' >&2; }
msg_bad_token() { printf '✘ 令牌无效或连不上 dsh-ee，请检查后重试\n' >&2; }
msg_token_ok() { printf '✔ 令牌有效\n'; }
msg_profiles_empty() { printf 'DSH_PROFILES 不能为空\n' >&2; }
msg_global_npmrc_written() { printf '✔ 已按 DSH_EE_GLOBAL_NPMRC=1 同时写入 ~/.npmrc\n'; }
msg_saved_env() { printf '✔ 已把接入令牌写进 %s（%s）\n' "$1" "$2"; }
msg_saved_pending() { printf '✔ 令牌已保存到 %s，DSH 启动后会导入凭据库并删除此文件\n' "$1"; }
msg_ask_remove_old() { printf '检测到 %s 中仍有旧令牌，是否删除？[y/N] ' "$1"; }
msg_old_token_removed() { printf '✔ 已删除 .env 中的旧令牌\n'; }
msg_old_token_kept() { printf '提示：已保留 .env 中的旧令牌；凭据库优先，如不再使用请手动删除该行。\n'; }
msg_no_connect_plugin() { printf '管理员还没有发布 DSH 接入插件；发布后重新运行本脚本即可接入企业技能与系统。\n'; }
msg_index_hint() { printf '现在可以先在 DSH 插件页 → 发现 → 添加索引源，填入\n  %s\n' "$1"; }
msg_need_pnpm() { printf '✘ 安装 DSH 插件需要 pnpm：先运行 npm install -g pnpm（或 corepack enable），再重新运行本脚本\n' >&2; }
msg_no_dsh_cmd() { printf '没有找到 dsh 命令。安装 DSH 后执行：\n  dsh plugin --profile %s add %s\n' "$1" "$2" >&2; }
msg_installed() { printf '✔ 已在 DSH 的 %s profile 里装好 %s\n' "$1" "$2"; }
msg_done() { printf '完成。重启 DSH（dsh web）后，企业技能、提示词、微应用与企业系统就会出现。\n'; }
msg_hub_hint() { printf '以后在 %s/hub 启用新的能力，DSH 会自动同步，不用再运行本脚本。\n' "$1"; }
msg_quit_dsh() { printf '请按 ⌘Q 完全退出 DSH，脚本会等待最多 120 秒。\n'; }
msg_dsh_running() { printf 'DSH 仍在运行，请完全退出后重新运行本命令。\n' >&2; }
msg_open_dsh_first() { printf '请先打开一次 DSH 再完全退出，然后重新运行本命令。\n' >&2; }
msg_saved_pending_desktop() { printf '✔ 令牌已保存，Desktop 启动后会导入凭据库\n'; }
msg_installed_desktop() { printf '✔ 已在 Desktop 的 desktop profile 里装好 %s\n' "$1"; }
msg_no_connect_plugin_desktop() { printf '管理员还没有发布 DSH 接入插件；发布后重新运行本命令即可。\n'; }
msg_done_desktop() { printf '完成。重新打开 DSH 后即可使用企业能力。\n'; }
`

const setupShellMessagesEn = `msg_wrote_npmrc() { printf '✔ Wrote %s (DSH only; ~/.npmrc untouched)\n' "$1"; }
msg_old_global_npmrc() { printf 'Note: ~/.npmrc still has dsh-ee settings written by an older setup script. DSH no longer needs them; if they get in your way, remove the lines containing %s and the _authToken line.\n' "$1"; }
msg_usage() { printf 'Usage: sh -s -- --token <token>\n' >&2; }
msg_unknown_arg() { printf 'Unknown argument: %s. Omit arguments for device sign-in; for manual setup use --token <token>\n' "$1" >&2; }
msg_no_device_login() { printf 'Browser device sign-in is not available in this environment. Use --token <token> or set DSH_EE_TOKEN.\n' >&2; }
msg_no_node() { printf 'Node.js / npm not found. Install Node.js first (the Node bundled with DSH works too; add it to PATH).\n' >&2; }
msg_bad_token_format() { printf 'Invalid token format\n' >&2; }
msg_bad_token() { printf '✘ The token is invalid or dsh-ee is unreachable. Check and retry\n' >&2; }
msg_token_ok() { printf '✔ Token is valid\n'; }
msg_profiles_empty() { printf 'DSH_PROFILES cannot be empty\n' >&2; }
msg_global_npmrc_written() { printf '✔ Also wrote ~/.npmrc because DSH_EE_GLOBAL_NPMRC=1\n'; }
msg_saved_env() { printf '✔ Wrote the access token to %s (%s)\n' "$1" "$2"; }
msg_saved_pending() { printf '✔ Token saved to %s; DSH imports it into the credential store and deletes this file on startup\n' "$1"; }
msg_ask_remove_old() { printf 'Found an old token in %s. Delete it? [y/N] ' "$1"; }
msg_old_token_removed() { printf '✔ Removed the old token from .env\n'; }
msg_old_token_kept() { printf 'Note: kept the old token in .env. The credential store takes precedence; delete the line manually if you no longer use it.\n'; }
msg_no_connect_plugin() { printf 'The admin has not published the DSH connect plugin yet. Run this script again after it is published to connect company skills and systems.\n'; }
msg_index_hint() { printf 'For now you can add an index source in DSH: Plugins → Discover → Add index source, and enter\n  %s\n' "$1"; }
msg_need_pnpm() { printf '✘ Installing the DSH plugin requires pnpm. Run npm install -g pnpm (or corepack enable), then run this script again\n' >&2; }
msg_no_dsh_cmd() { printf 'The dsh command was not found. After installing DSH, run:\n  dsh plugin --profile %s add %s\n' "$1" "$2" >&2; }
msg_installed() { printf '✔ Installed %s in the DSH %s profile\n' "$1" "$2"; }
msg_done() { printf 'Done. Restart DSH (dsh web) and company skills, prompts, micro-apps and systems will appear.\n'; }
msg_hub_hint() { printf 'From now on, enable new capabilities at %s/hub; DSH syncs automatically, so you will not need to run this script again.\n' "$1"; }
msg_quit_dsh() { printf 'Quit DSH completely with ⌘Q; the script waits up to 120 seconds.\n'; }
msg_dsh_running() { printf 'DSH is still running. Quit it completely and run this command again.\n' >&2; }
msg_open_dsh_first() { printf 'Open DSH once, quit it completely, then run this command again.\n' >&2; }
msg_saved_pending_desktop() { printf '✔ Token saved; Desktop imports it into the credential store on startup\n'; }
msg_installed_desktop() { printf '✔ Installed %s in the Desktop profile\n' "$1"; }
msg_no_connect_plugin_desktop() { printf 'The admin has not published the DSH connect plugin yet. Run this command again after it is published.\n'; }
msg_done_desktop() { printf 'Done. Reopen DSH to use company capabilities.\n'; }
`

// setupDeviceJSBody 是设备登录用的 node 脚本：提示文字放在 T 里（按语言生成），
// 逻辑与原来完全一致。
const setupDeviceJSBody = `
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const { spawn, spawnSync } = require("node:child_process");
const base = process.env.DSH_EE_SETUP_BASE.replace(/\/+$/, "");
const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
/*__SETUP_T__*/
const fmt = (s, ...a) => s.replace(/\{(\d+)\}/g, (_, i) => (a[+i] === undefined ? "" : String(a[+i])));
const post = async (url, body) => {
  const response = await fetch(base + url, { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify(body) });
  const data = await response.json().catch(() => ({}));
  return { status: response.status, data };
};
(async () => {
  const version = spawnSync("dsh", ["--version"], { encoding: "utf8" });
  const started = await post("/api/device/code", {
    client_name: os.hostname(),
    client_os: process.platform + " " + os.release(),
    client_version: version.status === 0 ? version.stdout.trim() : ""
  });
  if (!started.data.device_code) throw new Error(started.data.error || fmt(T.deviceStartFailed, started.status));
  const flow = started.data;
  console.log(fmt(T.browserConfirm, flow.verification_uri_complete));
  console.log(T.codeLabel + flow.user_code);
  console.log(T.waiting);
  const opener = process.platform === "darwin" ? "open" : process.platform === "win32" ? "cmd" : "xdg-open";
  const args = process.platform === "win32" ? ["/c", "start", "", flow.verification_uri_complete] : [flow.verification_uri_complete];
  try { spawn(opener, args, { detached: true, stdio: "ignore" }).unref(); } catch {}
  let interval = Math.max(1, Number(flow.interval) || 5) * 1000;
  const deadline = Date.now() + Math.max(1, Number(flow.expires_in) || 600) * 1000;
  while (Date.now() < deadline) {
    await sleep(interval);
    const polled = await post("/api/device/token", { device_code: flow.device_code });
    if (polled.data.access_token) {
      const home = process.env.DSH_HOME || path.join(os.homedir(), ".dsh");
      const dir = path.join(home, "dsh-ee");
      fs.mkdirSync(dir, { recursive: true, mode: 0o700 });
      fs.chmodSync(dir, 0o700);
      const pending = path.join(dir, "token.pending");
      fs.writeFileSync(pending, polled.data.access_token + "\n", { mode: 0o600 });
      fs.chmodSync(pending, 0o600);
      const user = polled.data.user || {};
      console.log(user.department ? fmt(T.loggedInWithDept, user.username || "", user.department) : fmt(T.loggedIn, user.username || ""));
      return;
    }
    // slow_down / rate_limited just mean "retry later"; they are not failures
    if (polled.data.error === "slow_down" || polled.data.error === "rate_limited") interval += 5000;
    else if (polled.data.error !== "authorization_pending") throw new Error(polled.data.error || fmt(T.deviceFailed, polled.status));
  }
  throw new Error(T.timeout);
})().catch((error) => { console.error("✘ " + error.message); process.exitCode = 1; });
`

// setupDeviceMsgKeys 固定字段顺序，生成的对象文案稳定，测试与 diff 更好读。
var setupDeviceMsgKeys = []string{
	"deviceStartFailed",
	"browserConfirm",
	"codeLabel",
	"waiting",
	"loggedIn",
	"loggedInWithDept",
	"deviceFailed",
	"timeout",
}

var setupDeviceMsgs = map[string]map[string]string{
	"zh": {
		"deviceStartFailed": "设备登录发起失败：HTTP {0}",
		"browserConfirm":    "请在浏览器中确认登录（已尝试自动打开）：\n  {0}",
		"codeLabel":         "确认码：",
		"waiting":           "等待确认…（Ctrl-C 取消）",
		"loggedIn":          "✔ 已登录 {0}",
		"loggedInWithDept":  "✔ 已登录 {0}（{1}）",
		"deviceFailed":      "设备登录失败：HTTP {0}",
		"timeout":           "设备登录超时，请重新运行 setup",
	},
	"en": {
		"deviceStartFailed": "Device sign-in failed to start: HTTP {0}",
		"browserConfirm":    "Confirm the sign-in in your browser (tried to open it automatically):\n  {0}",
		"codeLabel":         "Code: ",
		"waiting":           "Waiting for confirmation… (Ctrl-C to cancel)",
		"loggedIn":          "✔ Signed in as {0}",
		"loggedInWithDept":  "✔ Signed in as {0} ({1})",
		"deviceFailed":      "Device sign-in failed: HTTP {0}",
		"timeout":           "Device sign-in timed out. Run setup again",
	},
}

// jsMessagesObject 把一组文案渲染成 JS 对象字面量；Go 的 %q 引号规则与 JS 兼容。
func jsMessagesObject(msgs map[string]string) string {
	var b strings.Builder
	b.WriteString("{ ")
	for i, k := range setupDeviceMsgKeys {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(k)
		b.WriteString(": ")
		b.WriteString(strconv.Quote(msgs[k]))
	}
	b.WriteString(" }")
	return b.String()
}

// setupDeviceJS 生成设备登录的 node 脚本：
//   - 指定语言时只带该语言文案（en 脚本里没有中文）；
//   - 自动模式按 setup.sh 导出的 DSH_EE_SETUP_LANG 在两种文案间选择。
func setupDeviceJS(lang string) string {
	var decl string
	switch lang {
	case "zh", "en":
		decl = "const T = " + jsMessagesObject(setupDeviceMsgs[lang]) + ";"
	default:
		decl = `const T = process.env.DSH_EE_SETUP_LANG === "zh" ? ` + jsMessagesObject(setupDeviceMsgs["zh"]) + ` : ` + jsMessagesObject(setupDeviceMsgs["en"]) + `;`
	}
	return strings.Replace(setupDeviceJSBody, "/*__SETUP_T__*/", decl, 1)
}
