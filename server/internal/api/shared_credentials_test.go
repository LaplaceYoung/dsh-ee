package api_test

import (
	"context"
	"crypto/rand"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/hikariming/dsh-ee/server/internal/api"
	"github.com/hikariming/dsh-ee/server/internal/secret"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

func TestNamedSharedCredentials(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if _, err := api.Bootstrap(ctx, st, "admin", "admin-password-123"); err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	box, _ := secret.NewBox(key)
	var mu sync.Mutex
	var selected []string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		selected = append(selected, r.Header.Get("X-Credential"))
		mu.Unlock()
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer up.Close()
	srv := api.New(st, box, api.Options{DataDir: t.TempDir()}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	admin := newClient(t, ts.URL)
	code, body := admin.do("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "admin-password-123"}, nil)
	expect(t, "管理员登录", code, 200, body)

	doc := `{"openapi":"3.0.0","info":{"title":"named"},"servers":[{"url":"` + up.URL + `"}],"paths":{"/lookup":{"get":{"operationId":"lookup","responses":{"200":{"description":"ok"}}}}}}`
	create := map[string]any{
		"kind": "connection", "slug": "named", "delivery": "auto", "secret": "legacy-secret",
		"spec": map[string]any{"title": "命名测试", "type": "openapi", "openapi": doc, "auth": map[string]any{"type": "header", "header": "X-Credential"}},
	}
	code, body = admin.do("POST", "/api/hub", create, nil)
	expect(t, "创建连接", code, 201, body)
	assetID := decodeAs[struct {
		Asset struct{ ID int64 } `json:"asset"`
	}](t, body).Asset.ID
	code, body = admin.do("POST", "/api/me/tokens", map[string]any{"name": "named-calls", "scopes": []string{"connections"}}, nil)
	expect(t, "创建调用令牌", code, 201, body)
	dsh := dshClient{t: t, base: ts.URL, token: decodeAs[struct{ Token string }](t, body).Token}

	putCred := func(name, secret string) {
		code, out := admin.do("PUT", "/api/admin/connections/"+itoa(assetID)+"/shared-credentials/"+name, map[string]string{"secret": secret}, nil)
		expect(t, "保存命名凭据 "+name, code, 200, out)
		if strings.Contains(string(out), secret) {
			t.Fatalf("保存响应不得回显完整凭据：%s", out)
		}
	}
	putCred("prod", "prod-secret")
	putCred("test", "test-secret")
	selectNamed := func(name string) {
		spec := map[string]any{"title": "命名测试", "type": "openapi", "openapi": doc, "auth": map[string]any{"type": "header", "header": "X-Credential", "shared_name": name}}
		code, out := admin.do("POST", "/api/hub/"+itoa(assetID)+"/versions", map[string]any{"spec": spec}, nil)
		expect(t, "选择命名凭据 "+name, code, 201, out)
	}
	call := func() {
		r := dsh.rpc("tools/call", map[string]any{"name": "named__lookup", "arguments": map[string]any{}})
		result, _ := r["result"].(map[string]any)
		if isErr, _ := result["isError"].(bool); isErr {
			t.Fatalf("真实连接调用失败：%v", r)
		}
	}
	// 缺省凭据仍然是旧的 assets.secret_enc。
	call()
	selectNamed("prod")
	call()
	putCred("prod", "prod-secret-2")
	call()
	selectNamed("test")
	call()
	mu.Lock()
	got := append([]string(nil), selected...)
	mu.Unlock()
	want := []string{"legacy-secret", "prod-secret", "prod-secret-2", "test-secret"}
	if len(got) != len(want) {
		t.Fatalf("命名凭据选择没有生效：got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("第 %d 次调用错误凭据：got %q want %q", i, got[i], want[i])
		}
	}

	recent, err := st.RecentConnectionCalls(ctx, assetID, 20)
	if err != nil || len(recent) == 0 || recent[0].SharedName != "test" {
		t.Fatalf("最近调用必须记录 selected shared_name，calls=%+v err=%v", recent, err)
	}
	code, body = admin.do("DELETE", "/api/admin/connections/"+itoa(assetID)+"/shared-credentials/test", nil, nil)
	expect(t, "正在使用的凭据不能删除", code, 409, body)
	selectNamed("")
	code, body = admin.do("DELETE", "/api/admin/connections/"+itoa(assetID)+"/shared-credentials/prod", nil, nil)
	expect(t, "删除未在使用的凭据", code, 204, body)
	selectNamed("prod")
	r := dsh.rpc("tools/call", map[string]any{"name": "named__lookup", "arguments": map[string]any{}})
	if !strings.Contains(string(mustJSON(r)), "不存在") {
		t.Fatalf("选择不存在的命名凭据必须明确失败：%v", r)
	}
	// 删除连接后也必须清理命名记录；不能把同名凭据留给后续重建的连接。
	code, body = admin.do("DELETE", "/api/hub/"+itoa(assetID), nil, nil)
	expect(t, "删除连接", code, 204, body)
	if _, err := st.SharedCredentialSecret(ctx, assetID, "test"); err != store.ErrNotFound {
		t.Fatalf("删除连接后 test 凭据应同步清理，err=%v", err)
	}
}
