# Connect a third-party Harness or your own client

## The five-minute path (H1: just `/mcp`)

For a third-party Harness (Claude Code, Codex, your own Agent, scripts) connecting to dsh-ee, the shortest path is **H1: one `ee_at_` token + `/mcp`**. Follow the repository docs for H2 when you need skill files, a local extension on the machine, or a single command that generates the config; the DSH-only deep integration (H3) is not offered for third-party clients, so do not design an integration around H3.

| Step | What to do | Where to read |
| --- | --- | --- |
| 1 | Create an `ee_at_` token in "My tokens"; keep scopes minimal for the purpose | "Tokens and scopes" below |
| 2 | Read the anonymous discovery document `GET /.well-known/dsh-ee` and confirm `api_version`, the device code endpoint, capabilities and limits | "Discovery and version" |
| 3 | Call `/v1/models` and `/v1/chat/completions` with the `models` scope | "Model gateway" |
| 4 | Connect to `/mcp` with the `connections` scope and call `tools/list` dynamically per session | "MCP" |
| 5 | Write tools go through server-side confirmation: first get the "confirm on the web" link, then call again with identical parameters after the employee approves | "Write operations" |

## Tokens and scopes

Tokens start with `ee_at_` and map to an individual user; scopes are only an upper bound — every request is additionally intersected with the account's permissions on the server (if the account does not have the permission code, the token still cannot use it).

| Purpose | Required scope | Notes |
| --- | --- | --- |
| Call models | `models` | `/v1/models`, `/v1/chat/completions`; the account also needs the "Call models" permission |
| Use company connection, knowledge base and app data tools through `/mcp` | `connections` | The `/mcp` entry checks `connections` |
| Use the `ee_*` Workbench tools in `/mcp` | `connections` + `agent` | The global Agent switch in system settings must also be on |
| Read the capability directory (optional) | `hub` | Apps and revisions in `/v1/apps` and `/v1/capabilities` |

Treat tokens like passwords: read them from environment variables or a credential store, never put them in repositories, logs or screenshots; issue one per integrator and revoke per person.

## Discovery and version

The anonymous endpoint `GET /.well-known/dsh-ee` returns `api_version`, the device code endpoint, `scopes_supported`, model/MCP/app capabilities, request size limits and the deprecation policy. `GET /v1/capabilities` (Bearer optional) returns the models, tool counts and revisions visible to the current token; both responses carry the `Dsh-EE-API-Version: 1` header.

Device code sign-in is a public optional entry: `POST /api/device/code` returns `device_code` / `user_code`; after the employee confirms on the `/device` page, poll `POST /api/device/token` at the returned `interval`. A device code is valid for 10 minutes and the token can only be fetched once; there is no refresh token, so restart the flow after expiry.

## Model gateway (`/v1`)

OpenAI-compatible; minimal call:

```sh
export BASE='https://<dsh-ee>'
export TOKEN='ee_at_…'

curl -fsS "$BASE/v1/models" -H "Authorization: Bearer $TOKEN"

curl -fsS "$BASE/v1/chat/completions" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"model":"<模型名>","messages":[{"role":"user","content":"你好"}]}'
```

Model names follow what `/v1/models` returns; streaming, rate limits and error codes follow the OpenAI-compatible conventions.

## MCP (`/mcp`)

`/mcp` is Streamable HTTP + JSON-RPC 2.0: `POST /mcp` sends requests with `Accept: application/json, text/event-stream`; the `Mcp-Session-Id` returned by `initialize` is used for subsequent requests.

```sh
curl -fsS "$BASE/mcp" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"my-harness","version":"1.0"}}}'
```

| Tool prefix | Source |
| --- | --- |
| `<connection-name>__<tool-name>` | Tools in company connections |
| `kb__*` | Knowledge base tools |
| `app__*` / `data__*` | Tool surfaces of micro-apps |
| `ee_*` | Workbench tools (need the `agent` scope and the global Agent switch) |

Visible tools depend on the permissions of the token's owner, so different employees may see different results: **call `tools/list` dynamically in each session, never hard-code it**. Treat a tool as a write tool when `annotations.readOnlyHint` is missing.

## Write operations: server-side confirmation

By default (`non_dsh`), a third-party Harness cannot execute write tools directly: `tools/call` returns `isError: true` and a "confirm on the web" link. After the employee checks the parameters and approves on the `/confirm/<id>` page, call again with **exactly the same parameters** to actually execute; one approval works once, is valid for 10 minutes, and any changed parameter requires a new confirmation. DSH clients may bypass this according to their own confirmation setting, but do not assume a third-party Harness has that shortcut.

## Limits

| Limit | Notes |
| --- | --- |
| Not covered by H1 | Skill directories, the Workbench, micro-app web pages, and automatic plugin install and update are DSH-only deep integration; third-party Harnesses must not assume them |
| Request size | `max_request_bytes` / `max_response_bytes` follow the discovery document |
| Token lifecycle | No refresh token; reissue after expiry; the device code flow can only fetch a token once |
| Scopes and permissions | Scopes are an upper bound; the effective permission is "account permissions ∩ scopes" |
| API stability | The existing shapes of `/v1`, `/.well-known/dsh-ee` and `/mcp` stay unchanged; new capabilities follow the discovery document |
| Write operations | Anything without `readOnlyHint` is treated as a write operation and goes through the confirmation flow |

## Full text in the repository

This page is only a guide; protocol and endpoint details live in the repository docs (docs marked "available after merge" are introduced by the companion docs PR):

- [Third-party Harness access (docs/harness-integration.md, available after merge)](https://github.com/hikariming/dsh-ee/blob/main/docs/harness-integration.md)
- [Non-DSH client access L1 (docs/l1-client.md)](https://github.com/hikariming/dsh-ee/blob/main/docs/l1-client.md)
- [Discovery and capabilities (docs/l1-client.md §2, available after merge)](https://github.com/hikariming/dsh-ee/blob/main/docs/l1-client.md#2-%E5%8F%91%E7%8E%B0%E4%B8%8E%E8%83%BD%E5%8A%9B%E4%B8%AD%E7%AB%8B%E5%A5%91%E7%BA%A6-v1)
- [The Harness page in the developer docs (docs/dev/harness.md, available after merge)](https://github.com/hikariming/dsh-ee/blob/main/docs/dev/harness.md)
- [Extension protocol overview (docs/extension-protocol/README.md, available after merge)](https://github.com/hikariming/dsh-ee/blob/main/docs/extension-protocol/README.md)
