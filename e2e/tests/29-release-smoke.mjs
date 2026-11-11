// 发布前冒烟：不依赖具体页面文案，检查一个全新部署最先被用户碰到的三个入口：
// /healthz、/setup.sh + /setup.ps1、接入插件 tarball。
import { ADMIN_PW, BASE, api, assert, assertEq, login, newPage, step } from "../lib.mjs";

export default async function (t) {
  const health = await fetch(`${BASE}/healthz`);
  assertEq(health.status, 200, "/healthz");
  const healthBody = await health.json();
  assertEq(healthBody.status, "ok", "/healthz 状态");
  step(`/healthz：${healthBody.version ? healthBody.version : "ok"}`);

  for (const [path, marker] of [
    ["/setup.sh", "dsh-ee 接入脚本"],
    ["/setup.ps1", "dsh-ee 接入脚本"],
  ]) {
    const response = await fetch(BASE + path);
    assertEq(response.status, 200, path);
    const text = await response.text();
    assert(text.length > 1000 && text.includes(marker), `${path} 内容不完整（${text.length} 字节）`);
    step(`${path}：${text.length} 字节，含「${marker}」`);
  }

  const admin = await newPage(t);
  await login(admin, "admin", ADMIN_PW);
  assertEq((await api(admin, "POST", "/api/admin/connect/publish", { force: false })).status, 200, "发布接入插件");
  const connect = await api(admin, "GET", "/api/connect");
  assertEq(connect.status, 200, "/api/connect");
  const url = connect.data.tarball_url;
  assert(url, "接入插件应给出 tarball_url：" + JSON.stringify(connect.data));

  const tarball = await fetch(url);
  assertEq(tarball.status, 200, "下载接入插件");
  const body = new Uint8Array(await tarball.arrayBuffer());
  assert(body.length > 1000, "接入插件 tarball 太小：" + body.length);
  assert(body[0] === 0x1f && body[1] === 0x8b, "接入插件应是 gzip tarball");
  step(`接入插件 tarball：${(body.length / 1024).toFixed(1)} KiB，gzip 头正确`);
}
