package api

import (
	"testing"

	"github.com/hikariming/dsh-ee/server/internal/rbac"
)

// 改管理约束的权限判定（policyChangeProblem）：直接改约束和配置变更（submit / decide / apply）共用，
// 必须保持「关超管只有超管能做、其它约束要 roles.manage、没变化谁都能过」。
func TestPolicyChangeProblem(t *testing.T) {
	super := accessWith(true)
	roles := accessWith(false, rbac.RolesManage)
	fields := accessWith(false, rbac.FieldsManage)

	disabled := adminPolicy{SuperDisabled: true}
	noSelfGrant := adminPolicy{NoSelfGrant: true}

	exclusiveAB := adminPolicy{Exclusive: [][]string{{"a", "b"}}}
	exclusiveBA := adminPolicy{Exclusive: [][]string{{"b", "a"}}}
	dualConfig := adminPolicy{DualApproval: []string{DualConfig}}
	dualBoth := adminPolicy{DualApproval: []string{DualConfig, DualRolesAssign}}
	dualBothReordered := adminPolicy{DualApproval: []string{DualRolesAssign, DualConfig, DualConfig}}

	cases := []struct {
		name   string
		actor  *rbac.Access
		cur    adminPolicy
		next   adminPolicy
		want   string
		reason string
	}{
		{"没变化谁都能过", fields, adminPolicy{}, adminPolicy{}, "", "配置里 policy 没改时不能多要权限"},
		{"没变化（只有 fields.manage）也该过", fields, noSelfGrant, noSelfGrant, "", "同上"},
		{"关超管：fields.manage 不行", fields, adminPolicy{}, disabled, "只有超级管理员能关掉超级管理员", "绕过点"},
		{"关超管：roles.manage 也不行", roles, adminPolicy{}, disabled, "只有超级管理员能关掉超级管理员", "关超管只看是不是超管"},
		{"关超管：超管可以", super, adminPolicy{}, disabled, "", "超管本人才行"},
		{"重新打开超管：roles.manage 可以", roles, disabled, adminPolicy{}, "", "关掉之后没有超管，要能管角色的人来开"},
		{"重新打开超管：fields.manage 不行", fields, disabled, adminPolicy{}, "改管理约束要有 roles.manage 权限", ""},
		{"改其它约束：fields.manage 不行", fields, adminPolicy{}, noSelfGrant, "改管理约束要有 roles.manage 权限", ""},
		{"改其它约束：roles.manage 可以", roles, adminPolicy{}, noSelfGrant, "", ""},
		{"改其它约束：超管可以", super, adminPolicy{}, noSelfGrant, "", ""},
		{"超管已关时改其它约束：仍要 roles.manage", fields, disabled, adminPolicy{SuperDisabled: true, NoSelfGrant: true}, "改管理约束要有 roles.manage 权限", ""},
		{"互斥组顺序不同算没改", fields, exclusiveAB, exclusiveBA, "", "顺序不影响互斥判断"},
		{"双人批准顺序/重复不同算没改", fields, dualBoth, dualBothReordered, "", "列表语义是集合"},
		{"少于两个角色的互斥组不生效，算没改", fields, adminPolicy{}, adminPolicy{Exclusive: [][]string{{"a"}}}, "", "policyProblems 也跳过这种组"},
		{"真的加了互斥组：要 roles.manage", fields, adminPolicy{}, exclusiveAB, "改管理约束要有 roles.manage 权限", ""},
		{"真的加了双人批准：要 roles.manage", fields, adminPolicy{}, dualConfig, "改管理约束要有 roles.manage 权限", ""},
	}
	for _, c := range cases {
		if got := policyChangeProblem(c.actor, c.cur, c.next); got != c.want {
			t.Errorf("%s：policyChangeProblem = %q，期望 %q（%s）", c.name, got, c.want, c.reason)
		}
	}
}

// nil 和空列表是同一份约束：配置导出的 YAML 会丢掉空列表，再导入时不能算成改了约束。
func TestPolicySame(t *testing.T) {
	if !policySame(adminPolicy{}, adminPolicy{Exclusive: [][]string{}, DualApproval: []string{}}) {
		t.Errorf("nil 和空列表应算同一份约束")
	}
	if policySame(adminPolicy{}, adminPolicy{SuperDisabled: true}) {
		t.Errorf("super_disabled 不同不能算同一份约束")
	}
	if !policySame(adminPolicy{DualApproval: []string{DualConfig}}, adminPolicy{DualApproval: []string{DualConfig}}) {
		t.Errorf("相同的约束应算同一份")
	}
}
