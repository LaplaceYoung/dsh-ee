# Anthropic Messages compatibility (POST /v1/messages)

Beyond the OpenAI-compatible gateway, dsh-ee also offers a model-calling entry in Anthropic Messages format: Claude Code, the Anthropic SDK,
or any client written against the Anthropic Messages protocol can use **dsh-ee's unified access token** to call the models the company has connected directly.
Requests are translated into a single OpenAI chat completions call and handed to the existing gateway, so model permissions, account/token monthly quota, upstream failover,
usage accounting and auditing are exactly the same as for the OpenAI gateway.

**v0 covers**: non-streaming and streaming (SSE) `POST /v1/messages`, auth, request/response envelopes and error envelopes.
**Not covered**: `count_tokens`, Batches, Files, full beta headers, and execution of server-side tools (`web_search` etc.).

## 1. Connect with Claude Code

Treat dsh-ee as the Anthropic API base URL, and use dsh-ee's **public model names** as model names (the `id` returned by `GET /v1/models`,
not the upstream vendor's name):

```sh
export ANTHROPIC_BASE_URL='https://dsh-ee.example.com'
export ANTHROPIC_API_KEY='ee_at_…'            # 客户端发 x-api-key
# 或者：export ANTHROPIC_AUTH_TOKEN='ee_at_…'   # 客户端发 Authorization: Bearer

# 关键：Claude Code 默认的 claude-* 模型名在 dsh-ee 里不存在，必须显式指定对外模型名
export ANTHROPIC_MODEL='company-chat'
export ANTHROPIC_DEFAULT_HAIKU_MODEL='company-chat'
export ANTHROPIC_DEFAULT_SONNET_MODEL='company-chat'
export ANTHROPIC_DEFAULT_OPUS_MODEL='company-chat'
```

> dsh-ee **does not** map `claude-*` names: the model name must be one of the models available to the account/token, otherwise you get a 403.
> Anthropic-compatible layers from vendors such as DeepSeek automatically map claude names to their own models; dsh-ee's models are multi-vendor models
> connected by administrators and have no single mapping target, so it explicitly requires public model names.

There are two equivalent spellings of the base URL (same handler; behavior, auth and error envelopes are exactly the same):

| `ANTHROPIC_BASE_URL` | What the client actually requests | Notes |
|---|---|---|
| `https://dsh-ee.example.com` | `POST /v1/messages` | Standard Anthropic base URL form |
| `https://dsh-ee.example.com/anthropic` | `POST /anthropic/v1/messages` | Matches DeepSeek's Anthropic base URL convention (`<base>/anthropic`) |

You can also verify it directly with curl:

```sh
curl -fsS "$BASE/v1/messages" \
  -H "x-api-key: $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "company-chat",
    "max_tokens": 256,
    "messages": [{"role": "user", "content": [{"type": "text", "text": "ping"}]}]
  }'

# 同一个端点的别名（DeepSeek 风格 base URL：ANTHROPIC_BASE_URL=$BASE/anthropic）
curl -fsS "$BASE/anthropic/v1/messages" ...  # 参数与上面完全相同
```

## 2. Authentication and permissions

- Credentials: `x-api-key: <token>` or `Authorization: Bearer <token>`; the two are equivalent, and `x-api-key` wins when both are sent.
- Tokens: the unified access token `ee_at_…` (the legacy `sk-ee-…` / `ee_npm_…` are also accepted, with the same behavior as the OpenAI gateway).
- Permissions: the token needs the `models` scope, the account needs the "Call models" (`llm.use`) permission, and the model must be within the account's and the token's available range.
- Quota: the account/token monthly quota and quota expiry are shared with the OpenAI gateway; rejected requests are not forwarded upstream and are not accounted.
- Rate limiting: shares the fixed-window per-token rate limit with `/v1/*` (60 requests per minute by default, adjusted with `DSH_EE_RATE_LIMIT`).
  Requests carrying only `x-api-key` are still counted against the token's quota and do not fall back to counting by IP; over-limit requests also return the Anthropic error envelope and `Retry-After`.
- `anthropic-version` / `anthropic-beta`: the DeepSeek native passthrough path passes these two headers **unchanged** to the upstream (whether they are supported is up to DeepSeek; dsh-ee does not parse them); the fallback conversion path for other providers ignores them without reporting an error.
- **The Files API is not in v0 scope**: sending `anthropic-beta: files-api-2025-04-14` still does not make dsh-ee implement Files; file references with `image.source.type=file` on the fallback conversion path return 400 (images only support `base64` / `url`), while on the native passthrough path file/vision blocks are handed to DeepSeek unchanged; the Files / Batches endpoints return 404.
- Service discovery: `models.anthropic_messages_endpoint` in `GET /.well-known/dsh-ee` gives this endpoint's address,
  and `GET /v1/openapi.json` also contains the request/response/error descriptions for `POST /v1/messages`.

## 3. Upstream forwarding: DeepSeek native passthrough / conversion fallback for other providers

dsh-ee decides how a request reaches the upstream based on the candidate routes of the public model (the candidate order is the priority in `/admin/providers`):

| `provider.vendor` of the first candidate route | Forwarding mode | What dsh-ee does |
|---|---|---|
| `deepseek` | **Native passthrough**: `POST <provider.base>/anthropic/v1/messages` | Only auth, model permission, quota, rate limiting and `model` rewriting (public name → upstream name); passes `anthropic-version` / `anthropic-beta` through unchanged; forwards the remaining request body fields (server tools, citations, safety, title, latest_reminder, file/vision blocks, etc.) unchanged; forwards successful JSON/SSE responses **byte for byte**; parses usage out-of-band for accounting (estimates when usage is absent); forwards upstream errors with the original status/body. |
| Others | **Conversion fallback**: Anthropic Messages → OpenAI chat completions | See §4 "Request conversion" and §5 "Response and streaming"; `anthropic-version` / `anthropic-beta` are ignored; unsupported blocks return 400 or are ignored. |

- Under native passthrough, dsh-ee **does not validate** generation parameters such as `max_tokens` / `temperature` (that is left to DeepSeek) and does not parse content blocks; monthly quota, per-token rate limiting and model permissions are still enforced by dsh-ee.
- **Failover only happens within the same protocol**: if the first route is `deepseek`, it only switches among other `deepseek` routes; otherwise it only switches among conversion routes of other vendors, and **never across protocols**; if all fail, the last route's error is returned. When one public model has several routes, the first one (highest priority) decides the protocol.
- Native passthrough also supports the `/anthropic/v1/messages` alias; `x-api-key` / Bearer authentication is exactly the same as in the conversion layer.
- The discovery document's `models.anthropic_messages_native_vendors` (default `["deepseek"]`) and `models.anthropic_messages_conversion_fallback` (`openai-chat`) announce this policy.

## 4. Request conversion (fallback conversion path: Anthropic → OpenAI chat)

| Anthropic field | Handling |
|---|---|
| `model` | Checks permission against the dsh-ee public model name and replaces it with the upstream model name when forwarding |
| `max_tokens` | Required positive integer; the per-request limit matches the OpenAI gateway (32768); over the limit returns 400 |
| `system` | A string or an array of text blocks is merged into a single OpenAI `system` message |
| `messages[].content` | A string passes through unchanged; content blocks are converted per the table below |
| `temperature` / `top_p` | Passed through unchanged (`temperature` range is decided by the upstream) |
| `top_k` | OpenAI chat has no corresponding parameter; ignored |
| `stop_sequences` | The array is converted to OpenAI `stop` |
| `tools` | Client tools `{name, description, input_schema}` → `{type:"function", function:{name, description, parameters}}` |
| `tool_choice` | `auto`→`auto`, `any`→`required`, `none`→`none`, `tool`→the specified function; `disable_parallel_tool_use`→`parallel_tool_calls=false` |
| `metadata.user_id` | Converted to OpenAI's `user` so the upstream can rate-limit/isolate by end user |
| `thinking` | `enabled` / `adaptive` → OpenAI extension parameter `thinking:{"type":"enabled"}`; `disabled` → `thinking:{"type":"disabled"}`; `budget_tokens` ignored |
| `reasoning_effort` / `output_config.effort` | Converted to OpenAI `reasoning_effort`; `none` turns thinking off; `xhigh` / `max` / `ultra` normalize to `high` (range `minimal/low/medium/high`) |
| `stream` | Forwards as streaming; automatically adds `stream_options.include_usage=true` for accounting |
| `container` / `mcp_servers` / `service_tier` / `top_k` / `cache_control` | Ignored |

Content blocks:

| Block type | Position | Conversion |
|---|---|---|
| `text` | user / assistant | Text (multiple text blocks in one message are joined with newlines) |
| `image` | user | `source.type=base64` / `url` → OpenAI `image_url` content part (the upstream must support vision) |
| `tool_use` | assistant | → OpenAI `tool_calls` (`input` serialized into an `arguments` JSON string) |
| `tool_result` | user | → OpenAI `role:"tool"` message (`tool_use_id` → `tool_call_id`; content flattened into text, images replaced by `[image]` placeholders) |
| `thinking` | assistant | → the assistant message's `reasoning_content` (upstreams such as DeepSeek require it back in follow-up turns with tools) |
| `redacted_thinking` | assistant | Cannot be restored to text; ignored |
| `document` and other blocks | — | 400 `invalid_request_error` (v0 does not support file/document blocks) |

Server-side tools (tools executed by Anthropic, whose `type` is `web_search_20250305`, `bash_20250124`, etc.) are **ignored**
rather than failing the whole request — this keeps other Claude Code features usable, but web search/code execution tools do not actually take effect.

## 5. Response and streaming (fallback conversion path)

A non-streaming response is a standard Anthropic `message`: `id` (`msg_…`), `type:"message"`, `role:"assistant"`,
`model` (the public model name requested by the client), the `content` block array, `stop_reason`, `stop_sequence` and `usage`.

- `content` order: `thinking` (when the upstream returns `reasoning_content` / `reasoning`) → `text` → `tool_use`.
  The `signature` of a thinking block is an opaque placeholder generated by dsh-ee (the server does not verify signatures); the client can send it back unchanged.
- `stop_reason`: tool calls → `tool_use`; `finish_reason=length` → `max_tokens`; `content_filter` → `refusal`;
  anything else → `end_turn`. OpenAI cannot distinguish a `stop_sequences` hit, so `stop_sequence` is never returned.
- `usage`: `output_tokens` takes the upstream `completion_tokens`; `input_tokens = prompt_tokens - cache hits`,
  `cache_read_input_tokens` takes the upstream `prompt_tokens_details.cached_tokens` (0 when the field is absent),
  and `cache_creation_input_tokens` is fixed at 0. **Accounting** still counts the upstream `prompt_tokens + completion_tokens` against the monthly quota.

Streaming uses `Content-Type: text/event-stream`, with this event sequence (the same as Anthropic's official one):

```text
event: message_start          # message 骨架，usage.input_tokens 是估算值
event: content_block_start    # 依次出现 thinking / text / tool_use 块
event: content_block_delta    # thinking_delta / text_delta / input_json_delta / signature_delta
event: content_block_stop
event: message_delta          # delta.stop_reason + usage（最终用量，以这里为准）
event: message_stop
```

- OpenAI's `data: [DONE]` is not sent; a connection that drops before `message_stop` means the response is incomplete.
- `message_start.usage.input_tokens` is an estimate from the request body (the upstream usage is not known when the stream starts);
  the `input_tokens` / `output_tokens` in `message_delta.usage` are the final values.
- When the upstream gives no streaming usage (for example, the client stops midway), dsh-ee estimates accounting from the request and the content already forwarded, so quota is not undercounted.

## 6. Errors

The error body has the Anthropic shape, with HTTP status codes corresponding to `error.type`:

```json
{"type":"error","error":{"type":"rate_limit_error","message":"请求过于频繁，请稍后重试"}}
```

| HTTP | `error.type` | Trigger |
|---|---|---|
| 400 | `invalid_request_error` | Request body is not JSON, missing `model` / `max_tokens`, parameter out of range, unsupported block type |
| 401 | `authentication_error` | Missing or invalid token |
| 403 | `permission_error` | Token lacks the `models` scope, account lacks `llm.use`, model not in range, quota expired |
| 404 | `not_found_error` | The public model has no available upstream; compatible endpoints that are not implemented, such as `count_tokens` |
| 413 | `request_too_large` | Request body exceeds 16 MiB |
| 429 | `rate_limit_error` | Account/token monthly quota exhausted, per-token request rate limit, upstream returned 429 |
| 5xx | `api_error` / `overloaded_error` | All upstream routes unavailable, upstream 5xx, upstream response cannot be parsed |

`/v1/messages/count_tokens` authenticates first and then returns 404 `not_found_error` (not implemented in v0),
rather than falling through to the OpenAI error envelope at `/v1/`.

On the DeepSeek native passthrough path, errors returned by the upstream (DeepSeek itself uses the Anthropic error envelope) are forwarded with the **original status code and body**
and are not re-wrapped; errors produced by dsh-ee itself (auth/permission/quota/rate limit/no upstream for the model/upstream unreachable) still use the table above.

## 7. Known limits (v0)

- Only `POST /v1/messages` (and the alias `POST /anthropic/v1/messages`) is implemented; `count_tokens` (including its alias),
  Batches, Files, `/v1/messages/batches` and the like return 404.
- Server-side tools are not executed (see §3); `container`, `mcp_servers`, `service_tier` and `top_k` are ignored.
- No `claude-*` model name mapping: the client must configure the model name as a public model name from `/v1/models`.
- Thinking content is recognized through the upstream `reasoning_content` / `reasoning` fields; when the upstream returns no thinking content,
  the response has no `thinking` block. `budget_tokens` has no effect; control thinking depth with `reasoning_effort` / `output_config.effort`.
- Images are converted to OpenAI's `image_url` content part, and whether the upstream model supports images is up to the upstream; file blocks such as `document` are not supported.
- A `stop_sequences` hit and a normal finish are both `finish_reason=stop` upstream, so they cannot be distinguished and both return `end_turn`.
- No signature verification: the `signature` of a thinking block is a placeholder value, and the server neither verifies nor relies on it.
- The DeepSeek native passthrough path does not validate generation parameters or parse content blocks in dsh-ee; limits such as `max_tokens` are decided by DeepSeek;
  upstream errors are forwarded unchanged; responses on the native path do not go through dsh-ee's Anthropic conversion layer.
- Failover does not cross protocols: once the higher-priority route decides the protocol, a lower-priority route using the other protocol is never tried.

## 8. Related

- OpenAI-compatible gateway, tokens and scopes: [Harness access](https://github.com/hikariming/dsh-ee/blob/main/docs/dev/harness.md)
- Full third-party client manual and error table: [l1-client.md](https://github.com/hikariming/dsh-ee/blob/main/docs/l1-client.md)
- Model gateway error code index: [errors.md](https://github.com/hikariming/dsh-ee/blob/main/docs/errors.md)
- Implementation: `server/internal/api/anthropic.go`, `server/internal/api/anthropic_convert.go`
