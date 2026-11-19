package store

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/secret"
)

// refresh token 的库层行为（docs/token-lifecycle.md §3）：轮换/复用族吊销、批量吊销联动、
// 过期清理。需要数据库，和 package 内其它 DB 测试一样由 DSH_EE_TEST_DSN 打开。

func createRefreshableToken(t *testing.T, st *Store, u *User, plain string) *AccessToken {
	t.Helper()
	nowT := now()
	expires := nowT.Add(time.Hour)
	token := &AccessToken{
		UserID: u.ID, Name: "refresh-store", Prefix: plain[:12], Source: "manual",
		ClientKind: ClientKindThirdParty, Scopes: []string{"models"}, CreatedBy: u.ID, ExpiresAt: &expires,
	}
	rt := &RefreshToken{
		FamilyID: NewRefreshFamilyID(), UserID: u.ID, ClientKind: ClientKindThirdParty,
		SecretHash: secret.Hash(plain), IssuedAt: nowT, ExpiresAt: nowT.Add(time.Hour),
		FamilyExpiresAt: nowT.Add(2 * time.Hour), CreatedIP: "127.0.0.1",
	}
	if err := st.CreateAccessTokenWithRefresh(context.Background(), token, secret.Hash("access-"+plain), rt); err != nil {
		t.Fatal(err)
	}
	return token
}

func refreshRowState(t *testing.T, st *Store, plain string) (rotated, revoked, reuse sql.NullTime) {
	t.Helper()
	err := st.DB().QueryRowContext(context.Background(),
		"SELECT rotated_at, revoked_at, reuse_detected_at FROM token_refresh_tokens WHERE secret_hash = ?", secret.Hash(plain)).
		Scan(&rotated, &revoked, &reuse)
	if err != nil {
		t.Fatalf("查 refresh 记录: %v", err)
	}
	return rotated, revoked, reuse
}

func TestRefreshTokenStoreRotationAndReuse(t *testing.T) {
	st := openTokenTestStore(t)
	ctx := context.Background()
	u := tokenTestUser(t, st, false)
	seed := createRefreshableToken(t, st, u, "ee_rt_store_seed")
	before := now()
	grant := RefreshGrant{
		OldHash:       secret.Hash("ee_rt_store_seed"),
		IP:            "10.0.0.1",
		NewAccessHash: "hash-new-access", NewAccessPrefix: "ee_at_new",
		NewRefreshHash: "hash-new-refresh",
		AccessTTL:      30 * time.Minute, RefreshTTL: time.Hour,
	}
	rotation, err := st.RotateRefreshToken(ctx, grant)
	if err != nil {
		t.Fatal(err)
	}
	if rotation.Reused || rotation.AccessID == 0 || rotation.RefreshID == 0 || rotation.FamilyID == "" {
		t.Fatalf("轮换结果不对：%+v", rotation)
	}
	if rotation.AccessExpires.Before(before.Add(29 * time.Minute)) {
		t.Fatalf("新 access 的 TTL 不对：%v", rotation.AccessExpires)
	}
	rotated, revoked, reuse := refreshRowState(t, st, "ee_rt_store_seed")
	if !rotated.Valid || revoked.Valid || reuse.Valid {
		t.Fatalf("旧行应只标记 rotated：rotated=%v revoked=%v reuse=%v", rotated, revoked, reuse)
	}
	// 新 access token 已落库且同一个 family
	newAccess, err := st.AccessTokenByID(ctx, rotation.AccessID)
	if err != nil {
		t.Fatal(err)
	}
	if newAccess.FamilyID == nil || *newAccess.FamilyID != rotation.FamilyID || newAccess.RevokedAt != nil {
		t.Fatalf("新 access token 的族信息不对：%+v", newAccess)
	}
	if seed.FamilyID == nil || *seed.FamilyID != rotation.FamilyID {
		t.Fatalf("初始 access token 也应在同一个族：%+v", seed.FamilyID)
	}

	firstAccessID := rotation.AccessID
	// 旧 refresh 再次出现：按复用处理，吊销整族
	rotation, err = st.RotateRefreshToken(ctx, grant)
	if err != nil {
		t.Fatal(err)
	}
	if !rotation.Reused || rotation.RevokedAccess == 0 || rotation.RevokedRefresh == 0 {
		t.Fatalf("复用应吊销整族：%+v", rotation)
	}
	_, revoked, reuse = refreshRowState(t, st, "ee_rt_store_seed")
	if !revoked.Valid || !reuse.Valid {
		t.Fatalf("复用后旧行应 revoked + reuse_detected：revoked=%v reuse=%v", revoked, reuse)
	}
	newAccess, err = st.AccessTokenByID(ctx, firstAccessID)
	if err != nil {
		t.Fatal(err)
	}
	if newAccess.RevokedAt == nil {
		t.Fatal("复用后同族的 access token 也应被吊销")
	}
	// 族内新 refresh 也已失效
	state, err := scanRefreshToken(st.DB().QueryRowContext(ctx,
		"SELECT "+refreshTokenColumns+" FROM token_refresh_tokens WHERE secret_hash = ?", grant.NewRefreshHash))
	if err != nil {
		t.Fatal(err)
	}
	if state.RevokedAt == nil {
		t.Fatal("复用后同族的新 refresh token 也应被吊销")
	}
}

func TestRefreshTokenStoreBulkRevokeAndCleanup(t *testing.T) {
	st := openTokenTestStore(t)
	ctx := context.Background()
	u := tokenTestUser(t, st, false)
	createRefreshableToken(t, st, u, "ee_rt_store_bulk_a")
	createRefreshableToken(t, st, u, "ee_rt_store_bulk_b")

	// 批量吊销 access token：族里的 refresh 也要一起失效（否则被吊销的令牌能用 refresh 续期）
	if n, err := st.RevokeTokensBy(ctx, TokenRevokeFilter{UserID: &u.ID}); err != nil || n != 2 {
		t.Fatalf("批量吊销：n=%d err=%v", n, err)
	}
	var live int
	if err := st.DB().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM token_refresh_tokens WHERE user_id = ? AND revoked_at IS NULL", u.ID).Scan(&live); err != nil {
		t.Fatal(err)
	}
	if live != 0 {
		t.Fatalf("批量吊销后还有 %d 枚 refresh 未失效", live)
	}

	// 清理：过期且已轮换/吊销的 refresh 记录会被删除；successor 密文到时间也会清空
	if _, err := st.DB().ExecContext(ctx, `
		INSERT INTO token_refresh_tokens (family_id, access_token_id, user_id, client_kind, secret_hash,
			issued_at, expires_at, family_expires_at, rotated_at, revoked_at, created_ip, successor_sealed)
		VALUES (?, 1, ?, 'third_party', ?, ?, ?, ?, ?, ?, '', ?)`,
		NewRefreshFamilyID(), u.ID, secret.Hash("ee_rt_store_expired"),
		now().Add(-2*time.Hour), now().Add(-time.Hour), now().Add(time.Hour), now().Add(-2*time.Hour), now().Add(-time.Hour),
		[]byte("sealed")); err != nil {
		t.Fatal(err)
	}
	if n, err := st.DeleteExpiredRefreshTokens(ctx, now()); err != nil || n != 1 {
		t.Fatalf("清理过期 refresh：n=%d err=%v", n, err)
	}
	// successor 密文：没轮换到宽的旧记录会被清空，窗口内的保留
	if _, err := st.DB().ExecContext(ctx, `
		INSERT INTO token_refresh_tokens (family_id, access_token_id, user_id, client_kind, secret_hash,
			issued_at, expires_at, family_expires_at, rotated_at, created_ip, successor_sealed)
		VALUES (?, 1, ?, 'third_party', ?, ?, ?, ?, ?, '', ?)`,
		NewRefreshFamilyID(), u.ID, secret.Hash("ee_rt_store_sealed"),
		now().Add(-2*time.Hour), now().Add(time.Hour), now().Add(time.Hour), now().Add(-2*time.Hour), []byte("sealed")); err != nil {
		t.Fatal(err)
	}
	if n, err := st.ClearRefreshSuccessors(ctx, now().Add(-time.Hour)); err != nil || n != 1 {
		t.Fatalf("清理重放密文：n=%d err=%v", n, err)
	}
	var remaining int
	if err := st.DB().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM token_refresh_tokens WHERE secret_hash = ? AND successor_sealed IS NOT NULL",
		secret.Hash("ee_rt_store_sealed")).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Fatal("过了宽限期的 successor 密文应被清空")
	}
}

// 令牌的月度额度管的是这把令牌，不是某一代 access token：开了 refresh 之后每次续期都换一个新 id，
// 用量要按整族算，否则额度每次续期都清零（每小时一次续期 = 每小时一份「月度」额度）。
func TestTokenMonthUsageCountsWholeRefreshFamily(t *testing.T) {
	st := openTokenTestStore(t)
	ctx := context.Background()
	u := tokenTestUser(t, st, false)
	seed := createRefreshableToken(t, st, u, "ee_rt_usage_seed")
	other := createRefreshableToken(t, st, u, "ee_rt_usage_other") // 同一个人的另一把令牌，不算进来
	record := func(tokenID int64, n int) {
		if err := st.RecordUsage(ctx, Usage{UserID: u.ID, TokenID: tokenID, Model: "m", TotalTokens: n, StatusCode: 200}); err != nil {
			t.Fatal(err)
		}
	}
	record(seed.ID, 70)
	record(other.ID, 1000)
	rotation, err := st.RotateRefreshToken(ctx, RefreshGrant{
		OldHash: secret.Hash("ee_rt_usage_seed"), IP: "10.0.0.1",
		NewAccessHash: "hash-usage-access", NewAccessPrefix: "ee_at_usage", NewRefreshHash: "hash-usage-refresh",
		AccessTTL: 30 * time.Minute, RefreshTTL: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	record(rotation.AccessID, 5)
	for name, id := range map[string]int64{"续期前": seed.ID, "续期后": rotation.AccessID} {
		got, err := st.TokenMonthUsage(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if got != 75 {
			t.Fatalf("%s的令牌本月用量应按整族算 75，得到 %d", name, got)
		}
	}
	if got, _ := st.TokenMonthUsage(ctx, other.ID); got != 1000 {
		t.Fatalf("另一把令牌的用量不受影响：%d", got)
	}
}
