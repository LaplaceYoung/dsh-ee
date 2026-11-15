package store

import (
	"io/fs"
	"strings"
	"testing"
)

// 迁移文件是按行尾分号切分后逐条执行的（Migrate 里用 splitStatements）；
// 这里不连数据库，先保证每个文件都能被完整解析成可执行语句，
// 并且 token client_kind 这次改动落在预期的表和列上。
func TestMigrationsParse(t *testing.T) {
	files, err := fs.Glob(migrationFS, "migrations/*.sql")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("没有迁移文件")
	}
	for _, f := range files {
		body, err := migrationFS.ReadFile(f)
		if err != nil {
			t.Fatalf("读 %s: %v", f, err)
		}
		stmts := splitStatements(string(body))
		if len(stmts) == 0 {
			t.Fatalf("%s 解析出 0 条语句", f)
		}
		for _, stmt := range stmts {
			trimmed := strings.TrimSpace(stmt)
			if trimmed == "" {
				t.Fatalf("%s 解析出了空语句", f)
			}
			// 切分器不处理字符串里的分号；迁移里只允许注释和语句，不允许残留的注释行混进语句
			for _, line := range strings.Split(stmt, "\n") {
				if strings.HasPrefix(strings.TrimSpace(line), "--") {
					t.Fatalf("%s 有注释混进了语句：%q", f, stmt)
				}
			}
		}
	}
}

// 0032：令牌客户端类型与调用/确认记录里的令牌归属。
func TestTokenClientKindMigration(t *testing.T) {
	body, err := migrationFS.ReadFile("migrations/0032_token_client_kind.sql")
	if err != nil {
		t.Fatalf("读 0032 迁移: %v", err)
	}
	stmts := splitStatements(string(body))
	if len(stmts) == 0 {
		t.Fatal("0032 迁移没有可执行语句")
	}
	joined := strings.Join(stmts, "\n")
	for _, want := range []string{
		"ALTER TABLE access_tokens",
		"client_kind VARCHAR(16) NOT NULL DEFAULT 'dsh'",
		"ALTER TABLE connection_calls",
		"token_id BIGINT NULL",
		"ALTER TABLE write_confirmations",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("0032 迁移缺少 %q：\n%s", want, joined)
		}
	}
}

// 0036：多实例共享的一次性状态表（web link / 接入码 / OIDC state）。
func TestEphemeralStatesMigration(t *testing.T) {
	body, err := migrationFS.ReadFile("migrations/0036_ephemeral_states.sql")
	if err != nil {
		t.Fatalf("读 0036 迁移: %v", err)
	}
	stmts := splitStatements(string(body))
	if len(stmts) == 0 {
		t.Fatal("0036 迁移没有可执行语句")
	}
	joined := strings.Join(stmts, "\n")
	for _, want := range []string{
		"CREATE TABLE ephemeral_states",
		"kind        VARCHAR(32)  NOT NULL",
		"state_hash  CHAR(64)     NOT NULL",
		"owner_key   VARCHAR(191) NULL",
		"payload     JSON         NOT NULL",
		"expires_at  DATETIME(3)  NOT NULL",
		"created_at  DATETIME(3)  NOT NULL",
		"consumed_at DATETIME(3)  NULL",
		"PRIMARY KEY (kind, state_hash)",
		"UNIQUE KEY uk_ephemeral_states_owner (kind, owner_key)",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("0036 迁移缺少 %q：\n%s", want, joined)
		}
	}
}

// 0036：API 客户端注册（client_credentials）与审计来源加宽。
func TestAPIClientsMigration(t *testing.T) {
	body, err := migrationFS.ReadFile("migrations/0036_api_clients.sql")
	if err != nil {
		t.Fatalf("读 0036 迁移: %v", err)
	}
	joined := strings.Join(splitStatements(string(body)), "\n")
	for _, want := range []string{
		"CREATE TABLE api_clients",
		"client_id     VARCHAR(64)   NOT NULL",
		"secret_hash   CHAR(64)      NOT NULL",
		"owner_user_id BIGINT        NOT NULL",
		"scopes        VARCHAR(1024) NOT NULL",
		"client_kind   VARCHAR(16)   NOT NULL DEFAULT 'third_party'",
		"created_by    BIGINT        NOT NULL DEFAULT 0",
		"disabled_at   DATETIME(3)   NULL",
		"last_used_at  DATETIME(3)   NULL",
		"UNIQUE KEY uk_api_clients_client_id (client_id)",
		"ALTER TABLE audit_logs MODIFY COLUMN via VARCHAR(32)",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("0036 迁移缺少 %q：\n%s", want, joined)
		}
	}
}

// 0035：刷新令牌数据模型（docs/token-lifecycle.md §3）。
func TestTokenRefreshMigration(t *testing.T) {
	body, err := migrationFS.ReadFile("migrations/0035_token_refresh_tokens.sql")
	if err != nil {
		t.Fatalf("读 0035 迁移: %v", err)
	}
	stmts := splitStatements(string(body))
	if len(stmts) == 0 {
		t.Fatal("0035 迁移没有可执行语句")
	}
	joined := strings.Join(stmts, "\n")
	for _, want := range []string{
		"CREATE TABLE token_refresh_tokens",
		"family_id          CHAR(32)",
		"access_token_id    BIGINT",
		"user_id            BIGINT",
		"client_kind        VARCHAR(16)",
		"secret_hash        CHAR(64)",
		"issued_at          DATETIME(3)",
		"expires_at         DATETIME(3)",
		"rotated_at         DATETIME(3)",
		"revoked_at         DATETIME(3)",
		"reuse_detected_at  DATETIME(3)",
		"created_ip         VARCHAR(64)",
		"UNIQUE KEY uk_token_refresh_hash",
		"ALTER TABLE access_tokens",
		"family_id CHAR(32) NULL",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("0035 迁移缺少 %q：\n%s", want, joined)
		}
	}
}

// 0037：公开协议面（/v1 与 /mcp）的每令牌限流改为数据库共享固定窗口。
func TestSharedRateLimitsMigration(t *testing.T) {
	body, err := migrationFS.ReadFile("migrations/0037_shared_rate_limits.sql")
	if err != nil {
		t.Fatalf("读 0037 迁移: %v", err)
	}
	stmts := splitStatements(string(body))
	if len(stmts) == 0 {
		t.Fatal("0037 迁移没有可执行语句")
	}
	// 迁移里为了对齐列写了很多空格，断言前先压成单个空格。
	joined := strings.Join(strings.Fields(strings.Join(stmts, "\n")), " ")
	for _, want := range []string{
		"CREATE TABLE shared_rate_limits",
		"rate_key VARCHAR(191) NOT NULL",
		"window_start DATETIME(3) NOT NULL",
		"count INT NOT NULL DEFAULT 0",
		"PRIMARY KEY (rate_key, window_start)",
		"KEY idx_shared_rate_limits_window (window_start)",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("0037 迁移缺少 %q：\n%s", want, joined)
		}
	}
}
