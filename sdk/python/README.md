# dsh-ee Harness Python 客户端（最小实现）

dsh-ee 的第三方 Harness 最小 Python 客户端，**仅标准库**（`urllib`），无需安装依赖。
与 [`../typescript`](../typescript) 是同一套 dsh-ee L1 契约的两种语言实现；TypeScript 版
覆盖流式 chat、SSE 响应、ETag 细节更完整，Python 版用于脚本/自动化快速接入。

> 状态：`0.1.0`，未发布 PyPI。这是通用 HTTP/MCP 客户端，不实现 Anthropic Messages /
> OpenAI Responses 协议，也不声称兼容 Claude Code / Codex 的专有协议。

## 覆盖范围

| 能力 | 方法 |
|---|---|
| 服务发现（ETag 缓存） | `client.discovery(refresh=False)` |
| 能力摘要 | `client.capabilities()` |
| 模型目录 | `client.list_models()` |
| 非流式 chat completion | `client.chat_completion(model, messages, thinking=..., reasoning_effort=..., tools=..., safety_check=..., title=..., **params)` |
| 设备码登录 | `request_device_code()` / `poll_device_token()` / `wait_for_device_token()` / `login_with_device_code()` |
| MCP | `connect_mcp()` / `list_tools()` / `list_all_tools()` / `call_tool()` / `close()` |
| 写确认 | `extract_confirmation(result)` / `is_pending_confirmation(result)` |
| 错误与限流头 | `DshClientError`、`DshJsonRpcError`、`DeviceFlowError`、`parse_rate_limit_headers()`、`parse_retry_after()` |

依赖的服务端契约与 TypeScript 版相同：发现/能力/API 版本头（#123）、MCP 分页与写确认
`_meta`（#128）、限流与 `X-Request-Id`（#129）、令牌 `client_kind`（#132）。

## 安装

```sh
# 从本仓库直接使用（推荐先设置 PYTHONPATH 或 pip install -e）
cd sdk/python
python3 -m pip install -e .

# 或只跑测试（不需要安装）
cd sdk/python
python3 -m unittest discover -s tests -v
```

要求 Python >= 3.9。

## 快速开始

```python
from dsh_ee_harness import DshClient

client = DshClient("https://dsh-ee.example.com", token="ee_at_...")

print(client.discovery()["version"])
print([model["id"] for model in client.list_models()])

completion = client.chat_completion(
    "company-chat",
    [{"role": "user", "content": "ping"}],
)
print(completion["choices"][0]["message"]["content"])
```

## 思考模式（thinking / reasoning_effort）

dsh-ee 网关把思考模式参数**原样转发**给上游，不做白名单/映射；能力以服务端公布为准
（`GET /v1/models` 每项的 `capabilities.reasoning`、发现文档的 `models.reasoning`）。
Python 客户端提供对应类型：`ChatMessage` / `ChatCompletionRequest` / `ThinkingConfig` /
`ReasoningEffort` / `ChatCompletion` / `ChatCompletionChunk`（TypedDict，仅类型提示）。

```python
from dsh_ee_harness import ChatMessage, DshClient

client = DshClient("https://dsh-ee.example.com", token="ee_at_...")

messages: list[ChatMessage] = [{"role": "user", "content": "9.11 和 9.8 哪个大？"}]
completion = client.chat_completion(
    "company-reasoner",
    messages,
    thinking={"type": "enabled"},   # 原样转发；DeepSeek 格式 {"type": "enabled"} / {"type": "disabled"}
    reasoning_effort="high",        # 原样转发；可用值见 /v1/models 的 capabilities.reasoning.efforts
)
message = completion["choices"][0]["message"]
print("思维链:", message.get("reasoning_content"))
print("回答:", message.get("content"))
```

**工具调用轮次必须回传历史 `reasoning_content`**：请求带 `tools` 时，后续请求要把历史
assistant 消息的 `reasoning_content` 原样带上（即使该轮没有实际调用工具），否则上游返回 400
（`The reasoning_content in the thinking mode must be passed back to the API.`）；不带 `tools`
的多轮对话可以不回传。dsh-ee 网关不补、不删、不校验，保留它是调用方的责任：

```python
messages: list[ChatMessage] = [{"role": "user", "content": "杭州明天天气？"}]
first = client.chat_completion(
    "company-reasoner", messages, tools=tools,
    thinking={"type": "enabled"}, reasoning_effort="high",
)
# 直接追加返回的 message：content / reasoning_content / tool_calls 都在里面。
messages.append(first["choices"][0]["message"])
# ...执行 tool_calls 后追加 role=tool 消息，再发下一轮；历史 reasoning_content 会一起回传。
```

Python 版只实现非流式 chat（标准库 `urllib` 一次读完响应体）；流式 `delta.reasoning_content`
的类型见 `ChatCompletionChunk`，需要流式请用 TypeScript 版或自行解析 SSE。

## DeepSeek Chatbot 扩展（server tools / safety / title / 拒答）

DeepSeek 的 Chatbot 能力通过 chat completions 扩展字段声明与返回，dsh-ee 网关原样透传；
**Python SDK 不执行服务端工具逻辑**（搜索/读链接由上游完成）。完整示例见
[`../examples/chatbot-server-tools.md`](../examples/chatbot-server-tools.md)。

```python
result = client.chat_completion(
    "company-bot",
    [{"role": "user", "content": "DeepSeek V4.1 有什么新特性？"}],
    tools=[{"type": "deepseek_web_search"}, {"type": "deepseek_read_url"}],
    safety_check={"level": "high"},
    title={"enabled": True},
)
message = result["choices"][0]["message"]
print(message.get("title"))                       # 会话标题
for step in message.get("server_tool_calls", []):
    for call in step["calls"]:
        print(call["type"], call["kind"], call.get("signature"))
print(result["choices"][0]["finish_reason"])      # content_filter = 拒答
print(message.get("refusal"))                     # 拒答说明（如被拦截的图片）
```

- 多轮：把上一轮 assistant 消息原样回传，**保留 `server_tool_calls` 里的 `kind` 与 `signature`**；
  丢掉 `signature` 请求仍成功，但之前读过的页面不会进入上下文。
- 内容安全拒答：`finish_reason` 为 `content_filter`，`message.refusal` 是说明；移除已展示回答。
- 类型：`types.py` 里提供 `DeepSeekServerToolDeclaration` / `DeepSeekSafetyCheck` /
  `DeepSeekTitleConfig` / `DeepSeekServerToolStep` / `DeepSeekServerToolCall` /
  `DeepSeekUrlCitation` / `DeepSeekRefusal` / `DeepSeekStopDetails`，以及 Messages API 的
  `server_tool_use` / `web_search_tool_result` / `web_fetch_tool_result` / citations 块类型。
- Python 版只实现非流式；流式 `server_tool_event` / `delta.title` 的类型保留在
  `dsh_ee_harness.types` 里，需要流式请用 TypeScript 版或自行解析 SSE。
- 启用 Chatbot 能力后，上游会忽略部分采样参数（`temperature` / `top_p` / `frequency_penalty` /
  `presence_penalty` / `stop` / `seed` / `n` 等），`max_tokens` 仍生效；dsh-ee 不改写请求。

### 用户元信息标记（`latest_reminder`）

```python
from dsh_ee_harness import format_latest_reminder

messages = [
    {
        "role": "latest_reminder",
        "content": format_latest_reminder(
            {
                "date": "2026-11-11",
                "weekday": "Wednesday",
                "location": "Singapore",
                "platform": "Web",
                "language": "English",
            }
        ),
    },
    {"role": "user", "content": "今天有什么新闻？"},
]
client.chat_completion("company-bot", messages)
```

`format_latest_reminder` 生成 `<date>,<weekday>,<location>,<platform>,<language>`；`location`
可为空、允许含逗号。消息必须紧贴对应的 `user` 消息之前（上游校验）；历史轮次可原样带回。
类型：`DeepSeekLatestReminder` / `DeepSeekLatestReminderMessage`。

### 文件 / 文档输入（DeepSeek Files API）

文件需先上传到 **DeepSeek Files API**（`POST /files`，`purpose` = `user_data` /
`text_recognition` / `rich_layout_recognition`）并解析拿到 `file-api-...`，再在 chat 请求里引用。
**dsh-ee 的 `/v1` 只透传 chat 请求，不代理 Files API；本 SDK 不实现 Files API**。

```python
result = client.chat_completion(
    "company-bot",
    [
        {
            "role": "user",
            "content": [
                {"type": "text", "text": "总结这份文档的重点。"},
                {"type": "file", "file_id": "file-api-xxxxxxxxxxxxxxxx"},  # 文档只能这样引用
            ],
        }
    ],
)
```

- `file` 内容块：`file_id` 与 `file_data` 恰好二选一（`DeepSeekFileContentPart`）；`file_data`
  是内联 base64 data URL（可带 `filename`）。
- Messages API 形状的 `document` 块类型：`DeepSeekDocumentBlock` / `DeepSeekDocumentSource`
  （`source.type = file / text`），SDK 不调用 Messages API，仅供统一渲染参考。
- 限制（上游）：单请求最多 20 个文档、文本合计 ≤8 MiB；文档上传 ≤100 MiB、≤1000 页，上传/解析
  各 120 秒内完成；dsh-ee/SDK 不校验、不代理，上游错误原样返回。

## 设备码登录

```python
from dsh_ee_harness import DeviceFlowError, DshClient

client = DshClient("https://dsh-ee.example.com")
token = client.login_with_device_code(
    client_name="my-harness",
    on_code=lambda code: print("请打开", code["verification_uri_complete"]),
    on_pending=lambda code, attempt: print("等待确认:", code),
)
# 登录成功后 client 已带令牌
print(token["user"]["username"], token["expires_at"])
```

`wait_for_device_token()` 会处理 `authorization_pending`、`slow_down`（自动 +5 秒）与
`rate_limited`；`access_denied` / `expired_token` 立即抛 `DeviceFlowError`。

## MCP 工具

```python
from dsh_ee_harness import extract_confirmation

session = client.connect_mcp()
print(session.server_info, session.session_id)

tools = session.list_all_tools()          # 自动跟随 nextCursor 取全量
page = session.list_tools(limit=50)       # 单页；limit=0 取全量
everything = session.list_tools(limit=0)

result = session.call_tool("crm__lookup", {"q": "张三"})
if result.get("isError"):
    confirmation = extract_confirmation(result)
    if confirmation:
        print("请在网页确认:", confirmation["url"], confirmation["expires_at"])
        # 用户确认后，用完全相同的参数再调一次才会真正执行
    else:
        print("工具返回错误:", result["content"])
session.close()
```

## 错误与限流

```python
from dsh_ee_harness import DshClientError, DshJsonRpcError

try:
    client.list_models()
except (DshClientError, DshJsonRpcError) as error:
    print(error.status, error.request_id, error)
    if error.is_rate_limited:
        wait = error.retry_after or 60
        print(f"{wait}s 后重试")
```

`DshClientError` 带 `status` / `request_id` / `api_version` / `rate_limit` / `retry_after` /
`code` / `error_type` / `body`；JSON-RPC 错误另有 `rpc_code` / `rpc_data`。
限流头也可单独解析：`parse_rate_limit_headers(headers)`、`parse_retry_after(headers)`。

## 自定义传输（代理 / 测试）

`transport` 是签名为 `(method, url, headers, body, timeout) -> (status, headers, body)`
的可调用对象；默认 `urllib_transport`。测试里可以注入假的 transport，不访问网络：

```python
def fake_transport(method, url, headers, body, timeout):
    return 200, {"content-type": "application/json"}, b'{"object":"list","data":[]}'

client = DshClient("https://ee.example.com", transport=fake_transport)
assert client.list_models() == []
```

## 测试

```sh
cd sdk/python
python3 -m unittest discover -s tests -v
```

测试全部使用假的 transport，不访问网络。

## 已知边界

- 不支持流式 chat（`urllib` 一次性读 body）；需要流式请用 TypeScript 版。
- 不自动重试；不实现 MCP 的 GET 事件流、resources/prompts、sampling/roots、刷新令牌。
- 不发送 `X-DshEE-Client: dsh-connect/…`（保持第三方 Harness 的默认分页语义）。
- 未连接真实 dsh-ee 部署做端到端验证；契约按服务端 PR #123/#128/#129/#132 核对。

## 示例与兼容性

- 示例（MCP-only Claude Code / Codex、通用 Harness、浏览器 CORS）：
  [`../examples/`](../examples/README.md)
- 与 dsh-ee API 版本、MCP、工具面的兼容矩阵：[`COMPATIBILITY.md`](./COMPATIBILITY.md)
- 各版本变更记录：[`CHANGELOG.md`](./CHANGELOG.md)
- 包元数据：`pyproject.toml` 的 `[project.urls]` 指向
  <https://github.com/hikariming/dsh-ee>；本包**尚未发布**到 PyPI。

## License

Apache-2.0（见仓库根 `LICENSE`）。
