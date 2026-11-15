package api

import (
	"bytes"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/hikariming/dsh-ee/server/internal/store"
)

// 这组测试用包级 comparePassword 计数器证明：没有本地密码的账号（OIDC / 设备码建号，
// password_hash 为空）在本地登录路径上也会跑一次完整的 bcrypt，而不是让 bcrypt 对空 hash
// 立刻返回——否则响应时间就能区分「账号存在但没有本地密码」与「账号不存在」。

// TestVerifyLocalPasswordNoLocalHashDoesDummyWork 断言空 / 非 bcrypt 的 PasswordHash 一律
// 走 dummyHash 比对一次并返回 false（空密码也不能登录）。
func TestVerifyLocalPasswordNoLocalHashDoesDummyWork(t *testing.T) {
	origCompare := comparePassword
	defer func() { comparePassword = origCompare }()

	var seen [][]byte
	comparePassword = func(hash, password []byte) error {
		seen = append(seen, append([]byte(nil), hash...))
		return origCompare(hash, password)
	}

	// 等时化的前提：dummyHash 是真实 cost 12 的 bcrypt，和有本地密码的账号同量级。
	if cost, err := bcrypt.Cost(dummyHash); err != nil || cost != bcryptCost {
		t.Fatalf("dummyHash 不是 cost %d 的 bcrypt：cost=%d err=%v", bcryptCost, cost, err)
	}

	// 真实的 Go bcrypt 哈希只用来构造「长度 60、前缀不对」的脏数据。
	valid, err := bcrypt.GenerateFromPassword([]byte("dsh-ee-dummy-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("GenerateFromPassword: %v", err)
	}

	hashes := map[string]string{
		"空串（OIDC / 设备码建的号）": "",
		"普通文本":              "not-a-bcrypt-hash",
		"长度不足":              "$2a$12$short",
		"60 字节但非 bcrypt":    strings.Repeat("x", bcryptHashLen),
		"长度差一字节的 bcrypt":    string(valid[:bcryptHashLen-1]),
		"前缀不认识的 bcrypt":     "$1$" + string(valid[4:]),
	}
	for name, hash := range hashes {
		t.Run(name, func(t *testing.T) {
			u := &store.User{Username: "oidc-alice", AuthSource: store.AuthOIDC, PasswordHash: hash}
			for _, password := range []string{"", "whatever-password"} {
				seen = nil
				if verifyLocalPassword(u, password) {
					t.Fatalf("PasswordHash=%q password=%q：必须登录失败", hash, password)
				}
				if len(seen) != 1 {
					t.Fatalf("PasswordHash=%q：应恰好比对一次，实际 %d 次", hash, len(seen))
				}
				if !bytes.Equal(seen[0], dummyHash) {
					t.Fatalf("PasswordHash=%q：应比对 dummyHash（等时化），实际比对了 %q", hash, seen[0])
				}
			}
		})
	}

	// 防御性：authenticate 只在 u != nil 时调用，但 helper 自己也不应 panic。
	seen = nil
	if verifyLocalPassword(nil, "whatever-password") {
		t.Fatal("nil 用户必须返回 false")
	}
	if len(seen) != 1 || !bytes.Equal(seen[0], dummyHash) {
		t.Fatalf("nil 用户也应跑一次 dummyHash 比对，实际 %d 次", len(seen))
	}
}

// TestVerifyLocalPasswordBcryptUser 断言有真实 bcrypt 哈希的账号仍按原语义验证：
// 恰好比对一次用户自己的 hash，正确密码通过、错误密码拒绝。
func TestVerifyLocalPasswordBcryptUser(t *testing.T) {
	origCompare := comparePassword
	defer func() { comparePassword = origCompare }()

	var seen [][]byte
	comparePassword = func(hash, password []byte) error {
		seen = append(seen, append([]byte(nil), hash...))
		return origCompare(hash, password)
	}

	hash, err := HashPassword("correct-horse-battery")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	u := &store.User{Username: "local-bob", AuthSource: store.AuthLocal, PasswordHash: hash}

	if !verifyLocalPassword(u, "correct-horse-battery") {
		t.Fatal("正确密码应通过")
	}
	if len(seen) != 1 {
		t.Fatalf("正确密码应恰好比对一次，实际 %d 次", len(seen))
	}
	if !bytes.Equal(seen[0], []byte(hash)) {
		t.Fatalf("应比对用户自己的 hash，实际比对了 %q", seen[0])
	}

	if verifyLocalPassword(u, "wrong-horse-battery") {
		t.Fatal("错误密码必须返回 false")
	}
	if len(seen) != 2 {
		t.Fatalf("错误密码应再比对一次（共 2 次），实际 %d 次", len(seen))
	}
	if !bytes.Equal(seen[1], []byte(hash)) {
		t.Fatalf("应比对用户自己的 hash，实际比对了 %q", seen[1])
	}
}

// TestIsBcryptHash 只做长度 + 前缀粗筛；不认识的格式宁可当「无本地密码」处理（跑 dummy
// 比对后拒绝），也不能把非 bcrypt 字符串送进 bcrypt 的快速失败路径。
func TestIsBcryptHash(t *testing.T) {
	valid, err := bcrypt.GenerateFromPassword([]byte("dsh-ee-dummy-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("GenerateFromPassword: %v", err)
	}
	cases := []struct {
		name string
		in   []byte
		want bool
	}{
		{"nil", nil, false},
		{"空串", []byte(""), false},
		{"普通文本", []byte("password"), false},
		{"Go 生成的真实 hash（$2a$）", valid, true},
		{"$2b$ 前缀", []byte("$2b$" + string(valid[4:])), true},
		{"$2y$ 前缀", []byte("$2y$" + string(valid[4:])), true},
		{"少一字节", valid[:bcryptHashLen-1], false},
		{"多一字节", append(append([]byte(nil), valid...), 'x'), false},
		{"$1$ 前缀", []byte("$1$" + string(valid[4:])), false},
		{"$2x$ 前缀", []byte("$2x$" + string(valid[4:])), false},
		{"60 字节非 base64", []byte(strings.Repeat("a", bcryptHashLen)), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isBcryptHash(c.in); got != c.want {
				t.Fatalf("isBcryptHash(%s) = %v，期望 %v", strconv.Quote(string(c.in)), got, c.want)
			}
		})
	}
}
