package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/hikariming/dsh-ee/server/internal/secret"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// TestKeyRateBoundedKeyCount 卡住 keyRate 的上界：键来自攻击者可控的输入
// （/x/<token>/… 的客户端 IP），超出上限后必须进溢出桶，map 不能无界增长，
// 也不能把每个请求都变成 O(n) 全表扫描。
func TestKeyRateBoundedKeyCount(t *testing.T) {
	l := &keyRate{limit: 2}
	for i := 0; i < maxExtRateKeys; i++ {
		if !l.allow("ip-" + strconv.Itoa(i)) {
			t.Fatalf("前 %d 个键各自第一次都应放行（第 %d 个被限流）", maxExtRateKeys, i+1)
		}
	}
	// 超过上限的陌生键全部进溢出桶，不再新增 map 条目。
	for i := 0; i < 100; i++ {
		l.allow("new-" + strconv.Itoa(i))
	}
	if len(l.m) > maxExtRateKeys+1 {
		t.Fatalf("限流器的键数应有上限：%d", len(l.m))
	}
	if _, ok := l.m[extRateOverflowID]; !ok {
		t.Fatal("超限后的新键应记进溢出桶")
	}
	// 溢出桶早已超过额度：任何陌生键都不应再放行，且不应 panic。
	if l.allow("another-unknown-key") {
		t.Fatal("溢出桶超过额度后，陌生键不应再放行")
	}
	// 已经有窗口的键不受溢出影响，继续按自己的窗口计数（limit=2）。
	if !l.allow("ip-0") {
		t.Fatal("已有键的第二次请求应在额度内")
	}
	if l.allow("ip-0") {
		t.Fatal("已有键的第 3 次请求应被限流")
	}
}

// TestExtEndpointRateLimitKeyedByClientIP 卡住 /x/<token>/… 的限流键：
// 同一个客户端 IP 换不同的随机 token 也必须共用一份额度；不同 IP 互不影响。
// 修复前限流键是 token，每次换随机 token 就是一个新键，这里第 limit+1 次会 404 而不是 429。
func TestExtEndpointRateLimitKeyedByClientIP(t *testing.T) {
	st := openExtTestStore(t)
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	box, err := secret.NewBox(key)
	if err != nil {
		t.Fatalf("new box: %v", err)
	}
	srv := New(st, box, Options{DataDir: t.TempDir()}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	const limit = 3
	// 用很小的额度，免得测试里真打出 120 个请求。
	srv.extEndpointRate = keyRate{limit: limit}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	// 每次都换一个随机 token（攻击者的做法），经可信代理头指定客户端 IP。
	hit := func(clientIP string) int {
		t.Helper()
		raw := make([]byte, 16)
		if _, err := rand.Read(raw); err != nil {
			t.Fatalf("random token: %v", err)
		}
		token := hex.EncodeToString(raw) // 32 个字符，满足回调地址的最短长度
		req, err := http.NewRequest(http.MethodGet, ts.URL+"/x/"+token+"/x", nil)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		// 测试服务器的直连对端是回环地址（可信代理），所以这个头会被采信。
		req.Header.Set("X-Forwarded-For", clientIP)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("请求回调地址失败：%v", err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}

	const attackerIP = "203.0.113.10"
	for i := 0; i < limit; i++ {
		if code := hit(attackerIP); code != http.StatusNotFound {
			t.Fatalf("同一 IP 的第 %d 个随机 token 应放行（token 不存在 → 404），得到 %d", i+1, code)
		}
	}
	if code := hit(attackerIP); code != http.StatusTooManyRequests {
		t.Fatalf("同一 IP 换随机 token 也应按 IP 限流：得到 %d，期望 429", code)
	}
	// 另一个 IP 有自己的额度，不受攻击者 IP 的影响。
	if code := hit("203.0.113.11"); code != http.StatusNotFound {
		t.Fatalf("另一个 IP 不应受该 IP 的额度影响：得到 %d，期望 404", code)
	}
}

// openExtTestStore 开一个临时库跑数据库集成测试；没有 DSN 时跳过。
func openExtTestStore(t *testing.T) *store.Store {
	t.Helper()
	base := os.Getenv("DSH_EE_TEST_DSN")
	if base == "" {
		t.Skip("未设置 DSH_EE_TEST_DSN，跳过数据库集成测试")
	}
	suffix := make([]byte, 4)
	_, _ = rand.Read(suffix)
	dbName := "dsh_ee_test_xrate_" + hex.EncodeToString(suffix)
	dsn := strings.Replace(base, ")/", ")/"+dbName, 1)
	ctx := context.Background()
	st, err := store.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() {
		_, _ = st.DB().ExecContext(context.Background(), "DROP DATABASE `"+dbName+"`")
		st.Close()
	})
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return st
}
