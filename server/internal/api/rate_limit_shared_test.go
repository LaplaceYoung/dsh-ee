package api_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/api"
	"github.com/hikariming/dsh-ee/server/internal/secret"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// newSharedRateInstance 起一个启用每令牌限流的服务端实例。
// 两个实例共用同一个 store 时，限流计数应该落在数据库里、合计一份额度。
func newSharedRateInstance(t *testing.T, st *store.Store, limit int, window time.Duration) *httptest.Server {
	t.Helper()
	box, err := secret.NewBox(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	opts := api.Options{DataDir: t.TempDir(), RateLimitPerMinute: limit, RateLimitWindow: window}
	srv := api.New(st, box, opts, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts
}

// sharedRateCall 发一次协议面请求。无效令牌会拿到 401，但限流中间件已经先计过数，
// 测试只关心「是否被限流（429）」和 X-RateLimit-* / Retry-After。
func sharedRateCall(t *testing.T, base, method, path, token string) (int, http.Header, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, base+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("请求 %s %s：%v", method, path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, resp.Header, body
}

// waitSharedRateWindow 等到当前固定窗口刚开始、还剩足够时间跑完断言（最多等一个窗口），
// 避免断言过程中跨过窗口边界导致偶发失败。限流窗口按 wall clock 对齐，跨窗口会重新计数。
func waitSharedRateWindow(t *testing.T, window time.Duration) {
	t.Helper()
	now := time.Now()
	remaining := now.Truncate(window).Add(window).Sub(now)
	if remaining < 5*time.Second {
		time.Sleep(remaining + 200*time.Millisecond)
	}
}

func wantSharedRateHeader(t *testing.T, h http.Header, name, want string) {
	t.Helper()
	if got := h.Get(name); got != want {
		t.Fatalf("%s：得到 %q，期望 %q", name, got, want)
	}
}

// 多实例共享配额：limit 次额度是两个实例合计，第 limit+1 次在另一个实例上也 429。
func TestSharedRateLimitAcrossInstances(t *testing.T) {
	st := openTestStore(t)
	a := newSharedRateInstance(t, st, 3, time.Minute)
	b := newSharedRateInstance(t, st, 3, time.Minute)
	waitSharedRateWindow(t, time.Minute)
	now := time.Now()
	token := "ee_at_shared_rate_" + secret.Token()

	// A 实例用掉 2 次、B 实例用掉 1 次：三份额度都算在同一行上。
	for i, inst := range []*httptest.Server{a, a, b} {
		code, header, body := sharedRateCall(t, inst.URL, "GET", "/v1/models", token)
		if code != http.StatusUnauthorized {
			t.Fatalf("第 %d 次（额度内）应 401（令牌无效），得到 %d：%s", i+1, code, body)
		}
		wantSharedRateHeader(t, header, "X-RateLimit-Limit", "3")
		wantSharedRateHeader(t, header, "X-RateLimit-Remaining", strconv.Itoa(2-i))
		reset, err := strconv.ParseInt(header.Get("X-RateLimit-Reset"), 10, 64)
		if err != nil || reset <= now.Unix() || reset > now.Add(61*time.Second).Unix() {
			t.Fatalf("X-RateLimit-Reset 应是本窗口结束的 Unix 秒：%q (%v)", header.Get("X-RateLimit-Reset"), err)
		}
	}

	// 第 4 次换到 B 实例：共享额度已经用完，应 429。
	limited := func() {
		code, header, body := sharedRateCall(t, b.URL, "GET", "/v1/models", token)
		if code != http.StatusTooManyRequests {
			t.Fatalf("跨实例的第 4 次应 429，得到 %d：%s", code, body)
		}
		wantSharedRateHeader(t, header, "X-RateLimit-Remaining", "0")
		wantSharedRateHeader(t, header, "X-RateLimit-Limit", "3")
		if retry, err := strconv.Atoi(header.Get("Retry-After")); err != nil || retry < 1 {
			t.Fatalf("429 应带不小于 1 秒的 Retry-After：%q (%v)", header.Get("Retry-After"), err)
		}
		if !strings.Contains(string(body), "rate_limit_error") {
			t.Fatalf("/v1 的 429 应保持 OpenAI 错误壳：%s", body)
		}
	}
	limited()

	// 数据库里只有一行，计数封顶在 limit+1（多实例共用这一行）。
	var rows, count int
	if err := st.DB().QueryRowContext(context.Background(),
		"SELECT COUNT(*), MAX(count) FROM shared_rate_limits").Scan(&rows, &count); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("同一个令牌只应有一行共享计数，实际 %d 行", rows)
	}
	if count != 4 {
		t.Fatalf("计数应封顶在 limit+1=4，实际 %d", count)
	}
}

// 隔离：不同令牌互不影响；同一个令牌的 /v1 与 /mcp 共用一份额度（跨实例也一样）。
func TestSharedRateLimitIsolatesTokensAcrossInstances(t *testing.T) {
	st := openTestStore(t)
	a := newSharedRateInstance(t, st, 1, time.Minute)
	b := newSharedRateInstance(t, st, 1, time.Minute)
	waitSharedRateWindow(t, time.Minute)
	tokenA := "ee_at_shared_rate_a_" + secret.Token()
	tokenB := "ee_at_shared_rate_b_" + secret.Token()

	// A 的额度在 a 实例上用完。
	if code, _, body := sharedRateCall(t, a.URL, "GET", "/v1/models", tokenA); code != http.StatusUnauthorized {
		t.Fatalf("令牌 A 第一次应 401，得到 %d：%s", code, body)
	}
	// B 有自己的额度，在另一个实例上第一次也应放行。
	if code, _, body := sharedRateCall(t, b.URL, "GET", "/v1/models", tokenB); code != http.StatusUnauthorized {
		t.Fatalf("令牌 B 不该被 A 的额度影响，得到 %d：%s", code, body)
	}
	// A 在 b 实例上继续受限。
	if code, _, body := sharedRateCall(t, b.URL, "GET", "/v1/models", tokenA); code != http.StatusTooManyRequests {
		t.Fatalf("令牌 A 在另一实例上应继续受限，得到 %d：%s", code, body)
	}
	// B 的 /mcp 与 /v1 共用额度：在 b 实例用完 /v1 后，/mcp 在 a 实例上也应 429。
	if code, _, body := sharedRateCall(t, a.URL, "POST", "/mcp", tokenB); code != http.StatusTooManyRequests {
		t.Fatalf("同一令牌的 /v1 与 /mcp 应共用额度，得到 %d：%s", code, body)
	}
}

// 数据库不可用（表缺失/连接断开）时退回单实例内存限流：仍返回 X-RateLimit-* 且会 429，
// 不 fail open（不限流）、也不把协议面打成 5xx。
func TestSharedRateLimitFallsBackWhenDBUnavailable(t *testing.T) {
	st := openTestStore(t)
	if _, err := st.DB().ExecContext(context.Background(), "DROP TABLE shared_rate_limits"); err != nil {
		t.Fatal(err)
	}
	ts := newSharedRateInstance(t, st, 1, time.Minute)
	token := "ee_at_shared_rate_db_down_" + secret.Token()

	first, header, body := sharedRateCall(t, ts.URL, "GET", "/v1/models", token)
	if first != http.StatusUnauthorized {
		t.Fatalf("数据库不可用时第一次仍应正常处理请求（401），得到 %d：%s", first, body)
	}
	wantSharedRateHeader(t, header, "X-RateLimit-Limit", "1")
	wantSharedRateHeader(t, header, "X-RateLimit-Remaining", "0")

	second, header, body := sharedRateCall(t, ts.URL, "GET", "/v1/models", token)
	if second != http.StatusTooManyRequests {
		t.Fatalf("退回内存限流后第二次应 429（不是 fail open），得到 %d：%s", second, body)
	}
	if retry, err := strconv.Atoi(header.Get("Retry-After")); err != nil || retry < 1 {
		t.Fatalf("降级后的 429 也应带 Retry-After：%q (%v)", header.Get("Retry-After"), err)
	}
}

// 共享计数只存 key 的摘要：令牌明文不落库。
func TestSharedRateLimitKeyIsHashed(t *testing.T) {
	st := openTestStore(t)
	ts := newSharedRateInstance(t, st, 10, time.Minute)
	plain := "ee_at_shared_rate_plain_" + secret.Token()
	if code, _, body := sharedRateCall(t, ts.URL, "GET", "/v1/models", plain); code != http.StatusUnauthorized {
		t.Fatalf("请求应正常处理：%d %s", code, body)
	}
	var key string
	if err := st.DB().QueryRowContext(context.Background(),
		"SELECT rate_key FROM shared_rate_limits").Scan(&key); err != nil {
		t.Fatal(err)
	}
	want := "token:" + secret.Hash(plain)
	if key != want {
		t.Fatalf("限流 key 应是令牌的 sha256，得到 %q，期望 %q", key, want)
	}
	if strings.Contains(key, plain) {
		t.Fatalf("限流 key 里不能有令牌明文：%q", key)
	}
}
