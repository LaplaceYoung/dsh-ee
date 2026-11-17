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

// 能力中心的资产：技能、提示词、微应用、企业连接。插件仍在 npm 表里，另有 plugin_policies 记可用范围。

const (
	KindSkill      = "skill"
	KindPrompt     = "prompt"
	KindApp        = "app"
	KindConnection = "connection"

	AssetActive      = "active"
	AssetUnpublished = "unpublished"

	DeliveryAuto     = "auto"
	DeliveryOptional = "optional"
)

// Scope 是可用范围。All 为真时其余字段不看；否则命中任一部门、角色或用户即可。
type Scope struct {
	All         bool     `json:"all"`
	Departments []string `json:"departments"`
	Roles       []int64  `json:"roles"`
	Users       []int64  `json:"users"`
}

// Normalize 去重、去空，让存下来的 JSON 稳定。
func (s Scope) Normalize() Scope {
	out := Scope{All: s.All, Departments: []string{}, Roles: []int64{}, Users: []int64{}}
	if s.All {
		return out
	}
	for _, d := range s.Departments {
		if d = strings.TrimSpace(d); d != "" && !slices.Contains(out.Departments, d) {
			out.Departments = append(out.Departments, d)
		}
	}
	for _, r := range s.Roles {
		if r > 0 && !slices.Contains(out.Roles, r) {
			out.Roles = append(out.Roles, r)
		}
	}
	for _, u := range s.Users {
		if u > 0 && !slices.Contains(out.Users, u) {
			out.Users = append(out.Users, u)
		}
	}
	return out
}

// Allows 判断用户是否在范围内。
func (s Scope) Allows(userID int64, department string, roleIDs []int64) bool {
	if s.All || slices.Contains(s.Users, userID) {
		return true
	}
	if department != "" && slices.Contains(s.Departments, department) {
		return true
	}
	for _, r := range roleIDs {
		if slices.Contains(s.Roles, r) {
			return true
		}
	}
	return false
}

// OnlyUser 判断范围是不是只有这一个人（「自己用」）。
func (s Scope) OnlyUser(userID int64) bool {
	n := s.Normalize()
	return !n.All && len(n.Departments) == 0 && len(n.Roles) == 0 && len(n.Users) == 1 && n.Users[0] == userID
}

// Within 判断 s 是否没有超出 outer：收窄范围不用审核，扩大才要。
// 单独列出的人如果本来就在 outer 的部门或角色里，也算没超出（userIn 判断）。
func (s Scope) Within(outer Scope, userIn func(userID int64) bool) bool {
	if outer.All {
		return true
	}
	if s.All {
		return false
	}
	for _, d := range s.Departments {
		if !slices.Contains(outer.Departments, strings.TrimSpace(d)) {
			return false
		}
	}
	for _, r := range s.Roles {
		if !slices.Contains(outer.Roles, r) {
			return false
		}
	}
	for _, u := range s.Users {
		if !slices.Contains(outer.Users, u) && (userIn == nil || !userIn(u)) {
			return false
		}
	}
	return true
}

func scopeText(s Scope) (string, error) {
	b, err := json.Marshal(s.Normalize())
	return string(b), err
}

func parseScope(raw string) Scope {
	var s Scope
	if json.Unmarshal([]byte(raw), &s) != nil {
		return Scope{}
	}
	return s.Normalize()
}

type Asset struct {
	ID          int64     `json:"id"`
	Kind        string    `json:"kind"`
	Slug        string    `json:"slug"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Category    string    `json:"category"`
	OwnerID     *int64    `json:"owner_id"`
	Scope       Scope     `json:"scope"`
	Delivery    string    `json:"delivery"`
	Status      string    `json:"status"`
	SecretHint  string    `json:"secret_hint,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	// 最高的已上架版本号，0 表示还没有上架过；列表查询时填充。
	LiveVersion int `json:"live_version"`
	// 待审核的版本数；列表查询时填充。
	Pending int `json:"pending"`
	// 负责人申请扩大到的范围，审核通过前不生效；nil 表示没有在审的范围变更
	PendingScope     *Scope     `json:"pending_scope"`
	ScopeRequestedAt *time.Time `json:"scope_requested_at"`
	// 上一次范围申请被驳回的原因（再次申请或通过时清空）
	ScopeNote string `json:"scope_note"`
	// 受管分类（asset_categories）；nil 表示没归类
	CategoryID *int64 `json:"category_id"`
	// 精选：DSH 面板里排在最前（只有管理员能设）
	Featured bool `json:"featured"`

	// 下面两项由 attachGates 在读出资产时填上，判断某个人能不能用（Gate）
	// 所在分类的可用范围；没归类时为 nil
	CategoryScope *Scope `json:"-"`
	// 命中这个资产的禁用策略的范围
	Blocks []Scope `json:"-"`
}

// 资产对某个人不可用的原因（Gate 的返回值）。
const (
	GateCategory = "category"
	GateBlocked  = "blocked"
)

// Gate 判断分类范围与禁用策略：返回空表示放行，否则是挡住的原因。
// 资产自己的可用范围、上下架不在这里看（api 层的 available）。
func (a *Asset) Gate(userID int64, department string, roleIDs []int64) string {
	if a.CategoryScope != nil && !a.CategoryScope.Allows(userID, department, roleIDs) {
		return GateCategory
	}
	for _, b := range a.Blocks {
		if b.Allows(userID, department, roleIDs) {
			return GateBlocked
		}
	}
	return ""
}

const assetCols = "a.id, a.kind, a.slug, a.title, a.description, a.category, a.owner_id, a.scope, a.delivery, a.status, a.secret_hint, a.created_at, a.updated_at, a.pending_scope, a.scope_requested_at, a.scope_note, a.category_id, a.featured"

// 列表查询附带线上版本号与待审核数。
const assetListCols = assetCols + `,
	COALESCE((SELECT MAX(v.version) FROM asset_versions v WHERE v.asset_id = a.id AND v.status = 'published'), 0),
	(SELECT COUNT(*) FROM asset_versions v WHERE v.asset_id = a.id AND v.status = 'pending')`

func scanAsset(row interface{ Scan(...any) error }, extra ...any) (*Asset, error) {
	var a Asset
	var owner sql.NullInt64
	var scope string
	var pendingScope sql.NullString
	var requestedAt sql.NullTime
	var categoryID sql.NullInt64
	dest := []any{&a.ID, &a.Kind, &a.Slug, &a.Title, &a.Description, &a.Category, &owner, &scope, &a.Delivery, &a.Status, &a.SecretHint, &a.CreatedAt, &a.UpdatedAt, &pendingScope, &requestedAt, &a.ScopeNote, &categoryID, &a.Featured}
	err := row.Scan(append(dest, extra...)...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if owner.Valid {
		v := owner.Int64
		a.OwnerID = &v
	}
	a.Scope = parseScope(scope)
	if pendingScope.Valid && pendingScope.String != "" {
		ps := parseScope(pendingScope.String)
		a.PendingScope = &ps
	}
	if requestedAt.Valid {
		t := requestedAt.Time
		a.ScopeRequestedAt = &t
	}
	if categoryID.Valid {
		v := categoryID.Int64
		a.CategoryID = &v
	}
	return &a, nil
}

func scanAssetListed(row interface{ Scan(...any) error }) (*Asset, error) {
	var live, pending int
	a, err := scanAsset(row, &live, &pending)
	if err != nil {
		return nil, err
	}
	a.LiveVersion, a.Pending = live, pending
	return a, nil
}

// AssetVersion 是资产的一个版本。Spec 的内容按 kind 解释（见 api/hub.go）。
type AssetVersion struct {
	AssetID    int64           `json:"asset_id"`
	Version    int             `json:"version"`
	Spec       json.RawMessage `json:"spec"`
	Status     string          `json:"status"`
	ReviewNote string          `json:"review_note"`
	CreatedBy  *int64          `json:"created_by"`
	ReviewedBy *int64          `json:"reviewed_by"`
	ReviewedAt *time.Time      `json:"reviewed_at"`
	CreatedAt  time.Time       `json:"created_at"`
}

const assetVersionCols = "asset_id, version, spec, status, review_note, created_by, reviewed_by, reviewed_at, created_at"

func scanAssetVersion(row interface{ Scan(...any) error }) (*AssetVersion, error) {
	var v AssetVersion
	var spec string
	var by, rby sql.NullInt64
	var rat sql.NullTime
	err := row.Scan(&v.AssetID, &v.Version, &spec, &v.Status, &v.ReviewNote, &by, &rby, &rat, &v.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	v.Spec = json.RawMessage(spec)
	if by.Valid {
		x := by.Int64
		v.CreatedBy = &x
	}
	if rby.Valid {
		x := rby.Int64
		v.ReviewedBy = &x
	}
	if rat.Valid {
		x := rat.Time
		v.ReviewedAt = &x
	}
	return &v, nil
}

// CreateAsset 新建资产与第一个版本。slug 已存在时返回 ErrDuplicate。
func (s *Store) CreateAsset(ctx context.Context, a *Asset, spec json.RawMessage, status string, by int64) error {
	scope, err := scopeText(a.Scope)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	t := now()
	if a.Status == "" {
		a.Status = AssetActive
	}
	res, err := tx.ExecContext(ctx,
		`INSERT INTO assets (kind, slug, title, description, category, category_id, featured, owner_id, scope, delivery, status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.Kind, a.Slug, a.Title, a.Description, a.Category, a.CategoryID, a.Featured, a.OwnerID, scope, a.Delivery, a.Status, t, t)
	if isDuplicate(err) {
		return ErrDuplicate
	}
	if err != nil {
		return err
	}
	if a.ID, err = res.LastInsertId(); err != nil {
		return err
	}
	if err := insertAssetVersion(ctx, tx, a.ID, 1, spec, status, by, t); err != nil {
		return err
	}
	a.CreatedAt, a.UpdatedAt = t, t
	if status == StatusPublished {
		a.LiveVersion = 1
	} else if status == StatusPending {
		a.Pending = 1
	}
	return tx.Commit()
}

func insertAssetVersion(ctx context.Context, e execer, assetID int64, version int, spec json.RawMessage, status string, by int64, t time.Time) error {
	var reviewed any
	var reviewer any
	if status == StatusPublished {
		reviewed, reviewer = t, by
	}
	_, err := e.ExecContext(ctx,
		`INSERT INTO asset_versions (asset_id, version, spec, status, created_by, reviewed_by, reviewed_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		assetID, version, string(spec), status, nullID(by), reviewer, reviewed, t)
	return err
}

func nullID(id int64) any {
	if id <= 0 {
		return nil
	}
	return id
}

// AddAssetVersion 追加一个版本，返回版本号。
func (s *Store) AddAssetVersion(ctx context.Context, assetID int64, spec json.RawMessage, status string, by int64) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var max sql.NullInt64
	// 锁住资产行，避免两个人同时提交拿到同一个版本号
	if _, err := tx.ExecContext(ctx, "SELECT id FROM assets WHERE id = ? FOR UPDATE", assetID); err != nil {
		return 0, err
	}
	if err := tx.QueryRowContext(ctx, "SELECT MAX(version) FROM asset_versions WHERE asset_id = ?", assetID).Scan(&max); err != nil {
		return 0, err
	}
	v := int(max.Int64) + 1
	t := now()
	if err := insertAssetVersion(ctx, tx, assetID, v, spec, status, by, t); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE assets SET updated_at = ? WHERE id = ?", t, assetID); err != nil {
		return 0, err
	}
	return v, tx.Commit()
}

// DeleteAssetVersion 撤销一个刚追加、还没生效的版本（回滚在写库之后失败时用）。
// 只删这一个版本，不碰其它版本和资产状态；版本不存在时返回 ErrNotFound。
func (s *Store) DeleteAssetVersion(ctx context.Context, assetID int64, version int) error {
	res, err := s.db.ExecContext(ctx,
		"DELETE FROM asset_versions WHERE asset_id = ? AND version = ?", assetID, version)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) GetAsset(ctx context.Context, id int64) (*Asset, error) {
	return s.gated(ctx)(scanAssetListed(s.db.QueryRowContext(ctx, "SELECT "+assetListCols+" FROM assets a WHERE a.id = ?", id)))
}

func (s *Store) GetAssetBySlug(ctx context.Context, slug string) (*Asset, error) {
	return s.gated(ctx)(scanAssetListed(s.db.QueryRowContext(ctx, "SELECT "+assetListCols+" FROM assets a WHERE a.slug = ?", slug)))
}

// gated 给读出来的单个资产填上分类范围与禁用策略。
func (s *Store) gated(ctx context.Context) func(*Asset, error) (*Asset, error) {
	return func(a *Asset, err error) (*Asset, error) {
		if err != nil {
			return nil, err
		}
		return a, s.attachGates(ctx, []*Asset{a})
	}
}

// ListAssets 列出资产；kinds 为空表示全部类型。
func (s *Store) ListAssets(ctx context.Context, kinds ...string) ([]*Asset, error) {
	q := "SELECT " + assetListCols + " FROM assets a"
	var args []any
	if len(kinds) > 0 {
		q += " WHERE a.kind IN (" + placeholders(len(kinds)) + ")"
		for _, k := range kinds {
			args = append(args, k)
		}
	}
	rows, err := s.db.QueryContext(ctx, q+" ORDER BY a.id", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Asset{}
	for rows.Next() {
		a, err := scanAssetListed(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, s.attachGates(ctx, out)
}

// AssetPatch 里为 nil 的字段不修改。
type AssetPatch struct {
	Title       *string
	Description *string
	Category    *string
	Scope       *Scope
	Delivery    *string
	Status      *string
	// ExpectedUpdatedAt 非空时做乐观并发检查：只有 updated_at 还是这个值才写，
	// 否则返回 ErrConflict。范围是整段覆盖的，用旧快照保存会丢掉期间新审批进来的授权。
	ExpectedUpdatedAt *time.Time
	// ClearPendingScope 在同一条 UPDATE 里撤销在审的范围申请（范围改成了不需要审核的样子）。
	// 必须和 Scope 一起原子写：分两句的话，「保存成功」到「清空」之间的窗口会把并发提交的
	// 新申请静默清掉；清空本身也要受 ExpectedUpdatedAt 的同一次检查保护。
	ClearPendingScope bool
	// 受管分类：指向 0 表示取消归类；设了分类时 Category 文本由调用方同步成分类名
	CategoryID *int64
	Featured   *bool
}

func (s *Store) UpdateAsset(ctx context.Context, id int64, p AssetPatch) error {
	sets := []string{"updated_at = ?"}
	args := []any{now()}
	add := func(col string, v any) {
		sets = append(sets, col+" = ?")
		args = append(args, v)
	}
	if p.Title != nil {
		add("title", *p.Title)
	}
	if p.Description != nil {
		add("description", *p.Description)
	}
	if p.Category != nil {
		add("category", *p.Category)
	}
	if p.Scope != nil {
		text, err := scopeText(*p.Scope)
		if err != nil {
			return err
		}
		add("scope", text)
	}
	if p.Delivery != nil {
		add("delivery", *p.Delivery)
	}
	if p.Status != nil {
		add("status", *p.Status)
	}
	if p.ClearPendingScope {
		// 与上面一样进同一条 UPDATE、同一次 updated_at 检查：不能拆成两句
		sets = append(sets, "pending_scope = NULL", "scope_requested_at = NULL")
	}
	if p.CategoryID != nil {
		add("category_id", nullID(*p.CategoryID))
	}
	if p.Featured != nil {
		add("featured", *p.Featured)
	}
	args = append(args, id)
	where := "id = ?"
	if p.ExpectedUpdatedAt != nil {
		where += " AND updated_at = ?"
		args = append(args, *p.ExpectedUpdatedAt)
	}
	res, err := s.db.ExecContext(ctx, "UPDATE assets SET "+strings.Join(sets, ", ")+" WHERE "+where, args...)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		if p.ExpectedUpdatedAt != nil {
			return s.casMiss(ctx, "assets", "id", id, *p.ExpectedUpdatedAt)
		}
		var existing int64
		err := s.db.QueryRowContext(ctx, "SELECT id FROM assets WHERE id = ?", id).Scan(&existing)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	return nil
}

// GrantAssetScopeUser 把用户追加进资产的可用范围，返回是否真的加了。
// 在事务里锁住资产行再读改写，两个审批并发时不会互相覆盖（否则后写者会丢掉前一个用户）。
// 范围是「所有人」或已经包含该用户时不写库，返回 added=false（幂等）。
func (s *Store) GrantAssetScopeUser(ctx context.Context, assetID, userID int64) (added bool, err error) {
	return s.grantScopeUser(ctx, "assets", assetID, userID)
}

// grantScopeUser 是 GrantAssetScopeUser / GrantKBaseScopeUser 的共同实现。
// table 只能是本包内的常量表名，不能来自用户输入。
func (s *Store) grantScopeUser(ctx context.Context, table string, id, userID int64) (bool, error) {
	if userID <= 0 { // 与 Scope.Normalize 丢弃非法用户 id 的语义保持一致，不写库
		return false, nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var raw string
	err = tx.QueryRowContext(ctx, "SELECT scope FROM "+table+" WHERE id = ? FOR UPDATE", id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return false, ErrNotFound
	}
	if err != nil {
		return false, err
	}
	sc := parseScope(raw)
	if sc.All || slices.Contains(sc.Users, userID) {
		return false, nil // 已经能用：不写库，也不动 updated_at
	}
	sc.Users = append(sc.Users, userID)
	text, err := scopeText(sc)
	if err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE "+table+" SET scope = ?, updated_at = ? WHERE id = ?", text, now(), id); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

// RequestAssetScope 记下负责人申请的新范围，等审核；已有的申请被替换。
// expected 非空时做乐观并发检查（申请也是整段范围的快照，要看快照是不是最新的）。
// 返回这次申请的快照标识 scope_requested_at：审批人必须原样带回，
// 否则期间被替换掉的旧申请会去消费新申请（见 ResolveAssetScope）。
func (s *Store) RequestAssetScope(ctx context.Context, id int64, sc Scope, expected *time.Time) (time.Time, error) {
	text, err := scopeText(sc)
	if err != nil {
		return time.Time{}, err
	}
	t := now()
	q := "UPDATE assets SET pending_scope = ?, scope_requested_at = ?, scope_note = '', updated_at = ? WHERE id = ?"
	args := []any{text, t, t, id}
	if expected != nil {
		q += " AND updated_at = ?"
		args = append(args, *expected)
	}
	res, err := s.db.ExecContext(ctx, q, args...)
	if err != nil {
		return time.Time{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 && expected != nil {
		return time.Time{}, s.casMiss(ctx, "assets", "id", id, *expected)
	}
	return t, nil
}

// ResolveAssetScope 审核范围申请：通过时新范围生效，驳回时记下原因。返回是否有在审的申请。
// expectedRequestedAt 是审批人评审的那份申请的快照标识（scope_requested_at）：事务里锁住资产行后
// 必须与当前 pending_scope 的标识一致。不一致说明负责人已经把申请换成了新的，返回 ErrConflict，
// 既不通过也不消费新申请——否则审批人点一次通过，生效的会是他没看过的快照。
// 通过时不能把 pending_scope 直接整段写回：它是申请那一刻的快照，之后可能又有人
// （如审批「申请使用」）往范围里追加了用户。这里以快照为基础合并当前范围里的 Users，
// 只让申请本身想加的 All / Departments / Roles 生效；当前已经是「所有人」时保持所有人，
// 不因为旧申请再收窄。
func (s *Store) ResolveAssetScope(ctx context.Context, id int64, approve bool, note string, expectedRequestedAt time.Time) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var curRaw string
	var pendingRaw sql.NullString
	var requestedAt sql.NullTime
	err = tx.QueryRowContext(ctx, "SELECT scope, pending_scope, scope_requested_at FROM assets WHERE id = ? FOR UPDATE", id).Scan(&curRaw, &pendingRaw, &requestedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !pendingRaw.Valid || pendingRaw.String == "" {
		return false, nil
	}
	// 令牌缺失或对不上都按冲突处理：调用方不能用「不传令牌」绕过快照校验。
	if expectedRequestedAt.IsZero() || !requestedAt.Valid || !requestedAt.Time.Equal(expectedRequestedAt) {
		return false, ErrConflict
	}
	if !approve {
		if _, err := tx.ExecContext(ctx,
			"UPDATE assets SET pending_scope = NULL, scope_requested_at = NULL, scope_note = ?, updated_at = ? WHERE id = ?",
			note, now(), id); err != nil {
			return false, err
		}
		return true, tx.Commit()
	}
	cur := parseScope(curRaw)
	next := parseScope(pendingRaw.String)
	if cur.All {
		next = Scope{All: true}
	} else if !next.All {
		for _, u := range cur.Users {
			if !slices.Contains(next.Users, u) {
				next.Users = append(next.Users, u)
			}
		}
	}
	text, err := scopeText(next)
	if err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx,
		"UPDATE assets SET scope = ?, pending_scope = NULL, scope_requested_at = NULL, scope_note = '', updated_at = ? WHERE id = ?",
		text, now(), id); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

func (s *Store) DeleteAsset(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, q := range []string{
		"DELETE FROM asset_versions WHERE asset_id = ?",
		"DELETE FROM asset_subscriptions WHERE asset_id = ?",
		"DELETE FROM connection_tools WHERE asset_id = ?",
		"DELETE FROM connection_shared_credentials WHERE asset_id = ?",
		"DELETE FROM user_credentials WHERE asset_id = ?",
		// 还没处理的权限申请没有对象了，一起删；处理过的留着当记录
		"DELETE FROM connection_requests WHERE asset_id = ? AND kind <> 'knowledge' AND status = 'pending'",
		"DELETE FROM assets WHERE id = ?",
	} {
		if _, err := tx.ExecContext(ctx, q, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// SetAssetSecret 保存连接的上游凭据（已加密）；enc 为 nil 表示清除。
func (s *Store) SetAssetSecret(ctx context.Context, id int64, enc []byte, hint string) error {
	_, err := s.db.ExecContext(ctx, "UPDATE assets SET secret_enc = ?, secret_hint = ?, updated_at = ? WHERE id = ?", enc, hint, now(), id)
	return err
}

func (s *Store) AssetSecret(ctx context.Context, id int64) ([]byte, error) {
	var enc []byte
	err := s.db.QueryRowContext(ctx, "SELECT secret_enc FROM assets WHERE id = ?", id).Scan(&enc)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return enc, err
}

func (s *Store) ListAssetVersions(ctx context.Context, assetID int64) ([]*AssetVersion, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+assetVersionCols+" FROM asset_versions WHERE asset_id = ? ORDER BY version DESC", assetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*AssetVersion{}
	for rows.Next() {
		v, err := scanAssetVersion(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) GetAssetVersion(ctx context.Context, assetID int64, version int) (*AssetVersion, error) {
	return scanAssetVersion(s.db.QueryRowContext(ctx, "SELECT "+assetVersionCols+" FROM asset_versions WHERE asset_id = ? AND version = ?", assetID, version))
}

// LiveAssetVersion 是最高的已上架版本。
func (s *Store) LiveAssetVersion(ctx context.Context, assetID int64) (*AssetVersion, error) {
	return scanAssetVersion(s.db.QueryRowContext(ctx,
		"SELECT "+assetVersionCols+" FROM asset_versions WHERE asset_id = ? AND status = 'published' ORDER BY version DESC LIMIT 1", assetID))
}

// LiveAssetVersions 一次取多个资产的线上版本。
func (s *Store) LiveAssetVersions(ctx context.Context, ids []int64) (map[int64]*AssetVersion, error) {
	out := map[int64]*AssetVersion{}
	if len(ids) == 0 {
		return out, nil
	}
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+assetVersionCols+` FROM asset_versions v
		WHERE v.asset_id IN (`+placeholders(len(ids))+`) AND v.status = 'published'
		  AND v.version = (SELECT MAX(x.version) FROM asset_versions x WHERE x.asset_id = v.asset_id AND x.status = 'published')`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		v, err := scanAssetVersion(rows)
		if err != nil {
			return nil, err
		}
		out[v.AssetID] = v
	}
	return out, rows.Err()
}

// PendingAssetVersions 是审核队列里的资产版本，按提交时间排序。
func (s *Store) PendingAssetVersions(ctx context.Context) ([]*AssetVersion, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+assetVersionCols+" FROM asset_versions WHERE status = 'pending' ORDER BY created_at")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*AssetVersion{}
	for rows.Next() {
		v, err := scanAssetVersion(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// SetAssetVersionStatus 把处于 from 状态之一的版本改成 to；返回是否改到了。
func (s *Store) SetAssetVersionStatus(ctx context.Context, assetID int64, version int, from []string, to string, reviewer int64, note string) (bool, error) {
	args := []any{to, nullID(reviewer), now(), note, assetID, version}
	for _, f := range from {
		args = append(args, f)
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE asset_versions SET status = ?, reviewed_by = ?, reviewed_at = ?, review_note = ?
		 WHERE asset_id = ? AND version = ? AND status IN (`+placeholders(len(from))+`)`, args...)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		_, _ = s.db.ExecContext(ctx, "UPDATE assets SET updated_at = ? WHERE id = ?", now(), assetID)
	}
	return n > 0, nil
}

// ---- 订阅与使用 ----

func (s *Store) Subscribe(ctx context.Context, userID, assetID int64) error {
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO asset_subscriptions (user_id, asset_id, created_at) VALUES (?, ?, ?) ON DUPLICATE KEY UPDATE user_id = user_id",
		userID, assetID, now())
	return err
}

func (s *Store) Unsubscribe(ctx context.Context, userID, assetID int64) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM asset_subscriptions WHERE user_id = ? AND asset_id = ?", userID, assetID)
	return err
}

// Subscriptions 是用户启用了的资产 id。
func (s *Store) Subscriptions(ctx context.Context, userID int64) (map[int64]bool, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT asset_id FROM asset_subscriptions WHERE user_id = ?", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

func (s *Store) RecordAssetUsage(ctx context.Context, assetID, userID int64) error {
	_, err := s.db.ExecContext(ctx, "INSERT INTO asset_usage (asset_id, user_id, created_at) VALUES (?, ?, ?)", assetID, userID, now())
	return err
}

// AssetUsage 是窗口内每个资产的使用次数与人数：技能类按加载次数，连接按工具调用次数。
type AssetUse struct {
	Uses  int `json:"uses"`
	Users int `json:"users"`
}

func (s *Store) AssetUsage(ctx context.Context, from time.Time) (map[int64]AssetUse, error) {
	out := map[int64]AssetUse{}
	for _, q := range []string{
		"SELECT asset_id, COUNT(*), COUNT(DISTINCT user_id) FROM asset_usage WHERE created_at >= ? GROUP BY asset_id",
		"SELECT asset_id, COUNT(*), COUNT(DISTINCT user_id) FROM connection_calls WHERE created_at >= ? GROUP BY asset_id",
	} {
		rows, err := s.db.QueryContext(ctx, q, from)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id int64
			var u AssetUse
			if err := rows.Scan(&id, &u.Uses, &u.Users); err != nil {
				rows.Close()
				return nil, err
			}
			out[id] = u
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// ---- 插件的可用范围 ----

type PluginPolicy struct {
	Name      string    `json:"name"`
	Scope     Scope     `json:"scope"`
	Delivery  string    `json:"delivery"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (s *Store) PluginPolicies(ctx context.Context) (map[string]PluginPolicy, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT name, scope, delivery, updated_at FROM plugin_policies")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]PluginPolicy{}
	for rows.Next() {
		var p PluginPolicy
		var scope string
		if err := rows.Scan(&p.Name, &scope, &p.Delivery, &p.UpdatedAt); err != nil {
			return nil, err
		}
		p.Scope = parseScope(scope)
		out[p.Name] = p
	}
	return out, rows.Err()
}

// SetPluginPolicy 保存插件策略。expected 非空时做乐观并发检查（策略的可用范围是整段覆盖的，
// 拿旧快照保存会覆盖掉别人的改动）；0 行受影响 → ErrNotFound / ErrConflict。
// expected 为空表示新建（页面上还没有这条策略）：已有记录时不覆盖，返回 ErrConflict。
// 首次创建是单条 INSERT，靠 name 的唯一键冲突判定「已经有人先建了」（不做先查再插），
// 所以两个管理员同时首次设置时只有一个 200，另一个稳定拿到 409。
func (s *Store) SetPluginPolicy(ctx context.Context, p *PluginPolicy, by int64, expected *time.Time) error {
	scope, err := scopeText(p.Scope)
	if err != nil {
		return err
	}
	n := now()
	if expected == nil {
		_, err := s.db.ExecContext(ctx,
			"INSERT INTO plugin_policies (name, scope, delivery, updated_by, updated_at) VALUES (?, ?, ?, ?, ?)",
			p.Name, scope, p.Delivery, nullID(by), n)
		if isDuplicate(err) {
			return ErrConflict
		}
		if err != nil {
			return err
		}
		p.UpdatedAt = n
		return nil
	}
	res, err := s.db.ExecContext(ctx,
		"UPDATE plugin_policies SET scope = ?, delivery = ?, updated_by = ?, updated_at = ? WHERE name = ? AND updated_at = ?",
		scope, p.Delivery, nullID(by), n, p.Name, *expected)
	if err != nil {
		return err
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		return s.casMiss(ctx, "plugin_policies", "name", p.Name, *expected)
	}
	p.UpdatedAt = n
	return nil
}

// DefaultPluginPolicy 是没有记录时的策略：全公司可见、自选安装。
func DefaultPluginPolicy(name string) PluginPolicy {
	return PluginPolicy{Name: name, Scope: Scope{All: true}.Normalize(), Delivery: DeliveryOptional}
}
