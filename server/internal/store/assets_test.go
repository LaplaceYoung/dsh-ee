package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

// DeleteAssetVersion 只用于撤销一次还没完成的回滚：删掉刚追加的版本，线上版本回退到上一个。
func TestDeleteAssetVersion(t *testing.T) {
	st := openTokenTestStore(t)
	ctx := context.Background()
	a := &Asset{Kind: KindConnection, Slug: "itsm", Title: "工单", Scope: Scope{All: true}, Delivery: DeliveryOptional}
	if err := st.CreateAsset(ctx, a, json.RawMessage(`{"title":"工单","type":"openapi","url":"https://itsm.example.com","extension":{"id":"acme/itsm","version":"1.0.0","digest":"x","manifest":"{}"}}`), StatusPublished, 0); err != nil {
		t.Fatal(err)
	}
	v2, err := st.AddAssetVersion(ctx, a.ID, json.RawMessage(`{"title":"工单","type":"openapi","url":"https://itsm.example.com","extension":{"id":"acme/itsm","version":"1.1.0","digest":"y","manifest":"{}"}}`), StatusPublished, 0)
	if err != nil || v2 != 2 {
		t.Fatalf("加版本：%d %v", v2, err)
	}
	if lv, err := st.LiveAssetVersion(ctx, a.ID); err != nil || lv.Version != 2 {
		t.Fatalf("线上版本应是 2：%v %v", lv, err)
	}
	if err := st.DeleteAssetVersion(ctx, a.ID, v2); err != nil {
		t.Fatal(err)
	}
	lv, err := st.LiveAssetVersion(ctx, a.ID)
	if err != nil || lv.Version != 1 {
		t.Fatalf("删掉新版本后线上版本应回到 1：%v %v", lv, err)
	}
	if vs, err := st.ListAssetVersions(ctx, a.ID); err != nil || len(vs) != 1 {
		t.Fatalf("版本历史应只剩一条：%v %v", len(vs), err)
	}
	if err := st.DeleteAssetVersion(ctx, a.ID, v2); !errors.Is(err, ErrNotFound) {
		t.Fatalf("删不存在的版本应返回 ErrNotFound：%v", err)
	}
}

// Gate：分类范围不含这个人，或命中禁用策略，都挡住；分类优先报。
func TestAssetGate(t *testing.T) {
	cat := Scope{Departments: []string{"财务部"}}
	a := &Asset{CategoryScope: &cat}
	if got := a.Gate(1, "财务部", nil); got != "" {
		t.Fatalf("分类范围内应放行：%q", got)
	}
	if got := a.Gate(1, "研发部", nil); got != GateCategory {
		t.Fatalf("分类范围外应报 category：%q", got)
	}
	a.Blocks = []Scope{{Roles: []int64{7}}}
	if got := a.Gate(1, "财务部", []int64{7}); got != GateBlocked {
		t.Fatalf("命中禁用策略应报 blocked：%q", got)
	}
	if got := a.Gate(1, "研发部", []int64{7}); got != GateCategory {
		t.Fatalf("两样都不满足时先报分类：%q", got)
	}
	if got := (&Asset{}).Gate(1, "", nil); got != "" {
		t.Fatalf("没有分类和策略时放行：%q", got)
	}
}
