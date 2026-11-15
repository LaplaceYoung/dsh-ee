package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/hikariming/dsh-ee/server/internal/store"
)

// openAuthenticateTestStore 为包内测试开一个临时库，约定与 api_test.openTestStore 相同
// （需要 DSH_EE_TEST_DSN，跑完 DROP DATABASE）。包内测试不能复用 api_test 包的 helper。
func openAuthenticateTestStore(t *testing.T) *store.Store {
	t.Helper()
	base := os.Getenv("DSH_EE_TEST_DSN")
	if base == "" {
		t.Skip("未设置 DSH_EE_TEST_DSN，跳过数据库集成测试")
	}
	suffix := make([]byte, 4)
	_, _ = rand.Read(suffix)
	dbName := "dsh_ee_test_" + hex.EncodeToString(suffix)
	ctx := context.Background()
	st, err := store.Open(ctx, strings.Replace(base, ")/", ")/"+dbName, 1))
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

// TestAuthRejectBranchesCompareOnce 用 comparePassword 计数器证明 authenticate 的每个
// 「本地可判定」拒绝分支都恰好跑了一次 bcrypt，不会因提前返回而比正常路径快：
//   - bot 账号（存在、按建号约定没有本地密码）：dummyHash 一次
//   - 用户不存在且 LDAP 未启用：dummyHash 一次
//   - 本地账号密码错误 / 被禁用：用户自己的 hash 一次（Disabled 在 bcrypt 之后判断）
//   - 本地账号密码正确：用户自己的 hash 一次
//
// 不在这条测试覆盖范围的只有 LDAP 网络路径（见 authenticate 的注释）。
func TestAuthRejectBranchesCompareOnce(t *testing.T) {
	st := openAuthenticateTestStore(t)
	ctx := context.Background()
	if ok, err := Bootstrap(ctx, st, "admin", "admin-password-123"); err != nil || !ok {
		t.Fatalf("bootstrap: %v %v", ok, err)
	}

	// bot 账号：建号接口不允许设置密码，所以 PasswordHash 为空。
	if err := st.CreateUser(ctx, &store.User{Username: "bot-reporter", DisplayName: "监控机器人", AccountType: store.AccountBot}); err != nil {
		t.Fatalf("创建 bot 账号: %v", err)
	}
	bot, err := st.GetUserByUsername(ctx, "bot-reporter")
	if err != nil {
		t.Fatalf("读取 bot 账号: %v", err)
	}
	if bot.AccountType != store.AccountBot || bot.PasswordHash != "" {
		t.Fatalf("bot 测试账号构造不对：account_type=%q password_hash=%q", bot.AccountType, bot.PasswordHash)
	}

	localHash, err := HashPassword("local-alice-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if err := st.CreateUser(ctx, &store.User{Username: "local-alice", DisplayName: "Local Alice", AccountType: store.AccountHuman, PasswordHash: localHash}); err != nil {
		t.Fatalf("创建本地账号: %v", err)
	}
	disabledHash, err := HashPassword("disabled-bob-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if err := st.CreateUser(ctx, &store.User{Username: "disabled-bob", DisplayName: "Disabled Bob", AccountType: store.AccountHuman, PasswordHash: disabledHash, Disabled: true}); err != nil {
		t.Fatalf("创建被禁用账号: %v", err)
	}

	// 只测本地判定分支，不接 LDAP / 后台任务，裸 Server 足够。
	s := &Server{store: st, log: slog.New(slog.NewTextHandler(io.Discard, nil))}

	origCompare := comparePassword
	var seen [][]byte
	comparePassword = func(hash, password []byte) error {
		seen = append(seen, append([]byte(nil), hash...))
		return origCompare(hash, password)
	}
	defer func() { comparePassword = origCompare }()

	cases := []struct {
		name     string
		username string
		password string
		wantErr  error
		wantHash []byte
	}{
		{"bot 账号（存在，无本地密码）", "bot-reporter", "whatever-password", errBadCredentials, dummyHash},
		{"用户不存在且 LDAP 未启用", "ghost-user", "whatever-password", errBadCredentials, dummyHash},
		{"本地账号 + 错误密码", "local-alice", "wrong-password", errBadCredentials, []byte(localHash)},
		{"本地账号 + 正确密码", "local-alice", "local-alice-password", nil, []byte(localHash)},
		{"本地账号被禁用（密码正确，bcrypt 之后才拒绝）", "disabled-bob", "disabled-bob-password", errBadCredentials, []byte(disabledHash)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			seen = nil
			u, err := s.authenticate(ctx, c.username, c.password)
			if c.wantErr != nil {
				if !errors.Is(err, c.wantErr) {
					t.Fatalf("错误 = %v，期望 %v", err, c.wantErr)
				}
				if u != nil {
					t.Fatalf("拒绝时不应返回用户，实际 %+v", u)
				}
			} else if err != nil {
				t.Fatalf("应登录成功，实际 %v", err)
			}
			if len(seen) != 1 {
				t.Fatalf("应恰好比对一次 bcrypt，实际 %d 次", len(seen))
			}
			if !bytes.Equal(seen[0], c.wantHash) {
				t.Fatalf("比对的 hash 不对：%q", seen[0])
			}
		})
	}
}
