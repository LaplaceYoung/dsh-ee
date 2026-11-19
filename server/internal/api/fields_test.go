package api_test

import (
	"fmt"
	"strings"
	"testing"
)

// 扩展字段和访问规则：文档上标等级，用户上标等级，规则生效后每个出口都按规则过滤；explain 说清楚为什么。
func TestFieldAccessRules(t *testing.T) {
	f := newSvcFixture(t)
	admin := f.admin
	code, body := admin.do("POST", "/api/kb", map[string]any{"slug": "plans", "title": "规划"}, nil)
	expect(t, "建库", code, 201, body)
	kbID := decodeAs[struct{ ID int64 }](t, body).ID
	newDoc := func(title, text string) int64 {
		code, body := admin.upload(fmt.Sprintf("/api/kb/%d/docs", kbID), map[string]string{"title": title, "markdown": "# " + title + "\n\n" + text + "\n"}, nil)
		expect(t, "新建文档", code, 201, body)
		return decodeAs[struct {
			Results []struct{ Doc struct{ ID int64 } }
		}](t, body).Results[0].Doc.ID
	}
	open := newDoc("公开计划", "年度目标是增长。")
	secret := newDoc("核心计划", "年度目标是收购。")
	blank := newDoc("没标等级", "年度目标备忘。")
	alice, aliceDSH := f.user("alice")
	_, bobDSH := f.user("bob")

	mkField := func(object, key string, access bool) int64 {
		code, body := admin.do("POST", "/api/admin/fields", map[string]any{"object": object, "key": key, "label": "等级", "type": "level", "options": []string{"普通", "重要", "核心"}, "access": access}, nil)
		expect(t, "建字段 "+object, code, 201, body)
		return decodeAs[struct{ ID int64 }](t, body).ID
	}
	mkField("document", "level", true)
	mkField("user", "level", true)
	code, body = admin.do("POST", "/api/admin/fields", map[string]any{"object": "document", "key": "note", "label": "备注", "type": "text", "access": true}, nil)
	expect(t, "文本字段不能用于访问控制", code, 400, body)

	set := func(c *client, object string, id int64, vals map[string]any, want int) {
		t.Helper()
		code, body := c.do("PUT", fmt.Sprintf("/api/fields/%s/%d", object, id), map[string]any{"values": vals}, nil)
		expect(t, "改字段值", code, want, body)
	}
	set(admin, "document", open, map[string]any{"level": "普通"}, 200)
	set(admin, "document", secret, map[string]any{"level": "核心"}, 200)
	set(admin, "document", open, map[string]any{"level": "绝密"}, 400)
	// 普通员工不能改用于访问控制的字段
	set(alice, "document", open, map[string]any{"level": "核心"}, 403)
	code, body = admin.do("GET", "/api/admin/users", nil, nil)
	users := decodeAs[struct {
		Users []struct {
			ID       int64
			Username string
		}
	}](t, body)
	ids := map[string]int64{}
	for _, u := range users.Users {
		ids[u.Username] = u.ID
	}
	if ids["alice"] == 0 {
		t.Fatalf("没找到 alice：%s", body)
	}
	set(admin, "user", ids["alice"], map[string]any{"level": "核心"}, 200)
	set(alice, "user", ids["alice"], map[string]any{"level": "核心"}, 403)

	titles := func(d dshClient) []string {
		res := kbText(t, d, "kb__search", map[string]any{"query": "年度目标"})
		var out []string
		for _, h := range res["hits"].([]any) {
			out = append(out, fmt.Sprint(h.(map[string]any)["title"]))
		}
		return out
	}
	// 规则还没建：都能看到
	if got := titles(bobDSH); len(got) != 3 {
		t.Fatalf("没有规则时都能看到：%v", got)
	}
	// 规则要用于访问控制的字段
	code, body = admin.do("POST", "/api/admin/access-rules", map[string]any{"name": "按等级", "enabled": true, "op": "level_gte", "user_field": "level", "doc_field": "level", "missing": "deny"}, nil)
	expect(t, "建规则", code, 201, body)
	ruleID := decodeAs[struct{ ID int64 }](t, body).ID

	if got := strings.Join(titles(bobDSH), ","); got != "公开计划" {
		t.Fatalf("bob（没有等级）只该看到普通的：%s", got)
	}
	if got := titles(aliceDSH); len(got) != 2 || strings.Contains(strings.Join(got, ","), "没标等级") {
		t.Fatalf("alice（核心）该看到普通和核心，没标等级的当作受限：%v", got)
	}
	// 阅读、目录、文档页：看不到就当不存在
	if text, isErr := toolCall(t, bobDSH, asDSH, "kb__read", map[string]any{"doc": secret}); !isErr || !strings.Contains(text, "没有这篇文档") {
		t.Fatalf("bob 读核心文档应当不存在：%s", text)
	}
	browse := kbText(t, bobDSH, "kb__browse", map[string]any{"kb": "plans"})
	if n := len(browse["docs"].([]any)); n != 1 {
		t.Fatalf("bob 的目录里只该有 1 篇：%v", browse)
	}
	bob := newClient(t, f.ts.URL)
	bob.do("POST", "/api/auth/login", map[string]string{"username": "bob", "password": "bob-password-123"}, nil)
	code, body = bob.do("GET", fmt.Sprintf("/api/kb-docs/%d", secret), nil, nil)
	expect(t, "bob 打开核心文档", code, 404, body)
	code, body = bob.do("GET", fmt.Sprintf("/api/kb/%d", kbID), nil, nil)
	expect(t, "bob 看库", code, 200, body)
	if strings.Contains(string(body), "核心计划") {
		t.Fatalf("库详情里不该有 bob 看不到的文档：%s", body)
	}
	code, body = bob.do("POST", "/api/kb/search", map[string]any{"query": "收购"}, nil)
	expect(t, "网页试搜", code, 200, body)
	if strings.Contains(string(body), "核心计划") {
		t.Fatalf("网页试搜也要过滤：%s", body)
	}
	// 指定 docs 搜也不能绕过
	if text, isErr := toolCall(t, bobDSH, asDSH, "kb__search", map[string]any{"query": "收购", "docs": []int64{secret}}); !isErr {
		t.Fatalf("指定看不到的文档搜应报不存在：%s", text)
	}

	// 把库绑进自己的应用、导出带文档的应用文件，或从库生成应用草稿：都不能带出看不到的文档
	code, body = bob.do("POST", "/api/hub", map[string]any{"kind": "app", "slug": "bob-plans", "scope": map[string]any{"users": []int64{ids["bob"]}},
		"spec": map[string]any{"title": "规划助手", "description": "问规划", "persona": "你是规划助手。", "knowledge": []string{"plans"}}}, nil)
	expect(t, "bob 建一个自己用的应用", code, 201, body)
	appID := decodeAs[struct{ Asset struct{ ID int64 } }](t, body).Asset.ID
	code, body = bob.do("GET", fmt.Sprintf("/api/hub/%d/export?docs=1", appID), nil, nil)
	expect(t, "bob 导出应用（带文档）", code, 200, body)
	if strings.Contains(string(body), "核心计划") || strings.Contains(string(body), "收购") || !strings.Contains(string(body), "公开计划") {
		t.Fatalf("导出的文档要按访问规则过滤：%s", body)
	}
	code, body = bob.do("POST", "/api/me/app-drafts", map[string]any{"from": "knowledge", "slug": "plans"}, nil)
	expect(t, "bob 从库生成应用草稿", code, 200, body)
	if strings.Contains(string(body), "核心计划") {
		t.Fatalf("应用草稿里不该出现 bob 看不到的文档：%s", body)
	}

	// explain：说清楚判断经过了哪些规则、卡在哪一条
	code, body = admin.do("POST", "/api/admin/access/explain", map[string]any{"user": "bob", "doc": secret}, nil)
	expect(t, "explain", code, 200, body)
	ex := decodeAs[struct {
		Allowed bool
		Steps   []struct{ Stage, Result, Reason string }
	}](t, body)
	if ex.Allowed || len(ex.Steps) < 2 || ex.Steps[len(ex.Steps)-1].Result != "deny" || !strings.Contains(ex.Steps[len(ex.Steps)-1].Reason, "按最低") {
		t.Fatalf("explain 不对：%s", body)
	}
	// 试算还没应用的配置：把规则改成没填不限制，bob 能看到没标等级的
	code, body = admin.do("POST", "/api/admin/access/simulate", map[string]any{"users": []string{"bob", "alice"}, "kb": "plans", "config": map[string]any{
		"fields": []map[string]any{
			{"object": "document", "key": "level", "label": "等级", "type": "level", "options": []string{"普通", "重要", "核心"}, "access": true},
			{"object": "user", "key": "level", "label": "等级", "type": "level", "options": []string{"普通", "重要", "核心"}, "access": true}},
		"rules": []map[string]any{{"name": "按等级", "enabled": true, "op": "level_gte", "user_field": "level", "doc_field": "level", "missing": "open"}},
	}}, nil)
	expect(t, "simulate", code, 200, body)
	sim := decodeAs[struct {
		Rows []struct {
			User string
			Docs []struct {
				Doc     int64
				Allowed bool
			}
		}
	}](t, body)
	seen := map[string]int{}
	for _, row := range sim.Rows {
		for _, d := range row.Docs {
			if d.Allowed {
				seen[row.User]++
			}
		}
	}
	if seen["bob"] != 2 || seen["alice"] != 3 {
		t.Fatalf("试算结果不对：%v %s", seen, body)
	}
	// 试算不改任何东西
	if got := strings.Join(titles(bobDSH), ","); got != "公开计划" {
		t.Fatalf("试算不该生效：%s", got)
	}
	// 被规则用到的字段不能删、不能关掉访问控制
	code, body = admin.do("GET", "/api/admin/fields", nil, nil)
	docField := int64(0)
	for _, fd := range decodeAs[struct {
		Fields []struct {
			ID     int64
			Object string
		}
	}](t, body).Fields {
		if fd.Object == "document" {
			docField = fd.ID
		}
	}
	code, body = admin.do("DELETE", fmt.Sprintf("/api/admin/fields/%d", docField), nil, nil)
	expect(t, "删被规则用到的字段", code, 409, body)
	code, body = admin.do("PATCH", fmt.Sprintf("/api/admin/fields/%d", docField), map[string]any{"access": false}, nil)
	expect(t, "关掉访问控制", code, 409, body)
	// 停用规则后恢复
	code, body = admin.do("PUT", fmt.Sprintf("/api/admin/access-rules/%d", ruleID), map[string]any{"name": "按等级", "enabled": false, "op": "level_gte", "user_field": "level", "doc_field": "level"}, nil)
	expect(t, "停用规则", code, 200, body)
	if got := titles(bobDSH); len(got) != 3 {
		t.Fatalf("停用后都能看到：%v", got)
	}
	_ = blank
}
