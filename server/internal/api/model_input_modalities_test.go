package api_test

import (
	"context"
	"crypto/rand"
	"io"
	"log/slog"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/hikariming/dsh-ee/server/internal/api"
	"github.com/hikariming/dsh-ee/server/internal/rbac"
	"github.com/hikariming/dsh-ee/server/internal/secret"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// 模型输入模态：管理端 CRUD/规范化、/v1/models additive capabilities.input、
// 发现文档与 /v1/capabilities 的能力矩阵。用例不依赖具体模型名。

type modalityFixture struct {
	st     *store.Store
	ts     *httptest.Server
	admin  *client
	token  string
	vision string
	text   string
	docs   string
}

func newModalityFixture(t *testing.T) *modalityFixture {
	t.Helper()
	st := openTestStore(t)
	if _, err := api.Bootstrap(context.Background(), st, "admin", "admin-password-123"); err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	box, _ := secret.NewBox(key)
	srv := api.New(st, box, api.Options{DataDir: t.TempDir()}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	admin := newClient(t, ts.URL)
	code, body := admin.do("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "admin-password-123"}, nil)
	expect(t, "管理员登录", code, 200, body)

	code, body = admin.do("POST", "/api/admin/providers", map[string]any{
		"name": "modality-upstream", "vendor": "custom", "base_url": "http://127.0.0.1:1/v1", "api_key": "sk-modality",
	}, nil)
	expect(t, "创建上游", code, 201, body)
	providerID := decode[struct{ ID int64 }](t, body).ID

	code, body = admin.do("POST", "/api/admin/models", map[string]any{
		"provider_id": providerID,
		"items": []map[string]any{
			{"upstream_model": "vendor-vision", "name": "company-vision", "input_modalities": []string{"image", "text"}},
			{"upstream_model": "vendor-text", "name": "company-text"},
			{"upstream_model": "vendor-docs", "name": "company-docs", "input_modalities": []string{"file"}},
		},
	}, nil)
	expect(t, "创建模型", code, 201, body)

	code, body = admin.do("POST", "/api/admin/users", map[string]any{
		"username": "modality-user", "password": "modality-password-1", "department": "研发效能部",
	}, nil)
	expect(t, "创建员工", code, 201, body)
	user := decode[store.User](t, body)
	limit := int64(1_000_000)
	code, body = admin.do("PATCH", "/api/admin/users/"+itoa(user.ID)+"/quota", map[string]any{"monthly_token_limit": limit}, nil)
	expect(t, "分配额度", code, 200, body)

	token := &store.AccessToken{
		UserID: user.ID, Name: "ee_at_modality", Prefix: "ee_at_modality", Source: "manual",
		Scopes: []string{rbac.ScopeModels}, ClientName: "test-client", CreatedBy: user.ID,
	}
	if err := st.CreateAccessToken(context.Background(), token, secret.Hash("ee_at_modality")); err != nil {
		t.Fatal(err)
	}
	return &modalityFixture{
		st: st, ts: ts, admin: admin, token: "ee_at_modality",
		vision: "company-vision", text: "company-text", docs: "company-docs",
	}
}

func TestModelInputModalitiesAdmin(t *testing.T) {
	f := newModalityFixture(t)

	// 列表返回规范化后的输入模态：text 始终存在、规范顺序 text/image/file、未知值被拒。
	code, body := f.admin.do("GET", "/api/admin/models", nil, nil)
	expect(t, "管理端模型列表", code, 200, body)
	list := decode[struct {
		Models []struct {
			Name            string   `json:"name"`
			InputModalities []string `json:"input_modalities"`
		} `json:"models"`
	}](t, body)
	modalities := map[string][]string{}
	for _, m := range list.Models {
		modalities[m.Name] = m.InputModalities
	}
	if got := modalities[f.vision]; !reflect.DeepEqual(got, []string{"text", "image"}) {
		t.Fatalf("company-vision 输入模态 = %v，期望 [text image]", got)
	}
	if got := modalities[f.text]; !reflect.DeepEqual(got, []string{"text"}) {
		t.Fatalf("缺省模型输入模态 = %v，期望 [text]", got)
	}
	if got := modalities[f.docs]; !reflect.DeepEqual(got, []string{"text", "file"}) {
		t.Fatalf("company-docs 输入模态 = %v，期望 [text file]", got)
	}

	// 找 company-vision 的 ID 用于 PATCH。
	code, body = f.admin.do("GET", "/api/admin/models", nil, nil)
	expect(t, "再次读模型", code, 200, body)
	ids := decode[struct {
		Models []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"models"`
	}](t, body)
	var visionID int64
	for _, m := range ids.Models {
		if m.Name == f.vision {
			visionID = m.ID
		}
	}
	if visionID == 0 {
		t.Fatal("找不到 company-vision")
	}

	code, body = f.admin.do("PATCH", "/api/admin/models/"+itoa(visionID), map[string]any{
		"input_modalities": []string{"file", "image"},
	}, nil)
	expect(t, "更新输入模态", code, 200, body)
	patched := decode[struct {
		InputModalities []string `json:"input_modalities"`
	}](t, body)
	if !reflect.DeepEqual(patched.InputModalities, []string{"text", "image", "file"}) {
		t.Fatalf("PATCH 后输入模态 = %v，期望 [text image file]", patched.InputModalities)
	}

	// 非法模态 400，且不写入。
	code, body = f.admin.do("PATCH", "/api/admin/models/"+itoa(visionID), map[string]any{
		"input_modalities": []string{"audio"},
	}, nil)
	expect(t, "非法输入模态", code, 400, body)
	if !strings.Contains(string(body), "输入模态") {
		t.Fatalf("400 文案应说明输入模态：%s", body)
	}
}

func TestModelInputModalitiesDiscovery(t *testing.T) {
	f := newModalityFixture(t)
	c := newClient(t, f.ts.URL)

	// /v1/models：additive capabilities.input（与 capabilities.reasoning 并存）。
	code, body := c.do("GET", "/v1/models", nil, bearer(f.token))
	expect(t, "网关模型列表", code, 200, body)
	models := decode[struct {
		Data []struct {
			ID           string `json:"id"`
			Capabilities struct {
				Reasoning struct {
					Supported bool `json:"supported"`
				} `json:"reasoning"`
				Input []string `json:"input"`
			} `json:"capabilities"`
		} `json:"data"`
	}](t, body)
	inputs := map[string][]string{}
	for _, m := range models.Data {
		inputs[m.ID] = m.Capabilities.Input
	}
	if got := inputs[f.vision]; !reflect.DeepEqual(got, []string{"text", "image"}) {
		t.Fatalf("company-vision capabilities.input = %v，期望 [text image]", got)
	}
	if got := inputs[f.docs]; !reflect.DeepEqual(got, []string{"text", "file"}) {
		t.Fatalf("company-docs capabilities.input = %v，期望 [text file]", got)
	}

	// 发现文档：匿名公布输入模态取值（能力矩阵）。
	code, body = c.do("GET", "/.well-known/dsh-ee", nil, nil)
	expect(t, "发现文档", code, 200, body)
	discovery := decode[struct {
		Models struct {
			InputModalities []string `json:"input_modalities"`
		} `json:"models"`
	}](t, body)
	if !reflect.DeepEqual(discovery.Models.InputModalities, []string{"text", "image", "file"}) {
		t.Fatalf("发现文档 models.input_modalities = %v", discovery.Models.InputModalities)
	}

	// /v1/capabilities：协议级能力矩阵（匿名也应公布）。
	for _, hdr := range []map[string]string{nil, bearer(f.token)} {
		code, body = c.do("GET", "/v1/capabilities", nil, hdr)
		expect(t, "能力摘要", code, 200, body)
		caps := decode[struct {
			InputModalities []string `json:"input_modalities"`
		}](t, body)
		if !reflect.DeepEqual(caps.InputModalities, []string{"text", "image", "file"}) {
			t.Fatalf("能力摘要 input_modalities = %v", caps.InputModalities)
		}
	}
}
