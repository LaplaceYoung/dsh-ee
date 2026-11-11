# 声明式扩展：安装与运行

> 对应 [plugins.md](./plugins.md) §4.1、§5、§6.1、§7、§9 和 §12 的第 1 批。本文记录本次完成的安装纵切，不代表整个 v0.2 或扩展第 1 批全部完成。

## 1. 范围与边界

已实现：管理员上传声明式 ZIP → 静态预检 → 填写安装配置与凭据 → 生成并运行企业连接。复用 #59 的员工身份和 #60 的 `extension.Check`，不另造一套 OpenAPI 执行引擎。

本次不包含：

- 多份命名共享凭据（当前仍是资产上的一份共享凭据）；
- 从 Git 仓库/服务地址安装、自动升级、签名策略和企业目录；
- 现有连接自动转换成声明式扩展；
- `fill: app` 在单个应用上的自定义覆盖。当前只允许清单中的默认值，模型和全局安装配置都不能覆盖它；
- OAuth、服务扩展、知识源、回调、端侧扩展和飞书 D1。

## 2. 安装流程

管理端「企业连接 → 安装声明式扩展」：

1. 选择 ZIP。最多 8 MB、16 个文件；每个文件解压后最多 4 MB，总解压量不超过 8 MB。拒绝绝对路径、`..`、反斜杠和重复文件名。只读取 `dsh-ee.extension.json` 与清单指向的 OpenAPI 文件，不执行任何包里内容。
2. 服务端用 #60 同一份 `extension.Check` 检查。同一个后端也处理 `POST /api/admin/extensions/check`，返回身份、digest、服务器、凭据方式、工具与安装字段；结果含标准 `code / severity / where / message / fix`。
3. 检查通过后，管理员选择清单 `servers` 允许的根地址、连接标识（未来工具名前缀）、安装配置和凭据。`fill: admin` 收集 `config` 与工具参数并合并去重；`enum` / `options` 渲染为下拉框并在安装时校验。
4. `POST /api/admin/extensions/install` 再次校验后创建已发布的 OpenAPI 连接及 `extension_installations` 记录。slug 或扩展 id 冲突返回 409；后续元数据/凭据保存失败时删除刚创建的资产，避免半成品。
5. 按连接配置生成工具。写工具依照现有 OpenAPI 规则默认停用，必须由管理员显式启用，不会因为上传 ZIP 自动开放写操作。

安装接口只要求现有 `connections.manage` 权限，不开放成 `ee_*` Agent 工具。预检、安装权限在服务端强制执行，不能只靠隐藏菜单。

## 3. 数据与秘密保存

迁移 `0025_declarative_extensions.sql` 新增：

- `extension_installations`：`asset_id`、`extension_id`、版本、digest、种类、原始清单/接口文档、公开安装配置、加密的 secret 配置与创建人/时间；扩展 id 唯一。
- `connection_calls.extension_id / extension_version / extension_digest`：每次调用可定位到实际清单版本与摘要。

仍是单一 `/api` 与 `store` 分层，TiDB 与 OceanBase 共用 MySQL 语法。删除连接时同步删除安装记录。

秘密分类：

| 内容 | 保存位置 | 响应与员工可见性 |
|---|---|---|
| 原有共享凭据 | 资产 `secret_enc`，主密钥加密 | 不回显；直接用于对应共享请求头 |
| `credential: user_secret` | #59 的 `user_credentials`，按员工加密 | 员工自己填写；管理员只能看授权状态和收回 |
| `config` 中 `type: secret` | `extension_installations.secret_enc`，整个 secret 对象加密 | 不进资产 spec / 安装响应 / 详情接口；实际调用时才合并解密值 |
| 非 secret 固定配置 | 资产 spec 的 `extension_config` + 安装表的 `config` | 按原连接的版本查看权限可见 |

扩展的身份、版本、digest 展示在连接详情。正常资产编辑器提交新版本时会保留清单与公开配置，不会无意清除安装信息。

## 4. Token 与请求头

`credential.shared` 表示另外一份共享凭据。`credential.mode` 为 `user_secret` 或 `ee_jwt` 时，可叠加一层员工身份：

- 共享凭据沿用原 `auth` 配置；
- 员工身份沿用 #59 的 `user_auth` 配置；
- 二者不能占用同一个请求头，由已有校验拦截。

跨主机重定向仍按 #59 的 `sameHostRedirect` 拒绝，避免任一密钥被转发到清单外地址。清单服务器只能来自 ZIP 的 `servers`，接口调用根地址不能由模型修改。

## 5. 参数、枚举与返回过滤

### `fill`

- `model` / 缺省：参数留在开放给 Agent 的 JSON Schema 中，由模型填写。
- `admin`：从安装配置与清单默认值补齐，模型手动伪造也会被服务端覆盖。
- `app`：本次只允许清单 `default`，不从安装配置读取，模型也不能覆盖；单个应用的自定义绑定留给下一阶段。

保存工具 schema 时会删除 `fill: app/admin` 对应属性和 required 项，包括嵌套的 `body.<字段>`。全部固定字段被删掉时也移除空的 `required`，而不是留下可能不兼容的空数组。底层必填仍由 OpenAPI 接口负责，模型可填参数不会被误删。

工具返回字段只读枚举不能通过手工构造 MCP 请求绕过；可选枚举缺省时不判错。未配置的固定参数会删除模型伪造值，不发送 `null`。

### `keep`

`keep` 只处理工具的 **第一层字段**，与 #60 规格说明一致。若上游返回数组，会逐条保留声明字段；若返回对象包装（例如 `items`），只会保留根对象上的声明字段，不递归过滤嵌套对象。成功响应不是 JSON 时拒绝把未过滤内容交给模型，错误响应保留原有 HTTP 错误语义。

先受控读取再做过滤，随后仍受现有 64 KiB 返回上限约束，避免大字段成为内存或上下文炸弹。

## 6. 自动化验证

- 纯函数测试：固定 schema、嵌套 body 参数、空 required 删除、数字枚举、可选枚举缺省、未发现固定值删除、keep 和非法 JSON。
- ZIP 安全：正常安装包、路径穿越和绝对路径。
- TiDB 集成：真实上传、安装、重复安装回滚、两位员工身份、共享凭据、秘密配置、写工具启用、真实 POST 参数覆盖与扩展审计，删除连接时安装级联清理。
- 浏览器 E2E `27-declarative-extensions`：实际是 dsh-ee 自家的 ITSM 系统，上传/预检/改连接前缀/安装/启停/真实 MCP 创建工单与列表过滤。
- `DSH_EE_TEST_DSN='root:@tcp(127.0.0.1:4000)/' go test -timeout 30m -count=1 ./...`：全部通过。
- 全量浏览器 E2E：27/27 通过。
- 录屏：`docs/images/declarative-extension-install.gif`，8 fps、960 × 675，PR 可预览。

最终命令及全量 Go 结果在 PR 正文中逐项列出，不用“只构建”或数据库 skip 冒充集成验证。

## 7. 下一项

1. 应用级 `fill: app` 绑定：明确应用保存哪个参数、如何影响工具 schema、审计和升级，不能让当前应用之外的人改写。
2. 多份命名共享凭据：兼容现有单份默认凭据和按应用选择规则。
3. 旧连接自动转换、Git/上传包升级与回滚；飞书 D1 独立推进。
