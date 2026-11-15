package api_test

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"

	"github.com/hikariming/dsh-ee/server/internal/api"
	"github.com/hikariming/dsh-ee/server/internal/secret"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// 回归：OIDC / 设备码等渠道建的账号、以及 bot 账号都没有本地密码（password_hash 为空）。
// 这类账号登录必须仍然失败（空密码也不能进），且错误文案与普通密码错误完全一致；等时化
// 本身在 TestAuthRejectBranchesCompareOnce 里用计数器证明，这里只确认 HTTP 语义没变、
// 正常本地账号不受影响。
func TestLoginEmptyPasswordHashRejected(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if ok, err := api.Bootstrap(ctx, st, "admin", "admin-password-123"); err != nil || !ok {
		t.Fatalf("bootstrap: %v %v", ok, err)
	}

	// 模拟 OIDC 首次登录建的号：只有 auth_source，没有本地密码。
	oidcUser := &store.User{Username: "oidc-alice", DisplayName: "OIDC Alice", AuthSource: store.AuthOIDC, AccountType: store.AccountHuman}
	if err := st.CreateUser(ctx, oidcUser); err != nil {
		t.Fatalf("创建 OIDC 账号: %v", err)
	}
	stored, err := st.GetUserByUsername(ctx, "oidc-alice")
	if err != nil {
		t.Fatalf("读取 OIDC 账号: %v", err)
	}
	if stored.PasswordHash != "" {
		t.Fatalf("OIDC 账号不应有本地密码，实际 hash=%q", stored.PasswordHash)
	}

	// bot 账号：建号时不允许设置密码，只能拿 token 用；网页登录必须仍然 401。
	if err := st.CreateUser(ctx, &store.User{Username: "bot-reporter", DisplayName: "监控机器人", AccountType: store.AccountBot}); err != nil {
		t.Fatalf("创建 bot 账号: %v", err)
	}

	// 对照：有真实 bcrypt 哈希的本地账号。
	hash, err := api.HashPassword("alice-password-1")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if err := st.CreateUser(ctx, &store.User{
		Username: "local-alice", DisplayName: "Local Alice",
		AuthSource: store.AuthLocal, AccountType: store.AccountHuman, PasswordHash: hash,
	}); err != nil {
		t.Fatalf("创建本地账号: %v", err)
	}

	key := make([]byte, 32)
	_, _ = rand.Read(key)
	box, err := secret.NewBox(key)
	if err != nil {
		t.Fatalf("NewBox: %v", err)
	}
	srv := api.New(st, box, api.Options{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	c := newClient(t, ts.URL)

	// 空密码、任意密码都必须 401（不能因 hash 为空反而放行）。
	for _, password := range []string{"", "whatever-password"} {
		code, body := c.do("POST", "/api/auth/login", map[string]string{"username": "oidc-alice", "password": password}, nil)
		expect(t, "无本地密码账号登录", code, 401, body)
	}

	// bot 账号：同样 401，且错误文案与普通密码错误一致（不泄露账号存在/类型）。
	for _, password := range []string{"", "whatever-password"} {
		code, body := c.do("POST", "/api/auth/login", map[string]string{"username": "bot-reporter", "password": password}, nil)
		expect(t, "bot 账号登录", code, 401, body)
		var got struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(body, &got); err != nil || got.Error != "用户名或密码错误" {
			t.Fatalf("bot 账号登录错误文案变了：%s（解析错误 %v）", body, err)
		}
	}

	// 本地账号：错误密码 401、正确密码 200，确认等时化辅助没有破坏正常路径。
	code, body := c.do("POST", "/api/auth/login", map[string]string{"username": "local-alice", "password": "wrong-password-1"}, nil)
	expect(t, "本地账号错误密码", code, 401, body)
	code, body = c.do("POST", "/api/auth/login", map[string]string{"username": "local-alice", "password": "alice-password-1"}, nil)
	expect(t, "本地账号正确密码", code, 200, body)
}
