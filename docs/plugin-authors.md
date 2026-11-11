# 插件作者指南：把 DSH 插件发布到 dsh-ee

> 面向在企业内部分发 DSH 插件的作者。dsh-ee 还在 v0.x，接口和审核规则可能变化；以仓库里的代码和页面为准。

dsh-ee 里有两种插件来源：

- **上游镜像**：插件已经在 npm 上发布。管理员在「上游预载」里选中它，dsh-ee 把插件和依赖闭包镜像进企业 npm 源，不改包内容。
- **企业托管**：内部插件或不能公开的插件。作者把包发布到 dsh-ee 的企业作用域（`@company/...`），经审核后上架。

下面以企业托管为主；上游镜像只需要满足同样的 `package.json` / `dsh.bundle` 要求。

## 1. package.json

最小要求：

```json
{
  "name": "@company/dsh-my-plugin",
  "version": "0.1.0",
  "description": "一句话说明这个插件做什么",
  "license": "MIT",
  "main": "./lib/index.js",
  "files": ["lib", "cordis.patch.yml", "README.md", "LICENSE"],
  "engines": {
    "dsh": ">=0.2.1-0 <0.3.0",
    "node": "^22.19.0 || >=24.9.0"
  },
  "dsh": {
    "bundle": { "patch": "cordis.patch.yml" }
  },
  "dsh-ee": {
    "title": "我的插件",
    "category": "tools",
    "kind": "plugin",
    "icon": "https://example.com/icon.png"
  }
}
```

几个容易漏的点：

- **`version` 必须是严格 semver**（`0.1.0`，不能写 `v0.1`）。发布后同一个版本号永远不能重发，包括被驳回、下架的版本。
- **`files` 必须包含 `cordis.patch.yml` 和构建产物**。npm 安装不会执行 `prepare`，所以 `lib/` 要在 `npm publish` 前 build 好。
- **`engines.dsh` 写你实际验证过的范围**。dsh-ee 会在审核页展示它，DSH 安装时也会用它判断兼容性；没有声明会有警告。
- **`dsh.bundle.patch` 是相对路径，指向包内的组合清单**。路径不能是绝对路径、不能含 `..` 或反斜杠。
- **`dsh-ee` 字段是可选的展示信息**：`title`、`category`（`skin` / `ui` / `agent` / `memory` / `client` / `channel` / `tools` / `fun` / `resource`）、`kind`（`skill` / `connector` / `knowledge` / `plugin`）、`icon`（HTTP(S) 图片）。没有它，市场里会用 `displayName` / `description`。
- **浏览器端插件**再加 `dsh.client`，例如：

```json
"dsh": {
  "bundle": { "patch": "cordis.patch.yml" },
  "client": {
    "platform": "web",
    "inject": ["@deepseek-ai/dsh-client-connection", "@deepseek-ai/dsh-client-ui-slots"]
  }
}
```

只声明 `dsh.client`、没有 `dsh.bundle` 的包不会出现在企业市场的插件索引里。

## 2. cordis.patch.yml

组合清单里引用自己的包名：

```yaml
- insert:
    - id: dsh-my-plugin
      name: '@company/dsh-my-plugin'
      config: {}
```

- `name` 必须和 `package.json` 的 `name` 完全一致；写成旧名字会导致 DSH 在 `node_modules` 里找不到。
- `id` 是 profile 里的行 id，建议和插件名一致；覆盖别人的 id 会替换整行，升级时要保留原有配置。
- 配置项要和插件的 Schema 一致；无效配置会让 profile 加载失败，不只是插件不生效。

## 3. 包内技能（skills/）

如果插件包内带 `SKILL.md`（通常在 `skills/<名字>/SKILL.md`），这些技能由插件运行时注入 DSH，**不会出现在 dsh-ee 能力中心的技能 Tab**，也不能在 dsh-ee 里按技能单独授权、下架或审核。预检会统计并提示这类文件；企业要按技能治理，应把技能单独发布为能力，或等 v0.3 的 skill 包设计。

## 4. 发布前先预检

网页「分享插件 → 网页上传」选完 `.tgz` 后先点 **预检**（接口是 `POST /api/me/packages/check`，请求体就是 `.tgz`）。预检不会写库、不会占用版本号。

预检会返回：

- **自动检查**：`dsh.bundle` / patch 文件、安装脚本、疑似密钥、license、`engines.dsh`、依赖、体积；
- **发布冲突**：包名、版本号、企业作用域、同名上游包、归属、版本占用；
- **结果**：`can_submit`（没有硬冲突，发布接口会接受）、`will_publish`（开放收录模式下会直接上架）、`requires_review`（提交后进入审核）。

有硬冲突时不要提交；有检查失败项时可以先修，也可以仍然提交审核，但版本号会被占用。

## 5. 发布方式

两种都可以，效果一样：

```bash
# 1) 命令行
npm publish --registry https://<dsh-ee>/npm/

# 2) 网页上传
npm pack
# 然后在「分享插件 → 网页上传」里选 .tgz
```

发布前确认：

- 包名在系统设置的企业作用域下（默认 `@company`）；
- `.npmrc` 里配了企业源和可发布令牌（接入指南里生成）；
- 不要在包里放 `.env`、`.npmrc`、私钥或真实 token；自动检查会直接判失败。

## 6. 审核与上架

- 默认「白名单」模式：发布后是 `pending`，管理员通过后才进 packument、tarball 和 `/index/dsh-index.json`。
- 「开放」模式：没有 fail 项的包发布即上架；有 fail 项的仍进待审核。
- 上架后管理员可以设置可用范围（部门 / 角色 / 人）和下发方式（自选 / 自动安装）。
- 版本号被驳回或下架后不能复用；修完请升版本号。

## 7. 本地验证

发布前至少跑一遍：

```bash
# 看包内容
npm pack
tar -tzf company-dsh-my-plugin-0.1.0.tgz

# 在干净的 DSH profile 里装
DSH_HOME=/tmp/dsh-plugin-test dsh plugin --profile web add ./company-dsh-my-plugin-0.1.0.tgz
```

然后确认：

- 插件出现在 profile 里，DSH 能启动；
- 在 DSH 里能看到/调用它注册的工具、命令或界面；
- 卸载、重装、升级（新版本号）都正常；
- 带浏览器端的插件在 Desktop 和 `dsh web` 里各看一次；
- 断网安装一次，确认依赖闭包完整（如果目标环境是内网）。

## 8. 常见失败

| 现象 | 原因 |
|---|---|
| 发布 403 | 包名不在企业作用域 |
| 发布 409 | 版本号已存在（含驳回/下架） |
| 审核页 `bundle` fail | 没有 `dsh.bundle`、没有 `patch` 字段，或 patch 文件没进 tarball |
| `secrets` fail | 包里含 `.env`、`.npmrc`、私钥或疑似 token |
| DSH 里插件不生效 | `cordis.patch.yml` 的 `name` 和包名不一致，或配置不符合 Schema |
| 安装时 Node 版本报错 | `engines.node` 和员工电脑的 Node 不匹配；Desktop 与 CLI 的 Node 版本可能不同 |
| 只能看到界面、没有后端行为 | 浏览器端入口和 host 入口没有同时声明，或 `dsh.client` 的 inject 不完整 |
