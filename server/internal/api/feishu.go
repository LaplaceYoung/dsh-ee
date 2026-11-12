package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/feishu"
	"github.com/hikariming/dsh-ee/server/internal/oidc"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

const feishuSyncLastKey = "feishu_sync_last"

type feishuSyncResult struct {
	At         time.Time `json:"at"`
	OK         bool      `json:"ok"`
	Error      string    `json:"error,omitempty"`
	Remote     int       `json:"remote"`
	Created    int       `json:"created"`
	Reconciled int       `json:"reconciled"`
	Disabled   []string  `json:"disabled"`
	Conflicts  []string  `json:"conflicts"`
	Skipped    []string  `json:"skipped"`
	// 目录里没看到、但飞书没有明确说离职或冻结的账号：没有禁用，列出来给管理员确认
	Kept []string `json:"kept"`
}

func (s *Server) feishuConfig(ctx context.Context) (*oidc.Config, error) {
	cfg, err := s.loadOIDC(ctx)
	if err != nil {
		return nil, err
	}
	if !cfg.Enabled || cfg.Preset != "feishu" {
		return nil, errors.New("请先选择飞书并开启统一登录")
	}
	if cfg.FeishuAPIBase == "" {
		return nil, errors.New("请先填写飞书通讯录 API 地址")
	}
	if cfg.ClientSecret == "" {
		return nil, errors.New("组织同步需要 app secret（client secret）；它只用于获取 tenant token，不会展示给员工")
	}
	return cfg, nil
}

func externalDeptNames(dir *feishu.Directory, ids []string) ([]string, []string) {
	var names []string
	groups := append([]string(nil), ids...)
	for _, id := range ids {
		if name := strings.TrimSpace(dir.Departments[id]); name != "" {
			names = append(names, name)
			groups = append(groups, name)
		}
	}
	return names, groups
}

func (s *Server) syncFeishu(ctx context.Context) (*feishuSyncResult, error) {
	res := &feishuSyncResult{At: time.Now().UTC(), Disabled: []string{}, Conflicts: []string{}, Skipped: []string{}, Kept: []string{}}
	finish := func(err error) (*feishuSyncResult, error) {
		if err != nil {
			res.Error = err.Error()
		} else {
			res.OK = true
		}
		if storeErr := s.store.PutSettingJSON(ctx, feishuSyncLastKey, res, 0); storeErr != nil {
			return nil, storeErr
		}
		return res, nil
	}
	cfg, err := s.feishuConfig(ctx)
	if err != nil {
		return finish(err)
	}
	client := feishu.NewClient(cfg.FeishuAPIBase, cfg.ClientID, cfg.ClientSecret, s.oidcHTTP().HTTP)
	dir, err := client.Directory(ctx)
	if err != nil {
		return finish(err)
	}
	if len(dir.Users) == 0 {
		return finish(errors.New("通讯录没有员工；为避免误更新或误禁用整批账号，本轮不生效"))
	}
	res.Remote = len(dir.Users)
	seen := map[string]bool{}
	// 目录里明确标了冻结 / 离职的
	gone := map[string]bool{}
	for _, remote := range dir.Users {
		subject := feishu.ExternalSubject(remote.OpenID)
		// 目录明确标记冻结/离职也不是「仍然存在」，进入禁用流程（仍需 SyncDisable）。
		seen[subject] = !remote.Disabled()
		gone[subject] = remote.Disabled()
		if remote.Disabled() {
			res.Skipped = append(res.Skipped, remote.OpenID)
			continue
		}
		username := oidcUsername(remote.Email)
		if username == "" {
			username = oidcUsername(remote.EmployeeNo)
		}
		if username == "" {
			username = oidcUsername(remote.OpenID)
		}
		if username == "" {
			res.Conflicts = append(res.Conflicts, remote.OpenID+"（没有可用的用户名）")
			continue
		}
		var previous *store.User
		if existing, lookupErr := s.store.GetUserByExternalDN(ctx, store.AuthOIDC, subject); lookupErr == nil {
			previous = existing
			if previous.Disabled {
				res.Skipped = append(res.Skipped, username+"（本地已禁用，不自动恢复）")
				continue
			}
		} else if !notFound(lookupErr) {
			return finish(lookupErr)
		}
		if previous == nil && !cfg.AutoCreate {
			res.Skipped = append(res.Skipped, username+"（未预先开通）")
			continue
		}
		departments, groups := externalDeptNames(dir, remote.DepartmentIDs)
		identity := oidc.Identity{Subject: subject, Username: username, DisplayName: remote.Name, Email: remote.Email,
			Department: strings.Join(departments, " / "), Groups: groups}
		_, err := s.provisionOIDC(ctx, cfg, identity)
		switch {
		case errors.Is(err, errOIDCConflict), errors.Is(err, errOIDCIdentity):
			res.Conflicts = append(res.Conflicts, username)
		case errors.Is(err, errBadCredentials), errors.Is(err, errNotProvisioned):
			res.Skipped = append(res.Skipped, username)
		case err != nil:
			return finish(err)
		case previous == nil:
			res.Created++
		default:
			res.Reconciled++
		}
	}
	if cfg.FeishuSyncDisable {
		existing, err := s.store.ListUsersBySource(ctx, store.AuthOIDC)
		if err != nil {
			return finish(err)
		}
		for _, u := range existing {
			// 已经禁用的不用每一轮再禁一次、再记一条审计
			if u.Disabled || !strings.HasPrefix(u.ExternalDN, "feishu:") || seen[u.ExternalDN] {
				continue
			}
			// 目录里没看到不等于离职：应用的通讯录权限范围可能只覆盖一部分员工。
			// 单独向飞书确认，只有飞书明确说冻结 / 离职才禁用；看不到、查不到的留着，列给管理员。
			if !gone[u.ExternalDN] {
				state, err := client.UserState(ctx, feishuOpenID(u))
				if err != nil {
					return finish(err)
				}
				if state != feishu.UserGone {
					why := map[feishu.UserState]string{
						feishu.UserActive:     "飞书里仍在职，但不在这次拉到的目录里",
						feishu.UserOutOfScope: "不在飞书应用的通讯录权限范围内，看不到状态",
						feishu.UserUnknown:    "飞书查不到这个账号",
					}[state]
					res.Kept = append(res.Kept, u.Username+"（"+why+"）")
					continue
				}
			}
			yes := true
			if err := s.store.UpdateUser(ctx, u.ID, store.UserPatch{Disabled: &yes}); err != nil {
				return finish(err)
			}
			if err := s.store.DeleteUserSessions(ctx, u.ID); err != nil {
				return finish(err)
			}
			s.purgeUserCredentials(ctx, u.ID)
			res.Disabled = append(res.Disabled, u.Username)
			s.auditSystem(ctx, auditUserUpdate, u.Username, map[string]any{"disabled": true, "reason": "feishu_missing"})
		}
	}
	s.auditSystem(ctx, auditFeishuSync, "", map[string]any{"remote": res.Remote, "created": res.Created, "reconciled": res.Reconciled, "disabled": len(res.Disabled), "kept": len(res.Kept), "conflicts": len(res.Conflicts)})
	s.feishuNotifyKBGaps(ctx)
	return finish(nil)
}

// POST /api/admin/auth/feishu/sync：手动完整同步一次组织资料和角色映射；只有目录缺失才按配置禁用。
func (s *Server) handleSyncFeishu(w http.ResponseWriter, r *http.Request) {
	res, err := s.syncFeishu(r.Context())
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	s.audit(r, auditFeishuSync, "", map[string]any{"manual": true, "ok": res.OK, "remote": res.Remote, "created": res.Created, "reconciled": res.Reconciled, "disabled": len(res.Disabled), "conflicts": len(res.Conflicts)})
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleFeishuSyncStatus(w http.ResponseWriter, r *http.Request) {
	var result *feishuSyncResult
	if _, err := s.store.GetSettingJSON(r.Context(), feishuSyncLastKey, &result); err != nil {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"last": result})
}

func (s *Server) runFeishuSync(ctx context.Context) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			cfg, err := s.loadOIDC(ctx)
			if err != nil || !cfg.Enabled || cfg.Preset != "feishu" || !cfg.FeishuSync {
				continue
			}
			result, err := s.syncFeishu(ctx)
			if err != nil {
				s.log.Warn("飞书组织同步失败", "err", err)
			} else if !result.OK {
				s.log.Warn("飞书组织同步未完成", "err", result.Error)
			}
		}
	}
}
