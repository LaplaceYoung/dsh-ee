"""dsh-ee Harness 的最小 Python 客户端（仅标准库）。

覆盖范围（与 TypeScript 版对齐，但更小）：
  - 发现文档 / 能力摘要 / 模型目录 / 非流式 chat completion
  - 思考模式参数（thinking / reasoning_effort）与 reasoning_content 类型
  - DeepSeek Chatbot 透传参数（tools / safety_check / title）与拒绝/标题/工具过程类型
  - 设备码登录（申请 + 轮询）
  - MCP：initialize / ping / tools/list（cursor 分页）/ tools/call + 写确认 _meta
  - 统一错误与限流头解析

不实现流式 chat（标准库 urllib 读完整 body 再返回；流式请用 TypeScript 版或自行实现）、
Anthropic Messages / OpenAI Responses 等协议。

用法::

    from dsh_ee_harness import DshClient
    client = DshClient("https://dsh-ee.example.com", token="ee_at_...")
    print(client.discovery()["version"])
    print([m["id"] for m in client.list_models()])
"""

from __future__ import annotations

import json
import socket
import threading
import time
import urllib.error
import urllib.request
from dataclasses import dataclass
from typing import Any, Callable, Dict, Iterable, List, Mapping, Optional, Tuple

from .types import (
    ChatCompletion,
    ChatMessage,
    DeepSeekSafetyCheck,
    DeepSeekTitleConfig,
    ReasoningEffort,
    ThinkingConfig,
)

__all__ = [
    "DshClient",
    "DshClientError",
    "DshNetworkError",
    "DshJsonRpcError",
    "DeviceFlowError",
    "McpSession",
    "RateLimit",
    "Response",
    "Transport",
    "urllib_transport",
    "extract_confirmation",
    "is_pending_confirmation",
    "parse_rate_limit_headers",
    "parse_retry_after",
    "MCP_DEFAULT_PROTOCOL_VERSION",
]

DEFAULT_TIMEOUT = 60.0
DEFAULT_API_VERSION = "1"
MCP_DEFAULT_PROTOCOL_VERSION = "2025-06-18"
DEFAULT_MCP_CLIENT_INFO = {"name": "dsh-ee-harness-sdk-py", "version": "0.1.0"}
DEVICE_ERROR_CODES = {
    "authorization_pending": True,
    "slow_down": True,
    "access_denied": False,
    "expired_token": False,
    "rate_limited": True,
}

# Transport: (method, url, headers, body, timeout) -> (status, headers, body)
Transport = Callable[
    [str, str, Dict[str, str], Optional[bytes], Optional[float]],
    Tuple[int, Mapping[str, str], bytes],
]


def _lower_headers(headers: Mapping[str, str]) -> Dict[str, str]:
    return {str(key).lower(): str(value) for key, value in headers.items()}


def _to_int(value: Optional[str]) -> Optional[int]:
    if value is None or str(value).strip() == "":
        return None
    try:
        return int(str(value).strip())
    except (TypeError, ValueError):
        return None


@dataclass(frozen=True)
class RateLimit:
    """X-RateLimit-* 解析结果；reset 是 Unix 秒。"""

    limit: int
    remaining: int
    reset: int

    @property
    def reset_at(self) -> float:
        return float(self.reset)


def parse_rate_limit_headers(headers: Mapping[str, str]) -> Optional[RateLimit]:
    """解析 X-RateLimit-Limit / Remaining / Reset；三个头缺任一返回 None。"""
    lower = _lower_headers(headers)
    limit = _to_int(lower.get("x-ratelimit-limit"))
    remaining = _to_int(lower.get("x-ratelimit-remaining"))
    reset = _to_int(lower.get("x-ratelimit-reset"))
    if limit is None or remaining is None or reset is None:
        return None
    return RateLimit(limit=limit, remaining=remaining, reset=reset)


def parse_retry_after(headers: Mapping[str, str]) -> Optional[int]:
    """解析 Retry-After（秒数或 HTTP-date），返回向上取整的秒数。"""
    raw = _lower_headers(headers).get("retry-after")
    if raw is None or raw.strip() == "":
        return None
    try:
        seconds = float(raw.strip())
    except ValueError:
        seconds = None
    if seconds is not None and seconds >= 0:
        return int(seconds) if seconds == int(seconds) else int(seconds) + 1
    try:
        from email.utils import parsedate_to_datetime

        when = parsedate_to_datetime(raw)
        return max(0, int((when.timestamp() - time.time()) + 0.999))
    except (TypeError, ValueError):
        return None


@dataclass
class Response:
    """一次 HTTP 响应。headers 已统一为小写 key。"""

    status: int
    headers: Dict[str, str]
    body: bytes

    @property
    def ok(self) -> bool:
        return 200 <= self.status < 300

    @property
    def text(self) -> str:
        return self.body.decode("utf-8", "replace")

    @property
    def json(self) -> Any:
        if not self.body:
            return None
        try:
            return json.loads(self.body)
        except (ValueError, UnicodeDecodeError):
            return None

    @property
    def rate_limit(self) -> Optional[RateLimit]:
        return parse_rate_limit_headers(self.headers)

    @property
    def retry_after(self) -> Optional[int]:
        return parse_retry_after(self.headers)


class DshClientError(Exception):
    """dsh-ee HTTP 接口返回的错误 / 协议错误。"""

    def __init__(
        self,
        message: str,
        *,
        status: Optional[int] = None,
        request_id: Optional[str] = None,
        api_version: Optional[str] = None,
        rate_limit: Optional[RateLimit] = None,
        retry_after: Optional[int] = None,
        code: Optional[str] = None,
        error_type: Optional[str] = None,
        body: Any = None,
    ) -> None:
        super().__init__(message)
        self.status = status
        self.request_id = request_id
        self.api_version = api_version
        self.rate_limit = rate_limit
        self.retry_after = retry_after
        self.code = code
        self.error_type = error_type
        self.body = body

    @property
    def is_rate_limited(self) -> bool:
        return self.status == 429


class DshNetworkError(DshClientError):
    """网络层错误：network / timeout / aborted。"""

    def __init__(self, message: str, network_code: str) -> None:
        super().__init__(message)
        self.network_code = network_code


class DshJsonRpcError(DshClientError):
    """MCP JSON-RPC 错误（HTTP 状态可能仍是 200，也可能是 429）。"""

    def __init__(self, message: str, rpc_code: int, *, rpc_data: Any = None, **kwargs: Any) -> None:
        super().__init__(message, **kwargs)
        self.rpc_code = rpc_code
        self.rpc_data = rpc_data


class DeviceFlowError(DshClientError):
    """设备码轮询业务错误；retryable 为 True 时继续轮询。"""

    def __init__(self, flow_code: str, *, retryable: bool, **kwargs: Any) -> None:
        super().__init__(_device_error_message(flow_code), **kwargs)
        self.flow_code = flow_code
        self.retryable = retryable


def _device_error_message(flow_code: str) -> str:
    return {
        "authorization_pending": "等待用户在网页上确认设备码",
        "slow_down": "轮询过快，需要降低频率",
        "access_denied": "用户拒绝了这次设备码授权",
        "expired_token": "设备码已过期或已被使用",
        "rate_limited": "设备码轮询被限流",
    }.get(flow_code, "设备码流程失败")


def _error_from_response(response: Response) -> DshClientError:
    """把非 2xx 响应解析成带上下文的错误（OpenAI 壳 / 中立壳 / JSON-RPC / 字符串壳）。"""
    payload = response.json
    common: Dict[str, Any] = {
        "status": response.status,
        "request_id": response.headers.get("x-request-id"),
        "api_version": response.headers.get("dsh-ee-api-version"),
        "rate_limit": response.rate_limit,
        "retry_after": response.retry_after,
        "body": payload if payload is not None else response.text,
    }
    if isinstance(payload, dict):
        error = payload.get("error")
        if payload.get("jsonrpc") == "2.0" and isinstance(error, dict) and isinstance(error.get("code"), int):
            return DshJsonRpcError(
                str(error.get("message") or f"JSON-RPC 错误 {error.get('code')}"),
                error["code"],
                rpc_data=error.get("data"),
                **common,
            )
        if isinstance(error, dict):
            common["request_id"] = error.get("request_id") or common["request_id"]
            if isinstance(error.get("retry_after"), int):
                common["retry_after"] = error["retry_after"]
            return DshClientError(
                str(error.get("message") or f"HTTP {response.status}"),
                code=error.get("code") if isinstance(error.get("code"), str) else None,
                error_type=error.get("type") if isinstance(error.get("type"), str) else None,
                **common,
            )
        if isinstance(error, str):
            return DshClientError(
                error,
                code=payload.get("code") if isinstance(payload.get("code"), str) else None,
                **common,
            )
    text = response.text[:300]
    return DshClientError(f"HTTP {response.status}: {text}" if text else f"HTTP {response.status}", **common)


def urllib_transport(
    method: str,
    url: str,
    headers: Dict[str, str],
    body: Optional[bytes],
    timeout: Optional[float],
) -> Tuple[int, Mapping[str, str], bytes]:
    """默认传输实现（urllib）。网络错误统一抛 DshNetworkError。"""
    request = urllib.request.Request(url=url, data=body, headers=headers, method=method)
    try:
        with urllib.request.urlopen(request, timeout=timeout) as response:  # noqa: S310 - 由调用方给出 URL
            return response.status, dict(response.headers), response.read()
    except urllib.error.HTTPError as error:  # 4xx/5xx 仍然读 body
        return error.code, dict(error.headers or {}), error.read()
    except socket.timeout as error:
        raise DshNetworkError(f"请求超时（{timeout}s）：{url}", "timeout") from error
    except urllib.error.URLError as error:
        reason = getattr(error, "reason", error)
        if isinstance(reason, socket.timeout):
            raise DshNetworkError(f"请求超时（{timeout}s）：{url}", "timeout") from error
        raise DshNetworkError(f"网络请求失败：{url}（{reason}）", "network") from error
    except OSError as error:
        raise DshNetworkError(f"网络请求失败：{url}（{error}）", "network") from error


def _is_record(value: Any) -> bool:
    return isinstance(value, dict)


def _parse_sse_data(text: str) -> List[str]:
    """解析完整 SSE 文本里的 data 事件（用于 MCP POST 响应）。"""
    events: List[str] = []
    for block in text.replace("\r\n", "\n").split("\n\n"):
        data_lines: List[str] = []
        for line in block.split("\n"):
            if line == "" or line.startswith(":"):
                continue
            if not line.startswith("data:"):
                continue
            value = line[len("data:") :]
            if value.startswith(" "):
                value = value[1:]
            data_lines.append(value)
        if data_lines:
            events.append("\n".join(data_lines))
    return events


class McpSession:
    """MCP 会话（dsh-ee 的 POST /mcp 面）。通常由 ``DshClient.connect_mcp()`` 创建。

    dsh-ee 的实现要点（见服务端 #128）：
      - initialize 响应头带 Mcp-Session-Id，后续请求原样回带；
      - 第三方客户端 tools/list 默认分页；limit=0 表示不分页；
      - 写确认结果在 ``_meta["dsh-ee/confirmation"]``，用 :func:`extract_confirmation` 取。
    """

    def __init__(
        self,
        send: Callable[..., Response],
        *,
        protocol_version: str = MCP_DEFAULT_PROTOCOL_VERSION,
        client_info: Optional[Dict[str, str]] = None,
        initialize: bool = True,
    ) -> None:
        self._send = send
        self._protocol_version = protocol_version
        self._requested_protocol_version = protocol_version
        self._client_info = dict(client_info or DEFAULT_MCP_CLIENT_INFO)
        self._next_id = 1
        self._session_id: Optional[str] = None
        self._initialized = False
        self._closed = False
        self.server_info: Optional[Dict[str, Any]] = None
        self.instructions: Optional[str] = None
        self.server_capabilities: Dict[str, Any] = {}
        if initialize:
            self.start()

    # ---- 状态 ----

    @property
    def session_id(self) -> Optional[str]:
        return self._session_id

    @property
    def protocol_version(self) -> str:
        return self._protocol_version

    @property
    def is_initialized(self) -> bool:
        return self._initialized

    @property
    def is_closed(self) -> bool:
        return self._closed

    # ---- 生命周期 ----

    def start(self) -> Dict[str, Any]:
        """initialize + notifications/initialized；重复调用幂等。"""
        if self._initialized:
            return {
                "protocolVersion": self._protocol_version,
                "capabilities": self.server_capabilities,
                "serverInfo": self.server_info or {"name": "dsh-ee", "version": ""},
                "instructions": self.instructions,
            }
        result, response = self._request_with_response(
            "initialize",
            {
                "protocolVersion": self._requested_protocol_version,
                "capabilities": {},
                "clientInfo": self._client_info,
            },
        )
        session_id = response.headers.get("mcp-session-id")
        if session_id:
            self._session_id = session_id
        if isinstance(result, dict):
            version = result.get("protocolVersion")
            if isinstance(version, str) and version:
                self._protocol_version = version
            server_info = result.get("serverInfo")
            self.server_info = server_info if _is_record(server_info) else None
            instructions = result.get("instructions")
            self.instructions = instructions if isinstance(instructions, str) else None
            capabilities = result.get("capabilities")
            self.server_capabilities = capabilities if _is_record(capabilities) else {}
        self._notify("notifications/initialized")
        self._initialized = True
        return result if isinstance(result, dict) else {}

    def ping(self) -> Dict[str, Any]:
        return self._request("ping", None)

    def list_tools(self, limit: Optional[int] = None, cursor: Optional[str] = None) -> Dict[str, Any]:
        """取一页工具；不传 limit 用服务端默认页大小，limit=0 取全量。"""
        params: Dict[str, Any] = {}
        if limit is not None:
            params["limit"] = limit
        if cursor is not None:
            params["cursor"] = cursor
        result = self._request("tools/list", params)
        if not _is_record(result) or not isinstance(result.get("tools"), list):
            raise DshClientError("MCP tools/list 响应缺少 tools 数组")
        next_cursor = result.get("nextCursor")
        if next_cursor is not None and not isinstance(next_cursor, str):
            raise DshClientError("MCP tools/list 的 nextCursor 不是字符串")
        return {"tools": result["tools"], "nextCursor": next_cursor}

    def list_all_tools(
        self,
        page_size: Optional[int] = None,
        on_page: Optional[Callable[[Dict[str, Any]], None]] = None,
    ) -> List[Dict[str, Any]]:
        """跟随 nextCursor 取全部工具。"""
        tools: List[Dict[str, Any]] = []
        seen: set[str] = set()
        cursor: Optional[str] = None
        while True:
            page = self.list_tools(limit=page_size, cursor=cursor)
            tools.extend(page["tools"])
            if on_page is not None:
                on_page(page)
            next_cursor = page.get("nextCursor")
            if next_cursor is None:
                return tools
            if next_cursor in seen:
                raise DshClientError("MCP tools/list 返回了重复的 nextCursor，已停止翻页")
            seen.add(next_cursor)
            cursor = next_cursor

    def call_tool(self, name: str, arguments: Optional[Dict[str, Any]] = None) -> Dict[str, Any]:
        """调用工具；结果里的 isError 不抛错，写确认用 extract_confirmation() 取。"""
        result = self._request("tools/call", {"name": name, "arguments": arguments or {}})
        if not _is_record(result) or not isinstance(result.get("content"), list):
            raise DshClientError(f"MCP tools/call（{name}）响应缺少 content 数组")
        return result

    def close(self) -> None:
        """结束会话：有 session id 时 DELETE /mcp；错误忽略。"""
        if self._closed:
            return
        self._closed = True
        if not self._initialized or not self._session_id:
            return
        try:
            self._send(
                method="DELETE",
                body=None,
                session_id=self._session_id,
                protocol_version=self._protocol_version,
            )
        except DshClientError:
            pass

    # ---- JSON-RPC ----

    def _request(self, method: str, params: Any) -> Any:
        result, _ = self._request_with_response(method, params)
        return result

    def _request_with_response(self, method: str, params: Any) -> Tuple[Any, Response]:
        if self._closed:
            raise DshClientError("MCP 会话已关闭，请重新 connect_mcp()")
        request_id = self._next_id
        self._next_id += 1
        body: Dict[str, Any] = {"jsonrpc": "2.0", "id": request_id, "method": method}
        if params is not None:
            body["params"] = params
        response = self._send(
            method="POST",
            body=body,
            session_id=self._session_id,
            protocol_version=self._protocol_version,
        )
        message = _rpc_message(response)
        if message is None:
            raise DshClientError(f"MCP {method} 没有返回匹配 id={request_id} 的 JSON-RPC 响应")
        if "result" not in message:
            raise DshClientError(f"MCP {method} 响应既没有 result 也没有 error")
        return message["result"], response

    def _notify(self, method: str, params: Any = None) -> None:
        if self._closed:
            raise DshClientError("MCP 会话已关闭，请重新 connect_mcp()")
        body: Dict[str, Any] = {"jsonrpc": "2.0", "method": method}
        if params is not None:
            body["params"] = params
        response = self._send(
            method="POST",
            body=body,
            session_id=self._session_id,
            protocol_version=self._protocol_version,
        )
        _rpc_message(response)  # 只在有 JSON-RPC error / HTTP 错误时抛


def _rpc_message(response: Response) -> Optional[Dict[str, Any]]:
    """从普通 JSON 或 SSE 响应里找 JSON-RPC 响应消息；错误优先抛。"""
    messages: List[Any] = []
    content_type = response.headers.get("content-type", "")
    if "text/event-stream" in content_type:
        for data in _parse_sse_data(response.text):
            try:
                messages.append(json.loads(data))
            except ValueError:
                continue
    else:
        payload = response.json
        if payload is not None:
            messages.append(payload)
    for message in messages:
        if not _is_record(message):
            continue
        error = message.get("error")
        if _is_record(error) and isinstance(error.get("code"), int):
            raise DshJsonRpcError(
                str(error.get("message") or "JSON-RPC 错误"),
                error["code"],
                rpc_data=error.get("data"),
                status=response.status,
                request_id=response.headers.get("x-request-id"),
                api_version=response.headers.get("dsh-ee-api-version"),
                rate_limit=response.rate_limit,
                retry_after=response.retry_after,
                body=error,
            )
    if not response.ok:
        raise _error_from_response(response)
    for message in messages:
        if _is_record(message) and "id" in message and "method" not in message:
            return message
    return None


def extract_confirmation(result: Mapping[str, Any]) -> Optional[Dict[str, Any]]:
    """从 tools/call 结果里取 ``_meta["dsh-ee/confirmation"]``。"""
    meta = result.get("_meta")
    if not _is_record(meta):
        return None
    raw = meta.get("dsh-ee/confirmation")
    if not _is_record(raw):
        return None
    confirmation_id = raw.get("id")
    url = raw.get("url")
    expires_at = raw.get("expires_at")
    if not isinstance(confirmation_id, (int, str)) or not isinstance(url, str) or not isinstance(expires_at, str):
        return None
    parsed: Dict[str, Any] = {"id": confirmation_id, "url": url, "expires_at": expires_at}
    try:
        from datetime import datetime

        parsed["expires_at_dt"] = datetime.fromisoformat(expires_at.replace("Z", "+00:00"))
    except ValueError:
        parsed["expires_at_dt"] = None
    return parsed


def is_pending_confirmation(result: Mapping[str, Any]) -> bool:
    """判断 tools/call 结果是否是「等待网页确认」。"""
    return extract_confirmation(result) is not None


class DshClient:
    """dsh-ee Harness 客户端（Python，标准库实现）。

    :param base_url: dsh-ee 部署地址，例如 ``https://dsh-ee.example.com``。
    :param token: 统一令牌 ``ee_at_...``；也可以设备码登录后由客户端保存。
    :param transport: 可注入的传输函数（测试用）；默认 :func:`urllib_transport`。
    :param headers: 附加的默认请求头。
    :param timeout: 单次请求超时（秒），``None`` 表示不设超时。
    """

    def __init__(
        self,
        base_url: str,
        *,
        token: Optional[str] = None,
        transport: Optional[Transport] = None,
        headers: Optional[Mapping[str, str]] = None,
        timeout: Optional[float] = DEFAULT_TIMEOUT,
    ) -> None:
        if not isinstance(base_url, str) or not base_url.strip():
            raise ValueError("DshClient 需要 base_url")
        self.base_url = base_url.strip().rstrip("/")
        if not self.base_url:
            raise ValueError("DshClient 的 base_url 不能只有斜杠")
        self._token = token
        self._transport: Transport = transport or urllib_transport
        self._default_headers = dict(headers or {})
        self._timeout = timeout
        self._discovery_cache: Optional[Dict[str, Any]] = None
        self._discovery_etag: Optional[str] = None
        self._mcp_session: Optional[McpSession] = None
        self._lock = threading.Lock()
        self.last_request_id: Optional[str] = None
        self.last_api_version: Optional[str] = None
        self.last_rate_limit: Optional[RateLimit] = None

    # ---- 令牌 ----

    def set_token(self, token: Optional[str]) -> None:
        self._token = token

    @property
    def has_token(self) -> bool:
        return bool(self._token)

    # ---- 发现 / 能力 / 模型 ----

    def discovery(self, refresh: bool = False) -> Dict[str, Any]:
        """GET /.well-known/dsh-ee；默认用 ETag 缓存（304 时返回缓存）。"""
        headers: Dict[str, str] = {}
        if not refresh and self._discovery_etag:
            headers["If-None-Match"] = self._discovery_etag
        response = self._send("GET", "/.well-known/dsh-ee", auth=False, headers=headers)
        if response.status == 304:
            if self._discovery_cache is None:
                raise DshClientError("发现文档返回 304，但本地没有缓存", status=304)
            return self._discovery_cache
        payload = self._request_json(response)
        if not _is_record(payload):
            raise DshClientError("发现文档不是 JSON 对象")
        self._discovery_cache = payload
        self._discovery_etag = response.headers.get("etag")
        return payload

    def capabilities(self) -> Dict[str, Any]:
        """GET /v1/capabilities（不带令牌时返回公开子集）。"""
        return self._request_json(self._send("GET", "/v1/capabilities"))

    def list_models(self) -> List[Dict[str, Any]]:
        """GET /v1/models：当前令牌可用的对外模型名。"""
        payload = self._request_json(self._send("GET", "/v1/models"))
        if not _is_record(payload) or not isinstance(payload.get("data"), list):
            raise DshClientError("GET /v1/models 响应缺少 data 数组")
        return list(payload["data"])

    def chat_completion(
        self,
        model: str,
        messages: Iterable[ChatMessage],
        *,
        thinking: Optional[ThinkingConfig] = None,
        reasoning_effort: Optional[ReasoningEffort] = None,
        tools: Optional[Iterable[Mapping[str, Any]]] = None,
        safety_check: Optional[DeepSeekSafetyCheck] = None,
        title: Optional[DeepSeekTitleConfig] = None,
        **params: Any,
    ) -> ChatCompletion:
        """POST /v1/chat/completions（非流式）。

        ``thinking`` / ``reasoning_effort`` / ``tools`` / ``safety_check`` / ``title`` 原样转发给
        上游（网关不做白名单/映射）。响应里的 ``choices[].message.reasoning_content`` 是思维链，
        ``message.server_tool_calls`` / ``message.annotations`` / ``message.title`` /
        ``message.refusal`` 是 DeepSeek Chatbot 扩展，均由上游原样返回。

        - 请求携带 ``tools`` 时，历史 assistant 消息必须保留 ``reasoning_content``；
        - 带服务端工具（deepseek_web_search / deepseek_read_url）的轮次，多轮要把
          ``server_tool_calls``（含 ``kind`` / ``signature``）原样回传；
        - 内容安全拒答时 ``finish_reason`` 为 ``content_filter``，``message.refusal`` 是说明；
        - ``latest_reminder`` 消息（role=latest_reminder）必须紧贴其 user 消息之前，content 用
          :func:`dsh_ee_harness.types.format_latest_reminder` 生成；
        - 文件/文档用 ``file`` 内容块（``file_id`` / ``file_data``）或 Messages API 的
          ``document`` 块引用；文件需先通过 DeepSeek Files API 上传，dsh-ee 只透传 chat 请求；
        - dsh-ee 与 SDK 都不执行服务端工具，搜索/读链接由上游完成。
        """
        body: Dict[str, Any] = {"model": model, "messages": list(messages), "stream": False}
        if thinking is not None:
            body["thinking"] = thinking
        if reasoning_effort is not None:
            body["reasoning_effort"] = reasoning_effort
        if tools is not None:
            body["tools"] = list(tools)
        if safety_check is not None:
            body["safety_check"] = safety_check
        if title is not None:
            body["title"] = title
        body.update(params)
        payload = self._request_json(self._send("POST", "/v1/chat/completions", body=body))
        if not _is_record(payload) or not isinstance(payload.get("choices"), list):
            raise DshClientError("POST /v1/chat/completions 响应缺少 choices 数组")
        return payload

    # ---- 设备码 ----

    def request_device_code(
        self,
        *,
        client_name: Optional[str] = None,
        client_os: Optional[str] = None,
        client_version: Optional[str] = None,
        scopes: Optional[Iterable[str]] = None,
        setup_code: Optional[str] = None,
    ) -> Dict[str, Any]:
        """POST /api/device/code。"""
        body: Dict[str, Any] = {}
        if client_name is not None:
            body["client_name"] = client_name
        if client_os is not None:
            body["client_os"] = client_os
        if client_version is not None:
            body["client_version"] = client_version
        if scopes is not None:
            body["scopes"] = list(scopes)
        if setup_code is not None:
            body["setup_code"] = setup_code
        payload = self._request_json(self._send("POST", "/api/device/code", body=body, auth=False))
        if not _is_record(payload) or not isinstance(payload.get("device_code"), str):
            raise DshClientError("设备码响应缺少 device_code")
        return payload

    def poll_device_token(self, device_code: str) -> Dict[str, Any]:
        """POST /api/device/token 轮询一次；等待确认时抛 retryable 的 DeviceFlowError。"""
        response = self._send("POST", "/api/device/token", body={"device_code": device_code}, auth=False)
        if response.ok:
            payload = response.json
            if not _is_record(payload) or not isinstance(payload.get("access_token"), str):
                raise DshClientError("设备码令牌响应缺少 access_token")
            return payload
        payload = response.json
        code = None
        if _is_record(payload) and isinstance(payload.get("error"), str):
            code = payload["error"]
        if code in DEVICE_ERROR_CODES:
            raise DeviceFlowError(code, retryable=DEVICE_ERROR_CODES[code], status=response.status)
        raise _error_from_response(response)

    def wait_for_device_token(
        self,
        device_code: Dict[str, Any],
        *,
        interval: Optional[float] = None,
        timeout: Optional[float] = None,
        on_pending: Optional[Callable[[str, int], None]] = None,
        sleep: Optional[Callable[[float], None]] = None,
    ) -> Dict[str, Any]:
        """轮询直到用户确认 / 过期；interval 单位为秒。"""
        device_code_value = str(device_code["device_code"])
        interval_seconds = interval
        if interval_seconds is None:
            interval_seconds = float(device_code.get("interval") or 5)
        timeout_seconds = timeout
        if timeout_seconds is None:
            timeout_seconds = float(device_code.get("expires_in") or 600)
        deadline = time.monotonic() + timeout_seconds
        sleep_fn = sleep or time.sleep
        attempt = 0
        while True:
            if time.monotonic() >= deadline:
                raise DeviceFlowError("expired_token", retryable=False, status=400)
            attempt += 1
            try:
                return self.poll_device_token(device_code_value)
            except DeviceFlowError as error:
                if not error.retryable:
                    raise
                if on_pending is not None:
                    on_pending(error.flow_code, attempt)
                delay = interval_seconds
                if error.flow_code == "slow_down":
                    interval_seconds += 5
                    delay = interval_seconds
                elif error.flow_code == "rate_limited":
                    delay = max(interval_seconds, 5)
                remaining = deadline - time.monotonic()
                if remaining <= 0:
                    raise DeviceFlowError("expired_token", retryable=False, status=400) from error
                sleep_fn(min(delay, remaining))

    def login_with_device_code(
        self,
        *,
        client_name: Optional[str] = None,
        client_os: Optional[str] = None,
        client_version: Optional[str] = None,
        scopes: Optional[Iterable[str]] = None,
        on_code: Optional[Callable[[Dict[str, Any]], None]] = None,
        on_pending: Optional[Callable[[str, int], None]] = None,
        interval: Optional[float] = None,
        timeout: Optional[float] = None,
        sleep: Optional[Callable[[float], None]] = None,
    ) -> Dict[str, Any]:
        """申请设备码 → on_code 展示 → 轮询 → 保存令牌。"""
        code = self.request_device_code(
            client_name=client_name,
            client_os=client_os,
            client_version=client_version,
            scopes=scopes,
        )
        if on_code is not None:
            on_code(code)
        token = self.wait_for_device_token(
            code,
            interval=interval,
            timeout=timeout,
            on_pending=on_pending,
            sleep=sleep,
        )
        self.set_token(token["access_token"])
        return token

    # ---- MCP ----

    def connect_mcp(
        self,
        *,
        protocol_version: str = MCP_DEFAULT_PROTOCOL_VERSION,
        client_info: Optional[Dict[str, str]] = None,
        initialize: bool = True,
    ) -> McpSession:
        """建立（或复用）MCP 会话；默认自动 initialize。"""
        with self._lock:
            session = self._mcp_session
            if session is not None and not session.is_closed:
                if initialize and not session.is_initialized:
                    session.start()
                return session
            session = McpSession(
                self._send_mcp,
                protocol_version=protocol_version,
                client_info=client_info,
                initialize=initialize,
            )
            self._mcp_session = session
            return session

    def list_tools(self, limit: Optional[int] = None, cursor: Optional[str] = None) -> Dict[str, Any]:
        return self.connect_mcp().list_tools(limit=limit, cursor=cursor)

    def list_all_tools(self, page_size: Optional[int] = None) -> List[Dict[str, Any]]:
        return self.connect_mcp().list_all_tools(page_size=page_size)

    def call_tool(self, name: str, arguments: Optional[Dict[str, Any]] = None) -> Dict[str, Any]:
        return self.connect_mcp().call_tool(name, arguments)

    def close(self) -> None:
        """关闭缓存的 MCP 会话。"""
        session = self._mcp_session
        self._mcp_session = None
        if session is not None:
            session.close()

    # ---- 内部 ----

    def _send_mcp(
        self,
        *,
        method: str,
        body: Optional[Mapping[str, Any]],
        session_id: Optional[str],
        protocol_version: Optional[str],
    ) -> Response:
        headers = {"Accept": "application/json, text/event-stream"}
        if session_id:
            headers["Mcp-Session-Id"] = session_id
        if protocol_version:
            headers["MCP-Protocol-Version"] = protocol_version
        return self._send("POST" if method.upper() != "DELETE" else "DELETE", "/mcp", body=body, headers=headers)

    def _send(
        self,
        method: str,
        path: str,
        *,
        body: Optional[Mapping[str, Any]] = None,
        auth: bool = True,
        headers: Optional[Mapping[str, str]] = None,
    ) -> Response:
        request_headers = dict(self._default_headers)
        request_headers.update(headers or {})
        if "accept" not in {key.lower() for key in request_headers}:
            request_headers["Accept"] = "application/json"
        if body is not None:
            request_headers["Content-Type"] = "application/json"
        lower_keys = {key.lower() for key in request_headers}
        if auth and self._token and "authorization" not in lower_keys:
            request_headers["Authorization"] = f"Bearer {self._token}"
        if (path.startswith("/v1/") or path == "/.well-known/dsh-ee") and "dsh-ee-api-version" not in lower_keys:
            request_headers["Dsh-EE-API-Version"] = DEFAULT_API_VERSION
        payload = None if body is None else json.dumps(body).encode("utf-8")
        status, raw_headers, raw_body = self._transport(
            method, f"{self.base_url}{path}", request_headers, payload, self._timeout
        )
        response = Response(status=status, headers=_lower_headers(raw_headers), body=raw_body)
        self.last_request_id = response.headers.get("x-request-id")
        self.last_api_version = response.headers.get("dsh-ee-api-version")
        self.last_rate_limit = response.rate_limit
        return response

    def _request_json(self, response: Response) -> Any:
        if not response.ok:
            raise _error_from_response(response)
        if not response.body:
            raise DshClientError(
                f"HTTP {response.status} 响应为空（content-type: {response.headers.get('content-type', '未知')}）"
            )
        payload = response.json
        if payload is None:
            raise DshClientError(f"HTTP {response.status} 响应不是 JSON：{response.text[:200]}")
        return payload
