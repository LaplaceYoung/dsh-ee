// 由 dsh-plugin/ 生成，勿手改。

var __create = Object.create;
var __defProp = Object.defineProperty;
var __getOwnPropDesc = Object.getOwnPropertyDescriptor;
var __knownSymbol = (name2, symbol) => (symbol = Symbol[name2]) ? symbol : Symbol.for("Symbol." + name2);
var __typeError = (msg) => {
  throw TypeError(msg);
};
var __defNormalProp = (obj, key, value) => key in obj ? __defProp(obj, key, { enumerable: true, configurable: true, writable: true, value }) : obj[key] = value;
var __name = (target, value) => __defProp(target, "name", { value, configurable: true });
var __decoratorStart = (base) => [, , , __create(base?.[__knownSymbol("metadata")] ?? null)];
var __decoratorStrings = ["class", "method", "getter", "setter", "accessor", "field", "value", "get", "set"];
var __expectFn = (fn) => fn !== void 0 && typeof fn !== "function" ? __typeError("Function expected") : fn;
var __decoratorContext = (kind, name2, done, metadata, fns) => ({ kind: __decoratorStrings[kind], name: name2, metadata, addInitializer: (fn) => done._ ? __typeError("Already initialized") : fns.push(__expectFn(fn || null)) });
var __decoratorMetadata = (array, target) => __defNormalProp(target, __knownSymbol("metadata"), array[3]);
var __runInitializers = (array, flags, self, value) => {
  for (var i = 0, fns = array[flags >> 1], n = fns && fns.length; i < n; i++) flags & 1 ? fns[i].call(self) : value = fns[i].call(self, value);
  return value;
};
var __decorateElement = (array, flags, name2, decorators, target, extra) => {
  var fn, it, done, ctx, access, k = flags & 7, s = !!(flags & 8), p = !!(flags & 16);
  var j = k > 3 ? array.length + 1 : k ? s ? 1 : 2 : 0, key = __decoratorStrings[k + 5];
  var initializers = k > 3 && (array[j - 1] = []), extraInitializers = array[j] || (array[j] = []);
  var desc = k && (!p && !s && (target = target.prototype), k < 5 && (k > 3 || !p) && __getOwnPropDesc(k < 4 ? target : { get [name2]() {
    return __privateGet(this, extra);
  }, set [name2](x) {
    return __privateSet(this, extra, x);
  } }, name2));
  k ? p && k < 4 && __name(extra, (k > 2 ? "set " : k > 1 ? "get " : "") + name2) : __name(target, name2);
  for (var i = decorators.length - 1; i >= 0; i--) {
    ctx = __decoratorContext(k, name2, done = {}, array[3], extraInitializers);
    if (k) {
      ctx.static = s, ctx.private = p, access = ctx.access = { has: p ? (x) => __privateIn(target, x) : (x) => name2 in x };
      if (k ^ 3) access.get = p ? (x) => (k ^ 1 ? __privateGet : __privateMethod)(x, target, k ^ 4 ? extra : desc.get) : (x) => x[name2];
      if (k > 2) access.set = p ? (x, y) => __privateSet(x, target, y, k ^ 4 ? extra : desc.set) : (x, y) => x[name2] = y;
    }
    it = (0, decorators[i])(k ? k < 4 ? p ? extra : desc[key] : k > 4 ? void 0 : { get: desc.get, set: desc.set } : target, ctx), done._ = 1;
    if (k ^ 4 || it === void 0) __expectFn(it) && (k > 4 ? initializers.unshift(it) : k ? p ? extra = it : desc[key] = it : target = it);
    else if (typeof it !== "object" || it === null) __typeError("Object expected");
    else __expectFn(fn = it.get) && (desc.get = fn), __expectFn(fn = it.set) && (desc.set = fn), __expectFn(fn = it.init) && initializers.unshift(fn);
  }
  return k || __decoratorMetadata(array, target), desc && __defProp(target, name2, desc), p ? k ^ 4 ? extra : desc : target;
};
var __accessCheck = (obj, member, msg) => member.has(obj) || __typeError("Cannot " + msg);
var __privateIn = (member, obj) => Object(obj) !== obj ? __typeError('Cannot use the "in" operator on this value') : member.has(obj);
var __privateGet = (obj, member, getter) => (__accessCheck(obj, member, "read from private field"), getter ? getter.call(obj) : member.get(obj));
var __privateSet = (obj, member, value, setter) => (__accessCheck(obj, member, "write to private field"), setter ? setter.call(obj, value) : member.set(obj, value), value);
var __privateMethod = (obj, member, method) => (__accessCheck(obj, member, "access private method"), method);

// src/config.ts
import os from "node:os";
import path from "node:path";
function normalizeConfig(input) {
  const base = String(input.base ?? "").replace(/\/+$/, "");
  if (!/^https?:\/\//.test(base)) throw new Error("[dsh-ee] base \u5FC5\u987B\u662F http(s) \u5730\u5740");
  return {
    base,
    org: input.org || "\u4F01\u4E1A",
    serverName: input.serverName || "ee",
    tokenRef: input.tokenRef || "DSHEE_TOKEN",
    autoInstall: input.autoInstall !== false,
    version: input.version || "dev",
    scopes: Array.isArray(input.scopes) ? input.scopes.filter((s) => typeof s === "string" && s.startsWith("@")) : []
  };
}
function dshHome() {
  return process.env.DSH_HOME || path.join(os.homedir(), ".dsh");
}
function npmrcPath(ctx) {
  const dir = ctx.get("profileContext")?.dir;
  if (typeof dir === "string" && dir) return path.join(dir, ".npmrc");
  return process.env.NPM_CONFIG_USERCONFIG || path.join(os.homedir(), ".npmrc");
}
function npmrcEntries(config, token) {
  const registry = `${config.base}/npm/`;
  const entries = /* @__PURE__ */ new Map();
  for (const scope of config.scopes) entries.set(`${scope}:registry`, registry);
  entries.set(`${registry.replace(/^https?:/, "")}:_authToken`, token);
  return entries;
}

// src/token.ts
import fs from "node:fs";
import path2 from "node:path";
function npmrcState(ctx, config, token) {
  const file = npmrcPath(ctx);
  if (!fs.existsSync(file)) return "missing";
  const lines = new Map(fs.readFileSync(file, "utf8").split(/\r?\n/).filter((l) => l.includes("=")).map((l) => [l.slice(0, l.indexOf("=")), l.slice(l.indexOf("=") + 1)]));
  let state = "ok";
  for (const [key, value] of npmrcEntries(config, token)) {
    const have = lines.get(key);
    if (have === void 0) return "missing";
    if (have !== value) state = "mismatch";
  }
  return state;
}
var TokenManager = class {
  constructor(ctx, config) {
    this.ctx = ctx;
    this.config = config;
  }
  tokenPath() {
    return path2.join(dshHome(), "dsh-ee", "token");
  }
  envPath() {
    return path2.join(dshHome(), ".env");
  }
  async resolve() {
    let stored;
    try {
      stored = await this.ctx.get("credentials")?.resolve?.(this.config.tokenRef);
    } catch {
      stored = void 0;
    }
    if (stored?.value) return stored;
    try {
      const value = fs.readFileSync(this.tokenPath(), "utf8").trim();
      if (value) return { value, source: "file" };
    } catch {
    }
    return void 0;
  }
  async set(value) {
    const credentials = this.ctx.get("credentials");
    if (typeof credentials?.set === "function") {
      try {
        await credentials.set(this.config.tokenRef, value);
        fs.rmSync(this.tokenPath(), { force: true });
        return;
      } catch {
      }
    }
    const file = this.tokenPath();
    fs.mkdirSync(path2.dirname(file), { recursive: true, mode: 448 });
    fs.writeFileSync(file, `${value}
`, { mode: 384 });
    this.writeEnvToken(value);
  }
  async unset(expected) {
    fs.rmSync(this.tokenPath(), { force: true });
    try {
      await this.ctx.get("credentials")?.unset(this.config.tokenRef);
    } catch {
    }
    this.stripEnvToken(expected);
  }
  writeEnvToken(value) {
    const file = this.envPath();
    const before = fs.existsSync(file) ? fs.readFileSync(file, "utf8") : "";
    const kept = before.split(/\r?\n/).filter((line) => line.trim() && !line.startsWith(`${this.config.tokenRef}=`));
    const after = `${[...kept, `${this.config.tokenRef}=${value}`].join("\n")}
`;
    if (after === before) return;
    fs.mkdirSync(path2.dirname(file), { recursive: true });
    fs.writeFileSync(file, after, { mode: 384 });
  }
  stripEnvToken(expected) {
    const file = this.envPath();
    if (!fs.existsSync(file)) return;
    const prefix = `${this.config.tokenRef}=`;
    const lines = fs.readFileSync(file, "utf8").split(/\r?\n/);
    const hit = lines.find((line) => line.startsWith(prefix));
    if (!hit) return;
    if (expected !== void 0 && hit.slice(prefix.length) !== expected) return;
    fs.writeFileSync(file, lines.filter((line) => line !== hit).join("\n"), { mode: 384 });
  }
  pendingPath() {
    return path2.join(dshHome(), "dsh-ee", "token.pending");
  }
  async importPending() {
    const file = this.pendingPath();
    if (!fs.existsSync(file)) return false;
    const token = fs.readFileSync(file, "utf8").trim();
    if (!token) {
      fs.unlinkSync(file);
      return false;
    }
    await this.set(token);
    fs.unlinkSync(file);
    return true;
  }
  async invalidate(failedToken) {
    const current = await this.resolve();
    if (current?.value !== failedToken) return false;
    await this.unset(failedToken);
    return true;
  }
  /** 把企业作用域与令牌写进 profile 的 .npmrc，其它行原样保留；内容没变就不写。返回是否改动了文件。 */
  async syncNpmrc(token) {
    const file = npmrcPath(this.ctx);
    const entries = npmrcEntries(this.config, token);
    const before = fs.existsSync(file) ? fs.readFileSync(file, "utf8") : "";
    const kept = before.split(/\r?\n/).filter((line) => line && !entries.has(line.split("=", 1)[0]));
    const after = [...kept, ...[...entries].map(([key, value]) => `${key}=${value}`)].join("\n") + "\n";
    if (after === before) return false;
    fs.mkdirSync(path2.dirname(file), { recursive: true });
    fs.writeFileSync(file, after, { mode: 384 });
    fs.chmodSync(file, 384);
    return true;
  }
  async npmrcMatches(token) {
    return npmrcState(this.ctx, this.config, token) === "ok";
  }
};

// src/http.ts
function dshHost(versions = process.versions) {
  const electron = versions.electron;
  if (!electron) return "cli";
  return `desktop/${process.env.DSH_DESKTOP_VERSION || process.env.DSH_VERSION || electron}`;
}
var HttpClient = class {
  constructor(tokens) {
    this.tokens = tokens;
  }
  async fetch(path8, init = {}, timeoutMs = 3e4) {
    const current = await this.tokens.resolve();
    const headers = new Headers(init.headers);
    if (current?.value) headers.set("Authorization", `Bearer ${current.value}`);
    const config = this.tokens.config;
    if (!headers.has("X-DshEE-Client")) headers.set("X-DshEE-Client", `dsh-connect/${config.version || "dev"}`);
    if (!headers.has("X-DshEE-Host")) headers.set("X-DshEE-Host", dshHost());
    const timeout = timeoutMs > 0 ? AbortSignal.timeout(timeoutMs) : void 0;
    const signal = init.signal && timeout ? AbortSignal.any([init.signal, timeout]) : timeout ?? init.signal ?? void 0;
    const response = await fetch(`${config.base}${path8}`, { ...init, headers, signal });
    if (response.status === 401 && current?.value) {
      const body = await response.clone().json().catch(() => null);
      if (body?.code?.startsWith("token_")) await this.tokens.invalidate(current.value);
    }
    return response;
  }
  async json(path8, init) {
    const response = await this.fetch(path8, init);
    if (!response.ok) throw new Error(`HTTP ${response.status}`);
    return response.json();
  }
};

// src/device.ts
import { execFile } from "node:child_process";
import os2 from "node:os";
var DeviceFlow = class {
  constructor(config, fetchImpl = fetch) {
    this.config = config;
    this.fetchImpl = fetchImpl;
  }
  /** setupCode：网页「开始使用」页给的接入码，带上后这次登录会出现在那个页面上，可以就地确认。 */
  async start(setupCode = "") {
    const response = await this.fetchImpl(`${this.config.base}/api/device/code`, {
      method: "POST",
      headers: { "content-type": "application/json" },
      signal: AbortSignal.timeout(3e4),
      body: JSON.stringify({ client_name: os2.hostname(), client_os: `${process.platform} ${os2.release()}`, client_version: this.config.version, ...setupCode ? { setup_code: setupCode } : {} })
    });
    if (!response.ok) {
      const body = await response.json().catch(() => null);
      throw new Error(body?.error ? `\u8BBE\u5907\u767B\u5F55\u53D1\u8D77\u5931\u8D25\uFF1A${body.error}` : `\u8BBE\u5907\u767B\u5F55\u53D1\u8D77\u5931\u8D25\uFF1AHTTP ${response.status}`);
    }
    return response.json();
  }
  async wait(deviceCode, intervalSeconds = 5, timeoutMs = 6e5, signal) {
    const deadline = Date.now() + timeoutMs;
    let interval = Math.max(1, intervalSeconds) * 1e3;
    while (Date.now() < deadline) {
      if (signal?.aborted) throw new Error("\u8BBE\u5907\u767B\u5F55\u5DF2\u53D6\u6D88");
      const response = await this.fetchImpl(`${this.config.base}/api/device/token`, {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ device_code: deviceCode }),
        signal: signal ? AbortSignal.any([signal, AbortSignal.timeout(3e4)]) : AbortSignal.timeout(3e4)
      });
      const body = await response.json();
      if (body.access_token) return body;
      if (body.error === "slow_down" || body.error === "rate_limited") {
        interval += 5e3;
      } else if (body.error !== "authorization_pending") {
        throw new Error(body.error || `\u8BBE\u5907\u767B\u5F55\u5931\u8D25\uFF1AHTTP ${response.status}`);
      }
      await new Promise((resolve, reject) => {
        const timer = setTimeout(resolve, interval);
        signal?.addEventListener("abort", () => {
          clearTimeout(timer);
          reject(new Error("\u8BBE\u5907\u767B\u5F55\u5DF2\u53D6\u6D88"));
        }, { once: true });
      });
    }
    throw new Error("\u8BBE\u5907\u767B\u5F55\u8D85\u65F6");
  }
};
function openBrowser(url) {
  if (process.platform === "win32") execFile("cmd", ["/c", "start", "", url], () => {
  });
  else execFile(process.platform === "darwin" ? "open" : "xdg-open", [url], () => {
  });
}

// src/commands.ts
function parseSetupCode(rawInput) {
  const code = String(rawInput ?? "").trim().toUpperCase();
  return /^[A-Z0-9]{4}-?[A-Z0-9]{4}$/.test(code) ? code : "";
}
function registerCommands(ctx, config, tokens, onLogin = async () => {
}) {
  const flow = new DeviceFlow(config);
  let pending = null;
  const login = async (rawInput) => {
    if (pending) return { kind: "success", text: `\u8BF7\u5728\u6D4F\u89C8\u5668\u786E\u8BA4\uFF0C\u786E\u8BA4\u7801 ${pending.code}\uFF0C\u94FE\u63A5 ${pending.url}` };
    const setupCode = parseSetupCode(rawInput);
    if (String(rawInput ?? "").trim() && !setupCode) return { kind: "error", text: "\u63A5\u5165\u7801\u683C\u5F0F\u4E0D\u5BF9\uFF0C\u5E94\u5F62\u5982 ABCD-EFGH\uFF1B\u4E0D\u5E26\u63A5\u5165\u7801\u76F4\u63A5\u8F93\u5165 /ee-login \u4E5F\u53EF\u4EE5" };
    const started = await flow.start(setupCode);
    const url = started.verification_uri_complete;
    if (!setupCode) openBrowser(url);
    const task = (async () => {
      try {
        const result = await flow.wait(started.device_code, started.interval);
        await tokens.set(result.access_token);
        await tokens.syncNpmrc(result.access_token);
        await onLogin().catch(() => void 0);
      } finally {
        pending = null;
      }
    })();
    pending = { code: started.user_code, url, task };
    void task.catch(() => void 0);
    if (setupCode) return { kind: "success", text: `\u56DE\u5230 dsh-ee \u7F51\u9875\u7684\u300C\u5F00\u59CB\u4F7F\u7528\u300D\u9875\uFF0C\u6838\u5BF9\u786E\u8BA4\u7801 ${started.user_code} \u540E\u70B9\u300C\u786E\u8BA4\u6388\u6743\u300D\uFF08\u7F51\u9875\u5173\u4E86\u7684\u8BDD\u6253\u5F00 ${url}\uFF09\u3002\u786E\u8BA4\u540E\u56DE\u5230\u8FD9\u91CC\u7EE7\u7EED\u5BF9\u8BDD\u3002` };
    return { kind: "success", text: `\u8BF7\u5728\u6D4F\u89C8\u5668\u786E\u8BA4\uFF0C\u786E\u8BA4\u7801 ${started.user_code}\uFF0C\u94FE\u63A5 ${url}` };
  };
  const commands = ctx.get("commands");
  if (commands) {
    const register = (definition) => {
      const dispose = commands.register(definition);
      ctx.effect(() => dispose);
    };
    register({
      name: "ee-login",
      description: "\u767B\u5F55\u4F01\u4E1A\u8D26\u53F7\uFF08\u53EF\u4EE5\u5E26\u4E0A\u7F51\u9875\u300C\u5F00\u59CB\u4F7F\u7528\u300D\u9875\u7ED9\u7684\u63A5\u5165\u7801\uFF09",
      input: { hint: "[\u63A5\u5165\u7801]" },
      handler: (invocation) => login(invocation?.rawInput)
    });
    register({
      name: "ee-status",
      description: "\u67E5\u770B\u4F01\u4E1A\u767B\u5F55\u72B6\u6001",
      handler: async () => {
        const current = await tokens.resolve();
        if (!current) return { kind: "success", text: "\u672A\u767B\u5F55\uFF0C\u5728\u5BF9\u8BDD\u6846\u8F93\u5165 /ee-login" };
        const response = await fetch(`${config.base}/api/dsh/whoami`, { headers: { Authorization: `Bearer ${current.value}` }, signal: AbortSignal.timeout(3e4) });
        if (!response.ok) return { kind: "success", text: "\u767B\u5F55\u5DF2\u5931\u6548\uFF0C\u5728\u5BF9\u8BDD\u6846\u8F93\u5165 /ee-login" };
        const me = await response.json();
        const matches = await tokens.npmrcMatches(current.value);
        const expiry = me.expires_at ? ` \xB7 ${new Date(me.expires_at).toLocaleDateString()} \u5230\u671F` : " \xB7 \u957F\u671F\u6709\u6548";
        return { kind: "success", text: `\u5DF2\u767B\u5F55 ${me.username}\uFF08${me.department}\uFF09\xB7 \u4EE4\u724C ${me.token_prefix}${expiry} \xB7 \u63D2\u4EF6 ${config.version} \xB7 npmrc ${matches ? "\u4E00\u81F4" : "\u4E0D\u4E00\u81F4"}` };
      }
    });
    register({
      name: "ee-logout",
      description: "\u9000\u51FA\u4F01\u4E1A\u8D26\u53F7",
      handler: async () => {
        const current = await tokens.resolve();
        if (!current) return { kind: "success", text: "\u5F53\u524D\u6CA1\u6709\u767B\u5F55\u4F01\u4E1A\u8D26\u53F7" };
        const revoked = await fetch(`${config.base}/api/dsh/logout`, { method: "POST", headers: { Authorization: `Bearer ${current.value}` }, signal: AbortSignal.timeout(3e4) }).then((response) => response.ok, () => false);
        await tokens.unset(current.value);
        const left = await tokens.resolve();
        if (left?.value === current.value) {
          return {
            kind: "error",
            text: `${revoked ? "\u5DF2\u5728\u670D\u52A1\u7AEF\u540A\u9500\u8BE5\u4EE4\u724C\uFF0C" : ""}\u4F46\u5B83\u6765\u81EA\u73AF\u5883\u53D8\u91CF\u6216 $DSH_HOME/.env\uFF08${config.tokenRef}\uFF09\uFF0C\u8BF7\u624B\u52A8\u5220\u9664\u540E\u91CD\u542F DSH`
          };
        }
        return { kind: "success", text: revoked ? "\u5DF2\u9000\u51FA\u4F01\u4E1A\u8D26\u53F7" : "\u672C\u673A\u5DF2\u9000\u51FA\uFF1B\u8FDE\u4E0D\u4E0A dsh-ee\uFF0C\u4EE4\u724C\u672A\u80FD\u5728\u670D\u52A1\u7AEF\u540A\u9500\uFF0C\u53EF\u5728\u7F51\u9875\u300C\u6211\u7684\u4EE4\u724C\u300D\u91CC\u540A\u9500" };
      }
    });
  }
  return { login };
}

// src/plugins.ts
var installing = false;
async function installRequired(ctx, config, required) {
  if (!config.autoInstall || installing || required.length === 0) return;
  const manager = ctx.get("pluginManager");
  if (!manager) return;
  installing = true;
  try {
    const have = new Set((await manager.listBundles()).filter((b) => b.installed || b.enabled).map((b) => b.name));
    for (const plugin of required) {
      if (have.has(plugin.name)) continue;
      await manager.installBundle(plugin.name, { registry: `${config.base}/npm/` });
    }
  } finally {
    installing = false;
  }
}

// src/skills.ts
async function registerSkills(ctx, config, http) {
  let catalog = { skills: [], required_plugins: [] };
  let etag = "";
  const refresh = async () => {
    try {
      const response = await http.fetch("/api/dsh/catalog", { headers: etag ? { "If-None-Match": etag } : {} });
      if (response.status === 304) return false;
      if (!response.ok) return false;
      catalog = await response.json();
      etag = response.headers.get("etag") ?? "loaded";
      return true;
    } catch {
      return false;
    }
  };
  const install = () => installRequired(ctx, config, catalog.required_plugins).catch(() => void 0);
  void (async () => {
    await refresh();
    await install();
  })();
  let invalidate = () => {
  };
  ctx.skills.registerProvider((control) => {
    invalidate = () => control.invalidate();
    let timer;
    const pollSeconds = Math.max(15, Number(config.pollSeconds) || 60) * 1e3;
    const tick = async () => {
      try {
        if (await refresh()) {
          control.invalidate();
          await install();
        }
      } finally {
        if (!control.signal.aborted) timer = setTimeout(tick, pollSeconds);
      }
    };
    timer = setTimeout(tick, 1e3);
    control.signal.addEventListener("abort", () => timer && clearTimeout(timer));
    return {
      name: "dsh-ee",
      async list() {
        if (!etag) await refresh();
        return catalog.skills.map((skill) => ({
          name: skill.name,
          description: skill.description || skill.title,
          invocation: { modelInvocable: !!skill.model_invocable, userInvocable: !!skill.user_invocable },
          provider: "dsh-ee",
          source: "dsh-ee",
          rank: 350,
          locator: skill.name,
          metadata: { title: skill.title, kind: skill.kind, version: skill.version },
          ...skill.when_to_use ? { whenToUse: skill.when_to_use } : {}
        }));
      },
      async get(candidate, options) {
        const response = await http.fetch(`/api/dsh/skills/${encodeURIComponent(String(candidate.locator))}`, { signal: options.signal }).catch(() => void 0);
        if (!response) return void 0;
        if (!response.ok) return void 0;
        const body = await response.json();
        const { rank: _rank, locator: _locator, ...summary } = candidate;
        return { ...summary, content: body.content };
      }
    };
  });
  return {
    readBrand: async () => {
      await refresh();
      return catalog.brand ?? { enabled: true, name: config.org, logo_url: "", revision: "" };
    },
    readApps: async () => {
      await refresh();
      return (catalog.apps ?? []).map((app) => ({ ...app, web_url: appWebUrl(config.base, app), detail_url: app.asset_id ? `${config.base}/assets/${app.asset_id}` : void 0 }));
    },
    readAvailableApps: async () => {
      await refresh();
      return (catalog.available_apps ?? []).map((app) => ({ ...app, web_url: appWebUrl(config.base, app), detail_url: app.asset_id ? `${config.base}/assets/${app.asset_id}` : void 0 }));
    },
    refreshNow: async () => {
      if (await refresh()) {
        invalidate();
        await install();
      }
    }
  };
}
function appWebUrl(base, app) {
  if (app.web) return `${base}/a/${encodeURIComponent(app.name)}`;
  return app.asset_id ? `${base}/assets/${app.asset_id}` : void 0;
}

// src/write-log.ts
import fs2 from "node:fs";
import path3 from "node:path";
import { createHash } from "node:crypto";
var MAX_PER_SESSION = 200;
function writeOf(tool, args) {
  if (!tool || tool.annotations?.readOnlyHint === true) return void 0;
  const meta = tool._meta?.dshee ?? {};
  const actions = meta.actions;
  if (actions && typeof actions === "object") {
    const action = actions[String(args?.action)];
    if (!action) return void 0;
    if (action.e === "danger" && !args?.confirm) return void 0;
    return { label: String(action.zh || args?.action) };
  }
  const name2 = String(tool.name ?? "");
  const short = name2.includes("__") ? name2.slice(name2.indexOf("__") + 2) : name2;
  const asset = Number(meta.asset);
  return {
    label: short,
    ...typeof meta.system === "string" && meta.system ? { system: meta.system } : systemOf(tool.description),
    ...Number.isInteger(asset) && asset > 0 ? { asset } : {}
  };
}
function systemOf(description) {
  const match = /^【([^】]{1,80})】/.exec(String(description ?? ""));
  return match ? { system: match[1] } : {};
}
function resultOk(result) {
  return !(result && typeof result === "object" && result.isError === true);
}
function writesBetween(records, from, to) {
  return records.filter((item) => item.at >= from && (to === void 0 || item.at <= to));
}
var safeName = /^[A-Za-z0-9_-]{1,80}$/;
var WriteLog = class {
  constructor(dir = path3.join(dshHome(), "dsh-ee", "writes")) {
    this.dir = dir;
  }
  sequence = 0;
  file(sessionId) {
    if (typeof sessionId !== "string" || !sessionId) throw new Error("\u4F1A\u8BDD id \u4E0D\u5408\u6CD5");
    const name2 = safeName.test(sessionId) ? sessionId : createHash("sha256").update(sessionId).digest("hex").slice(0, 32);
    return path3.join(this.dir, `${name2}.json`);
  }
  read(sessionId) {
    try {
      const value = JSON.parse(fs2.readFileSync(this.file(sessionId), "utf8"));
      if (value?.sessionId !== sessionId || !Array.isArray(value?.writes)) return [];
      return value.writes;
    } catch {
      return [];
    }
  }
  record(sessionId, item) {
    const writes = [...this.read(sessionId), item].slice(-MAX_PER_SESSION);
    const file = this.file(sessionId);
    fs2.mkdirSync(this.dir, { recursive: true, mode: 448 });
    const temp = `${file}.${process.pid}.${++this.sequence}.tmp`;
    try {
      fs2.writeFileSync(temp, JSON.stringify({ sessionId, writes }) + "\n", { mode: 384 });
      fs2.renameSync(temp, file);
    } finally {
      if (fs2.existsSync(temp)) fs2.unlinkSync(temp);
    }
  }
  between(sessionId, from, to) {
    if (typeof sessionId !== "string" || !sessionId || !Number.isFinite(from)) return [];
    return writesBetween(this.read(sessionId), from, Number.isFinite(to) ? to : void 0);
  }
};

// src/bridge.ts
function renderResult(value) {
  if (value && Array.isArray(value.content)) {
    const text = value.content.filter((part) => part?.type === "text").map((part) => String(part.text)).join("\n");
    return value.isError ? `\u9519\u8BEF\uFF1A${text}` : text;
  }
  return JSON.stringify(value);
}
function canonArgs(args) {
  const sort = (value) => {
    if (Array.isArray(value)) return value.map(sort);
    if (value && typeof value === "object") {
      return Object.fromEntries(Object.keys(value).filter((key) => key !== "confirm").sort().map((key) => [key, sort(value[key])]));
    }
    return value;
  };
  return JSON.stringify(sort(args ?? {}));
}
function listChangedCounter() {
  const decoder = new TextDecoder();
  let buffer = "";
  return {
    push(chunk) {
      if (!chunk) return 0;
      buffer += decoder.decode(chunk, { stream: true });
      let count = 0;
      for (; ; ) {
        const end = buffer.search(/\r?\n\r?\n/);
        if (end < 0) break;
        const event = buffer.slice(0, end);
        buffer = buffer.slice(end).replace(/^\r?\n\r?\n/, "");
        const data = event.split(/\r?\n/).filter((line) => line.startsWith("data:")).map((line) => line.slice(5).trim()).join("\n");
        if (!data) continue;
        try {
          if (JSON.parse(data)?.method === "notifications/tools/list_changed") count++;
        } catch {
        }
      }
      return count;
    }
  };
}
var McpBridge = class {
  constructor(ctx, config, http) {
    this.ctx = ctx;
    this.config = config;
    this.http = http;
  }
  disposers = /* @__PURE__ */ new Map();
  /** 已注册工具的定义指纹（DSH 里的工具全名 -> 定义的 JSON），用来发现「名字没变、定义变了」 */
  signatures = /* @__PURE__ */ new Map();
  session = "";
  controller;
  /** 结束长连接的退避等待，马上重连（刚登录时用）。 */
  wakeUp;
  /** DSH 里的工具全名 -> 服务端工具名 */
  serverNames = /* @__PURE__ */ new Map();
  metas = /* @__PURE__ */ new Map();
  previews = /* @__PURE__ */ new Map();
  local = /* @__PURE__ */ new Map();
  /** 服务端在 tools/list 里标了 readOnlyHint 的工具（服务端工具名）；工作台取数只能用它们 */
  readOnly = /* @__PURE__ */ new Set();
  status = { lastSyncAt: 0, tools: 0, error: "" };
  /** 有写操作完成时调用；index.ts 接到 WriteLog 上 */
  onWrite;
  async rpc(method, params, timeoutMs = 3e4, retried = false) {
    const headers = { "content-type": "application/json", accept: "application/json, text/event-stream" };
    if (this.session) headers["Mcp-Session-Id"] = this.session;
    const response = await this.http.fetch("/mcp", { method: "POST", headers, body: JSON.stringify({ jsonrpc: "2.0", id: 1, method, params }) }, timeoutMs);
    if (response.status === 404) {
      this.session = "";
      if (retried) throw new Error(`MCP ${method} HTTP 404`);
      return this.rpc(method, params, timeoutMs, true);
    }
    if (!response.ok) throw new Error(`MCP ${method} HTTP ${response.status}`);
    this.session = response.headers.get("mcp-session-id") || this.session;
    const body = await response.json();
    if (body.error) throw new Error(body.error.message);
    return body.result;
  }
  /** 注册由本机完成的 action（例如要读本机文件的上传）。键是 `<工具>.<action>`。 */
  registerLocal(key, fn) {
    this.local.set(key, fn);
  }
  /** 直接调服务端工具（本机诊断用）。 */
  async callTool(name2, args) {
    return this.rpc("tools/call", { name: name2, arguments: args });
  }
  /** 当前员工能用、并且服务端标为只读的工具；没权限的工具不在 tools/list 里，也就不在这里。 */
  isReadOnlyTool(serverName) {
    return this.readOnly.has(serverName);
  }
  /** 当前员工能不能用这个工具（服务端按权限列出的工具）。 */
  hasTool(serverName) {
    return [...this.serverNames.values()].includes(serverName);
  }
  /** 这次调用该不该先让用户批准；只读的返回 undefined。 */
  approval(fullName, args) {
    const metas = this.metas.get(fullName);
    const meta = metas?.[String(args?.action)];
    if (!meta) return void 0;
    if (meta.e === "danger") {
      if (!args?.confirm) return void 0;
      return { effect: "danger", zh: meta.zh, en: meta.en, preview: this.previews.get(`${fullName}
${canonArgs(args)}`) };
    }
    return { effect: "write", zh: meta.zh, en: meta.en };
  }
  remember(fullName, args, result) {
    try {
      const text = result?.content?.[0]?.text;
      if (typeof text !== "string" || !text.includes("confirm_required")) return;
      const body = JSON.parse(text);
      if (body?.confirm_required && typeof body.preview === "string") {
        if (this.previews.size > 50) this.previews.clear();
        this.previews.set(`${fullName}
${canonArgs(args)}`, body.preview);
      }
    } catch {
    }
  }
  /** 写操作记一笔；记录失败不能影响工具结果。exec.agent.session.id 是调用它的会话。 */
  noteWrite(tool, args, exec, at, ok) {
    const sessionId = exec?.agent?.session?.id;
    if (!this.onWrite || typeof sessionId !== "string" || !sessionId) return;
    const write2 = writeOf(tool, args);
    if (!write2) return;
    try {
      this.onWrite(sessionId, { at, tool: String(tool.name), ok, ...write2 });
    } catch {
    }
  }
  register(tool) {
    const name2 = `mcp__${this.config.serverName}__${tool.name}`;
    const actions = tool._meta?.dshee?.actions;
    if (actions && typeof actions === "object") this.metas.set(name2, actions);
    this.serverNames.set(name2, tool.name);
    if (tool.annotations?.readOnlyHint === true) this.readOnly.add(tool.name);
    const dispose = this.ctx.tools.register({
      name: name2,
      description: tool.description || tool.name,
      parameters: tool.inputSchema || { type: "object", properties: {} },
      output: { schema: {}, render: (_args, value) => [{ type: "text", text: renderResult(value) }] },
      execute: async (args, exec) => {
        const local = this.local.get(`${tool.name}.${args?.action}`);
        if (local) return local(args);
        const at = Date.now();
        let result;
        try {
          result = await this.rpc("tools/call", { name: tool.name, arguments: args }, 0);
        } catch (error) {
          this.noteWrite(tool, args, exec, at, false);
          throw error;
        }
        this.remember(name2, args, result);
        this.noteWrite(tool, args, exec, at, resultOk(result));
        return result;
      }
    });
    this.signatures.set(name2, JSON.stringify(tool));
    this.disposers.set(name2, () => {
      dispose();
      this.signatures.delete(name2);
      this.metas.delete(name2);
      this.serverNames.delete(name2);
      this.readOnly.delete(tool.name);
    });
  }
  async sync() {
    let result;
    try {
      if (!this.session) await this.rpc("initialize", { protocolVersion: "2025-06-18", capabilities: {}, clientInfo: { name: "dsh-ee-connect", version: this.config.version } });
      result = await this.rpc("tools/list");
    } catch (error) {
      this.status.error = error instanceof Error ? error.message : String(error);
      throw error;
    }
    this.status.error = "";
    this.status.lastSyncAt = Date.now();
    this.status.tools = (result.tools ?? []).length;
    const next = new Map((result.tools ?? []).map((tool) => [`mcp__${this.config.serverName}__${tool.name}`, tool]));
    for (const [name2, dispose] of this.disposers) {
      const tool = next.get(name2);
      if (!tool || JSON.stringify(tool) !== this.signatures.get(name2)) {
        dispose();
        this.disposers.delete(name2);
      }
    }
    for (const [name2, tool] of next) {
      if (!this.disposers.has(name2)) this.register(tool);
    }
  }
  async start() {
    this.controller = new AbortController();
    const signal = this.controller.signal;
    await this.sync().catch(() => void 0);
    void (async () => {
      let delay = 1e3;
      while (!signal.aborted) {
        try {
          const response = await this.http.fetch("/mcp", { headers: { accept: "text/event-stream" }, signal }, 0);
          const reader = response.ok ? response.body?.getReader() : void 0;
          if (reader) {
            delay = 1e3;
            await this.sync().catch(() => void 0);
            const events = listChangedCounter();
            while (true) {
              const { done, value } = await reader.read();
              if (done) break;
              if (events.push(value) > 0) await this.sync().catch(() => void 0);
            }
          }
        } catch {
        }
        if (signal.aborted) break;
        await new Promise((resolve) => {
          const timer = setTimeout(resolve, delay);
          this.wakeUp = () => {
            clearTimeout(timer);
            delay = 1e3;
            resolve(void 0);
          };
          signal.addEventListener("abort", () => {
            clearTimeout(timer);
            resolve(void 0);
          }, { once: true });
        });
        this.wakeUp = void 0;
        delay = Math.min(delay * 2, 6e4);
      }
    })();
  }
  wake() {
    this.wakeUp?.();
  }
  /** 换了 dsh-ee 地址：断开旧的长连接和会话，连到新地址重新同步；已注册的工具留着，由同步增删。 */
  async restart() {
    this.controller?.abort();
    this.session = "";
    await this.start();
  }
  stop() {
    this.controller?.abort();
    for (const dispose of this.disposers.values()) dispose();
    this.disposers.clear();
  }
};

// src/approval.ts
function describeArgs(args, zh = true) {
  if (!args || typeof args !== "object") return "";
  const parts = [];
  for (const key of ["id", "slug", "kind", "name", "path", "username", "user_id", "department", "decision", "enabled", "reason"]) {
    const value = args[key];
    if (typeof value === "string" || typeof value === "number") {
      const text = String(value);
      parts.push(`${key}=${text.length > 60 ? `${text.slice(0, 60)}\u2026` : text}`);
    }
  }
  const title = args.spec?.title;
  if (typeof title === "string") parts.push(`title=${title.length > 60 ? `${title.slice(0, 60)}\u2026` : title}`);
  if (!parts.length) return "";
  return zh ? `\uFF08${parts.join("\uFF0C")}\uFF09` : ` (${parts.join(", ")})`;
}
function registerApproval(ctx, bridge) {
  ctx.on("tools/pre-execute", async (exec, next) => {
    const need = bridge.approval(String(exec.name), exec.arguments);
    if (!need) return next();
    const preview = need.preview ? `
${need.preview}` : "";
    return {
      kind: "ask",
      reason: `dsh-ee ${need.effect}: ${exec.name}`,
      displayReason: {
        en: `dsh-ee: ${need.en}${describeArgs(exec.arguments, false)}${preview}`,
        zh: `dsh-ee\uFF1A${need.zh}${describeArgs(exec.arguments)}${preview}`
      }
    };
  });
}

// src/doctor.ts
function compareVersions(a, b) {
  const parse = (v) => {
    const [core, pre] = v.replace(/^v/, "").split("-", 2);
    return { nums: core.split(".").map((n) => Number.parseInt(n, 10) || 0), pre };
  };
  const x = parse(a);
  const y = parse(b);
  for (let i = 0; i < Math.max(x.nums.length, y.nums.length); i++) {
    const d = (x.nums[i] ?? 0) - (y.nums[i] ?? 0);
    if (d) return d;
  }
  if (x.pre === y.pre) return 0;
  if (!x.pre) return 1;
  if (!y.pre) return -1;
  return x.pre < y.pre ? -1 : 1;
}
async function runLocalDoctor(ctx, config, tokens, http, bridge, bindings) {
  const checks = [];
  const add = (check) => checks.push(check);
  if (bindings?.warning()) add({ id: "app_bindings", status: "warn", title: bindings.warning(), fix: "\u91CD\u65B0\u6253\u5F00\u5FAE\u5E94\u7528\u4F1A\u5EFA\u7ACB\u65B0\u7ED1\u5B9A\uFF1B\u539F\u635F\u574F\u6587\u4EF6\u65E0\u9700\u624B\u52A8\u5220\u9664" });
  const current = await tokens.resolve();
  if (!current?.value) {
    add({ id: "credential", status: "fail", title: "\u672C\u673A\u6CA1\u6709\u4F01\u4E1A\u767B\u5F55\u51ED\u636E", fix: "\u5728\u5BF9\u8BDD\u6846\u8F93\u5165 /ee-login \u767B\u5F55" });
    return { summary: summarize(checks), checks };
  }
  add({ id: "credential", status: "ok", title: "\u672C\u673A\u6709\u4F01\u4E1A\u767B\u5F55\u51ED\u636E", detail: `\u6765\u6E90\uFF1A${current.source}` });
  let reachable = true;
  try {
    const response = await http.fetch("/api/dsh/whoami");
    if (response.ok) {
      const me = await response.json();
      const days = me.expires_at ? (new Date(me.expires_at).getTime() - Date.now()) / 864e5 : Infinity;
      add(days < 7 ? { id: "token", status: "warn", title: `\u767B\u5F55\u5C06\u5728 ${Math.max(0, Math.ceil(days))} \u5929\u5185\u5230\u671F`, detail: me.username, fix: "\u5230\u671F\u524D\u8F93\u5165 /ee-login \u91CD\u65B0\u767B\u5F55" } : { id: "token", status: "ok", title: `\u5DF2\u767B\u5F55 ${me.username}\uFF08${me.department}\uFF09` });
    } else {
      const body = await response.json().catch(() => null);
      add({ id: "token", status: "fail", title: "\u4F01\u4E1A\u767B\u5F55\u5DF2\u5931\u6548", detail: body?.code ?? `HTTP ${response.status}`, fix: "\u5728\u5BF9\u8BDD\u6846\u8F93\u5165 /ee-login \u91CD\u65B0\u767B\u5F55" });
    }
  } catch {
    reachable = false;
    add({ id: "token", status: "fail", title: `\u8FDE\u4E0D\u4E0A dsh-ee\uFF08${config.base}\uFF09`, fix: "\u68C0\u67E5\u7F51\u7EDC\u6216 VPN\uFF1B\u5730\u5740\u4E0D\u5BF9\u7684\u8BDD\u5728 DSH \u7684\u300C\u8BBE\u7F6E \u2192 dsh-ee\u300D\u91CC\u6539\u6210\u6D4F\u89C8\u5668\u80FD\u6253\u5F00\u7684 dsh-ee \u5730\u5740\u5E76\u6D4B\u8BD5" });
  }
  const npmrc = npmrcState(ctx, config, current.value);
  add(npmrc === "ok" ? { id: "npmrc", status: "ok", title: "DSH \u7684 .npmrc \u91CC\u4F01\u4E1A\u6E90\u914D\u7F6E\u4E0E\u5F53\u524D\u767B\u5F55\u4E00\u81F4", detail: npmrcPath(ctx) } : {
    id: "npmrc",
    status: "warn",
    title: npmrc === "missing" ? "DSH \u7684 .npmrc \u91CC\u7F3A\u5C11\u4F01\u4E1A npm \u6E90\u914D\u7F6E" : "DSH \u7684 .npmrc \u91CC\u4F01\u4E1A\u6E90\u4EE4\u724C\u4E0E\u5F53\u524D\u767B\u5F55\u4E0D\u4E00\u81F4",
    detail: `${npmrcPath(ctx)}\uFF1B\u88C5\u4F01\u4E1A\u63D2\u4EF6\u4F1A\u56E0 401 \u5931\u8D25`,
    fix: "\u91CD\u542F DSH \u6216\u91CD\u65B0\u767B\u5F55\u4F1A\u81EA\u52A8\u540C\u6B65"
  });
  if (!reachable) return { summary: summarize(checks), checks };
  let models = [];
  try {
    const response = await http.fetch("/v1/models");
    if (response.ok) models = (await response.json()).data.map((m) => m.id);
    add(models.length ? { id: "models", status: "ok", title: `\u53EF\u7528\u6A21\u578B ${models.join("\u3001")}` } : { id: "models", status: "fail", title: response.ok ? "\u6CA1\u6709\u53EF\u7528\u7684\u6A21\u578B" : `\u8BFB\u53D6\u6A21\u578B\u5217\u8868\u5931\u8D25\uFF08HTTP ${response.status}\uFF09`, fix: "\u8054\u7CFB\u7BA1\u7406\u5458\u5F00\u901A\u6A21\u578B\u6743\u9650" });
  } catch {
    add({ id: "models", status: "fail", title: "\u8BFB\u53D6\u6A21\u578B\u5217\u8868\u5931\u8D25" });
  }
  let server;
  try {
    const result = await bridge.callTool("ee_doctor", { action: "check" });
    if (!result?.isError) server = JSON.parse(result.content[0].text);
  } catch {
  }
  if (server) {
    for (const c of server.checks ?? []) add({ ...c, id: `server.${c.id}` });
    if (server.default_model && models.length && !models.includes(server.default_model)) {
      add({ id: "default_model", status: "warn", title: `\u4F01\u4E1A\u9ED8\u8BA4\u6A21\u578B ${server.default_model} \u4E0D\u5728\u4F60\u7684\u53EF\u7528\u5217\u8868\u91CC`, fix: "\u5728 DSH \u91CC\u6539\u9009\u53EF\u7528\u7684\u6A21\u578B" });
    }
    if (server.connect_version && config.version !== "dev") {
      add(compareVersions(server.connect_version, config.version) > 0 ? { id: "plugin_version", status: "warn", title: `\u63A5\u5165\u63D2\u4EF6\u6709\u65B0\u7248\u672C ${server.connect_version}\uFF08\u5F53\u524D ${config.version}\uFF09`, fix: "\u5728 DSH \u7684\u300C\u8BBE\u7F6E \u2192 dsh-ee\u300D\u70B9\u300C\u66F4\u65B0\u5230\u6700\u65B0\u7248\u300D\uFF0C\u88C5\u597D\u540E\u91CD\u542F DSH" } : { id: "plugin_version", status: "ok", title: `\u63A5\u5165\u63D2\u4EF6 ${config.version} \u5DF2\u662F\u6700\u65B0` });
    }
    const manager = ctx.get("pluginManager");
    if (manager && (server.required_plugins ?? []).length) {
      try {
        const have = new Set((await manager.listBundles()).filter((b) => b.installed || b.enabled).map((b) => b.name));
        const missing = server.required_plugins.map((p) => p.name).filter((n) => !have.has(n));
        add(missing.length ? { id: "required_plugins", status: "warn", title: `\u7F3A\u5C11\u5FC5\u88C5\u63D2\u4EF6 ${missing.join("\u3001")}`, fix: "\u5728 DSH \u63D2\u4EF6\u9875\u5B89\u88C5\uFF0C\u6216\u91CD\u542F DSH \u8BA9\u63A5\u5165\u63D2\u4EF6\u81EA\u52A8\u5B89\u88C5" } : { id: "required_plugins", status: "ok", title: "\u5FC5\u88C5\u63D2\u4EF6\u90FD\u5DF2\u5B89\u88C5" });
      } catch {
      }
    }
  }
  add(bridge.status.error ? { id: "bridge", status: "fail", title: "\u4F01\u4E1A\u5DE5\u5177\u540C\u6B65\u5931\u8D25", detail: bridge.status.error, fix: "\u68C0\u67E5\u7F51\u7EDC\uFF1B\u8F93\u5165 /ee-status \u770B\u767B\u5F55\u72B6\u6001" } : { id: "bridge", status: "ok", title: `\u4F01\u4E1A\u5DE5\u5177\u5DF2\u540C\u6B65\uFF08${bridge.status.tools} \u4E2A\uFF09` });
  return { summary: summarize(checks), checks };
}
function summarize(checks) {
  const fails = checks.filter((c) => c.status === "fail").length;
  const warns = checks.filter((c) => c.status === "warn").length;
  return fails ? `${fails} \u9879\u5931\u8D25\uFF0C${warns} \u9879\u9700\u8981\u6CE8\u610F` : warns ? `${warns} \u9879\u9700\u8981\u6CE8\u610F` : "\u5168\u90E8\u6B63\u5E38";
}
function registerDoctor(ctx, config, tokens, http, bridge, bindings) {
  const dispose = ctx.tools.register({
    name: "ee_local_doctor",
    description: "\u68C0\u67E5\u8FD9\u53F0\u7535\u8111\u4E0A DSH \u4E0E\u4F01\u4E1A\u5DE5\u4F5C\u53F0 dsh-ee \u7684\u63A5\u5165\uFF1A\u767B\u5F55\u51ED\u636E\u3001\u4EE4\u724C\u662F\u5426\u6709\u6548\u3001.npmrc\u3001\u63A5\u5165\u63D2\u4EF6\u7248\u672C\u3001\u5FC5\u88C5\u63D2\u4EF6\u3001\u6A21\u578B\u3001\u4F01\u4E1A\u5DE5\u5177\u540C\u6B65\uFF0C\u5E76\u9644\u5E26\u670D\u52A1\u7AEF\u68C0\u67E5\u3002\u53EA\u8BFB\uFF0C\u4E0D\u4FEE\u6539\u4EFB\u4F55\u4E1C\u897F\u3002",
    parameters: { type: "object", properties: {} },
    output: { schema: {}, render: (_args, value) => [{ type: "text", text: JSON.stringify(value) }] },
    execute: async () => runLocalDoctor(ctx, config, tokens, http, bridge, bindings)
  });
  ctx.effect(() => dispose);
}

// node_modules/@deepseek-ai/cosmokit/lib/index.js
function isNullable(value) {
  return value === null || value === void 0;
}
function defineProperty(object, key, value) {
  return Object.defineProperty(object, key, {
    writable: true,
    value,
    enumerable: false
  });
}
var write = Symbol.for("cosmokit.volatile.write");
function is(type, value) {
  if (arguments.length === 1) return (value2) => is(type, value2);
  return type in globalThis && value instanceof globalThis[type] || Object.prototype.toString.call(value).slice(8, -1) === type;
}
function isArrayBufferLike(value) {
  return is("ArrayBuffer", value) || is("SharedArrayBuffer", value);
}
function isArrayBufferSource(value) {
  return isArrayBufferLike(value) || ArrayBuffer.isView(value);
}
var Binary;
(function(Binary2) {
  Binary2.is = isArrayBufferLike;
  Binary2.isSource = isArrayBufferSource;
  function fromSource(source) {
    if (ArrayBuffer.isView(source)) return source.buffer.slice(source.byteOffset, source.byteOffset + source.byteLength);
    else return source;
  }
  Binary2.fromSource = fromSource;
  function toBase64(source) {
    source = fromSource(source);
    if (typeof Buffer !== "undefined") return Buffer.from(source).toString("base64");
    let binary = "";
    const bytes = new Uint8Array(source);
    for (let i = 0; i < bytes.byteLength; i++) binary += String.fromCharCode(bytes[i]);
    return btoa(binary);
  }
  Binary2.toBase64 = toBase64;
  function fromBase64(source) {
    if (typeof Buffer !== "undefined") return fromSource(Buffer.from(source, "base64"));
    return Uint8Array.from(atob(source), (c) => c.charCodeAt(0));
  }
  Binary2.fromBase64 = fromBase64;
  function toHex(source) {
    source = fromSource(source);
    if (typeof Buffer !== "undefined") return Buffer.from(source).toString("hex");
    return Array.from(new Uint8Array(source), (byte) => byte.toString(16).padStart(2, "0")).join("");
  }
  Binary2.toHex = toHex;
  function fromHex(source) {
    if (typeof Buffer !== "undefined") return fromSource(Buffer.from(source, "hex"));
    const hex = source.length % 2 === 0 ? source : source.slice(0, source.length - 1);
    const buffer = [];
    for (let i = 0; i < hex.length; i += 2) buffer.push(parseInt(`${hex[i]}${hex[i + 1]}`, 16));
    return Uint8Array.from(buffer).buffer;
  }
  Binary2.fromHex = fromHex;
})(Binary || (Binary = {}));
var base64ToArrayBuffer = Binary.fromBase64;
var arrayBufferToBase64 = Binary.toBase64;
var hexToArrayBuffer = Binary.fromHex;
var arrayBufferToHex = Binary.toHex;
function tokenize(source, delimiters, delimiter) {
  const output = [];
  let state = 0;
  for (let i = 0; i < source.length; i++) {
    const code = source.charCodeAt(i);
    if (code >= 65 && code <= 90) {
      if (state === 1) {
        const next = source.charCodeAt(i + 1);
        if (next >= 97 && next <= 122) output.push(delimiter);
        output.push(code + 32);
      } else {
        if (state !== 0) output.push(delimiter);
        output.push(code + 32);
      }
      state = 1;
    } else if (code >= 97 && code <= 122) {
      output.push(code);
      state = 2;
    } else if (delimiters.includes(code)) {
      if (state !== 0) output.push(delimiter);
      state = 0;
    } else output.push(code);
  }
  return String.fromCharCode(...output);
}
function paramCase(source) {
  return tokenize(source, [45, 95], 45);
}
var hyphenate = paramCase;
var Time;
(function(Time2) {
  Time2.millisecond = 1;
  Time2.second = 1e3;
  Time2.minute = Time2.second * 60;
  Time2.hour = Time2.minute * 60;
  Time2.day = Time2.hour * 24;
  Time2.week = Time2.day * 7;
  let timezoneOffset = (/* @__PURE__ */ new Date()).getTimezoneOffset();
  function setTimezoneOffset(offset) {
    timezoneOffset = offset;
  }
  Time2.setTimezoneOffset = setTimezoneOffset;
  function getTimezoneOffset() {
    return timezoneOffset;
  }
  Time2.getTimezoneOffset = getTimezoneOffset;
  function getDateNumber(date = /* @__PURE__ */ new Date(), offset) {
    if (typeof date === "number") date = new Date(date);
    if (offset === void 0) offset = timezoneOffset;
    return Math.floor((date.valueOf() / Time2.minute - offset) / 1440);
  }
  Time2.getDateNumber = getDateNumber;
  function fromDateNumber(value, offset) {
    const date = new Date(value * Time2.day);
    if (offset === void 0) offset = timezoneOffset;
    return new Date(+date + offset * Time2.minute);
  }
  Time2.fromDateNumber = fromDateNumber;
  const numeric = /\d+(?:\.\d+)?/.source;
  const timeRegExp = new RegExp(`^${[
    "w(?:eek(?:s)?)?",
    "d(?:ay(?:s)?)?",
    "h(?:our(?:s)?)?",
    "m(?:in(?:ute)?(?:s)?)?",
    "s(?:ec(?:ond)?(?:s)?)?"
  ].map((unit) => `(${numeric}${unit})?`).join("")}$`);
  function parseTime(source) {
    const capture = timeRegExp.exec(source);
    if (!capture) return 0;
    return (parseFloat(capture[1]) * Time2.week || 0) + (parseFloat(capture[2]) * Time2.day || 0) + (parseFloat(capture[3]) * Time2.hour || 0) + (parseFloat(capture[4]) * Time2.minute || 0) + (parseFloat(capture[5]) * Time2.second || 0);
  }
  Time2.parseTime = parseTime;
  function parseDate(date) {
    const parsed = parseTime(date);
    if (parsed) date = Date.now() + parsed;
    else if (/^\d{1,2}(:\d{1,2}){1,2}$/.test(date)) date = `${(/* @__PURE__ */ new Date()).toLocaleDateString()}-${date}`;
    else if (/^\d{1,2}-\d{1,2}-\d{1,2}(:\d{1,2}){1,2}$/.test(date)) date = `${(/* @__PURE__ */ new Date()).getFullYear()}-${date}`;
    return date ? new Date(date) : /* @__PURE__ */ new Date();
  }
  Time2.parseDate = parseDate;
  function format(ms) {
    const abs = Math.abs(ms);
    if (abs >= Time2.day - Time2.hour / 2) return Math.round(ms / Time2.day) + "d";
    else if (abs >= Time2.hour - Time2.minute / 2) return Math.round(ms / Time2.hour) + "h";
    else if (abs >= Time2.minute - Time2.second / 2) return Math.round(ms / Time2.minute) + "m";
    else if (abs >= Time2.second) return Math.round(ms / Time2.second) + "s";
    return ms + "ms";
  }
  Time2.format = format;
  function toDigits(source, length = 2) {
    return source.toString().padStart(length, "0");
  }
  Time2.toDigits = toDigits;
  function template(template2, time = /* @__PURE__ */ new Date()) {
    return template2.replace("yyyy", time.getFullYear().toString()).replace("yy", time.getFullYear().toString().slice(2)).replace("MM", toDigits(time.getMonth() + 1)).replace("dd", toDigits(time.getDate())).replace("hh", toDigits(time.getHours())).replace("mm", toDigits(time.getMinutes())).replace("ss", toDigits(time.getSeconds())).replace("SSS", toDigits(time.getMilliseconds(), 3));
  }
  Time2.template = template;
})(Time || (Time = {}));

// node_modules/@deepseek-ai/cordis/lib/index.js
var DisposableList = class {
  sn = 0;
  map = /* @__PURE__ */ new Map();
  weak = /* @__PURE__ */ new WeakMap();
  get length() {
    return this.map.size;
  }
  push(value) {
    const sn = ++this.sn;
    this.map.set(sn, value);
    this.weak.set(value, sn);
    return () => this.map.delete(sn);
  }
  delete(value) {
    const sn = this.weak.get(value);
    if (!sn) return false;
    return this.map.delete(sn);
  }
  clear() {
    const values = [...this.map.values()];
    this.map.clear();
    return values.reverse();
  }
  [Symbol.iterator]() {
    return this.map.values();
  }
  [Symbol.for("nodejs.util.inspect.custom")]() {
    return [...this];
  }
};
var symbols = {
  shadow: Symbol.for("cordis.shadow"),
  receiver: Symbol.for("cordis.receiver"),
  original: Symbol.for("cordis.original"),
  metadata: Symbol.for("cordis.metadata"),
  initHooks: Symbol.for("cordis.initHooks"),
  checkProto: Symbol.for("cordis.checkProto"),
  effect: Symbol.for("cordis.effect"),
  filter: Symbol.for("cordis.filter"),
  isolate: Symbol.for("cordis.isolate"),
  intercept: Symbol.for("cordis.intercept"),
  init: Symbol.for("cordis.init"),
  check: Symbol.for("cordis.check"),
  config: Symbol.for("cordis.config"),
  invoke: Symbol.for("cordis.invoke"),
  extend: Symbol.for("cordis.extend"),
  tracker: Symbol.for("cordis.tracker"),
  resolveConfig: Symbol.for("cordis.resolveConfig")
};
var GeneratorFunction = function* () {
}.constructor;
var AsyncGeneratorFunction = async function* () {
}.constructor;
function isConstructor(func) {
  if (!func.prototype) return false;
  if (func instanceof GeneratorFunction) return false;
  if (AsyncGeneratorFunction !== Function && func instanceof AsyncGeneratorFunction) return false;
  return true;
}
function joinPrototype(proto1, proto2) {
  if (proto1 === Object.prototype) return proto2;
  const result = Object.create(joinPrototype(Object.getPrototypeOf(proto1), proto2));
  for (const key of Reflect.ownKeys(proto1)) Object.defineProperty(result, key, Object.getOwnPropertyDescriptor(proto1, key));
  return result;
}
function isObject(value) {
  return value && (typeof value === "object" || typeof value === "function");
}
function getPropertyDescriptor(target, prop) {
  let proto = target;
  while (proto) {
    const desc = Reflect.getOwnPropertyDescriptor(proto, prop);
    if (desc) return desc;
    proto = Object.getPrototypeOf(proto);
  }
}
function getTraceable(ctx, value) {
  if (!isObject(value)) return value;
  if (Object.hasOwn(value, symbols.shadow)) return Object.getPrototypeOf(value);
  const tracker = value[symbols.tracker];
  if (!tracker) return value;
  return createTraceable(ctx, value, tracker);
}
function withProps(target, props) {
  if (!props) return target;
  return new Proxy(target, {
    get: (target2, prop, receiver) => {
      if (prop in props && prop !== "constructor") return Reflect.get(props, prop, receiver);
      return Reflect.get(target2, prop, receiver);
    },
    set: (target2, prop, value, receiver) => {
      if (prop in props && prop !== "constructor") return Reflect.set(props, prop, value, receiver);
      return Reflect.set(target2, prop, value, receiver);
    }
  });
}
function withProp(target, prop, value) {
  return withProps(target, Object.defineProperty(/* @__PURE__ */ Object.create(null), prop, {
    value,
    writable: false
  }));
}
function createShadow(ctx, target, property, receiver) {
  if (!property) return receiver;
  const origin = Reflect.getOwnPropertyDescriptor(target, property)?.value;
  if (!origin) return receiver;
  return withProp(receiver, property, ctx.extend({ [symbols.shadow]: origin }));
}
function createShadowMethod(ctx, value, outer, shadow) {
  return new Proxy(value, { apply: (target, thisArg, args) => {
    if (thisArg === outer) thisArg = shadow;
    return getTraceable(ctx, Reflect.apply(target, thisArg, args));
  } });
}
function createTraceable(ctx, value, tracker) {
  if (ctx[symbols.shadow] && !tracker.noShadow) ctx = Object.getPrototypeOf(ctx);
  const proxy = new Proxy(value, {
    get: (target, prop, receiver) => {
      if (prop === symbols.original) return target;
      if (prop === tracker.property) return ctx;
      if (typeof prop === "symbol") return Reflect.get(target, prop, receiver);
      if (tracker.associate && ctx.reflect.props[`${tracker.associate}.${prop}`]) return Reflect.get(ctx, `${tracker.associate}.${prop}`, withProp(ctx, symbols.receiver, receiver));
      let shadow, innerValue;
      const desc = getPropertyDescriptor(target, prop);
      if (desc && "value" in desc) innerValue = desc.value;
      else {
        shadow = createShadow(ctx, target, tracker.property, receiver);
        innerValue = Reflect.get(target, prop, shadow);
      }
      const innerTracker = innerValue?.[symbols.tracker];
      if (innerTracker) return createTraceable(ctx, innerValue, innerTracker);
      else if (!tracker.noShadow && typeof innerValue === "function") {
        shadow ??= createShadow(ctx, target, tracker.property, receiver);
        return createShadowMethod(ctx, innerValue, receiver, shadow);
      } else return innerValue;
    },
    set: (target, prop, value2, receiver) => {
      if (prop === symbols.original) return false;
      if (prop === tracker.property) return false;
      if (typeof prop === "symbol") return Reflect.set(target, prop, value2, receiver);
      if (tracker.associate && ctx.reflect.props[`${tracker.associate}.${prop}`]) return Reflect.set(ctx, `${tracker.associate}.${prop}`, value2, withProp(ctx, symbols.receiver, receiver));
      const shadow = createShadow(ctx, target, tracker.property, receiver);
      return Reflect.set(target, prop, value2, shadow);
    },
    apply: (target, thisArg, args) => {
      return applyTraceable(proxy, target, thisArg, args);
    }
  });
  return proxy;
}
function applyTraceable(proxy, value, thisArg, args) {
  if (!value[symbols.invoke]) return Reflect.apply(value, thisArg, args);
  return value[symbols.invoke].apply(proxy, args);
}
function createCallable(name2, proto, tracker) {
  const self = function(...args) {
    return applyTraceable(createTraceable(self["ctx"], self, tracker), self, this, args);
  };
  defineProperty(self, "name", name2);
  return Object.setPrototypeOf(self, proto);
}
function handleError(info, reason3, getOuterStack) {
  const innerLines = info.error.stack.split("\n");
  if (typeof reason3?.stack !== "string") {
    const outerError = new Error(reason3);
    const lines2 = outerError.stack.split("\n");
    lines2.splice(1, Infinity, ...getOuterStack());
    outerError.stack = lines2.join("\n");
    throw outerError;
  }
  const lines = reason3.stack.split("\n");
  let index = lines.indexOf(innerLines[2]);
  if (index === -1) throw reason3;
  index -= info.offset;
  while (index > 0) {
    if (!lines[index - 1].endsWith(" (<anonymous>)")) break;
    index -= 1;
  }
  lines.splice(index, Infinity, ...getOuterStack());
  reason3.stack = lines.join("\n");
  throw reason3;
}
function composeError(callback, getOuterStack = buildOuterStack()) {
  const info = {
    offset: 1,
    error: /* @__PURE__ */ new Error()
  };
  try {
    const result = callback(info);
    if (isObject(result) && "then" in result) return result.then(void 0, (reason3) => handleError(info, reason3, getOuterStack));
    else return result;
  } catch (reason3) {
    handleError(info, reason3, getOuterStack);
  }
}
function buildOuterStack(offset = 0) {
  const outerError = /* @__PURE__ */ new Error();
  return () => outerError.stack.split("\n").slice(3 + offset);
}
function isBailed(value) {
  return value !== null && value !== false && value !== void 0;
}
var EventsService = class {
  ctx;
  _hooks = {};
  constructor(ctx) {
    this.ctx = ctx;
    defineProperty(this, symbols.tracker, {
      property: "ctx",
      noShadow: true
    });
    this.on("internal/listener", function(name2, listener, options) {
      if (name2 === "internal/update" && !options.global) return (this.fiber._hooks["internal/update"] ??= new DisposableList())[options.prepend ? "unshift" : "push"](listener);
    });
    this.on("internal/update", function(config, noSave, next) {
      const cbs = [...this._hooks["internal/update"] || []];
      const _next = () => {
        return (cbs.shift() ?? next).call(this, config, noSave, _next);
      };
      return _next();
    }, {
      global: true,
      prepend: true
    });
  }
  /**
  * Resolve listeners for one dispatch and apply context filtering.
  *
  * @param type — the dispatch mode, reported on `internal/dispatch`.
  * @param args — the raw dispatch arguments; consumed up to the event name.
  * @returns the matching listener callbacks, bound to the dispatch `this`.
  */
  dispatch(type, args) {
    const thisArg = typeof args[0] === "object" || typeof args[0] === "function" ? args.shift() : null;
    const name2 = args.shift();
    if (!name2.startsWith("internal/")) this.emit("internal/dispatch", type, name2, args, thisArg);
    const filter = thisArg?.[Context.filter];
    return (this._hooks[name2] || []).filter((hook) => hook.global || !filter || filter.call(thisArg, hook.ctx)).map((hook) => hook.callback.bind(thisArg));
  }
  /**
  * Run listeners concurrently and wait for all of them.
  *
  * @param args — optional `this`, the event name, then listener arguments.
  * @returns a promise resolving once every listener has settled.
  */
  async parallel(...args) {
    const errors = (await Promise.allSettled(this.dispatch("emit", args).map(async (cb) => cb(...args)))).filter((result) => result.status === "rejected");
    if (errors.length) throw new AggregateError(errors.map((error) => error.reason));
  }
  /**
  * Run listeners synchronously without waiting for returned promises.
  *
  * @param args — optional `this`, the event name, then listener arguments.
  */
  emit(...args) {
    this.dispatch("emit", args).map((cb) => cb(...args));
  }
  /**
  * Run listeners in order, awaiting each, until one returns a bail value.
  *
  * @param args — optional `this`, the event name, then listener arguments.
  * @returns the first bail value (see {@link isBailed}), if any.
  */
  async serial(...args) {
    for (const cb of this.dispatch("serial", args)) {
      const result = await cb(...args);
      if (isBailed(result)) return result;
    }
  }
  /**
  * Run listeners synchronously until one returns a bail value.
  *
  * @param args — optional `this`, the event name, then listener arguments.
  * @returns the first bail value (see {@link isBailed}), if any.
  */
  bail(...args) {
    for (const cb of this.dispatch("bail", args)) {
      const result = cb(...args);
      if (isBailed(result)) return result;
    }
  }
  /**
  * Compose listeners around the final `next` callback.
  *
  * The last dispatch argument is treated as the innermost `next`. Listeners
  * run outermost-first; a listener that does not call `next()` vetoes the
  * rest of the chain, including the built-in behavior.
  *
  * @param args — optional `this`, the event name, listener arguments, then `next`.
  * @returns the outermost listener's return value.
  */
  waterfall(...args) {
    const cbs = this.dispatch("waterfall", args);
    const inner = args.pop();
    const next = () => {
      return (cbs.shift() ?? inner)(...args);
    };
    args.push(next);
    return next();
  }
  /**
  * Store a listener record as an effect on the current fiber.
  *
  * @param label — effect label shown in fiber diagnostics.
  * @param hooks — the listener list for one event.
  * @param callback — the listener to store.
  * @param options — placement and filtering options.
  * @returns a disposer that unregisters the listener.
  */
  register(label, hooks, callback, options) {
    const method = options.prepend ? "unshift" : "push";
    return this.ctx.fiber.effect(() => {
      hooks[method]({
        ctx: this.ctx,
        callback,
        ...options
      });
      return () => this.unregister(hooks, callback);
    }, label);
  }
  /**
  * Remove a stored listener record.
  *
  * @param hooks — the listener list for one event.
  * @param callback — the listener to remove.
  * @returns `true` if the listener was found and removed.
  */
  unregister(hooks, callback) {
    const index = hooks.findIndex((hook) => hook.callback === callback);
    if (index >= 0) {
      hooks.splice(index, 1);
      return true;
    }
  }
  /**
  * Register an event listener owned by the current fiber.
  *
  * The listener is removed automatically when the fiber unloads. Throws
  * `CordisError('INACTIVE_EFFECT')` if the fiber is already disposed.
  *
  * @param name — the event name to listen for.
  * @param listener — called with the dispatch arguments.
  * @param options — listener options; a boolean is shorthand for `prepend`.
  * @returns a disposer removing the listener; `true` if it was still registered.
  */
  on(name2, listener, options) {
    if (typeof options !== "object") options = { prepend: options };
    this.ctx.fiber.assertActive();
    listener = this.ctx.reflect.bind(listener);
    const result = this.bail(this.ctx, "internal/listener", name2, listener, options);
    if (result) return result;
    const hooks = this._hooks[name2] ||= [];
    const label = `ctx.on(${typeof name2 === "string" ? JSON.stringify(name2) : name2.toString()})`;
    return this.register(label, hooks, listener, options);
  }
  /**
  * Register an event listener that disposes itself after the first call.
  *
  * @param name — the event name to listen for.
  * @param listener — called at most once with the dispatch arguments.
  * @param options — listener options; a boolean is shorthand for `prepend`.
  * @returns a disposer removing the listener; `true` if it was still registered.
  */
  once(name2, listener, options) {
    const dispose = this.on(name2, function(...args) {
      dispose();
      return listener.apply(this, args);
    }, options);
    return dispose;
  }
};
var defaultFormatters = {
  s: (value) => String(value),
  d: (value) => Math.trunc(Number(value)),
  i: (value) => Math.trunc(Number(value)),
  f: (value) => Number(value),
  o: (value) => JSON.stringify(value),
  O: (value) => JSON.stringify(value),
  c: () => "",
  C: (value, exporter, message) => {
    return Logger.color(exporter, Logger.code(message.name, exporter.colors), value);
  }
};
function isAggregateError(error) {
  return error instanceof Error && Array.isArray(error["errors"]);
}
var Logger = class {
  service;
  static color(exporter, code, value, decoration = "") {
    if (!exporter.colors) return "" + value;
    return `\x1B[3${code < 8 ? code : "8;5;" + code}${exporter.colors >= 2 ? decoration : ""}m${value}\x1B[0m`;
  }
  static code(name2, level) {
    let hash = 0;
    for (let i = 0; i < name2.length; i++) {
      hash = (hash << 3) - hash + name2.charCodeAt(i) + 13;
      hash |= 0;
    }
    const colors = !level ? [] : level >= 2 ? c256 : c16;
    return colors[Math.abs(hash) % colors.length];
  }
  static format(exporter, message) {
    const args = message.args.slice();
    if (args[0] instanceof Error) {
      args[0] = args[0].stack || args[0].message;
      args.unshift("%s");
    } else if (typeof args[0] !== "string") args.unshift("%o");
    let format = args.shift();
    format = format.replace(/%([a-zA-Z%])/g, (match, char) => {
      if (match === "%%") return "%";
      const formatter = exporter.formatters?.[char] ?? defaultFormatters[char];
      if (typeof formatter === "function") return formatter(args.shift(), exporter, message);
      return match;
    });
    const oFormatter = exporter.formatters?.o ?? defaultFormatters.o;
    for (let arg of args) {
      if (typeof arg === "object" && arg) arg = oFormatter(arg, exporter, message);
      format += " " + arg;
    }
    const { maxLength = 10240 } = exporter;
    return format.split(/\r?\n/g).map((line) => {
      return line.slice(0, maxLength) + (line.length > maxLength ? "..." : "");
    }).join("\n");
  }
  constructor(options, service) {
    this.service = service;
    Object.assign(this, options);
    this.error = this._method("error", 0);
    this.info = this._method("info", 1);
    this.warn = this._method("warn", 2);
    this.debug = this._method("debug", 3);
  }
  _method(type, level) {
    return (...args) => {
      if (args.length === 1 && args[0] instanceof Error) {
        if (args[0].cause) this[type](args[0].cause);
        else if (isAggregateError(args[0])) {
          args[0].errors.forEach((error) => this[type](error));
          return;
        }
      }
      const sn = ++this.service._snMessage;
      const ts = Date.now();
      for (const exporter of this.service.exporters.values()) {
        if ((exporter.levels?.[this.name] ?? exporter.levels?.default ?? this.level ?? 1) < level) continue;
        const message = {
          sn,
          ts,
          type,
          level,
          name: this.name,
          ...this.meta,
          args
        };
        exporter.export(message);
      }
    };
  }
};
var c16 = [
  6,
  2,
  3,
  4,
  5,
  1
];
var c256 = [
  20,
  21,
  26,
  27,
  32,
  33,
  38,
  39,
  40,
  41,
  42,
  43,
  44,
  45,
  56,
  57,
  62,
  63,
  68,
  69,
  74,
  75,
  76,
  77,
  78,
  79,
  80,
  81,
  92,
  93,
  98,
  99,
  112,
  113,
  129,
  134,
  135,
  148,
  149,
  160,
  161,
  162,
  163,
  164,
  165,
  166,
  167,
  168,
  169,
  170,
  171,
  172,
  173,
  178,
  179,
  184,
  185,
  196,
  197,
  198,
  199,
  200,
  201,
  202,
  203,
  204,
  205,
  206,
  207,
  208,
  209,
  214,
  215,
  220,
  221
];
var LoggerService = class LoggerService2 {
  bufferSize = 1e3;
  buffer = [];
  ctx;
  _snMessage = 0;
  _snExporter = 0;
  exporters = /* @__PURE__ */ new Map();
  constructor(ctx) {
    const tracker = {
      property: "ctx",
      noShadow: true
    };
    const self = createCallable("logger", joinPrototype(Object.getPrototypeOf(this), Function.prototype), tracker);
    Object.assign(self, this);
    self.ctx = ctx;
    defineProperty(self, symbols.tracker, tracker);
    self.exporter({
      colors: 3,
      export: (message) => {
        self.buffer.push(message);
        if (self.buffer.length > self.bufferSize) self.buffer = self.buffer.slice(-self.bufferSize);
      }
    });
    return self;
  }
  /**
  * Register an exporter and dispose it with the current fiber.
  *
  * @param exporter — the sink that receives structured log messages.
  * @returns a disposer that removes the exporter.
  */
  exporter(exporter) {
    return this.ctx.effect(() => {
      const id = ++this._snExporter;
      this.exporters.set(id, exporter);
      return () => this.exporters.delete(id);
    }, "ctx.logger.exporter()");
  }
  _resolveConfig() {
    let intercept = this.ctx[symbols.intercept];
    const configs = [];
    while ("logger" in intercept) {
      if (Object.hasOwn(intercept, "logger")) configs.unshift(intercept["logger"]);
      intercept = Object.getPrototypeOf(intercept);
    }
    return Object.assign({}, ...configs);
  }
  [symbols.invoke](name2) {
    const config = this._resolveConfig();
    const fiber = (this.ctx[symbols.shadow] ?? this.ctx).fiber;
    name2 ??= config.name;
    name2 ??= hyphenate(fiber.name);
    return new Logger({
      name: name2,
      level: config.level,
      meta: { fiber: new WeakRef(fiber) }
    }, this);
  }
  static {
    for (const type of [
      "error",
      "info",
      "warn",
      "debug"
    ]) LoggerService2.prototype[type] = function(...args) {
      return this()[type](...args);
    };
  }
};
function enhanceError(error) {
  const lines = error.stack.split("\n");
  lines.splice(0, 2, `Error: ${error.message}`);
  error.stack = lines.join("\n");
  return error;
}
var RESERVED_WORDS = ["prototype", "then"];
function isSpecialProperty(prop) {
  return typeof prop === "symbol" || RESERVED_WORDS.includes(prop) || parseInt(prop).toString() === prop || prop.startsWith("_");
}
var ReflectService = class {
  ctx;
  /** Proxy traps implementing service resolution for every context object. */
  static handler = {
    get: (target, prop, ctx) => {
      if (isSpecialProperty(prop)) return Reflect.get(target, prop, ctx);
      if (Reflect.has(target, prop)) return getTraceable(ctx, Reflect.get(target, prop, ctx));
      const error = /* @__PURE__ */ new Error(`cannot get property "${prop}" without inject`);
      try {
        const def = target.reflect.props[prop];
        if (def?.type === "accessor") return def.get.call(ctx, ctx[symbols.receiver], error);
        if (!ctx.fiber.runtime) return ctx.reflect.get(prop, false);
        return ctx.events.waterfall("internal/get", ctx, prop, error, () => {
          const key = target[symbols.isolate][prop];
          let fiber = (ctx[symbols.shadow] ?? ctx).fiber;
          while (true) {
            const impl = fiber.store?.[prop];
            if (impl) return getTraceable(ctx, impl.value);
            if (prop in fiber.inject) {
              error.message = `cannot get required service "${prop}" in inactive context`;
              throw error;
            }
            if (!fiber.runtime) throw error;
            if (fiber.parent[symbols.isolate][prop] !== key) throw error;
            fiber = fiber.parent.fiber;
          }
        });
      } catch (e) {
        throw e === error ? enhanceError(e) : e;
      }
    },
    set: (target, prop, value, ctx) => {
      if (isSpecialProperty(prop)) return Reflect.set(target, prop, value, ctx);
      const error = /* @__PURE__ */ new Error(`cannot set property "${prop}" without provide`);
      const def = target.reflect.props[prop];
      if (!def) {
        if (!ctx.fiber.runtime) return Reflect.set(target, prop, value, ctx);
        throw enhanceError(error);
      }
      try {
        if (def.type === "accessor") {
          if (!def.set) return false;
          return def.set.call(ctx, value, ctx[symbols.receiver], error);
        }
        return ctx.events.waterfall("internal/set", ctx, prop, value, error, () => {
          return ctx.reflect.set(prop, value, error);
        });
      } catch (e) {
        throw e === error ? enhanceError(e) : e;
      }
    },
    has: (target, prop) => {
      if (isSpecialProperty(prop)) return Reflect.has(target, prop);
      if (Reflect.has(target, prop)) return true;
      return !!target.reflect.props[prop];
    }
  };
  /** Service implementations, keyed by isolation label. */
  store = /* @__PURE__ */ Object.create(null);
  /** Declared context properties (services and accessors), by name. */
  props = /* @__PURE__ */ Object.create(null);
  constructor(ctx) {
    this.ctx = ctx;
    defineProperty(this, symbols.tracker, {
      property: "ctx",
      noShadow: true
    });
    this.mixin("reflect", [
      "get",
      "set",
      "provide",
      "accessor",
      "mixin"
    ]);
    this.mixin("fiber", ["runtime", "effect"]);
    this.mixin("registry", ["inject", "plugin"]);
    this.mixin("events", [
      "on",
      "once",
      "parallel",
      "emit",
      "serial",
      "bail",
      "waterfall"
    ]);
  }
  /**
  * Read a service from the store without the inject requirement.
  *
  * @param name — the service name.
  * @param strict — when `true`, only return implementations whose providing
  * fiber is currently active.
  * @returns the service value, or `undefined` when not (yet) provided.
  */
  get(name2, strict = true) {
    return getTraceable(this.ctx, this._getImpl(name2, strict)?.value);
  }
  _getImpl(name2, strict = true) {
    const key = this.ctx[symbols.isolate][name2];
    const impl = key && this.store[key];
    if (!impl) return;
    if (strict && impl.fiber.state !== 2) return;
    return impl;
  }
  /**
  * Overwrite a provided service's value.
  *
  * @param name — the service name.
  * @param value — the new service value.
  * @param error — carrier for the caller stack in diagnostics.
  * @returns `true` on success.
  * @throws when `name` was never provided, or was provided by another fiber.
  */
  set(name2, value, error) {
    const key = this.ctx[symbols.isolate][name2];
    const impl = this.store[key];
    if (!impl) throw new Error(`cannot set property "${name2}" without provide`);
    if (impl.fiber !== this.ctx.fiber) throw new Error(`cannot set property "${name2}" in multiple fibers`);
    impl.value = value;
    return true;
  }
  /**
  * Register a service implementation owned by the current fiber.
  *
  * See the `ctx.provide()` overload above for the full contract.
  *
  * @param name — the service name.
  * @param value — the service value.
  * @param check — optional availability predicate for dependents.
  * @returns a disposer that unregisters the service.
  */
  provide(name2, value, check) {
    return this.ctx.fiber.effect(() => {
      if (!this.props[name2]) this.props[name2] ??= { type: "service" };
      else if (this.props[name2].type !== "service") throw new Error(`property "${name2}" is already declared as ${this.props[name2].type}`);
      this.props[name2] = { type: "service" };
      this.ctx.root[symbols.isolate][name2] ??= Symbol(name2);
      const key = this.ctx[symbols.isolate][name2];
      const impl = {
        name: name2,
        value,
        fiber: this.ctx.fiber,
        check
      };
      if (this.store[key]) throw new Error(`service "${name2}" has been registered at <${this.store[key].fiber.name}>`);
      this.store[key] = impl;
      this.ctx.fiber.store[name2] = impl;
      if (this.ctx.fiber.state === 2) this.notify([name2]);
      return async () => {
        delete this.store[key];
        const fibers = this.notify([name2]);
        await Promise.allSettled(fibers.map((fiber) => fiber.await()));
        delete this.ctx.fiber.store[name2];
      };
    }, `ctx.provide(${JSON.stringify(name2)})`);
  }
  /**
  * Re-evaluate every fiber that requires one of the given services.
  *
  * @param names — the service names that changed.
  * @param filter — restricts notification to matching isolation scopes.
  * @returns the fibers whose dependency state was refreshed.
  */
  notify(names, filter = (ctx, name2) => ctx[symbols.isolate][name2] === this.ctx[symbols.isolate][name2]) {
    const fibers = [];
    for (const runtime of this.ctx.registry.values()) for (const fiber of runtime.fibers) {
      let hasUpdate = false;
      for (const name2 of names) {
        if (!(name2 in fiber.inject)) continue;
        if (!filter(fiber.ctx, name2)) continue;
        hasUpdate = true;
        fiber._checkImpl(name2);
      }
      if (!hasUpdate) continue;
      fiber._refresh();
      fibers.push(fiber);
    }
    for (const name2 of names) {
      const self = Object.create(this.ctx);
      self[symbols.filter] = (target) => filter(target, name2);
      this.ctx.events.emit(self, "internal/service", name2, this._getImpl(name2, false)?.value);
    }
    return fibers;
  }
  /**
  * Define a computed context property backed by get/set hooks.
  *
  * @param name — the context property name.
  * @param options — the `get` hook and optional `set` hook.
  * @returns a disposer that removes the accessor.
  */
  accessor(name2, options) {
    return this.ctx.fiber.effect(() => {
      if (name2 in this.props) throw new Error(`property "${name2}" is already declared as ${this.props[name2].type}`);
      this.props[name2] = {
        type: "accessor",
        ...options
      };
      return () => delete this.props[name2];
    }, `ctx.accessor(${JSON.stringify(name2)})`);
  }
  /**
  * Expose selected members of a service directly on `ctx`.
  *
  * See the `ctx.mixin()` overload above for the full contract.
  *
  * @param source — a context property name or a source object.
  * @param mixins — keys to forward, or a source-key → ctx-key map.
  * @returns a disposer that removes all created accessors.
  */
  mixin(source, mixins) {
    const self = this;
    return this.ctx.fiber.effect(function* () {
      const entries = Array.isArray(mixins) ? mixins.map((key) => [key, key]) : Object.entries(mixins);
      const getTarget = (ctx, error) => {
        return ctx[source];
      };
      for (const [key, value] of entries) yield self.accessor(value, {
        get(receiver, error) {
          const service = getTarget(this, error);
          if (isNullable(service)) return service;
          const mixin = receiver ? withProps(receiver, service) : service;
          const value2 = Reflect.get(service, key, mixin);
          if (typeof value2 !== "function") return value2;
          return value2.bind(mixin ?? service);
        },
        set(value2, receiver, error) {
          const service = getTarget(this, error);
          const mixin = receiver ? withProps(receiver, service) : service;
          return Reflect.set(service, key, value2, mixin);
        }
      });
    }, `ctx.mixin(${JSON.stringify(source)})`);
  }
  /**
  * Attach this context's tracing wrapper to a value.
  *
  * @param value — the value to wrap.
  * @returns the traceable wrapper (or the value itself when not applicable).
  */
  trace(value) {
    return getTraceable(this.ctx, value);
  }
  /**
  * Wrap a callback so calls trace `this` and arguments to this context.
  *
  * @param callback — the function to wrap.
  * @returns a proxy delegating to `callback` with traced values.
  */
  bind(callback) {
    return new Proxy(callback, {
      apply: (target, thisArg, args) => {
        return Reflect.apply(target, this.trace(thisArg), args.map((arg) => this.trace(arg)));
      },
      construct: (target, args, newTarget) => {
        return Reflect.construct(target, args.map((arg) => this.trace(arg)), newTarget);
      }
    });
  }
};
var kValidationError = Symbol.for("ValidationError");
var ValidationError = class extends TypeError {
  name = "ValidationError";
  /**
  * Build the aggregated message from schema issues.
  *
  * @param issues — the standard-schema issues, one message line each.
  */
  constructor(issues) {
    super(`invalid config:
` + issues.map((issue) => {
      if (issue.path) return `  - ${issue.message} (at ${issue.path.join(".")})`;
      else return `  - ${issue.message}`;
    }).join("\n"));
  }
};
Object.defineProperty(ValidationError.prototype, kValidationError, { value: true });
function resolveConfig(runtime, config) {
  if (!runtime.Config) return config;
  const result = runtime.Config["~standard"].validate(config);
  if ("then" in result) throw new TypeError("Async config validation is not supported");
  if (result.issues) throw new ValidationError(result.issues);
  else return result.value;
}
var effectInertia = /* @__PURE__ */ new WeakMap();
function runDisposable(dispose) {
  const result = dispose();
  return effectInertia.get(dispose)?.() ?? result;
}
function emitPluginDisposed(context, fiber) {
  const args = ["internal/plugin", fiber];
  let callbacks;
  try {
    callbacks = context.events.dispatch("emit", args);
  } catch (error) {
    context.logger.error(error);
    return;
  }
  for (const callback of callbacks) try {
    const returned = callback(...args);
    Promise.resolve(returned).catch((error) => context.logger.error(error));
  } catch (error) {
    context.logger.error(error);
  }
}
var CordisError = class CordisError2 extends Error {
  code;
  /**
  * @param code — the stable error code; also the default message.
  * @param message — optional human-readable override.
  */
  constructor(code, message) {
    super(message ?? CordisError2.Code[code]);
    this.code = code;
  }
};
(function(CordisError3) {
  CordisError3.Code = { INACTIVE_EFFECT: "cannot create effect on inactive context" };
})(CordisError || (CordisError = {}));
var INACTIVE = "__INACTIVE__";
var Fiber = class {
  parent;
  inject;
  runtime;
  /** Unique id within the registry; 0 for the root fiber, `null` once disposed. */
  uid;
  /** The context this fiber's plugin runs in (extends the parent context). */
  ctx;
  /** The validated plugin config (updated by `update()`); read through {@link effectiveConfig}. */
  config;
  /** The raw plugin config, re-resolved before activation; use {@link rawConfig} to read or retain it. */
  _config;
  /** Current lifecycle state; transitions emit `internal/status`. */
  state = 0;
  /** Dispose this fiber: unload the plugin, then settle once cleanup finished. */
  dispose;
  /** Snapshot of required service implementations while loaded; `undefined` otherwise. */
  store;
  /** The in-flight load/unload transition, if one is currently running. */
  inertia;
  _hooks = /* @__PURE__ */ Object.create(null);
  _disposables = new DisposableList();
  context;
  _error;
  _runner;
  _store = /* @__PURE__ */ Object.create(null);
  /**
  * Create a fiber. Plugin authors normally obtain fibers from `ctx.plugin()`
  * rather than constructing them directly.
  *
  * @param parent — the context the plugin was loaded from.
  * @param config — raw config, validated against the runtime's schema.
  * @param inject — resolved dependency map (service name → intercept config).
  * @param runtime — the shared plugin runtime, or `null` for the root fiber.
  * @param getOuterStack — captures the caller stack for effect diagnostics.
  */
  constructor(parent, config, inject2, runtime, getOuterStack) {
    this.parent = parent;
    this.inject = inject2;
    this.runtime = runtime;
    this._config = config;
    const collect = (dispose) => {
      this._disposables.push(dispose);
    };
    if (runtime) {
      this.uid = parent.registry.counter;
      this.ctx = this.context = parent.extend({ fiber: this });
      const injectEntries = Object.entries(this.inject);
      if (injectEntries.length) {
        this.ctx[Context.intercept] = Object.create(parent[Context.intercept]);
        for (const [name2, config2] of injectEntries) {
          if (isNullable(config2)) continue;
          this.ctx[Context.intercept][name2] = config2;
        }
      }
      this._runner = {
        epoch: INACTIVE,
        getOuterStack,
        execute: function() {
          if (isConstructor(runtime.callback)) {
            const instance = new runtime.callback(this.ctx, this.config);
            for (const hook of instance?.[symbols.initHooks] ?? []) hook();
            return instance?.[symbols.init]?.();
          } else return runtime.callback(this.ctx, this.config);
        },
        collect
      };
      this.dispose = parent.fiber.effect(() => {
        const remove = runtime.fibers.push(this);
        return async () => {
          this.uid = null;
          emitPluginDisposed(this.context, this);
          if (this.ctx.registry.has(runtime.callback)) {
            remove();
            if (!runtime.fibers.length) this.ctx.registry.delete(runtime.callback);
          }
          this._setEpoch(INACTIVE);
          if (!this.inertia) this._updateState(() => {
            this.inertia = this._unload();
            return 5;
          });
          while (this.inertia) await this.inertia;
        };
      }, "ctx.plugin()");
      try {
        this.context.emit("internal/plugin", this);
      } catch (error) {
        Promise.resolve(this.dispose()).catch((reason3) => this.ctx.logger.error(reason3));
        throw error;
      }
      if (this.uid !== null && parent.fiber.state !== 5) {
        for (const name2 of Object.keys(this.inject)) this._checkImpl(name2);
        this._refresh();
      }
    } else {
      this.uid = 0;
      this.ctx = this.context = parent;
      this.state = 2;
      this.store = /* @__PURE__ */ Object.create(null);
      this._runner = {
        epoch: "",
        getOuterStack,
        execute: () => {
        },
        collect
      };
      this.dispose = () => this.restart();
    }
  }
  /** The plugin's display name, inherited from the nearest named ancestor, else `'root'`. */
  get name() {
    let fiber = this;
    do {
      if (fiber.runtime?.name) return fiber.runtime.name;
      fiber = fiber.parent.fiber;
    } while (fiber !== fiber.parent.fiber);
    return "root";
  }
  /**
  * Throw if the fiber has already been disposed.
  *
  * @returns nothing when the fiber is still active.
  * @throws {CordisError} `INACTIVE_EFFECT` when the fiber's uid has been cleared.
  */
  assertActive() {
    if (this.uid !== null) return;
    throw new CordisError("INACTIVE_EFFECT");
  }
  _execute(runner) {
    const oldEpoch = runner.epoch;
    return composeError((info) => {
      const safeCollect = (dispose) => {
        if (typeof dispose === "function") runner.collect(dispose);
        else if (!isNullable(dispose)) throw new TypeError("Invalid effect");
      };
      const effect = runner.execute.call(this);
      if (typeof effect === "function") return runner.collect(effect);
      else if (isNullable(effect)) {
      } else if (!isObject(effect)) throw new TypeError("Invalid effect");
      else if ("then" in effect) return effect.then(safeCollect);
      else if (Symbol.iterator in effect) {
        info.error = /* @__PURE__ */ new Error();
        const iter = effect[Symbol.iterator]();
        while (true) {
          const result = iter.next();
          safeCollect(result.value);
          if (result.done) return;
        }
      } else if (Symbol.asyncIterator in effect) {
        const iter = effect[Symbol.asyncIterator]();
        return (async () => {
          await Promise.resolve();
          info.error = /* @__PURE__ */ new Error();
          while (true) {
            if (runner.epoch !== oldEpoch) return;
            const result = await iter.next();
            safeCollect(result.value);
            if (result.done) return;
          }
        })();
      } else throw new TypeError("Invalid effect");
    }, runner.getOuterStack);
  }
  effect(execute, label = "anonymous") {
    this.assertActive();
    if (this.state === 5) throw new CordisError("INACTIVE_EFFECT");
    const disposables = [];
    let disposing = false;
    let disposalTask;
    const dispose = () => {
      if (disposing) return disposalTask;
      disposing = true;
      let task2;
      for (const disposable of disposables.splice(0).reverse()) if (task2) task2 = task2.then(() => runDisposable(disposable));
      else {
        const result = runDisposable(disposable);
        if (isObject(result) && "then" in result) task2 = result;
      }
      return disposalTask = task2;
    };
    const meta = {
      label,
      children: []
    };
    const runner = {
      execute,
      epoch: true,
      collect: (dispose2) => {
        disposables.push(dispose2);
        this._disposables.delete(dispose2);
        if (dispose2[symbols.effect]) meta.children.push(dispose2[symbols.effect]);
      },
      getOuterStack: buildOuterStack()
    };
    let task;
    let executing = true;
    let resolveSetup;
    let rejectSetup;
    let setupBarrier;
    let setupFailed = false;
    let inFlight;
    let removeWrapper = () => false;
    const waitForSetup = () => {
      setupBarrier ??= new Promise((resolve, reject) => {
        resolveSetup = resolve;
        rejectSetup = reject;
      });
      return setupBarrier;
    };
    const disposeAfter = (setup) => {
      return Promise.resolve(setup).then(() => dispose(), async (reason3) => {
        await dispose();
        throw reason3;
      });
    };
    const finalizeDisposal = (callback) => {
      let result;
      try {
        result = callback();
      } catch (error) {
        removeWrapper();
        throw error;
      }
      if (isObject(result) && "then" in result) {
        const pending = Promise.resolve(result).finally(() => {
          removeWrapper();
          if (inFlight === pending) inFlight = void 0;
        });
        return inFlight = pending;
      }
      removeWrapper();
      return result;
    };
    const wrapper = defineProperty(() => {
      if (!runner.epoch) return setupFailed ? inFlight : void 0;
      runner.epoch = false;
      return finalizeDisposal(() => {
        if (executing) return disposeAfter(waitForSetup());
        return task ? disposeAfter(task) : dispose();
      });
    }, symbols.effect, meta);
    effectInertia.set(wrapper, () => inFlight);
    removeWrapper = this._disposables.push(wrapper);
    try {
      task = this._execute(runner);
    } catch (reason3) {
      executing = false;
      setupFailed = true;
      runner.epoch = false;
      let cleanup;
      try {
        cleanup = finalizeDisposal(dispose);
      } finally {
        rejectSetup?.(reason3);
      }
      if (isObject(cleanup) && "then" in cleanup) cleanup.catch((error) => this.ctx.logger.error(error));
      throw reason3;
    }
    executing = false;
    if (setupBarrier) Promise.resolve(task).then(resolveSetup, rejectSetup);
    task?.catch(() => {
      if (!runner.epoch) return dispose();
      return finalizeDisposal(dispose);
    }).catch((error) => this.ctx.logger.error(error));
    const disposeAsync = () => {
      if (!runner.epoch) return;
      runner.epoch = false;
      return finalizeDisposal(dispose);
    };
    wrapper.then = async (onFulfilled, onRejected) => {
      return Promise.resolve(task).then(() => disposeAsync).then(onFulfilled, onRejected);
    };
    return wrapper;
  }
  /**
  * Return metadata for currently registered effects.
  *
  * @returns one {@link EffectMeta} tree per labeled live effect.
  */
  getEffects() {
    return [...this._disposables].map((dispose) => dispose[symbols.effect]).filter(Boolean);
  }
  _getState() {
    if (this.uid === null) return 4;
    if (this._error) return 3;
    if (this._runner.epoch !== INACTIVE) return 2;
    return 0;
  }
  _updateState(callback) {
    const oldState = this.state;
    this.state = callback() ?? this._getState();
    if (oldState === this.state) return;
    this.context.emit("internal/status", this, oldState);
    if (oldState !== 2 && this.state !== 2) return;
    for (const key of Reflect.ownKeys(this.ctx.reflect.store)) {
      const impl = this.ctx.reflect.store[key];
      if (impl.fiber !== this) continue;
      this.ctx.reflect.notify([impl.name]);
    }
  }
  _checkImpl(name2) {
    const impl = this.ctx.reflect._getImpl(name2, true);
    if (!impl) return delete this._store[name2];
    try {
      if (impl.check && !impl.check.call(getTraceable(this.ctx, impl.value))) return delete this._store[name2];
    } catch (error) {
      impl.fiber.ctx.logger.error(error);
      return delete this._store[name2];
    }
    this._store[name2] = impl;
  }
  _refresh() {
    let epoch = false;
    epoch = "";
    for (const name2 of Object.keys(this.inject)) {
      const impl = this._store[name2];
      if (!impl) {
        epoch = INACTIVE;
        break;
      }
      epoch += ":" + impl.fiber.uid;
    }
    this._setEpoch(epoch);
  }
  _setEpoch(epoch) {
    const oldEpoch = this._runner.epoch;
    if (epoch === oldEpoch) return;
    this._runner.epoch = epoch;
    if (this.inertia) return;
    this._updateState(() => {
      if (epoch !== INACTIVE && oldEpoch === INACTIVE) {
        this.inertia = this._reload();
        return 1;
      } else {
        this.inertia = this._unload();
        return 5;
      }
    });
  }
  /** Raw input retained for the next activation, without evaluating expressions or validating it. */
  get rawConfig() {
    return this.ctx.fiber._config;
  }
  /** Retain raw input without changing the running config or triggering an update. */
  set rawConfig(config) {
    this.ctx.fiber._config = config;
  }
  /**
  * The config object published by the existing activation/update lifecycle, without cloning or re-parsing.
  * It is undefined before the first resolution and remains readable after unload; callers requiring a live instance check `state`.
  */
  get effectiveConfig() {
    return this.ctx.fiber.config;
  }
  /**
  * Evaluate and validate a candidate using this fiber's `internal/config` hooks and runtime schema.
  * Does not publish the result, replace raw input, or commit running volatile references.
  * Required injections must be available when a hook uses them.
  * @param config Raw candidate input.
  * @returns The candidate's resolved config, retaining the schema's result identity.
  * @throws {ValidationError} when the runtime schema rejects the candidate; hook errors propagate.
  */
  resolveConfig(config) {
    const fiber = this.ctx.fiber;
    config = fiber.context.waterfall(fiber, "internal/config", config, () => config);
    return fiber.runtime ? resolveConfig(fiber.runtime, config) : config;
  }
  async _reload() {
    this.store = { ...this._store };
    const oldEpoch = this._runner.epoch;
    try {
      await Promise.resolve();
      if (this._runner.epoch === oldEpoch) {
        this.config = this.resolveConfig(this._config);
        await this._execute(this._runner);
        this._error = void 0;
      }
    } catch (reason3) {
      this.ctx.logger.error(reason3);
      this._error = reason3;
      this._runner.epoch = INACTIVE;
    }
    this._updateState(() => {
      if (this._runner.epoch === oldEpoch) this.inertia = void 0;
      else {
        this.inertia = this._unload();
        return 5;
      }
    });
  }
  async _unload() {
    await Promise.all(this._disposables.clear().map(async (dispose) => {
      try {
        await composeError(async (info) => {
          await Promise.resolve();
          info.error = /* @__PURE__ */ new Error();
          await runDisposable(dispose);
        }, this._runner.getOuterStack);
      } catch (reason3) {
        this.ctx.logger.error(reason3);
      }
    }));
    this.store = void 0;
    this._updateState(() => {
      if (this._runner.epoch === INACTIVE) this.inertia = void 0;
      else {
        this.inertia = this._reload();
        return 1;
      }
    });
  }
  /**
  * Wait for current lifecycle work and rethrow startup errors.
  *
  * @returns this fiber, once it has settled into a stable state.
  * @throws the config-validation or plugin-startup error, if any.
  */
  async await() {
    while (this.inertia) await this.inertia;
    if (this._error) throw this._error;
    return this;
  }
  /**
  * Dispose and immediately reload this plugin with its current config.
  *
  * @returns a promise resolving once the reload settled.
  * @throws {CordisError} `INACTIVE_EFFECT` when the fiber is already disposed.
  */
  async restart() {
    const fiber = this.ctx.fiber;
    fiber.assertActive();
    fiber._setEpoch(INACTIVE);
    fiber._refresh();
    await fiber.await();
  }
  /**
  * Validate and apply new config, then restart the plugin.
  *
  * Runs the `internal/update` waterfall first, so update hooks (and HMR)
  * can veto or replace the restart.
  *
  * @param config — the new raw config; validated before anything restarts.
  * @param noSave — hint for persistence hooks not to write the change back.
  * @returns nothing; the restart runs behind the `internal/update` waterfall.
  * @throws {ValidationError} when the new config fails validation.
  */
  update(config, noSave = false) {
    const fiber = this.ctx.fiber;
    fiber.assertActive();
    fiber._config = config;
    if (fiber.state !== 2) {
      fiber._error = void 0;
      fiber._setEpoch(INACTIVE);
      fiber._refresh();
      return;
    }
    config = fiber.resolveConfig(config);
    fiber.context.waterfall(fiber, "internal/update", config, noSave, () => {
      fiber.config = config;
      fiber._error = void 0;
      return fiber.restart();
    });
  }
};
function isApplicable(object) {
  return object && typeof object === "object" && typeof object.apply === "function";
}
function Inject(name2, config) {
  return function(value, decorator) {
    if (decorator.kind === "class") {
      if (!Object.hasOwn(value, "inject")) {
        defineProperty(value, "inject", Object.create(Object.getPrototypeOf(value).inject ?? null));
        defineProperty(value.inject, symbols.checkProto, true);
      }
      value.inject[name2] = config;
    } else if (decorator.kind === "method") {
      const inject2 = (value[symbols.metadata] ??= {}).inject ??= /* @__PURE__ */ Object.create(null);
      inject2[name2] = config;
      decorator.addInitializer(function() {
        const property = this[symbols.tracker]?.property;
        (this[symbols.initHooks] ??= []).push(() => {
          this.ctx.inject(inject2, (ctx) => {
            return value.call(property ? withProps(this, { [property]: ctx }) : this);
          });
        });
      });
    } else throw new Error("@Inject() can only be used on class or class methods");
  };
}
(function(Inject2) {
  function resolve(inject2, result = /* @__PURE__ */ Object.create(null)) {
    if (!inject2) return result;
    if (Array.isArray(inject2)) for (const name2 of inject2) result[name2] = null;
    else if (Reflect.has(inject2, symbols.checkProto)) {
      Object.assign(result, resolve(Object.getPrototypeOf(inject2)));
      for (const name2 of Object.keys(inject2)) result[name2] = inject2[name2] ?? null;
    } else for (const name2 of Object.keys(inject2)) result[name2] = inject2[name2] ?? null;
    return result;
  }
  Inject2.resolve = resolve;
})(Inject || (Inject = {}));
var RegistryService = class {
  ctx;
  _counter = 0;
  _internal = /* @__PURE__ */ new Map();
  constructor(ctx) {
    this.ctx = ctx;
    defineProperty(this, symbols.tracker, {
      property: "ctx",
      noShadow: true
    });
  }
  /** Allocate the next fiber uid (increments on every read). */
  get counter() {
    return ++this._counter;
  }
  /** Number of registered plugin runtimes. */
  get size() {
    return this._internal.size;
  }
  /**
  * Resolve a supported plugin shape to its executable callback.
  *
  * @param plugin — a function, class, or `{ apply }` object plugin.
  * @returns the callback identifying the plugin, or `undefined` if invalid.
  */
  resolve(plugin) {
    try {
      if (typeof plugin === "function") return plugin;
      if (isApplicable(plugin)) return plugin.apply;
    } catch {
    }
  }
  /**
  * Look up the runtime record for a plugin.
  *
  * @param plugin — any supported plugin shape.
  * @returns the runtime, or `undefined` when the plugin is not registered.
  */
  get(plugin) {
    const key = this.resolve(plugin);
    return key && this._internal.get(key);
  }
  /**
  * Check whether a plugin has a registered runtime.
  *
  * @param plugin — any supported plugin shape.
  * @returns `true` when at least one fiber of the plugin exists.
  */
  has(plugin) {
    const key = this.resolve(plugin);
    return !!key && this._internal.has(key);
  }
  /**
  * Dispose every running fiber for a plugin and remove its runtime record.
  *
  * @param plugin — any supported plugin shape.
  * @returns the removed runtime, or `undefined` when none was registered.
  */
  delete(plugin) {
    const key = this.resolve(plugin);
    const runtime = key && this._internal.get(key);
    if (!runtime) return;
    this._internal.delete(key);
    for (const fiber of runtime.fibers) fiber.dispose();
    return runtime;
  }
  /** Iterate the registered plugin callbacks. */
  keys() {
    return this._internal.keys();
  }
  /** Iterate the registered plugin runtimes. */
  values() {
    return this._internal.values();
  }
  /** Iterate `[callback, runtime]` pairs. */
  entries() {
    return this._internal.entries();
  }
  /**
  * Visit every registered runtime.
  *
  * @param callback — receives each runtime and its identifying callback.
  */
  forEach(callback) {
    return this._internal.forEach(callback);
  }
  /**
  * Start a callback once the requested dependencies are available.
  *
  * @param inject — required services, as an array or a name → config map.
  * @param callback — plugin body called with `(ctx, config)`.
  * @returns the fiber; awaiting it settles once loading finished.
  */
  inject(inject2, callback) {
    return this.plugin({
      inject: inject2,
      apply: callback,
      name: callback.name
    });
  }
  /**
  * Start a plugin in the current context and return its fiber.
  *
  * Creates (or reuses) the plugin's runtime record, then starts a new fiber
  * under the current context. Throws if `plugin` is not a supported shape or
  * if the current fiber is already disposed.
  *
  * @param plugin — a function, class, or `{ apply }` object plugin.
  * @param config — the plugin config, validated against its `Config` schema.
  * @param getOuterStack — captures the caller stack for effect diagnostics.
  * @returns the fiber; awaiting it settles once loading finished.
  */
  plugin(plugin, config, getOuterStack = buildOuterStack()) {
    const callback = this.resolve(plugin);
    if (!callback) throw new Error('invalid plugin, expect function or object with an "apply" method, received ' + typeof plugin);
    this.ctx.fiber.assertActive();
    let runtime = this._internal.get(callback);
    if (!runtime) {
      let name2 = plugin.name;
      if (name2 === "apply") name2 = void 0;
      runtime = {
        name: name2,
        callback,
        fibers: new DisposableList(),
        Config: plugin.Config
      };
      this._internal.set(callback, runtime);
    }
    const fiber = new Fiber(this.ctx, config, Inject.resolve(plugin.inject), runtime, getOuterStack);
    const wrapped = Object.create(fiber);
    wrapped.then = (onFulfilled, onRejected) => {
      return fiber.await().then(onFulfilled, onRejected);
    };
    return wrapped;
  }
};
var Context = class Context2 {
  /** Symbol key under which a disposer exposes its {@link EffectMeta} diagnostics tree. */
  static effect = symbols.effect;
  /** Symbol key for a context's listener filter, consulted on every event dispatch. */
  static filter = symbols.filter;
  /** Symbol key of the isolation map (see the `Context[symbols.isolate]` property). */
  static isolate = symbols.isolate;
  /** Symbol key of the intercept map (see the `Context[symbols.intercept]` property). */
  static intercept = symbols.intercept;
  /**
  * Returns true for Cordis context proxies and context prototypes.
  *
  * Works across realms and across multiple copies of cordis, because the
  * brand is keyed by a global symbol rather than by `instanceof`.
  *
  * @param value — the value to test.
  * @returns `true` if `value` is a Cordis context, narrowing its type.
  */
  static is(value) {
    return !!value?.[Context2.is];
  }
  static {
    Context2.is[Symbol.toPrimitive] = () => Symbol.for("cordis.is");
    Context2.prototype[Context2.is] = true;
  }
  /** Create the root context and install the built-in services. */
  constructor() {
    this[symbols.isolate] = /* @__PURE__ */ Object.create(null);
    this[symbols.intercept] = /* @__PURE__ */ Object.create(null);
    const self = new Proxy(this, ReflectService.handler);
    this.root = self;
    this.baseUrl = void 0;
    this.fiber = new Fiber(self, {}, /* @__PURE__ */ Object.create(null), null, () => []);
    this.reflect = new ReflectService(self);
    this.registry = new RegistryService(self);
    this.events = new EventsService(self);
    this.logger = new LoggerService(self);
    this.fiber._disposables.clear();
    return self;
  }
  [Symbol.for("nodejs.util.inspect.custom")]() {
    return `Context <${this.fiber.name}>`;
  }
  /**
  * Create a child context with extra metadata on top of the current scope.
  *
  * The child prototypally inherits every property of this context; own
  * properties of `meta` shadow the inherited ones. The parent is not mutated.
  *
  * @param meta — own properties (including symbol keys) to define on the child.
  * @returns a child context inheriting from this one.
  */
  extend(meta = {}) {
    const shadow = Reflect.getOwnPropertyDescriptor(this, symbols.shadow)?.value;
    const self = Object.create(getTraceable(this, this));
    for (const prop of Reflect.ownKeys(meta)) Object.defineProperty(self, prop, Reflect.getOwnPropertyDescriptor(meta, prop));
    if (!shadow) return self;
    return Object.assign(Object.create(self), { [symbols.shadow]: shadow });
  }
  /**
  * Create a child context with an independent service scope for `name`.
  *
  * Below the returned context, reads and writes of the service `name`
  * resolve against the new label instead of the parent's, so a different
  * implementation can be provided without affecting the parent scope.
  * Passing the same `label` to two `isolate()` calls joins their scopes.
  *
  * @param name — the service name to isolate.
  * @param label — scope label to join; defaults to a fresh unique symbol.
  * @returns a child context whose `name` service resolves in the new scope.
  */
  isolate(name2, label) {
    const shadow = Object.create(this[symbols.isolate]);
    shadow[name2] = label ?? Symbol(name2);
    return this.extend({ [symbols.isolate]: shadow });
  }
  intercept(name2, config) {
    const intercept = Object.create(this[symbols.intercept]);
    intercept[name2] = config;
    return this.extend({ [symbols.intercept]: intercept });
  }
};
var Service = class Service2 {
  ctx;
  /** Symbol key of an instance method run after construction (class plugins). */
  static init = symbols.init;
  /** Symbol key of the availability predicate passed to `ctx.provide()`. */
  static check = symbols.check;
  /** Symbol key of the phantom intercept-config type parameter. */
  static config = symbols.config;
  /** Symbol key of the call body making a service callable (e.g. `ctx.logger()`). */
  static invoke = symbols.invoke;
  /** Symbol key of the helper deriving an extended service instance. */
  static extend = symbols.extend;
  /** Symbol key of the tracker metadata used for context tracing. */
  static tracker = symbols.tracker;
  /** Symbol key of the intercept-config resolution helper below. */
  static resolveConfig = symbols.resolveConfig;
  /** The service name this instance is registered under. */
  name;
  /**
  * Register this instance as `name` in the current context.
  *
  * Calls `ctx.reflect.provide(name, this, this[Service.check])`, so the
  * service is unregistered automatically when the owning fiber unloads.
  * Services with a `[Service.invoke]` body return a callable instance.
  *
  * @param ctx — the context to register in (stored as `this.ctx`).
  * @param name — the service name; defaults to the static `provide` field.
  */
  constructor(ctx, name2) {
    this.ctx = ctx;
    name2 ??= this.constructor["provide"];
    let self = this;
    const tracker = {
      associate: name2,
      property: "ctx"
    };
    if (self[symbols.invoke]) self = createCallable(name2, joinPrototype(Object.getPrototypeOf(this), Function.prototype), tracker);
    self.ctx = ctx;
    self.name = name2;
    defineProperty(self, symbols.tracker, tracker);
    self.ctx.reflect.provide(name2, self, this[symbols.check]);
    return self;
  }
  [symbols.filter](ctx) {
    return ctx[symbols.isolate][this.name] === this.ctx[symbols.isolate][this.name];
  }
  [symbols.extend](props) {
    let self;
    if (this[Service2.invoke]) self = createCallable(this.name, this, this[symbols.tracker]);
    else self = Object.create(this);
    return Object.assign(self, props);
  }
  /**
  * Merge intercept config from ancestors with optional base and head values.
  *
  * Entries added closer to the root apply first; `base` is prepended and
  * `head` appended. Uses `Config.merge` when the service declares one,
  * otherwise a shallow `Object.assign`.
  *
  * @param base — lowest-precedence config merged before all intercepts.
  * @param head — highest-precedence config merged after all intercepts.
  * @returns the merged config.
  */
  [symbols.resolveConfig](base, head) {
    let intercept = this.ctx[Context.intercept];
    const configs = [];
    while (this.name in intercept) {
      if (Object.hasOwn(intercept, this.name)) configs.unshift(intercept[this.name]);
      intercept = Object.getPrototypeOf(intercept);
    }
    if (base) configs.unshift(base);
    if (head) configs.push(head);
    if (this["Config"]?.merge) return this["Config"].merge(...configs);
    else return Object.assign({}, ...configs);
  }
  static [Symbol.hasInstance](instance) {
    if (!instance) return false;
    let constructor = instance.constructor;
    while (constructor) {
      constructor = constructor.prototype?.constructor;
      if (constructor === this) return true;
      constructor &&= Object.getPrototypeOf(constructor);
    }
    return false;
  }
};

// node_modules/@deepseek-ai/dsh-typert-protocol/lib/index.js
var TYPERT_OWNED_VALUE = Symbol.for("dsh.typert.owned-value");
var TYPERT_REMOTE_SEGMENT_PATTERN = /^[A-Za-z0-9_$.-]+$/;
function isTypertRemoteSegment(value) {
  return value !== "." && value !== ".." && TYPERT_REMOTE_SEGMENT_PATTERN.test(value);
}
var REMOTE_METHOD_DESCRIPTOR = "@deepseek-ai/dsh-typert-protocol/remote-methods";
function bindTypertRemote(service, serviceKey, options = {}) {
  validateName("service key", serviceKey);
  const namespace = options.namespace ?? serviceKey;
  validateName("namespace", namespace);
  const ctx = Reflect.get(service, "ctx");
  if (Context.is(ctx)) provideInvocationAccessor(ctx);
  return Object.freeze({
    service,
    serviceKey,
    namespace
  });
}
var TypertRemoteService = class extends Service {
  /** Visible binding consumed by the Gateway's source-mode discovery. */
  typertRemote;
  /**
  * Register the Service and bind the same key to Typert Gateway.
  * @param ctx - owning Cordis Context.
  * @param serviceKey - exact Cordis service key and default wire namespace.
  * @param options - optional wire namespace.
  */
  constructor(ctx, serviceKey, options = {}) {
    super(ctx, serviceKey);
    this.typertRemote = bindTypertRemote(this, this.name, options);
  }
};
function provideInvocationAccessor(ctx) {
  if (Object.hasOwn(ctx.root.reflect.props, "invocation")) return;
  ctx.root.accessor("invocation", { get: () => void 0 });
}
function Remote(methodExportOrOptions, context) {
  if (typeof methodExportOrOptions === "string") {
    validateName("Remote export name", methodExportOrOptions);
    return remoteDecorator({ kind: "direct" }, { exportName: methodExportOrOptions });
  }
  if (typeof methodExportOrOptions === "object") return remoteDecorator({ kind: "direct" }, markerOptions(methodExportOrOptions, REMOTE_OPTION_KEYS, 'Remote options require name, mode: "stream", or resources'));
  if (context === void 0) throw new TypeError("typert-protocol: Remote decorator context is missing");
  addMarkerInitializer(context, { kind: "direct" });
}
var REMOTE_OPTION_KEYS = /* @__PURE__ */ new Set([
  "name",
  "mode",
  "resources"
]);
function markerOptions(options, accepted, requirement) {
  const keys = Reflect.ownKeys(options);
  if (keys.length === 0 || keys.some((key) => typeof key !== "string" || !accepted.has(key)) || Object.hasOwn(options, "mode") && options.mode !== "stream") throw new TypeError(`typert-protocol: ${requirement}`);
  const { mode, name: name2, resources } = options;
  if (name2 !== void 0) validateName("Remote export name", name2);
  return {
    ...mode === void 0 ? {} : { mode },
    ...name2 === void 0 ? {} : { exportName: name2 },
    ...resources === void 0 ? {} : { resources: resourceReaders(resources) }
  };
}
function resourceReaders(resources) {
  const readers = {};
  for (const [kind, reader] of Object.entries(resources)) {
    if (typeof reader !== "function") throw new TypeError(`typert-protocol: Remote resource reader "${kind}" must be a function`);
    readers[kind] = reader;
  }
  if (Object.keys(readers).length === 0) throw new TypeError("typert-protocol: Remote resources require at least one reader");
  return Object.freeze(readers);
}
function remoteDecorator(invocation, options = {}) {
  return function(_method, context) {
    addMarkerInitializer(context, invocation, options);
  };
}
function readRemoteMethodDescriptor(prototype) {
  const property = Object.getOwnPropertyDescriptor(prototype, REMOTE_METHOD_DESCRIPTOR);
  if (property === void 0) return void 0;
  const descriptor = property.value;
  if (descriptor === null || typeof descriptor !== "object") throw new TypeError("typert-protocol: Remote method descriptor must be an object");
  const version = Reflect.get(descriptor, "version");
  if (version !== 1) throw new TypeError(`typert-protocol: unsupported Remote method descriptor version ${String(version)}`);
  const methods = Reflect.get(descriptor, "methods");
  if (!Array.isArray(methods)) throw new TypeError("typert-protocol: Remote method descriptor methods must be an array");
  return descriptor;
}
function addMarkerInitializer(context, invocation, options = {}) {
  if (context.private || context.static || typeof context.name !== "string") throw new TypeError("typert-protocol: Remote decorators require a public instance method with a string name");
  const method = context.name;
  context.addInitializer(function() {
    const prototype = Object.getPrototypeOf(this);
    if (prototype === null) throw new TypeError(`typert-protocol: cannot mark Remote method "${method}" on an object without a prototype`);
    mark(prototype, method, invocation, options);
  });
}
function mark(prototype, method, invocation, { mode, exportName, resources }) {
  const descriptor = readRemoteMethodDescriptor(prototype);
  const marker = Object.freeze({
    method,
    ...exportName === void 0 || exportName === method ? {} : { exportName },
    ...mode === void 0 ? {} : { mode },
    invocation: Object.freeze(invocation),
    ...resources === void 0 ? {} : { resources }
  });
  const current = descriptor?.methods.find((candidate) => candidate.method === method);
  if (current !== void 0) {
    if (current.exportName === marker.exportName && current.mode === marker.mode && current.resources === marker.resources && sameInvocation(current.invocation, invocation)) return;
    throw new Error(`typert-protocol: Remote method "${method}" has conflicting invocation markers`);
  }
  Object.defineProperty(prototype, REMOTE_METHOD_DESCRIPTOR, {
    configurable: true,
    value: Object.freeze({
      version: 1,
      methods: Object.freeze([...descriptor?.methods ?? [], marker])
    })
  });
}
function sameInvocation(left, right) {
  if (left.kind === "direct") return right.kind === "direct";
  if (right.kind === "direct") return false;
  return left.context === right.context;
}
function validateName(subject, value) {
  if (!isTypertRemoteSegment(value)) throw new TypeError(`typert-protocol: ${subject} must contain only RPC endpoint segment characters`);
}

// src/brand.ts
var _vote_dec, _writeTableRow_dec, _insertFormRow_dec, _webLink_dec, _updatePlugin_dec, _saveServer_dec, _testServer_dec, _serverSettings_dec, _doctor_dec, _login_dec, _overview_dec, _turnWrites_dec, _kbDoc_dec, _setChecklist_dec, _checklists_dec, _board_dec, _workbenchData_dec, _unbindApp_dec, _bindApp_dec, _appBindings_dec, _disableApp_dec, _enableApp_dec, _availableApps_dec, _apps_dec, _brand_dec, _a, _init;
var DshEEClientService = class extends (_a = TypertRemoteService, _brand_dec = [Remote], _apps_dec = [Remote], _availableApps_dec = [Remote], _enableApp_dec = [Remote], _disableApp_dec = [Remote], _appBindings_dec = [Remote], _bindApp_dec = [Remote], _unbindApp_dec = [Remote], _workbenchData_dec = [Remote], _board_dec = [Remote], _checklists_dec = [Remote], _setChecklist_dec = [Remote], _kbDoc_dec = [Remote], _turnWrites_dec = [Remote], _overview_dec = [Remote], _login_dec = [Remote], _doctor_dec = [Remote], _serverSettings_dec = [Remote], _testServer_dec = [Remote], _saveServer_dec = [Remote], _updatePlugin_dec = [Remote], _webLink_dec = [Remote], _insertFormRow_dec = [Remote], _writeTableRow_dec = [Remote], _vote_dec = [Remote], _a) {
  constructor(ctx, actions) {
    super(ctx, "dshEE");
    this.actions = actions;
    __runInitializers(_init, 5, this);
  }
  async brand() {
    return this.actions.readBrand();
  }
  async apps() {
    return this.actions.readApps();
  }
  async availableApps() {
    return this.actions.readAvailableApps();
  }
  async enableApp(request) {
    return this.actions.setAppSubscription(request?.name, true);
  }
  async disableApp(request) {
    return this.actions.setAppSubscription(request?.name, false);
  }
  async appBindings() {
    return this.actions.readAppBindings();
  }
  async bindApp(request) {
    return this.actions.bindApp(request.name, request.sessionId);
  }
  async unbindApp(request) {
    return this.actions.unbindApp(request.name);
  }
  async workbenchData(request) {
    return this.actions.workbenchData(request);
  }
  async board(request) {
    return this.actions.readBoard(request);
  }
  async checklists(request) {
    return this.actions.readChecklists(request);
  }
  async setChecklist(request) {
    return this.actions.setChecklist(request);
  }
  async kbDoc(request) {
    return this.actions.readKBDoc(request);
  }
  async turnWrites(request) {
    return this.actions.turnWrites(request);
  }
  async overview() {
    return this.actions.readOverview();
  }
  async login() {
    return this.actions.login();
  }
  async doctor() {
    return this.actions.doctor();
  }
  async serverSettings() {
    return this.actions.serverSettings();
  }
  async testServer(request) {
    return this.actions.testServer(request ?? {});
  }
  async saveServer(request) {
    return this.actions.saveServer(request ?? {});
  }
  async updatePlugin() {
    return this.actions.updatePlugin();
  }
  async webLink(request) {
    return this.actions.webLink(request ?? {});
  }
  async insertFormRow(request) {
    return this.actions.insertFormRow(request);
  }
  async writeTableRow(request) {
    return this.actions.writeTableRow(request);
  }
  async vote(request) {
    return this.actions.vote(request);
  }
};
_init = __decoratorStart(_a);
__decorateElement(_init, 1, "brand", _brand_dec, DshEEClientService);
__decorateElement(_init, 1, "apps", _apps_dec, DshEEClientService);
__decorateElement(_init, 1, "availableApps", _availableApps_dec, DshEEClientService);
__decorateElement(_init, 1, "enableApp", _enableApp_dec, DshEEClientService);
__decorateElement(_init, 1, "disableApp", _disableApp_dec, DshEEClientService);
__decorateElement(_init, 1, "appBindings", _appBindings_dec, DshEEClientService);
__decorateElement(_init, 1, "bindApp", _bindApp_dec, DshEEClientService);
__decorateElement(_init, 1, "unbindApp", _unbindApp_dec, DshEEClientService);
__decorateElement(_init, 1, "workbenchData", _workbenchData_dec, DshEEClientService);
__decorateElement(_init, 1, "board", _board_dec, DshEEClientService);
__decorateElement(_init, 1, "checklists", _checklists_dec, DshEEClientService);
__decorateElement(_init, 1, "setChecklist", _setChecklist_dec, DshEEClientService);
__decorateElement(_init, 1, "kbDoc", _kbDoc_dec, DshEEClientService);
__decorateElement(_init, 1, "turnWrites", _turnWrites_dec, DshEEClientService);
__decorateElement(_init, 1, "overview", _overview_dec, DshEEClientService);
__decorateElement(_init, 1, "login", _login_dec, DshEEClientService);
__decorateElement(_init, 1, "doctor", _doctor_dec, DshEEClientService);
__decorateElement(_init, 1, "serverSettings", _serverSettings_dec, DshEEClientService);
__decorateElement(_init, 1, "testServer", _testServer_dec, DshEEClientService);
__decorateElement(_init, 1, "saveServer", _saveServer_dec, DshEEClientService);
__decorateElement(_init, 1, "updatePlugin", _updatePlugin_dec, DshEEClientService);
__decorateElement(_init, 1, "webLink", _webLink_dec, DshEEClientService);
__decorateElement(_init, 1, "insertFormRow", _insertFormRow_dec, DshEEClientService);
__decorateElement(_init, 1, "writeTableRow", _writeTableRow_dec, DshEEClientService);
__decorateElement(_init, 1, "vote", _vote_dec, DshEEClientService);
__decoratorMetadata(_init, DshEEClientService);
function registerBrand(ctx, actions) {
  return new DshEEClientService(ctx, actions);
}

// src/overview.ts
function overviewFailure(config, error) {
  const err = error;
  const message = String(err?.message ?? error);
  if (message === "HTTP 401") return { reason: "auth", base: config.base, detail: message };
  if (/^HTTP \d+$/.test(message)) return { reason: "server", base: config.base, detail: message };
  const detail = err?.name === "TimeoutError" ? "\u8FDE\u63A5\u8D85\u65F6" : err?.cause?.code || err?.cause?.message || message;
  return { reason: "unreachable", base: config.base, detail };
}
async function bridgeJson(bridge, name2, args) {
  const result = await bridge.callTool(name2, args);
  if (result?.isError) throw new Error("\u4F01\u4E1A\u5DE5\u5177\u8C03\u7528\u5931\u8D25");
  if (result?.structuredContent) return result.structuredContent;
  const text = result?.content?.find((part) => part.type === "text")?.text;
  if (typeof text !== "string") throw new Error("\u4F01\u4E1A\u5DE5\u5177\u6CA1\u6709\u8FD4\u56DE JSON");
  return JSON.parse(text);
}
async function readEnterpriseOverview(config, http, bridge) {
  const me = await http.json("/api/dsh/whoami");
  const [usage, modelList, doctor] = await Promise.all([
    bridgeJson(bridge, "ee_me", { action: "usage" }).catch(() => void 0),
    http.json("/v1/models").catch(() => void 0),
    bridgeJson(bridge, "ee_doctor", { action: "check" }).catch(() => void 0)
  ]);
  return {
    me,
    usage,
    models: modelList?.data?.map((model) => model.id) ?? [],
    default_model: doctor?.default_model,
    version: config.version,
    latest_version: doctor?.connect_version
  };
}

// src/server-settings.ts
import fs3 from "node:fs";
import path4 from "node:path";
function serverSettingsPath() {
  return path4.join(dshHome(), "dsh-ee", "server.json");
}
function normalizeBase(input) {
  const raw = String(input ?? "").trim().replace(/\/+$/, "");
  try {
    const url = new URL(raw);
    if (url.protocol !== "http:" && url.protocol !== "https:" || !url.host || url.username || url.password || url.search || url.hash) return void 0;
    return raw;
  } catch {
    return void 0;
  }
}
function readBaseOverride() {
  try {
    const data = JSON.parse(fs3.readFileSync(serverSettingsPath(), "utf8"));
    return normalizeBase(data.base);
  } catch {
    return void 0;
  }
}
function writeBaseOverride(base) {
  const file = serverSettingsPath();
  if (!base) {
    fs3.rmSync(file, { force: true });
    return;
  }
  fs3.mkdirSync(path4.dirname(file), { recursive: true, mode: 448 });
  fs3.writeFileSync(file, `${JSON.stringify({ base }, null, 2)}
`, { mode: 384 });
}
function reason(error) {
  const err = error;
  if (err?.name === "TimeoutError") return "\u8FDE\u63A5\u8D85\u65F6\uFF1A\u8FD9\u53F0\u7535\u8111\u8BBF\u95EE\u4E0D\u5230\u8FD9\u4E2A\u5730\u5740\uFF08\u5185\u7F51\u5730\u5740\u3001VPN \u6CA1\u8FDE\uFF0C\u6216\u8005\u88AB\u9632\u706B\u5899\u6321\u4F4F\uFF09";
  const code = err?.cause?.code;
  if (code === "ECONNREFUSED") return "\u8FDE\u63A5\u88AB\u62D2\u7EDD\uFF1A\u5730\u5740\u80FD\u627E\u5230\uFF0C\u4F46\u8FD9\u4E2A\u7AEF\u53E3\u4E0A\u6CA1\u6709\u670D\u52A1";
  if (code === "ENOTFOUND" || code === "EAI_AGAIN") return "\u627E\u4E0D\u5230\u8FD9\u4E2A\u57DF\u540D\uFF1A\u68C0\u67E5\u62FC\u5199\uFF0C\u6216\u8005\u662F\u5426\u9700\u8981\u8FDE VPN";
  if (code === "CERT_HAS_EXPIRED" || code?.startsWith?.("ERR_TLS") || code?.includes?.("CERT")) return `HTTPS \u8BC1\u4E66\u6709\u95EE\u9898\uFF08${code}\uFF09`;
  return code || err?.cause?.message || err?.message || String(error);
}
var ServerSettingsService = class {
  constructor(config, tokens, onChange, timeoutMs = 8e3) {
    this.config = config;
    this.tokens = tokens;
    this.onChange = onChange;
    this.timeoutMs = timeoutMs;
    this.packaged = config.base;
    const override = readBaseOverride();
    if (override) config.base = override;
  }
  packaged;
  read() {
    return { base: this.config.base, packaged: this.packaged, overridden: this.config.base !== this.packaged, org: this.config.org, version: this.config.version };
  }
  /** 按顺序测：地址格式、能不能连上、是不是 dsh-ee、本机的登录在这台 dsh-ee 上还有没有效。 */
  async test(input) {
    const base = input === void 0 || input === "" ? this.config.base : normalizeBase(input);
    const checks = [];
    const done = (b) => ({ base: b, ok: checks.every((c) => c.ok), checks });
    if (!base) {
      checks.push({ id: "address", ok: false, title: "\u5730\u5740\u683C\u5F0F\u4E0D\u5BF9", detail: "\u8981\u5199\u5B8C\u6574\u7684 http:// \u6216 https:// \u5730\u5740\uFF0C\u4F8B\u5982 https://ee.example.com\uFF0C\u4E0D\u8981\u5E26\u8DEF\u5F84\u4EE5\u5916\u7684\u53C2\u6570" });
      return done(String(input ?? ""));
    }
    const started = Date.now();
    let health;
    try {
      health = await fetch(`${base}/healthz`, { signal: AbortSignal.timeout(this.timeoutMs) });
    } catch (error) {
      checks.push({ id: "reach", ok: false, title: "\u8FDE\u4E0D\u4E0A", detail: reason(error) });
      return done(base);
    }
    checks.push({ id: "reach", ok: true, title: `\u80FD\u8FDE\u4E0A\uFF08${Date.now() - started} ms\uFF09` });
    const body = await health.json().catch(() => null);
    if (!health.ok || body?.status !== "ok") {
      checks.push({ id: "server", ok: false, title: "\u8FD9\u4E2A\u5730\u5740\u4E0A\u4E0D\u662F dsh-ee", detail: `\u8BBF\u95EE /healthz \u8FD4\u56DE HTTP ${health.status}\uFF1B\u786E\u8BA4\u5730\u5740\u662F dsh-ee \u7F51\u9875\u7684\u5730\u5740` });
      return done(base);
    }
    checks.push({ id: "server", ok: true, title: `\u662F dsh-ee${body.version ? `\uFF08${body.version}\uFF09` : ""}` });
    const current = await this.tokens.resolve();
    if (!current?.value) {
      checks.push({ id: "login", ok: false, title: "\u8FD9\u53F0\u7535\u8111\u8FD8\u6CA1\u6709\u767B\u5F55\u4F01\u4E1A\u8D26\u53F7", detail: "\u4FDD\u5B58\u5730\u5740\u540E\uFF0C\u5728\u4FA7\u680F\u4F01\u4E1A\u9762\u677F\u70B9\u300C\u767B\u5F55\u300D\uFF0C\u6216\u5728\u5BF9\u8BDD\u6846\u8F93\u5165 /ee-login" });
      return done(base);
    }
    try {
      const me = await fetch(`${base}/api/dsh/whoami`, { headers: { Authorization: `Bearer ${current.value}` }, signal: AbortSignal.timeout(this.timeoutMs) });
      if (me.ok) {
        const who = await me.json();
        checks.push({ id: "login", ok: true, title: `\u5DF2\u767B\u5F55 ${who.username ?? ""}${who.department ? `\uFF08${who.department}\uFF09` : ""}` });
      } else {
        checks.push({ id: "login", ok: false, title: "\u672C\u673A\u7684\u767B\u5F55\u5728\u8FD9\u53F0 dsh-ee \u4E0A\u65E0\u6548", detail: "\u4FDD\u5B58\u5730\u5740\u540E\u91CD\u65B0\u767B\u5F55\u5373\u53EF\uFF08\u4EE4\u724C\u662F\u6309 dsh-ee \u53D1\u7684\uFF0C\u6362\u4E86\u670D\u52A1\u5668\u8981\u91CD\u65B0\u767B\u5F55\uFF09" });
      }
    } catch (error) {
      checks.push({ id: "login", ok: false, title: "\u68C0\u67E5\u767B\u5F55\u5931\u8D25", detail: reason(error) });
    }
    return done(base);
  }
  /** 保存地址并立即生效；传空或与插件包里的一样就恢复默认。 */
  async save(input) {
    const base = input === void 0 || input === null || input === "" ? this.packaged : normalizeBase(input);
    if (!base) throw new Error("\u5730\u5740\u683C\u5F0F\u4E0D\u5BF9\uFF1A\u8981\u5199\u5B8C\u6574\u7684 http:// \u6216 https:// \u5730\u5740");
    writeBaseOverride(base === this.packaged ? void 0 : base);
    if (base !== this.config.base) {
      this.config.base = base;
      await this.onChange();
    }
    return this.read();
  }
};

// src/connect-update.ts
var reason2 = (error) => error instanceof Error ? error.message : String(error);
async function updateConnect(ctx, config, http) {
  const manager = ctx.get("pluginManager");
  if (!manager || typeof manager.installBundle !== "function") {
    return { ok: false, version: config.version, message: "\u8FD9\u53F0 DSH \u6CA1\u6709\u63D2\u4EF6\u7BA1\u7406\u63A5\u53E3\uFF0C\u8BF7\u5728\u63D2\u4EF6\u9875\u624B\u52A8\u66F4\u65B0\u63A5\u5165\u63D2\u4EF6" };
  }
  let info;
  try {
    info = await http.json("/api/connect");
  } catch (error) {
    const why = reason2(error);
    return { ok: false, version: config.version, message: `\u67E5\u4E0D\u5230\u4F01\u4E1A\u7AEF\u7684\u63A5\u5165\u63D2\u4EF6\u7248\u672C\uFF1A${why}${why === "HTTP 401" ? "\uFF1B\u5148\u767B\u5F55\u4F01\u4E1A\u8D26\u53F7\u518D\u8BD5" : ""}` };
  }
  const latest = String(info?.version ?? "");
  const pkg = String(info?.package ?? "");
  if (!info?.ready || !latest || !pkg) {
    return { ok: false, version: config.version, message: info?.error || "\u4F01\u4E1A\u7AEF\u8FD8\u6CA1\u6709\u53D1\u5E03\u63A5\u5165\u63D2\u4EF6\uFF0C\u8BF7\u7BA1\u7406\u5458\u5148\u5728\u7CFB\u7EDF\u8BBE\u7F6E\u91CC\u53D1\u5E03" };
  }
  if (config.version !== "dev" && compareVersions(latest, config.version) <= 0) {
    return { ok: true, version: config.version, latest, package: pkg, message: `\u5DF2\u662F\u6700\u65B0\uFF08${config.version}\uFF09` };
  }
  const done = { ok: true, updated: true, version: config.version, latest, package: pkg, message: `\u5DF2\u5B89\u88C5 ${pkg}@${latest}\uFF0C\u91CD\u542F DSH \u540E\u751F\u6548` };
  try {
    await manager.installBundle(info.tarball_url || `${config.base}/dl/connect/dsh-connect-${latest}.tgz`);
    return done;
  } catch (tarballError) {
    try {
      await manager.installBundle(`${pkg}@${latest}`, { registry: `${config.base}/npm/` });
      return done;
    } catch (registryError) {
      return { ok: false, version: config.version, latest, package: pkg, message: `\u5B89\u88C5\u5931\u8D25\uFF1A${reason2(registryError) || reason2(tarballError)}` };
    }
  }
}

// src/upload.ts
import fs4 from "node:fs";
import os3 from "node:os";
import path5 from "node:path";
var MAX_BYTES = 128 * 1024 * 1024;
function fail(text) {
  return { content: [{ type: "text", text }], isError: true };
}
async function uploadPackage(http, args) {
  const raw = String(args.path ?? "").trim();
  if (!raw) return fail("\u7F3A\u5C11 path\uFF1A.tgz \u6587\u4EF6\u7684\u672C\u673A\u8DEF\u5F84");
  const file = path5.resolve(raw.startsWith("~") ? path5.join(os3.homedir(), raw.slice(1)) : raw);
  if (!/\.(tgz|tar\.gz)$/i.test(file)) return fail("\u53EA\u80FD\u4E0A\u4F20 npm pack \u751F\u6210\u7684 .tgz \u6587\u4EF6");
  let stat;
  try {
    stat = fs4.statSync(file);
  } catch {
    return fail(`\u627E\u4E0D\u5230\u6587\u4EF6\uFF1A${file}`);
  }
  if (!stat.isFile()) return fail(`\u4E0D\u662F\u6587\u4EF6\uFF1A${file}`);
  if (stat.size > MAX_BYTES) return fail("\u6587\u4EF6\u592A\u5927");
  let response;
  try {
    response = await http.fetch("/api/agent/packages", {
      method: "POST",
      headers: { "content-type": "application/octet-stream" },
      body: fs4.readFileSync(file)
    });
  } catch {
    return fail("\u8FDE\u4E0D\u4E0A dsh-ee\uFF0C\u8BF7\u68C0\u67E5\u7F51\u7EDC\u6216 VPN");
  }
  const body = await response.json().catch(() => null);
  if (!response.ok) return fail(body?.error ?? `\u4E0A\u4F20\u5931\u8D25\uFF08HTTP ${response.status}\uFF09`);
  return { content: [{ type: "text", text: JSON.stringify(body) }], isError: false };
}

// src/app-bindings.ts
import fs5 from "node:fs";
import path6 from "node:path";
var AppBindings = class {
  constructor(file = path6.join(dshHome(), "dsh-ee", "apps.json")) {
    this.file = file;
  }
  sequence = 0;
  warningMessage = "";
  list() {
    return this.read().bindings;
  }
  warning() {
    return this.warningMessage;
  }
  bind(name2, sessionId) {
    if (!/^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(name2) || name2.length > 48) throw new Error("\u5FAE\u5E94\u7528\u540D\u4E0D\u5408\u6CD5");
    if (typeof sessionId !== "string" || !sessionId.trim() || sessionId.length > 256) throw new Error("\u4F1A\u8BDD id \u4E0D\u5408\u6CD5");
    const bindings = this.list();
    bindings[name2] = { sessionId: sessionId.trim(), updatedAt: (/* @__PURE__ */ new Date()).toISOString() };
    this.write(bindings);
    return bindings;
  }
  unbind(name2) {
    if (!/^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(name2) || name2.length > 48) throw new Error("\u5FAE\u5E94\u7528\u540D\u4E0D\u5408\u6CD5");
    const bindings = this.list();
    delete bindings[name2];
    this.write(bindings);
    return bindings;
  }
  read() {
    this.warningMessage = "";
    if (!fs5.existsSync(this.file)) return { version: 1, bindings: {} };
    try {
      const value = JSON.parse(fs5.readFileSync(this.file, "utf8"));
      if (value?.version !== 1 || !value.bindings || typeof value.bindings !== "object" || Array.isArray(value.bindings)) throw new Error("invalid");
      const bindings = {};
      for (const [name2, binding] of Object.entries(value.bindings)) {
        const sessionId = binding?.sessionId;
        const updatedAt = binding?.updatedAt;
        if (typeof sessionId === "string" && sessionId && typeof updatedAt === "string") bindings[name2] = { sessionId, updatedAt };
      }
      return { version: 1, bindings };
    } catch {
      this.warningMessage = `\u5FAE\u5E94\u7528\u4F1A\u8BDD\u7ED1\u5B9A\u6587\u4EF6\u5DF2\u635F\u574F\uFF0C\u5DF2\u6309\u7A7A\u7ED1\u5B9A\u5904\u7406\uFF08${this.file}\uFF09`;
      return { version: 1, bindings: {} };
    }
  }
  write(bindings) {
    fs5.mkdirSync(path6.dirname(this.file), { recursive: true, mode: 448 });
    const temp = `${this.file}.${process.pid}.${++this.sequence}.tmp`;
    try {
      fs5.writeFileSync(temp, JSON.stringify({ version: 1, bindings }, null, 2) + "\n", { mode: 384 });
      fs5.renameSync(temp, this.file);
      fs5.chmodSync(this.file, 384);
      this.warningMessage = "";
    } finally {
      if (fs5.existsSync(temp)) fs5.unlinkSync(temp);
    }
  }
};

// src/app-subscription.ts
var nameRe = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;
async function setAppSubscription(http, name2, enabled) {
  if (typeof name2 !== "string" || !nameRe.test(name2) || name2.length > 48) return { ok: false, error: "\u5FAE\u5E94\u7528\u540D\u4E0D\u5408\u6CD5" };
  const response = await http.fetch(`/api/dsh/apps/${encodeURIComponent(name2)}/subscription`, { method: enabled ? "POST" : "DELETE" });
  if (response.ok) return { ok: true };
  let error = `HTTP ${response.status}`;
  try {
    const body = await response.json();
    if (body?.error) error = body.error;
  } catch {
  }
  return { ok: false, error };
}

// src/web-link.ts
async function exchangeWebLink(base, http, url) {
  if (!base || typeof url !== "string" || !url.startsWith(`${base}/`)) return url;
  const next = url.slice(base.length);
  if (next.startsWith("/api/")) return url;
  try {
    const response = await http.fetch("/api/dsh/web-link", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ next }) }, 1e4);
    if (!response.ok) return url;
    const value = await response.json();
    return typeof value?.url === "string" && /^https?:\/\//.test(value.url) ? value.url : url;
  } catch {
    return url;
  }
}

// src/workbench-host.ts
import fs6 from "node:fs";
import path7 from "node:path";
import { createHash as createHash2 } from "node:crypto";

// src/workbench.ts
var checklistKey = (pane, widget) => `${pane}:${widget}`;
function toolValue(result) {
  if (result?.isError) {
    const text2 = (result.content ?? []).filter((part) => part?.type === "text").map((part) => part.text).join("\n");
    throw new Error(text2 || "\u4F01\u4E1A\u5DE5\u5177\u8C03\u7528\u5931\u8D25");
  }
  if (result?.structuredContent !== void 0) return result.structuredContent;
  const text = (result?.content ?? []).filter((part) => part?.type === "text").map((part) => String(part.text)).join("\n");
  try {
    return JSON.parse(text);
  } catch {
    return text;
  }
}
var maxBoardBytes = 32 << 10;
function checkBoardBlocks(blocks) {
  if (!Array.isArray(blocks) || blocks.length === 0) return { error: "blocks \u5FC5\u987B\u662F\u975E\u7A7A\u6570\u7EC4" };
  if (blocks.length > 20) return { error: "\u770B\u677F\u6700\u591A 20 \u5757" };
  if (new TextEncoder().encode(JSON.stringify(blocks)).length > maxBoardBytes) return { error: "\u770B\u677F\u5185\u5BB9\u4E0D\u80FD\u8D85\u8FC7 32 KB" };
  const out = [];
  const text = (value) => value === null || value === void 0 ? "" : String(value);
  for (const [index, block] of blocks.entries()) {
    const where = `\u7B2C ${index + 1} \u5757`;
    switch (block?.type) {
      case "text":
        if (typeof block.text !== "string" || !block.text.trim()) return { error: `${where}\uFF1Atext \u4E0D\u80FD\u4E3A\u7A7A` };
        out.push({ type: "text", text: block.text });
        break;
      case "list":
        if (!Array.isArray(block.items) || block.items.length === 0 || block.items.length > 50) return { error: `${where}\uFF1Aitems \u8981\u6709 1\uFF5E50 \u9879` };
        out.push({ type: "list", items: block.items.map(text) });
        break;
      case "table":
        if (!Array.isArray(block.columns) || block.columns.length === 0 || block.columns.length > 12) return { error: `${where}\uFF1Acolumns \u8981\u6709 1\uFF5E12 \u5217` };
        if (!Array.isArray(block.rows) || block.rows.length > 50 || !block.rows.every((row) => Array.isArray(row))) return { error: `${where}\uFF1Arows \u5FC5\u987B\u662F\u6700\u591A 50 \u884C\u7684\u4E8C\u7EF4\u6570\u7EC4` };
        out.push({ type: "table", columns: block.columns.map(text), rows: block.rows.map((row) => row.map(text)) });
        break;
      case "citations": {
        if (!Array.isArray(block.items) || block.items.length === 0 || block.items.length > 30) return { error: `${where}\uFF1Aitems \u8981\u6709 1\uFF5E30 \u6761\u5F15\u7528` };
        const items = [];
        for (const [i, item] of block.items.entries()) {
          const at = `${where}\u7B2C ${i + 1} \u6761\u5F15\u7528`;
          const int = (value) => Number.isInteger(value) && value > 0;
          if (!int(item?.doc) || !int(item?.from) || !int(item?.to) || item.to < item.from) return { error: `${at}\uFF1Adoc\u3001from\u3001to \u8981\u662F\u6B63\u6574\u6570\uFF0C\u4E14 to \u4E0D\u5C0F\u4E8E from` };
          if (item.version !== void 0 && item.version !== null && !int(item.version)) return { error: `${at}\uFF1Aversion \u8981\u662F\u6B63\u6574\u6570` };
          if (typeof item.quote !== "string" || !item.quote.trim() || item.quote.length > 500) return { error: `${at}\uFF1Aquote \u8981\u5199 1\uFF5E500 \u5B57\u7684\u539F\u6587\u6458\u53E5` };
          items.push({ doc: item.doc, version: item.version ?? void 0, from: item.from, to: item.to, quote: item.quote, title: item.title === void 0 ? void 0 : text(item.title).slice(0, 200) });
        }
        out.push({ type: "citations", items });
        break;
      }
      default:
        return { error: `${where}\uFF1Atype \u53EA\u80FD\u662F text\u3001list\u3001table \u6216 citations` };
    }
  }
  return { blocks: out };
}
function rowWriteOf(action) {
  if (!action.table) return void 0;
  if (action.delete) return "delete";
  return action.set && Object.keys(action.set).length ? "set" : void 0;
}

// src/workbench-host.ts
function findWidget(apps, request) {
  const app = apps.find((item) => item.name === request?.app);
  const pane = app?.workbench?.panes.find((item) => item.id === request?.pane && item.type === "panel");
  return Number.isInteger(request?.widget) ? pane?.widgets?.[request.widget] : void 0;
}
async function readWidgetData(apps, tools, request) {
  const widget = findWidget(apps, request);
  if (!widget?.source) return { error: "\u76EE\u5F55\u91CC\u6CA1\u6709\u8FD9\u4E2A\u7EC4\u4EF6" };
  const name2 = `${widget.source.connection}__${widget.source.tool}`;
  if (!tools.hasTool(name2)) return { forbidden: true };
  if (!tools.isReadOnlyTool(name2)) return { error: "\u8FD9\u4E2A\u6570\u636E\u6E90\u4E0D\u662F\u53EA\u8BFB\u5DE5\u5177\uFF0C\u9762\u677F\u4E0D\u80FD\u7528\u5B83\u53D6\u6570" };
  try {
    return { value: toolValue(await tools.callTool(name2, widget.source.args ?? {})) };
  } catch (error) {
    return { error: error instanceof Error ? error.message : String(error) };
  }
}
async function insertFormRow(apps, tools, request) {
  const widget = findWidget(apps, request);
  const table = widget?.type === "form" ? widget.submit?.table : void 0;
  if (!table) return { error: "\u8FD9\u4E2A\u8868\u5355\u6CA1\u6709\u914D\u7F6E\u63D0\u4EA4\u5230\u8868" };
  const allowed = new Set((widget.fields ?? []).map((field) => field.name));
  const row = {};
  for (const [key, value] of Object.entries(request.values ?? {})) {
    if (allowed.has(key) && value !== "" && value !== void 0) row[key] = value;
  }
  try {
    toolValue(await tools.callTool("app__write", { action: "insert", app: request.app, table, rows: [row] }));
    return { ok: true };
  } catch (error) {
    return { error: error instanceof Error ? error.message : String(error) };
  }
}
async function writeTableRow(apps, tools, request) {
  const widget = findWidget(apps, request);
  const action = widget?.type === "table" && Number.isInteger(request?.action) ? widget.row_actions?.[request.action] : void 0;
  const write2 = action ? rowWriteOf(action) : void 0;
  const table = widget?.source?.table;
  if (!action || !write2 || !table || action.table !== table) return { error: "\u8FD9\u4E2A\u6309\u94AE\u4E0D\u80FD\u6539\u8868" };
  if (!Number.isInteger(request.id) || request.id <= 0) return { error: "\u8FD9\u4E00\u884C\u6CA1\u6709\u7F16\u53F7" };
  try {
    if (write2 === "set") {
      toolValue(await tools.callTool("app__write", { action: "update", app: request.app, table, id: request.id, row: action.set }));
      return { ok: true };
    }
    const args = { action: "delete", app: request.app, table, ids: [request.id] };
    const preview = toolValue(await tools.callTool("app__write", args));
    if (preview?.confirm) toolValue(await tools.callTool("app__write", { ...args, confirm: preview.confirm }));
    return { ok: true };
  } catch (error) {
    return { error: error instanceof Error ? error.message : String(error) };
  }
}
var kbTool = (name2) => `kb__${name2}`;
async function readKBDoc(tools, request) {
  const name2 = kbTool("read");
  if (!tools.hasTool(name2)) return { forbidden: true };
  const int = (value) => Number.isInteger(value) && value > 0 ? value : void 0;
  if (!int(request?.doc)) return { error: "\u6587\u6863\u7F16\u53F7\u4E0D\u5BF9" };
  try {
    return { value: toolValue(await tools.callTool(name2, { doc: request.doc, version: int(request.version), from: int(request.from) ?? 1, to: int(request.to) })) };
  } catch (error) {
    return { error: error instanceof Error ? error.message : String(error) };
  }
}
async function verifyCitations(tools, blocks) {
  const all = [];
  for (const block of blocks) if (block.type === "citations") all.push(...block.items);
  if (!all.length) return { blocks, bad: [], stale: [] };
  let results;
  if (tools?.hasTool(kbTool("verify"))) {
    try {
      const value = toolValue(await tools.callTool(kbTool("verify"), { citations: all.map(({ doc, version, from, to, quote }) => ({ doc, version, from, to, quote })) }));
      if (Array.isArray(value?.results)) results = value.results;
    } catch {
    }
  }
  const bad = [];
  const stale = [];
  let index = 0;
  const out = blocks.map((block) => {
    if (block.type !== "citations") return block;
    return {
      ...block,
      items: block.items.map((item) => {
        const result = results?.find((r) => r?.index === index);
        index++;
        const status = ["ok", "stale", "mismatch", "missing"].includes(result?.status) ? result.status : "unchecked";
        if (status === "mismatch" || status === "missing") bad.push(index);
        if (status === "stale") stale.push(index);
        const near = Number.isInteger(result?.near_skipped) && result.near_skipped > 0 ? result.near_skipped : void 0;
        return { ...item, title: item.title || result?.title || void 0, status, near_skipped: near };
      })
    };
  });
  return { blocks: out, bad, stale };
}
var safeName2 = /^[A-Za-z0-9_-]{1,80}$/;
function sessionFile(dir, app, sessionId) {
  if (!/^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(app) || app.length > 48) throw new Error("\u5FAE\u5E94\u7528\u540D\u4E0D\u5408\u6CD5");
  if (typeof sessionId !== "string" || !sessionId) throw new Error("\u4F1A\u8BDD id \u4E0D\u5408\u6CD5");
  const session = safeName2.test(sessionId) ? sessionId : createHash2("sha256").update(sessionId).digest("hex").slice(0, 32);
  return path7.join(dir, `${app}--${session}.json`);
}
function writeAtomic(dir, file, value, sequence) {
  fs6.mkdirSync(dir, { recursive: true, mode: 448 });
  const temp = `${file}.${process.pid}.${sequence}.tmp`;
  try {
    fs6.writeFileSync(temp, JSON.stringify(value) + "\n", { mode: 384 });
    fs6.renameSync(temp, file);
  } finally {
    if (fs6.existsSync(temp)) fs6.unlinkSync(temp);
  }
}
var BoardStore = class {
  constructor(dir = path7.join(dshHome(), "dsh-ee", "boards")) {
    this.dir = dir;
  }
  sequence = 0;
  file(app, sessionId) {
    return sessionFile(this.dir, app, sessionId);
  }
  read(app, sessionId) {
    try {
      const value = JSON.parse(fs6.readFileSync(this.file(app, sessionId), "utf8"));
      if (value?.app !== app || value?.sessionId !== sessionId || !Array.isArray(value?.blocks)) return void 0;
      return value;
    } catch {
      return void 0;
    }
  }
  write(app, sessionId, blocks) {
    const file = this.file(app, sessionId);
    const board = { app, sessionId, updatedAt: (/* @__PURE__ */ new Date()).toISOString(), blocks };
    writeAtomic(this.dir, file, board, ++this.sequence);
    return board;
  }
};
var maxChecked = 100;
var ChecklistStore = class {
  constructor(dir = path7.join(dshHome(), "dsh-ee", "checklists")) {
    this.dir = dir;
  }
  sequence = 0;
  read(app, sessionId) {
    try {
      const value = JSON.parse(fs6.readFileSync(sessionFile(this.dir, app, sessionId), "utf8"));
      if (value?.app !== app || value?.sessionId !== sessionId || typeof value?.lists !== "object" || !value.lists) return void 0;
      return value;
    } catch {
      return void 0;
    }
  }
  /** 把 done 里的条目勾上、undone 里的取消；返回这个清单现在勾了哪些。 */
  update(app, sessionId, key, done, undone) {
    if (!/^[a-z0-9-]{1,32}:\d{1,2}$/.test(key)) throw new Error("\u6E05\u5355\u7F16\u53F7\u4E0D\u5408\u6CD5");
    const file = sessionFile(this.dir, app, sessionId);
    const current = this.read(app, sessionId) ?? { app, sessionId, updatedAt: "", lists: {} };
    const set = new Set(current.lists[key] ?? []);
    for (const id of done) set.add(id);
    for (const id of undone) set.delete(id);
    const next = [...set].slice(0, maxChecked);
    const value = { app, sessionId, updatedAt: (/* @__PURE__ */ new Date()).toISOString(), lists: { ...current.lists, [key]: next } };
    writeAtomic(this.dir, file, value, ++this.sequence);
    return next;
  }
};
var workbenchToolName = "ee_workbench";
async function runWorkbenchTool(deps, args, exec) {
  if (args?.action !== "board" && args?.action !== "checklist") throw new Error("action \u53EA\u80FD\u662F board \u6216 checklist");
  const app = String(args?.app ?? "");
  const apps = await deps.readApps();
  const target = apps.find((item) => item.name === app);
  if (!target) throw new Error(`\u6CA1\u6709\u5FAE\u5E94\u7528 ${app}\uFF0C\u6216\u5B83\u4E0D\u5728\u4F60\u7684\u53EF\u7528\u8303\u56F4\u5185`);
  if (args.action === "checklist") return runChecklist(deps, target, args, exec);
  if (!target.workbench?.panes.some((pane) => pane.widgets?.some((widget) => widget.type === "board"))) throw new Error(`\u5FAE\u5E94\u7528 ${app} \u7684\u5DE5\u4F5C\u53F0\u6CA1\u6709 AI \u770B\u677F`);
  const checked = checkBoardBlocks(args?.blocks);
  if (!checked.blocks) throw new Error(checked.error);
  const sessionId = exec?.agent?.session?.id ?? deps.boundSession(app);
  if (typeof sessionId !== "string" || !sessionId) throw new Error("\u627E\u4E0D\u5230\u8981\u66F4\u65B0\u770B\u677F\u7684\u4F1A\u8BDD");
  const verified = await verifyCitations(deps.tools, checked.blocks);
  deps.boards.write(app, sessionId, verified.blocks);
  const notes = ["\u5DF2\u66F4\u65B0\u5DE5\u4F5C\u53F0\u770B\u677F"];
  if (verified.bad.length) notes.push(`\u5F15\u7528 ${verified.bad.map((n) => `[${n}]`).join(" ")} \u5BF9\u4E0D\u4E0A\u539F\u6587\uFF1A\u8BF7\u7528 kb__read \u91CD\u65B0\u8BFB\u90A3\u51E0\u884C\uFF0C\u6539\u6B63\u540E\u518D\u5199\u4E00\u6B21\u770B\u677F\uFF0C\u5E76\u5728\u5BF9\u8BDD\u91CC\u5411\u7528\u6237\u66F4\u6B63\u3002`);
  if (verified.stale.length) notes.push(`\u5F15\u7528 ${verified.stale.map((n) => `[${n}]`).join(" ")} \u7684\u6587\u6863\u5DF2\u7ECF\u66F4\u65B0\uFF0C\u8BF7\u8BFB\u6700\u65B0\u7248\u672C\u786E\u8BA4\u7ED3\u8BBA\u6CA1\u6709\u53D8\u3002`);
  return notes.join("\n");
}
function runChecklist(deps, target, args, exec) {
  if (!deps.checklists) throw new Error("\u8FD9\u4E2A\u7248\u672C\u7684\u63A5\u5165\u63D2\u4EF6\u4E0D\u652F\u6301\u6E05\u5355");
  const found = [];
  for (const pane of target.workbench?.panes ?? []) {
    (pane.widgets ?? []).forEach((widget2, index) => {
      if (widget2.type === "checklist") found.push({ key: checklistKey(pane.id, index), widget: widget2 });
    });
  }
  if (!found.length) throw new Error(`\u5FAE\u5E94\u7528 ${target.name} \u7684\u5DE5\u4F5C\u53F0\u6CA1\u6709\u6E05\u5355`);
  const title = typeof args?.title === "string" ? args.title.trim() : "";
  const picked = title ? found.filter((item) => item.widget.title === title) : found;
  if (picked.length !== 1) throw new Error(title ? `\u6CA1\u6709\u6807\u9898\u662F\u300C${title}\u300D\u7684\u6E05\u5355\uFF1B\u53EF\u9009\uFF1A${found.map((item) => `\u300C${item.widget.title}\u300D`).join("")}` : `\u6709 ${found.length} \u4E2A\u6E05\u5355\uFF0C\u8BF7\u7528 title \u6307\u5B9A\uFF1A${found.map((item) => `\u300C${item.widget.title}\u300D`).join("")}`);
  const ids = (value) => Array.isArray(value) ? value.map((x) => String(x).trim()).filter((x) => x && x.length <= 80) : [];
  const done = ids(args?.done);
  const undone = ids(args?.undone);
  if (!done.length && !undone.length) throw new Error("done \u4E0E undone \u81F3\u5C11\u586B\u4E00\u4E2A");
  const { key, widget } = picked[0];
  if (widget.items?.length) {
    const known = widget.items.map((item) => item.id);
    const unknown = [...done, ...undone].filter((id) => !known.includes(id));
    if (unknown.length) throw new Error(`\u6E05\u5355\u300C${widget.title}\u300D\u91CC\u6CA1\u6709\u6761\u76EE ${unknown.join("\u3001")}\uFF1B\u6761\u76EE\u7F16\u53F7\uFF1A${known.join("\u3001")}`);
  }
  const sessionId = exec?.agent?.session?.id ?? deps.boundSession(target.name);
  if (typeof sessionId !== "string" || !sessionId) throw new Error("\u627E\u4E0D\u5230\u8981\u66F4\u65B0\u6E05\u5355\u7684\u4F1A\u8BDD");
  const now = deps.checklists.update(target.name, sessionId, key, done, undone);
  const total = widget.items?.length;
  return total ? `\u5DF2\u66F4\u65B0\u6E05\u5355\u300C${widget.title}\u300D\uFF1A\u5B8C\u6210 ${widget.items.filter((item) => now.includes(item.id)).length}/${total}` : `\u5DF2\u66F4\u65B0\u6E05\u5355\u300C${widget.title}\u300D\uFF1A\u5DF2\u5B8C\u6210 ${now.length} \u9879`;
}
function registerWorkbenchTool(ctx, deps) {
  return ctx.tools.register({
    name: workbenchToolName,
    description: "\u66F4\u65B0\u5F53\u524D\u5FAE\u5E94\u7528\u5DE5\u4F5C\u53F0\uFF1Aaction=board \u628A\u672C\u8F6E\u5904\u7406\u7ED3\u679C\u5199\u5230\u300CAI \u770B\u677F\u300D\uFF08\u6BCF\u6B21\u6574\u4F53\u66FF\u6362\uFF0Ccitations \u5757\u91CC\u7684\u77E5\u8BC6\u5E93\u5F15\u7528\u4F1A\u81EA\u52A8\u6838\u5BF9\uFF09\uFF1Baction=checklist \u52FE\u9009\u6216\u53D6\u6D88\u5DE5\u4F5C\u53F0\u6E05\u5355\u91CC\u7684\u6761\u76EE\uFF08done / undone \u586B\u6761\u76EE\u7F16\u53F7\uFF09\u3002\u4E0D\u4F1A\u6539\u4EFB\u4F55\u4E1A\u52A1\u6570\u636E\u3002",
    parameters: {
      type: "object",
      required: ["action", "app"],
      properties: {
        action: { type: "string", enum: ["board", "checklist"] },
        app: { type: "string", description: "\u5FAE\u5E94\u7528\u540D\uFF0C\u4E5F\u5C31\u662F /\u540D\u5B57 \u91CC\u7684\u540D\u5B57" },
        title: { type: "string", description: "checklist\uFF1A\u6E05\u5355\u7684\u6807\u9898\uFF1B\u5DE5\u4F5C\u53F0\u53EA\u6709\u4E00\u4E2A\u6E05\u5355\u65F6\u53EF\u4EE5\u4E0D\u586B" },
        done: { type: "array", items: { type: "string" }, description: "checklist\uFF1A\u6807\u6210\u5B8C\u6210\u7684\u6761\u76EE\u7F16\u53F7" },
        undone: { type: "array", items: { type: "string" }, description: "checklist\uFF1A\u64A4\u56DE\u5B8C\u6210\u7684\u6761\u76EE\u7F16\u53F7" },
        blocks: {
          type: "array",
          maxItems: 20,
          description: "\u770B\u677F\u5185\u5BB9\uFF0C\u4ECE\u4E0A\u5230\u4E0B\u663E\u793A\uFF1B\u53EA\u5F53\u6587\u5B57\u663E\u793A\uFF0C\u4E0D\u652F\u6301 HTML",
          items: {
            type: "object",
            required: ["type"],
            properties: {
              type: { type: "string", enum: ["text", "list", "table", "citations"] },
              text: { type: "string" },
              items: {
                type: "array",
                description: "list\uFF1A\u6BCF\u9879\u4E00\u6BB5\u6587\u5B57\uFF1Bcitations\uFF1A\u6BCF\u9879\u4E00\u6761\u77E5\u8BC6\u5E93\u5F15\u7528 {doc, version, from, to, quote, title}",
                items: {
                  anyOf: [
                    { type: "string" },
                    {
                      type: "object",
                      required: ["doc", "from", "to", "quote"],
                      properties: {
                        doc: { type: "integer" },
                        version: { type: "integer" },
                        from: { type: "integer" },
                        to: { type: "integer" },
                        quote: { type: "string" },
                        title: { type: "string" }
                      }
                    }
                  ]
                }
              },
              columns: { type: "array", items: { type: "string" } },
              rows: { type: "array", items: { type: "array", items: { type: "string" } } }
            }
          }
        }
      }
    },
    output: { schema: {}, render: (_args, value) => [{ type: "text", text: renderResult(value) }] },
    execute: async (args, exec) => ({ content: [{ type: "text", text: await runWorkbenchTool(deps, args, exec) }] })
  });
}

// src/index.ts
var name = "dsh-ee-connect";
var inject = {
  skills: { required: true },
  tools: { required: true },
  credentials: { required: false },
  commands: { required: false },
  pluginManager: { required: false }
};
function apply(ctx, input) {
  const config = normalizeConfig(input);
  const tokens = new TokenManager(ctx, config);
  const http = new HttpClient(tokens);
  const bridge = new McpBridge(ctx, config, http);
  const writes = new WriteLog();
  bridge.onWrite = (sessionId, record) => writes.record(sessionId, record);
  let refreshSkills;
  const server = new ServerSettingsService(config, tokens, async () => {
    const current = await tokens.resolve();
    if (current?.value) await tokens.syncNpmrc(current.value).catch(() => false);
    await bridge.restart();
    await refreshSkills?.().catch(() => void 0);
  });
  bridge.registerLocal("ee_plugins.upload", (args) => uploadPackage(http, args));
  registerApproval(ctx, bridge);
  ctx.effect(async () => {
    await tokens.importPending();
    const current = await tokens.resolve();
    if (current?.value) await tokens.syncNpmrc(current.value).catch(() => false);
    let skills;
    const commands = registerCommands(ctx, config, tokens, async () => {
      bridge.wake();
      await Promise.all([bridge.sync().catch(() => void 0), skills?.refreshNow()]);
    });
    const appBindings = new AppBindings();
    const boards = new BoardStore();
    const checklists = new ChecklistStore();
    registerDoctor(ctx, config, tokens, http, bridge, appBindings);
    const skillsReady = registerSkills(ctx, config, http).then((loaded) => {
      refreshSkills = loaded.refreshNow;
      return skills = loaded;
    });
    registerBrand(ctx, {
      readBrand: async () => (await skillsReady).readBrand(),
      readApps: async () => (await skillsReady).readApps(),
      readAvailableApps: async () => (await skillsReady).readAvailableApps(),
      setAppSubscription: async (name2, enabled) => {
        const result = await setAppSubscription(http, name2, enabled);
        if (result.ok) await skills?.refreshNow();
        return result;
      },
      readAppBindings: async () => appBindings.list(),
      bindApp: async (name2, sessionId) => {
        if (!(await (await skillsReady).readApps()).some((app) => app.name === name2)) throw new Error("\u76EE\u5F55\u91CC\u6CA1\u6709\u8FD9\u4E2A\u5FAE\u5E94\u7528");
        return appBindings.bind(name2, sessionId);
      },
      unbindApp: async (name2) => appBindings.unbind(name2),
      workbenchData: async (request) => readWidgetData(await (await skillsReady).readApps(), bridge, request),
      insertFormRow: async (request) => insertFormRow(await (await skillsReady).readApps(), bridge, request),
      writeTableRow: async (request) => writeTableRow(await (await skillsReady).readApps(), bridge, request),
      vote: async (request) => {
        const token = await tokens.resolve();
        if (!token) throw new Error("\u6CA1\u6709\u767B\u5F55\u4F01\u4E1A\u8D26\u53F7");
        const r = await fetch(`${config.base}/api/dsh/feedback`, {
          method: "POST",
          headers: { "content-type": "application/json", Authorization: `Bearer ${token.value}` },
          body: JSON.stringify(request),
          signal: AbortSignal.timeout(15e3)
        });
        if (!r.ok) throw new Error(`\u53CD\u9988\u5931\u8D25\uFF1AHTTP ${r.status}`);
      },
      readBoard: async (request) => boards.read(String(request?.app ?? ""), String(request?.sessionId ?? "")) ?? null,
      readChecklists: async (request) => checklists.read(String(request?.app ?? ""), String(request?.sessionId ?? ""))?.lists ?? {},
      setChecklist: async (request) => {
        const app = String(request?.app ?? "");
        const id = String(request?.id ?? "").trim();
        if (!id || id.length > 80) throw new Error("\u6761\u76EE\u7F16\u53F7\u4E0D\u5408\u6CD5");
        if (!(await (await skillsReady).readApps()).some((item) => item.name === app)) throw new Error("\u76EE\u5F55\u91CC\u6CA1\u6709\u8FD9\u4E2A\u5FAE\u5E94\u7528");
        return checklists.update(app, String(request?.sessionId ?? ""), String(request?.key ?? ""), request?.done ? [id] : [], request?.done ? [] : [id]);
      },
      readKBDoc: (request) => readKBDoc(bridge, request),
      turnWrites: async (request) => ({
        base: config.base,
        writes: writes.between(String(request?.sessionId ?? ""), Number(request?.from), request?.to == null ? void 0 : Number(request.to))
      }),
      readOverview: () => readEnterpriseOverview(config, http, bridge).catch((error) => ({ failure: overviewFailure(config, error) })),
      login: () => commands.login(),
      doctor: () => runLocalDoctor(ctx, config, tokens, http, bridge, appBindings),
      serverSettings: async () => server.read(),
      testServer: (request) => server.test(request?.base),
      saveServer: (request) => server.save(request?.base),
      updatePlugin: () => updateConnect(ctx, config, http),
      webLink: (request) => exchangeWebLink(config.base, http, String(request?.url ?? ""))
    });
    const disposeWorkbenchTool = registerWorkbenchTool(ctx, {
      boards,
      checklists,
      readApps: () => skills.readApps(),
      boundSession: (app) => appBindings.list()[app]?.sessionId,
      tools: bridge
    });
    await skillsReady;
    await bridge.start();
    return () => {
      disposeWorkbenchTool();
      bridge.stop();
    };
  });
}
export {
  apply,
  inject,
  name
};
