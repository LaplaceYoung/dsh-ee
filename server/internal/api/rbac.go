package api

import (
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/hikariming/dsh-ee/server/internal/rbac"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

var (
	roleCodeRe = regexp.MustCompile(`^[a-z][a-z0-9_.-]{1,63}$`)
	permCodeRe = regexp.MustCompile(`^[a-z][a-z0-9_.:-]{1,63}$`)
	pageKeyRe  = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)
	pathRe     = regexp.MustCompile(`^/[a-z0-9\-_/]*$`)
	iconRe     = regexp.MustCompile(`^[A-Za-z0-9]{1,64}$`)
)

// 前端固定占用、不能配置成菜单的路由。
var reservedPaths = []string{"/", "/login", "/go", "/start", "/get-started", "/account", "/plugins"}

func dedupe[T comparable](xs []T) []T {
	seen := map[T]bool{}
	out := []T{}
	for _, x := range xs {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

func errMsg(err error) string {
	_, msg, _ := strings.Cut(err.Error(), ": ")
	return msg
}

// subset 判断 target 的权限是否都在 actor 手里。
func subset(actor *rbac.Access, perms []string) bool {
	if actor.Super {
		return true
	}
	for _, p := range perms {
		if !actor.Perms[p] {
			return false
		}
	}
	return true
}

// outranks：actor 是超级管理员，或目标不是超级管理员且权限是 actor 的子集。
// 用来阻止低权限管理员禁用、重置或改动比自己权限大的人。
func outranks(actor, target *rbac.Access) bool {
	if actor.Super {
		return true
	}
	if target.Super {
		return false
	}
	for p := range target.Perms {
		if !actor.Perms[p] {
			return false
		}
	}
	return true
}

// codedErr 是一次待写回的拒绝：机器码 + 插值参数 + 中文默认文案。给「先做校验、
// 再由调用方决定状态码并写回」的辅助函数用（见 checkGrantable）。
type codedErr struct {
	code   string
	params map[string]any
	msg    string
}

// checkGrantable 校验当前用户能否把这些角色授予别人：角色必须存在；
// 超级管理员角色只有超级管理员能授；其它角色的权限必须是自己权限的子集。
func (s *Server) checkGrantable(r *http.Request, roleIDs []int64) (*codedErr, bool) {
	actor := currentAccess(r)
	for _, id := range roleIDs {
		role, err := s.store.GetRole(r.Context(), id)
		if err != nil {
			return &codedErr{code: codeRoleUnknown, params: map[string]any{"id": id}, msg: "角色不存在：" + itoa64(id)}, false
		}
		if role.IsSuper && !actor.Super {
			return &codedErr{code: codeRoleGrantSuperForbidden, params: map[string]any{"role": role.Name}, msg: "只有超级管理员能授予「" + role.Name + "」"}, false
		}
		if !subset(actor, role.Permissions) {
			return &codedErr{code: codeRoleGrantOverreach, params: map[string]any{"role": role.Name}, msg: "不能授予超出自己权限的角色「" + role.Name + "」"}, false
		}
	}
	return nil, true
}

// ---- 用户的角色 ----

type setRolesReq struct {
	RoleIDs []int64 `json:"role_ids"`
}

func (s *Server) handleSetUserRoles(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req setRolesReq
	if !readJSON(w, r, &req) {
		return
	}
	if _, err := s.store.GetUser(r.Context(), id); err != nil {
		if notFound(err) {
			writeErrCode(w, http.StatusNotFound, codeUserNotFound, nil, "用户不存在")
			return
		}
		s.internalErr(w, r, err)
		return
	}
	target, err := s.store.UserAccess(r.Context(), id)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	if !outranks(currentAccess(r), target) {
		writeErrCode(w, http.StatusForbidden, codeUserOutrankedRoles, nil, "不能修改权限比自己大的用户")
		return
	}
	ids := dedupe(req.RoleIDs)
	if e, ok := s.checkGrantable(r, ids); !ok {
		writeErrCode(w, http.StatusForbidden, e.code, e.params, e.msg)
		return
	}
	// 管理约束：不能给自己授权、互斥角色、双人批准（admin_policy.go）
	policy := s.adminPolicy(r.Context())
	msg, grantsAdmin, err := s.roleChangeProblem(r.Context(), policy, currentUser(r).ID, id, target, ids)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	if msg != "" {
		writeErr(w, http.StatusForbidden, msg)
		return
	}
	if grantsAdmin && policy.dual(DualRolesAssign) {
		c, err := s.submitRolesChange(r, id, ids)
		if err != nil {
			s.internalErr(w, r, err)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"change_request": c, "message": "开了双人批准：授管理角色要另一位管理员在「变更请求」里批准后才生效"})
		return
	}
	// 拿掉超级管理员时，确认还剩至少一个启用中的超级管理员
	if target.Super {
		stillSuper := false
		for _, rid := range ids {
			if role, err := s.store.GetRole(r.Context(), rid); err == nil && role.IsSuper {
				stillSuper = true
			}
		}
		if !stillSuper {
			n, err := s.store.CountSuperUsers(r.Context(), id)
			if err != nil {
				s.internalErr(w, r, err)
				return
			}
			if n == 0 {
				writeErrCode(w, http.StatusBadRequest, codeUserLastSuperAdminUnassign, nil, "这是最后一个启用中的超级管理员，不能取消")
				return
			}
		}
	}
	if err := s.store.SetUserRoles(r.Context(), id, ids); err != nil {
		s.internalErr(w, r, err)
		return
	}
	refs, err := s.store.UserRoleRefs(r.Context())
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	s.audit(r, auditUserRoles, s.usernameOf(r, id), map[string]any{"id": id, "roles": roleNames(refs[id])})
	s.hub.notify() // 角色变了，他的 Agent 工具可能随之增减
	s.hub.notify()
	writeJSON(w, http.StatusOK, map[string]any{"roles": nonNil(refs[id])})
}

// ---- 权限点 ----

func (s *Server) handleListPermissions(w http.ResponseWriter, r *http.Request) {
	ps, err := s.store.ListPermissions(r.Context())
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"permissions": nonNil(ps)})
}

type permissionReq struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Group       string `json:"group"`
	Description string `json:"description"`
}

func (s *Server) handleCreatePermission(w http.ResponseWriter, r *http.Request) {
	var req permissionReq
	if !readJSON(w, r, &req) {
		return
	}
	p := &store.Permission{
		Code:        strings.TrimSpace(req.Code),
		Name:        strings.TrimSpace(req.Name),
		Group:       strings.TrimSpace(req.Group),
		Description: strings.TrimSpace(req.Description),
	}
	if !permCodeRe.MatchString(p.Code) {
		writeErrCode(w, http.StatusBadRequest, codePermInvalidCode, nil, "权限点代码以小写字母开头，只含小写字母、数字和 . _ : -，2 到 64 个字符")
		return
	}
	if p.Name == "" || len([]rune(p.Name)) > 128 {
		writeErrCode(w, http.StatusBadRequest, codePermInvalidName, nil, "名称不能为空，且不超过 128 个字符")
		return
	}
	if p.Group == "" {
		p.Group = "自定义"
	}
	if err := s.store.CreatePermission(r.Context(), p); err != nil {
		if errors.Is(err, store.ErrDuplicate) {
			writeErrCode(w, http.StatusConflict, codePermExists, nil, "权限点代码已存在")
			return
		}
		s.internalErr(w, r, err)
		return
	}
	s.audit(r, auditPermissionCreate, p.Code, map[string]any{"name": p.Name})
	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) handleDeletePermission(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	if err := s.store.DeletePermission(r.Context(), code); err != nil {
		switch {
		case notFound(err):
			writeErrCode(w, http.StatusNotFound, codePermNotFound, nil, "权限点不存在")
		case errors.Is(err, store.ErrInUse):
			writeErrCode(w, http.StatusConflict, codePermInUse, nil, errMsg(err))
		default:
			s.internalErr(w, r, err)
		}
		return
	}
	s.audit(r, auditPermissionDelete, code, nil)
	w.WriteHeader(http.StatusNoContent)
}

// ---- 角色 ----

func (s *Server) handleListRoles(w http.ResponseWriter, r *http.Request) {
	roles, err := s.store.ListRoles(r.Context())
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"roles": nonNil(roles)})
}

// validPermissions 去重并确认权限点都存在，同时不允许授予自己没有的权限。
func (s *Server) validPermissions(w http.ResponseWriter, r *http.Request, codes []string) ([]string, bool) {
	codes = dedupe(codes)
	all, err := s.store.ListPermissions(r.Context())
	if err != nil {
		s.internalErr(w, r, err)
		return nil, false
	}
	known := map[string]bool{}
	for _, p := range all {
		known[p.Code] = true
	}
	for _, c := range codes {
		if !known[c] {
			writeErrCode(w, http.StatusBadRequest, codePermUnknown, map[string]any{"code": c}, "权限点不存在："+c)
			return nil, false
		}
	}
	if !subset(currentAccess(r), codes) {
		writeErrCode(w, http.StatusForbidden, codePermGrantOverreach, nil, "不能授予自己没有的权限")
		return nil, false
	}
	return codes, true
}

type createRoleReq struct {
	Code        string   `json:"code"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
}

func (s *Server) handleCreateRole(w http.ResponseWriter, r *http.Request) {
	if !s.guardDual(w, r) {
		return
	}
	var req createRoleReq
	if !readJSON(w, r, &req) {
		return
	}
	role := &store.Role{Code: strings.TrimSpace(req.Code), Name: strings.TrimSpace(req.Name), Description: strings.TrimSpace(req.Description)}
	if !roleCodeRe.MatchString(role.Code) {
		writeErrCode(w, http.StatusBadRequest, codeRoleInvalidCode, nil, "角色代码以小写字母开头，只含小写字母、数字和 . _ -，2 到 64 个字符")
		return
	}
	if role.Name == "" || len([]rune(role.Name)) > 128 {
		writeErrCode(w, http.StatusBadRequest, codeRoleInvalidName, nil, "名称不能为空，且不超过 128 个字符")
		return
	}
	perms, ok := s.validPermissions(w, r, req.Permissions)
	if !ok {
		return
	}
	role.Permissions = perms
	if err := s.store.CreateRole(r.Context(), role); err != nil {
		if errors.Is(err, store.ErrDuplicate) {
			writeErrCode(w, http.StatusConflict, codeRoleExists, nil, "角色代码已存在")
			return
		}
		s.internalErr(w, r, err)
		return
	}
	s.audit(r, auditRoleCreate, role.Name, map[string]any{"id": role.ID, "code": role.Code, "permissions": role.Permissions})
	writeJSON(w, http.StatusCreated, role)
}

type updateRoleReq struct {
	Name        *string   `json:"name"`
	Description *string   `json:"description"`
	Permissions *[]string `json:"permissions"`
}

func (s *Server) handleUpdateRole(w http.ResponseWriter, r *http.Request) {
	if !s.guardDual(w, r) {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req updateRoleReq
	if !readJSON(w, r, &req) {
		return
	}
	role, err := s.store.GetRole(r.Context(), id)
	if notFound(err) {
		writeErrCode(w, http.StatusNotFound, codeRoleNotFound, nil, "角色不存在")
		return
	}
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	actor := currentAccess(r)
	if role.IsSuper && !actor.Super {
		writeErrCode(w, http.StatusForbidden, codeRoleSuperModifyForbidden, nil, "只有超级管理员能修改超级管理员角色")
		return
	}
	// 改一个自己权限覆盖不了的角色，等于替别人做决定，同样拒绝
	if !subset(actor, role.Permissions) {
		writeErrCode(w, http.StatusForbidden, codeRoleOutrankedUpdate, nil, "这个角色有你没有的权限，不能修改")
		return
	}
	p := store.RolePatch{}
	if req.Name != nil {
		n := strings.TrimSpace(*req.Name)
		if n == "" || len([]rune(n)) > 128 {
			writeErrCode(w, http.StatusBadRequest, codeRoleInvalidName, nil, "名称不能为空，且不超过 128 个字符")
			return
		}
		p.Name = &n
	}
	if req.Description != nil {
		d := strings.TrimSpace(*req.Description)
		p.Description = &d
	}
	if req.Permissions != nil {
		if role.IsSuper {
			writeErrCode(w, http.StatusBadRequest, codeRoleSuperPermissionsImplicit, nil, "超级管理员自动拥有全部权限，不需要单独配置")
			return
		}
		perms, ok := s.validPermissions(w, r, *req.Permissions)
		if !ok {
			return
		}
		p.Permissions = perms
	}
	if err := s.store.UpdateRole(r.Context(), id, p); err != nil {
		s.internalErr(w, r, err)
		return
	}
	updated, err := s.store.GetRole(r.Context(), id)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	s.audit(r, auditRoleUpdate, updated.Name, map[string]any{"id": id, "code": updated.Code, "permissions": updated.Permissions})
	s.hub.notify()
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) handleDeleteRole(w http.ResponseWriter, r *http.Request) {
	if !s.guardDual(w, r) {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	role, err := s.store.GetRole(r.Context(), id)
	if notFound(err) {
		writeErrCode(w, http.StatusNotFound, codeRoleNotFound, nil, "角色不存在")
		return
	}
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	if !subset(currentAccess(r), role.Permissions) {
		writeErrCode(w, http.StatusForbidden, codeRoleOutrankedDelete, nil, "这个角色有你没有的权限，不能删除")
		return
	}
	if err := s.store.DeleteRole(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrInUse) {
			writeErrCode(w, http.StatusConflict, codeRoleInUse, nil, errMsg(err))
			return
		}
		s.internalErr(w, r, err)
		return
	}
	s.audit(r, auditRoleDelete, role.Name, map[string]any{"id": id, "code": role.Code})
	s.hub.notify()
	w.WriteHeader(http.StatusNoContent)
}

// ---- 菜单 ----

// handleMyMenus 返回当前用户能访问的菜单（含隐藏页面，前端要为它们建路由）。
// 分组只有在自己有权限、且至少有一个可访问的子项时才返回。
func (s *Server) handleMyMenus(w http.ResponseWriter, r *http.Request) {
	all, err := s.store.ListMenus(r.Context())
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	a := currentAccess(r)
	allowed := func(m *store.Menu) bool { return m.Permission == "" || a.Has(m.Permission) }
	groups := map[int64]*store.Menu{}
	for _, m := range all {
		if m.Kind == "group" {
			groups[m.ID] = m
		}
	}
	hasChild := map[int64]bool{}
	var leaves []*store.Menu
	for _, m := range all {
		if m.Kind == "group" || !allowed(m) {
			continue
		}
		if m.ParentID != nil {
			g := groups[*m.ParentID]
			if g == nil || !allowed(g) {
				continue
			}
			hasChild[g.ID] = true
		}
		leaves = append(leaves, m)
	}
	out := []*store.Menu{}
	for _, m := range all {
		if m.Kind == "group" {
			if hasChild[m.ID] {
				out = append(out, m)
			}
		} else if slices.Contains(leaves, m) {
			out = append(out, m)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"menus": out})
}

func (s *Server) handleListMenus(w http.ResponseWriter, r *http.Request) {
	ms, err := s.store.ListMenus(r.Context())
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"menus": nonNil(ms)})
}

type menuReq struct {
	ParentID   *int64 `json:"parent_id"`
	Kind       string `json:"kind"`
	Title      string `json:"title"`
	Path       string `json:"path"`
	PageKey    string `json:"page_key"`
	Icon       string `json:"icon"`
	Permission string `json:"permission"`
	Sort       int    `json:"sort"`
	Visible    *bool  `json:"visible"`
}

// validateMenu 校验并规范化一项菜单；selfID 为 0 表示新建。
func (s *Server) validateMenu(w http.ResponseWriter, r *http.Request, req menuReq, selfID int64) (*store.Menu, bool) {
	bad := func(code string, params map[string]any, msg string) (*store.Menu, bool) {
		writeErrCode(w, http.StatusBadRequest, code, params, msg)
		return nil, false
	}
	m := &store.Menu{
		ID: selfID, ParentID: req.ParentID, Kind: req.Kind, Title: strings.TrimSpace(req.Title),
		Path: strings.TrimSpace(req.Path), PageKey: strings.TrimSpace(req.PageKey), Icon: strings.TrimSpace(req.Icon),
		Permission: strings.TrimSpace(req.Permission), Sort: req.Sort, Visible: true,
	}
	if req.Visible != nil {
		m.Visible = *req.Visible
	}
	if m.Title == "" || len([]rune(m.Title)) > 64 {
		return bad(codeMenuInvalidTitle, nil, "标题不能为空，且不超过 64 个字符")
	}
	if m.Icon != "" && !iconRe.MatchString(m.Icon) {
		return bad(codeMenuInvalidIcon, nil, "图标名只能包含字母和数字")
	}
	all, err := s.store.ListMenus(r.Context())
	if err != nil {
		s.internalErr(w, r, err)
		return nil, false
	}
	switch m.Kind {
	case "group":
		m.Path, m.PageKey = "", ""
		if m.ParentID != nil {
			return bad(codeMenuGroupNotTop, nil, "分组只能放在顶层")
		}
		if selfID != 0 {
			for _, x := range all {
				if x.ID == selfID && x.Kind != "group" {
					for _, y := range all {
						if y.ParentID != nil && *y.ParentID == selfID {
							return bad(codeMenuGroupHasChildren, nil, "有子项的菜单不能改成分组")
						}
					}
				}
			}
		}
	case "page":
		if m.Path != "/" {
			m.Path = strings.TrimRight(m.Path, "/")
		}
		if !pathRe.MatchString(m.Path) || strings.Contains(m.Path, "//") {
			return bad(codeMenuInvalidPath, nil, "路由以 / 开头，只能包含小写字母、数字和 - _ /")
		}
		for _, rp := range reservedPaths {
			if m.Path == rp || (rp != "/" && strings.HasPrefix(m.Path, rp+"/")) {
				return bad(codeMenuReservedPath, map[string]any{"path": rp}, "路由 "+rp+" 是系统保留的，换一个")
			}
		}
		if !pageKeyRe.MatchString(m.PageKey) {
			return bad(codeMenuMissingPageKey, nil, "请选择页面组件（page_key）")
		}
		for _, x := range all {
			if x.ID != selfID && x.Kind == "page" && x.Path == m.Path {
				return bad(codeMenuPathInUse, map[string]any{"path": m.Path, "title": x.Title}, "路由 "+m.Path+" 已被「"+x.Title+"」使用")
			}
		}
	case "link":
		m.PageKey = ""
		u, err := url.Parse(m.Path)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			return bad(codeMenuInvalidLink, nil, "外部链接必须是 http(s) 地址")
		}
	default:
		return bad(codeMenuInvalidKind, nil, "kind 只能是 group、page 或 link")
	}
	if m.ParentID != nil {
		var parent *store.Menu
		for _, x := range all {
			if x.ID == *m.ParentID {
				parent = x
			}
		}
		if parent == nil || parent.Kind != "group" {
			return bad(codeMenuInvalidParent, nil, "上级必须是一个分组")
		}
	}
	if m.Permission != "" {
		ok, err := s.store.PermissionExists(r.Context(), m.Permission)
		if err != nil {
			s.internalErr(w, r, err)
			return nil, false
		}
		if !ok {
			return bad(codePermUnknown, map[string]any{"code": m.Permission}, "权限点不存在："+m.Permission)
		}
	}
	return m, true
}

func (s *Server) handleCreateMenu(w http.ResponseWriter, r *http.Request) {
	var req menuReq
	if !readJSON(w, r, &req) {
		return
	}
	m, ok := s.validateMenu(w, r, req, 0)
	if !ok {
		return
	}
	if err := s.store.CreateMenu(r.Context(), m); err != nil {
		s.internalErr(w, r, err)
		return
	}
	s.audit(r, auditMenuChange, m.Title, map[string]any{"op": "create", "id": m.ID, "path": m.Path, "permission": m.Permission})
	writeJSON(w, http.StatusCreated, m)
}

func (s *Server) handleUpdateMenu(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req menuReq
	if !readJSON(w, r, &req) {
		return
	}
	if _, err := s.store.GetMenu(r.Context(), id); err != nil {
		if notFound(err) {
			writeErrCode(w, http.StatusNotFound, codeMenuNotFound, nil, "菜单不存在")
			return
		}
		s.internalErr(w, r, err)
		return
	}
	m, ok := s.validateMenu(w, r, req, id)
	if !ok {
		return
	}
	if err := s.store.UpdateMenu(r.Context(), m); err != nil {
		s.internalErr(w, r, err)
		return
	}
	updated, err := s.store.GetMenu(r.Context(), id)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	s.audit(r, auditMenuChange, updated.Title, map[string]any{"op": "update", "id": id, "path": updated.Path, "permission": updated.Permission, "visible": updated.Visible})
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) handleDeleteMenu(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := s.store.DeleteMenu(r.Context(), id); err != nil {
		switch {
		case notFound(err):
			writeErrCode(w, http.StatusNotFound, codeMenuNotFound, nil, "菜单不存在")
		case errors.Is(err, store.ErrInUse):
			writeErrCode(w, http.StatusConflict, codeMenuInUse, nil, errMsg(err))
		default:
			s.internalErr(w, r, err)
		}
		return
	}
	s.audit(r, auditMenuChange, "", map[string]any{"op": "delete", "id": id})
	w.WriteHeader(http.StatusNoContent)
}

type reorderReq struct {
	Items []store.MenuOrder `json:"items"`
}

func (s *Server) handleReorderMenus(w http.ResponseWriter, r *http.Request) {
	var req reorderReq
	if !readJSON(w, r, &req) {
		return
	}
	all, err := s.store.ListMenus(r.Context())
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	byID := map[int64]*store.Menu{}
	for _, m := range all {
		byID[m.ID] = m
	}
	for _, it := range req.Items {
		m := byID[it.ID]
		if m == nil {
			writeErrCode(w, http.StatusBadRequest, codeMenuUnknown, map[string]any{"id": it.ID}, "菜单不存在："+itoa64(it.ID))
			return
		}
		if it.ParentID != nil {
			p := byID[*it.ParentID]
			if p == nil || p.Kind != "group" || m.Kind == "group" {
				writeErrCode(w, http.StatusBadRequest, codeMenuInvalidGroupChild, nil, "只能把页面或链接放进分组里")
				return
			}
		}
	}
	if err := s.store.ReorderMenus(r.Context(), req.Items); err != nil {
		s.internalErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleResetMenus(w http.ResponseWriter, r *http.Request) {
	if err := s.store.ResetMenus(r.Context(), rbac.DefaultMenus); err != nil {
		s.internalErr(w, r, err)
		return
	}
	s.audit(r, auditMenuChange, "", map[string]any{"op": "reset"})
	s.handleListMenus(w, r)
}

func itoa64(n int64) string { return strconv.FormatInt(n, 10) }

// usernameOf 用于审计日志里的对象名；查不到时退回 id。
func (s *Server) usernameOf(r *http.Request, id int64) string {
	if u, err := s.store.GetUser(r.Context(), id); err == nil {
		return u.Username
	}
	return "#" + itoa64(id)
}

func roleNames(refs []store.RoleRef) []string {
	out := []string{}
	for _, r := range refs {
		out = append(out, r.Name)
	}
	return out
}
