package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/hikariming/dsh-ee/server/internal/access"
	"github.com/hikariming/dsh-ee/server/internal/kb"
	"github.com/hikariming/dsh-ee/server/internal/rbac"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// 权限配置即代码（docs/agent-dev.md §6.2）：角色、管理约束、字段、规则导出成一个 YAML 文件，
// 改完先 plan（会变什么、影响谁）、test（跑访问测试），再提交成变更请求，由人批准后才生效。
// dsh-ee 自己记每一版，生效之后可以一键回滚。
//
// 文件里没写的角色保持不变（角色还牵着人，不能因为漏写就删掉）；没写的字段和规则会被删掉（规则和字段就是这份文件说了算）。

const configVersion = 1

// configRole 是配置文件里的一个角色。
type configRole struct {
	Code        string   `json:"code" yaml:"code"`
	Name        string   `json:"name" yaml:"name"`
	Description string   `json:"description,omitempty" yaml:"description,omitempty"`
	Permissions []string `json:"permissions" yaml:"permissions"`
}

// configDoc 是整份权限配置。
type configDoc struct {
	Version int            `json:"version" yaml:"version"`
	Policy  adminPolicy    `json:"policy" yaml:"policy"`
	Roles   []configRole   `json:"roles" yaml:"roles"`
	Fields  []access.Field `json:"fields" yaml:"fields"`
	Rules   []access.Rule  `json:"rules" yaml:"rules"`
}

func (c *configDoc) access() *access.Config { return &access.Config{Fields: c.Fields, Rules: c.Rules} }

// exportConfig 导出现在的配置。
func (s *Server) exportConfig(ctx context.Context) (*configDoc, error) {
	ac, err := s.accessConfig(ctx)
	if err != nil {
		return nil, err
	}
	roles, err := s.store.ListRoles(ctx)
	if err != nil {
		return nil, err
	}
	out := &configDoc{Version: configVersion, Policy: s.adminPolicy(ctx), Fields: ac.Fields, Rules: ac.Rules, Roles: []configRole{}}
	for _, r := range roles {
		if r.IsSuper {
			continue
		}
		perms := slices.Clone(r.Permissions)
		sort.Strings(perms)
		out.Roles = append(out.Roles, configRole{Code: r.Code, Name: r.Name, Description: r.Description, Permissions: perms})
	}
	for i := range out.Fields {
		out.Fields[i].ID = 0
	}
	for i := range out.Rules {
		out.Rules[i].ID = 0
	}
	return out, nil
}

// parseConfig 读一份 YAML（或 JSON）配置并检查。字段的 id 按键对上现有的。
func (s *Server) parseConfig(ctx context.Context, raw string) (*configDoc, []string, error) {
	var c configDoc
	if err := yaml.Unmarshal([]byte(raw), &c); err != nil {
		return nil, []string{"配置文件读不了：" + err.Error()}, nil
	}
	var problems []string
	if c.Version != configVersion {
		problems = append(problems, fmt.Sprintf("version 要写 %d", configVersion))
	}
	perms, err := s.store.ListPermissions(ctx)
	if err != nil {
		return nil, nil, err
	}
	knownPerm := map[string]bool{}
	for _, p := range perms {
		knownPerm[p.Code] = true
	}
	seenRole := map[string]bool{}
	for i := range c.Roles {
		r := &c.Roles[i]
		r.Code, r.Name = strings.TrimSpace(r.Code), strings.TrimSpace(r.Name)
		switch {
		case r.Code == "" || !slugRe.MatchString(r.Code):
			problems = append(problems, fmt.Sprintf("roles[%d] 的 code %q 不对：小写字母、数字、连字符", i, r.Code))
		case r.Code == rbac.RoleSuper:
			problems = append(problems, "超级管理员角色不写在配置文件里：要关掉它用 policy.super_disabled")
		case seenRole[r.Code]:
			problems = append(problems, "角色 "+r.Code+" 写了两次")
		case r.Name == "":
			problems = append(problems, "角色 "+r.Code+" 没有 name")
		}
		seenRole[r.Code] = true
		r.Permissions = dedupeStrings(r.Permissions)
		for _, p := range r.Permissions {
			if !knownPerm[p] {
				problems = append(problems, "角色 "+r.Code+" 里的权限点 "+p+" 不存在")
			}
		}
		sort.Strings(r.Permissions)
		if r.Permissions == nil {
			r.Permissions = []string{}
		}
	}
	ac, msg := s.prepareConfig(ctx, c.access())
	if msg != "" {
		problems = append(problems, msg)
	} else {
		c.Fields, c.Rules = ac.Fields, ac.Rules
	}
	seen := map[string]bool{}
	for _, f := range c.Fields {
		k := f.Object + "." + f.Key
		if seen[k] {
			problems = append(problems, "字段 "+k+" 写了两次")
		}
		seen[k] = true
	}
	seenRule := map[string]bool{}
	for _, r := range c.Rules {
		if seenRule[r.Name] {
			problems = append(problems, "规则「"+r.Name+"」写了两次")
		}
		seenRule[r.Name] = true
	}
	for _, a := range c.Policy.DualApproval {
		if a != DualRolesAssign && a != DualConfig {
			problems = append(problems, "policy.dual_approval 只能有 "+DualRolesAssign+" / "+DualConfig)
		}
	}
	if c.Fields == nil {
		c.Fields = []access.Field{}
	}
	if c.Rules == nil {
		c.Rules = []access.Rule{}
	}
	return &c, problems, nil
}

// ---- plan：会变什么、影响谁 ----

type planResult struct {
	OK       bool     `json:"ok"`
	Problems []string `json:"problems"`
	Changes  []string `json:"changes"`
	Notes    []string `json:"notes"`
	// 规则变化让谁多看到 / 少看到了哪些文档
	Impact []planImpact `json:"impact"`
	// 角色变化让谁多了 / 少了哪些权限点
	PermImpact []permImpact `json:"perm_impact"`
	Summary    string       `json:"summary"`
}

type planImpact struct {
	User   string   `json:"user"`
	KB     string   `json:"kb"`
	Lose   []string `json:"lose,omitempty"`
	Gain   []string `json:"gain,omitempty"`
	Recent []string `json:"recently_read_lost,omitempty"`
}

type permImpact struct {
	User string   `json:"user"`
	Lose []string `json:"lose,omitempty"`
	Gain []string `json:"gain,omitempty"`
}

// hypotheticalAccess 算一个人在新配置下的权限：角色按新的权限点算，文件里没写的角色用现在的；超级管理员看 policy。
func hypotheticalAccess(cur *rbac.Access, roles []*store.Role, next *configDoc) *rbac.Access {
	out := &rbac.Access{RoleIDs: cur.RoleIDs, RoleCodes: cur.RoleCodes, Perms: map[string]bool{}}
	byCode := map[string]configRole{}
	for _, r := range next.Roles {
		byCode[r.Code] = r
	}
	for _, id := range cur.RoleIDs {
		for _, r := range roles {
			if r.ID != id {
				continue
			}
			if r.IsSuper {
				out.Super = !next.Policy.SuperDisabled
				continue
			}
			perms := r.Permissions
			if nr, ok := byCode[r.Code]; ok {
				perms = nr.Permissions
			}
			for _, p := range perms {
				out.Perms[p] = true
			}
		}
	}
	return out
}

// rawAccess 是不看「超级管理员关没关」的权限（plan 要自己按新配置算）。
func (s *Server) rawAccess(ctx context.Context, userID int64, roles []*store.Role) (*rbac.Access, error) {
	a, err := s.store.UserAccess(ctx, userID)
	if err != nil {
		return nil, err
	}
	for _, id := range a.RoleIDs {
		for _, r := range roles {
			if r.ID == id && r.IsSuper {
				a.Super = true
			}
		}
	}
	return a, nil
}

const planMaxUsers = 2000

func (s *Server) planConfig(ctx context.Context, next *configDoc) (*planResult, error) {
	res := &planResult{Problems: []string{}, Changes: []string{}, Notes: []string{}, Impact: []planImpact{}, PermImpact: []permImpact{}}
	cur, err := s.exportConfig(ctx)
	if err != nil {
		return nil, err
	}
	// 配置本身的变化
	curRoles := map[string]configRole{}
	for _, r := range cur.Roles {
		curRoles[r.Code] = r
	}
	for _, r := range next.Roles {
		old, ok := curRoles[r.Code]
		switch {
		case !ok:
			res.Changes = append(res.Changes, "新增角色 "+r.Code+"（"+strings.Join(r.Permissions, "、")+"）")
		default:
			if add, del := diffStrings(old.Permissions, r.Permissions); len(add)+len(del) > 0 {
				res.Changes = append(res.Changes, fmt.Sprintf("角色 %s：加上 %v，去掉 %v", r.Code, add, del))
			}
			if old.Name != r.Name {
				res.Changes = append(res.Changes, "角色 "+r.Code+" 改名为 "+r.Name)
			}
		}
		delete(curRoles, r.Code)
	}
	for code := range curRoles {
		res.Notes = append(res.Notes, "配置文件里没写角色 "+code+"：保持不变（配置文件不会删角色）")
	}
	curFields := map[string]access.Field{}
	for _, f := range cur.Fields {
		curFields[f.Object+"."+f.Key] = f
	}
	for _, f := range next.Fields {
		k := f.Object + "." + f.Key
		old, ok := curFields[k]
		switch {
		case !ok:
			res.Changes = append(res.Changes, "新增字段 "+k+"（"+f.Type+"）")
		case old.Type != f.Type:
			res.Problems = append(res.Problems, "字段 "+k+" 的类型不能从 "+old.Type+" 改成 "+f.Type+"：要换类型请新建一个字段")
		default:
			if !slices.Equal(old.Options, f.Options) {
				res.Changes = append(res.Changes, fmt.Sprintf("字段 %s 的选项：%v → %v", k, old.Options, f.Options))
			}
			if old.Access != f.Access {
				res.Changes = append(res.Changes, fmt.Sprintf("字段 %s「用于访问控制」：%v → %v", k, old.Access, f.Access))
			}
			if old.Label != f.Label {
				res.Changes = append(res.Changes, "字段 "+k+" 改名为 "+f.Label)
			}
		}
		delete(curFields, k)
	}
	for k, f := range curFields {
		n, _ := s.store.CountFieldValues(ctx, f.ID)
		res.Changes = append(res.Changes, fmt.Sprintf("删掉字段 %s（%d 个对象填了值，会一起删掉）", k, n))
	}
	curRules := map[string]access.Rule{}
	for _, r := range cur.Rules {
		curRules[r.Name] = r
	}
	for _, r := range next.Rules {
		old, ok := curRules[r.Name]
		if !ok {
			res.Changes = append(res.Changes, "新增规则「"+r.Name+"」")
		} else if !ruleEqual(old, r) {
			res.Changes = append(res.Changes, "修改规则「"+r.Name+"」")
		}
		delete(curRules, r.Name)
	}
	for name := range curRules {
		res.Changes = append(res.Changes, "删掉规则「"+name+"」")
	}
	if pc, _ := json.Marshal(cur.Policy); true {
		pn, _ := json.Marshal(next.Policy)
		if string(pc) != string(pn) {
			res.Changes = append(res.Changes, "管理约束："+string(pc)+" → "+string(pn))
		}
	}
	// 能不能生效
	pol := next.Policy
	if probs, err := s.policyProblemsWith(ctx, &pol, next); err != nil {
		return nil, err
	} else {
		res.Problems = append(res.Problems, probs...)
	}
	// 影响范围
	users, err := s.store.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	roles, err := s.store.ListRoles(ctx)
	if err != nil {
		return nil, err
	}
	bases, err := s.store.ListKBases(ctx)
	if err != nil {
		return nil, err
	}
	curAC, nextAC := cur.access(), next.access()
	lostUsers, lostDocs := 0, map[int64]bool{}
	reads, _ := s.store.RecentKQueries(ctx, store.KQueryFilter{Tool: "read", Since: time.Now().AddDate(0, 0, -7), Limit: 20000})
	recentlyRead := map[int64]map[int64]bool{}
	for _, q := range reads {
		if recentlyRead[q.UserID] == nil {
			recentlyRead[q.UserID] = map[int64]bool{}
		}
		for _, d := range q.Docs {
			recentlyRead[q.UserID][d] = true
		}
	}
	checked := 0
	for _, u := range users {
		if u.Disabled {
			continue
		}
		if checked++; checked > planMaxUsers {
			res.Notes = append(res.Notes, fmt.Sprintf("人数超过 %d，只算了前 %d 个人的影响", planMaxUsers, planMaxUsers))
			break
		}
		curA, err := s.rawAccess(ctx, u.ID, roles)
		if err != nil {
			return nil, err
		}
		curA.Super = curA.Super && !cur.Policy.SuperDisabled
		nextA := hypotheticalAccess(curA, roles, next)
		if curA.Super && !next.Policy.SuperDisabled {
			nextA.Super = true
		}
		if add, del := diffPerms(curA, nextA); len(add)+len(del) > 0 {
			res.PermImpact = append(res.PermImpact, permImpact{User: u.Username, Gain: add, Lose: del})
		}
		lost := false
		for _, b := range bases {
			vc, vn := viewer{user: u, access: curA}, viewer{user: u, access: nextA}
			if !vc.kbReads(b) && !vn.kbReads(b) {
				continue
			}
			before, err := s.visibleDocs(ctx, curAC, vc, b)
			if err != nil {
				return nil, err
			}
			after, err := s.visibleDocs(ctx, nextAC, vn, b)
			if err != nil {
				return nil, err
			}
			imp := planImpact{User: u.Username, KB: b.Slug}
			for id, title := range before {
				if _, ok := after[id]; !ok {
					imp.Lose = append(imp.Lose, title)
					lostDocs[id] = true
					if recentlyRead[u.ID][id] {
						imp.Recent = append(imp.Recent, title)
					}
				}
			}
			for id, title := range after {
				if _, ok := before[id]; !ok {
					imp.Gain = append(imp.Gain, title)
				}
			}
			if len(imp.Lose)+len(imp.Gain) > 0 {
				sort.Strings(imp.Lose)
				sort.Strings(imp.Gain)
				sort.Strings(imp.Recent)
				if len(imp.Lose) > 0 {
					lost = true
				}
				res.Impact = append(res.Impact, imp)
			}
		}
		if lost {
			lostUsers++
		}
	}
	recent := 0
	for _, imp := range res.Impact {
		recent += len(imp.Recent)
	}
	res.Summary = fmt.Sprintf("%d 处配置变化；生效后 %d 个人将看不到 %d 份原来能看的文档（其中 %d 份是他们最近一周读过的）；%d 个人的权限点有变化",
		len(res.Changes), lostUsers, len(lostDocs), recent, len(res.PermImpact))
	res.OK = len(res.Problems) == 0
	return res, nil
}

// visibleDocs 是一个人在一个库里能看到的文档：id → 标题。
func (s *Server) visibleDocs(ctx context.Context, c *access.Config, v viewer, b *store.KBase) (map[int64]string, error) {
	out := map[int64]string{}
	if !v.kbReads(b) {
		return out, nil
	}
	docs, err := s.store.ListKDocs(ctx, b.ID)
	if err != nil {
		return nil, err
	}
	g, err := s.kbGate(ctx, c, v, b)
	if err != nil {
		return nil, err
	}
	for _, d := range docs {
		if g == nil || g(d.ID) {
			out[d.ID] = d.Title
		}
	}
	return out, nil
}

func diffStrings(a, b []string) (add, del []string) {
	for _, x := range b {
		if !slices.Contains(a, x) {
			add = append(add, x)
		}
	}
	for _, x := range a {
		if !slices.Contains(b, x) {
			del = append(del, x)
		}
	}
	return add, del
}

func diffPerms(a, b *rbac.Access) (add, del []string) {
	for _, p := range rbac.Catalog {
		ha, hb := a.Has(p.Code), b.Has(p.Code)
		switch {
		case hb && !ha:
			add = append(add, p.Code)
		case ha && !hb:
			del = append(del, p.Code)
		}
	}
	return add, del
}

func ruleEqual(a, b access.Rule) bool {
	a.ID, b.ID = 0, 0
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	return string(ja) == string(jb)
}

// policyProblemsWith 和 policyProblems 一样，但角色和权限点按新配置算（关超级管理员前要有人持有每个管理权限点）。
func (s *Server) policyProblemsWith(ctx context.Context, p *adminPolicy, next *configDoc) ([]string, error) {
	q := *p
	q.SuperDisabled = false
	out, err := s.policyProblems(ctx, &q)
	if err != nil {
		return nil, err
	}
	// 互斥组里的角色可以是这份文件里新建的
	var filtered []string
	for _, msg := range out {
		keep := true
		for _, r := range next.Roles {
			if strings.Contains(msg, "互斥角色里的 "+r.Code+" 不存在") {
				keep = false
			}
		}
		if keep {
			filtered = append(filtered, msg)
		}
	}
	if !p.SuperDisabled {
		return filtered, nil
	}
	users, err := s.store.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	roles, err := s.store.ListRoles(ctx)
	if err != nil {
		return nil, err
	}
	held := map[string]bool{}
	for _, u := range users {
		if u.Disabled {
			continue
		}
		a, err := s.rawAccess(ctx, u.ID, roles)
		if err != nil {
			return nil, err
		}
		for perm := range hypotheticalAccess(a, roles, next).Perms {
			held[perm] = true
		}
	}
	var missing []string
	for _, perm := range rbac.Catalog {
		if rbac.IsAdminPerm(perm.Code) && !held[perm.Code] {
			missing = append(missing, perm.Code)
		}
	}
	if len(missing) > 0 {
		filtered = append(filtered, "关掉超级管理员之后没人持有这些管理权限点，会把自己锁在外面："+strings.Join(missing, "、"))
	}
	return filtered, nil
}

// ---- test：访问测试（docs/agent-dev.md §4.3）----

type accessCase struct {
	As         string   `yaml:"as" json:"as"`
	CanRead    []string `yaml:"can_read" json:"can_read,omitempty"`
	CannotRead []string `yaml:"cannot_read" json:"cannot_read,omitempty"`
	Can        []string `yaml:"can" json:"can,omitempty"`
	Cannot     []string `yaml:"cannot" json:"cannot,omitempty"`
	// 检索：结果里（标题和片段）不能出现这些文档
	Search         string   `yaml:"search" json:"search,omitempty"`
	MustNotContain []string `yaml:"must_not_contain" json:"must_not_contain,omitempty"`
	MustContain    []string `yaml:"must_contain" json:"must_contain,omitempty"`
}

type caseResult struct {
	Case   int      `json:"case"`
	As     string   `json:"as"`
	OK     bool     `json:"ok"`
	Errors []string `json:"errors,omitempty"`
}

func (s *Server) runAccessTests(ctx context.Context, next *configDoc, cases []accessCase) ([]caseResult, error) {
	roles, err := s.store.ListRoles(ctx)
	if err != nil {
		return nil, err
	}
	bases, err := s.store.ListKBases(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.kbSync(ctx, bases); err != nil {
		return nil, err
	}
	ac := next.access()
	out := []caseResult{}
	for i, tc := range cases {
		res := caseResult{Case: i + 1, As: tc.As, OK: true}
		fail := func(f string, args ...any) { res.OK = false; res.Errors = append(res.Errors, fmt.Sprintf(f, args...)) }
		u, err := s.findUser(ctx, tc.As)
		if err != nil {
			fail("没有用户 %s", tc.As)
			out = append(out, res)
			continue
		}
		raw, err := s.rawAccess(ctx, u.ID, roles)
		if err != nil {
			return nil, err
		}
		a := hypotheticalAccess(raw, roles, next)
		v := viewer{user: u, access: a}
		for _, p := range tc.Can {
			if !a.Has(p) || u.Disabled {
				fail("%s 应该有 %s，但没有", tc.As, p)
			}
		}
		for _, p := range tc.Cannot {
			if a.Has(p) && !u.Disabled {
				fail("%s 不应该有 %s，但有", tc.As, p)
			}
		}
		visible := map[string]bool{}
		allTitles := map[string]bool{}
		gates := map[int64]docGate{}
		var readable []*store.KBase
		for _, b := range bases {
			docs, err := s.store.ListKDocs(ctx, b.ID)
			if err != nil {
				return nil, err
			}
			for _, d := range docs {
				allTitles[d.Title] = true
			}
			if !v.kbReads(b) || u.Disabled {
				continue
			}
			readable = append(readable, b)
			got, err := s.visibleDocs(ctx, ac, v, b)
			if err != nil {
				return nil, err
			}
			for _, t := range got {
				visible[t] = true
			}
			if g, err := s.kbGate(ctx, ac, v, b); err != nil {
				return nil, err
			} else if g != nil {
				gates[b.ID] = g
			}
		}
		for _, t := range append(slices.Clone(tc.CanRead), tc.CannotRead...) {
			if !allTitles[t] {
				fail("没有标题是「%s」的文档（测试里的标题要和文档标题一字不差）", t)
			}
		}
		for _, t := range tc.CanRead {
			if allTitles[t] && !visible[t] {
				fail("%s 应该能看「%s」，但看不到", tc.As, t)
			}
		}
		for _, t := range tc.CannotRead {
			if visible[t] {
				fail("%s 不应该看到「%s」，但能看到", tc.As, t)
			}
		}
		if tc.Search != "" {
			q := kb.Query{Text: tc.Search, Limit: 100, Allow: allowFunc(gates)}
			for _, b := range readable {
				q.Bases = append(q.Bases, b.ID)
			}
			found := map[string]bool{}
			if len(q.Bases) > 0 {
				r, err := s.kb.ix.Search(ctx, q)
				if err != nil {
					fail("检索「%s」出错：%v", tc.Search, err)
				} else {
					for _, h := range r.Hits {
						found[h.Title] = true
					}
					for _, d := range r.Docs {
						found[d.Title] = true
					}
				}
			}
			for _, t := range tc.MustNotContain {
				if found[t] {
					fail("%s 检索「%s」时结果里出现了「%s」", tc.As, tc.Search, t)
				}
			}
			for _, t := range tc.MustContain {
				if !found[t] {
					fail("%s 检索「%s」时结果里应该有「%s」", tc.As, tc.Search, t)
				}
			}
		}
		out = append(out, res)
	}
	return out, nil
}

// ---- 接口 ----

type configReq struct {
	// YAML 或 JSON 的配置文件内容
	Config string `json:"config"`
	// 访问测试（YAML 列表）
	Tests string `json:"tests"`
	Note  string `json:"note"`
}

// GET /api/admin/config：导出现在的配置（YAML）。
func (s *Server) handleConfigExport(w http.ResponseWriter, r *http.Request) {
	c, err := s.exportConfig(r.Context())
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	raw, err := yaml.Marshal(c)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	if r.URL.Query().Get("format") == "json" {
		writeJSON(w, http.StatusOK, c)
		return
	}
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	_, _ = w.Write(append([]byte("# dsh-ee 权限配置。改完用 dsh-ee config plan / test 看影响，再用 dsh-ee config apply 提交，批准后才生效。\n"), raw...))
}

func (s *Server) readConfigReq(w http.ResponseWriter, r *http.Request) (*configDoc, *configReq, []string, bool) {
	var req configReq
	if !readJSONLimit(w, r, &req, 4<<20) {
		return nil, nil, nil, false
	}
	c, problems, err := s.parseConfig(r.Context(), req.Config)
	if err != nil {
		s.internalErr(w, r, err)
		return nil, nil, nil, false
	}
	return c, &req, problems, true
}

// POST /api/admin/config/plan {config}
func (s *Server) handleConfigPlan(w http.ResponseWriter, r *http.Request) {
	c, _, problems, ok := s.readConfigReq(w, r)
	if !ok {
		return
	}
	if len(problems) > 0 {
		writeJSON(w, http.StatusOK, &planResult{Problems: problems, Changes: []string{}, Notes: []string{}, Impact: []planImpact{}, PermImpact: []permImpact{}})
		return
	}
	res, err := s.planConfig(r.Context(), c)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// POST /api/admin/config/test {config?, tests}：config 为空时用现在的配置。
func (s *Server) handleConfigTest(w http.ResponseWriter, r *http.Request) {
	var req configReq
	if !readJSONLimit(w, r, &req, 4<<20) {
		return
	}
	ctx := r.Context()
	var c *configDoc
	if strings.TrimSpace(req.Config) == "" {
		var err error
		if c, err = s.exportConfig(ctx); err != nil {
			s.internalErr(w, r, err)
			return
		}
	} else {
		var problems []string
		var err error
		if c, problems, err = s.parseConfig(ctx, req.Config); err != nil {
			s.internalErr(w, r, err)
			return
		}
		if len(problems) > 0 {
			writeJSON(w, http.StatusOK, map[string]any{"ok": false, "problems": problems, "results": []any{}})
			return
		}
	}
	var cases []accessCase
	if err := yaml.Unmarshal([]byte(req.Tests), &cases); err != nil || len(cases) == 0 {
		writeErr(w, http.StatusBadRequest, "tests 要是一个 YAML 列表，每一项有 as 和 can_read / cannot_read / can / cannot / search")
		return
	}
	if len(cases) > 500 {
		writeErr(w, http.StatusBadRequest, "访问测试最多 500 条")
		return
	}
	results, err := s.runAccessTests(ctx, c, cases)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	ok := true
	for _, res := range results {
		ok = ok && res.OK
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": ok, "problems": []string{}, "results": results})
}

// POST /api/admin/config/changes {config, note}：提交一条变更请求，等人批准。
func (s *Server) handleConfigSubmit(w http.ResponseWriter, r *http.Request) {
	c, req, problems, ok := s.readConfigReq(w, r)
	if !ok {
		return
	}
	if len(problems) > 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "配置文件有问题：" + strings.Join(problems, "；"), "problems": problems})
		return
	}
	plan, err := s.planConfig(r.Context(), c)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	if !plan.OK {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "这份配置生效不了：" + strings.Join(plan.Problems, "；"), "plan": plan})
		return
	}
	payload, _ := json.Marshal(c)
	_, viaToken := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	title := cutUTF8(strings.TrimSpace(req.Note), 190)
	if title == "" {
		title = plan.Summary
	}
	cr := &store.ChangeRequest{Kind: store.ChangeConfig, Title: cutUTF8(title, 190), Payload: string(payload), Note: cutUTF8(req.Note, 900), CreatedBy: currentUser(r).ID, ViaToken: viaToken}
	if err := s.store.CreateChangeRequest(r.Context(), cr); err != nil {
		s.internalErr(w, r, err)
		return
	}
	s.audit(r, "change_request.create", strconv.FormatInt(cr.ID, 10), map[string]any{"kind": cr.Kind, "summary": plan.Summary})
	writeJSON(w, http.StatusCreated, map[string]any{"change_request": cr, "plan": plan, "message": "已提交，等有权限的管理员在网页上批准后才生效"})
}

// GET /api/admin/config/changes
func (s *Server) handleConfigChanges(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListChangeRequests(r.Context(), 100)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	// 页面上要显示谁提交、谁批准的
	type item struct {
		*store.ChangeRequest
		CreatedByName string `json:"created_by_name"`
		DecidedByName string `json:"decided_by_name,omitempty"`
	}
	names := map[int64]string{}
	name := func(id int64) string {
		if n, ok := names[id]; ok {
			return n
		}
		names[id] = s.usernameByID(r.Context(), id)
		return names[id]
	}
	out := make([]item, len(list))
	for i, c := range list {
		out[i] = item{ChangeRequest: c, CreatedByName: name(c.CreatedBy)}
		if c.DecidedBy != nil {
			out[i].DecidedByName = name(*c.DecidedBy)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

var errChangeConflict = errors.New("conflict")

// POST /api/admin/config/changes/{id} {approve}：批准或驳回。只认网页会话：批准永远是人。
func (s *Server) handleConfigDecide(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req struct {
		Approve bool `json:"approve"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	ctx := r.Context()
	cr, err := s.store.GetChangeRequest(ctx, id)
	if notFound(err) {
		writeErr(w, http.StatusNotFound, "没有这条变更请求")
		return
	}
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	me := currentUser(r)
	policy := s.adminPolicy(ctx)
	if req.Approve && cr.CreatedBy == me.ID && (cr.Kind == store.ChangeUserRoles || policy.dual(DualConfig) || cr.ViaToken) {
		writeErr(w, http.StatusForbidden, "这条变更要另一位管理员批准，不能自己批准自己提交的")
		return
	}
	need := rbac.FieldsManage
	if cr.Kind == store.ChangeUserRoles {
		need = rbac.RolesManage
	}
	if cr.Kind == store.ChangeConfig {
		var c configDoc
		_ = json.Unmarshal([]byte(cr.Payload), &c)
		if len(c.Roles) > 0 {
			need = rbac.RolesManage
		}
	}
	if !currentAccess(r).Has(need) {
		writeErr(w, http.StatusForbidden, "批准这条变更要有 "+need+" 权限")
		return
	}
	status := store.ChangeRejected
	if req.Approve {
		status = store.ChangeApplied
	}
	claimed, err := s.store.ClaimChangeRequest(ctx, id, me.ID, status)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	if !claimed {
		writeErr(w, http.StatusConflict, "这条变更已经处理过了")
		return
	}
	if !req.Approve {
		s.audit(r, "change_request.reject", strconv.FormatInt(id, 10), map[string]any{"kind": cr.Kind})
		fresh, _ := s.store.GetChangeRequest(ctx, id)
		writeJSON(w, http.StatusOK, fresh)
		return
	}
	before, applyErr := s.applyChange(r, cr)
	final, msg := store.ChangeApplied, ""
	if applyErr != nil {
		final, msg = store.ChangeFailed, applyErr.Error()
	}
	if err := s.store.FinishChangeRequest(ctx, id, final, before, msg); err != nil {
		s.internalErr(w, r, err)
		return
	}
	s.audit(r, "change_request.apply", strconv.FormatInt(id, 10), map[string]any{"kind": cr.Kind, "ok": applyErr == nil, "error": msg})
	s.hub.notify()
	fresh, _ := s.store.GetChangeRequest(ctx, id)
	if applyErr != nil {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "批准了但没能生效：" + msg, "change_request": fresh})
		return
	}
	writeJSON(w, http.StatusOK, fresh)
}

// POST /api/admin/config/changes/{id}/rollback：把这条变更生效前的配置作为一条新的变更请求提交。
func (s *Server) handleConfigRollback(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	cr, err := s.store.GetChangeRequest(r.Context(), id)
	if err != nil || cr.Kind != store.ChangeConfig || cr.Status != store.ChangeApplied || cr.Before == "" {
		writeErr(w, http.StatusBadRequest, "只有已经生效的配置变更能回滚")
		return
	}
	nc := &store.ChangeRequest{Kind: store.ChangeConfig, Title: fmt.Sprintf("回滚变更 #%d", id), Payload: cr.Before, CreatedBy: currentUser(r).ID}
	if err := s.store.CreateChangeRequest(r.Context(), nc); err != nil {
		s.internalErr(w, r, err)
		return
	}
	s.audit(r, "change_request.create", strconv.FormatInt(nc.ID, 10), map[string]any{"kind": nc.Kind, "rollback_of": id})
	writeJSON(w, http.StatusCreated, map[string]any{"change_request": nc})
}

// applyChange 执行一条批准了的变更，返回执行前的配置（回滚用）。
func (s *Server) applyChange(r *http.Request, cr *store.ChangeRequest) (string, error) {
	ctx := r.Context()
	switch cr.Kind {
	case store.ChangeUserRoles:
		var p struct {
			UserID  int64   `json:"user_id"`
			RoleIDs []int64 `json:"role_ids"`
		}
		if err := json.Unmarshal([]byte(cr.Payload), &p); err != nil {
			return "", err
		}
		// 用批准人的权限重新检查一遍：批准人不能借别人的提交授出自己都没有的权限
		target, err := s.store.UserAccess(ctx, p.UserID)
		if err != nil {
			return "", err
		}
		if !outranks(currentAccess(r), target) {
			return "", errors.New("不能修改权限比自己大的用户")
		}
		if e, ok := s.checkGrantable(r, p.RoleIDs); !ok {
			return "", errors.New(e.msg)
		}
		if msg, _, err := s.roleChangeProblem(ctx, s.adminPolicy(ctx), cr.CreatedBy, p.UserID, target, p.RoleIDs); err != nil || msg != "" {
			if err != nil {
				return "", err
			}
			return "", errors.New(msg)
		}
		return "", s.store.SetUserRoles(ctx, p.UserID, p.RoleIDs)
	case store.ChangeConfig:
		var c configDoc
		if err := json.Unmarshal([]byte(cr.Payload), &c); err != nil {
			return "", err
		}
		// 批准的时候再检查一遍：提交之后别人可能改过
		raw, _ := yaml.Marshal(&c)
		next, problems, err := s.parseConfig(ctx, string(raw))
		if err != nil {
			return "", err
		}
		if len(problems) > 0 {
			return "", errors.New(strings.Join(problems, "；"))
		}
		plan, err := s.planConfig(ctx, next)
		if err != nil {
			return "", err
		}
		if !plan.OK {
			return "", errors.New(strings.Join(plan.Problems, "；"))
		}
		before, err := s.exportConfig(ctx)
		if err != nil {
			return "", err
		}
		snap, _ := json.Marshal(before)
		return string(snap), s.applyConfig(r, next)
	}
	return "", errors.New("不认识的变更类型")
}

// applyConfig 让一份配置生效：角色 → 字段 → 规则 → 管理约束。
func (s *Server) applyConfig(r *http.Request, c *configDoc) error {
	ctx := r.Context()
	actor := currentAccess(r)
	for _, cr := range c.Roles {
		if !subset(actor, cr.Permissions) {
			return errors.New("批准人自己没有角色 " + cr.Code + " 的全部权限，不能批准这份配置")
		}
		role, err := s.store.GetRoleByCode(ctx, cr.Code)
		if notFound(err) {
			if err := s.store.CreateRole(ctx, &store.Role{Code: cr.Code, Name: cr.Name, Description: cr.Description, Permissions: cr.Permissions}); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		if !subset(actor, role.Permissions) {
			return errors.New("角色 " + cr.Code + " 有批准人没有的权限，不能由他改")
		}
		name, desc := cr.Name, cr.Description
		if err := s.store.UpdateRole(ctx, role.ID, store.RolePatch{Name: &name, Description: &desc, Permissions: cr.Permissions}); err != nil {
			return err
		}
	}
	cur, err := s.accessConfig(ctx)
	if err != nil {
		return err
	}
	// 先删掉文件里没有的规则，再动字段（字段被规则用着时删不掉）
	keepRule := map[string]bool{}
	for _, r := range c.Rules {
		keepRule[r.Name] = true
	}
	for _, old := range cur.Rules {
		if !keepRule[old.Name] {
			if err := s.store.DeleteAccessRule(ctx, old.ID); err != nil {
				return err
			}
		}
	}
	keepField := map[string]bool{}
	for i := range c.Fields {
		f := &c.Fields[i]
		keepField[f.Object+"."+f.Key] = true
		if old := cur.Field(f.Object, f.Key); old != nil {
			f.ID = old.ID
			f.Source = old.Source
			if err := s.store.UpdateField(ctx, f); err != nil {
				return err
			}
			continue
		}
		if err := s.store.CreateField(ctx, f, currentUser(r).ID); err != nil {
			return err
		}
	}
	for _, old := range cur.Fields {
		if !keepField[old.Object+"."+old.Key] {
			if err := s.store.DeleteField(ctx, old.ID); err != nil {
				return err
			}
		}
	}
	for _, nr := range c.Rules {
		nr := nr
		var oldID int64
		for _, old := range cur.Rules {
			if old.Name == nr.Name {
				oldID = old.ID
			}
		}
		if oldID > 0 {
			nr.ID = oldID
			if err := s.store.UpdateAccessRule(ctx, &nr); err != nil {
				return err
			}
		} else if err := s.store.CreateAccessRule(ctx, &nr, currentUser(r).ID); err != nil {
			return err
		}
	}
	pol := c.Policy
	if err := s.store.PutSettingJSON(ctx, adminPolicySetting, pol, currentUser(r).ID); err != nil {
		return err
	}
	s.store.SetSuperDisabled(pol.SuperDisabled)
	return nil
}

// guardDual 在开了双人批准（config）时拦下直接改字段、规则、角色的接口。
func (s *Server) guardDual(w http.ResponseWriter, r *http.Request) bool {
	if s.adminPolicy(r.Context()).dual(DualConfig) {
		writeErr(w, http.StatusConflict, "开了双人批准：字段、访问规则和角色要通过配置变更提交（dsh-ee config apply，或「字段与访问规则 → 变更请求」），由另一位管理员批准后生效")
		return false
	}
	return true
}
