package api_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/api"
	"github.com/hikariming/dsh-ee/server/internal/rbac"
	"github.com/hikariming/dsh-ee/server/internal/secret"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

func TestFeishuNpmReviewCards(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if _, err := api.Bootstrap(ctx, st, "admin", "admin-password-123"); err != nil {
		t.Fatal(err)
	}
	idp := newFakeIdP(t)
	messages := &fakeFeishuMessages{base: idp.Config.Handler}
	idp.Config.Handler = messages
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	box, _ := secret.NewBox(key)
	srv := api.New(st, box, api.Options{DataDir: t.TempDir(), OIDCClient: idp.Client()}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	mkUser := func(username, openID string, permissions ...string) *store.User {
		t.Helper()
		role := &store.Role{Code: "role-" + username, Name: username, Permissions: permissions}
		if err := st.CreateRole(ctx, role); err != nil {
			t.Fatal(err)
		}
		u := &store.User{Username: username, AuthSource: store.AuthOIDC, ExternalDN: "feishu:" + openID, AccountType: store.AccountHuman}
		if err := st.CreateUser(ctx, u); err != nil {
			t.Fatal(err)
		}
		if err := st.SetUserRoles(ctx, u.ID, []int64{role.ID}); err != nil {
			t.Fatal(err)
		}
		return u
	}
	publisher := mkUser("npm-author", "ou-author", rbac.RegistryPublish)
	reviewer := mkUser("npm-reviewer", "ou-reviewer", rbac.PluginsManage)
	mkUser("npm-bystander", "ou-bystander", rbac.RegistryPublish)
	publisherClient := newClient(t, ts.URL)
	if err := st.CreateSession(ctx, secret.Hash(fakeCardSessionID), publisher.ID, time.Hour); err != nil {
		t.Fatal(err)
	}
	baseURL, _ := url.Parse(ts.URL)
	publisherClient.hc.Jar.SetCookies(baseURL, []*http.Cookie{{Name: "dsh_ee_session", Value: fakeCardSessionID}})
	admin := newClient(t, ts.URL)
	code, body := admin.do("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "admin-password-123"}, nil)
	expect(t, "管理员登录", code, http.StatusOK, body)
	code, body = admin.do("PUT", "/api/admin/auth/oidc", map[string]any{
		"enabled": true, "preset": "feishu", "issuer": idp.URL, "clientId": "dsh-ee", "clientSecret": "idp-secret",
		"feishuApiBase": idp.URL, "feishuNotify": true, "feishuNotifyActions": true,
		"feishuVerificationToken": feishuTestToken, "feishuEncryptKey": "ek-npm-card", "autoCreate": true,
	}, nil)
	expect(t, "开启飞书卡片和加密回调", code, http.StatusOK, body)
	code, body = admin.do("GET", "/api/settings", nil, nil)
	expect(t, "读设置", code, http.StatusOK, body)
	set := decodeAs[map[string]any](t, body)
	set["scope"] = "@acme"
	set["baseUrl"] = ts.URL
	code, body = admin.do("PUT", "/api/admin/settings", set, nil)
	expect(t, "设置包作用域", code, http.StatusOK, body)
	code, body = publisherClient.do("POST", "/api/me/npm-tokens", map[string]string{"name": "card-cli", "access": "publish"}, nil)
	expect(t, "创建真正的发布令牌", code, http.StatusCreated, body)
	publishToken := decodeAs[struct{ Token string }](t, body).Token
	publishHeader := map[string]string{"Authorization": "Bearer " + publishToken}

	publish := func(name, version string) map[string]any {
		tgz := makeTgz(t, map[string]any{"name": name, "version": version, "dsh.bundle": "index.js"}, map[string]string{"index.js": "module.exports = true\n"})
		return publishBody(name, version, tgz, map[string]any{"name": name, "version": version, "dsh.bundle": "index.js"}, "latest")
	}
	cardCallback := func(actionID, messageID, operator, decision string) (int, []byte) {
		encrypted, headers := feishuEncrypt(t, "ek-npm-card", cardEvent(actionID, messageID, operator, decision))
		return admin.do("POST", "/api/auth/feishu/card", json.RawMessage(encrypted), headers)
	}
	expectMessage := func(fragment string) {
		t.Helper()
		messages.mu.Lock()
		all := append([]feishuMessage(nil), messages.messages...)
		messages.mu.Unlock()
		for _, msg := range all {
			if strings.Contains(msg.Content, fragment) {
				return
			}
		}
		t.Fatalf("通知中没有 %q", fragment)
	}

	code, body = publisherClient.do("PUT", "/npm/@acme%2Fcli-card", publish("@acme/cli-card", "1.0.0"), publishHeader)
	expect(t, "npm CLI 提交待审插件", code, http.StatusCreated, body)
	actionID, messageID := fakeCardAction(t, messages, 0)
	code, body = cardCallback(actionID, "om-wrong", "ou-reviewer", "approve")
	expectToast(t, "消息号不对不能审批插件", code, body, "error", "对不上")
	code, body = cardCallback(actionID, messageID, "ou-bystander", "approve")
	expectToast(t, "普通发布者不能审自己的插件", code, body, "error", "没有权限")
	if got, err := st.GetVersion(ctx, "@acme/cli-card", "1.0.0"); err != nil || got.Status != store.StatusPending {
		t.Fatalf("未授权动作稿件不得改变状态：%+v err=%v", got, err)
	}
	code, body = admin.do("PATCH", "/api/admin/users/"+itoa(reviewer.ID), map[string]any{"disabled": true}, nil)
	expect(t, "禁用原审批人", code, http.StatusOK, body)
	code, body = cardCallback(actionID, messageID, "ou-reviewer", "approve")
	expectToast(t, "发完卡片后被停用的账号不能审批", code, body, "error", "禁用")
	code, body = admin.do("PATCH", "/api/admin/users/"+itoa(reviewer.ID), map[string]any{"disabled": false}, nil)
	expect(t, "恢复审批人", code, http.StatusOK, body)
	code, body = cardCallback(actionID, messageID, "ou-reviewer", "approve")
	expectToast(t, "加密回调批准插件", code, body, "success", "已通过")
	got, err := st.GetVersion(ctx, "@acme/cli-card", "1.0.0")
	if err != nil || got.Status != store.StatusPublished || got.ReviewedBy == nil || *got.ReviewedBy != reviewer.ID {
		t.Fatalf("卡片批准的版本或审核人不对：%+v err=%v", got, err)
	}
	code, body = cardCallback(actionID, messageID, "ou-reviewer", "approve")
	expectToast(t, "重复插件审批不会重新上架", code, body, "info", "已经处理过")
	expectMessage("@acme/cli-card@1.0.0 · 已通过")

	code, body = publisherClient.do("PUT", "/npm/@acme%2Fcli-card", publish("@acme/cli-card", "1.1.0"), publishHeader)
	expect(t, "再发一版用于驳回", code, http.StatusCreated, body)
	actionID, messageID = fakeCardAction(t, messages, 1)
	code, body = cardCallback(actionID, messageID, "ou-reviewer", "reject")
	expectToast(t, "加密回调驳回插件", code, body, "success", "已驳回")
	rejected, err := st.GetVersion(ctx, "@acme/cli-card", "1.1.0")
	if err != nil || rejected.Status != store.StatusRejected {
		t.Fatalf("卡片驳回的版本不对：%+v err=%v", rejected, err)
	}
	expectMessage("@acme/cli-card@1.1.0 · 已驳回")

	// 网页上传和网页驳回也必须通知发布者；卡片不是别的审批引擎，只是调用同一 registry.Review。
	tgz := makeTgz(t, map[string]any{"name": "@acme/web-card", "version": "0.2.0", "dsh.bundle": "index.js"}, map[string]string{"index.js": "module.exports = true\n"})
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/me/packages", bytes.NewReader(tgz))
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := publisherClient.hc.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	expect(t, "网页上传插件", resp.StatusCode, http.StatusCreated, data)
	_, _ = fakeCardAction(t, messages, 2)
	code, body = admin.do("POST", "/api/admin/review", map[string]any{"name": "@acme/web-card", "version": "0.2.0", "action": "reject", "note": "网页说明"}, nil)
	expect(t, "网页审核插件", code, http.StatusOK, body)
	expectMessage("@acme/web-card@0.2.0 · 已驳回")

	// registry 允许包名 214、semver 128，合计 343 个 ASCII 字节。
	// 对象索引只能限前缀，不能因旧卡片的有限 object_key 丢掉这次合法操作。
	maxName := "@acme/" + strings.Repeat("n", 208)
	maxVersion := "0.0.0-" + strings.Repeat("a", 122)
	if len(maxName)+1+len(maxVersion) != 343 {
		t.Fatal("边界夹具未达到允许的最大对象键长度")
	}
	code, body = publisherClient.do("PUT", "/npm/"+url.PathEscape(maxName), publish(maxName, maxVersion), publishHeader)
	expect(t, "最长合法包名与版本进入待审", code, http.StatusCreated, body)
	actionID, messageID = fakeCardAction(t, messages, 3)
	code, body = cardCallback(actionID, messageID, "ou-reviewer", "approve")
	expectToast(t, "最长合法包名和版本也能审批", code, body, "success", "已通过")
	longPublished, err := st.GetVersion(ctx, maxName, maxVersion)
	if err != nil || longPublished.Status != store.StatusPublished {
		t.Fatalf("完整长对象键的审批结果应保存：%+v err=%v", longPublished, err)
	}
	messages.mu.Lock()
	all := append([]feishuMessage(nil), messages.messages...)
	messages.mu.Unlock()
	for _, msg := range all {
		if strings.Contains(msg.Content, "idp-secret") || strings.Contains(msg.Content, "ek-npm-card") {
			t.Fatalf("通知不能泄漏应用或加密密钥：%s", msg.Content)
		}
	}
}

const fakeCardSessionID = "npm-card-test-session"

func fakeCardAction(t *testing.T, messages *fakeFeishuMessages, index int) (actionID, messageID string) {
	t.Helper()
	return messages.interactiveAction(t, index)
}
