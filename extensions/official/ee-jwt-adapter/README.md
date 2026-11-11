# ee_jwt 适配层样例

企业系统那一侧怎么认 dsh-ee 签的身份令牌（`ee_jwt`）。只用 Go 标准库，一个文件，可以整段抄进自己的系统。协议见 [docs/ee-jwt.md](../../../docs/ee-jwt.md)。

## 跑起来

```bash
go run . -issuer https://ee.acme.com -audience hr-demo
```

- `-issuer`：dsh-ee 的访问地址（系统设置里的「访问地址」）。
- `-audience`：本系统的标识。连接里填了 aud 就用它，没填就是连接标识。
- `-header`：令牌不放在 `Authorization: Bearer` 时，指定请求头名字。

## 接进 dsh-ee

1. 企业连接 → 接入企业系统，类型选「OpenAPI」，标识填 `hr-demo`。
2. 接口文档贴 `http://<这个样例的地址>/openapi.json` 返回的内容。
3. 「员工身份」选「身份令牌（ee_jwt）」，保存。
4. 在 DSH 里让模型调 `hr-demo__whoami`，返回的就是当前员工自己。换一个员工登录，返回的是另一个人。

## 放进真实系统

把 `verify` 和 `keys` 抄进鉴权中间件：验过之后按 `preferred_username` 或 `email` 找到系统里的用户，换成系统自己的会话，后面的权限判断照旧。不要信任没验过签的请求头。
