package api

// 数据目录不可写时 newRegistry 返回 nil，服务仍然启动。这组用例保证依赖 npm 源的
// 入口只回 503 / 可读失败原因，不会解引用 s.registry panic；不依赖 npm 源的路径照旧。

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hikariming/dsh-ee/server/internal/directory"
	"github.com/hikariming/dsh-ee/server/internal/rbac"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// registryNilMessage 是 registryReady 在 npm 源没初始化时回的固定文案。
const registryNilMessage = "npm 源暂不可用：服务端数据目录未初始化或不可写"

// TestRegistryNilDirectoryPluginInstall 验证目录里的插件条目在 registry 为 nil 时，
// install / 升级都只回 503 固定文案，且不会 panic。
func TestRegistryNilDirectoryPluginInstall(t *testing.T) {
	s := &Server{}
	entry := &store.DirectoryEntry{
		Kind: directory.KindPlugin, Name: "dsh-plugin-nil", Version: "1.0.0",
		Channel: directory.ChannelStable, Digest: "sha512-" + strings.Repeat("A", 86),
		SourceType: directory.SourceNPM,
	}
	for _, tc := range []struct {
		name    string
		upgrade bool
	}{{"install", false}, {"upgrade", true}} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/admin/directory/entries/1/"+tc.name, nil)
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("registry 为 nil 时 %s 不能 panic：%v", tc.name, r)
					}
				}()
				s.dirInstallPlugin(context.Background(), rec, req, entry, &dirInstallReq{}, tc.upgrade)
			}()
			if rec.Code != http.StatusServiceUnavailable {
				t.Fatalf("registry 为 nil 时 %s 应回 503，得到 %d：%s", tc.name, rec.Code, rec.Body.String())
			}
			var body struct {
				Error string `json:"error"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("503 响应不是 JSON：%v；%s", err, rec.Body.String())
			}
			if body.Error != registryNilMessage {
				t.Fatalf("%s 的 503 文案应固定且不含内部错误，得到 %q", tc.name, body.Error)
			}
		})
	}
}

// TestRegistryNilDirectoryExtensionNotBlocked 验证扩展条目不经过 npm 源的判空：
// registry 为 nil 时仍然走扩展自己的校验（来源地址为空 → 400），不会被拦成 503。
func TestRegistryNilDirectoryExtensionNotBlocked(t *testing.T) {
	s := &Server{}
	entry := &store.DirectoryEntry{
		Kind: directory.KindExtension, Name: "acme/nil-ext", Version: "1.0.0",
		Channel: directory.ChannelStable, Digest: strings.Repeat("a", 64),
		SourceType: directory.SourceTarball,
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/directory/entries/1/install", nil)
	s.dirInstallExtension(context.Background(), rec, req, entry, &dirInstallReq{}, false)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("扩展条目应按来源校验回 400，得到 %d：%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "npm 源") {
		t.Fatalf("扩展条目不应被 npm 源不可用挡住：%s", rec.Body.String())
	}
}

// TestRegistryNilFeishuNpmDecision 验证飞书 npm 审批卡片在 registry 为 nil 时返回
// 可读失败原因（调用方据此写 FeishuActionFailed 终态），而不是解引用 nil panic。
func TestRegistryNilFeishuNpmDecision(t *testing.T) {
	s := &Server{}
	v := viewer{user: &store.User{ID: 7}, access: &rbac.Access{Super: true}}
	action := &store.FeishuCardAction{Kind: store.FeishuActionNpmVersion, ObjectKey: "dsh-plugin-nil@1.0.0"}
	for _, decision := range []string{"approve", "reject"} {
		t.Run(decision, func(t *testing.T) {
			var (
				key   string
				owner *int64
				err   error
			)
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("registry 为 nil 时飞书 %s 不能 panic：%v", decision, r)
					}
				}()
				key, owner, err = s.applyFeishuNpmDecision(context.Background(), v, action, decision)
			}()
			if err == nil {
				t.Fatalf("registry 为 nil 时飞书 %s 应返回可读失败原因", decision)
			}
			if !strings.Contains(err.Error(), "npm 源不可用") {
				t.Fatalf("飞书 %s 的失败原因应说明 npm 源不可用，得到 %q", decision, err.Error())
			}
			if key != "" || owner != nil {
				t.Fatalf("飞书 %s 失败时不应返回审批对象：key=%q owner=%v", decision, key, owner)
			}
		})
	}
}
