package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"strings"
	"time"
)

const userCodeAlphabet = "BCDFGHJKLMNPQRSTVWXZ23456789"

type DeviceCode struct {
	ID              int64      `json:"id"`
	DeviceHash      string     `json:"-"`
	UserCode        string     `json:"user_code"`
	ClientName      string     `json:"client_name"`
	ClientOS        string     `json:"client_os"`
	ClientVersion   string     `json:"client_version"`
	ClientIP        string     `json:"client_ip"`
	IntervalSeconds int        `json:"interval_seconds"`
	Scopes          string     `json:"scopes"`
	Status          string     `json:"status"`
	UserID          *int64     `json:"user_id"`
	TokenID         *int64     `json:"token_id"`
	TokenSealed     []byte     `json:"-"`
	LastPollAt      *time.Time `json:"last_poll_at"`
	ExpiresAt       time.Time  `json:"expires_at"`
	CreatedAt       time.Time  `json:"created_at"`
}

const deviceColumns = `id, device_hash, user_code, client_name, client_os, client_version, client_ip, interval_seconds, scopes,
	status, user_id, token_id, token_sealed, last_poll_at, expires_at, created_at`

func scanDevice(row interface{ Scan(...any) error }) (*DeviceCode, error) {
	var d DeviceCode
	var userID, tokenID sql.NullInt64
	var sealed []byte
	var poll sql.NullTime
	err := row.Scan(&d.ID, &d.DeviceHash, &d.UserCode, &d.ClientName, &d.ClientOS, &d.ClientVersion, &d.ClientIP, &d.IntervalSeconds, &d.Scopes,
		&d.Status, &userID, &tokenID, &sealed, &poll, &d.ExpiresAt, &d.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if userID.Valid {
		d.UserID = &userID.Int64
	}
	if tokenID.Valid {
		d.TokenID = &tokenID.Int64
	}
	d.TokenSealed = sealed
	if poll.Valid {
		d.LastPollAt = &poll.Time
	}
	return &d, nil
}

func NewUserCode() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	for i := range b {
		b[i] = userCodeAlphabet[int(b[i])%len(userCodeAlphabet)]
	}
	return string(b)
}

func NormalizeUserCode(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	code = strings.ReplaceAll(code, "-", "")
	return code
}

func FormatUserCode(code string) string {
	code = NormalizeUserCode(code)
	if len(code) == 8 {
		return code[:4] + "-" + code[4:]
	}
	return code
}

func (s *Store) CreateDeviceCode(ctx context.Context, d *DeviceCode, deviceHash string) error {
	d.DeviceHash = deviceHash
	d.UserCode = NormalizeUserCode(d.UserCode)
	d.CreatedAt = now()
	if d.IntervalSeconds == 0 {
		d.IntervalSeconds = 5
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO device_codes (device_hash, user_code, client_name, client_os, client_version, client_ip, interval_seconds, scopes,
		status, user_id, token_id, token_sealed, last_poll_at, expires_at, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		deviceHash, d.UserCode, d.ClientName, d.ClientOS, d.ClientVersion, d.ClientIP, d.IntervalSeconds, d.Scopes,
		d.Status, d.UserID, d.TokenID, d.TokenSealed, d.LastPollAt, d.ExpiresAt, d.CreatedAt)
	if err != nil {
		return err
	}
	d.ID, err = res.LastInsertId()
	return err
}

func (s *Store) DeviceByUserCode(ctx context.Context, code string) (*DeviceCode, error) {
	return scanDevice(s.db.QueryRowContext(ctx, "SELECT "+deviceColumns+" FROM device_codes WHERE user_code = ?", NormalizeUserCode(code)))
}

func (s *Store) DeviceByHash(ctx context.Context, hash string) (*DeviceCode, error) {
	return scanDevice(s.db.QueryRowContext(ctx, "SELECT "+deviceColumns+" FROM device_codes WHERE device_hash = ?", hash))
}

func (s *Store) PollDevice(ctx context.Context, code string, interval time.Duration) (bool, error) {
	d, err := s.DeviceByUserCode(ctx, code)
	if err != nil {
		return false, err
	}
	t := now()
	if d.LastPollAt != nil && d.LastPollAt.Add(interval).After(t) {
		_, _ = s.db.ExecContext(ctx, "UPDATE device_codes SET interval_seconds = interval_seconds + 5 WHERE id = ?", d.ID)
		return true, nil
	}
	_, err = s.db.ExecContext(ctx, "UPDATE device_codes SET last_poll_at = ? WHERE id = ?", t, d.ID)
	return false, err
}

// ApproveDevice 批准设备码并签出 access token（只在开启 refresh 时再带上第一枚 refresh token）。
// refresh 用可变参数是为了不影响已有调用方：不传表示这次签发不带 refresh。
func (s *Store) ApproveDevice(ctx context.Context, code string, userID int64, token *AccessToken, tokenHash string, sealed []byte, refresh ...*RefreshToken) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var d DeviceCode
	var userIDNull, tokenIDNull sql.NullInt64
	var sealedExisting []byte
	var poll sql.NullTime
	err = tx.QueryRowContext(ctx, `SELECT `+deviceColumns+` FROM device_codes WHERE user_code = ? AND status = 'pending' AND expires_at > ? FOR UPDATE`,
		NormalizeUserCode(code), now()).Scan(&d.ID, &d.DeviceHash, &d.UserCode, &d.ClientName, &d.ClientOS, &d.ClientVersion, &d.ClientIP, &d.IntervalSeconds, &d.Scopes,
		&d.Status, &userIDNull, &tokenIDNull, &sealedExisting, &poll, &d.ExpiresAt, &d.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	var accountType string
	if err := tx.QueryRowContext(ctx, "SELECT account_type FROM users WHERE id = ? AND disabled = 0", userID).Scan(&accountType); err != nil {
		return err
	}
	if accountType != "human" {
		return ErrNotFound
	}
	token.UserID = userID
	token.CreatedBy = userID
	token.Source = "device"
	// 设备码是 DSH 的登录方式：签出来的令牌默认就是 DSH 客户端
	token.ClientKind = ClientKindDSH
	token.ClientName = d.ClientName
	token.ClientOS = d.ClientOS
	token.ClientVersion = d.ClientVersion
	token.Scopes = strings.Split(d.Scopes, ",")
	var rt *RefreshToken
	if len(refresh) > 0 {
		rt = refresh[0]
	}
	if err := createAccessTokenWithRefreshTx(ctx, tx, token, tokenHash, rt); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `UPDATE device_codes SET status = 'approved', user_id = ?, token_id = ?, token_sealed = ?
		WHERE id = ? AND status = 'pending'`, userID, token.ID, sealed, d.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return ErrNotFound
	}
	// 没有 client_name 的客户端认不出是不是同一台设备，不能把别的设备当成旧登录吊销
	// （确认页也只在有名字时提示「会替换旧登录」）。
	if d.ClientName == "" {
		return tx.Commit()
	}
	if _, err := tx.ExecContext(ctx, `UPDATE access_tokens SET revoked_at = ?
		WHERE user_id = ? AND client_name = ? AND source = 'device' AND id <> ? AND revoked_at IS NULL`, now(), userID, d.ClientName, token.ID); err != nil {
		return err
	}
	// 同一台设备重新登录：旧令牌族里的 refresh token 一起作废，避免旧 refresh 还能续期。
	if _, err := tx.ExecContext(ctx, `UPDATE token_refresh_tokens rt JOIN access_tokens t ON t.family_id = rt.family_id
		SET rt.revoked_at = ? WHERE rt.revoked_at IS NULL AND t.user_id = ? AND t.client_name = ? AND t.source = 'device' AND t.id <> ?`,
		now(), userID, d.ClientName, token.ID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) DenyDevice(ctx context.Context, code string) error {
	res, err := s.db.ExecContext(ctx, "UPDATE device_codes SET status = 'denied' WHERE user_code = ? AND status = 'pending'", NormalizeUserCode(code))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ConsumeDevice(ctx context.Context, code string) ([]byte, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var id int64
	var sealed []byte
	err = tx.QueryRowContext(ctx, "SELECT id, token_sealed FROM device_codes WHERE user_code = ? AND status = 'approved' AND token_sealed IS NOT NULL FOR UPDATE",
		NormalizeUserCode(code)).Scan(&id, &sealed)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	res, err := tx.ExecContext(ctx, "UPDATE device_codes SET status = 'consumed', token_sealed = NULL WHERE id = ? AND status = 'approved'", id)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return nil, ErrNotFound
	}
	return sealed, tx.Commit()
}

func (s *Store) DeleteExpiredDeviceCodes(ctx context.Context) (int64, error) {
	res, err := s.db.ExecContext(ctx, "DELETE FROM device_codes WHERE expires_at <= ?", now())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
