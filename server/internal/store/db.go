// Package store 是唯一写 SQL 的地方。只用 OceanBase（MySQL 模式）与 TiDB 都支持的语法；
// 以后接达梦、人大金仓等非 MySQL 方言时，只需要改这一层。
package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-sql-driver/mysql"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

var ErrNotFound = errors.New("not found")

type Store struct {
	db *sql.DB
	// 超级管理员被关掉了（docs/access-control.md §3.1）：UserAccess 里 Super 永远是 false
	superOff atomic.Bool
}

// SetSuperDisabled 打开或关掉「超级管理员」这个内置角色。
func (s *Store) SetSuperDisabled(off bool) { s.superOff.Store(off) }

// SuperDisabled 报告超级管理员是不是关掉了。
func (s *Store) SuperDisabled() bool { return s.superOff.Load() }

// Open 连接数据库；库不存在时（MySQL 错误 1049）尝试创建，方便本地开发。
func Open(ctx context.Context, dsn string) (*Store, error) {
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return nil, fmt.Errorf("解析 DSN: %w", err)
	}
	// 统一按 UTC 读写时间。
	cfg.ParseTime = true
	cfg.Loc = time.UTC
	// 字符集用驱动默认的 utf8mb4（utf8mb4_general_ci）。

	db, err := connect(ctx, cfg)
	var me *mysql.MySQLError
	if errors.As(err, &me) && me.Number == 1049 && cfg.DBName != "" {
		if err := createDatabase(ctx, cfg); err != nil {
			return nil, err
		}
		db, err = connect(ctx, cfg)
	}
	if err != nil {
		return nil, err
	}
	return &Store{db: db}, nil
}

func connect(ctx context.Context, cfg *mysql.Config) (*sql.DB, error) {
	conn, err := mysql.NewConnector(cfg)
	if err != nil {
		return nil, err
	}
	db := sql.OpenDB(conn)
	db.SetMaxOpenConns(32)
	db.SetMaxIdleConns(8)
	db.SetConnMaxLifetime(30 * time.Minute)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func createDatabase(ctx context.Context, cfg *mysql.Config) error {
	if strings.ContainsAny(cfg.DBName, "`\\") {
		return fmt.Errorf("非法的库名 %q", cfg.DBName)
	}
	admin := cfg.Clone()
	admin.DBName = ""
	db, err := connect(ctx, admin)
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.ExecContext(ctx, "CREATE DATABASE IF NOT EXISTS `"+cfg.DBName+"` DEFAULT CHARACTER SET utf8mb4")
	if err != nil {
		return fmt.Errorf("创建数据库 %s: %w", cfg.DBName, err)
	}
	return nil
}

func (s *Store) Close() error { return s.db.Close() }

// DB 仅供测试清理使用。
func (s *Store) DB() *sql.DB { return s.db }

// Migrate 按文件名顺序执行未执行过的迁移。DDL 在这两种库里都不在事务内，
// 所以每个文件执行完才记版本；中途失败需要人工检查后重跑。
func (s *Store) Migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version    VARCHAR(64) NOT NULL,
		applied_at DATETIME(3) NOT NULL,
		PRIMARY KEY (version)
	)`); err != nil {
		return fmt.Errorf("创建 schema_migrations: %w", err)
	}

	applied := map[string]bool{}
	rows, err := s.db.QueryContext(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return err
	}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return err
		}
		applied[v] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		// 读不全就会把已执行的迁移当成未执行，下次启动重跑非幂等 DDL。
		return fmt.Errorf("读取已执行迁移: %w", err)
	}

	files, err := fs.Glob(migrationFS, "migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(files)
	for _, f := range files {
		version := strings.TrimSuffix(strings.TrimPrefix(f, "migrations/"), ".sql")
		if applied[version] {
			continue
		}
		body, err := migrationFS.ReadFile(f)
		if err != nil {
			return err
		}
		for _, stmt := range splitStatements(string(body)) {
			if _, err := s.db.ExecContext(ctx, stmt); err != nil {
				return fmt.Errorf("迁移 %s: %w\n%s", version, err, stmt)
			}
		}
		if _, err := s.db.ExecContext(ctx, "INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)", version, now()); err != nil {
			return err
		}
	}
	return nil
}

// MigrationState 是数据库迁移的当前状态，供就绪检查（GET /readyz）判断迁移是否已到最新。
type MigrationState struct {
	// Latest 是当前二进制内置迁移里按文件名排序的最后一条版本。
	Latest string
	// Pending 是还没在 schema_migrations 里登记的迁移版本，按文件名顺序。
	Pending []string
}

// MigrationStatus 只读查询迁移状态：比对 schema_migrations 与内置迁移文件的差集，
// 不建表、不执行迁移、不改数据。还没初始化（schema_migrations 不存在）时返回错误，
// 调用方据此判断未就绪。
func (s *Store) MigrationStatus(ctx context.Context) (MigrationState, error) {
	files, err := fs.Glob(migrationFS, "migrations/*.sql")
	if err != nil {
		return MigrationState{}, err
	}
	sort.Strings(files)
	known := make([]string, 0, len(files))
	var state MigrationState
	for _, f := range files {
		version := strings.TrimSuffix(strings.TrimPrefix(f, "migrations/"), ".sql")
		known = append(known, version)
		state.Latest = version
	}

	rows, err := s.db.QueryContext(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return MigrationState{}, err
	}
	defer rows.Close()
	applied := make(map[string]bool, len(known))
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return MigrationState{}, err
		}
		applied[v] = true
	}
	if err := rows.Err(); err != nil {
		return MigrationState{}, err
	}
	for _, version := range known {
		if !applied[version] {
			state.Pending = append(state.Pending, version)
		}
	}
	return state, nil
}

// splitStatements 按行尾分号切分；迁移文件里不写含分号的字符串。
func splitStatements(sqlText string) []string {
	var out []string
	var cur strings.Builder
	for _, line := range strings.Split(sqlText, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			continue
		}
		cur.WriteString(line)
		cur.WriteString("\n")
		if strings.HasSuffix(trimmed, ";") {
			stmt := strings.TrimSuffix(strings.TrimSpace(cur.String()), ";")
			out = append(out, stmt)
			cur.Reset()
		}
	}
	if rest := strings.TrimSpace(cur.String()); rest != "" {
		out = append(out, rest)
	}
	return out
}

func now() time.Time { return time.Now().UTC().Truncate(time.Millisecond) }
