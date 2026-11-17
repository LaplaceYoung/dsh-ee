package api

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hikariming/dsh-ee/server/internal/secret"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// newDefaultUpstreamServer 建一个不注入 Options.Upstream 的 Server，拿到的就是生产默认
// 上游客户端（New 里建的那只）。这条路径不碰数据库，nil store/box 够用。
func newDefaultUpstreamServer(t *testing.T) *Server {
	t.Helper()
	return New(nil, nil, Options{DataDir: t.TempDir()}, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// TestUpstreamDefaultClientRejectsCrossHostRedirect 覆盖漏洞场景：provider 的 base_url 指向
// 的上游（或中间人）返回 302/307 到另一台主机时，默认客户端不能跟过去。跟过去意味着
// GET /models 的响应被回给调用方（内网探测），以及 307 把 POST 请求体（消息内容）回放给新地址。
func TestUpstreamDefaultClientRejectsCrossHostRedirect(t *testing.T) {
	var stolen atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stolen.Add(1)
		_, _ = io.WriteString(w, `{"data":[{"id":"pwned"}]}`)
	}))
	defer destination.Close()

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/models":
			http.Redirect(w, r, destination.URL+"/internal/models", http.StatusFound)
		case "/chat/completions":
			http.Redirect(w, r, destination.URL+"/internal/chat", http.StatusTemporaryRedirect)
		case "/ok":
			// 同一主机内换路径的跳转是正常上游行为，仍应跟随。
			http.Redirect(w, r, "/ok2", http.StatusFound)
		case "/ok2":
			_, _ = io.WriteString(w, "ok")
		default:
			http.NotFound(w, r)
		}
	}))
	defer origin.Close()

	s := newDefaultUpstreamServer(t)

	// GET /models 的 302：不能跟。
	resp, err := s.upstream.Get(origin.URL + "/models")
	if err == nil {
		resp.Body.Close()
		t.Fatalf("跨主机 302 被跟随后拿到了 %d 响应", resp.StatusCode)
	}
	if !strings.Contains(err.Error(), "重定向") {
		t.Fatalf("错误应说明是重定向被拦下：%v", err)
	}

	// 307 POST：不能把请求体重放到另一个地址。
	resp, err = s.upstream.Post(origin.URL+"/chat/completions", "application/json",
		strings.NewReader(`{"model":"chat","messages":[{"role":"user","content":"secret"}]}`))
	if err == nil {
		resp.Body.Close()
		t.Fatalf("跨主机 307 被跟随，请求体已回放")
	}
	if n := stolen.Load(); n != 0 {
		t.Fatalf("重定向目标共收到 %d 个请求，跨主机跳转没有被拦住", n)
	}

	// 同主机换路径仍要放行，避免把正常上游的跳转一刀切拦掉。
	resp, err = s.upstream.Get(origin.URL + "/ok")
	if err != nil {
		t.Fatalf("同主机重定向不应被拦：%v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || string(body) != "ok" {
		t.Fatalf("同主机重定向结果：%d %q", resp.StatusCode, body)
	}
}

// TestUpstreamCheckProviderRejectsCrossHostRedirect 确认连通性检查走的是同一只默认客户端：
// 上游 302 到另一台主机时检查失败，企业 key 不会被送到新地址，内网响应也不会写进检查结果。
func TestUpstreamCheckProviderRejectsCrossHostRedirect(t *testing.T) {
	var stolenAuth atomic.Value
	var stolen atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stolen.Add(1)
		stolenAuth.Store(r.Header.Get("Authorization"))
		_, _ = io.WriteString(w, `{"data":[{"id":"pwned"}]}`)
	}))
	defer destination.Close()

	var upstreamAuth atomic.Value
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamAuth.Store(r.Header.Get("Authorization"))
		http.Redirect(w, r, destination.URL+"/v1/models", http.StatusFound)
	}))
	defer origin.Close()

	s := newDefaultUpstreamServer(t)
	box, err := secret.NewBox(bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatalf("建加密盒：%v", err)
	}
	s.box = box
	enc, err := box.Seal([]byte("sk-enterprise-secret"))
	if err != nil {
		t.Fatalf("加密 key：%v", err)
	}

	res := s.checkProvider(context.Background(), &store.Provider{BaseURL: origin.URL, APIKeyEnc: enc}, "")

	if res.OK {
		t.Fatalf("跨主机重定向不应算连接正常：%+v", res)
	}
	if n := stolen.Load(); n != 0 {
		t.Fatalf("企业 key 被送到重定向目标（%v）", stolenAuth.Load())
	}
	if got, _ := upstreamAuth.Load().(string); got != "Bearer sk-enterprise-secret" {
		t.Fatalf("原上游应收到企业 key，实际 %q", got)
	}
	if !strings.Contains(res.Message, "重定向") {
		t.Fatalf("检查结果应说明重定向被拦：%q", res.Message)
	}
}
