# 接入与扩展方案（草案，2026-11-11 第四稿）

> 状态：第 1 批进行中（进度见 §12.1），第 2～5 批的基本部分已经做出来（§12.2）。§13 记了已定的事和还没定的问题，§14 是对照代码做的一次评审（可行性、取舍、风险）。这是对 [roadmap.md](./roadmap.md) 阶段 D / E 的细化，v0.1 的范围冻结不受影响。
>
> 配套文档：[extension-authors.md](./extension-authors.md)（扩展作者手册：选形式、选凭据、工具契约、命令、上线和升级边界）、[harness-integration.md](./harness-integration.md)（非 DSH 的 Harness 怎么接：H1/H2/H3）、[access-control.md](./access-control.md)（可配置的管理角色、扩展字段和访问规则）、[agent-dev.md](./agent-dev.md)（让 Agent 完成扩展和权限配置的开发）。设计文档的先后顺序统一写在本文 §12。
>
> 改稿经过：
> - 第一稿：凭据模式、端侧执行、MCP + 清单。
> - 第二稿：参考 [egoist/mygo-music](https://github.com/egoist/mygo-music)，提出在服务端沙箱里跑第三方 JS。
> - 第三稿：参考 Dify 的插件体系（[dify](https://github.com/langgenius/dify)、[dify-plugin-daemon](https://github.com/langgenius/dify-plugin-daemon)、[dify-plugin-sdks](https://github.com/langgenius/dify-plugin-sdks)、[dify-official-plugins](https://github.com/langgenius/dify-official-plugins)），权重更大，补上企业级的包管理、签名、升级、凭据和 OAuth。
> - 第四稿：按 2026-11-11 的决定，**dsh-ee 不在服务端运行任何第三方代码**，去掉了第二、三稿的 JS 沙箱。不写代码的接入改用「声明式扩展」（§4.1），常用系统做成「内置扩展」（§4.3），要写代码的由企业自己部署成「服务扩展」（§4.2）。Dify 的企业级做法保留。

## 0. 叫法（已定）

dsh-ee 自身的插件叫**扩展**（extension），避开已经指 DSH 插件的「插件」一词（能力中心、`plugin-detail` 页面、插件索引）。清单叫 `dsh-ee.extension.json`。

## 1. 要解决什么

企业的系统各不一样，接不进来，harness（DSH，以后还有 Claude Code、Codex 等）就没有东西可用。现在遇到的几类情况：

- 企业系统要按人鉴权，需要员工自己的 JWT 或 token，不能用一个公共账号。
- 一些老系统没有接口，只能模拟浏览器取数，而且往往要用员工自己的登录态、只能在内网访问。
- 企业已经有知识库（飞书文档、Confluence、网盘、各机构的资料库），希望直接接进来，不想再导一遍。
- 以后要支持更多 harness。
- 要接一个系统，现在只能自己写一个 MCP 服务再部署，门槛高，生态起不来。

现状（`server/internal/api/hub.go` 的 `connAuth`、`connections.go`）：连接有 `mcp` 和 `openapi` 两种。OpenAPI 连接已经是 dsh-ee 读接口文档、自己发请求（`server/internal/openapi`）。凭据只有 `none / bearer / header`，**而且是管理员配的一份共享凭据**。每个工具能单独开关、限定角色。知识库只能上传文件。

### 1.1 做成之后企业得到什么

| 企业得到的 | 现在 | 做成之后 |
|---|---|---|
| 接一个内部系统的成本 | 自己写 MCP 服务、部署、运维 | 有 OpenAPI 文档的系统：写一份清单，不部署任何东西，Agent 半小时内能做完（[agent-dev.md](./agent-dev.md)） |
| Agent 以谁的身份办事 | 所有人共用管理员配的一个账号，源系统分不清是谁 | 以员工本人的身份调用，权限仍由源系统判定，离职自动失效 |
| 凭据放在哪 | 散在每个员工的 Agent 配置里，或一份共享凭据 | 只在 dsh-ee 一处，按人加密；能看到谁授权了什么，可以一键收回 |
| 出了事能不能查 | 只能查到「共享账号做了什么」 | 每次调用记到人、扩展版本、工具、凭据模式 |
| 已有的知识库 | 重新上传一遍 | 直接挂上，可以只挑一部分，也可以不复制、按员工权限实时查 |
| 换 Agent | 每换一个都要重接一遍 | 接一次，DSH、Claude Code、Codex 都能用 |

对产品来说，这是 dsh-ee 和别的方案拉开差别的地方：

- **不和 Dify 比工作流和应用编排**，dsh-ee 不跑 AI。dsh-ee 管的是 Agent 和企业系统之间那一层：身份、凭据、权限、写操作确认、审计。Dify 的凭据是工作区共用的（§2.3），「按员工本人的身份」正是它没做的。
- **扩展、知识、应用一起分发**（§6.5）：装一个扩展，员工当天就多一个能用的应用。这是能力中心和「办事的第一入口」（roadmap 远期目标）能长起来的前提。
- **接入可以交给 Agent 做**，企业不需要专门的开发人员，个人维护的开源项目也不需要一个实施团队。

衡量办法：接一个有 OpenAPI 文档的系统要多久（目标：Agent 30 分钟内做到测试通过）；月度报告里经 Agent 办成的事覆盖了多少个系统；用员工身份的调用占全部调用的比例。

## 2. 参考了什么

### 2.1 Dify（权重更大）

Dify 的插件由单独的服务 `dify-plugin-daemon`（Go）管理，插件用 Python 写，分工具、模型、回调地址、Agent 策略、数据源、触发器六类。下表是读源码确认过的做法：

| Dify 的做法（出处） | 对 dsh-ee 的意义 | 采用 |
|---|---|---|
| **包有确定的身份**：`作者/名字:版本@校验和`；调用时带 `X-Plugin-Unique-Identifier`，钉在调用方看到的版本上（`api/services/plugin/dispatch_identity.py`） | 审计、回滚、排查都能精确到哪个版本 | ✅ §5.1 |
| **签名校验**：默认强制官方签名，可以加第三方公钥（`FORCE_VERIFYING_SIGNATURE`、`THIRD_PARTY_SIGNATURE_VERIFICATION_PUBLIC_KEYS`），命令行有 `signature sign / verify` | 企业可以规定只认自己签过的扩展 | ✅ §9.3 |
| **谁能装、谁能调试**：「所有人 / 管理员 / 没人」，默认管理员能装、没人能调试（`TenantPluginInstallPermission`、`TenantPluginDebugPermission`） | 照搬 | ✅ §9.1 |
| **自动升级策略**：关闭 / 只升补丁版本（默认）/ 升到最新；可按名单包含或排除，指定每天几点（`TenantPluginAutoUpgradeStrategy`） | 声明式扩展和端侧扩展用 | ✅ §9.4 |
| **运行方式**：本机子进程、远程调试（插件在开发者电脑上跑，主动连到线上 daemon，用调试密钥认证）、Serverless（按 SRI 接口跑在 Lambda 之类的平台上，`docs/runtime/sri.md`） | dsh-ee 不跑第三方代码，只取后两种的思路：**服务扩展**就是「扩展跑在别处，dsh-ee 按协议调它」；**远程调试**和**反向连接**都是「扩展主动连 dsh-ee」 | ✅ §4.2、§10 |
| **清单声明权限**：`resource.permission` 列出插件要反过来调 Dify 的哪些能力（工具、模型、应用、存储及大小上限） | 扩展反过来调 dsh-ee（知识库、审批、审计回报）也要申请 | ✅ §5.3 |
| **凭据配置用表单声明**：`credentials_schema`，类型有 `secret-input`、`text-input`、`select`、`boolean`、`app-selector` 等；保存前调 `_validate_credentials` | 配错了当场就知道 | ✅ §7.1 |
| **同一插件存多份凭据**，各自命名，有一份默认（`tool_builtin_providers` 按「工作区 + 插件 + 名字」唯一，带 `is_default`、`expires_at`） | 测试 / 生产、多个子公司 | ✅ §7.1 |
| **OAuth 钩子**：插件实现取授权地址、换凭据、刷新、撤销；平台负责跳转和存储，按 `expires_at` 刷新；客户端分系统级和工作区级 | 飞书、钉钉的流程都不完全标准 | ✅ §7.3 |
| **工具参数分 `llm` 和 `form`**：一个由模型填，一个由搭应用的人固定；选项可以动态取（`_fetch_parameter_options`） | 应用把「项目编号」之类固定住，模型改不了 | ✅ §6.1 |
| **数据源三种**：在线文档（`get_pages` / `get_content`）、在线网盘（`browse_files` / `download_file`）、网页抓取；用户浏览后勾选要导入的 | 知识源要能挑范围 | ✅ §6.2 |
| **回调地址（endpoint）**：每次安装生成唯一路径，接第三方 webhook | 飞书事件、审批结果回调 | ✅ §6.3 |
| **触发器**：插件管外部系统上的订阅，事件分发给工作流 | dsh-ee 不跑 AI，事件变成待办或通知 | ⏳ 之后 |
| **应用依赖插件**：导出应用时记下用到的插件，导入时提示先装（`dependencies_analysis.py`）；多个插件可以打成 bundle | 示例应用、分享的应用 | ✅ §6.5 |
| 清单里的文字用 `{en_US, zh_Hans}` 多语言对象 | 照用，写成 `{zh, en}` | ✅ |

### 2.2 mygo-music

服务端跑 JS 这一条不采用，其他保留：

| mygo-music 的做法 | 采用 |
|---|---|
| 实现了哪些函数就有哪项能力，没实现的宿主就不显示 | ✅ SDK 按实现了哪些处理函数自动生成清单里的能力列表，§10 |
| 凭据由宿主按模板填进请求头，插件代码碰不到 | ✅ 声明式扩展里完全做到；服务扩展里只给短期令牌，§7.4 |
| 带 `code` 的错误，宿主据此提示授权、提示去设置 | ✅ §6.6 |
| `host.webSignIn`：独立窗口登录，取走指定的 cookie | ✅ 端侧 TS SDK 的一个辅助函数，§8 |
| 从 GitHub 仓库安装；仓库加 topic 就上官网列表页；安装深链 | ✅ §9.2、§9.5 |
| 一个用假数据把所有能力做一遍的示例，「复制它就能开始」 | ✅ §10 |
| 宿主内嵌 JS 运行时，插件不用部署 | ❌ 已定不采用 |

### 2.3 dsh-ee 和 Dify 刻意不同的地方

| 问题 | Dify | dsh-ee |
|---|---|---|
| 凭据归谁 | 工作区级，员工共用（读源码确认，没有按员工存的凭据） | 共享凭据照 Dify 管，**再加上按员工的凭据**（§7）。这是 dsh-ee 要解决的核心问题 |
| 插件代码在哪跑 | Dify 自己的 daemon 拉起 Python 进程，安装时按 `requirements` 下载依赖（所以有 `PIP_MIRROR_URL` 等配置） | **dsh-ee 不运行第三方代码**。声明式扩展没有代码；服务扩展由企业自己部署；内置扩展是 dsh-ee 自己的代码。内网安装不用联网下载依赖 |
| 安全靠什么 | 签名 + 市场审核；进程没有网络、文件沙箱 | 签名（照 Dify）+ 第三方代码根本不进 dsh-ee 的进程；凭据只给短期令牌 |
| 要不要单独部署一个服务 | 要（daemon，带数据库、Redis、存储） | 不要。dsh-ee 还是单个二进制 |
| 分发 | 官方市场（带审核）+ GitHub + 上传包 | Git 仓库 + 上传 + 企业目录 + topic 列表页，不做带审核的市场 |

## 3. 结论

1. **扩展点只开在企业之间差别大的地方**（§6），RBAC、审计、网关、管理后台页面都不做成扩展。
2. **数据接不进来，根本原因在两件事**：调用时用谁的身份（§7），调用在哪里执行（§8）。这两件做进核心，由 dsh-ee 强制执行。
3. **dsh-ee 不在服务端运行任何第三方代码。** 扩展只有三种形式（§4）：
   - **声明式扩展**：只有清单和接口文档，dsh-ee 自己发请求。覆盖「给一个内部接口包一层」这种最常见的需求，不用写代码，不用部署。
   - **服务扩展**：企业自己部署的服务，dsh-ee 按协议调它。需要写逻辑的（知识源、不标准的 OAuth、回调、拼接多个接口）都走这个。官方提供 Go SDK，企业也可以用任何语言按协议自己实现。
   - **内置扩展**：飞书这类大多数企业都要的，直接写在 dsh-ee 里，实现同样的接口，在扩展列表里标「内置」。

   另外还有**端侧扩展**（§8），是员工本机上跑的服务扩展。

   对 harness 来说几种没有区别，都通过 dsh-ee 的 `/mcp` 暴露出去，工具名还是 `连接名__工具名`。
4. **企业级的做法照 Dify 来**：包身份、签名、谁能装、升级策略、多份凭据、OAuth 钩子、应用依赖。
5. **生态靠这几样**：官方 SDK 和一个什么都做了一遍的示例扩展、远程调试、按 GitHub topic 自动生成的列表页、应用依赖提示。不做带后端的市场。

## 4. 三种扩展

### 4.1 声明式扩展

一个文件夹（或一个仓库）：

```
acme-hr/
  dsh-ee.extension.json
  openapi.yaml
  apps/hr-helper.yaml   # 可选
  README.md
```

- 工具来自 `openapi.yaml`，也就是现在的 OpenAPI 连接。清单在接口文档之外补上：凭据模式、配置表单、哪些接口是写操作、参数由谁填（§6.1）、返回结果要保留哪些字段（控制返回给模型的大小）。
- **dsh-ee 自己发请求**，请求只能发往清单里列出的服务器地址。凭据由 dsh-ee 加到请求头上，没有任何第三方代码经手。
- 这是现有 OpenAPI 连接的升级版，不需要新的执行引擎。
- 做不了的：多步调用、需要加工的返回、知识源、回调。这些用服务扩展。

### 4.2 服务扩展

企业自己部署的一个 HTTP 服务（容器、虚拟机、函数计算都行），dsh-ee 按协议调它：

- **工具走 MCP**，就是现在的 MCP 连接。
- **工具之外的能力**（配置检查、动态选项、知识源、OAuth 钩子、回调、健康检查）走同一个服务上的几个约定接口（§6），用 JSON 收发。SDK 把这些都包好了。
- **清单由服务自己提供**：`GET <服务地址>/.well-known/dsh-ee-extension.json`。管理员装的时候只要填服务地址。
- **dsh-ee 调扩展时要证明自己**：每个请求都带一个 dsh-ee 签的短期 JWT（和 §7.2 的 `ee_jwt` 是同一套），`aud` 是清单里的 `id`，`sub` 是当前员工。SDK 用 dsh-ee 的 JWKS 验签，所以扩展不用另外配密钥，也天然知道是哪个员工在调。
- **扩展调 dsh-ee**（查知识库、发起审批、回报审计）用安装时发给它的扩展令牌，只能做清单里申请过的事（§5.3）。
- **扩展出问题不能拖累 dsh-ee**：每次调用有超时（沿用现有连接的超时和 `connResultLimit` 结果上限）；一个扩展连续失败就暂时熔断，期间它的工具直接返回 `unavailable`，其他扩展和 dsh-ee 自身不受影响。
- **反向连接**（有需求再做，照 Dify 远程调试的思路）：扩展所在的网络 dsh-ee 访问不到时（比如分支机构的内网），由扩展主动连到 dsh-ee，调用从这条长连接上走。远程调试（§10）用的就是这个机制。服务端现在没有 WebSocket 依赖，实现时在 WebSocket 和「长轮询 + POST」之间选，后者只用标准库。

### 4.3 内置扩展

- 飞书、钉钉、企业微信、Confluence 这类大多数企业都要接的，写在 dsh-ee 里（Go），实现和服务扩展**同一套内部接口**，只是不走网络。
- 在扩展列表里显示为「内置」，配置、凭据、审计、可见范围的管理和别的扩展完全一样；不用的企业可以停用。
- 每个扩展点都有一个内置实现，这也是 roadmap 阶段 E「每个扩展点要有内置实现」的要求。
- 内置扩展跟着 dsh-ee 一起升级，不走 §9.4 的升级策略。

### 4.4 怎么选

| 情况 | 用 |
|---|---|
| 内部系统有 REST 接口，有或者能导出 OpenAPI 文档 | 声明式 |
| 已经有现成的 MCP 服务 | 服务扩展（加一份清单） |
| 要多步调用、加工结果、做知识源、接回调、OAuth 流程不标准 | 服务扩展（用 SDK 写） |
| 飞书、钉钉等常见系统 | 内置 |
| 要员工本机的登录态、只能在员工电脑上访问、要模拟浏览器 | 端侧扩展（§8） |

## 5. 清单、身份和权限

### 5.1 身份和版本

- 扩展的身份是 `作者/名字`，比如 `acme/oa`；具体版本是 `acme/oa@1.2.0`，再加清单（和声明式扩展的接口文档）的 sha256。
- 声明式扩展的清单和接口文档装进来之后存在 dsh-ee 里，之后的重启、回滚不再去网上取。
- 服务扩展的代码在企业手里，dsh-ee 管不了它什么时候升级。所以 dsh-ee 在每次健康检查时重新取清单，一旦发现变化就记下新版本；**新增的工具、写操作或权限先不启用，等管理员确认**（§9.4）。
- 每次调用、每条审计都记录扩展的版本和清单的哈希。
- **工具前缀**：安装时定一个「连接名」（默认取 `id` 里的名字，比如 `oa`，管理员可以改，全局唯一），工具对外叫 `连接名__工具名`。`id` 里的 `/` 不进工具名；连接名定了之后不随升级变化，已经用了这些工具的应用不受影响。

### 5.2 清单

```json
{
  "id": "acme/oa",
  "version": "1.2.0",
  "minVersion": "0.2.0",
  "name": { "zh": "Acme OA", "en": "Acme OA" },
  "description": { "zh": "查询和提交 OA 审批单" },
  "icon": "icon.svg",
  "repo": "https://github.com/acme/dsh-ee-oa",
  "kind": "service",
  "service": { "url": "https://oa-ext.acme.internal" },
  "capabilities": ["tools", "validate", "options", "knowledge.documents", "endpoint"],
  "permissions": { "kb": "read", "approval": true },
  "config": [
    { "key": "baseUrl", "type": "url", "label": { "zh": "OA 地址" }, "required": true },
    { "key": "tenant", "type": "select", "label": { "zh": "租户" }, "options": "dynamic" }
  ],
  "credential": { "mode": "ee_jwt" },
  "tools": [
    {
      "name": "list_forms",
      "description": { "zh": "列出当前员工待办的审批单" },
      "readOnly": true,
      "params": [
        { "name": "status", "type": "string", "enum": ["pending", "done"], "fill": "model" },
        { "name": "form_type", "type": "string", "fill": "app", "label": { "zh": "单据类型" }, "options": "dynamic" }
      ]
    },
    {
      "name": "submit_leave",
      "description": { "zh": "提交请假单" },
      "params": [
        { "name": "days", "type": "number", "required": true, "fill": "model" },
        { "name": "reason", "type": "string", "fill": "model" }
      ]
    }
  ]
}
```

- `kind`：`declarative`（配 `"openapi": "openapi.yaml"` 和 `"servers": [...]`）、`service`、`local`（端侧，配 `"package": "@acme/oa-local"`）。内置扩展不需要清单文件。
- `capabilities`：服务扩展实现了哪些约定接口。用 SDK 写的扩展会自动生成，不用手写（照 mygo-music「实现了什么就有什么」）。
- `config`：管理员填的配置，字段类型参照 Dify 的 `credentials_schema`：`text`、`secret`、`url`、`select`、`boolean`、`kb`（选一个知识库）、`app`（选一个应用）。`"options": "dynamic"` 表示选项由扩展的 `options` 接口给出。
- `tools`：服务扩展的工具同时在 MCP 里有定义；清单里的这份是给管理员看、给权限和审批用的。两边不一致时以清单为准，并在健康检查里报警。声明式扩展的 `tools` 写对 OpenAPI 接口的补充（`readOnly`、`fill`、保留字段）。
- **`readOnly`**：只读的工具才标 `"readOnly": true`，**没标的一律当写操作**（§7.5）。这和现有连接的 `ReadOnly` 字段是同一个意思：MCP 工具取 `readOnlyHint`，OpenAPI 接口只有 GET 算只读，写操作导入时默认不启用。漏标的后果是多确认一次，不会是少确认一次。
- `fields`：扩展要加的扩展字段和建议的访问规则，比如项目系统的扩展带一个「项目」字段。安装时管理员确认，字段名带扩展前缀。详见 [access-control.md](./access-control.md) §6.0。
- 显示给人看的字段可以写字符串，也可以写 `{zh, en}`。

### 5.3 扩展反过来调 dsh-ee 的权限

照 Dify 的反向调用权限，扩展用扩展令牌调 dsh-ee 时，只能做清单 `permissions` 里申请过的事，安装时逐项给管理员看：

| 权限 | 能做什么 |
|---|---|
| `kb: "read"` | 以当前员工的身份检索和读取知识库（只能看到这个员工有权限的） |
| `approval` | 发起写操作审批（端侧扩展必须有，§8） |
| `audit` | 回报调用摘要（端侧扩展必须有） |
| `notify` | 给员工发通知或待办（之后） |
| `endpoint` | 注册回调地址（§6.3） |

扩展令牌每个安装一个，可以在管理后台随时吊销、重新生成。

## 6. 扩展点

服务扩展里，每个扩展点是同一服务下的一个约定接口（`POST <服务地址>/ee/<名字>`），用 SDK 时就是注册一个处理函数。内置扩展实现同样的 Go 接口。

### 6.1 连接器（工具）

- 工具本身走 MCP，参数分三种（照 Dify 的 `llm` / `form`），用 `fill` 标明：
  - `model`：由模型填，出现在暴露给 harness 的工具定义里。
  - `app`：由搭应用的人在应用配置里选好。**不出现在工具定义里，模型看不到也改不了**，由 dsh-ee 在调用时补上。比如「只能查 A 项目」的应用，就把项目编号固定住。
  - `admin`：由管理员在扩展配置里统一设置。
- 返回：MCP 的内容类型（文本、图片、资源链接）。扩展返回的文件由 dsh-ee 存下来再给一个下载地址。返回太大时由 dsh-ee 截断（现有的 `connResultLimit`）。
- 约定接口：
  - `validate`：管理员保存配置或共享凭据前调用，配错了当场提示。照 Dify 的 `_validate_credentials`。
  - `options`：给 `"options": "dynamic"` 的字段提供选项，比如授权之后列出可选的飞书空间。照 Dify 的 `_fetch_parameter_options`。
  - `health`：「检查」按钮和定时健康检查；同时返回当前清单的哈希，dsh-ee 据此发现版本变化。

### 6.2 知识源

照 Dify 的数据源分成两种，外加一种 dsh-ee 自己的联邦检索：

| 类型 | 约定接口 | 用法 |
|---|---|---|
| 文档库（飞书文档、Confluence、Notion） | `kb/browse`（`parent` → 空间和页面的树）、`kb/changes`（`scope`、`cursor` → 增量）、`kb/document`（`id` → 正文，Markdown 或纯文本） | 同步 |
| 网盘（企业网盘、对象存储、共享目录） | `kb/browse`、`kb/changes`、`kb/download`（`id` → 文件，交给 dsh-ee 已有的 PDF / Word 解析） | 同步 |
| 联邦检索（大型知识库、权限必须留在源系统的） | `kb/search`（`query`、`limit` → 结果列表）、`kb/document` | 不复制，查询时实时去查 |

- **挑范围**（照 Dify 的「浏览后勾选」）：管理员把知识源挂到知识库时，dsh-ee 用 `kb/browse` 列出空间、文件夹，管理员勾选要同步的部分；之后只同步勾选的范围。
- **同步**：dsh-ee 定时拉取，用 `cursor` 做增量，内容进 dsh-ee 的知识库，之后照常用 `kb__search` / `kb__read`。同步时没有员工，用共享凭据。**同步过来的内容不带源系统里的权限**：谁能看由 dsh-ee 的库级权限和扩展字段规则决定（[access-control.md](./access-control.md)），所以同步适合「这个范围内的人本来都能看」的内容；源系统里按人、按页面授权的内容，用联邦检索。
- **联邦检索**：员工调 `kb__search` 时，dsh-ee 同时去查这些知识源，结果和本地结果合在一起，标明来源。用的是这个员工的凭据，所以员工只能看到自己在源系统里有权限的内容。每个来源有时间上限（默认 3 秒，可调），超时或报错不影响其他来源，只在返回里注明「某某来源暂时不可用」。联邦检索的结果不参与 dsh-ee 自己的排序打分，按来源分组排在本地结果后面，各来源内部保持源系统给的顺序。
- 现有的文件上传保留，作为内置的「本地文件」知识源。

### 6.3 回调地址

照 Dify 的 endpoint：

- 扩展申请 `endpoint` 权限后，每次安装由 dsh-ee 生成一个不可猜的地址 `https://<dsh-ee>/x/<随机串>/…`，管理后台显示完整地址，管理员复制到飞书开放平台、审批系统这类地方。
- dsh-ee 收到请求后原样转给扩展的 `endpoint` 接口，再把响应原样返回。好处是第三方只需要能访问 dsh-ee，不需要能访问扩展所在的服务。
- 回调没有员工身份；验签（比如飞书的加密和签名校验）由扩展自己做。
- 回调地址是不需要登录就能访问的入口，所以要有限制：请求体大小上限、按地址限流、只转发到这个扩展自己的服务地址；扩展停用或卸载后地址立即失效。

### 6.4 其他扩展点（有人要才做）

| 扩展点 | 约定接口 | dsh-ee 强制的底线 |
|---|---|---|
| 事件（照 Dify 触发器） | `events/subscribe`、`events/unsubscribe`、`events/refresh`；事件经回调地址进来，扩展转成 `{ to, title, body, link }` | dsh-ee 不跑 AI：事件只变成员工工作台上的待办或通知，不自动触发 Agent |
| 通知渠道 | `notify` | 失败时退回站内通知 |
| 组织同步 | `org/changes`，可以带扩展字段（等级、项目等） | 只能增改，删除用户要管理员确认；只能写清单里声明过的字段 |
| 权限策略 | `authorize/batch` → 每项 `allow` / `deny` | 只能在内置 RBAC 和字段规则允许的基础上再收紧；超时或出错一律拒绝（沿用 roadmap 阶段 E） |
| 字段填充 | `fields/suggest` → 建议的字段值和理由 | 只给建议；对用于访问控制的字段，更宽松的建议永远要人确认 |
| 审计投递 | `audit/deliver` | 按顺序推送，失败重试不丢 |

可配置的管理角色和扩展字段（比如文档等级）的完整设计在 [access-control.md](./access-control.md)。原则是判断和拦截一律在核心里做，扩展只提供数据和建议。
| Agent 适配器 | `agent/render`、`agent/config` | 生成的东西由 dsh-ee 下发 |

Dify 的模型插件和 Agent 策略插件不做：dsh-ee 不自己跑 AI（roadmap 已定），模型接入走现有的 `/v1` 网关。

### 6.5 扩展和应用

- **扩展附带应用**：扩展可以带应用（人设 + 技能 + 提示词 + 示例问题），安装时管理员选要不要一起导入。装上 OA 扩展，得到的不只是几个工具，还有一个能直接用的「OA 助手」。
- **应用声明依赖**（照 Dify 的 `dependencies_analysis`）：应用用到了扩展的工具，导出时自动记下 `"requires": ["acme/oa@^1.2"]`。导入应用时如果缺扩展、或版本不够，就提示管理员先装（有安装来源就直接给安装按钮）。一键导入示例内容时也走这个检查。
- **组合**（照 Dify 的 bundle）：先不做专门的格式，企业目录（§9.2）里可以把几个扩展和应用列成一组，一起装。

工具、知识、应用一起分发，扩展才真正成为 dsh-ee 生态的一部分。

### 6.6 错误码

扩展返回带 `code` 的错误，dsh-ee 把它转成统一的、模型能看懂的工具结果：

| `code` | 含义 | dsh-ee 怎么处理 |
|---|---|---|
| `authorize` | 这个员工还没授权或授权过期了 | 返回「请先授权：<链接>」，指向「我的连接」里的授权按钮。凭据模式是 `oauth` / `user_secret` 时，dsh-ee 在调用前就会检查 |
| `setup` | 管理员没配好 | 告诉员工「请联系管理员」，同时提醒管理员 |
| `forbidden` | 源系统说这个员工没权限 | 原样告诉员工，不重试 |
| `unavailable` | 源系统暂时不可用 | 联邦检索时跳过这个来源 |
| 其他 | | 显示错误信息，写进扩展日志 |

## 7. 用谁的身份：凭据（核心）

### 7.1 凭据的两层

- **共享凭据**（照 Dify）：管理员配。同一个扩展可以存**多份**，各自命名（比如「生产」「测试」「华东子公司」），有一份默认；应用可以指定用哪一份。保存前调扩展的 `validate`。

  实现进度：[named-shared-credentials.md](./named-shared-credentials.md) 已落地服务端连接的多份凭据、原默认兼容、切换与审计。按应用指定凭据及声明式扩展的 `validate`，尚不在该批次。
- **员工凭据**（dsh-ee 比 Dify 多的部分）：每个员工自己的，在「我的连接」里管理。

### 7.2 凭据模式

`credential.mode` 决定员工调用时带什么：

| 模式 | 适用 | 调用时怎么带 |
|---|---|---|
| `shared` | 只读报表、公共数据、知识同步、回调 | 共享凭据 |
| `user_secret` | 员工各有 API token 的系统 | 员工在「我的连接」里填，按人加密存放（复用 `server/internal/secret`） |
| `ee_jwt` | 企业自研系统，或愿意加一层适配的系统 | dsh-ee 为这次调用签一个短期 JWT，对方验签就知道是谁 |
| `oauth` | 飞书、钉钉、Confluence 这类 SaaS | 员工授权一次，dsh-ee 按人保存令牌，到期前刷新 |

一个扩展可以同时用共享凭据和员工凭据（比如服务账号的 API Key 加员工的 `ee_jwt`），用两个请求头分开放。

**`ee_jwt` 最先做：**

- dsh-ee 在 `/.well-known/jwks.json` 公开签名公钥。私钥用主密钥加密后存在数据库里（复用 `server/internal/secret`），多个副本共用；轮换时新旧公钥同时公开一段时间，JWT 头里的 `kid` 标明用的是哪一把。服务端现在没有 JWT 依赖，签发只需要标准库的 `crypto`，不必引入新的库。
- 每次调用签一个 JWT：`iss` 是 dsh-ee 的地址，`aud` 是对方系统的标识（连接里填的 `audience`，没填就是连接标识；服务扩展固定用清单 `id`），`sub` 和 `email` 标识员工，`dept` 和 `roles` 按管理员的设置带上，`exp` 是 5 分钟，再加一个 `jti`。
- 企业那边：系统支持 JWKS 验签就直接用；不支持就写一层很薄的适配，验签之后换成系统自己的会话。
- 服务扩展收到的也是这个 JWT（§4.2），SDK 已经验过签，处理函数直接拿到员工是谁。
- 好处：dsh-ee 不用存员工密码；权限仍由源系统判定。

### 7.3 OAuth

- **OAuth 客户端**（client_id / secret）：照 Dify 分两种来源。扩展作者可以在企业目录里提供预置的客户端（相当于 Dify 的系统级），企业也可以填自己在飞书、钉钉开放平台上建的应用（相当于工作区级）。大多数企业会用自己的。
- **授权流程**：
  - 标准 OAuth 2.0：清单里写授权地址、令牌地址、scope，dsh-ee 自己完成跳转、换令牌、刷新。
  - 不标准的（飞书要先换应用令牌再换用户令牌，钉钉也有自己的流程）：扩展实现 `oauth/authorize-url`、`oauth/exchange`、`oauth/refresh`、`oauth/revoke` 四个接口，照 Dify 的钩子。dsh-ee 负责回调地址、state 校验、存储、按 `expiresAt` 刷新。飞书、钉钉是内置扩展，这些逻辑直接写在 dsh-ee 里。
- 员工离职或被停用时，dsh-ee 先调撤销，再删掉本地存的令牌。
- 现在 OIDC 登录（`server/internal/oidc`）只用 ID Token 认人，不存令牌；转发 IdP 令牌（RFC 8693 token exchange）等有企业要求再做。

### 7.4 凭据交给谁

- **声明式扩展和内置扩展**：凭据只在 dsh-ee 里，由 dsh-ee 加到请求头上，请求只能发往清单里的服务器地址，重定向到别的域名时不带。没有任何第三方代码经手。
- **服务扩展**：扩展要替员工去调源系统，必须拿到凭据。原则是**只给这一次调用要用的短期凭据**：
  - `ee_jwt`：本来就是短期的。
  - `oauth`：只给 access token，refresh token 和 client secret 一直留在 dsh-ee 里。
  - `user_secret` / `shared`：只能原样给（这类凭据本身没有短期形式），安装时提示管理员，能换成 `ee_jwt` 或 `oauth` 就换。
  - 凭据放在 dsh-ee 调扩展的请求头里，只在这一次请求里有效，SDK 不落盘、不打日志。
- 服务扩展是企业自己部署的，信任来自部署本身；dsh-ee 能做的是让它拿到的东西尽量少、尽量短命。

### 7.5 写操作要员工确认

现状：写操作的确认在 DSH 客户端做。工具带着「是不是只读」的标记下发，DSH 遇到非只读的工具会弹审批卡；没有审批通道的无头模式下直接不执行。**服务端自己并不拦截**，这对 DSH 够用，但对其他 Agent 不够：Claude Code、Codex 可以被设成自动同意。

要做的：

- 没标 `readOnly` 的工具一律当写操作（§5.2）。`dsh-ee extension check` 反过来提醒：标了 `readOnly`、但名字像写操作（`submit_`、`create_`、`delete_` 等开头）或 OpenAPI 方法不是 GET 的工具。
- **服务端确认**：调用方不是 DSH（或管理员要求一律服务端确认）时，写操作不直接执行，dsh-ee 返回「需要你确认：<链接>」，员工在网页或飞书卡片上确认后才把调用转给扩展。这就是 roadmap 阶段 E 里说的「L1 的主要安全工作」，和扩展一起做。
- DSH 里仍然用审批卡，体验不变。

### 7.6 底线

- 员工凭据只用于这个员工自己的调用；管理员能看到「谁授权了哪个扩展」，看不到凭据内容。
- 员工离职或被停用时，按人存的凭据一起删掉，OAuth 先撤销。
- 审计记录写明扩展、版本、清单哈希、工具、凭据模式（共享的还要记是哪一份）、耗时和成败，不记返回内容。

## 8. 端侧扩展

> 第 5 批。动手前要先在真机上验证 §13 待定 2、3。

| 运行位置 | 适合 | 调用路径 |
|---|---|---|
| 服务端（声明式、服务、内置） | 有接口、能用 §7 任一模式的系统 | harness → dsh-ee `/mcp` → 扩展 |
| 端侧（员工本机） | 必须用员工的登录态、只能在内网访问、要模拟浏览器的系统 | harness → 本机的扩展 → 系统 |

- 端侧扩展就是跑在员工电脑上的服务扩展：一个 npm 包，用端侧 TS SDK 写（协议和服务扩展相同），经企业 npm 源和能力中心下发，按授权范围决定谁能装。DSH 由接入插件写入本机 MCP 配置；其他 harness 由适配器生成配置片段。这是员工电脑上的事，dsh-ee 服务端仍然不运行第三方代码。
- 端侧 SDK 提供 `webSignIn({ url, cookiesOf, until })`（照 mygo-music）：在一个单独的窗口里打开系统自己的登录页，员工照常登录（包括扫码、短信验证），等指定的 cookie 都出现后关掉窗口，cookie 存在本机。**大部分「没有接口的老系统」靠这个加上发 HTTP 请求就能解决**，用不着整套浏览器自动化。需要浏览器自动化的，再用 Playwright 连员工自己的浏览器。
- 审计：本机调用不经过服务端，SDK 把调用摘要（扩展、版本、工具、耗时、成败，不含返回内容）回报给 dsh-ee。**这是一个弱保证**，见 §13 问题 2。
- 写操作确认：SDK 在执行没标 `readOnly` 的工具前，先调 dsh-ee 的确认接口，员工确认之后才执行。
- 端侧扩展的包要过签名策略（§9.3），升级走升级策略（§9.4）。
- 端侧扩展的开发者文档见 [extension-local.md](./extension-local.md)；SDK 用法见 [`extensions/sdk-local/README.md`](../extensions/sdk-local/README.md)，要开始做可以复制 [`extensions/official/legacy-local`](../extensions/official/legacy-local)。

## 9. 安装、签名和升级

> 第 1、2 批只做其中的基本部分：谁能装（§9.1）、从服务地址 / Git 仓库 / 上传安装（§9.2）、安装时给管理员看清楚再确认、清单变化时新增部分先不启用（§9.4）。签名做了最小闭环（§9.3）：命令行签名 / 验签、安装计划显示签名状态、可选强制签名；企业目录 v1 做了「管理员贴入整体签过名的目录 JSON → 验签 + 逐条校验 → 登记来源」，只登记不安装、不自动升级（见 `docs/plugin-signing.md` §8），v1.1 加了只读更新检查（`GET /api/admin/directory/updates`：登记条目 vs 已装扩展 / 企业源已上架插件版本）和条目可选 `auto_upgrade` / 设置 `directory.autoUpgrade`（只登记 / 展示，不执行）；受信任公钥的管理页面已做，自动升级执行、从 URL 拉取目录 / 缓存、列表页和安装按钮（§9.5）仍是设计，有人要才做（§12），完整的设计选项见 #130 的 `docs/plugin-signing.md`（设计稿）。

### 9.1 谁能装、谁能调试

照 Dify，在「设置 → 扩展」里：

- **谁能安装**：超级管理员 / 有「扩展管理」权限的角色 / 没人（锁定，只能用已装的）。默认超级管理员。
- **谁能远程调试**（§10）：同样三档，默认没人。打开后，调试中的扩展只对调试者本人可见。

### 9.2 从哪里装

管理后台「扩展」页面的「+」：

- **服务地址**：填服务扩展的地址，dsh-ee 去取 `/.well-known/dsh-ee-extension.json`。
- **Git 仓库**：GitHub 的 `owner/repo` 或地址，可以带子目录（`owner/repo/path/to/ext`）；企业内网的 GitLab / Gitea 地址。声明式扩展直接从仓库取清单和接口文档；服务扩展的仓库里写着怎么部署（官方扩展提供容器镜像），部署好之后再填服务地址。私有仓库用管理员配的访问令牌。
- **上传**：声明式扩展打成 zip 上传，给完全隔离的内网。
- **企业目录**：v1 已实现「管理员把一份整体签过名的 `dsh-ee-directory.json` 贴进管理接口 → dsh-ee 验签 + 逐条校验 → 登记条目」；登记后管理员可对单条显式「安装 / 升级」（扩展按登记的固定 tarball 下载后复用安装计划；插件只认 npm 固定版本、走现有预载 + 审核）。管理入口在「能力与连接 → 企业插件目录」（导入 / 校验 / 登记、看计划再安装 / 升级）。不自动安装、不自动升级；格式、接口和边界见 `docs/plugin-signing.md` §8；v1.1 提供只读更新检查（`GET /api/admin/directory/updates`，对比扩展连接 / 企业源已上架版本，返回 available / current / not_installed / unknown）和条目可选 `auto_upgrade` 声明 / 设置 `directory.autoUpgrade`（默认 false，只登记 / 展示，不执行）。从 URL 自动拉取目录 / 缓存 / 离线包仍是设计。

### 9.3 签名（最小闭环已实现，2026-11-13；默认「警告后允许」）

策略照旧：默认「警告后允许」，因为 dsh-ee 没有官方市场，强制签名会把第三方扩展都挡在外面；企业可以切到「拒绝」。这里做的是最小闭环——命令行签名 / 验签、安装计划显示签名状态、可选强制签名。企业目录、自助分发、回滚、受信任公钥的管理页面等完整设计选项见 #130 的 `docs/plugin-signing.md`（设计稿，未实现部分仍以那份为准）。

**签什么、怎么签**

- 签名对象是 `作者/名字@版本:摘要`：`作者/名字@版本` 是扩展身份，摘要就是 `extension.Digest`（清单的 sha256；声明式扩展连同接口文档，和 `extension check` 打印的 sha256、升级时判断内容有没有变的是同一个口径）。身份和摘要绑在一起，换身份、改内容、把别的扩展的签名挪过来都验不过。
- 算法是 Ed25519（Go 标准库），detached 签名写在扩展目录的 `signature.json`：

  ```json
  {
    "version": 1,
    "alg": "ed25519",
    "key_id": "2ed5a736768c6d8d",
    "identity": "acme/oa@1.2.0",
    "digest": "<清单和接口文档的 sha256>",
    "signature": "<base64>"
  }
  ```

- `key_id` 是签名公钥 sha256 的前 16 位十六进制。私钥只在作者本地（`keygen` 生成，文件权限 0600），**dsh-ee 不生成、不托管、不入库、不上传私钥**；服务端只保存受信任的公钥。
- 命令（都在 dsh-ee 二进制里，不需要数据库）：
  - `dsh-ee extension keygen <私钥文件> [--public 公钥文件]`：生成 Ed25519 密钥对（PEM，私钥 PKCS#8）。也可以用 `openssl genpkey -algorithm ed25519` 生成。
  - `dsh-ee extension sign <目录> --key <私钥文件> [-o 文件] [--json]`：先做 `check`，通过后签名，默认写 `<目录>/signature.json`。
  - `dsh-ee extension verify <目录> [--key <公钥文件>] [--json]`：用受信任的公钥核对。没有签名、签名对不上、签名者不在受信任公钥里都返回退出码 1。
  - `dsh-ee extension pack` 会把 `signature.json` 一起打进 zip，上传后安装计划里就能看到签名状态。
- 受信任的公钥有两处来源，验签时合并（#134）：
  - **库里的信任根**（主要来源）：表 `trusted_signing_keys`，管理员通过 `GET/POST /api/admin/signing-keys` 看和加，`POST /api/admin/signing-keys/{key_id}/retire` 换下，`POST /api/admin/signing-keys/{key_id}/revoke` 撤销；权限是 `connections.manage` 或 `settings.manage`，增 / 换下 / 撤销都记审计（`signing_key.add / retire / revoke`）。库里存的公钥是 PEM（`PUBLIC KEY` 或 `CERTIFICATE`，不收私钥）。换下、撤销都保留记录，不删行。
  - **环境变量**（只读兼容）：`DSH_EE_EXTENSION_TRUSTED_KEYS=<PEM 文件>`，可以放多把（公钥、证书、私钥文件都认）；同一个 key id 以库里的状态为准，所以撤销一把也写在 env 里的公钥会生效。环境的公钥仍然要在启动时读，换它要重启；库里的改动立即生效。
  - 命令行 `verify --key` 和环境变量等价，只是本地核对，不读库。
- 公钥状态：`active`（正常使用）、`retired`（换下：不再用于新签名，历史签名仍认，安装计划里带 `key_state`）、`revoked`（撤销：历史签名显示 `revoked`，不管有没有开强制签名，新装 / 升级都拒绝；不能重新加回来，要恢复信任请加新钥匙）、`pending`（`valid_from` 还没到，验签时先不用）。
- 私钥文件不要放进扩展目录（`pack` 只打清单、接口文档、签名等固定文件，不会把目录里的其他文件打进去）。
- 服务扩展的清单是从对方服务实时取的，目前安装计划里显示 unsigned、也不阻断；服务扩展如何随清单提供签名还没做。所以配了强制签名时，从服务地址安装的服务扩展也会被拦下——这是现在的最小闭环的边界。

**安装时怎么处理**

- 安装计划（`POST /api/admin/extensions/check`、管理后台「安装扩展」、`dsh-ee extension verify`）显示签名状态，分开显示，不合并成一个「可信」：
  - `unsigned`：没有 `signature.json`；
  - `valid`：签名对得上，签名者也在受信任的公钥里（带 `key_state`，换下的公钥也算有效，但会标出来）；
  - `invalid`：有签名但格式不对、内容被改过、或者签名者不受信任（消息里会说是哪一种）；
  - `revoked`：签名对得上，但签名者的公钥已撤销——历史签名仍能核对，但不能用它新装 / 升级（不管有没有开强制签名）。
- 默认只在计划里提示，不阻断安装（「警告后允许」）。
- 可选强制：`DSH_EE_EXTENSION_REQUIRE_SIGNATURE=1` 时，没签名或 `invalid` 的扩展检查不通过、装不了（fail closed）；这时库里或 `DSH_EE_EXTENSION_TRUSTED_KEYS` 至少要有一把受信任公钥。一把都没有时启动只警告、不拒绝启动（管理员可以先起服务再用管理 API 加公钥），否则所有扩展都会被拦下。库里的公钥改完立即生效；环境变量里的公钥在启动时读一次，换它要重启。
- 边界：签名只证明「这段清单 / 接口文档是谁签的、没被换过」，不是安全审计，也不覆盖服务扩展运行时的行为、端侧扩展在本机的执行、npm 插件包；装之前仍要看清单、接口地址、凭据和权限（[plugin-quality.md](./plugin-quality.md)）。受信任公钥通过管理后台「能力与连接 → 扩展签名」页面（`/api/admin/signing-keys`，列表 / 新增 PEM / 换下 / 撤销，环境变量来源只读展示）和 `DSH_EE_EXTENSION_TRUSTED_KEYS` 配置，只有 `settings.manage` 的管理员也能直接打开；官方扩展也还没有用官方密钥签名。

### 9.4 安装、升级和卸载

1. **取清单并检查**：核对校验和（签名校验是 §9.3 的设计，还未实现），检查清单格式；服务扩展再做一次健康检查，确认清单和 MCP 工具一致。
2. **给管理员看**：身份和版本、签名状态（§9.3 的设计，现在没有）、扩展形式、请求会发往哪些地址、凭据模式和会交给扩展什么凭据（§7.4）、申请的反向权限、提供哪些工具、哪些是写操作、附带哪些应用、兼容的版本。
3. 管理员确认后，填配置（保存前调 `validate`），选可见范围（哪些部门、角色能用），启用。
4. **升级**：
   - 声明式扩展和端侧扩展，照 Dify 的升级策略：关闭 / 只升补丁版本（默认）/ 升到最新；可以只对名单里的扩展生效或排除名单里的；可以指定每天几点升。
   - 服务扩展由企业自己升级，dsh-ee 在健康检查时发现清单变了就记下新版本。
   - 不管哪种，**新版本申请了更多权限、多了写操作工具、或者换了凭据模式，新增的部分都先不启用**，等管理员确认（照 mygo-music）。
   - 现状（2026-11-13）：企业目录 v1.1 只登记 / 展示 `auto_upgrade` 声明与 `directory.autoUpgrade` 设置（默认 false），**不执行**上面这些自动升级策略；升级仍要管理员显式触发。企业目录的只读更新检查见 `docs/plugin-signing.md` §8。
5. **回滚（已实现）**：装过的每一版都留在连接的版本历史里（清单 / 接口文档 / 摘要 / 管理员参数 / 服务扩展地址和配置的密文引用，不含明文凭据），管理员可以回到上一版或指定历史版本：`GET /api/admin/extensions/{slug-or-id}/history` 看历史，`POST /api/admin/extensions/{slug-or-id}/rollback`（`{"version": <连接版本号>, "confirm": true}`，不填 `version` 表示上一版；`slug-or-id` 可以是连接标识或扩展 id）执行。回滚复用安装时的检查和差异计算（清单、接口文档、版本兼容、签名按当前信任根重新核对、按差异提示），没存签名材料的旧版本在默认策略下可回滚但会提示，强制验签（`DSH_EE_EXTENSION_REQUIRE_SIGNATURE=1`）时不放行。回滚会先按历史快照把工具取回来（服务扩展连不上时什么都不改），再追加一个新版本并替换工具清单——旧版本不会被改写，回滚本身也可以再回滚，失败时当前版本不受影响。网页入口：打开由扩展装成的连接，在「扩展」卡片点「安装历史」，选中历史版本看回滚计划和敏感项，确认后回滚并刷新工具清单。自动升级策略仍是设计。
6. **卸载**时问管理员：保留数据（以后装回来还能用），还是连共享凭据和员工授权一起删（OAuth 先撤销）；同时吊销扩展令牌。

### 9.5 让扩展能被找到

- 约定给仓库加 topic `dsh-ee-extension`。演示站（或文档站）上的 `/extensions` 页面在构建时用 GitHub 搜索把它们列出来，浏览器里再刷新一次；GitHub 限流时就显示构建时的列表。没有后端，不用审核，不用运维（照 mygo-music 的 `website/src/plugins.ts`）。
- 安装按钮：网页链接 `https://<你的 dsh-ee>/admin/extensions/install?repo=owner/repo`。列表页第一次点安装时问一下你的 dsh-ee 地址，存在浏览器里。打开后进入 §9.4 的确认流程，不会直接装上。
- 内置扩展和官方扩展也列在这个页面上，带「内置」或「官方签名」标记。
- 不做评分、计费和带审核的市场。Dify 的市场要一个团队来审核运营，dsh-ee 没有这个条件；企业要审核，就用企业目录加签名策略自己审。

## 10. 写扩展：SDK、示例和调试

**SDK 分两套，按运行位置选语言**（2026-11-11 已定）：

| SDK | 语言 | 给谁用 | 批次（§12） |
|---|---|---|---|
| 服务端 SDK | **Go**，模块 `github.com/hikariming/dsh-ee/extensions/sdk-go` | 服务扩展 | 第 2 批 |
| 端侧 SDK | **TypeScript**，npm 包 `@dsh-ee/extension-local` | 端侧扩展 | 第 5 批 |

为什么这样分：

- 服务端用 Go：扩展编译出来是单个二进制，企业内网部署不需要装 Node，和 dsh-ee 单文件的风格一致；清单类型、`ee_jwt` 验签、约定接口的请求和响应结构，和 dsh-ee 服务端用的是**同一份代码**。做法是 SDK 是一个独立的 Go 模块，`server/` 反过来依赖它（仓库里用 `go.work` 连起来），这样外部扩展引用 SDK 时不会把整个服务端的依赖带进去。
- 端侧用 TS：端侧扩展要跑在员工电脑上，现有的 DSH 接入插件（`dsh-plugin/`）就是 TS / Node，经 dsh-ee 已有的 npm 源下发；`webSignIn`、Playwright 也都在 Node 这边。
- 两套 SDK 实现的是同一份协议（§4.2、§6），协议文档是唯一标准。企业想用 Python、Java 自己写服务扩展，照协议实现就行；Python SDK 以后有人需要再说。

两套 SDK 都负责：

- 起一个 HTTP 服务（端侧是本机 MCP 服务），同时提供 MCP 和 §6 的约定接口；按注册了哪些处理函数，自动生成清单里的 `capabilities`，并在 `/.well-known/dsh-ee-extension.json` 提供清单。
- 用 dsh-ee 的 JWKS 验证每个请求，把员工身份和本次凭据交给处理函数。
- Go SDK 需要 MCP 的服务端实现。现在 `server/internal/mcp` 只有客户端，dsh-ee 自己的 `/mcp` 入口写在 `api` 包里；做 SDK 时把服务端这部分抽成可复用的包，或者直接用官方的 MCP Go SDK，做的时候比较一下再定。
- 错误码有现成的函数。端侧 SDK 另外提供审批、审计回报和 `webSignIn`。

**示例扩展代替脚手架。** `examples/demo-extension`（Go）用服务端 SDK 和假数据把所有能力都做一遍：模型填和应用填的参数、动态选项、`validate`、一个写工具、文档库知识源（带 `browse`）、联邦检索、回调地址、`ee_jwt` 和 `user_secret` 两种凭据、所有错误码、中英双语、附带一个应用。README 第一句就是「复制它就能开始」。另外再给一个声明式扩展的示例。CI 里把两个示例都装上，把每个能力调一遍。

**命令**（`dsh-ee extension …` 和 `dsh-ee schema …` 都在 dsh-ee 二进制里；SDK 另有各自的本机辅助命令）：

| 命令 | 作用 |
|---|---|
| `dsh-ee extension dev [--listen 地址]` | 起一个**模拟的 dsh-ee**（预置员工、JWKS、假凭据），不需要数据库；扩展用原来的方式启动（`go run .`、`node --watch`），再用 `call` / `test` 调它。扩展热重载和网页调用面板**（设计，未实现）** |
| `dsh-ee extension call <地址> <工具> [--as 员工]` | 以模拟环境里某位员工的身份调一次扩展的工具；扩展启动时的 `DSH_EE_URL` 要指向模拟环境 |
| `go run . dev --remote https://ee.acme.com --key …` | **远程调试**（照 Dify，设计，未实现）：扩展在开发者电脑上跑，主动连到真实的 dsh-ee（走 §4.2 的反向连接），只对开发者本人可见。调试密钥在管理后台生成，要先有 §9.1 的调试权限 |
| `dsh-ee extension init <目录> --id 作者/名字 [--kind declarative\|service]` | 生成扩展模板，改好名字，带上 `AGENTS.md`。端侧扩展没有脚手架，复制 `extensions/official/legacy-local` |
| `dsh-ee extension check <目录> [--json]` | 静态检查：清单、接口文档、清单和 MCP 工具是否一致、`readOnly` 标记是否可疑、知识源接口是否完整。**只收目录**，没有 URL 形式（检查已经在跑的服务用 `test <地址>`） |
| `dsh-ee extension test <目录\|地址>` | 把扩展跑起来，按协议逐项测（[agent-dev.md](./agent-dev.md) §4.2）。全部通过才算合格 |
| `dsh-ee extension pack <目录>` | 声明式扩展打成 zip，算出校验和。服务扩展不用打包，交付的是代码仓库或镜像 |
| `dsh-ee extension report <目录>` | 生成给管理员看的交付说明 |
| `dsh-ee schema list / get` | 查清单、约定接口、错误码的规格 |
| `dsh-ee extension keygen / sign / verify` | 生成签名密钥、签名和校验（§9.3） |

**都放在 dsh-ee 主仓库里，先不另开仓库**（2026-11-11 已定）。协议还在变，SDK、示例、官方扩展和服务端放在一起，改一次协议能在一个 PR 里全部改完、一起跑 CI；维护者只有一个人，少一个仓库就少一套发布流程。等协议稳定（至少 v0.3 之后）再考虑拆出去。目录：

```
extensions/
  sdk-go/              # 服务端 SDK（Go 模块，server/ 也依赖它）
  sdk-local/           # 端侧 SDK @dsh-ee/extension-local（TypeScript，第 5 批）
  examples/
    demo-extension/    # 服务扩展（Go）：用假数据把所有能力做一遍
    demo-declarative/  # 声明式扩展：清单 + OpenAPI
  official/            # 官方扩展，每个覆盖一种难点（照 dify-official-plugins）
    ee-jwt-adapter/    # 1. ee_jwt 适配层样例：验签之后换成系统会话，约 50 行；给企业系统那边用的
    hr-declarative/    # 2. 声明式样例：常见的 HR 或工单系统接口，带 user_secret 和写操作
    legacy-local/      # 3. 端侧老系统模板（TS）：webSignIn 取 cookie 后发 HTTP 请求取数，带写操作
    claude-code/       # 4. Claude Code 适配器：roadmap L2
```

- 飞书不在这里，它是内置扩展，代码在 `server/` 里。
- 从 Git 仓库安装要支持子目录，比如 `hikariming/dsh-ee/extensions/official/hr-declarative`（§9.2）。
- 官方扩展的版本跟着 dsh-ee 的版本走，用 `扩展名/v1.2.0` 这样的 tag 也可以单独发版，先不做。
- `/extensions` 列表页（§9.5）除了按 topic 搜第三方仓库，还要把主仓库 `extensions/official/` 下的官方扩展列进去。
- Go SDK 直接用主仓库的 Go 模块路径引用，打 `extensions/sdk-go/v0.x.y` 这样的 tag 发版；端侧 SDK 从 `extensions/sdk-local` 发到 npm，在那之前示例用本地路径引用。

让 Agent 从头到尾完成扩展开发（配套技能、机器能读的规格、`extension test` 协议一致性测试、开发环境、交付说明）的设计在 [agent-dev.md](./agent-dev.md)。写扩展的作者手册（选形式、选凭据、工具契约、命令、上线和升级边界）见 [extension-authors.md](./extension-authors.md)。非 DSH 的 Harness（Claude Code、Codex、自研 Agent）怎么接见 [harness-integration.md](./harness-integration.md)。

## 11. 现有功能怎么过渡

- 现有的 OpenAPI 连接自动转成声明式扩展，MCP 连接自动转成服务扩展。`id` 用 `local/<连接名>`，原来的共享凭据变成默认的那一份共享凭据，工具开关和角色限制保留。不需要手工迁移。
- 工具名 `连接名__工具名` 不变，已经用了这些工具的应用和员工不受影响。
- 知识库的文件上传保留，作为内置的「本地文件」知识源。

## 12. 先后顺序

三份文档（本文、[access-control.md](./access-control.md)、[agent-dev.md](./agent-dev.md)）加起来的工作量，一个人不可能在一个版本里做完。所以不按「v0.2 全做」来排，而是分成几批，**每一批单独做完都能用、都能发布**，后一批要不要做、先做哪个，看前一批上线后的反馈。

| 批次 | 内容 | 做完之后企业能做什么 | 大致对应 |
|---|---|---|---|
| **第 1 批：按人身份 + 声明式扩展** | `ee_jwt`、`user_secret`、「我的连接」页面；清单 v1；声明式扩展（现有 OpenAPI 连接升级）；参数 `fill`；多份共享凭据；现有连接自动转换；规格文件、`check --json`、声明式示例、`dsh-ee-extension-dev` 技能的声明式部分 | 不部署任何服务，把有接口的内部系统接进来，并且以员工本人的身份调用 | v0.2，和 roadmap 阶段 D 的飞书一起 |
| **第 2 批：服务扩展 + 知识源** | 服务扩展协议、Go SDK、`extension test`、开发用的模拟环境；知识源（先做文档库同步和联邦检索）；`oauth`（标准流程 + 钩子）；回调地址；写操作的服务端确认；应用依赖检查；飞书文档作为内置知识源 | 接需要写逻辑的系统；把已有的知识库挂进来 | v0.3，就是 roadmap 阶段 E 的「插件系统」 |
| **第 3 批：扩展字段 + 访问规则** | [access-control.md](./access-control.md) §4～§5，组织同步带字段，扩展带字段 | 按文档等级、项目控制谁能看 | v0.3 或 v0.4；和第 2 批互不依赖，哪个需求先来就先做哪个 |
| **第 4 批：管理角色 + 配置交给 Agent** | 可关闭的超级管理员、互斥角色、双人批准；配置导出 / `plan` / `test` / `apply`、`explain` / `simulate`；另外两个技能 | 几个管理员各管一块；权限配置交给 Agent 做 | 第 3 批之后 |
| **第 5 批：端侧扩展** | TS SDK、`webSignIn`、本机回报审计、Agent 适配器 | 接没有接口、只能用员工登录态的老系统 | 取决于 §13 待定 2、3 的真机验证 |

### 12.1 第 1 批的进度

| 项 | 状态 | 在哪 |
|---|---|---|
| `ee_jwt`、`user_secret`、「我的连接」 | ✅ | [ee-jwt.md](./ee-jwt.md)、`server/internal/api/user_identity.go` |
| 清单 v1、`extension check --json`、规格文件、声明式示例、技能的声明式部分 | ✅ | `extensions/`、`server/internal/extension` |
| 安装声明式扩展（上传 zip 或清单 + 接口文档 → 检查 → 安装计划 → 装成连接；升级时新增的写操作、只读变写、凭据模式和地址的变化先停用或要确认） | ✅ | `server/internal/api/extensions.go`，管理后台「企业连接 → 安装扩展」 |
| 参数 `fill`：`admin` 由管理员安装时设置，`app` 先取清单的 default；两种都不出现在给模型的工具定义里 | ✅（`app` 按应用固定还没做） | `extension.Tool.Schema` / `Fix` |
| `keep` 只保留返回字段（可写到第二层：`items.id`）、请求只发往清单列出的地址、调用记录带扩展版本和摘要 | ✅ | 同上 |
| `shared` 模式由清单指定共享凭据的请求头（`credential.header`）；管理员填的值按清单的类型和可选值核对；`minVersion` 高于当前版本的不让装；扩展装的连接不能换成另一个扩展或变回手工连接 | ✅ | `server/internal/api/extensions.go` |
| 从 Git 仓库安装（§9.2） | ⏳ | |
| 多份共享凭据（§7.1） | ✅ 连接级选择（按应用指定哪一份还没做） | [named-shared-credentials.md](./named-shared-credentials.md)、`server/internal/api/shared_credentials.go` |
| 现有连接自动转换成扩展（§11） | ⏳ | |

### 12.2 第 2～5 批的进度（2026-11-12）

| 批 | 项 | 状态 | 在哪 |
|---|---|---|---|
| 2 | 服务扩展协议 | ✅ | [extension-protocol.md](./extension-protocol.md) |
| 2 | Go SDK、示例服务扩展 | ✅ | `extensions/sdk-go`、`extensions/examples/demo-extension` |
| 2 | `extension test`（协议一致性 + 场景测试）、模拟环境 `extension dev`、`extension call / pack / report / init` | ✅ | `server/cmd/dsh-ee/cli_ext.go`、`server/internal/extension/exttest` |
| 2 | 从服务地址安装、健康检查、清单变了先不启用、熔断 | ✅ | `server/internal/api/service_ext.go`，网页「企业连接 → 安装扩展 → 填服务地址」，连接详情页的「扩展」 |
| 2 | 知识源：文档库同步（勾选范围、增量）、联邦检索 | ✅ | `server/internal/api/kb_sources.go`，知识库页面「知识源」 |
| 2 | 飞书文档内置知识源 | ✅ 只做同步；源系统里删掉的文档不会自动删，只同步新版文档（docx） | `server/internal/api/kb_feishu_docs.go` |
| 2 | `oauth`：标准流程 + 钩子，员工授权、刷新、撤销 | ✅ | `server/internal/api/ext_oauth.go`，连接详情页「我的凭据」 |
| 2 | 回调地址 `/x/<随机串>/…` | ✅ | `service_ext.go` |
| 2 | 写操作的服务端确认（off / non_dsh / always）和确认页 | ✅ 网页确认；飞书卡片确认没做 | `server/internal/api/write_confirm.go`、`/confirm/<id>`、系统设置 |
| 2 | 应用依赖检查（导出记 `requires`，导入时提示缺扩展、版本不够） | ✅ 只提示，不给安装按钮 | `server/internal/api/app_requires.go` |
| 3 | 扩展字段（文档、知识库、用户）、三种规则、各出口过滤、索引带字段、`explain`、`simulate` | ✅ | `server/internal/access`、`server/internal/api/fields.go`，网页「字段与访问规则」 |
| 3 | 扩展带字段（清单 `fields`），知识源给字段值 | ✅ | 同上 |
| 3 | 组织同步带字段（LDAP 登录时、飞书组织同步时） | ✅ | `server/internal/api/field_sync.go`，「字段与访问规则 → 组织同步」 |
| 4 | 可关闭的超级管理员、互斥角色、不能给自己授权、双人批准、预设 | ✅ | `server/internal/api/admin_policy.go`，「字段与访问规则 → 管理约束」 |
| 4 | 配置导出 / `plan` / `test` / `apply`、变更请求、批准、回滚 | ✅ | `dsh-ee config …`（`server/cmd/dsh-ee/cli_config.go`），「字段与访问规则 → 变更请求」 |
| 4 | `dsh-ee admin recover` | ✅ | 同上 |
| 4 | 配置令牌（`config` 范围）、`extensions.develop` 权限点 | ✅ | `rbac/scopes.go` |
| 4 | `dsh-ee-access-config` 技能；两个技能作为系统技能下发给有对应权限的人 | ✅ | `extensions/skills/`，`server/internal/api/agent_skills.go` |
| 4 | `ee_dev__*` 工具、开发令牌 | ⏳ 按 [agent-dev.md](./agent-dev.md) §10.3「先只做命令行」没做 | |
| 5 | 端侧 TS SDK（本机 MCP、写操作确认、审计回报） | ✅ | `extensions/sdk-local`，服务端 `server/internal/api/local_ext.go` |
| 5 | `webSignIn` | ✅ 用 Playwright 起的浏览器窗口；DSH 插件里开独立窗口读 cookie 没验证（§13 待定 3） | `extensions/sdk-local/src/web-signin.ts` |
| 5 | 端侧老系统模板、Claude Code 适配器 | ✅ | `extensions/official/legacy-local`、`extensions/official/claude-code` |
| 5 | §13 待定 2、3 的真机验证 | ⏳ | |

**有人要才做**（设计留着，不排期）：受信任公钥的策略界面（如自动批准、多环境标记）、自动升级执行、从 URL 自动拉取 / 缓存企业目录、`/extensions` 列表页和安装按钮、反向连接和远程调试、网盘类知识源、事件、通知渠道、权限策略扩展、字段填充、审计投递、更多内置扩展、Python SDK、服务端无头浏览器、token exchange。

**已经做了的最小部分**：签名（§9.3）——`dsh-ee extension keygen / sign / verify`、安装计划显示 `unsigned / valid / invalid / revoked`、`DSH_EE_EXTENSION_REQUIRE_SIGNATURE=1` 强制验证签名，以及信任根落库（`trusted_signing_keys` 表 + `/api/admin/signing-keys` 管理接口，换下 / 撤销保留历史，env 只读兼容）；企业目录 v1（#142）已实现登记来源，v1.1 加了只读更新检查与 `auto_upgrade` 声明 / 设置（只登记 / 展示，不执行，见 `docs/plugin-signing.md` §8）；自动升级执行、自助分发仍是设计（#130 的 `docs/plugin-signing.md`），扩展版本回滚已实现（§9.4）。

这样排的理由：

- 第 1 批改动最小（都在现有的连接和 OpenAPI 代码上改），却解决了最初的那个问题：按人鉴权的系统接不进来。
- 第 1 批不需要 SDK、不需要新的运行方式，万一后面的方向要调整，它也不会白做。
- 签名、升级策略、目录、列表页是「扩展多了之后」才有用的东西，在还没有第三方扩展的时候做是浪费。
- 飞书的登录、组织同步、通知照 roadmap 阶段 D 的安排在 v0.2 做，写成内部接口；等第 2 批的扩展接口定下来，再把它们归到「内置扩展」名下，界面上统一管理。不让飞书等扩展接口。

每一批的完成标准：有一个内置实现、一份协议文档、一个能通过 `check`（第 2 批起还有 `test`）的样例；并且过一次 [agent-dev.md](./agent-dev.md) §9 的验收：只给 Agent 需求和技能，看它能不能做完。

## 13. 已定和待定

**已定（2026-11-11）：**

1. 叫「扩展」。
2. 服务端不运行第三方代码，不做 JS 沙箱。
3. 签名策略默认「警告后允许」。
4. SDK、示例和官方扩展先都放在 dsh-ee 主仓库的 `extensions/` 下，不另开仓库（§10）。
5. 服务端 SDK 用 Go，端侧 SDK 用 TypeScript，见 §10。
6. 清单里只读的工具才标 `readOnly`，没标的一律当写操作（§5.2、§7.5）。
7. 分批做，每一批单独能发布；签名、升级策略、列表页等有人要才做（§12）。

**待定：**

1. **服务扩展能拿到哪些凭据？** §7.4 的方案是「只给短期的，`user_secret` / `shared` 原样给并提示」。有没有企业会要求「凭据一律不出 dsh-ee」？如果有，这类企业只能用声明式和内置扩展。
2. **端侧审计能不能接受「本机回报」这种弱保证？** 如果要求所有调用都必须经过服务端，端侧就要改成「本机做中继，由 dsh-ee 发起调用」，复杂度会高不少。
3. **端侧的 `webSignIn` 放在哪里？** 取决于 DSH 插件能不能开一个独立的登录窗口并读到 cookie，要在 Mac 真机上验证；不能的话，端侧 SDK 里用 Playwright 起一个本机浏览器窗口代替。
4. **`ee_jwt` 里带哪些员工信息？** 建议默认只带 `id`、`name`、`email`，部门、角色、工号由管理员按扩展逐个打开。
5. **有没有一个具体企业的接入案例？** 比如哪个系统要 JWT、哪个要模拟浏览器。有的话，拿它当第一个样例来定接口。这是现在最大的不确定（§14.3）。

## 14. 评审记录（2026-11-11）

对照代码和 roadmap 把三份文档过了一遍，结论和已经改掉的地方记在这里。

### 14.1 能不能做出来

| 项 | 结论 | 依据 |
|---|---|---|
| 声明式扩展 | 能，改动小 | OpenAPI 连接已经是 dsh-ee 自己解析文档、自己发请求（`server/internal/openapi`），工具已有 `ReadOnly`、单独开关和角色限制 |
| `ee_jwt` | 能 | 签发和 JWKS 用标准库就够，私钥存放复用 `secret` 包 |
| 按人存凭据 | 能 | 复用 `secret` 包的加密，新增一张按「员工 + 扩展」存的表 |
| 服务扩展 + Go SDK | 能，工作量中等 | 调用方复用现有的 MCP 客户端；SDK 需要 MCP 服务端实现，现在没有现成的包（§10） |
| 检索按字段过滤 | 能，改动小 | 索引在内存里，`kb/index.go` 的 `scope.has` 已经在按目录和文档过滤，加字段条件是同一个位置 |
| 去掉超级管理员 | 能，但要细 | `Access.Super` 有十几处直接判断，逐个改，靠测试兜住 |
| 写操作的服务端确认 | 能，是新工作 | 现在的确认在 DSH 客户端，服务端不拦（§7.5） |
| 端侧 `webSignIn` | **不确定** | 取决于 DSH 插件能不能开独立窗口并读到 cookie，要真机验证（§13 待定 3） |
| 多副本部署 | 第 1 批不受影响 | 按人凭据、JWT 私钥都在数据库里。知识源同步是后台任务，和 roadmap 里「预载任务移出进程内存」是同一类问题，第 2 批要一起考虑 |

### 14.2 这次评审改掉的地方

1. **写操作标记反过来了。** 原来是「标了 `write` 才确认」，漏标就等于放行。改成「标了 `readOnly` 才免确认」，和现有代码一致，漏标只会多确认一次。
2. **「写操作由 dsh-ee 拦截，扩展绕不过去」说过头了。** 现在拦截在 DSH 客户端。补上了服务端确认，并说明它是新工作（§7.5）。
3. **版本安排堆得太满。** 原来 v0.2 一个版本里有十几项，改成分批，并把「扩展多了才有用」的东西移到有人要才做（§12）。
4. **开发环境不现实。** 原来写「本机起一个完整的 dsh-ee」，但服务端依赖 MySQL / TiDB。改成写扩展用 SDK 自带的模拟环境（不要数据库），配权限用服务端的试算接口（[agent-dev.md](./agent-dev.md) §5）。
5. **补了几处没说清的**：工具前缀怎么定（§5.1）、扩展出问题时的超时和熔断（§4.2）、同步过来的内容不带源系统权限（§6.2）、联邦检索的时间上限和排序（§6.2）、回调地址的限制（§6.3）、JWT 私钥怎么存和轮换（§7.2）、命令表补齐（§10）。
6. **补了「企业得到什么」和衡量办法**（§1.1）。

### 14.3 还在的风险

| 风险 | 说明 | 怎么应对 |
|---|---|---|
| **还没有真实的接入案例** | 整套设计是照 Dify 和推想出来的，没有被一个真实企业的系统检验过（§13 待定 5） | 第 1 批做完后，先拿一个真实系统接一遍，再定第 2 批的接口。示例公司里的假系统不算 |
| **范围相对一个人来说仍然很大** | 即使分了批，第 2 批也有协议、SDK、测试工具、知识源四块 | 第 2 批可以再拆：先协议 + SDK + 工具类扩展，知识源单独一步 |
| **服务扩展还是要企业自己部署** | 不跑第三方代码是已定的取舍，代价是写了代码的扩展门槛比 Dify 高 | 靠声明式扩展覆盖大多数情况；Go SDK 编译出单文件，部署尽量简单；官方扩展提供镜像 |
| **声明式扩展能力有限** | 多步调用、加工返回结果做不了；接口文档质量差的系统，模型用起来效果也差 | 清单里可以改写工具说明、只保留需要的字段；做不了的明确引导到服务扩展 |
| **内置扩展会让核心变大** | 每多一个内置扩展，就多一份要维护的第三方接口 | 只有「大多数企业都要」的才内置，先只有飞书；其他由社区写服务扩展 |
| **服务扩展会拿到凭据** | `user_secret` 和 `shared` 只能原样给（§7.4） | 安装时明确告诉管理员；鼓励用 `ee_jwt` 和 `oauth` |
| **端侧审计是弱保证** | 本机回报可以被绕过（§13 待定 2） | 第 5 批之前定下来；要强保证就得换方案 |
| **权限规则配错的后果是看不到或看多了** | 字段和规则越灵活，越容易配错 | `plan` 给出影响范围、访问测试、`explain`（[agent-dev.md](./agent-dev.md)），第 3 批就应该带上 `explain`，不要等到第 4 批 |

### 14.4 和 roadmap 的关系

- roadmap 阶段 D（v0.2 飞书）不变，第 1 批和它并行。
- roadmap 阶段 E（v0.3 插件系统与多 Agent）由本文第 2 批细化。roadmap 里列的插件类型和这里的扩展点是对应的：业务系统 → 连接器；登录与组织同步 → 组织同步；通知渠道与事件 → 通知、事件；权限策略 → 权限策略；Agent 适配器 → Agent 适配器；文档解析 / OCR 仍按 roadmap 阶段 F 做成外部 HTTP 服务，不走扩展。
- roadmap 里「写操作的审批卡在 L1 下需要服务端兜底」就是 §7.5 的服务端确认。

