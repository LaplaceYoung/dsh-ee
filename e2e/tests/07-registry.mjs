// npm 源全流程，用真实的 npm 命令行：
// 接入指南建令牌 → npm publish 到企业作用域 → 待审核时装不上 → 管理员在审核页通过 →
// npm install（依赖从假的上游回源）→ 能力市场 / DSH 索引可见 → 能力管理下架 → 再装失败
import { execFile } from "node:child_process";
import crypto from "node:crypto";
import fs from "node:fs";
import http from "node:http";
import os from "node:os";
import path from "node:path";
import zlib from "node:zlib";
import { promisify } from "node:util";
import { ADMIN_PW, BASE, api, assert, assertEq, login, newPage, shot, step, tag } from "../lib.mjs";

const run = promisify(execFile);

// 最小的 tar 写入器：npm pack 格式（package/ 目录）
function tgz(files) {
  const blocks = [];
  for (const [name, body] of Object.entries(files)) {
    const data = Buffer.from(body);
    const h = Buffer.alloc(512);
    h.write(`package/${name}`, 0);
    h.write("0000644\0", 100);
    h.write("0000000\0", 108);
    h.write("0000000\0", 116);
    h.write(data.length.toString(8).padStart(11, "0") + "\0", 124);
    h.write("00000000000\0", 136);
    h.write("        ", 148);
    h.write("0", 156);
    h.write("ustar\0" + "00", 257);
    let sum = 0;
    for (const b of h) sum += b;
    h.write(sum.toString(8).padStart(6, "0") + "\0 ", 148);
    blocks.push(h, data, Buffer.alloc((512 - (data.length % 512)) % 512));
  }
  blocks.push(Buffer.alloc(1024));
  return zlib.gzipSync(Buffer.concat(blocks));
}

// 假的上游 registry：只有一个依赖包
function fakeUpstream(depName) {
  const pkg = { name: depName, version: "1.2.0", main: "index.js", license: "MIT" };
  const body = tgz({ "package.json": JSON.stringify(pkg), "index.js": "module.exports = 42;\n" });
  const integrity = "sha512-" + crypto.createHash("sha512").update(body).digest("base64");
  const shasum = crypto.createHash("sha1").update(body).digest("hex");
  const hits = { packument: 0, tarball: 0 };
  const server = http.createServer((req, res) => {
    const url = decodeURIComponent(req.url);
    if (url === `/${depName}`) {
      hits.packument++;
      const port = server.address().port;
      res.setHeader("Content-Type", "application/json");
      return res.end(JSON.stringify({
        name: depName, "dist-tags": { latest: "1.2.0" }, time: { "1.2.0": "2026-01-01T00:00:00Z" },
        versions: { "1.2.0": { ...pkg, dist: { integrity, shasum, tarball: `http://127.0.0.1:${port}/${depName}/-/${depName}-1.2.0.tgz` } } },
      }));
    }
    if (url === `/${depName}/-/${depName}-1.2.0.tgz`) {
      hits.tarball++;
      return res.end(body);
    }
    res.statusCode = 404;
    res.end("{}");
  });
  return { server, hits };
}

async function npm(dir, ...args) {
  try {
    const r = await run("npm", args, { cwd: dir, env: { ...process.env, npm_config_userconfig: path.join(dir, ".npmrc"), npm_config_cache: path.join(dir, ".npm-cache"), npm_config_update_notifier: "false", npm_config_audit: "false", npm_config_fund: "false" } });
    return { ok: true, out: r.stdout + r.stderr };
  } catch (e) {
    return { ok: false, out: (e.stdout ?? "") + (e.stderr ?? "") };
  }
}

export default async function (t) {
  const depName = `left-pad-e2e-${tag}`;
  const { server, hits } = fakeUpstream(depName);
  await new Promise((r) => server.listen(0, "127.0.0.1", r));
  const tmp = fs.mkdtempSync(path.join(os.tmpdir(), "dsh-ee-e2e-"));
  const page = await newPage(t);
  await login(page, "admin", ADMIN_PW);
  const original = (await api(page, "GET", "/api/settings")).data;

  try {
    const s = structuredClone(original);
    s.scope = "@acme";
    s.admissionMode = "allowlist";
    s.upstream.mode = "online";
    s.upstream.registry = "custom";
    s.upstream.customRegistry = `http://127.0.0.1:${server.address().port}`;
    s.registry.proxyMode = "cache";
    s.registry.requireToken = true;
    assertEq((await api(page, "PUT", "/api/admin/settings", s)).status, 200, "设置 npm 源");
    step("主作用域 @acme，按需回源到假的上游");

    // 接入指南：建一个可发布令牌，页面只显示一次
    await page.goto(`${BASE}/setup`);
    await page.getByPlaceholder("例如「我的笔记本」").fill(`e2e-${tag}`);
    await page.getByRole("button", { name: "读取 + 发布" }).click();
    await page.getByRole("button", { name: "新建令牌" }).click();
    const tokenBox = page.locator("pre").filter({ hasText: /^\s*ee_npm_\S+\s*$/ }).first();
    await tokenBox.waitFor();
    const token = (await tokenBox.innerText()).trim();
    await page.getByRole("button", { name: "手动配置", exact: true }).click();
    assert((await page.locator("pre").filter({ hasText: "_authToken=" }).innerText()).includes(token), "手动 npmrc 里应自动填上新令牌");
    step("接入指南创建令牌：" + token.slice(0, 14) + "…");
    await shot(page, "registry-setup");

    const registry = `${BASE}/npm/`;
    const npmrc = `registry=${registry}\n${registry.replace(/^https?:/, "")}:_authToken=${token}\n`;
    const pub = path.join(tmp, "pkg");
    const app = path.join(tmp, "app");
    fs.mkdirSync(pub);
    fs.mkdirSync(app);
    fs.writeFileSync(path.join(pub, ".npmrc"), npmrc);
    fs.writeFileSync(path.join(app, ".npmrc"), npmrc);
    const name = `@acme/e2e-${tag}`;
    fs.writeFileSync(path.join(pub, "package.json"), JSON.stringify({
      name, version: "1.0.0", description: "端到端测试插件", license: "MIT",
      dsh: { bundle: { patch: "cordis.patch.yml" } },
      "dsh-ee": { title: `E2E 插件 ${tag}`, category: "tools", kind: "skill" },
      dependencies: { [depName]: "^1.0.0" },
    }));
    fs.writeFileSync(path.join(pub, "cordis.patch.yml"), "plugins: {}\n");
    fs.writeFileSync(path.join(app, "package.json"), JSON.stringify({ name: "app", version: "1.0.0", private: true }));

    const who = await npm(pub, "whoami");
    assert(who.ok && who.out.includes("admin"), "npm whoami：" + who.out);
    const published = await npm(pub, "publish");
    assert(published.ok, "npm publish 失败：" + published.out);
    assert(published.out.includes("pending review"), "发布后应提示待审核：" + published.out);
    step("npm publish 成功，提示待审核");

    const early = await npm(app, "install", name);
    assert(!early.ok && /404|E404/.test(early.out), "待审核时不应能安装：" + early.out);
    step("待审核时 npm install 返回 404");

    await page.goto(`${BASE}/admin/review`);
    await page.getByText(`${name}@1.0.0`).first().click();
    await page.getByText("声明了 dsh.bundle").waitFor();
    await shot(page, "registry-review");
    await page.getByRole("button", { name: "通过并上架" }).click();
    await page.getByText(`已通过：${name}@1.0.0 已上架`).waitFor();
    step("审核页显示自动检查，点「通过并上架」");

    const installed = await npm(app, "install", name);
    assert(installed.ok, "审核通过后安装失败：" + installed.out);
    assert(fs.existsSync(path.join(app, "node_modules", depName, "index.js")), "依赖没有装上");
    assert(hits.tarball === 1, `依赖 tarball 应只从上游取一次，实际 ${hits.tarball} 次`);
    step(`npm install 成功，依赖 ${depName} 经 dsh-ee 回源`);

    await page.goto(`${BASE}/hub?tab=plugin`);
    await page.getByText(`E2E 插件 ${tag}`).waitFor();
    await page.getByText(`E2E 插件 ${tag}`).click();
    await page.getByText(`dsh plugin --profile web add ${name}`).waitFor();
    await shot(page, "registry-detail");
    const index = await fetch(`${BASE}/index/dsh-index.json`).then((r) => r.json());
    assert(index.format === 1 && index.plugins.some((p) => p.name === name), "DSH 索引里没有新插件");
    step("能力市场、详情页与 DSH 索引都能看到");

    await page.goto(`${BASE}/admin/plugins`);
    await page.getByPlaceholder("搜索…").fill(name);
    const row = page.locator("tr", { hasText: name }).first();
    await row.getByRole("button", { name: "下架" }).click();
    await page.getByPlaceholder("下架原因（必填，会记录在版本上）").fill("e2e 下架测试");
    await page.locator("tr").getByRole("button", { name: "下架" }).last().click();
    await row.getByText("已下架").waitFor();
    step("能力管理里下架");
    const gone = await npm(path.join(tmp, "app"), "view", name, "version");
    assert(!gone.ok, "下架后 npm view 应失败：" + gone.out);
    step("下架后 npm view 返回 404");
  } finally {
    await api(page, "PUT", "/api/admin/settings", original);
    server.close();
    fs.rmSync(tmp, { recursive: true, force: true });
  }
}
