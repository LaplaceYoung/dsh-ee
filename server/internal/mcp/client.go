// Package mcp 实现 dsh-ee 需要的那部分 Model Context Protocol：
// 连上游 MCP 服务的 Streamable HTTP 客户端，以及消息类型（服务端在 api 包里）。
package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"sync"
)

// 支持的协议版本，新的在前。初始化时用第一个，服务端回别的受支持版本也接受。
var Versions = []string{"2025-11-25", "2025-06-18", "2025-03-26"}

const maxResponse = 8 << 20

// Tool 是 tools/list 返回的一项。
type Tool struct {
	Name        string          `json:"name"`
	Title       string          `json:"title,omitempty"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema"`
	Annotations *struct {
		ReadOnlyHint *bool `json:"readOnlyHint,omitempty"`
	} `json:"annotations,omitempty"`
}

// ReadOnly 读工具注解里的只读提示；没写时当作会写。
func (t Tool) ReadOnly() bool {
	return t.Annotations != nil && t.Annotations.ReadOnlyHint != nil && *t.Annotations.ReadOnlyHint
}

// Error 是 JSON-RPC 错误。
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return fmt.Sprintf("MCP 错误 %d：%s", e.Code, e.Message) }

type request struct {
	JSONRPC string `json:"jsonrpc"`
	ID      *int64 `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type response struct {
	ID     json.RawMessage `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *Error          `json:"error"`
}

// Client 是到一个上游 MCP 服务的连接。并发安全；会话在第一次调用时建立，服务端说会话失效时重建一次。
type Client struct {
	URL    string
	Header http.Header
	// 每次请求现取的请求头（如只有几分钟有效的身份令牌），和 Header 一起带上；可以为 nil
	Dynamic func(ctx context.Context) (http.Header, error)
	HC      *http.Client

	mu      sync.Mutex
	session string
	version string
	ready   bool
	nextID  int64
	// 初始化时服务端给的说明，管理页展示
	Instructions string
	ServerName   string
}

func (c *Client) id() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nextID++
	return c.nextID
}

// post 发一条消息；isCall 时等待对应 id 的响应。
func (c *Client) post(ctx context.Context, msg request) (*response, int, error) {
	body, err := json.Marshal(msg)
	if err != nil {
		return nil, 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL, bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	for k, vs := range c.Header {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	if c.Dynamic != nil {
		extra, err := c.Dynamic(ctx)
		if err != nil {
			return nil, 0, err
		}
		for k, vs := range extra {
			req.Header[k] = vs
		}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	c.mu.Lock()
	if c.session != "" {
		req.Header.Set("Mcp-Session-Id", c.session)
	}
	if c.version != "" {
		req.Header.Set("Mcp-Protocol-Version", c.version)
	}
	c.mu.Unlock()
	resp, err := c.HC.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	if sid := resp.Header.Get("Mcp-Session-Id"); sid != "" {
		c.mu.Lock()
		c.session = sid
		c.mu.Unlock()
	}
	if msg.ID == nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
		if resp.StatusCode >= 300 {
			return nil, resp.StatusCode, fmt.Errorf("上游返回 HTTP %d", resp.StatusCode)
		}
		return nil, resp.StatusCode, nil
	}
	if resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, resp.StatusCode, fmt.Errorf("上游返回 HTTP %d：%s", resp.StatusCode, strings.TrimSpace(string(snippet)))
	}
	mt, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if mt == "text/event-stream" {
		r, err := readSSEResponse(io.LimitReader(resp.Body, maxResponse), *msg.ID)
		return r, resp.StatusCode, err
	}
	var r response
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponse)).Decode(&r); err != nil {
		return nil, resp.StatusCode, fmt.Errorf("上游响应不是合法的 JSON-RPC：%w", err)
	}
	return &r, resp.StatusCode, nil
}

// readSSEResponse 从事件流里找到 id 对应的那条响应；中间的通知与服务端请求跳过。
func readSSEResponse(body io.Reader, id int64) (*response, error) {
	sc := bufio.NewScanner(body)
	sc.Buffer(make([]byte, 64<<10), maxResponse)
	var data strings.Builder
	want := fmt.Sprint(id)
	flush := func() (*response, bool) {
		defer data.Reset()
		if data.Len() == 0 {
			return nil, false
		}
		var r response
		if json.Unmarshal([]byte(data.String()), &r) != nil {
			return nil, false
		}
		if strings.Trim(string(r.ID), `"`) == want && (r.Result != nil || r.Error != nil) {
			return &r, true
		}
		return nil, false
	}
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			if r, ok := flush(); ok {
				return r, nil
			}
			continue
		}
		if v, ok := strings.CutPrefix(line, "data:"); ok {
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(strings.TrimPrefix(v, " "))
		}
	}
	if r, ok := flush(); ok {
		return r, nil
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return nil, errors.New("上游的事件流里没有对应的响应")
}

func (c *Client) initialize(ctx context.Context) error {
	c.mu.Lock()
	c.session, c.version, c.ready = "", "", false
	c.mu.Unlock()
	id := c.id()
	r, _, err := c.post(ctx, request{JSONRPC: "2.0", ID: &id, Method: "initialize", Params: map[string]any{
		"protocolVersion": Versions[0],
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "dsh-ee", "version": "1"},
	}})
	if err != nil {
		return fmt.Errorf("初始化 MCP 会话：%w", err)
	}
	if r.Error != nil {
		return fmt.Errorf("初始化 MCP 会话：%w", r.Error)
	}
	var res struct {
		ProtocolVersion string `json:"protocolVersion"`
		Instructions    string `json:"instructions"`
		ServerInfo      struct {
			Name string `json:"name"`
		} `json:"serverInfo"`
	}
	if err := json.Unmarshal(r.Result, &res); err != nil {
		return fmt.Errorf("初始化 MCP 会话：响应格式不对：%w", err)
	}
	c.mu.Lock()
	c.version = res.ProtocolVersion
	c.Instructions, c.ServerName = res.Instructions, res.ServerInfo.Name
	c.mu.Unlock()
	if _, _, err := c.post(ctx, request{JSONRPC: "2.0", Method: "notifications/initialized"}); err != nil {
		return fmt.Errorf("初始化 MCP 会话：%w", err)
	}
	c.mu.Lock()
	c.ready = true
	c.mu.Unlock()
	return nil
}

// Call 发一个请求并解出结果。会话不存在或已失效（HTTP 404）时重新初始化后再试一次。
func (c *Client) Call(ctx context.Context, method string, params any, out any) error {
	for attempt := 0; ; attempt++ {
		c.mu.Lock()
		ready := c.ready
		c.mu.Unlock()
		if !ready {
			if err := c.initialize(ctx); err != nil {
				return err
			}
		}
		id := c.id()
		r, status, err := c.post(ctx, request{JSONRPC: "2.0", ID: &id, Method: method, Params: params})
		if status == http.StatusNotFound && attempt == 0 {
			c.mu.Lock()
			c.ready = false
			c.mu.Unlock()
			continue
		}
		if err != nil {
			return err
		}
		if r.Error != nil {
			return r.Error
		}
		if out == nil {
			return nil
		}
		return json.Unmarshal(r.Result, out)
	}
}

// ListTools 取全部工具（跟着 nextCursor 翻页，最多 50 页）。
func (c *Client) ListTools(ctx context.Context) ([]Tool, error) {
	var all []Tool
	cursor := ""
	for page := 0; page < 50; page++ {
		var params any
		if cursor != "" {
			params = map[string]string{"cursor": cursor}
		}
		var res struct {
			Tools      []Tool `json:"tools"`
			NextCursor string `json:"nextCursor"`
		}
		if err := c.Call(ctx, "tools/list", params, &res); err != nil {
			return nil, err
		}
		all = append(all, res.Tools...)
		if res.NextCursor == "" {
			return all, nil
		}
		cursor = res.NextCursor
	}
	return all, nil
}

// CallTool 调用一个工具，原样返回 result（content、structuredContent、isError）。
func (c *Client) CallTool(ctx context.Context, name string, args json.RawMessage) (json.RawMessage, error) {
	if len(args) == 0 {
		args = json.RawMessage("{}")
	}
	var res json.RawMessage
	err := c.Call(ctx, "tools/call", map[string]any{"name": name, "arguments": args}, &res)
	return res, err
}

// TextResult 构造只有一段文字的工具结果。
func TextResult(text string, isError bool) json.RawMessage {
	b, _ := json.Marshal(map[string]any{"content": []map[string]string{{"type": "text", "text": text}}, "isError": isError})
	return b
}
