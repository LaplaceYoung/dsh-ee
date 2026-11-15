package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/secret"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// 每个用户最多同时保留的有效 key 数。key 只是凭证，额度按用户共享，
// 限制数量主要是防止误建和泄露面扩大。
const maxKeysPerUser = 10

// ---- 可空字段：区分 JSON 里「没传」「显式 null」「有值」 ----

type nullableInt64 struct {
	Set   bool
	Value *int64
}

func (n *nullableInt64) UnmarshalJSON(b []byte) error {
	n.Set = true
	if string(b) == "null" {
		n.Value = nil
		return nil
	}
	var v int64
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	n.Value = &v
	return nil
}

type nullableStrings struct {
	Set   bool
	Value []string
}

func (n *nullableStrings) UnmarshalJSON(b []byte) error {
	n.Set = true
	if string(b) == "null" {
		n.Value = nil
		return nil
	}
	return json.Unmarshal(b, &n.Value)
}

type nullableTime struct {
	Set   bool
	Value *time.Time
}

func (n *nullableTime) UnmarshalJSON(b []byte) error {
	n.Set = true
	if string(b) == "null" {
		n.Value = nil
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return err
	}
	n.Value = &t
	return nil
}

// ---- 管理员：给用户分配额度 ----

type updateQuotaReq struct {
	MonthlyTokenLimit nullableInt64   `json:"monthly_token_limit"`
	AllowedModels     nullableStrings `json:"allowed_models"`
	QuotaExpiresAt    nullableTime    `json:"quota_expires_at"`
}

// handleUpdateUserQuota 只改传了的字段：显式 null 表示不限 / 全部模型 / 永不过期。
func (s *Server) handleUpdateUserQuota(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req updateQuotaReq
	if !readJSON(w, r, &req) {
		return
	}
	if !req.MonthlyTokenLimit.Set && !req.AllowedModels.Set && !req.QuotaExpiresAt.Set {
		writeErrCode(w, http.StatusBadRequest, codeQuotaNothingToUpdate, nil, "没有要修改的额度字段")
		return
	}
	u, err := s.store.GetUser(r.Context(), id)
	if notFound(err) {
		writeErrCode(w, http.StatusNotFound, codeUserNotFound, nil, "用户不存在")
		return
	}
	if err != nil {
		s.internalErr(w, r, err)
		return
	}

	q := store.UserQuotaPatch{
		MonthlyTokenLimit: u.MonthlyTokenLimit,
		AllowedModels:     u.AllowedModels,
		QuotaExpiresAt:    u.QuotaExpiresAt,
	}
	if req.MonthlyTokenLimit.Set {
		if req.MonthlyTokenLimit.Value != nil && *req.MonthlyTokenLimit.Value < 0 {
			writeErrCode(w, http.StatusBadRequest, codeQuotaInvalidLimit, nil, "monthly_token_limit 不能为负数；0 表示不分配额度，null 表示不限额")
			return
		}
		q.MonthlyTokenLimit = req.MonthlyTokenLimit.Value
	}
	if req.AllowedModels.Set {
		q.AllowedModels = cleanModels(req.AllowedModels.Value)
	}
	if req.QuotaExpiresAt.Set {
		if req.QuotaExpiresAt.Value != nil {
			t := req.QuotaExpiresAt.Value.UTC()
			if !t.After(time.Now()) {
				writeErrCode(w, http.StatusBadRequest, codeQuotaInvalidExpiry, nil, "quota_expires_at 必须是将来的时间")
				return
			}
			q.QuotaExpiresAt = &t
		} else {
			q.QuotaExpiresAt = nil
		}
	}
	if err := s.store.SetUserQuota(r.Context(), id, q); err != nil {
		if notFound(err) {
			writeErrCode(w, http.StatusNotFound, codeUserNotFound, nil, "用户不存在")
			return
		}
		s.internalErr(w, r, err)
		return
	}
	updated, err := s.store.GetUser(r.Context(), id)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	s.audit(r, auditUserQuota, updated.Username, map[string]any{"id": id, "monthly_token_limit": updated.MonthlyTokenLimit, "allowed_models": updated.AllowedModels, "expires_at": updated.QuotaExpiresAt})
	writeJSON(w, http.StatusOK, updated)
}

// ---- 员工：额度 + 自助创建 / 吊销自己的 key ----

type myQuota struct {
	MonthlyTokenLimit *int64     `json:"monthly_token_limit"`
	MonthTokens       int64      `json:"month_tokens"`
	AllowedModels     []string   `json:"allowed_models"`
	ExpiresAt         *time.Time `json:"expires_at"`
	ActiveKeys        int        `json:"active_keys"`
	MaxKeys           int        `json:"max_keys"`
}

func (s *Server) handleMyKeys(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	keys, err := s.store.ListAPIKeys(r.Context(), u.ID)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	used, err := s.store.MonthTokensByUser(r.Context(), u.ID)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	active, err := s.store.CountActiveKeys(r.Context(), u.ID)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"keys": nonNil(keys),
		"quota": myQuota{
			MonthlyTokenLimit: u.MonthlyTokenLimit,
			MonthTokens:       used,
			AllowedModels:     nonNil(u.AllowedModels),
			ExpiresAt:         u.QuotaExpiresAt,
			ActiveKeys:        active,
			MaxKeys:           maxKeysPerUser,
		},
	})
}

type createMyKeyReq struct {
	Name      string     `json:"name"`
	ExpiresAt *time.Time `json:"expires_at"`
}

func (s *Server) handleCreateMyKey(w http.ResponseWriter, r *http.Request) {
	if !s.requireFullSession(w, r) {
		return
	}
	var req createMyKeyReq
	if !readJSON(w, r, &req) {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeErrCode(w, http.StatusBadRequest, codeKeyInvalidName, nil, "name 不能为空")
		return
	}
	if len([]rune(req.Name)) > 128 {
		writeErrCode(w, http.StatusBadRequest, codeKeyNameTooLong, nil, "name 不能超过 128 个字符")
		return
	}
	if req.ExpiresAt != nil {
		t := req.ExpiresAt.UTC()
		if !t.After(time.Now()) {
			writeErrCode(w, http.StatusBadRequest, codeKeyInvalidExpiry, nil, "expires_at 必须是将来的时间")
			return
		}
		req.ExpiresAt = &t
	}
	u := currentUser(r)
	n, err := s.store.CountActiveKeys(r.Context(), u.ID)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	if n >= maxKeysPerUser {
		writeErrCode(w, http.StatusBadRequest, codeKeyLimitReached, map[string]any{"max": maxKeysPerUser}, "最多同时保留 "+strconv.Itoa(maxKeysPerUser)+" 个有效 key，请先吊销不用的")
		return
	}

	plain := keyPrefix + secret.Token()
	k := &store.APIKey{
		UserID:    u.ID,
		Username:  u.Username,
		Name:      req.Name,
		Prefix:    plain[:len(keyPrefix)+6],
		ExpiresAt: req.ExpiresAt,
		CreatedBy: u.ID,
	}
	if err := s.store.CreateAPIKey(r.Context(), k, secret.Hash(plain)); err != nil {
		s.internalErr(w, r, err)
		return
	}
	s.audit(r, auditKeyCreate, u.Username, map[string]any{"id": k.ID, "name": k.Name, "prefix": k.Prefix, "self": true})
	// 明文只在这里返回一次
	writeJSON(w, http.StatusCreated, map[string]any{"key": plain, "api_key": k})
}

func (s *Server) handleRevokeMyKey(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	k, err := s.store.GetAPIKey(r.Context(), id)
	// 不是自己的 key 一律按不存在处理，不暴露别人的 key id
	if notFound(err) || (err == nil && k.UserID != currentUser(r).ID) {
		writeErrCode(w, http.StatusNotFound, codeKeyNotFound, nil, "key 不存在")
		return
	}
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	if err := s.store.RevokeAPIKey(r.Context(), id); err != nil {
		if notFound(err) {
			writeErrCode(w, http.StatusNotFound, codeKeyRevoked, nil, "key 已吊销")
			return
		}
		s.internalErr(w, r, err)
		return
	}
	s.audit(r, auditKeyRevoke, k.Username, map[string]any{"id": id, "name": k.Name, "prefix": k.Prefix, "self": true})
	w.WriteHeader(http.StatusNoContent)
}
