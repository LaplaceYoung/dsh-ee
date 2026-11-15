package api_test

import (
	"context"
	"crypto/rand"
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

// 端到端验证飞书消息按接收者语言渲染：
//   - 审批卡：每个审批人自己的语言；审批人没设语言 → 组织默认；组织也没设 → zh。
//   - 结果通知：申请人/作者自己的语言优先于组织默认。
//   - 按钮 toast：点按钮的人自己的语言；组织默认改语言后后续消息跟着变。
//
// 同时核对参数（标题、版本、路径）确实替换，卡片 JSON 结构（callback behaviors）没变。
func TestFeishuNotificationLanguage(t *testing.T) {
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
	expect(t, "管理员登录", code, http.StatusOK, body)
	super, err := st.GetRoleByCode(ctx, rbac.RoleSuper)
	if err != nil {
		t.Fatal(err)
	}
	member, err := st.GetRoleByCode(ctx, rbac.RoleMember)
	if err != nil {
		t.Fatal(err)
	}
	// 审批人：飞书身份 + 审核权限，语言先留空（跟随组织默认）。
	reviewer := &store.User{Username: "lang-reviewer", AuthSource: store.AuthOIDC, ExternalDN: "feishu:ou-reviewer", AccountType: store.AccountHuman}
	if err := st.CreateUser(ctx, reviewer); err != nil {
		t.Fatal(err)
	}
	if err := st.SetUserRoles(ctx, reviewer.ID, []int64{super.ID}); err != nil {
		t.Fatal(err)
	}
	// 申请人：飞书身份 + 普通员工权限，通过网页会话提交待审内容。
	author := &store.User{Username: "lang-author", DisplayName: "作者", Department: "研发部", AuthSource: store.AuthOIDC,
		ExternalDN: "feishu:ou-author", AccountType: store.AccountHuman}
	if err := st.CreateUser(ctx, author); err != nil {
		t.Fatal(err)
	}
	if err := st.SetUserRoles(ctx, author.ID, []int64{member.ID}); err != nil {
		t.Fatal(err)
	}
	const authorSession = "p5-author-language-session"
	if err := st.CreateSession(ctx, secret.Hash(authorSession), author.ID, time.Hour); err != nil {
		t.Fatal(err)
	}
	authorClient := newClient(t, ts.URL)
	baseURL, _ := url.Parse(ts.URL)
	authorClient.hc.Jar.SetCookies(baseURL, []*http.Cookie{{Name: "dsh_ee_session", Value: authorSession}})

	code, body = admin.do("PUT", "/api/admin/auth/oidc", map[string]any{
		"enabled": true, "preset": "feishu", "issuer": idp.URL, "clientId": "dsh-ee", "clientSecret": "idp-secret",
		"feishuApiBase": idp.URL, "feishuNotify": true, "feishuNotifyActions": true,
		"feishuVerificationToken": feishuTestToken, "autoCreate": true,
	}, nil)
	expect(t, "开启飞书通知与卡片", code, http.StatusOK, body)

	submit := func(slug, title string) {
		t.Helper()
		code, body = authorClient.do("POST", "/api/hub", map[string]any{
			"kind": "skill", "slug": slug, "scope": map[string]any{"departments": []string{"研发部"}},
			"spec": map[string]any{"title": title, "description": "i18n", "body": "检查多语言渲染"},
		}, nil)
		expect(t, "提交待审技能 "+slug, code, http.StatusCreated, body)
	}
	approve := func(cardIndex int, wantLang, wantToast string) {
		t.Helper()
		actionID, messageID := fake.interactiveAction(t, cardIndex)
		code, body = admin.do("POST", "/api/auth/feishu/card", cardEvent(actionID, messageID, "ou-reviewer", "approve"), nil)
		expectToast(t, wantLang+" 审批通过", code, body, "success", wantToast)
	}

	// 1) 组织默认 zh、审批人没设语言 → 卡片中文；参数替换成技能标题和版本。
	submit("lang-card-zh", "中文标题技能")
	cards := feishuCardContents(t, fake)
	if len(cards) != 1 {
		t.Fatalf("应发出 1 张审批卡，实际 %d 张", len(cards))
	}
	zhCard := cards[0]
	for _, want := range []string{"待审核", "通过", "驳回", "中文标题技能", "版本：v1", "/assets/", `"behaviors"`, `"type":"callback"`} {
		if !strings.Contains(zhCard, want) {
			t.Fatalf("中文卡片缺少 %q：%s", want, zhCard)
		}
	}

	// 2) 审批人语言 en（用户偏好优先于组织默认 zh）→ 英文卡片与英文 toast；
	//    作者没设语言 → 结果通知跟组织默认 zh，仍是中文。
	en := "en"
	if err := st.UpdateUser(ctx, reviewer.ID, store.UserPatch{Language: &en}); err != nil {
		t.Fatal(err)
	}
	submit("lang-card-en", "English title skill")
	cards = feishuCardContents(t, fake)
	if len(cards) != 2 {
		t.Fatalf("应发出 2 张审批卡，实际 %d 张", len(cards))
	}
	enCard := cards[1]
	for _, want := range []string{"Pending review", "Approve", "Reject", "English title skill", "Version: v1", "Type: skill", `"behaviors"`} {
		if !strings.Contains(enCard, want) {
			t.Fatalf("英文卡片缺少 %q：%s", want, enCard)
		}
	}
	if strings.Contains(enCard, "待审核") || strings.Contains(enCard, "通过") {
		t.Fatalf("英文审批人的卡片不应再出现中文：%s", enCard)
	}
	approve(1, "英文", "Approved")
	texts := feishuTextContents(t, fake)
	if len(texts) != 1 || !strings.Contains(texts[0], "审批结果") || !strings.Contains(texts[0], "English title skill") || !strings.Contains(texts[0], "已通过") {
		t.Fatalf("作者没设语言时应按组织默认 zh 发结果通知：%v", texts)
	}

	// 3) 组织默认改成 es、审批人清空语言 → 卡片西班牙语；作者语言 hi 优先于组织默认 → 印地语结果通知。
	setFeishuTestOrgLanguage(t, admin, "es")
	empty := ""
	if err := st.UpdateUser(ctx, reviewer.ID, store.UserPatch{Language: &empty}); err != nil {
		t.Fatal(err)
	}
	hi := "hi"
	if err := st.UpdateUser(ctx, author.ID, store.UserPatch{Language: &hi}); err != nil {
		t.Fatal(err)
	}
	submit("lang-card-es", "Título de prueba")
	cards = feishuCardContents(t, fake)
	esCard := cards[2]
	for _, want := range []string{"Pendiente de revisión", "Aprobar", "Rechazar", "Título de prueba", "Versión: v1"} {
		if !strings.Contains(esCard, want) {
			t.Fatalf("西班牙语卡片缺少 %q：%s", want, esCard)
		}
	}
	approve(2, "西班牙语", "Aprobado")
	texts = feishuTextContents(t, fake)
	last := texts[len(texts)-1]
	if !strings.Contains(last, "अनुमोदन परिणाम") || !strings.Contains(last, "स्वीकृत") || !strings.Contains(last, "Título de prueba") {
		t.Fatalf("作者语言 hi 应优先于组织默认 es：%v", texts)
	}

	// 4) 组织默认回到 zh、用户语言都清空 → 回中文（都没有时兜底 zh）。
	setFeishuTestOrgLanguage(t, admin, "zh")
	if err := st.UpdateUser(ctx, author.ID, store.UserPatch{Language: &empty}); err != nil {
		t.Fatal(err)
	}
	submit("lang-card-default", "默认中文标题")
	cards = feishuCardContents(t, fake)
	if lastCard := cards[len(cards)-1]; !strings.Contains(lastCard, "待审核") || !strings.Contains(lastCard, "通过") {
		t.Fatalf("用户语言与组织默认都回落后应发中文卡片：%s", lastCard)
	}
}

// setFeishuTestOrgLanguage 走管理端设置接口改组织默认语言，验证 API 读写链路。
func setFeishuTestOrgLanguage(t *testing.T, admin *client, lang string) {
	t.Helper()
	set := decodeAs[map[string]any](t, mustGet(t, admin, "/api/settings"))
	set["defaultLanguage"] = lang
	code, body := admin.do("PUT", "/api/admin/settings", set, nil)
	expect(t, "改组织默认语言 "+lang, code, http.StatusOK, body)
	if got := decodeAs[map[string]any](t, body)["defaultLanguage"]; got != lang {
		t.Fatalf("组织默认语言应为 %s，得到 %v", lang, got)
	}
}

func feishuCardContents(t *testing.T, fake *fakeFeishuMessages) []string {
	t.Helper()
	fake.mu.Lock()
	defer fake.mu.Unlock()
	var out []string
	for _, m := range fake.messages {
		if m.Type == "interactive" {
			out = append(out, m.Content)
		}
	}
	return out
}

func feishuTextContents(t *testing.T, fake *fakeFeishuMessages) []string {
	t.Helper()
	fake.mu.Lock()
	defer fake.mu.Unlock()
	var out []string
	for _, m := range fake.messages {
		if m.Type == "text" {
			out = append(out, m.Content)
		}
	}
	return out
}
