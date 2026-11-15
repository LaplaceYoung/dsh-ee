# dsh-ee Harness SDK

给第三方 Harness / 自研 Agent / 企业集成方接入 dsh-ee 的客户端 SDK。
协议面是 dsh-ee 的 L1 中立契约（发现、设备码、OpenAI 兼容模型网关、MCP 工具），
**不是** DSH 接入插件，也不实现 Anthropic Messages / OpenAI Responses 等协议。

| 语言 | 目录 | 依赖 | 状态 |
|---|---|---|---|
| TypeScript / ESM（Node 18+ / 浏览器） | [`typescript/`](typescript/) | 零运行时依赖（`fetch`） | 0.1.0，未发布 npm |
| Python（>= 3.9） | [`python/`](python/) | 仅标准库（`urllib`） | 0.1.0，未发布 PyPI |

两者覆盖的能力略有差异：

| 能力 | TypeScript | Python |
|---|---|---|
| 发现文档（ETag 缓存）/ 能力摘要 / 模型目录 | ✅ | ✅ |
| chat completion 非流式 | ✅ | ✅ |
| chat completion SSE 流式 | ✅ | — |
| Chatbot 流式事件（`server_tool_event` / `delta.title`）类型 | ✅ | 仅类型 |
| 设备码登录（申请 / 轮询 / `slow_down`） | ✅ | ✅ |
| MCP initialize / `tools/list` 分页 / `tools/call` | ✅ | ✅ |
| 写确认 `_meta["dsh-ee/confirmation"]` | ✅ | ✅ |
| DeepSeek Chatbot 扩展类型（server tools / safety / title / 拒答 / latest_reminder / file·document；仅透传） | ✅ | ✅ |
| 错误壳 / `X-RateLimit-*` / `Retry-After` 解析 | ✅ | ✅ |

示例与兼容性：

- 示例（MCP-only Claude Code / Codex、通用 Harness、浏览器 CORS、思考模式、Chatbot 服务端工具）：
  [`examples/`](examples/README.md)
- 兼容性矩阵：[`typescript/COMPATIBILITY.md`](typescript/COMPATIBILITY.md)、
  [`python/COMPATIBILITY.md`](python/COMPATIBILITY.md)
- 变更记录：[`typescript/CHANGELOG.md`](typescript/CHANGELOG.md)、
  [`python/CHANGELOG.md`](python/CHANGELOG.md)

> DeepSeek Chatbot 的搜索/阅读链接/内容安全审核/会话标题由上游服务端执行；`latest_reminder`
> 只是 messages 里的标记消息；文件/文档需先通过 DeepSeek Files API 上传（dsh-ee 只透传 chat 请求，
> 不代理 `/files`）。dsh-ee 与 SDK 只透传字段和提供类型，不实现服务端工具或文件解析逻辑。

依赖的服务端契约：

- 发现 / 能力 / `Dsh-EE-API-Version` / 中立错误壳：PR #123
- MCP `tools/list` 分页与写确认结果级 `_meta`：PR #128
- `/v1/*` 与 `/mcp` 限流头、`X-Request-Id`：PR #129
- 令牌 `client_kind`（第三方令牌 = `third_party`，写操作始终人工确认）：PR #132

协议细节与完整 API 说明见各语言目录的 README；服务端侧的接入说明见仓库
`docs/l1-client.md`（L1 客户端）与 `docs/harness-integration.md`（Harness 接入）。

## 快速验证

```sh
# TypeScript
cd sdk/typescript
npm install
npm test               # 构建 + node:test（全部 mock fetch）
npm pack --dry-run     # 检查发布包内容，不发布

# Python
cd sdk/python
python3 -m unittest discover -s tests -v
```

两者都未发布到 npm / PyPI；在当前仓库内按各目录 README 的方式本地安装即可。

## 非目标

- 不做 DSH 接入插件（那在 `dsh-plugin/`），不写本机 profile，不处理 `/ee-login` 设备确认页面。
- 不实现 Claude Code / Codex 的专有协议；不对这些客户端做兼容性声明。
- 不在 SDK 内做自动重试、令牌持久化或服务端 API 变更；写操作/工具调用是否重试由调用方决定。
