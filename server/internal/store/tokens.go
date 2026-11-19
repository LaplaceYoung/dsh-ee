package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"
)

// 令牌的客户端类型：dsh 是 DSH 的接入插件/设备登录，third_party 是其他 Harness。
// 写确认只对 dsh 令牌走 DSH 审批卡的快捷语义，third_party 始终人工确认（api/write_confirm.go）。
const (
	ClientKindDSH        = "dsh"
	ClientKindThirdParty = "third_party"
)

// NormalizeClientKind 把空值与未知值收敛到 dsh：旧令牌、旧代码路径按原有 DSH 行为处理。
func NormalizeClientKind(kind string) string {
	if kind == ClientKindThirdParty {
		return ClientKindThirdParty
	}
	return ClientKindDSH
}

type AccessToken struct {
	ID          int64  `json:"id"`
	UserID      int64  `json:"user_id"`
	Username    string `json:"username"`
	Department  string `json:"department"`
	AccountType string `json:"account_type"`
	Name        string `json:"name"`
	Prefix      string `json:"prefix"`
	TokenHash   string `json:"-"`
	Source      string `json:"source"`
	ClientKind  string `json:"client_kind"`
	// 令牌族（refresh 轮换出来的代际共用）；未开启 refresh 的令牌为 nil
	FamilyID          *string    `json:"-"`
	Scopes            []string   `json:"scopes"`
	MonthlyTokenLimit *int64     `json:"monthly_token_limit"`
	AllowedModels     []string   `json:"allowed_models"`
	ExpiresAt         *time.Time `json:"expires_at"`
	SlidingDays       *int       `json:"sliding_days"`
	AdminUntil        *time.Time `json:"admin_until"`
	ClientName        string     `json:"client_name"`
	ClientOS          string     `json:"client_os"`
	ClientVersion     string     `json:"client_version"`
	LastUsedAt        *time.Time `json:"last_used_at"`
	LastClient        string     `json:"last_client"`
	LastHost          string     `json:"last_host"`
	LastIP            string     `json:"last_ip"`
	RevokedAt         *time.Time `json:"revoked_at"`
	CreatedBy         int64      `json:"created_by"`
	CreatedAt         time.Time  `json:"created_at"`
}

const accessTokenColumns = `t.id, t.user_id, t.name, t.prefix, t.token_hash, t.source, t.client_kind, t.family_id, t.scopes, t.monthly_token_limit,
	t.allowed_models, t.expires_at, t.sliding_days, t.admin_until, t.client_name, t.client_os, t.client_version,
	t.last_used_at, t.last_client, t.last_host, t.last_ip, t.revoked_at, t.created_by, t.created_at`

func scanAccessToken(row interface{ Scan(...any) error }, joined ...any) (*AccessToken, error) {
	var t AccessToken
	var limit sql.NullInt64
	var models, scopes sql.NullString
	var family sql.NullString
	var expires, adminUntil, used, revoked sql.NullTime
	var sliding sql.NullInt32
	err := row.Scan(append([]any{
		&t.ID, &t.UserID, &t.Name, &t.Prefix, &t.TokenHash, &t.Source, &t.ClientKind, &family, &scopes, &limit,
		&models, &expires, &sliding, &adminUntil, &t.ClientName, &t.ClientOS, &t.ClientVersion,
		&used, &t.LastClient, &t.LastHost, &t.LastIP, &revoked, &t.CreatedBy, &t.CreatedAt,
	}, joined...)...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	// 旧数据/旧迁移里没有 kind：按 DSH 兼容处理
	t.ClientKind = NormalizeClientKind(t.ClientKind)
	if family.Valid && family.String != "" {
		t.FamilyID = &family.String
	}
	if len(joined) == 3 {
		t.Username = *joined[0].(*string)
		t.Department = *joined[1].(*string)
		t.AccountType = *joined[2].(*string)
	}
	if limit.Valid {
		t.MonthlyTokenLimit = &limit.Int64
	}
	if models.Valid && models.String != "" {
		if err := json.Unmarshal([]byte(models.String), &t.AllowedModels); err != nil {
			return nil, err
		}
	}
	if scopes.Valid && scopes.String != "" {
		t.Scopes = strings.Split(scopes.String, ",")
	}
	if expires.Valid {
		t.ExpiresAt = &expires.Time
	}
	if sliding.Valid {
		v := int(sliding.Int32)
		t.SlidingDays = &v
	}
	if adminUntil.Valid {
		t.AdminUntil = &adminUntil.Time
	}
	if used.Valid {
		t.LastUsedAt = &used.Time
	}
	if revoked.Valid {
		t.RevokedAt = &revoked.Time
	}
	return &t, nil
}

type execContext interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func createAccessToken(ctx context.Context, db execContext, t *AccessToken, hash string) error {
	var models any
	if len(t.AllowedModels) > 0 {
		b, _ := json.Marshal(t.AllowedModels)
		models = string(b)
	}
	t.TokenHash = hash
	t.ClientKind = NormalizeClientKind(t.ClientKind)
	t.CreatedAt = now()
	res, err := db.ExecContext(ctx, `INSERT INTO access_tokens (user_id, name, prefix, token_hash, source, client_kind, family_id, scopes, monthly_token_limit,
		allowed_models, expires_at, sliding_days, admin_until, client_name, client_os, client_version,
		last_used_at, last_ip, revoked_at, created_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.UserID, t.Name, t.Prefix, hash, t.Source, t.ClientKind, t.FamilyID, strings.Join(t.Scopes, ","), t.MonthlyTokenLimit,
		models, t.ExpiresAt, t.SlidingDays, t.AdminUntil, t.ClientName, t.ClientOS, t.ClientVersion,
		t.LastUsedAt, t.LastIP, t.RevokedAt, t.CreatedBy, t.CreatedAt)
	if err != nil {
		return err
	}
	t.ID, err = res.LastInsertId()
	return err
}

func (s *Store) CreateAccessToken(ctx context.Context, t *AccessToken, hash string) error {
	return createAccessToken(ctx, s.db, t, hash)
}

func (s *Store) ActiveAccessToken(ctx context.Context, hash string) (*AccessToken, error) {
	return scanAccessToken(s.db.QueryRowContext(ctx,
		`SELECT `+accessTokenColumns+`, u.username, u.department, u.account_type
		 FROM access_tokens t JOIN users u ON u.id = t.user_id
		 WHERE t.token_hash = ? AND t.revoked_at IS NULL AND (t.expires_at IS NULL OR t.expires_at > ?) AND u.disabled = 0`,
		hash, now()), new(string), new(string), new(string))
}

func (s *Store) AccessTokenByHash(ctx context.Context, hash string) (*AccessToken, error) {
	return scanAccessToken(s.db.QueryRowContext(ctx,
		`SELECT `+accessTokenColumns+`, u.username, u.department, u.account_type
		 FROM access_tokens t JOIN users u ON u.id = t.user_id WHERE t.token_hash = ?`, hash), new(string), new(string), new(string))
}

// AccessTokenByID 按 id 取令牌（含已吊销、已过期），带出持有者的用户名、部门与账号类型。
func (s *Store) AccessTokenByID(ctx context.Context, id int64) (*AccessToken, error) {
	return scanAccessToken(s.db.QueryRowContext(ctx,
		`SELECT `+accessTokenColumns+`, u.username, u.department, u.account_type
		 FROM access_tokens t JOIN users u ON u.id = t.user_id WHERE t.id = ?`, id), new(string), new(string), new(string))
}

func (s *Store) TouchAccessToken(ctx context.Context, id int64, ip string, clientHost ...string) error {
	t := now()
	client, host := "", ""
	if len(clientHost) > 0 {
		client = clientHost[0]
	}
	if len(clientHost) > 1 {
		host = clientHost[1]
	}
	cutoff := t.Add(-time.Minute)
	// 滑动续期每天最多写一次；活跃时间、IP 与插件宿主共用每分钟一次的写入窗口。
	_, err := s.db.ExecContext(ctx, `UPDATE access_tokens SET
		expires_at = IF(sliding_days IS NOT NULL AND (expires_at IS NULL OR expires_at < DATE_SUB(DATE_ADD(?, INTERVAL sliding_days DAY), INTERVAL 1 DAY)),
			DATE_ADD(?, INTERVAL sliding_days DAY), expires_at),
		last_ip = IF(? <> '' AND (last_ip = '' OR last_used_at IS NULL OR last_used_at < ?), ?, last_ip),
		last_client = IF(? <> '' AND (last_used_at IS NULL OR last_used_at < ?), ?, last_client),
		last_host = IF(? <> '' AND (last_used_at IS NULL OR last_used_at < ?), ?, last_host),
		last_used_at = IF(last_used_at IS NULL OR last_used_at < ?, ?, last_used_at)
		WHERE id = ? AND revoked_at IS NULL`, t, t, ip, cutoff, ip, client, cutoff, client, host, cutoff, host, cutoff, t, id)
	return err
}

func (s *Store) ListActiveDshClients(ctx context.Context, userID int64, since time.Time) ([]*AccessToken, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+accessTokenColumns+` FROM access_tokens t
		WHERE t.user_id = ? AND t.revoked_at IS NULL AND t.last_used_at >= ? AND t.last_client LIKE 'dsh-connect/%'
		ORDER BY t.last_used_at DESC LIMIT 50`, userID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*AccessToken
	for rows.Next() {
		item, err := scanAccessToken(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) ListUserTokens(ctx context.Context, userID int64) ([]*AccessToken, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+accessTokenColumns+`, u.username, u.department, u.account_type
		 FROM access_tokens t JOIN users u ON u.id = t.user_id WHERE t.user_id = ? ORDER BY t.id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAccessTokens(rows)
}

type TokenFilter struct {
	UserID     *int64
	Department *string
	Source     *string
	ClientName *string
	Revoked    *bool
}

func (s *Store) ListTokens(ctx context.Context, f TokenFilter) ([]*AccessToken, error) {
	q := `SELECT ` + accessTokenColumns + `, u.username, u.department, u.account_type
		FROM access_tokens t JOIN users u ON u.id = t.user_id WHERE 1 = 1`
	var args []any
	if f.UserID != nil {
		q += " AND t.user_id = ?"
		args = append(args, *f.UserID)
	}
	if f.Department != nil {
		q += " AND u.department = ?"
		args = append(args, *f.Department)
	}
	if f.Source != nil {
		q += " AND t.source = ?"
		args = append(args, *f.Source)
	}
	if f.ClientName != nil {
		q += " AND t.client_name = ?"
		args = append(args, *f.ClientName)
	}
	if f.Revoked != nil {
		if *f.Revoked {
			q += " AND t.revoked_at IS NOT NULL"
		} else {
			q += " AND t.revoked_at IS NULL"
		}
	}
	rows, err := s.db.QueryContext(ctx, q+" ORDER BY t.id DESC", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAccessTokens(rows)
}

func scanAccessTokens(rows *sql.Rows) ([]*AccessToken, error) {
	var out []*AccessToken
	for rows.Next() {
		t, err := scanAccessToken(rows, new(string), new(string), new(string))
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// RevokeToken 吊销一枚 access token。带令牌族（refresh 开启时签发）的令牌会连带吊销
// 整族的 refresh token 与其它代际 access token，避免「吊销了 access 还能用 refresh 续期」。
func (s *Store) RevokeToken(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var family sql.NullString
	err = tx.QueryRowContext(ctx, "SELECT family_id FROM access_tokens WHERE id = ? FOR UPDATE", id).Scan(&family)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	t := now()
	res, err := tx.ExecContext(ctx, "UPDATE access_tokens SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL", t, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	if family.Valid && family.String != "" {
		if _, _, err := revokeRefreshFamilyTx(ctx, tx, family.String, t); err != nil {
			return err
		}
	}
	return tx.Commit()
}

type TokenRevokeFilter struct {
	UserID     *int64
	ClientName *string
	Department *string
	// BotOnly 只吊销机器人账号的令牌（部门令牌管理员只能管本部门的机器人）
	BotOnly bool
}

// Empty 表示没有任何筛选条件；调用方必须拒绝，否则会吊销全部令牌。
func (f TokenRevokeFilter) Empty() bool {
	return f.UserID == nil && f.ClientName == nil && f.Department == nil
}

// tokenRevokeConditions 把 TokenRevokeFilter 拼成 WHERE 片段（每段都以 AND 开头），
// 批量吊销与「先查出要波及的令牌族」共用，避免两处条件漂移。
func tokenRevokeConditions(f TokenRevokeFilter) (string, []any) {
	var q string
	var args []any
	if f.UserID != nil {
		q += " AND t.user_id = ?"
		args = append(args, *f.UserID)
	}
	if f.ClientName != nil {
		q += " AND t.client_name = ?"
		args = append(args, *f.ClientName)
	}
	if f.Department != nil {
		q += " AND u.department = ?"
		args = append(args, *f.Department)
	}
	if f.BotOnly {
		q += " AND u.account_type = ?"
		args = append(args, AccountBot)
	}
	return q, args
}

func (s *Store) RevokeTokensBy(ctx context.Context, f TokenRevokeFilter) (int64, error) {
	if f.Empty() {
		return 0, errors.New("吊销令牌必须带筛选条件")
	}
	where, filterArgs := tokenRevokeConditions(f)
	q := "UPDATE access_tokens t JOIN users u ON u.id = t.user_id SET t.revoked_at = ? WHERE t.revoked_at IS NULL" + where
	args := append([]any{now()}, filterArgs...)
	// 先记下这次会波及的令牌族：批量吊销后整族的 refresh token 也要一起作废。
	families, err := s.tokenFamiliesBy(ctx, f)
	if err != nil {
		return 0, err
	}
	res, err := s.db.ExecContext(ctx, q, args...)
	if err != nil {
		return 0, err
	}
	if len(families) > 0 {
		t := now()
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(families)), ",")
		familyArgs := make([]any, 0, len(families)+2)
		familyArgs = append(familyArgs, t)
		for _, family := range families {
			familyArgs = append(familyArgs, family)
		}
		// 族里的其它代际 access token 与全部 refresh token 一起吊销
		if _, err := s.db.ExecContext(ctx,
			"UPDATE access_tokens SET revoked_at = ? WHERE revoked_at IS NULL AND family_id IN ("+placeholders+")", familyArgs...); err != nil {
			return 0, err
		}
		if _, err := s.db.ExecContext(ctx,
			"UPDATE token_refresh_tokens SET revoked_at = ? WHERE revoked_at IS NULL AND family_id IN ("+placeholders+")", familyArgs...); err != nil {
			return 0, err
		}
	}
	return res.RowsAffected()
}

// tokenFamiliesBy 返回符合吊销条件的令牌当前所属的族 id（去重）。
func (s *Store) tokenFamiliesBy(ctx context.Context, f TokenRevokeFilter) ([]string, error) {
	where, args := tokenRevokeConditions(f)
	rows, err := s.db.QueryContext(ctx,
		"SELECT DISTINCT t.family_id FROM access_tokens t JOIN users u ON u.id = t.user_id WHERE t.family_id IS NOT NULL"+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var family string
		if err := rows.Scan(&family); err != nil {
			return nil, err
		}
		out = append(out, family)
	}
	return out, rows.Err()
}

// TokenMonthUsage 是一把令牌本月用掉的 token。开了 refresh token 的令牌每次续期都换一个新的 access
// token id，额度管的是这把令牌，所以按整族（family_id 相同的各代）一起算；没有族的令牌只算它自己。
func (s *Store) TokenMonthUsage(ctx context.Context, id int64) (int64, error) {
	var family sql.NullString
	if err := s.db.QueryRowContext(ctx, "SELECT family_id FROM access_tokens WHERE id = ?", id).Scan(&family); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	var n int64
	var err error
	if family.Valid && family.String != "" {
		err = s.db.QueryRowContext(ctx,
			`SELECT COALESCE(SUM(total_tokens), 0) FROM usage_logs
			 WHERE created_at >= ? AND token_id IN (SELECT id FROM access_tokens WHERE family_id = ?)`,
			MonthStart(now()), family.String).Scan(&n)
	} else {
		err = s.db.QueryRowContext(ctx,
			"SELECT COALESCE(SUM(total_tokens), 0) FROM usage_logs WHERE token_id = ? AND created_at >= ?",
			id, MonthStart(now())).Scan(&n)
	}
	return n, err
}

// ---- 管理模式 ----

// ElevateToken 开启管理模式：补上 admin 范围（若还没有），admin_until 设为到期时间。
func (s *Store) ElevateToken(ctx context.Context, id int64, until time.Time) error {
	t, err := s.AccessTokenByID(ctx, id)
	if err != nil {
		return err
	}
	scopes := t.Scopes
	if !slices.Contains(scopes, "admin") {
		scopes = append(append([]string{}, scopes...), "admin")
	}
	res, err := s.db.ExecContext(ctx, "UPDATE access_tokens SET scopes = ?, admin_until = ? WHERE id = ? AND revoked_at IS NULL",
		strings.Join(scopes, ","), until.UTC(), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// EndElevation 提前结束管理模式。
func (s *Store) EndElevation(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, "UPDATE access_tokens SET admin_until = NULL WHERE id = ?", id)
	return err
}

// NextElevationExpiry 是还在管理模式的令牌里最早的到期时间；没有则返回 nil。
func (s *Store) NextElevationExpiry(ctx context.Context) (*time.Time, error) {
	var t sql.NullTime
	err := s.db.QueryRowContext(ctx, "SELECT MIN(admin_until) FROM access_tokens WHERE admin_until IS NOT NULL AND revoked_at IS NULL").Scan(&t)
	if err != nil || !t.Valid {
		return nil, err
	}
	return &t.Time, nil
}

// ExpireElevations 清掉已经到期的管理模式并返回这些令牌（调用方写审计、通知 DSH）。
func (s *Store) ExpireElevations(ctx context.Context) ([]*AccessToken, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+accessTokenColumns+" FROM access_tokens t WHERE t.admin_until IS NOT NULL AND t.admin_until <= ? AND t.revoked_at IS NULL", now())
	if err != nil {
		return nil, err
	}
	var out []*AccessToken
	for rows.Next() {
		t, err := scanAccessToken(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, t)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, t := range out {
		if _, err := s.db.ExecContext(ctx, "UPDATE access_tokens SET admin_until = NULL WHERE id = ? AND admin_until <= ?", t.ID, now()); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// CountTokensBy 是 RevokeTokensBy 会动到的令牌数，用于吊销前的预览。
func (s *Store) CountTokensBy(ctx context.Context, f TokenRevokeFilter) (int64, error) {
	if f.Empty() {
		return 0, errors.New("必须带筛选条件")
	}
	q := "SELECT COUNT(*) FROM access_tokens t JOIN users u ON u.id = t.user_id WHERE t.revoked_at IS NULL"
	where, args := tokenRevokeConditions(f)
	var n int64
	err := s.db.QueryRowContext(ctx, q+where, args...).Scan(&n)
	return n, err
}
