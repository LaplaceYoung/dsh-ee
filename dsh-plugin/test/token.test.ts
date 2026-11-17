import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import os from "node:os";
import { test } from "node:test";
import { TokenManager, restrictCredentialFile } from "../src/token.js";
import { normalizeConfig } from "../src/config.js";

function context() {
  const values = new Map<string, string>();
  return {
    values,
    credentials: {
      async resolve() { return values.has("DSHEE_TOKEN") ? { value: values.get("DSHEE_TOKEN")!, source: "store" } : undefined; },
      async set(_ref: string, value: string) { values.set("DSHEE_TOKEN", value); },
      async unset() { values.delete("DSHEE_TOKEN"); },
    },
    get(name: string) { return name === "credentials" ? this.credentials : undefined; },
  };
}

test("pending token imports once and deletes file", async () => {
  const home = fs.mkdtempSync(path.join(os.tmpdir(), "ee-token-"));
  process.env.DSH_HOME = home;
  const ctx = context();
  const config = normalizeConfig({ base: "http://localhost" });
  const tokens = new TokenManager(ctx as any, config);
  fs.mkdirSync(path.dirname(tokens.pendingPath()), { recursive: true });
  fs.writeFileSync(tokens.pendingPath(), "ee_at_test");
  assert.equal(await tokens.importPending(), true);
  assert.equal(ctx.values.get("DSHEE_TOKEN"), "ee_at_test");
  assert.equal(fs.existsSync(tokens.pendingPath()), false);
  assert.equal(await tokens.importPending(), false);
});

test("invalidate only clears the failed token", async () => {
  const ctx = context();
  const tokens = new TokenManager(ctx as any, normalizeConfig({ base: "http://localhost" }));
  await tokens.set("old");
  assert.equal(await tokens.invalidate("other"), false);
  assert.equal(ctx.values.get("DSHEE_TOKEN"), "old");
  assert.equal(await tokens.invalidate("old"), true);
  assert.equal(ctx.values.has("DSHEE_TOKEN"), false);
});

test("凭据库不可用时本地文件与 .env 兜底", async () => {
  const home = fs.mkdtempSync(path.join(os.tmpdir(), "ee-token-fallback-"));
  process.env.DSH_HOME = home;
  const ctx = { get: () => undefined };
  const tokens = new TokenManager(ctx as any, normalizeConfig({ base: "http://localhost" }));
  await tokens.set("ee_at_local");
  assert.equal(fs.readFileSync(tokens.tokenPath(), "utf8").trim(), "ee_at_local");
  assert.equal(fs.statSync(tokens.tokenPath()).mode & 0o777, 0o600);
  assert.match(fs.readFileSync(tokens.envPath(), "utf8"), /DSHEE_TOKEN=ee_at_local/);
  assert.equal((await tokens.resolve())?.value, "ee_at_local");
  await tokens.unset("ee_at_local");
  assert.equal(await tokens.resolve(), undefined);
  assert.equal(fs.existsSync(tokens.tokenPath()), false);
  assert.doesNotMatch(fs.readFileSync(tokens.envPath(), "utf8"), /DSHEE_TOKEN/);
});

test("unset 不动别人的 .env 令牌", async () => {
  const home = fs.mkdtempSync(path.join(os.tmpdir(), "ee-env-"));
  process.env.DSH_HOME = home;
  fs.writeFileSync(path.join(home, ".env"), "DSHEE_TOKEN=ci-token\nOTHER=1\n");
  const tokens = new TokenManager({ get: () => undefined } as any, normalizeConfig({ base: "http://localhost" }));
  await tokens.unset("different");
  assert.match(fs.readFileSync(tokens.envPath(), "utf8"), /DSHEE_TOKEN=ci-token/);
  await tokens.unset("ci-token");
  assert.doesNotMatch(fs.readFileSync(tokens.envPath(), "utf8"), /DSHEE_TOKEN/);
});

test("npmrc 写进 profile 目录：保留其它行，替换令牌，补上作用域，不碰 ~/.npmrc", async () => {
  const home = fs.mkdtempSync(path.join(os.tmpdir(), "ee-npmrc-"));
  process.env.HOME = home;
  const profile = path.join(home, ".dsh", "profiles", "desktop");
  fs.mkdirSync(profile, { recursive: true });
  const file = path.join(profile, ".npmrc");
  fs.writeFileSync(file, "registry=https://npm.example\n//localhost/npm/:_authToken=old\n");
  const ctx = { ...context(), get(name: string) { return name === "profileContext" ? { dir: profile } : undefined; } };
  const tokens = new TokenManager(ctx as any, normalizeConfig({ base: "http://localhost", scopes: ["@acme", "@lab"] }));
  assert.equal(await tokens.npmrcMatches("new"), false);
  assert.equal(await tokens.syncNpmrc("new"), true);
  assert.equal(fs.readFileSync(file, "utf8"),
    "registry=https://npm.example\n@acme:registry=http://localhost/npm/\n@lab:registry=http://localhost/npm/\n//localhost/npm/:_authToken=new\n");
  assert.equal(fs.statSync(file).mode & 0o777, 0o600);
  assert.equal(await tokens.npmrcMatches("new"), true);
  assert.equal(await tokens.syncNpmrc("new"), false, "内容没变不应重写");
  assert.equal(fs.existsSync(path.join(home, ".npmrc")), false);
});

test("凭据库可用时令牌只进凭据库，不写明文文件和 .env", async () => {
  const home = fs.mkdtempSync(path.join(os.tmpdir(), "ee-token-store-"));
  process.env.DSH_HOME = home;
  const ctx = context();
  const tokens = new TokenManager(ctx as any, normalizeConfig({ base: "http://localhost" }));
  fs.mkdirSync(path.dirname(tokens.tokenPath()), { recursive: true });
  fs.writeFileSync(tokens.tokenPath(), "ee_at_stale\n");
  await tokens.set("ee_at_store");
  assert.equal(ctx.values.get("DSHEE_TOKEN"), "ee_at_store");
  assert.equal(fs.existsSync(tokens.tokenPath()), false, "旧的兜底文件应删掉");
  assert.equal(fs.existsSync(tokens.envPath()), false, "不应写 .env");
  assert.equal((await tokens.resolve())?.value, "ee_at_store");
});

test("凭据库写入失败时才落本地文件与 .env", async () => {
  const home = fs.mkdtempSync(path.join(os.tmpdir(), "ee-token-broken-"));
  process.env.DSH_HOME = home;
  const ctx = { get: () => ({ async set() { throw new Error("locked"); }, async resolve() { return undefined; }, async unset() {} }) };
  const tokens = new TokenManager(ctx as any, normalizeConfig({ base: "http://localhost" }));
  await tokens.set("ee_at_fallback");
  assert.equal((await tokens.resolve())?.value, "ee_at_fallback");
  assert.match(fs.readFileSync(tokens.envPath(), "utf8"), /DSHEE_TOKEN=ee_at_fallback/);
});

// ---- 凭据文件权限加固：mode 只对新建文件生效，已存在的 0644 必须显式 chmod ----

function permHome(prefix: string): string {
  const home = fs.mkdtempSync(path.join(os.tmpdir(), prefix));
  process.env.DSH_HOME = home;
  return home;
}

function modeOf(file: string): number {
  return fs.statSync(file).mode & 0o777;
}

function chmod(file: string, mode: number): void {
  fs.writeFileSync(file, fs.readFileSync(file));
  fs.chmodSync(file, mode);
  assert.equal(modeOf(file), mode, "测试前置：先把文件摆成目标权限");
}

test("已存在的 0644 .env 写入令牌后收紧到 0600", async () => {
  const home = permHome("ee-env-perm-");
  const file = path.join(home, ".env");
  fs.writeFileSync(file, "OTHER=1\n");
  chmod(file, 0o644);
  const tokens = new TokenManager({ get: () => undefined } as any, normalizeConfig({ base: "http://localhost" }));
  await tokens.set("ee_at_perm");
  assert.equal(modeOf(file), 0o600);
  assert.match(fs.readFileSync(file, "utf8"), /DSHEE_TOKEN=ee_at_perm/);
  assert.match(fs.readFileSync(file, "utf8"), /OTHER=1/, ".env 里别人的行要保留");
});

test("内容已含同一令牌的历史 0644 .env 也会收紧（不重写内容也要 chmod）", async () => {
  const home = permHome("ee-env-perm-same-");
  const file = path.join(home, ".env");
  fs.writeFileSync(file, "OTHER=1\nDSHEE_TOKEN=ee_at_same\n");
  chmod(file, 0o644);
  const tokens = new TokenManager({ get: () => undefined } as any, normalizeConfig({ base: "http://localhost" }));
  await tokens.set("ee_at_same");
  assert.equal(modeOf(file), 0o600);
  assert.equal(fs.readFileSync(file, "utf8"), "OTHER=1\nDSHEE_TOKEN=ee_at_same\n", "内容没变就不该重写");
});

test("已存在的 0644 兜底 token 文件写入后收紧到 0600", async () => {
  const home = permHome("ee-token-perm-");
  const tokens = new TokenManager({ get: () => undefined } as any, normalizeConfig({ base: "http://localhost" }));
  fs.mkdirSync(path.dirname(tokens.tokenPath()), { recursive: true });
  fs.writeFileSync(tokens.tokenPath(), "ee_at_old\n");
  chmod(tokens.tokenPath(), 0o644);
  await tokens.set("ee_at_new");
  assert.equal(fs.readFileSync(tokens.tokenPath(), "utf8"), "ee_at_new\n");
  assert.equal(modeOf(tokens.tokenPath()), 0o600);
});

test("新建 .env 仍是 0600，凭证目录 0700", async () => {
  permHome("ee-env-new-perm-");
  const tokens = new TokenManager({ get: () => undefined } as any, normalizeConfig({ base: "http://localhost" }));
  await tokens.set("ee_at_fresh");
  assert.equal(modeOf(tokens.envPath()), 0o600);
  assert.equal(modeOf(tokens.tokenPath()), 0o600);
  assert.equal(modeOf(path.dirname(tokens.tokenPath())), 0o700);
});

test("stripEnvToken 删掉令牌后 .env 仍保持 0600", async () => {
  const home = permHome("ee-env-strip-perm-");
  const file = path.join(home, ".env");
  fs.writeFileSync(file, "DSHEE_TOKEN=ee_at_strip\nOTHER=1\n");
  chmod(file, 0o644);
  const tokens = new TokenManager({ get: () => undefined } as any, normalizeConfig({ base: "http://localhost" }));
  await tokens.unset("ee_at_strip");
  assert.doesNotMatch(fs.readFileSync(file, "utf8"), /DSHEE_TOKEN/);
  assert.match(fs.readFileSync(file, "utf8"), /OTHER=1/);
  assert.equal(modeOf(file), 0o600);
});

test("token 写入只动凭据文件，非凭据文件权限不变", async () => {
  const home = permHome("ee-noncred-perm-");
  const notes = path.join(home, "notes.json");
  fs.writeFileSync(notes, "{}\n");
  chmod(notes, 0o644);
  const tokens = new TokenManager({ get: () => undefined } as any, normalizeConfig({ base: "http://localhost" }));
  await tokens.set("ee_at_scope");
  assert.equal(modeOf(notes), 0o644);
});

test("已存在的 0644 .npmrc 同步令牌后也收紧到 0600（内容没变同样收紧）", async () => {
  const home = permHome("ee-npmrc-perm-");
  const profile = path.join(home, ".dsh", "profiles", "desktop");
  fs.mkdirSync(profile, { recursive: true });
  const file = path.join(profile, ".npmrc");
  fs.writeFileSync(file, "//localhost/npm/:_authToken=ee_at_npm\n");
  chmod(file, 0o644);
  const ctx = { get: (name: string) => (name === "profileContext" ? { dir: profile } : undefined) };
  const tokens = new TokenManager(ctx as any, normalizeConfig({ base: "http://localhost" }));
  assert.equal(await tokens.syncNpmrc("ee_at_npm"), false, "内容没变不应重写");
  assert.equal(modeOf(file), 0o600, "内容没变也要收紧历史权限");
});

test("chmod 平台行为：Windows 跳过，POSIX 收紧，失败不抛错", () => {
  const home = permHome("ee-chmod-platform-");
  const file = path.join(home, "credential");
  fs.writeFileSync(file, "secret\n");
  chmod(file, 0o640);
  assert.equal(restrictCredentialFile(file, "win32"), false);
  assert.equal(modeOf(file), 0o640, "Windows 没有 POSIX 位，不做 chmod");
  assert.equal(restrictCredentialFile(file, "linux"), true);
  assert.equal(modeOf(file), 0o600);
  assert.equal(restrictCredentialFile(path.join(home, "missing"), "linux"), false, "chmod 失败不应该抛错");
});
