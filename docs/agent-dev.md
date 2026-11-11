# 让 Agent 完成扩展和权限配置的开发（草案，2026-11-11）

> 状态：设计稿，还没开工。§10 记了已定的事。先后顺序统一写在 [plugins.md](./plugins.md) §12，评审记录在它的 §14。和 [plugins.md](./plugins.md)（扩展）、[access-control.md](./access-control.md)（管理角色、扩展字段、访问规则）配套。

## 1. 目标

企业要接一个系统、加一个字段、改一套权限，**从写代码、写配置、测试到打包，全部由 Agent（先支持 DSH、Claude Code、Codex）完成**。人只做三件事：

1. 说清楚要什么（「把 OA 的待办接进来，只有本部门能看」）；
2. 提供 Agent 拿不到的东西（真实系统的地址、测试账号、生产凭据）；
3. 批准上线（装扩展、应用配置变更）。

要做到这一点，靠的不是 Agent 更聪明，而是让 dsh-ee 这边满足四个条件：

| 条件 | 意思 | 对应 |
|---|---|---|
| 说得清 | 所有约定都有机器能读的规格，不靠 Agent 去猜 | §3 |
| 能验证 | 有一条命令能判断「做完了没有」，失败时说清楚哪里错、怎么改 | §4 |
| 能试 | 不碰生产就能把整个流程跑一遍，包括「某个员工调用会怎样」 | §5 |
| 能交付 | 改动以可审阅的形式交给人批准，而不是 Agent 直接改生产 | §6 |

再加上一份**技能**（§2），把这些串成 Agent 照着做就能完成的流程。

## 2. 配套技能

### 2.1 有哪几个

| 技能 | 用途 |
|---|---|
| `dsh-ee-extension-dev` | 写扩展：声明式扩展、Go 服务扩展、端侧 TS 扩展 |
| `dsh-ee-access-config` | 配权限：管理角色、扩展字段、访问规则 |
| `dsh-ee-extension-review` | 审扩展：拿到别人写的扩展，检查权限申请、写操作标记、凭据用法，给管理员出一份审阅意见 |

### 2.2 怎么下发

- **DSH**：作为系统技能下发，和现有的 `dsh-ee-*` 系统技能一样，由服务端按当前设置渲染（`server/internal/api/agent_skills.go`）。渲染时填进这家企业的 dsh-ee 地址、已装的扩展、已有的字段和规则，Agent 一开始就知道现状。只下发给有对应权限的人（新增权限点 `extensions.develop`；`dsh-ee-access-config` 要 `fields.manage` 或 `roles.manage`）。
- **Claude Code、Codex**：同样的内容放在主仓库 `extensions/skills/` 下，是标准的 `SKILL.md` 格式；仓库根目录的 `AGENTS.md` 指向它们。`dsh-ee extension init` 生成的新扩展里也会带一份 `AGENTS.md`，Agent 打开这个目录就知道该怎么做。
- 技能内容和 dsh-ee 版本一起维护，改协议时同一个 PR 里改技能（[plugins.md](./plugins.md) §10 的「都放在主仓库」就是为了这个）。

### 2.3 `dsh-ee-extension-dev` 的内容

技能本身要短，把细节交给规格和命令。结构：

1. **先判断用哪种扩展**：照 [plugins.md](./plugins.md) §4.4 的表，问清楚三件事：对方系统有没有 OpenAPI 文档、要不要员工本人的身份、能不能从服务端访问到。
2. **固定流程**：

   ```
   init → 写清单 → 写代码 → check → test → dev 里按场景试 → 生成交付说明
   ```

   每一步都有命令，命令的输出就是下一步的依据。**`check` 和 `test` 全部通过才算做完**，Agent 不能自己宣布做完。
3. **规格去哪查**：`dsh-ee schema` 列出全部规格（§3），不要凭记忆写字段名。
4. **常见错误和改法**：把写操作标成了 `readOnly`、参数该用 `fill: app` 却用了 `model`、知识源的 `cursor` 不是单调递增、错误没带 `code`、在日志里打出了凭据。每一条都对应 `check` 或 `test` 的一个错误码。
5. **不能做的事**：不要在代码里写死凭据；不要申请比需要更宽的权限；不要为了让测试通过去改测试；需要真实凭据时停下来问人。
6. **交付**：按模板写一份交付说明（§6.1）给管理员。

### 2.4 `dsh-ee-access-config` 的内容

1. **先读现状**：`dsh-ee config export` 拿到当前的角色、字段、规则（§6.2）。
2. **把需求写成测试用例**：在改配置之前，先把「谁应该能看到什么、谁不应该看到什么」写成访问测试（§4.3）。
3. **改配置文件**，跑 `dsh-ee config plan`，看影响范围（§6.2），和需求对不上就回去改。
4. 访问测试全部通过、影响范围符合预期之后，提交给管理员批准，Agent 不直接应用。

## 3. 说得清：机器能读的规格

所有约定都发布成规格文件，Agent 和命令行用的是同一份：

| 规格 | 格式 | 内容 |
|---|---|---|
| 清单 | JSON Schema | `dsh-ee.extension.json` 的全部字段，每个字段带说明和例子 |
| 约定接口 | OpenAPI | 服务扩展要实现的 `/ee/*` 接口（[plugins.md](./plugins.md) §6），请求和响应的结构、错误码 |
| ee_jwt | JSON Schema + 说明 | 声明有哪些、怎么验签 |
| 权限配置 | JSON Schema | §6.2 的配置文件：角色、约束、字段、规则 |
| 错误码 | 列表 | `check`、`test`、`plan` 会报的每一个错误码，含义和改法 |

获取方式：

- 命令行：`dsh-ee schema list`、`dsh-ee schema get manifest`，离线可用（规格编译在二进制里）。
- 服务：`GET /.well-known/dsh-ee/schemas/<名字>.json`，和这台 dsh-ee 的版本一致。
- 仓库：`extensions/schemas/`，Go SDK 的类型由这份规格生成，保证三处一致。
- 另外提供 `GET /llms.txt`：一页纸说明 dsh-ee 有哪些规格、命令、技能，给第一次接触的 Agent 看。

## 4. 能验证：检查和测试

### 4.1 输出给机器看

所有开发和配置命令都支持 `--json`。出错时每一条结果都是这样：

```json
{
  "code": "tool.readonly_suspect",
  "severity": "error",
  "where": "dsh-ee.extension.json#/tools/2",
  "message": "工具 submit_leave 标了 readOnly，但名字像写操作，对应的接口方法是 POST",
  "fix": "去掉 tools[2] 的 \"readOnly\": true；如果确实只读，在 checkIgnore 里说明理由",
  "doc": "https://<dsh-ee>/docs/errors#tool.readonly_suspect"
}
```

`where` 精确到文件和位置，`fix` 给出具体改法。Agent 照着改，再跑一遍。

### 4.2 `dsh-ee extension test`：协议一致性测试

`check` 只看静态的清单和代码；`test` 真的把扩展跑起来，按协议逐项调用，判断它是不是一个合格的扩展：

| 测试组 | 测什么 |
|---|---|
| 清单 | `/.well-known/dsh-ee-extension.json` 能取到，符合规格，和 MCP 的工具列表一致，`capabilities` 和实际实现的接口一致 |
| 身份 | 不带 JWT、JWT 过期、`aud` 不对、签名不对的请求都被拒绝 |
| 工具 | 每个工具用规格生成的合法参数调一次、用不合法的参数调一次；`fill: app` 的参数确实不在 MCP 定义里 |
| 错误码 | 构造未授权、未配置的情况，返回的是约定的错误码 |
| 知识源 | `kb/changes` 翻页能走完、`cursor` 能续上、删除能报出来；`kb/browse` 的树能遍历；`kb/document` 对不存在的 id 返回约定错误 |
| 凭据 | 凭据没有出现在扩展的响应和日志里（测试时用一个可识别的假凭据，看它有没有漏出来） |
| 稳定性 | 并发调用、超时、重复调用 |

扩展作者还可以在 `tests/` 下写自己的场景测试（「员工 A 查待办应该看到 3 条」），`test` 一起跑。场景测试用 YAML 写，不用写代码：

```yaml
- name: 本部门员工能看到待办
  as: zhangsan            # §5 示例公司里的员工
  call: list_forms
  args: { status: pending }
  expect:
    ok: true
    count: { gte: 1 }
- name: 没授权时提示授权
  as: lisi
  credential: none
  call: list_forms
  expect:
    error: authorize
```

**全部通过就是「做完了」的唯一标准。** 交付说明里附上测试结果。

### 4.3 权限的测试：访问测试

权限配置也一样要能测。访问测试写成「谁、对什么、应该怎样」：

```yaml
- as: zhangsan            # 等级「重要」，项目 A
  can_read: ["产品规划 2027", "A 项目周报"]
  cannot_read: ["B 项目合同"]
- as: auditor
  can: [audit.view]
  cannot: [roles.assign, users.create]
- search: "合同"
  as: lisi
  must_not_contain: ["B 项目合同"]   # 检索结果里不能出现，片段也不能
```

`dsh-ee config test` 拿当前配置（或改过还没应用的配置）跑这些用例。一条不过，就报出这条判断经过了哪些规则、卡在哪一条（§5.2 的解释接口）。

## 5. 能试：不碰生产的环境

### 5.1 开发环境

dsh-ee 服务端依赖 MySQL / TiDB，「在本机起一个完整的 dsh-ee」对写扩展的人太重。所以分两种情况：

**写扩展：用模拟环境，不需要数据库。** `dsh-ee extension dev <目录>` 在本机起一个假的 dsh-ee，只实现扩展会碰到的那部分：

- **几个预置的员工身份**（沿用示例公司的部门和人），覆盖不同部门、角色、字段值，Agent 可以「以某人的身份」调用；
- **假的 JWKS 和凭据**：`ee_jwt`、`user_secret`、`oauth` 都有模拟实现，OAuth 的授权页面直接自动同意；
- **扩展会反过来调的接口**（查知识库、发起确认）给出固定的假数据；
- **调用记录**：每次调用的请求和响应都记下来，可以用命令查。

`extension test`（§4.2）用的也是这个模拟环境，所以在 CI 里也能跑。

**配权限：在真实的服务端上试算，不生效。** 权限规则要对着真实的人和文档才有意义，所以 `plan`、`test`、`explain`、`simulate` 都是服务端的接口，针对还没应用的配置来算（§5.2、§6.2），算完不留任何改动。要一个可以随便折腾的环境，用现有的 `deploy/demo/demo.sh` 起一套带示例公司数据的 dsh-ee。

### 5.2 解释接口

Agent 调试权限时最需要的是「为什么」：

- `POST /api/admin/access/explain`，参数 `{ user, action, object }`，返回最终结果和判断过程：经过了库级权限、哪几条字段规则、有没有问权限策略扩展，每一步的输入和结论。
- `POST /api/admin/access/simulate`，参数是一组用户和一组对象，返回一张「谁能看什么」的表，用来批量核对。
- 两个接口都可以针对**还没应用的配置**来算，所以 Agent 可以先改、先算，确认了再提交。
- 只能问到令牌主人自己看得到的对象：结果里只有「能不能看」和标题，不返回内容；令牌主人自己看不到的文档，不会出现在结果里。
- 命令行对应 `dsh-ee config explain` 和 `dsh-ee config simulate`。

### 5.3 连真实系统

需要连真实的内网系统调试时，用远程调试（[plugins.md](./plugins.md) §10）：扩展在开发者电脑上跑，连到真实的 dsh-ee，只对开发者本人可见。这一步要人提供调试密钥，技能里写明到这一步就停下来问人。

## 6. 能交付：改动交给人批准

### 6.1 扩展的交付

测试全部通过之后（声明式扩展再 `dsh-ee extension pack` 打成 zip；服务扩展交付的是代码仓库或镜像），技能要求 Agent 生成一份交付说明（模板由 `dsh-ee extension report` 生成，Agent 补充）：

- 这个扩展做什么、提供哪些工具、哪些是写操作；
- 申请了哪些权限、为什么需要；
- 用哪种凭据模式、会交给扩展什么凭据；
- 测试结果（`check`、`test` 的输出）；
- 管理员安装时要填什么、去哪里拿。

管理员看完说明再去装，安装流程照 [plugins.md](./plugins.md) §9.4。

### 6.2 权限配置的交付：配置即代码

角色、约束、字段、规则都可以导出成一个 YAML 文件，改完再导回去：

```
dsh-ee config export > access.yaml      # 导出当前配置
（Agent 修改 access.yaml）
dsh-ee config plan access.yaml          # 看会变什么、影响谁
dsh-ee config test access.yaml          # 跑访问测试
dsh-ee config apply access.yaml         # 提交，等管理员批准
```

- **`plan` 给出影响范围**，不只是配置的差异：「新增字段『项目』；规则生效后，12 个员工将看不到 37 份文档，其中 3 份是他们最近一周看过的」。Agent 拿这个和需求核对，人也拿这个来批准。
- **`apply` 不直接生效**：它生成一个变更请求，进入管理后台的「待批准」，由有权限的管理员批准后才生效；开了双人批准（[access-control.md](./access-control.md) §3.2）的，要两个人。
- dsh-ee 自己保存每一版配置，生效之后可以一键回滚到上一版。

### 6.3 管理接口

上面的命令背后都是管理接口，Agent 也可以直接调：

| 接口 | 用途 |
|---|---|
| `GET /api/admin/config` | 导出配置 |
| `POST /api/admin/config/plan` | 算差异和影响范围 |
| `POST /api/admin/config/test` | 跑访问测试 |
| `POST /api/admin/config/changes` | 提交变更请求 |
| `POST /api/admin/access/explain`、`/simulate` | §5.2 |
| `POST /api/admin/extensions/check`、`/test` | 对一个地址或上传的包跑检查和测试 |

这些接口同时通过 dsh-ee 的 `/mcp` 暴露成工具（`ee_dev__*`），Agent 在 DSH 里不用命令行也能用。

## 7. Agent 能做什么、不能做什么

Agent 用的令牌和员工自己的令牌分开，权限按用途给：

| 令牌 | 能做 | 不能做 |
|---|---|---|
| 开发令牌 | 用开发环境、跑检查和测试、远程调试（有调试权限时） | 碰生产数据、装扩展 |
| 配置令牌 | 导出配置、`plan`、`test`、`explain`、`simulate`、提交变更请求 | 批准变更、直接应用 |

- **批准永远是人**：装扩展、批准配置变更，都不能用 Agent 的令牌做。
- `explain` 和 `simulate` 只返回「能不能看」和标题，不返回内容，而且只涉及令牌主人自己看得到的对象（§5.2），所以配置令牌不会变成绕过权限读文档的口子。
- Agent 的所有操作照常记审计，并标明是通过哪个令牌、哪个 Agent 做的。

## 8. 「100% 由 Agent 完成」到底包括什么

| 环节 | Agent 完成 | 需要人 |
|---|---|---|
| 判断用哪种扩展 | ✅ | |
| 写清单、代码、接口文档补充 | ✅ | |
| 写场景测试、访问测试 | ✅ | 确认测试写的就是需求 |
| 检查、测试、修到全部通过 | ✅ | |
| 在开发环境里模拟各种员工 | ✅ | |
| 连真实系统调试 | ✅ | 提供调试密钥和测试账号 |
| 写交付说明 | ✅ | |
| 安装扩展、批准配置变更 | | ✅ |
| 填生产凭据 | | ✅ |

说「100%」指的是**开发工作**全部由 Agent 完成；上线的决定和生产凭据始终在人手里。这不是做不到，而是不该交给 Agent。

## 9. 先后顺序和验收

统一写在 [plugins.md](./plugins.md) §12。本文的内容跟着它要服务的那一批走：

| 批次 | 本文的内容 |
|---|---|
| 第 1 批 | 规格文件（清单、错误码）和 `dsh-ee schema`；`check --json`；`dsh-ee-extension-dev` 技能的声明式部分 |
| 第 2 批 | 约定接口的规格；`extension test`；模拟环境；技能补上服务扩展部分；交付说明 |
| 第 3 批 | `explain`、访问测试 |
| 第 4 批 | 配置导出和 `plan` / `test` / `apply`、`simulate`；`dsh-ee-access-config` 技能；开发令牌和配置令牌；`ee_dev__*` 工具 |
| 有人要才做 | `dsh-ee-extension-review` 技能（等有了第三方扩展再说） |

**验收办法**：每一批做完、发布之前，拿一个没做过的需求（比如「接一个新的工单系统」「加一个『客户』字段，只有对应的客户经理能看」），在干净的环境里只给 Agent 需求和技能，看它能不能不经人改代码、一路做到 `test` 全部通过、交出交付说明。做不到的地方，就是技能、规格或错误提示要补的地方。

## 10. 已定（2026-11-11）

1. **技能先支持 DSH、Claude Code、Codex 三个。** DSH 走系统技能；Claude Code 和 Codex 都读仓库里的 `AGENTS.md` 和 `SKILL.md` 格式的技能文件，用同一份内容。各自放技能文件的具体目录，做的时候按当时的官方文档核对一遍。
2. **配置的修改只走 dsh-ee 自己的变更请求和批准**，不和 Git 绑定。版本历史和回滚由 dsh-ee 自己记。导出的 YAML 只是一个普通文件，企业愿意放在哪里都行。
3. **先只做命令行。** Agent 用命令行最顺；命令背后都是 §6.3 的管理接口，以后要做网页版，直接用同一套接口。
