# 按员工本人的身份调用：ee_jwt 与 user_secret

> 这是 [plugins.md](./plugins.md) §7 第 1 批里已经做完的部分的协议说明：连接上的「员工身份」。
> 对方系统的适配层样例在 [extensions/official/ee-jwt-adapter](../extensions/official/ee-jwt-adapter)。

连接原来只有一份管理员配的共享凭据，源系统分不清是谁在调。现在连接上可以再开一层**员工身份**，和共享凭据并存（各放一个请求头）：

| 方式 | 适用 | 每次调用带什么 |
|---|---|---|
| `ee_jwt` | 企业自研系统，或愿意加一层适配的系统 | dsh-ee 为当前员工现签的 JWT，5 分钟有效 |
| `user_secret` | 员工各有 API token 的系统 | 员工自己在连接页面填的 token，按人加密存放 |

权限仍由源系统判定，dsh-ee 只负责告诉它「是谁」。

> 保存前「试连」：`ee_jwt` 会用页面上的连接标识作为 `aud`（填了 `audience` 则用配置值），所以对方校验 `aud` 的连接也能在保存前试通。`user_secret` 的员工凭据要保存连接后才能填，试连只测共享凭据。

## 1. 在连接里打开

企业连接 → 编辑 → 「员工身份」。存在连接的版本内容里：

```json
{
  "type": "openapi",
  "url": "https://oa.acme.internal/api",
  "auth": { "type": "header", "header": "X-API-Key" },
  "user_auth": { "mode": "ee_jwt", "header": "", "audience": "oa", "claims": ["dept"] }
}
```

| 字段 | 说明 |
|---|---|
| `mode` | `ee_jwt` 或 `user_secret` |
| `header` | 放在哪个请求头。空则 `Authorization: Bearer <值>`；填了就把值原样放进这个头。不能和共享凭据用同一个请求头 |
| `audience` | 只用于 `ee_jwt`：令牌的 `aud`。空则用连接标识 |
| `claims` | 只用于 `ee_jwt`：额外带哪些员工信息，可选 `dept`、`roles`、`idp_sub`，默认都不带 |
| `hint` | 只用于 `user_secret`：告诉员工这个凭据去哪里拿 |

MCP 连接和 OpenAPI 连接都支持。MCP 连接开了员工身份后，每位员工各用各的上游会话。

## 2. ee_jwt

### 2.1 令牌

RS256 签名的 JWT。头部带 `kid`，标明用的是哪一把钥匙。

| 声明 | 内容 |
|---|---|
| `iss` | dsh-ee 的访问地址（系统设置里的「访问地址」；没填时按请求推断，生产环境请填上，否则可能不稳定） |
| `aud` | 对方系统的标识：连接里填的 `audience`，没填就是连接标识 |
| `sub` | 员工在 dsh-ee 里的 id（字符串，不会变） |
| `preferred_username` | 用户名 |
| `name` | 显示名 |
| `email` | 邮箱，没有就不带 |
| `iat` / `nbf` / `exp` | 签发时间、生效时间、过期时间。`exp - iat` 固定 300 秒 |
| `jti` | 每个令牌不同的随机串 |
| `dept` | 部门。连接里打开了才带 |
| `roles` | 角色标识的数组。连接里打开了才带 |
| `idp_sub` | 员工在企业目录或统一登录里的标识（LDAP 的 DN、OIDC 的 `sub`）。打开了、而且员工是从那里来的才带 |

每次调用现签，不缓存：员工调了岗，下一次调用的 `dept` 就是新的。

### 2.2 验签

对方系统要做四件事，缺一不可：

1. 只接受 `alg` 为 `RS256` 的令牌。
2. 从 `https://<dsh-ee>/.well-known/jwks.json` 取公钥（不需要登录），按头部的 `kid` 找到对应的那把，验签。
3. 检查 `exp` 没过期。
4. 检查 `iss` 是这台 dsh-ee，`aud` 是自己。**`aud` 必须查**，否则 dsh-ee 给别的系统签的令牌也能拿来调这个系统。

验过之后按 `preferred_username`、`email` 或 `idp_sub` 找到自己系统里的用户，后面的权限判断照旧。

用现成的 JWT 库（Spring Security、`jose`、PyJWT 等）配 JWKS 地址即可。不想引入依赖的，[适配层样例](../extensions/official/ee-jwt-adapter/main.go)用 Go 标准库写了完整的验签，一百行出头。

### 2.3 密钥和轮换

- 第一次用到时生成一把 RSA-2048 密钥。私钥用主密钥（`DSH_EE_SECRET_KEY`）加密后存在数据库里，几个副本共用。
- 企业连接页面（有连接用了 `ee_jwt` 时）显示签发方、JWKS 地址和钥匙列表，可以「轮换密钥」。
- 轮换是平滑的：新公钥马上出现在 JWKS 里，**过 10 分钟才开始用它签名**，这段时间仍用旧钥匙签。对方系统多半缓存着 JWKS，这样它们有时间取到新公钥，不会出现「新令牌验不过」的空档。
- 换下来的公钥继续在 JWKS 里公开 24 小时，已经签出去的令牌照常能验。
- 已经有一把钥匙在排队时，再点「轮换」不会再生成新的。
- 对方系统按 JWKS 地址取公钥、按 `kid` 选钥匙即可，轮换时不用改任何配置。几个副本各自每分钟重读一次密钥，到时间一起换。
- 这是例行轮换，不是紧急吊销：怀疑私钥泄露时，还要同时更换主密钥并重新部署。

## 3. user_secret

- 员工在连接的页面上看到「我的凭据」，填自己的 token。只加密存在 dsh-ee，不下发到员工电脑。
- 没填的员工调用这个连接的工具时，请求不会发出去，工具结果里告诉他去哪个页面填。
- 管理员在连接页面能看到哪些员工填了、什么时候填的，可以逐个收回；看不到内容，也看不到末尾几位。
- 员工被停用（包括企业目录同步时停用）时，他按人存的凭据一起删掉。连接被删除时，上面所有员工的凭据一起删掉。
- MCP 连接同步工具清单时没有「当前员工」，用的是点同步的管理员自己的凭据，所以管理员要先填自己的。

## 4. 记录

- 连接的调用记录多了一列，写明这次带的是哪种员工身份。
- 填、删、收回凭据和轮换密钥都进审计（`connection.credential_set` / `credential_delete` / `credential_revoke` / `ee_jwt_rotate`），不记凭据内容。

## 5. 还没做的

[plugins.md](./plugins.md) §7 里的这些留给后面的批次：`oauth`、同一连接存多份共享凭据、写操作的服务端确认、清单（`dsh-ee.extension.json`）里的 `credential.mode`。
