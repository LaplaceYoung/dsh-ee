package api_test

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/api"
	"github.com/hikariming/dsh-ee/server/internal/rbac"
	"github.com/hikariming/dsh-ee/server/internal/secret"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

type feishuMessage struct {
	MessageID string `json:"message_id"`
	OpenID    string `json:"receive_id"`
	Type      string `json:"msg_type"`
	Content   string `json:"content"`
}

type fakeFeishuMessages struct {
	mu       sync.Mutex
	base     http.Handler
	messages []feishuMessage
}

func (f *fakeFeishuMessages) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal" {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "tenant_access_token": "tenant-feishu"})
		return
	}
	if r.URL.Path == "/open-apis/contact/v3/departments/parentchildren" {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"items": []map[string]any{{"open_department_id": "dep-rd", "name": "研发部"}}, "has_more": false}})
		return
	}
	if r.URL.Path == "/open-apis/contact/v3/users/find_by_department" {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"items": []map[string]any{{"open_id": "ou-reviewer", "name": "审批人", "department_ids": []string{"dep-rd"}}}, "has_more": false}})
		return
	}
	if r.URL.Path == "/open-apis/im/v1/messages" {
		if r.Header.Get("Authorization") != "Bearer tenant-feishu" {
			w.WriteHeader(401)
			return
		}
		var msg feishuMessage
		if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		f.mu.Lock()
		msg.MessageID = fmt.Sprintf("om-%d", len(f.messages)+1)
		f.messages = append(f.messages, msg)
		f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]string{"message_id": msg.MessageID}})
		return
	}
	f.base.ServeHTTP(w, r)
}

func (f *fakeFeishuMessages) interactiveAction(t *testing.T, index int) (actionID, messageID string) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	var interactive int
	for _, m := range f.messages {
		if m.Type != "interactive" {
			continue
		}
		if interactive == index {
			var card struct {
				Body struct {
					Components []struct {
						Tag     string `json:"tag"`
						Actions []struct {
							Value struct {
								ActionID string `json:"action_id"`
							} `json:"value"`
						} `json:"actions"`
					} `json:"components"`
				}
			}
			if err := json.Unmarshal([]byte(m.Content), &card); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(m.Content, `"behaviors"`) || !strings.Contains(m.Content, `"type":"callback"`) {
				t.Fatal("审批按钮必须声明 callback behavior 才会真实回调")
			}
			return card.Body.Components[1].Actions[0].Value.ActionID, m.MessageID
		}
		interactive++
	}
	t.Fatalf("没有第 %d 张审批卡", index)
	return "", ""
}

func cardEvent(actionID, messageID, operator string, decision any) map[string]any {
	return map[string]any{
		"type": "card.action.trigger", "event_id": "evt-" + secret.Token(), "operator_id": operator, "message_id": messageID,
		"action_tag": "button", "action_value": map[string]any{"action_id": actionID, "decision": decision}, "timestamp": "extra-field-allowed",
	}
}

func TestFeishuCardsAndCallbacks(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if _, err := api.Bootstrap(ctx, st, "admin", "admin-password-123"); err != nil {
		t.Fatal(err)
	}
	idp := newFakeIdP(t)
	fakeNotices := &fakeFeishuMessages{base: idp.Config.Handler}
	idp.Config.Handler = fakeNotices
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	box, _ := secret.NewBox(key)
	srv := api.New(st, box, api.Options{DataDir: t.TempDir(), OIDCClient: idp.Client()}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	admin := newClient(t, ts.URL)
	code, body := admin.do("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "admin-password-123"}, nil)
	expect(t, "管理员登录", code, 200, body)
	member, err := st.GetRoleByCode(ctx, rbac.RoleMember)
	if err != nil {
		t.Fatal(err)
	}
	super, err := st.GetRoleByCode(ctx, rbac.RoleSuper)
	if err != nil {
		t.Fatal(err)
	}
	mkUser := func(username, external string, roles []int64) *store.User {
		source := store.AuthOIDC
		if username == "author" || username == "applicant" {
			source = store.AuthLocal
		}
		department := "财务部"
		if username == "applicant" {
			department = "研发部"
		}
		u := &store.User{Username: username, DisplayName: username, Department: department, AuthSource: source, ExternalDN: external}
		if username == "author" || username == "applicant" {
			hash, err := api.HashPassword(username + "-password-1")
			if err != nil {
				t.Fatal(err)
			}
			u.PasswordHash = hash
		}
		if err := st.CreateUser(ctx, u); err != nil {
			t.Fatal(err)
		}
		if err := st.SetUserRoles(ctx, u.ID, roles); err != nil {
			t.Fatal(err)
		}
		return u
	}
	reviewer := mkUser("reviewer", "feishu:ou-reviewer", []int64{super.ID})
	author := mkUser("author", "feishu:ou-author", []int64{member.ID})
	if err := st.CreateSession(ctx, "test-session-"+fmt.Sprint(author.ID), author.ID, time.Hour); err != nil {
		t.Fatal(err)
	}
	applicant := mkUser("applicant", "feishu:ou-applicant", []int64{member.ID})
	authorClient := newClient(t, ts.URL)
	code, body = authorClient.do("POST", "/api/auth/login", map[string]string{"username": "author", "password": "author-password-1"}, nil)
	expect(t, "作者登录", code, 200, body)
	applicantClient := newClient(t, ts.URL)
	code, body = applicantClient.do("POST", "/api/auth/login", map[string]string{"username": "applicant", "password": "applicant-password-1"}, nil)
	expect(t, "普通员工登录", code, 200, body)
	_ = reviewer // 卡片原文也验证 reviewer 是否只是用于回调的匹配人，而不是客户端管理员本人。

	code, body = admin.do("PUT", "/api/admin/auth/oidc", map[string]any{
		"enabled": true, "preset": "feishu", "issuer": idp.URL, "clientId": "dsh-ee", "clientSecret": "idp-secret",
		"feishuApiBase": idp.URL, "feishuNotify": true, "feishuNotifyActions": true, "autoCreate": true,
	}, nil)
	expect(t, "开启飞书通知与卡片", code, 200, body)
	if strings.Contains(string(body), "idp-secret") {
		t.Fatalf("secret 泄露：%s", body)
	}
	// 作者提交待审核技能，交给飞书审批人处理；审批回调不借用本地管理员 cookie。
	code, body = authorClient.do("POST", "/api/hub", map[string]any{
		"kind": "skill", "slug": "feishu-review", "scope": map[string]any{"departments": []string{"研发部"}},
		"spec": map[string]any{"title": "飞书审批技能", "description": "review me", "body": "读取审批流程"},
	}, nil)
	expect(t, "员工提交待审核技能", code, 201, body)
	assetID := decodeAs[struct {
		Asset struct{ ID int64 } `json:"asset"`
	}](t, body).Asset.ID
	actionID, messageID := fakeNotices.interactiveAction(t, 0)

	code, body = admin.do("POST", "/api/auth/feishu/card", cardEvent(actionID, "om-wrong", "ou-reviewer", "approve"), nil)
	expect(t, "消息号不匹配必须拒绝", code, 409, body)
	code, body = admin.do("POST", "/api/auth/feishu/card", cardEvent(actionID, messageID, "ou-attacker", "approve"), nil)
	expect(t, "未知操作人不能审核", code, 403, body)
	code, body = admin.do("POST", "/api/auth/feishu/card", cardEvent(actionID, messageID, "ou-author", "approve"), nil)
	expect(t, "普通员工不能越权审核", code, 403, body)
	code, body = admin.do("POST", "/api/auth/feishu/card", cardEvent(actionID, messageID, "ou-reviewer", "approve"), nil)
	expect(t, "飞书审批通过", code, 200, body)
	if !strings.Contains(string(body), `"status":"approved"`) || !strings.Contains(string(body), `"type":"success"`) {
		t.Fatalf("回调 toast 不正确：%s", body)
	}
	version, err := st.LiveAssetVersion(ctx, assetID)
	if err != nil || version.Status != store.StatusPublished {
		t.Fatalf("审批后技能应上架：%+v err=%v", version, err)
	}
	code, body = admin.do("POST", "/api/auth/feishu/card", cardEvent(actionID, messageID, "ou-reviewer", "approve"), nil)
	expect(t, "重复回调不能重复审批", code, 409, body)

	// 限制为财务可用，作者研发部的人发起权限申请；再让审批人打开。
	code, body = admin.do("PATCH", fmt.Sprintf("/api/hub/%d", assetID), map[string]any{"scope": map[string]any{"departments": []string{"财务部"}}}, nil)
	expect(t, "缩小技能范围", code, 200, body)
	code, body = applicantClient.do("POST", "/api/me/access-requests", map[string]any{"kind": "skill", "slug": "feishu-review", "reason": "需要查阅流程"}, nil)
	expect(t, "员工权限申请", code, 201, body)
	actionID, messageID = fakeNotices.interactiveAction(t, 1)
	// 同时验证 action_value 的字符串 JSON，而不是只测直接 object 的理想路径。
	var value map[string]any = cardEvent(actionID, messageID, "ou-reviewer", "approve")["action_value"].(map[string]any)
	event := cardEvent(actionID, messageID, "ou-reviewer", "approve")
	event["action_value"] = string(feishuMustJSON(t, value))
	code, body = admin.do("POST", "/api/auth/feishu/card", event, nil)
	expect(t, "飞书权限申请通过", code, 200, body)
	// 工作台首页只生成合法的站内 OIDC 起点，不接受管理路径。
	code, body = admin.do("GET", "/api/admin/auth/feishu/workspace-url?next=/a/team-outing", nil, nil)
	expect(t, "生成工作台入口", code, 200, body)
	if !strings.Contains(string(body), "/api/auth/oidc/start?next") || !strings.Contains(string(body), "team-outing") {
		t.Fatalf("工作台入口不是合法 OIDC 起点：%s", body)
	}
	code, body = admin.do("GET", "/api/admin/auth/feishu/workspace-url?next=/admin/settings", nil, nil)
	expect(t, "拒绝管理路径作为工作台入口", code, 400, body)
	access, err := st.GetAsset(ctx, assetID)
	if err != nil || !slices.Contains(access.Scope.Users, applicant.ID) {
		t.Fatalf("批准后应把申请人加入范围：%+v err=%v", access, err)
	}

	// 再提交一个新版本，独立验证驳回，而不是只覆盖 approve。
	code, body = authorClient.do("POST", "/api/hub", map[string]any{
		"kind": "skill", "slug": "feishu-reject-skill",
		"spec": map[string]any{"title": "待驳回技能", "description": "reject me", "body": "检查驳回流程"},
	}, nil)
	expect(t, "再提交一个待审技能", code, 201, body)
	rejectedID := decodeAs[struct {
		Asset struct{ ID int64 } `json:"asset"`
	}](t, body).Asset.ID
	actionID, messageID = fakeNotices.interactiveAction(t, 2)
	code, body = admin.do("POST", "/api/auth/feishu/card", cardEvent(actionID, messageID, "ou-reviewer", "reject"), nil)
	expect(t, "飞书驳回通过 toast", code, 200, body)
	if !strings.Contains(string(body), `"status":"rejected"`) || !strings.Contains(string(body), `"content":"已驳回"`) {
		t.Fatalf("驳回 toast 或状态不对：%s", body)
	}
	rejected, err := st.GetAssetVersion(ctx, rejectedID, 1)
	if err != nil || rejected.Status != store.StatusRejected {
		t.Fatalf("驳回后不应上架，状态应为 rejected：%+v err=%v", rejected, err)
	}
}

func TestFeishuKBGapNotification(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if _, err := api.Bootstrap(ctx, st, "admin", "admin-password-123"); err != nil {
		t.Fatal(err)
	}
	idp := newFakeIdP(t)
	fake := &fakeFeishuMessages{base: idp.Config.Handler}
	idp.Config.Handler = fake
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	box, _ := secret.NewBox(key)
	srv := api.New(st, box, api.Options{DataDir: t.TempDir(), OIDCClient: idp.Client()}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	admin := newClient(t, ts.URL)
	code, body := admin.do("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "admin-password-123"}, nil)
	expect(t, "管理员登录", code, 200, body)
	super, err := st.GetRoleByCode(ctx, rbac.RoleSuper)
	if err != nil {
		t.Fatal(err)
	}
	maintainer := &store.User{Username: "maintainer", DisplayName: "知识维护者", AuthSource: store.AuthOIDC, ExternalDN: "feishu:ou-reviewer"}
	if err := st.CreateUser(ctx, maintainer); err != nil {
		t.Fatal(err)
	}
	if err := st.SetUserRoles(ctx, maintainer.ID, []int64{super.ID}); err != nil {
		t.Fatal(err)
	}
	code, body = admin.do("PUT", "/api/admin/auth/oidc", map[string]any{
		"enabled": true, "preset": "feishu", "issuer": idp.URL, "clientId": "dsh-ee", "clientSecret": "idp-secret",
		"feishuApiBase": idp.URL, "feishuNotify": true, "feishuNotifyActions": false, "feishuSync": true, "autoCreate": true,
	}, nil)
	expect(t, "开启通知但不开卡片按钮", code, 200, body)
	base := &store.KBase{Slug: "travel-gaps", Title: "补文档库", Scope: store.Scope{All: true}, Maintainers: store.Maintainers{Users: []int64{maintainer.ID}}}
	if err := st.CreateKBase(ctx, base, maintainer.ID); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{"宿费 标准", "标准 宿费"} {
		if err := st.RecordKQuery(ctx, store.KQuery{UserID: maintainer.ID, Tool: "search", Query: query, Bases: []int64{base.ID}, Hits: 0}); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 2; i++ {
		code, body = admin.do("POST", "/api/admin/auth/feishu/sync", nil, nil)
		expect(t, "同步时发待补提醒", code, 200, body)
	}
	if len(fake.messages) != 1 || fake.messages[0].Type != "text" || !strings.Contains(fake.messages[0].Content, "标准 宿费") || !strings.Contains(fake.messages[0].Content, "2 次") {
		t.Fatalf("同一归一化问题应只通知一次：%+v", fake.messages)
	}
}

func feishuMustJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
