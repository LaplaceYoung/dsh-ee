package api_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/hikariming/dsh-ee/server/internal/api"
	"github.com/hikariming/dsh-ee/server/internal/secret"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// 需要真实的 MySQL 协议数据库（TiDB / OceanBase）。例：
//
//	DSH_EE_TEST_DSN='root:@tcp(127.0.0.1:4000)/' go test ./...
//
// 每次测试用一个随机库，结束后删除。
//
// 全量迁移是这个包测试耗时的大头（每个测试一次，几十个 DDL），而 TiDB 能并发跑
// 不同库的 DDL。所以池子空了时一次并发建好并迁移 testDBBatch 个库，之后的测试
// 直接拿现成的。每个测试仍然独占一个刚迁移完、没人用过的新库，语义不变。
//
// 只在测试自己调用 openTestStore 时（也就是测试主体还没开始跑时）才迁移，不在后台
// 迁移：后台 DDL 会拖慢同时在跑的测试，而不少测试对时间窗口敏感（例如几百毫秒的
// 密钥轮换延迟）。
func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	base := os.Getenv("DSH_EE_TEST_DSN")
	if base == "" {
		t.Skip("未设置 DSH_EE_TEST_DSN，跳过数据库集成测试")
	}
	p := testDBs.take(base)
	if p.err != nil {
		t.Fatalf("准备测试库: %v", p.err)
	}
	st := p.st
	t.Cleanup(func() { dropTestDB(st, p.name) })
	// 迁移可重复执行
	if err := st.Migrate(context.Background()); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	return st
}

// 一次并发准备几个库。再多 TiDB 的 DDL 也快不了多少，还会在包结束时多剩几个没用上的库。
const testDBBatch = 4

type preparedDB struct {
	st   *store.Store
	name string
	err  error
}

var testDBs testDBPool

type testDBPool struct {
	mu    sync.Mutex
	ready []preparedDB
}

func (p *testDBPool) take(base string) preparedDB {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.ready) == 0 {
		batch := make([]preparedDB, testDBBatch)
		var wg sync.WaitGroup
		for i := range batch {
			wg.Add(1)
			go func() {
				defer wg.Done()
				batch[i] = prepareTestDB(base)
			}()
		}
		wg.Wait()
		p.ready = batch
	}
	db := p.ready[0]
	p.ready = p.ready[1:]
	return db
}

// drain 删掉已经建好但没被用上的库。
func (p *testDBPool) drain() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, db := range p.ready {
		if db.err == nil {
			dropTestDB(db.st, db.name)
		}
	}
	p.ready = nil
}

func prepareTestDB(base string) preparedDB {
	suffix := make([]byte, 4)
	_, _ = rand.Read(suffix)
	dbName := "dsh_ee_test_" + hex.EncodeToString(suffix)
	dsn := strings.Replace(base, ")/", ")/"+dbName, 1)
	ctx := context.Background()
	st, err := store.Open(ctx, dsn)
	if err != nil {
		return preparedDB{err: fmt.Errorf("open: %w", err)}
	}
	if err := st.Migrate(ctx); err != nil {
		dropTestDB(st, dbName)
		return preparedDB{err: fmt.Errorf("migrate: %w", err)}
	}
	return preparedDB{st: st, name: dbName}
}

func dropTestDB(st *store.Store, name string) {
	_, _ = st.DB().ExecContext(context.Background(), "DROP DATABASE `"+name+"`")
	st.Close()
}

func TestMain(m *testing.M) {
	code := m.Run()
	testDBs.drain()
	os.Exit(code)
}

const upstreamSecret = "sk-upstream-secret-1234"

// fakeUpstream 模拟 OpenAI 兼容的上游，检查转发过来的鉴权与流式用量参数。
func fakeUpstream(t *testing.T, sawIncludeUsage *atomic.Bool) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+upstreamSecret {
			t.Errorf("上游收到的 Authorization 不对：%q", r.Header.Get("Authorization"))
			w.WriteHeader(401)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if stream, _ := body["stream"].(bool); stream {
			if opts, _ := body["stream_options"].(map[string]any); opts["include_usage"] == true {
				sawIncludeUsage.Store(true)
			}
			w.Header().Set("Content-Type", "text/event-stream")
			for _, chunk := range []string{
				`{"choices":[{"delta":{"content":"你"}}]}`,
				`{"choices":[{"delta":{"content":"好"}}]}`,
				`{"choices":[],"usage":{"prompt_tokens":8,"completion_tokens":7,"total_tokens":15}}`,
				`[DONE]`,
			} {
				_, _ = io.WriteString(w, "data: "+chunk+"\n\n")
				w.(http.Flusher).Flush()
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"x","choices":[{"message":{"role":"assistant","content":"你好"}}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`)
	}))
}

type client struct {
	t    *testing.T
	base string
	hc   *http.Client
}

func newClient(t *testing.T, base string) *client {
	jar, _ := cookiejar.New(nil)
	return &client{t: t, base: base, hc: &http.Client{Jar: jar}}
}

func (c *client) do(method, path string, body any, header map[string]string) (int, []byte) {
	c.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.base+path, rd)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range header {
		req.Header.Set(k, v)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		c.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, data
}

func expect(t *testing.T, what string, got, want int, body []byte) {
	t.Helper()
	if got != want {
		t.Fatalf("%s：状态码 %d，期望 %d；响应 %s", what, got, want, body)
	}
}

func TestLoginAndUserQuota(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if ok, err := api.Bootstrap(ctx, st, "admin", "admin-password-123"); err != nil || !ok {
		t.Fatalf("bootstrap: %v %v", ok, err)
	}
	if ok, _ := api.Bootstrap(ctx, st, "admin2", "admin-password-123"); ok {
		t.Fatal("已有用户时不应再创建管理员")
	}

	var sawIncludeUsage atomic.Bool
	up := fakeUpstream(t, &sawIncludeUsage)
	defer up.Close()

	key := make([]byte, 32)
	_, _ = rand.Read(key)
	box, _ := secret.NewBox(key)
	srv := api.New(st, box, api.Options{Upstream: up.Client()}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	admin := newClient(t, ts.URL)

	// 登录
	code, body := admin.do("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "wrong-password"}, nil)
	expect(t, "错误密码", code, 401, body)
	req, _ := http.NewRequest("POST", ts.URL+"/api/auth/login", strings.NewReader(`username=admin`))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, _ := http.DefaultClient.Do(req)
	expect(t, "表单登录应被拒（CSRF 防护）", resp.StatusCode, 415, nil)
	resp.Body.Close()
	code, body = admin.do("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "admin-password-123"}, nil)
	expect(t, "管理员登录", code, 200, body)
	code, body = admin.do("GET", "/api/auth/me", nil, nil)
	expect(t, "me", code, 200, body)

	// 上游：响应里不能出现明文 key
	code, body = admin.do("POST", "/api/admin/providers", map[string]any{
		"name": "deepseek", "base_url": up.URL, "api_key": upstreamSecret,
		"models": []string{"deepseek-chat", "deepseek-reasoner"},
	}, nil)
	expect(t, "创建上游", code, 201, body)
	if bytes.Contains(body, []byte(upstreamSecret)) {
		t.Fatalf("上游明文 key 被返回了：%s", body)
	}

	// 员工 alice：新用户默认额度为 0，管理员分配后才能调用
	code, body = admin.do("POST", "/api/admin/users", map[string]any{"username": "alice", "password": "short"}, nil)
	expect(t, "弱密码", code, 400, body)
	code, body = admin.do("POST", "/api/admin/users", map[string]any{
		"username": "alice", "password": "alice-password-1", "display_name": "Alice", "department": "研发效能部",
	}, nil)
	expect(t, "创建员工", code, 201, body)
	var alice store.User
	_ = json.Unmarshal(body, &alice)
	if alice.MonthlyTokenLimit == nil || *alice.MonthlyTokenLimit != 0 {
		t.Fatalf("新用户应默认 0 额度：%s", body)
	}

	// 给 alice 分配：每月 20 token，只能用 deepseek-chat
	code, body = admin.do("PATCH", "/api/admin/users/"+itoa(alice.ID)+"/quota", map[string]any{
		"monthly_token_limit": 20, "allowed_models": []string{"deepseek-chat"},
	}, nil)
	expect(t, "分配额度", code, 200, body)
	var aliceQuota store.User
	_ = json.Unmarshal(body, &aliceQuota)
	if aliceQuota.MonthlyTokenLimit == nil || *aliceQuota.MonthlyTokenLimit != 20 ||
		len(aliceQuota.AllowedModels) != 1 || aliceQuota.AllowedModels[0] != "deepseek-chat" {
		t.Fatalf("额度没有保存：%s", body)
	}
	code, body = admin.do("PATCH", "/api/admin/users/"+itoa(alice.ID)+"/quota", map[string]any{"monthly_token_limit": -1}, nil)
	expect(t, "负数额度", code, 400, body)
	code, body = admin.do("PATCH", "/api/admin/users/"+itoa(alice.ID)+"/quota", map[string]any{"quota_expires_at": "2020-01-01T00:00:00Z"}, nil)
	expect(t, "过去的额度有效期", code, 400, body)

	// alice 登录后自助创建两个 key：key 只是凭证，额度按用户共享
	ac := newClient(t, ts.URL)
	code, body = ac.do("POST", "/api/auth/login", map[string]string{"username": "alice", "password": "alice-password-1"}, nil)
	expect(t, "alice 登录", code, 200, body)
	mkKey := func(name string) (string, store.APIKey) {
		t.Helper()
		code, body := ac.do("POST", "/api/me/keys", map[string]any{"name": name}, nil)
		expect(t, "自助创建 key", code, 201, body)
		var issued struct {
			Key    string       `json:"key"`
			APIKey store.APIKey `json:"api_key"`
		}
		_ = json.Unmarshal(body, &issued)
		if !strings.HasPrefix(issued.Key, "sk-ee-") || !strings.HasPrefix(issued.Key, issued.APIKey.Prefix) {
			t.Fatalf("key 格式不对：%s", body)
		}
		if issued.APIKey.CreatedBy != alice.ID {
			t.Fatalf("自助创建的 key 应记在员工自己名下：%s", body)
		}
		return issued.Key, issued.APIKey
	}
	key1, k1 := mkKey("alice 的 DSH")
	key2, k2 := mkKey("alice 的脚本")
	bearer1 := map[string]string{"Authorization": "Bearer " + key1}
	bearer2 := map[string]string{"Authorization": "Bearer " + key2}

	type myKeysResp struct {
		Keys  []store.APIKey `json:"keys"`
		Quota struct {
			MonthlyTokenLimit *int64   `json:"monthly_token_limit"`
			MonthTokens       int64    `json:"month_tokens"`
			AllowedModels     []string `json:"allowed_models"`
			ActiveKeys        int      `json:"active_keys"`
			MaxKeys           int      `json:"max_keys"`
		} `json:"quota"`
	}
	code, body = ac.do("GET", "/api/me/keys", nil, nil)
	expect(t, "我的 key", code, 200, body)
	var mine myKeysResp
	_ = json.Unmarshal(body, &mine)
	if len(mine.Keys) != 2 || mine.Quota.MonthTokens != 0 || mine.Quota.ActiveKeys != 2 || mine.Quota.MaxKeys != 10 ||
		mine.Quota.MonthlyTokenLimit == nil || *mine.Quota.MonthlyTokenLimit != 20 {
		t.Fatalf("我的 key / 额度不对：%s", body)
	}

	gw := newClient(t, ts.URL)
	code, body = gw.do("GET", "/v1/models", nil, map[string]string{"Authorization": "Bearer sk-ee-bogus"})
	expect(t, "假 key", code, 401, body)
	code, body = gw.do("GET", "/v1/models", nil, bearer1)
	expect(t, "模型列表", code, 200, body)
	if !bytes.Contains(body, []byte("deepseek-chat")) || bytes.Contains(body, []byte("deepseek-reasoner")) {
		t.Fatalf("模型列表应只含额度允许的模型：%s", body)
	}

	chat := func(bearer map[string]string, model string, stream bool) (int, []byte) {
		return gw.do("POST", "/v1/chat/completions", map[string]any{
			"model": model, "stream": stream, "messages": []map[string]string{{"role": "user", "content": "你好"}},
		}, bearer)
	}
	code, body = chat(bearer1, "deepseek-reasoner", false)
	expect(t, "不允许的模型", code, 403, body)
	code, body = chat(bearer1, "deepseek-chat", false)
	expect(t, "非流式调用", code, 200, body)
	code, body = chat(bearer2, "deepseek-chat", true)
	expect(t, "另一个 key 的流式调用", code, 200, body)
	if !bytes.Contains(body, []byte("data: [DONE]")) || !bytes.Contains(body, []byte(`"好"`)) {
		t.Fatalf("流式响应没有完整转发：%s", body)
	}
	if !sawIncludeUsage.Load() {
		t.Fatal("流式请求应带上 stream_options.include_usage")
	}
	// 两个 key 各用 15 token，共享同一份 20 token 额度
	code, body = chat(bearer1, "deepseek-chat", false)
	expect(t, "共享额度用完后超额", code, 429, body)
	if !bytes.Contains(body, []byte("monthly_quota_exceeded")) {
		t.Fatalf("超额错误码不对：%s", body)
	}

	// 额度用量按用户统计，key 上只显示各自的用量
	code, body = ac.do("GET", "/api/me/keys", nil, nil)
	expect(t, "超额后的我的 key", code, 200, body)
	mine = myKeysResp{}
	_ = json.Unmarshal(body, &mine)
	if mine.Quota.MonthTokens != 30 {
		t.Fatalf("用户本月用量应为 30：%s", body)
	}
	for _, k := range mine.Keys {
		if k.MonthTokens != 15 {
			t.Fatalf("每个 key 的用量应为 15：%s", body)
		}
	}

	// 员工可以吊销自己的 key，但动不了别人的
	code, body = ac.do("DELETE", "/api/me/keys/"+itoa(k2.ID), nil, nil)
	expect(t, "吊销自己的 key", code, 204, body)
	code, body = chat(bearer2, "deepseek-chat", false)
	expect(t, "已吊销的 key", code, 401, body)

	// 管理员也能吊销任意 key
	code, body = admin.do("DELETE", "/api/admin/keys/"+itoa(k1.ID), nil, nil)
	expect(t, "管理员吊销", code, 204, body)
	code, body = chat(bearer1, "deepseek-chat", false)
	expect(t, "管理员吊销后的 key", code, 401, body)

	// 管理员仍可代发 key：额度仍按用户算，alice 已用完 → 429
	code, body = admin.do("POST", "/api/admin/keys", map[string]any{"user_id": alice.ID, "name": "运维代发"}, nil)
	expect(t, "管理员代发 key", code, 201, body)
	var proxyIssued struct {
		Key string `json:"key"`
	}
	_ = json.Unmarshal(body, &proxyIssued)
	code, body = chat(map[string]string{"Authorization": "Bearer " + proxyIssued.Key}, "deepseek-chat", false)
	expect(t, "代发 key 共享用户额度", code, 429, body)

	// 未分配额度的用户：能建 key，但调用会被拒；管理员可改成不限额
	code, body = admin.do("POST", "/api/admin/users", map[string]any{"username": "bob", "password": "bob-password-1"}, nil)
	expect(t, "创建 bob", code, 201, body)
	var bob store.User
	_ = json.Unmarshal(body, &bob)
	bc := newClient(t, ts.URL)
	code, body = bc.do("POST", "/api/auth/login", map[string]string{"username": "bob", "password": "bob-password-1"}, nil)
	expect(t, "bob 登录", code, 200, body)
	bobKey, _ := func() (string, store.APIKey) {
		code, body := bc.do("POST", "/api/me/keys", map[string]any{"name": "bob 的 key"}, nil)
		expect(t, "bob 建 key", code, 201, body)
		var issued struct {
			Key    string       `json:"key"`
			APIKey store.APIKey `json:"api_key"`
		}
		_ = json.Unmarshal(body, &issued)
		return issued.Key, issued.APIKey
	}()
	code, body = chat(map[string]string{"Authorization": "Bearer " + bobKey}, "deepseek-chat", false)
	expect(t, "未分配额度", code, 429, body)
	if !bytes.Contains(body, []byte("还没有给你分配额度")) {
		t.Fatalf("未分配额度的错误信息不对：%s", body)
	}
	code, body = admin.do("PATCH", "/api/admin/users/"+itoa(bob.ID)+"/quota", map[string]any{"monthly_token_limit": nil}, nil)
	expect(t, "改成不限额", code, 200, body)
	code, body = chat(map[string]string{"Authorization": "Bearer " + bobKey}, "deepseek-chat", false)
	expect(t, "不限额后调用", code, 200, body)
	code, body = admin.do("PATCH", "/api/admin/users/"+itoa(bob.ID)+"/quota", map[string]any{"monthly_token_limit": 0}, nil)
	expect(t, "改回 0 额度", code, 200, body)
	code, body = chat(map[string]string{"Authorization": "Bearer " + bobKey}, "deepseek-chat", false)
	expect(t, "0 额度", code, 429, body)

	// 自助建 key 有数量上限（默认 10 个有效 key）
	for i := 0; i < 9; i++ {
		code, body = bc.do("POST", "/api/me/keys", map[string]any{"name": "bob 的 key " + itoa(int64(i+2))}, nil)
		expect(t, "bob 继续建 key", code, 201, body)
	}
	code, body = bc.do("POST", "/api/me/keys", map[string]any{"name": "第 11 个"}, nil)
	expect(t, "超过 key 数量上限", code, 400, body)

	// 管理列表带额度与本月用量
	code, body = admin.do("GET", "/api/admin/users", nil, nil)
	expect(t, "用户列表", code, 200, body)
	var users struct {
		Users []store.User `json:"users"`
	}
	_ = json.Unmarshal(body, &users)
	var bobListed, aliceListed *store.User
	for i := range users.Users {
		switch users.Users[i].Username {
		case "bob":
			bobListed = &users.Users[i]
		case "alice":
			aliceListed = &users.Users[i]
		}
	}
	if bobListed == nil || bobListed.MonthTokens != 15 {
		t.Fatalf("bob 的本月用量应为 15：%s", body)
	}
	if aliceListed == nil || aliceListed.MonthTokens != 30 {
		t.Fatalf("alice 的本月用量应为 30：%s", body)
	}

	// 员工不能访问管理接口
	code, body = ac.do("GET", "/api/admin/users", nil, nil)
	expect(t, "员工访问管理接口", code, 403, body)

	// 禁用 alice：会话立即失效
	code, body = admin.do("PATCH", "/api/admin/users/"+itoa(alice.ID), map[string]any{"disabled": true}, nil)
	expect(t, "禁用员工", code, 200, body)
	code, body = ac.do("GET", "/api/auth/me", nil, nil)
	expect(t, "被禁用后访问", code, 401, body)

	// 管理员不能禁用自己
	var me store.User
	_, meBody := admin.do("GET", "/api/auth/me", nil, nil)
	_ = json.Unmarshal(meBody, &me)
	code, body = admin.do("PATCH", "/api/admin/users/"+itoa(me.ID), map[string]any{"disabled": true}, nil)
	expect(t, "禁用自己", code, 400, body)

	// 退出后会话失效
	code, _ = admin.do("POST", "/api/auth/logout", nil, nil)
	expect(t, "退出", code, 204, nil)
	code, body = admin.do("GET", "/api/auth/me", nil, nil)
	expect(t, "退出后访问", code, 401, body)
}

func TestLoginLockout(t *testing.T) {
	st := openTestStore(t)
	if _, err := api.Bootstrap(context.Background(), st, "admin", "admin-password-123"); err != nil {
		t.Fatal(err)
	}
	box, _ := secret.NewBox(make([]byte, 32))
	ts := httptest.NewServer(api.New(st, box, api.Options{}, slog.New(slog.NewTextHandler(io.Discard, nil))).Handler())
	defer ts.Close()
	c := newClient(t, ts.URL)
	for range 5 {
		c.do("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "nope-nope-nope"}, nil)
	}
	code, body := c.do("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "admin-password-123"}, nil)
	expect(t, "连续失败后即使密码正确也应锁定", code, 429, body)
}

func itoa(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}
