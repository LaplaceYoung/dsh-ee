package api

import (
	"context"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/store"
)

// 审计动作。前端按动作名翻译（audit.actions.*），新增动作时两边一起加。
const (
	auditLogin          = "auth.login"
	auditLoginFailed    = "auth.login_failed"
	auditPasswordChange = "auth.password_change"

	auditUserCreate = "user.create"
	auditUserUpdate = "user.update"
	auditUserRoles  = "user.roles"
	auditUserQuota  = "user.quota"

	auditRoleCreate       = "role.create"
	auditRoleUpdate       = "role.update"
	auditRoleDelete       = "role.delete"
	auditPermissionCreate = "permission.create"
	auditPermissionDelete = "permission.delete"
	auditMenuChange       = "menu.change"

	auditProviderCreate = "provider.create"
	auditProviderUpdate = "provider.update"
	auditProviderDelete = "provider.delete"
	auditModelChange    = "model.change"
	auditKeyCreate      = "key.create"
	auditKeyRevoke      = "key.revoke"

	auditSettingsUpdate = "settings.update"
	auditBrandUpdate    = "brand.update"
	auditLDAPUpdate     = "ldap.update"
	auditLDAPSync       = "ldap.sync"
	auditOIDCUpdate     = "oidc.update"

	auditPackagePublish         = "package.publish"
	auditPackageStatus          = "package.status"
	auditPackageEdit            = "package.edit"
	auditReviewApprove          = "review.approve"
	auditReviewReject           = "review.reject"
	auditImportRequest          = "package.import_request"
	auditUpstreamSync           = "upstream.sync"
	auditUpstreamImport         = "upstream.import"
	auditUpstreamExport         = "upstream.export"
	auditAssetCreate            = "asset.create"
	auditAssetVersion           = "asset.version"
	auditAssetEdit              = "asset.edit"
	auditAssetDelete            = "asset.delete"
	auditScopeRequest           = "asset.scope_request"
	auditConnTool               = "connection.tool"
	auditExtensionInstall       = "connection.extension_install"
	auditCredentialSet          = "connection.credential_set"
	auditCredentialDelete       = "connection.credential_delete"
	auditCredentialRevoke       = "connection.credential_revoke"
	auditEEJWTRotate            = "connection.ee_jwt_rotate"
	auditAppDataInsert          = "app_data.insert"
	auditAppDataUpdate          = "app_data.update"
	auditAppDataDelete          = "app_data.delete"
	auditAppDataExport          = "app_data.export"
	auditDataUpload             = "data.upload"
	auditDataDrop               = "data.drop"
	auditDataSetCreate          = "data.dataset_create"
	auditDataSetUpdate          = "data.dataset_update"
	auditDataSetDelete          = "data.dataset_delete"
	auditConnRequest            = "connection.request"
	auditAccessRequest          = "access.request"
	auditAppExport              = "app.export"
	auditAppImport              = "app.import"
	auditConnectPublish         = "connect.publish"
	auditDshReleaseMirrorFailed = "dsh_release.mirror_failed"
	auditDshReleaseCheck        = "dsh_release.check"
	auditDshReleaseMirror       = "dsh_release.mirror"
	auditDshReleaseRecommend    = "dsh_release.recommend"
	auditDshReleaseDelete       = "dsh_release.delete"

	auditNpmTokenCreate = "npm_token.create"
	auditNpmTokenRevoke = "npm_token.revoke"
)

// audit 以当前登录用户的身份记一条操作。写失败只记日志，不影响请求本身。
func (s *Server) audit(r *http.Request, action, target string, detail map[string]any) {
	s.auditAs(r, currentUser(r), action, target, detail)
}

// auditAs 指定操作人（npm 令牌、登录这类没有会话的请求）；u 为 nil 表示匿名。
func (s *Server) auditAs(r *http.Request, u *store.User, action, target string, detail map[string]any) {
	a := &store.AuditLog{Action: action, Target: target, Detail: detail, IP: clientIP(r)}
	if info, ok := agentFrom(r.Context()); ok {
		// 经 Agent 的操作：来源记 agent，并带上是哪个工具调的
		a.Via, a.Client = "agent", info.Principal.Client
		if info.Principal.TokenID > 0 {
			a.TokenID = &info.Principal.TokenID
		}
		a.Detail = map[string]any{"tool": info.Tool}
		for k, v := range detail {
			a.Detail[k] = v
		}
	} else if token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
		if p, err := s.resolveBearer(r.Context(), token, clientIP(r), r.Header.Get("X-DshEE-Client"), r.Header.Get("X-DshEE-Host")); err == nil {
			a.Via = "token"
			a.Client = p.Client
			if p.TokenID > 0 {
				a.TokenID = &p.TokenID
			}
		}
	}
	if u != nil {
		id := u.ID
		a.ActorID, a.ActorName = &id, u.Username
	}
	// 请求可能已经结束（客户端断开），记录不随之取消
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
	defer cancel()
	if err := s.store.InsertAudit(ctx, a); err != nil {
		s.log.Warn("写审计日志失败", "action", action, "err", err)
	}
}

// auditSystem 记录后台任务（如目录同步）的操作，没有请求。
func (s *Server) auditSystem(ctx context.Context, action, target string, detail map[string]any) {
	if err := s.store.InsertAudit(context.WithoutCancel(ctx), &store.AuditLog{Action: action, Target: target, Detail: detail, Via: "system"}); err != nil {
		s.log.Warn("写审计日志失败", "action", action, "err", err)
	}
}

// clientIP 取对端地址。对端是本机或内网地址（即经过反向代理）时，取 X-Forwarded-For 的最后一跳：
// 那是代理自己追加的、它看到的客户端地址；前面几跳客户端可以随便伪造。
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	if ip != nil && (ip.IsLoopback() || ip.IsPrivate()) {
		if xff := r.Header.Values("X-Forwarded-For"); len(xff) > 0 {
			hops := strings.Split(xff[len(xff)-1], ",")
			if last := strings.TrimSpace(hops[len(hops)-1]); net.ParseIP(last) != nil {
				return last
			}
		}
	}
	return host
}

// GET /api/admin/audit?before=&limit=&action=&actor=&q=&since=&until=
func (s *Server) handleListAudit(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := store.AuditFilter{
		Action: strings.TrimSpace(q.Get("action")),
		Query:  strings.TrimSpace(q.Get("q")),
		Via:    strings.TrimSpace(q.Get("via")),
	}
	f.Before, _ = strconv.ParseInt(q.Get("before"), 10, 64)
	f.ActorID, _ = strconv.ParseInt(q.Get("actor"), 10, 64)
	f.Limit, _ = strconv.Atoi(q.Get("limit"))
	for key, dst := range map[string]*time.Time{"since": &f.Since, "until": &f.Until} {
		if v := q.Get(key); v != "" {
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				writeErr(w, http.StatusBadRequest, key+" 必须是 RFC 3339 时间")
				return
			}
			*dst = t.UTC()
		}
	}
	if q := []rune(f.Query); len(q) > 100 {
		f.Query = string(q[:100])
	}
	logs, err := s.store.ListAudit(r.Context(), f)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"logs": logs})
}
