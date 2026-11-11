package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/hikariming/dsh-ee/server/internal/secret"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

func (s *Server) sharedConnectionSpec(ctx context.Context, a *store.Asset) (*assetSpec, error) {
	version, err := s.store.LiveAssetVersion(ctx, a.ID)
	if err != nil {
		return nil, err
	}
	return parseSpec(version.Spec), nil
}

// GET /api/admin/connections/{id}/shared-credentials：只列命名、尾号和时间，不回密文。
func (s *Server) handleListSharedCredentials(w http.ResponseWriter, r *http.Request) {
	a, ok := s.loadConnection(w, r)
	if !ok {
		return
	}
	items, err := s.store.ListSharedCredentials(r.Context(), a.ID)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	sp, err := s.sharedConnectionSpec(r.Context(), a)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"default_name": sp.Auth.SharedName,
		"legacy_hint":  a.SecretHint,
		"credentials":  nonNil(items),
		"count":        len(items),
	})
}

type sharedCredentialReq struct {
	Secret string `json:"secret"`
}

// PUT /api/admin/connections/{id}/shared-credentials/{name}：新增或替换命名共享凭据。
func (s *Server) handlePutSharedCredential(w http.ResponseWriter, r *http.Request) {
	a, ok := s.loadConnection(w, r)
	if !ok {
		return
	}
	name := strings.TrimSpace(r.PathValue("name"))
	if !sharedCredentialNameRe.MatchString(name) {
		writeErr(w, http.StatusBadRequest, "共享凭据名称只能包含字母、数字、下划线和中划线，最长 40 个字符")
		return
	}
	var req sharedCredentialReq
	if !readJSON(w, r, &req) {
		return
	}
	plain := strings.TrimSpace(req.Secret)
	if plain == "" || len(plain) > 4096 {
		writeErr(w, http.StatusBadRequest, "凭据必填，且不能超过 4096 字节；删除请用 DELETE")
		return
	}
	sp, err := s.sharedConnectionSpec(r.Context(), a)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	if sp.Auth.Type == "none" {
		writeErr(w, http.StatusBadRequest, "请先选择共享凭据的认证方式，再添加命名凭据")
		return
	}
	sealed, err := s.box.Seal([]byte(plain))
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	if err := s.store.SaveSharedCredential(r.Context(), &store.SharedCredential{AssetID: a.ID, Name: name, SecretEnc: sealed, SecretHint: secret.Hint(plain)}); err != nil {
		s.internalErr(w, r, err)
		return
	}
	s.dropConnClient(a.ID)
	s.audit(r, auditSharedCredentialSet, a.Slug+"/"+name, map[string]any{"asset_id": a.ID, "shared_name": name})
	writeJSON(w, http.StatusOK, map[string]any{"asset_id": a.ID, "name": name, "secret_hint": secret.Hint(plain)})
}

// DELETE /api/admin/connections/{id}/shared-credentials/{name}：删除命名共享凭据，当前正在使用的拒绝删除。
func (s *Server) handleDeleteSharedCredential(w http.ResponseWriter, r *http.Request) {
	a, ok := s.loadConnection(w, r)
	if !ok {
		return
	}
	name := strings.TrimSpace(r.PathValue("name"))
	if !sharedCredentialNameRe.MatchString(name) {
		writeErr(w, http.StatusBadRequest, "共享凭据名称不合法")
		return
	}
	sp, err := s.sharedConnectionSpec(r.Context(), a)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	if sp.Auth.SharedName == name {
		writeErr(w, http.StatusConflict, "这个连接正在使用命名凭据 "+name+"，请先改选默认或其它凭据，再删除")
		return
	}
	if err := s.store.DeleteSharedCredential(r.Context(), a.ID, name); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "命名凭据不存在")
			return
		}
		s.internalErr(w, r, err)
		return
	}
	s.dropConnClient(a.ID)
	s.audit(r, auditSharedCredentialDelete, a.Slug+"/"+name, map[string]any{"asset_id": a.ID, "shared_name": name})
	w.WriteHeader(http.StatusNoContent)
}
