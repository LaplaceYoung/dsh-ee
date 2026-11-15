package api_test

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
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

const feishuTestToken = "vt-test-token"

// expectToast：来源对了以后，结果一律是 HTTP 200 加一条 toast，响应体里只有 toast。
func expectToast(t *testing.T, what string, code int, body []byte, kind, contains string) {
	t.Helper()
	var out map[string]struct{ Type, Content string }
	if code != 200 || json.Unmarshal(body, &out) != nil || len(out) != 1 || out["toast"].Type != kind || !strings.Contains(out["toast"].Content, contains) {
		t.Fatalf("%s：应回 200 和 %s toast（含「%s」），得到 %d %s", what, kind, contains, code, body)
	}
}

// feishuEncrypt 按飞书的办法加密一份回调，并算出签名头。
func feishuEncrypt(t *testing.T, encryptKey string, event any) (body []byte, headers map[string]string) {
	t.Helper()
	plain := feishuMustJSON(t, event)
	pad := aes.BlockSize - len(plain)%aes.BlockSize
	plain = append(plain, bytes.Repeat([]byte{byte(pad)}, pad)...)
	key := sha256.Sum256([]byte(encryptKey))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		t.Fatal(err)
	}
	iv := make([]byte, aes.BlockSize)
	_, _ = rand.Read(iv)
	out := make([]byte, len(plain))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(out, plain)
	body, _ = json.Marshal(map[string]string{"encrypt": base64.StdEncoding.EncodeToString(append(iv, out...))})
	sum := sha256.Sum256(append([]byte("1700000000"+"nonce-1"+encryptKey), body...))
	return body, map[string]string{"X-Lark-Request-Timestamp": "1700000000", "X-Lark-Request-Nonce": "nonce-1", "X-Lark-Signature": hex.EncodeToString(sum[:])}
}

func cardEvent(actionID, messageID, operator string, decision any) map[string]any {
	// 飞书 card.action.trigger 的真实结构（schema 2.0）
	return map[string]any{
		"schema": "2.0",
		"header": map[string]any{"event_id": "evt-" + secret.Token(), "token": feishuTestToken, "create_time": "1700000000000", "event_type": "card.action.trigger", "tenant_key": "t1", "app_id": "dsh-ee"},
		"event": map[string]any{
			"operator": map[string]any{"tenant_key": "t1", "open_id": operator, "union_id": "on-x"},
			"token":    "c-update-token",
			"action":   map[string]any{"value": map[string]any{"action_id": actionID, "decision": decision}, "tag": "button", "timezone": "Asia/Shanghai"},
			"host":     "im_message",
			"context":  map[string]any{"open_message_id": messageID, "open_chat_id": "oc-x"},
		},
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
	mkUser("bystander", "feishu:ou-bystander", []int64{member.ID})
	_ = reviewer // 卡片原文也验证 reviewer 是否只是用于回调的匹配人，而不是客户端管理员本人。

	code, body = admin.do("PUT", "/api/admin/auth/oidc", map[string]any{
		"enabled": true, "preset": "feishu", "issuer": idp.URL, "clientId": "dsh-ee", "clientSecret": "idp-secret",
		"feishuApiBase": idp.URL, "feishuNotify": true, "feishuNotifyActions": true, "autoCreate": true,
	}, nil)
	expect(t, "没填 Verification Token 不能开卡片审批", code, 400, body)
	code, body = admin.do("PUT", "/api/admin/auth/oidc", map[string]any{
		"enabled": true, "preset": "feishu", "issuer": idp.URL, "clientId": "dsh-ee", "clientSecret": "idp-secret",
		"feishuApiBase": idp.URL, "feishuNotify": true, "feishuNotifyActions": true, "feishuVerificationToken": feishuTestToken, "autoCreate": true,
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

	// 公开接口：token 不对、别的应用发来的、旧的扁平格式，都不处理
	forged := cardEvent(actionID, messageID, "ou-reviewer", "approve")
	forged["header"].(map[string]any)["token"] = "guess"
	code, body = admin.do("POST", "/api/auth/feishu/card", forged, nil)
	expect(t, "Verification Token 不对", code, 403, body)
	other := cardEvent(actionID, messageID, "ou-reviewer", "approve")
	other["header"].(map[string]any)["app_id"] = "cli_other"
	code, body = admin.do("POST", "/api/auth/feishu/card", other, nil)
	expect(t, "别的应用的回调", code, 403, body)
	code, body = admin.do("POST", "/api/auth/feishu/card", map[string]any{"type": "card.action.trigger", "event_id": "e", "operator_id": "ou-reviewer", "message_id": messageID,
		"action_tag": "button", "action_value": map[string]any{"action_id": actionID, "decision": "approve"}}, nil)
	expect(t, "不带 token 的自造格式", code, 403, body)
	if v, err := st.LiveAssetVersion(ctx, assetID); err == nil && v.Status == store.StatusPublished {
		t.Fatal("伪造的回调不该让版本上架")
	}
	// 配置回调地址时飞书发的验证请求
	code, body = admin.do("POST", "/api/auth/feishu/card", map[string]any{"type": "url_verification", "token": feishuTestToken, "challenge": "abc123"}, nil)
	expect(t, "url_verification", code, 200, body)
	if !strings.Contains(string(body), `"challenge":"abc123"`) {
		t.Fatalf("应原样回 challenge：%s", body)
	}
	code, body = admin.do("POST", "/api/auth/feishu/card", map[string]any{"type": "url_verification", "token": "guess", "challenge": "abc123"}, nil)
	expect(t, "url_verification 的 token 不对", code, 403, body)

	code, body = admin.do("POST", "/api/auth/feishu/card", cardEvent(actionID, "om-wrong", "ou-reviewer", "approve"), nil)
	expectToast(t, "消息号不匹配必须拒绝", code, body, "error", "对不上")
	code, body = admin.do("POST", "/api/auth/feishu/card", cardEvent(actionID, messageID, "ou-attacker", "approve"), nil)
	expectToast(t, "未知操作人不能审核", code, body, "error", "未在 dsh-ee 开通")
	code, body = admin.do("POST", "/api/auth/feishu/card", cardEvent(actionID, messageID, "ou-author", "approve"), nil)
	expectToast(t, "没有飞书身份的本地账号不能审核", code, body, "error", "未在 dsh-ee 开通")
	// 有飞书账号但没有审核权限的人点了（比如卡片被转发）：告诉他没权限，卡片不作废，审批人还能用
	code, body = admin.do("POST", "/api/auth/feishu/card", cardEvent(actionID, messageID, "ou-bystander", "approve"), nil)
	expectToast(t, "普通员工不能越权审核", code, body, "error", "没有权限")
	if v, err := st.LiveAssetVersion(ctx, assetID); err == nil && v.Status == store.StatusPublished {
		t.Fatal("没权限的人点了不该上架")
	}
	code, body = admin.do("POST", "/api/auth/feishu/card", cardEvent(actionID, messageID, "ou-reviewer", "approve"), nil)
	expectToast(t, "飞书审批通过", code, body, "success", "已通过")
	version, err := st.LiveAssetVersion(ctx, assetID)
	if err != nil || version.Status != store.StatusPublished {
		t.Fatalf("审批后技能应上架：%+v err=%v", version, err)
	}
	code, body = admin.do("POST", "/api/auth/feishu/card", cardEvent(actionID, messageID, "ou-reviewer", "approve"), nil)
	expectToast(t, "重复回调不能重复审批", code, body, "info", "已经处理过")

	// 限制为财务可用，作者研发部的人发起权限申请；再让审批人打开。
	code, body = admin.do("PATCH", fmt.Sprintf("/api/hub/%d", assetID), map[string]any{"scope": map[string]any{"departments": []string{"财务部"}}}, nil)
	expect(t, "缩小技能范围", code, 200, body)
	code, body = applicantClient.do("POST", "/api/me/access-requests", map[string]any{"kind": "skill", "slug": "feishu-review", "reason": "需要查阅流程"}, nil)
	expect(t, "员工权限申请", code, 201, body)
	actionID, messageID = fakeNotices.interactiveAction(t, 1)
	// 同时验证 action_value 的字符串 JSON，而不是只测直接 object 的理想路径。
	event := cardEvent(actionID, messageID, "ou-reviewer", "approve")
	action := event["event"].(map[string]any)["action"].(map[string]any)
	action["value"] = string(feishuMustJSON(t, action["value"].(map[string]any)))
	code, body = admin.do("POST", "/api/auth/feishu/card", event, nil)
	expectToast(t, "飞书权限申请通过", code, body, "success", "已通过")
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
	expectToast(t, "飞书驳回", code, body, "success", "已驳回")
	rejected, err := st.GetAssetVersion(ctx, rejectedID, 1)
	if err != nil || rejected.Status != store.StatusRejected {
		t.Fatalf("驳回后不应上架，状态应为 rejected：%+v err=%v", rejected, err)
	}

	// 填了 Encrypt Key：回调按加密的收，密钥不回显
	settings := decodeAs[map[string]any](t, mustGet(t, admin, "/api/admin/auth/oidc"))
	delete(settings, "clientSecret")
	settings["feishuEncryptKey"] = "ek-secret-1"
	code, body = admin.do("PUT", "/api/admin/auth/oidc", settings, nil)
	expect(t, "填 Encrypt Key", code, 200, body)
	view := string(mustGet(t, admin, "/api/admin/auth/oidc"))
	if strings.Contains(view, "ek-secret-1") || !strings.Contains(view, `"feishuEncryptKeySet":true`) {
		t.Fatalf("Encrypt Key 不该回显，只说填没填：%s", view)
	}
	code, body = authorClient.do("POST", "/api/hub", map[string]any{
		"kind": "skill", "slug": "feishu-encrypted-skill",
		"spec": map[string]any{"title": "加密回调技能", "description": "encrypted", "body": "检查加密回调"},
	}, nil)
	expect(t, "提交第三个待审技能", code, 201, body)
	encID := decodeAs[struct {
		Asset struct{ ID int64 } `json:"asset"`
	}](t, body).Asset.ID
	actionID, messageID = fakeNotices.interactiveAction(t, 3)
	good := cardEvent(actionID, messageID, "ou-reviewer", "approve")
	code, body = admin.do("POST", "/api/auth/feishu/card", good, nil)
	expect(t, "配了 Encrypt Key 后不收明文", code, 403, body)
	enc, hdr := feishuEncrypt(t, "wrong-key", good)
	code, body = admin.do("POST", "/api/auth/feishu/card", json.RawMessage(enc), hdr)
	expect(t, "用别的密钥加密的", code, 403, body)
	enc, hdr = feishuEncrypt(t, "ek-secret-1", good)
	hdr["X-Lark-Signature"] = strings.Repeat("0", 64)
	code, body = admin.do("POST", "/api/auth/feishu/card", json.RawMessage(enc), hdr)
	expect(t, "签名不对", code, 403, body)
	if v, err := st.GetAssetVersion(ctx, encID, 1); err != nil || v.Status != store.StatusPending {
		t.Fatalf("没通过来源校验的回调不该改状态：%+v err=%v", v, err)
	}
	// 配置回调地址时的验证请求也是加密的，不带签名
	enc, _ = feishuEncrypt(t, "ek-secret-1", map[string]any{"type": "url_verification", "token": feishuTestToken, "challenge": "enc-challenge"})
	code, body = admin.do("POST", "/api/auth/feishu/card", json.RawMessage(enc), nil)
	expect(t, "加密的 url_verification", code, 200, body)
	if !strings.Contains(string(body), `"challenge":"enc-challenge"`) {
		t.Fatalf("应原样回 challenge：%s", body)
	}
	enc, hdr = feishuEncrypt(t, "ek-secret-1", good)
	code, body = admin.do("POST", "/api/auth/feishu/card", json.RawMessage(enc), hdr)
	expectToast(t, "加密回调审批通过", code, body, "success", "已通过")
	if v, err := st.GetAssetVersion(ctx, encID, 1); err != nil || v.Status != store.StatusPublished {
		t.Fatalf("加密回调通过后应上架：%+v err=%v", v, err)
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

// 飞书卡片审批与网页审批是同一张 pending 申请上的两条入口：每个审批人各有一张卡片，
// 所以「A 在网页通过、B 在飞书驳回」完全可能并发。卡片回调也必须先 CAS 抢占、抢到才授权；
// 抢不到时留失败终态并说明原因，不能出现「申请已被驳回、员工却有了权限」。
func TestFeishuApplyAccessCardCAS(t *testing.T) {
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
	adminUser, err := st.GetUserByUsername(ctx, "admin")
	if err != nil {
		t.Fatal(err)
	}
	super, err := st.GetRoleByCode(ctx, rbac.RoleSuper)
	if err != nil {
		t.Fatal(err)
	}
	member, err := st.GetRoleByCode(ctx, rbac.RoleMember)
	if err != nil {
		t.Fatal(err)
	}
	reviewer := &store.User{Username: "reviewer", DisplayName: "飞书审批人", Department: "研发部", AuthSource: store.AuthOIDC, ExternalDN: "feishu:ou-reviewer"}
	if err := st.CreateUser(ctx, reviewer); err != nil {
		t.Fatal(err)
	}
	if err := st.SetUserRoles(ctx, reviewer.ID, []int64{super.ID}); err != nil {
		t.Fatal(err)
	}
	hash, err := api.HashPassword("applicant-password-1")
	if err != nil {
		t.Fatal(err)
	}
	applicant := &store.User{Username: "applicant", DisplayName: "申请人", Department: "研发部", AuthSource: store.AuthLocal, PasswordHash: hash}
	if err := st.CreateUser(ctx, applicant); err != nil {
		t.Fatal(err)
	}
	if err := st.SetUserRoles(ctx, applicant.ID, []int64{member.ID}); err != nil {
		t.Fatal(err)
	}
	applicantClient := newClient(t, ts.URL)
	code, body = applicantClient.do("POST", "/api/auth/login", map[string]string{"username": "applicant", "password": "applicant-password-1"}, nil)
	expect(t, "申请人登录", code, 200, body)

	code, body = admin.do("PUT", "/api/admin/auth/oidc", map[string]any{
		"enabled": true, "preset": "feishu", "issuer": idp.URL, "clientId": "dsh-ee", "clientSecret": "idp-secret",
		"feishuApiBase": idp.URL, "feishuNotify": true, "feishuNotifyActions": true,
		"feishuVerificationToken": feishuTestToken, "autoCreate": true,
	}, nil)
	expect(t, "开启飞书通知与卡片", code, 200, body)

	// 管理员建一个只给财务部的技能：管理员提交直接上架，不会额外发审批卡。
	code, body = admin.do("POST", "/api/hub", map[string]any{
		"kind": "skill", "slug": "feishu-cas-skill", "delivery": "auto",
		"scope": map[string]any{"departments": []string{"财务部"}},
		"spec":  map[string]any{"title": "卡片回归技能", "description": "回归", "body": "审批顺序"},
	}, nil)
	expect(t, "建技能", code, 201, body)
	assetID := decodeAs[struct {
		Asset struct{ ID int64 } `json:"asset"`
	}](t, body).Asset.ID

	apply := func() int64 {
		t.Helper()
		code, body := applicantClient.do("POST", "/api/me/access-requests", map[string]any{"kind": "skill", "slug": "feishu-cas-skill", "reason": "需要查阅"}, nil)
		expect(t, "员工申请权限", code, 201, body)
		return decodeAs[struct {
			ID int64 `json:"id"`
		}](t, body).ID
	}
	granted := func() bool {
		t.Helper()
		a, err := st.GetAsset(ctx, assetID)
		if err != nil {
			t.Fatal(err)
		}
		return slices.Contains(a.Scope.Users, applicant.ID)
	}

	// 1) 另一位审批人先在网页驳回；后到的飞书卡片必须留失败终态，不授权。
	id := apply()
	actionID, messageID := fake.interactiveAction(t, 0)
	if err := st.ResolveAccessRequest(ctx, id, "rejected", adminUser.ID, "网页先驳回"); err != nil {
		t.Fatal(err)
	}
	code, body = admin.do("POST", "/api/auth/feishu/card", cardEvent(actionID, messageID, "ou-reviewer", "approve"), nil)
	expectToast(t, "已被别人处理的卡片", code, body, "error", "已被其他人处理")
	if granted() {
		t.Fatal("申请已被驳回，后到的飞书卡片不能授权")
	}
	q, err := st.GetAccessRequest(ctx, id)
	if err != nil || q.Status != "rejected" {
		t.Fatalf("申请应保持网页驳回的终态：%+v err=%v", q, err)
	}
	action, err := st.FeishuCardAction(ctx, secret.Hash(actionID))
	if err != nil || action.Status != store.FeishuActionFailed {
		t.Fatalf("失败的卡片应留 failed 终态：%+v err=%v", action, err)
	}

	// 2) 卡片抢到 CAS 后授权失败：申请退回待处理，卡片留可重试的失败说明，不静默停在 approved。
	id = apply()
	actionID, messageID = fake.interactiveAction(t, 1)
	if _, err := st.DB().ExecContext(ctx, "UPDATE connection_requests SET asset_id = ? WHERE id = ?", int64(1)<<40, id); err != nil {
		t.Fatalf("把申请目标改成不存在的能力（制造授权失败）：%v", err)
	}
	code, body = admin.do("POST", "/api/auth/feishu/card", cardEvent(actionID, messageID, "ou-reviewer", "approve"), nil)
	expectToast(t, "授权失败的卡片", code, body, "error", "退回待处理")
	if granted() {
		t.Fatal("授权失败时不该留下权限")
	}
	q, err = st.GetAccessRequest(ctx, id)
	if err != nil || q.Status != "pending" || q.HandledBy != nil {
		t.Fatalf("授权失败应退回干净的待处理状态：%+v err=%v", q, err)
	}
	action, err = st.FeishuCardAction(ctx, secret.Hash(actionID))
	if err != nil || action.Status != store.FeishuActionFailed {
		t.Fatalf("授权失败的卡片应留 failed 终态：%+v err=%v", action, err)
	}
}
