package api

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 回调地址里的令牌是免登录口令，访问日志里只能留 <redacted>。
func TestRedactLogPath(t *testing.T) {
	// 与真实生成的回调令牌同形：43 个 URL 安全字符。
	const token = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFG"
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"只有令牌段", "/x/" + token, "/x/<redacted>"},
		{"令牌段带尾斜杠", "/x/" + token + "/", "/x/<redacted>/"},
		{"令牌后有子路径", "/x/" + token + "/x", "/x/<redacted>/x"},
		{"令牌后有多段与尾斜杠", "/x/" + token + "/events/approve/", "/x/<redacted>/events/approve/"},
		{"令牌内容大小写不影响", "/x/AbC-_.~+/x", "/x/<redacted>/x"},
		{"首段大写的 X 不是回调路由", "/X/" + token + "/x", "/X/" + token + "/x"},
		{"普通 /api 路径", "/api/admin/connections", "/api/admin/connections"},
		{"/x/ 本身", "/x/", "/x/"},
		{"只有 /x", "/x", "/x"},
		{"/xy/ 前缀不是回调路由", "/xy/" + token, "/xy/" + token},
		{"第二段为空", "/x//x", "/x//x"},
		{"空路径", "", ""},
		{"不带前导斜杠", "x/" + token, "x/" + token},
		{"路径中间出现 /x/", "/api" + "/x/" + token + "/x", "/api/x/" + token + "/x"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := redactLogPath(c.in); got != c.want {
				t.Fatalf("redactLogPath(%q) = %q，期望 %q", c.in, got, c.want)
			}
		})
	}
}

// logRequests 只改 path 一个字段：method/status/耗时/request_id 照旧，短路径原样记录，
// 令牌无论是命中回调路由还是 404 都不会进日志。
func TestRedactLogPathInLogRequests(t *testing.T) {
	const token = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFG"
	var buf bytes.Buffer
	s := &Server{log: slog.New(slog.NewTextHandler(&buf, nil))}
	h := s.logRequests(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/x/"+token+"/events/approve?x=1", nil))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/me", nil))

	out := buf.String()
	if strings.Contains(out, token) {
		t.Fatalf("访问日志不应出现回调令牌：%q", out)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Fatalf("应有两条访问日志，实际 %d：%q", len(lines), out)
	}
	for _, want := range []string{"level=INFO", "msg=http", "method=POST", "path=/x/<redacted>/events/approve", "status=201"} {
		if !strings.Contains(lines[0], want) {
			t.Fatalf("第一条日志缺 %q：%q", want, lines[0])
		}
	}
	for _, key := range []string{"ms=", "request_id"} {
		if !strings.Contains(lines[0], key) {
			t.Fatalf("第一条日志缺字段 %q：%q", key, lines[0])
		}
	}
	for _, want := range []string{"method=GET", "path=/api/me", "status=201"} {
		if !strings.Contains(lines[1], want) {
			t.Fatalf("第二条日志缺 %q：%q", want, lines[1])
		}
	}
}
