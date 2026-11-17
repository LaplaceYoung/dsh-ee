package api_test

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/api"
	"github.com/hikariming/dsh-ee/server/internal/rbac"
)

// mcpStream 是一条已经建立（或已被拒绝）的 GET /mcp 事件流。
type mcpStream struct {
	resp   *http.Response
	cancel context.CancelFunc
}

func (c *mcpStream) close() {
	if c == nil {
		return
	}
	c.cancel()
	_ = c.resp.Body.Close()
}

// openMCPStream 发一次 SSE 请求；不检查状态码，调用方自己看 200 / 429。
func openMCPStream(url, token string) (*mcpStream, error) {
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		cancel()
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		return nil, err
	}
	return &mcpStream{resp: resp, cancel: cancel}, nil
}

// waitListChanged 在一条事件流上等一条 tools/list_changed。
func waitListChanged(t *testing.T, c *mcpStream, what string) {
	t.Helper()
	got := make(chan bool, 1)
	go func() {
		sc := bufio.NewScanner(c.resp.Body)
		for sc.Scan() {
			if strings.Contains(sc.Text(), "notifications/tools/list_changed") {
				got <- true
				return
			}
		}
		got <- false
	}()
	select {
	case ok := <-got:
		if !ok {
			t.Fatalf("%s：事件流结束也没收到 list_changed", what)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("%s：没有收到 tools/list_changed", what)
	}
}

// TestMCPStreamsPerUserLimit 验证 /mcp 事件流的每用户并发上限：上限内的连接照常建立、
// 照常收事件；超出的连接在建立阶段拿到 429；断开后名额释放，能再建。
func TestMCPStreamsPerUserLimit(t *testing.T) {
	const limit = 2
	f := newPrincipalFixtureWith(t, func(o *api.Options) { o.MCPMaxStreamsPerUser = limit })
	u := f.user(t, "sse-cap-user", nil)
	scopes := []string{rbac.ScopeHub, rbac.ScopeConnections}
	f.token(t, u.ID, "ee_at_sse_t1", scopes, nil, nil)
	f.token(t, u.ID, "ee_at_sse_t2", scopes, nil, nil)
	url := f.ts.URL + "/mcp"

	// 并发建立 limit+1 条：恰好 limit 条 200，其余 429。
	type result struct {
		stream *mcpStream
		err    error
	}
	results := make(chan result, limit+1)
	var wg sync.WaitGroup
	for i := 0; i < limit+1; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s, err := openMCPStream(url, "ee_at_sse_t1")
			results <- result{stream: s, err: err}
		}()
	}
	wg.Wait()
	close(results)
	var live, rejected []*mcpStream
	for r := range results {
		if r.err != nil {
			t.Fatalf("打开事件流：%v", r.err)
		}
		if r.stream.resp.StatusCode == http.StatusOK {
			live = append(live, r.stream)
		} else {
			rejected = append(rejected, r.stream)
		}
	}
	t.Cleanup(func() {
		for _, c := range append(live, rejected...) {
			c.close()
		}
	})
	if len(live) != limit || len(rejected) != 1 {
		t.Fatalf("并发 %d 条（上限 %d）：%d 条建立、%d 条被拒", limit+1, limit, len(live), len(rejected))
	}
	r := rejected[0]
	if code := r.resp.StatusCode; code != http.StatusTooManyRequests {
		t.Fatalf("超限的连接应 429，实际 %d", code)
	}
	if r.resp.Header.Get("Retry-After") == "" {
		t.Fatal("429 应带 Retry-After")
	}
	body, _ := io.ReadAll(r.resp.Body)
	if !strings.Contains(string(body), "最多 2 条") {
		t.Fatalf("429 文案要说明上限：%s", body)
	}
	r.close()

	// 同一个人的另一枚令牌也共用这份名额：按用户计数，多签令牌绕不过去。
	otherToken, err := openMCPStream(url, "ee_at_sse_t2")
	if err != nil {
		t.Fatalf("打开事件流：%v", err)
	}
	if code := otherToken.resp.StatusCode; code != http.StatusTooManyRequests {
		t.Fatalf("同一用户的另一枚令牌也应受上限约束，实际 %d", code)
	}
	otherToken.close()

	// 已建立的连接不回归：改员工资料触发 hub.notify，两条流都要收到 list_changed。
	code, respBody := f.admin.do("PATCH", "/api/admin/users/"+itoa(u.ID), map[string]any{"display_name": "SSE 上限测试"}, nil)
	expect(t, "改员工资料", code, 200, respBody)
	for i, c := range live {
		waitListChanged(t, c, "已建立的事件流 "+itoa(int64(i+1)))
	}

	// 断开两条后名额释放：轮询重开直到 200（服务端 handler 返回与 defer 释放是异步的）。
	for _, c := range live {
		c.close()
	}
	deadline := time.Now().Add(5 * time.Second)
	var reopened *mcpStream
	for {
		s, err := openMCPStream(url, "ee_at_sse_t2")
		if err != nil {
			t.Fatalf("重开事件流：%v", err)
		}
		if s.resp.StatusCode == http.StatusOK {
			reopened = s
			break
		}
		s.close()
		if time.Now().After(deadline) {
			t.Fatalf("断开后 5s 内名额没释放：最后一次 %d", s.resp.StatusCode)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Cleanup(reopened.close)

	// 释放出来的是恰好一个名额：再建一条仍 200，第三条又该 429。
	second, err := openMCPStream(url, "ee_at_sse_t1")
	if err != nil {
		t.Fatalf("重开第二条事件流：%v", err)
	}
	t.Cleanup(second.close)
	if code := second.resp.StatusCode; code != http.StatusOK {
		t.Fatalf("释放一个名额后第二条应能建立，实际 %d", code)
	}
	third, err := openMCPStream(url, "ee_at_sse_t1")
	if err != nil {
		t.Fatalf("打开第三条事件流：%v", err)
	}
	t.Cleanup(third.close)
	if code := third.resp.StatusCode; code != http.StatusTooManyRequests {
		t.Fatalf("名额用满后第三条应 429，实际 %d", code)
	}
}
