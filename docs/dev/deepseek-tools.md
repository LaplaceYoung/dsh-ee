---
slug: deepseek-tools
category: harness
title: "DeepSeek 工具代理（Files / ASR / TTS / Search / Fetch / OCR）"
title_en: "DeepSeek tool proxy: Files, ASR, TTS, Search, Fetch, OCR"
summary: "用同一个企业 Bearer 令牌调用 DeepSeek 的文件、语音、搜索抓取与 OCR 工具，以及 Anthropic 兼容 Files：代理只把请求转发到配置的上游，媒体与识别文本不落盘、不进 token 统计。"
summary_en: "Call DeepSeek Files, ASR, TTS, Search, Fetch and OCR plus the Anthropic-compatible Files API with the same enterprise Bearer token: the proxy only forwards to the configured upstream, never stores media or transcripts, and does not count these calls as tokens."
tags: [Harness, 模型网关, DeepSeek, Files, ASR, TTS, Search, Fetch, OCR, Anthropic]
updated_at: 2026-11-14
status: draft
since: 2026-11-14
lang: zh-CN
---

## 这是什么

DeepSeek 开放平台把 App 能力拆成了独立工具 API：

- **Files**：上传图片或文档，拿到 `file_id`，之后在对话请求里引用；文档在上传时完成解析。
- **ASR**：把一整段音频转写为文字，适合 Chatbot 的语音输入。
- **TTS**：把文本合成为音频，音频在响应体里边合成边返回。
- **Search / Fetch**：按 query 搜索网页，或抓取单个网页的 Markdown 正文，给 Agent / RAG 用。
- **OCR**：识别 PDF、Word、PowerPoint 或图片中的文字，按页返回结果。
- **Anthropic 兼容 Files**：同一套文件操作也可以用 Anthropic 兼容端点（`/anthropic/v1/files`）调用，方便 Anthropic SDK 生态。

这些接口原生只认 **DeepSeek 上游 API key**。如果每个 Chatbot / 第三方 Harness 都自己拿上游 key，企业就没法统一管住凭据、账号额度和使用范围。dsh-ee 因此提供一组**代理端点**：客户端仍然只带企业的 Bearer 令牌（`ee_at_…` 或网页登录后签发的令牌），dsh-ee 校验权限后，用「模型接入」里配置的上游 BaseURL + 解密出的上游 key 转发请求。

**默认只有管理员与机器人/服务账号能调用这些文件/工具接口**：文件属于上游共享账号、员工之间不隔离，普通员工默认 403，聊天里引用 `file_id` 也会被拦截（见下一节）。

代理只做四件事：

1. 复用模型网关的鉴权（`models` scope + 账号 `llm.use` 权限）；
2. 按部署配置选一个 DeepSeek 上游（见下一节）；
3. 限制请求体大小与超时；
4. 把请求与响应**原样转发**。

它**不解析 multipart/JSON、不落盘、不缓存、不记录文件内容、音频、搜索结果、网页正文或 OCR 识别文本**；文件解析、语音识别/合成、搜索抓取、OCR 本身都由上游完成。

上游字段、格式、限制与错误码以 DeepSeek 文档为准（本站只写代理层的行为）：

- [Files API](https://api-docs.deepseek.com/guides/files_api.md)
- [文件解析](https://api-docs.deepseek.com/guides/document_parsing.md)
- [语音识别 ASR](https://api-docs.deepseek.com/guides/asr.md)
- [语音合成 TTS](https://api-docs.deepseek.com/guides/tts.md)
- [网页搜索与抓取 Search / Fetch](https://api-docs.deepseek.com/guides/search.md)
- [OCR](https://api-docs.deepseek.com/guides/ocr.md)
- [使用 Anthropic API（兼容性细节、鉴权头）](https://api-docs.deepseek.com/guides/anthropic_api.md)

## 上游怎么选

dsh-ee 只使用**管理员在「模型接入」里配置的模型上游**，从不接受客户端传 base URL，也不做模型网关那样的多上游故障切换（`file_id` 绑定在上传时的上游账号上，换上游必然失效，切换没有意义）。

| 环境变量 | 取值 | 默认 | 作用 |
| --- | --- | --- | --- |
| `DSH_EE_DEEPSEEK_PROVIDER` | provider 的 `name` 或数字 `id` | 空 | 指定 Files / ASR / TTS 代理使用的上游 |

选择规则：

- **不设置**：取第一个**启用**且 `vendor=deepseek` 的上游（按「模型接入」列表的优先级、id 排序）。
- **设置为 name**：按名称精确匹配（先 name、name 不中再按数字 id），只匹配**启用**的上游。显式配置时不强制 `vendor=deepseek`：有些企业用自建网关转 DeepSeek，`vendor` 会填 `custom`，由管理员显式绑定即可。
- **找不到 / 未启用 / key 解不开**：接口返回 `503` + OpenAI 错误壳，错误码 `deepseek_provider_unavailable`，客户端应提示管理员检查配置，而不是把它当临时网络错误重试。
- 每次请求都重新解析，管理端改完上游立即生效，不用重启进程。

注意 BaseURL 的拼接方式：Files 转发到 `<base>/files`，ASR/TTS 转发到 `<base>/v1/tools/asr`、`<base>/v1/tools/tts`。所以 BaseURL 应填 `https://api.deepseek.com`（与 DeepSeek 文档一致），**不要**再带 `/v1`；带上会让 ASR/TTS 的路径变成 `/v1/v1/tools/...`。修改 BaseURL 后，之前上传得到的 `file_id` 在新上游上不存在，需要重新上传。

## 谁能调用（默认只允许管理员与机器人）

DeepSeek Files / 工具 API 操作的是**上游账号的共享文件空间**：任何能调用文件接口的人都能列出、查询、删除该上游账号里的所有文件，而 dsh-ee 不按员工隔离文件。因此这些接口默认只对受信账号开放：

| 调用方 | 默认 | 说明 |
| --- | --- | --- |
| 管理员 | ✅ 允许 | 账号带任一管理权限点（`CanAdmin`，含只读管理员）即可 |
| 机器人 / 服务账号 | ✅ 允许 | `account_type=bot` 的账号（`/api/admin/users` 创建） |
| 普通员工 | ❌ 403 | 错误码 `deepseek_tools_forbidden`，覆盖 `/v1/files`、`/anthropic/v1/files` 与 `/v1/tools/*` |
| 普通员工引用 `file_id` | ❌ 403 | 即使知道 `file_id`，在 `/v1/chat/completions` 的 `messages` 里引用 `file` / `document` 内容块也会被拦下，错误码 `file_access_denied` |

需要让普通员工使用时，显式打开开关（默认关闭）：

```sh
# 放开普通员工调用文件/工具接口（也会放开下面「聊天引用 file_id」的拦截）
DSH_EE_DEEPSEEK_TOOLS_ALLOW_EMPLOYEE=true
```

**风险提示（开启前务必评估）**：打开后，任何有 `models` scope + `llm.use` 权限的员工都能使用同一上游账号的文件空间——可以看到并删除所有人上传的文件，`file_id` 之间也没有按人隔离；删除是不可恢复的（上游不提供回收站）。多部门/多团队共用同一上游时，建议不要打开，或按部门拆多个 DeepSeek 上游、用 `DSH_EE_DEEPSEEK_PROVIDER` 分别部署。开关写错值会让启动失败（fail closed）。

聊天引用拦截只覆盖 Chat Completions 入口。Anthropic Messages（`/v1/messages`）在当前 main 上已有，但它的原生透传路径会把 file 块原样转发给上游，本代理没有在该入口加同一个拦截（保持 Anthropic Messages 的既有行为）；Responses API 尚未落地。以后要统一文件访问边界时，需要在各入口复用 `deepseekToolsAllowed` 后再放行 file/document 引用。

## 接口一览

所有端点都需要 `Authorization: Bearer <令牌>`，权限要求与 `/v1/models` 一致：令牌带 `models` scope，且账号有 `llm.use` 权限；并且默认只有管理员与机器人/服务账号可用（见上一节，普通员工 403 `deepseek_tools_forbidden`）。`/v1/*` 端点共享每令牌限流（`DSH_EE_RATE_LIMIT`），默认每分钟 60 次。Anthropic 兼容端点还接受 Anthropic SDK 默认的 `x-api-key`（等价 Bearer，不会转发给上游）。

| 方法 | dsh-ee 路径 | 转发到上游 | 请求体 | 说明 |
| --- | --- | --- | --- | --- |
| `POST` | `/v1/files` | `POST <base>/files` | multipart，上限 100 MiB | 上传文件；响应是上游文件对象（`file_id`） |
| `GET` | `/v1/files` | `GET <base>/files` | 无 | 列出上游账号里的文件；查询串 `after` / `limit` / `order` / `purpose` 原样转发 |
| `GET` | `/v1/files/{id}` | `GET <base>/files/{id}` | 无 | 查询单个文件 |
| `DELETE` | `/v1/files/{id}` | `DELETE <base>/files/{id}` | 无 | 删除单个文件 |
| `POST` | `/v1/tools/asr` | `POST <base>/v1/tools/asr` | multipart，上限 8 MiB | 整段音频转写；响应是 `{request_id, text, duration_ms}` |
| `POST` | `/v1/tools/tts` | `POST <base>/v1/tools/tts` | JSON，上限 64 KiB | 文本合成音频；音频在响应体里流式回传 |
| `POST` | `/v1/tools/search` | `POST <base>/v1/tools/search` | JSON，上限 64 KiB | 网页搜索，返回结构化结果数组 |
| `POST` | `/v1/tools/fetch` | `POST <base>/v1/tools/fetch` | JSON，上限 64 KiB | 抓取单个网页，返回 Markdown 正文 |
| `POST` | `/v1/tools/ocr` | `POST <base>/v1/tools/ocr` | multipart，上限 100 MiB | 文档/图片 OCR；换行保活后返回 JSON |
| `POST` | `/anthropic/v1/files` | `POST <base>/anthropic/v1/files` | multipart，上限 100 MiB | Anthropic 兼容上传；需 `anthropic-beta` 头 |
| `GET` | `/anthropic/v1/files` | `GET <base>/anthropic/v1/files` | 无 | Anthropic 兼容文件列表；查询串原样转发 |
| `GET` | `/anthropic/v1/files/{id}` | `GET <base>/anthropic/v1/files/{id}` | 无 | Anthropic 兼容查询单个文件 |
| `DELETE` | `/anthropic/v1/files/{id}` | `DELETE <base>/anthropic/v1/files/{id}` | 无 | Anthropic 兼容删除单个文件 |

大小上限是 dsh-ee 的请求边界，比上游宽松的按类型限制更粗：例如 Files 统一允许 100 MiB，上游再按 `purpose` 校验图片 64 MiB / 文档 100 MiB / 纯文本等 32 MiB。超限由 dsh-ee 直接返回 `413`，不会先读完请求体、也不会转发到上游。

### Files

上传请求按 multipart **逐字节原样转发**，dsh-ee 不解析字段，因此：

- 上游要求的字段顺序不受影响：上传文档时 `purpose` 必须排在 `file` 前面，代理不会重排。
- `Content-Type`（含 boundary）原样带给上游，Authorization 换企业上游 key。
- 表单字段以 DeepSeek Files API 为准：`file`、`purpose`（`user_data` / `text_recognition` / `rich_layout_recognition`）、可选的 `expires_after[anchor]` 与 `expires_after[seconds]`。

未知路径参数不会有影响，因为列表/查询/删除都只把查询串与（校验过的）`file_id` 拼到固定路径上：

- `file_id` 只允许字母、数字、点、下划线、短横线，且不能以点开头；编码过的 `/`、`..` 一律 `400 invalid_file_id`，不会被拼进 URL 带偏请求。
- 查询串原样转发给上游，dsh-ee 不解析分页/排序/过滤参数。

### ASR

`POST /v1/tools/asr` 的 multipart 请求体原样转发（`Content-Type` 含 boundary），响应 JSON 原样返回。上游限制：16 位 PCM WAV 或 Ogg Opus、≤8 MiB、≤120 秒、上传需在 10 分钟内完成；没有语种参数，语种自动识别。格式不支持等错误由上游返回 `400`，代理原样透传。

### TTS

`POST /v1/tools/tts` 的请求体是 JSON（`text`、`voice_id`、`format`、`filter_markdown`），代理统一以 `application/json` 转发。成功的响应是音频流：dsh-ee 边收边回传并逐块 flush，`Content-Type` 原样透传（`wav` 为 `audio/wav`，`pcm` 为 24 kHz、16 位、单声道原始 PCM），不会攒完整段音频再返回。上游的 `429` / `503` / `504` 等错误状态与错误体原样透传。

### Search / Fetch

`POST /v1/tools/search` 与 `POST /v1/tools/fetch` 的 JSON 请求体原样转发（Search 的 `query` / `count` / `need_content` 等搜索语法、Fetch 的 `url` 都由上游解释），响应 JSON 原样返回。两者都是只读接口，dsh-ee 不解析 query、不校验 URL、不缓存结果。上游的错误格式本身就是 OpenAI 兼容的（含 `param: null`），状态码与错误体原样透传：搜索/抓取失败、没抓到正文（`404`）、被安全策略拒绝（`403`）、余额不足（`402`）、限速（`429`）等都按上游返回。

Search / Fetch 分别有账号级 QPS 限制（上游默认 Search 100、Fetch 25），dsh-ee 不额外排队或重试。

### OCR

`POST /v1/tools/ocr` 的 multipart 请求体原样流式转发（只允许 `file` 与 `purpose` 两个字段，传其他字段由上游 400）。上游识别期间会**持续发送换行符保活**，识别完成后才返回 JSON；dsh-ee 逐块 flush 原始字节与 `Content-Type`，客户端收到的响应体仍是「前导换行 + JSON」（换行属于 JSON 前导空白，可直接按 JSON 解析）。

注意 OCR 的错误语义：**识别开始后 HTTP 状态码已经是 `200`**，失败/超时时响应体不是识别结果而是一个错误对象（`{"error":{...}}`），客户端必须检查响应体里有没有 `error`；dsh-ee 只负责原样透传，不把它改写成 HTTP 错误。上传阶段（识别开始前）的错误仍是 HTTP 4xx/5xx，按状态码透传。

### Anthropic 兼容 Files

同一套文件操作也可以走 Anthropic 兼容端点：

| 方法 | dsh-ee 路径 | 转发到上游 |
| --- | --- | --- |
| `POST` | `/anthropic/v1/files` | `POST <base>/anthropic/v1/files` |
| `GET` | `/anthropic/v1/files` | `GET <base>/anthropic/v1/files`（查询串 `after_id` / `before_id` / `limit` 原样转发） |
| `GET` | `/anthropic/v1/files/{id}` | `GET <base>/anthropic/v1/files/{id}` |
| `DELETE` | `/anthropic/v1/files/{id}` | `DELETE <base>/anthropic/v1/files/{id}` |

- **`anthropic-beta` 头**：DeepSeek 要求这些端点携带 `anthropic-beta: files-api-2025-04-14`。dsh-ee **不校验、不补默认值**，只把客户端传来的 `anthropic-beta` / `anthropic-version` 原样转发；缺失时由上游返回 `400`（错误体原样透传）。`anthropic-version` 上游会忽略，转发性上无害。
- **鉴权**：Anthropic SDK 默认用 `x-api-key`。dsh-ee 对这四个端点两种都接受——`Authorization: Bearer <企业令牌>` 或 `x-api-key: <企业令牌>`（等价）；dsh-ee **不会**把客户端的 `x-api-key` 转发给上游，上游拿到的始终是配置的上游 key。
- **响应形状**：Anthropic 兼容 Files 的对象形状与 OpenAI 兼容 Files 不同（`type` 而非 `object`、`size_bytes`、RFC 3339 的 `created_at`、`mime_type`；列表没有顶层 `object`；删除返回 `type: "file_deleted"`），dsh-ee 都原样透传。
- **错误形状**：上游返回的是 Anthropic 形状 `{"type":"error","error":{...}}`，原样透传；dsh-ee 自己产生的错误（没有可用上游、413、上游不可达）仍用 OpenAI 错误壳，客户端需要按状态码分别处理。
- 这不是 Anthropic Messages 的代理：`/v1/messages` 不在本代理范围内，这里只代理 Files。

### 错误语义

- dsh-ee 自己产生的错误用 **OpenAI 错误壳**：`{"error":{"message","type","code"}}`。稳定错误码：
  - `403 deepseek_tools_forbidden`：普通员工调用文件/工具接口，且没有开启 `DSH_EE_DEEPSEEK_TOOLS_ALLOW_EMPLOYEE`；
  - `403 file_access_denied`：普通员工在聊天里引用 `file_id`（file/document 内容块）；
  - `503 deepseek_provider_unavailable`：没有可用的 DeepSeek 上游；
  - `413 request_too_large`：超过上面的请求体上限；
  - `400 invalid_file_id`：`file_id` 含不允许的字符；
  - `502 upstream_unreachable` / `504 upstream_timeout`：连不上上游 / 上游在超时内没响应。
- **上游返回的状态码与错误体原样透传**（DeepSeek 的错误体本身就是 OpenAI 形状）。所以上游的 `401` / `403` 通常表示**企业配置的上游 key 无效或没有权限**，不是调用者手里的 dsh-ee 令牌有问题。
- Search / Fetch / ASR / TTS / OCR 的上游错误状态码与错误体原样透传（含 `400/402/403/404/408/413/429/5xx/504` 等）；OCR 开始识别后的错误在 `200` 响应体里（`error` 字段），见上文。
- Anthropic 兼容 Files 的上游错误体是 Anthropic 形状（`{"type":"error","error":{...}}`），dsh-ee 原样透传；dsh-ee 自身错误仍是 OpenAI 壳。
- 代理不做自动重试：非幂等的上传/删除重试可能在上游产生重复文件或重复计费，交给客户端决定；Search/Fetch/OCR 的 429/5xx 由客户端按上游建议退避。
- 上游开始返回音频后如果合成失败，连接会中途断开；客户端应把提前结束的响应视为失败（与 DeepSeek 原生行为一致）。OCR 在客户端断开后上游仍会继续识别并照常计费（与 DeepSeek 原生行为一致）。

### 超时

| 操作 | dsh-ee 超时 |
| --- | --- |
| 上传文件（`POST /v1/files`） | 10 分钟 |
| 列出 / 查询 / 删除文件 | 30 秒 |
| ASR | 2 分钟 |
| TTS（含音频流式回传） | 10 分钟 |
| Search | 30 秒 |
| Fetch | 60 秒 |
| OCR（上传 + 识别 + 回传） | 5 分钟 |
| Anthropic 兼容 Files 上传 | 10 分钟 |
| Anthropic 兼容 Files 列表 / 查询 / 删除 | 30 秒 |

上传文档时上游要「上传 + 解析」完成才返回（文档解析最多 120 秒），客户端与中间反向代理的超时应按上传时间加解析时间设置，不要短于这里的上限。

## 计费、文件归属与限额

### 计费在上游，不进 dsh-ee 的 token 统计

这些调用按 DeepSeek 的**工具计费**规则记在企业配置的上游账号上，不是 dsh-ee 的 token 额度：

- Files / Anthropic 兼容 Files：文档解析在上传成功时按页数一次性计费；纯文本/表格/电子书上传不计费。之后在对话里引用 `file_id`，拼入上下文的输入 token 按普通对话计入 `usage`（这部分和普通模型调用一样会进入 dsh-ee 的 token 统计）。
- ASR：按音频时长计费（精确到秒）。
- TTS：按计费字符数计费。
- Search / Fetch：按**成功请求次数**计费（每 1,000 次计价），不按 token 或结果条数；失败、无结果、超时不计费。
- OCR：按**成功识别的页数**计费（`page_count`，每 1,000 页计价），失败或超时不计费；响应里的 `usage` 只是参考，不用于计费。

dsh-ee 不会为 `/v1/files` 与 `/v1/tools/*` 写 `usage_logs`，所以：

- 这些调用**不消耗**账号的月度 token 额度，也不会出现在用量报表和 `/metrics` 的 token 统计里；
- 成本只受上游账号余额/限速约束；需要控制花费时，在上游账号或 `DSH_EE_DEEPSEEK_PROVIDER` 指向的上游上做限制；
- 反过来，token 额度用完不影响工具调用，工具调用也不会让 token 额度「看起来」变少。

### 文件归属上游账号，`file_id` 绑定所选上游

- 文件属于**上游 DeepSeek 账号**，同一账号下的 API key 共享文件。dsh-ee 不按员工隔离文件：能调用这些接口的令牌都能看到并删除该上游账号里的文件（`GET /v1/files` / `GET /anthropic/v1/files` 会列出账号里所有文件）。Anthropic 兼容 Files 与 OpenAI 兼容 Files 归属同一个上游账号。多部门共用同一上游时，请把它当成共享空间使用。
- `file_id` 与**上传时的上游**绑定。改了 `DSH_EE_DEEPSEEK_PROVIDER`、停用或删除该上游后，旧 `file_id` 在新上游不存在，接口会返回上游的 `404`，需要重新上传。
- 文件生命周期由上游管理：上传时可用 `expires_after` 设置 1 小时到 30 天有效期（不传则永久有效）；也可以用列表/删除接口清理。dsh-ee 不保存任何文件副本，删除后 `file_id` 立即失效、上游不提供回收站，需要重新上传。
- 默认访问边界（见「谁能调用」）：只有管理员与机器人能列/删文件；开启员工开关后所有允许的员工共享同一空间。

### 审计

文件的写/删/列出会记入审计日志（管理员在「审计日志」页按「DeepSeek 文件」分类查看），复用现有审计表与 i18n：

| 动作 | 触发 | target | 记录内容 |
| --- | --- | --- | --- |
| `files.upload` | `POST /v1/files`、`POST /anthropic/v1/files` 完成（无论成功失败） | 成功时是上游返回的 `file_id`，失败为空 | `actor`（操作人）、`token_id`（令牌 id）、`api`（openai / anthropic）、`status`（上游状态码） |
| `files.delete` | `DELETE /v1/files/{id}`、`DELETE /anthropic/v1/files/{id}` | `file_id` | 同上 |
| `files.list` | `GET /v1/files`、`GET /anthropic/v1/files` | `files` | 同上 |

审计**只记录 `file_id` 与状态码**，绝不记录文件名、文件内容、音频、OCR/转写文本或响应正文；被访问边界拒绝的请求（403）不会打到上游，也不写文件审计。单文件查询（`GET .../{id}`）目前不写审计。

### 不落盘、不缓存、不记内容

- 上传/下载都在内存/流中转发，不写 `DSH_EE_DATA_DIR`；搜索/抓取结果与 OCR 识别文本同样不落盘；
- 所有代理响应带 `Cache-Control: no-store`，文件元数据、转写文本、搜索结果、网页正文、OCR 文本与合成音频不会被反向代理/中间层缓存；
- 访问日志只有方法、路径、状态码、耗时与请求号，不含请求体、文件内容、音频或令牌；错误日志只带上游名称与错误摘要。
- 反向代理层如需限制请求体大小，记得放宽到 ≥100 MiB，否则在 dsh-ee 之前就会被拒。

## 启用方式

在「模型接入」里准备一个启用中的 DeepSeek 上游（BaseURL 如 `https://api.deepseek.com`，key 填上游 API key），然后按需指定：

```sh
# 不设置：自动选第一个启用的 vendor=deepseek 上游
# 指定 name（推荐，改完立即生效）
DSH_EE_DEEPSEEK_PROVIDER=deepseek-main
# 或指定数字 id
#DSH_EE_DEEPSEEK_PROVIDER=3
```

`deploy/systemd/dsh-ee.env.example` 与 `server/.env.example` 里有对应注释。非法超长值让启动失败，其余写错的情况启动不受影响，请求时按 `503 deepseek_provider_unavailable` 返回。

需要普通员工也能使用这些接口时再加（默认关闭，风险见「谁能调用」）：

```sh
DSH_EE_DEEPSEEK_TOOLS_ALLOW_EMPLOYEE=true
```

调用示例（与 DeepSeek 原生接口同形，只把地址换成 dsh-ee、key 换成企业令牌）：

```sh
# 上传文件
curl https://dsh.acme.internal/v1/files \
  -H "Authorization: Bearer $DSH_EE_TOKEN" \
  -F purpose="user_data" \
  -F file="@image.jpg"

# 语音识别
curl https://dsh.acme.internal/v1/tools/asr \
  -H "Authorization: Bearer $DSH_EE_TOKEN" \
  -F file="@speech.wav"

# 语音合成
curl https://dsh.acme.internal/v1/tools/tts \
  -H "Authorization: Bearer $DSH_EE_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"text":"你好，我是 DeepSeek。","voice_id":"stella"}' \
  --output speech.wav

# 网页搜索 / 抓取
curl https://dsh.acme.internal/v1/tools/search \
  -H "Authorization: Bearer $DSH_EE_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"query":"DeepSeek V4 模型 发布","count":5}'
curl https://dsh.acme.internal/v1/tools/fetch \
  -H "Authorization: Bearer $DSH_EE_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"url":"https://example.com/article"}'

# OCR
curl https://dsh.acme.internal/v1/tools/ocr \
  -H "Authorization: Bearer $DSH_EE_TOKEN" \
  -F purpose="text_recognition" \
  -F file="@document.pdf"

# Anthropic 兼容 Files（必须带 anthropic-beta；鉴权可用 Bearer 或 x-api-key）
curl https://dsh.acme.internal/anthropic/v1/files \
  -H "x-api-key: $DSH_EE_TOKEN" \
  -H "anthropic-beta: files-api-2025-04-14" \
  -F purpose="user_data" \
  -F file="@image.jpg"
```

## 边界（暂不做）

- 不做每员工/每部门的文件隔离与配额：文件在上游账号里共享；需要隔离请给不同部门配不同上游，并用 `DSH_EE_DEEPSEEK_PROVIDER` 分开部署。
- 不做 ASR 流式识别、TTS WebSocket 流式合成、文件内容下载（上游本身不提供原件/解析结果下载）；不做 Search 的图片搜索（上游暂不支持，只能经联网工具）。
- 不代理 Anthropic Messages（`/v1/messages`）与 Responses 协议；Anthropic 兼容范围只有 Files（`/anthropic/v1/files`）。Anthropic Messages 已在 main 上，但本代理不改变它的行为，也没有在它的入口加 file/document 引用拦截；Responses 尚未落地。以后落地 / 统一边界时，要在各自入口复用 `deepseekToolsAllowed` 校验 file/document 引用（本代理只覆盖 Chat Completions 的拦截）。
- 不解析文件/JSON 内容、不校验文件格式与 URL、不做内容审核：格式/大小/安全审核/搜索策略都由上游按 DeepSeek 文档执行。
- 不做自动重试与多上游故障切换；`file_id` 不跨上游迁移。
- 不把工具调用写进 token 用量统计（见上一节）；OCR/ASR/TTS/Search/Fetch 的成本全部落在上游账号。
