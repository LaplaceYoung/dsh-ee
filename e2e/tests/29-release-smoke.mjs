// 发布前冒烟：不依赖具体页面文案，检查一个全新部署最先被用户碰到的三个入口：
// /healthz、/setup.sh + /setup.ps1、接入插件 tarball。
import { ADMIN_PW, BASE, api, assert, assertEq, login, newPage, step } from "../lib.mjs";

export default async function (t) {
  const health = await fetch(`${BASE}/healthz`);
  assertEq(health.status, 200, "/healthz");
  const healthBody = await health.json();
  assertEq(healthBody.status, "ok", "/healthz 状态");
  step(`/healthz：${healthBody.version ? healthBody.version : "ok"}`);

  for (const [path, markers] of [
    // setup.sh 支持 zh/en（?lang= / Accept-Language / 运行端 LANG），默认生成带运行时语言分支的脚本；
    // 用语言中立断言，避免只有中文/只有英文时误报。setup.ps1 目前仍是中文。
    ["/setup.sh", ["dsh-ee 接入脚本", "dsh-ee setup script"]],
    ["/setup.ps1", ["dsh-ee 接入脚本"]],
  ]) {
    const response = await fetch(BASE + path);
    assertEq(response.status, 200, path);
    const text = await response.text();
    const marker = markers.find((m) => text.includes(m));
    assert(text.length > 1000 && marker, `${path} 内容不完整（${text.length} 字节）`);
    step(`${path}：${text.length} 字节，含「${marker}」`);
  }
  // setup.sh 双语显式验证：?lang=en 全脚本无中文；?lang=zh 含中文
  const enSetup = await (await fetch(`${BASE}/setup.sh?lang=en`)).text();
  assert(!/[\u4e00-\u9fff]/.test(enSetup), "/setup.sh?lang=en 不应含中文");
  const zhSetup = await (await fetch(`${BASE}/setup.sh?lang=zh`)).text();
  assert(/[\u4e00-\u9fff]/.test(zhSetup), "/setup.sh?lang=zh 应含中文");
  step("/setup.sh 语言参数：en 无中文、zh 有中文");

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
