package api

import (
	"context"
	"fmt"
	"strings"
)

// 专家 · 技能 · 连接器的管理工具（docs/expert-center.md）：分类、禁用策略、权限查询。
// 和其它管理工具一样只在管理模式里可用；写操作要员工在审批框里确认。

func init() {
	for k, v := range expertCenterAgentSpecs() {
		if _, dup := agentSpecs[k]; dup {
			panic("agentSpecs 重复：" + k)
		}
		v.Admin = true
		agentSpecs[k] = v
		delete(agentPending, k)
	}
}

// idList 把 asset_ids 参数排成「#1、#2」给预览用。
func idList(v any) string {
	xs, _ := v.([]any)
	parts := make([]string, 0, len(xs))
	for _, x := range xs {
		parts = append(parts, fmt.Sprintf("#%v", x))
	}
	return strings.Join(parts, "、")
}

func expertCenterAgentSpecs() map[string]*agentSpec {
	rd, wr, dg := effRead, effWrite, effDanger
	pCatID := agentParam{Name: "id", Type: "integer", Required: true, In: "path", Desc: "分类 id"}
	pBlockID := agentParam{Name: "id", Type: "integer", Required: true, In: "path", Desc: "禁用策略 id"}
	pAssetIDs := agentParam{Name: "asset_ids", Type: "array", Items: "integer", Required: true, Desc: "被禁用的能力 id"}
	return map[string]*agentSpec{
		"GET /api/admin/hub/categories": {
			Tool: "ee_expert_admin", Action: "categories", Summary: "列出专家 · 技能 · 连接器的分类（分组 app / skill / connection、可用范围、能力数）", Effect: rd,
		},
		"POST /api/admin/hub/categories": {
			Tool: "ee_expert_admin", Action: "create_category", Summary: "新建分类；group 是 app（专家与应用）/ skill（技能与提示词）/ connection（连接器）。分类有自己的可用范围，只能收窄不能放宽", En: "Create a category", Effect: wr,
			Params: []agentParam{
				{Name: "group", Type: "string", Required: true, Enum: []string{"app", "skill", "connection"}},
				{Name: "name", Type: "string", Required: true},
				{Name: "sort", Type: "integer"},
				{Name: "scope", Schema: scopeSchema},
			},
		},
		"PATCH /api/admin/hub/categories/{id}": {
			Tool: "ee_expert_admin", Action: "update_category", Summary: "改分类的名字、排序或可用范围（收窄范围后，范围外的人立即看不到其中的能力）", En: "Update a category", Effect: dg,
			Params: []agentParam{pCatID, {Name: "name", Type: "string"}, {Name: "sort", Type: "integer"}, {Name: "scope", Schema: scopeSchema}},
			Preview: func(ctx context.Context, s *Server, _ *Principal, args map[string]any) (string, error) {
				c, err := s.store.GetCategory(ctx, argID(args))
				if err != nil {
					return "", fmt.Errorf("没有 id 为 %d 的分类", argID(args))
				}
				msg := fmt.Sprintf("将修改分类「%s」：%s。", c.Name, changes(args, "id"))
				if _, ok := args["scope"]; ok {
					msg += "范围外的人会立即看不到这个分类下的能力。"
				}
				return msg, nil
			},
		},
		"DELETE /api/admin/hub/categories/{id}": {
			Tool: "ee_expert_admin", Action: "delete_category", Summary: "删除分类；归在里面的能力变成没归类", En: "Delete a category", Effect: dg,
			Params: []agentParam{pCatID},
			Preview: func(ctx context.Context, s *Server, _ *Principal, args map[string]any) (string, error) {
				c, err := s.store.GetCategory(ctx, argID(args))
				if err != nil {
					return "", fmt.Errorf("没有 id 为 %d 的分类", argID(args))
				}
				return fmt.Sprintf("将删除分类「%s」，其中的能力变成没归类，分类的范围限制也随之取消。", c.Name), nil
			},
		},
		"GET /api/admin/hub/blocks": {
			Tool: "ee_expert_admin", Action: "blocks", Summary: "列出禁用策略（谁、被禁用了哪些能力、原因）", Effect: rd,
		},
		"POST /api/admin/hub/blocks": {
			Tool: "ee_expert_admin", Action: "create_block", Summary: "新建禁用策略：范围内的人即使在能力的可用范围里也用不了这些能力（离职交接、外包账号、临时停用）；note 必填", En: "Block capabilities", Effect: dg,
			Params: []agentParam{{Name: "scope", Schema: scopeSchema, Required: true}, pAssetIDs, {Name: "note", Type: "string", Required: true, Desc: "原因"}},
			Preview: func(_ context.Context, _ *Server, _ *Principal, args map[string]any) (string, error) {
				return fmt.Sprintf("将对范围 %v 禁用能力 %s（原因：%v）。范围内的人会立即失去这些能力。", args["scope"], idList(args["asset_ids"]), args["note"]), nil
			},
		},
		"PUT /api/admin/hub/blocks/{id}": {
			Tool: "ee_expert_admin", Action: "update_block", Summary: "整条替换一个禁用策略（范围、能力、原因都要给全）", En: "Update a block policy", Effect: dg,
			Params: []agentParam{pBlockID, {Name: "scope", Schema: scopeSchema, Required: true}, pAssetIDs, {Name: "note", Type: "string", Required: true}},
			Preview: func(_ context.Context, _ *Server, _ *Principal, args map[string]any) (string, error) {
				return fmt.Sprintf("将把禁用策略 #%d 改成：范围 %v，禁用能力 %s（原因：%v）。", argID(args), args["scope"], idList(args["asset_ids"]), args["note"]), nil
			},
		},
		"DELETE /api/admin/hub/blocks/{id}": {
			Tool: "ee_expert_admin", Action: "delete_block", Summary: "删除一个禁用策略，范围内的人立即恢复访问", En: "Delete a block policy", Effect: dg,
			Params: []agentParam{pBlockID},
			Preview: func(ctx context.Context, s *Server, _ *Principal, args map[string]any) (string, error) {
				b, err := s.store.GetBlock(ctx, argID(args))
				if err != nil {
					return "", fmt.Errorf("没有 id 为 %d 的禁用策略", argID(args))
				}
				return fmt.Sprintf("将删除禁用策略 #%d（原因：%s），范围内的人立即恢复对 %d 项能力的访问。", b.ID, b.Note, len(b.AssetIDs)), nil
			},
		},
		"GET /api/admin/hub/effective": {
			Tool: "ee_expert_admin", Action: "effective", Summary: "查某个员工每项能力的最终结果（enabled / available / hidden）和被哪一步挡住（out_of_scope / category / blocked 等）", Effect: rd,
			Params: []agentParam{{Name: "user", Type: "integer", Required: true, In: "query", Desc: "用户 id（用 ee_users 查）"}},
		},
	}
}
