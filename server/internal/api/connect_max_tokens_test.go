package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/hikariming/dsh-ee/server/internal/store"
)

// 网关一次请求最多申请 maxTokensPerRequest 个输出 token，超过直接 400。DSH 0.2.2 起 llm-pi-ai
// 的默认 maxTokens 从 32768 提到 128000：不在模型配置里写明上限，新版 DSH 经 dsh-ee 的每一次对话都失败。
func TestConnectModelMaxTokensWithinGatewayLimit(t *testing.T) {
	f := newReleaseFixture(t, "notify")
	ctx := context.Background()
	provider := &store.Provider{Name: "limits", BaseURL: "http://127.0.0.1:8093", APIKeyEnc: []byte{1}, APIKeyHint: "…1", Enabled: true}
	if err := f.st.CreateProvider(ctx, provider); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"company-chat", "company-coder"} {
		if err := f.st.CreateModel(ctx, &store.Model{ProviderID: provider.ID, Name: name, UpstreamModel: "vendor-" + name, Enabled: true, InputModalities: []string{"text"}}); err != nil {
			t.Fatal(err)
		}
	}
	_, files, _, err := f.s.buildConnect(ctx, "http://127.0.0.1:8092", "1.0.1")
	if err != nil {
		t.Fatal(err)
	}
	patch := string(files["cordis.patch.yml"])
	for _, name := range []string{"company-chat", "company-coder"} {
		want := fmt.Sprintf("          - id: '%s'\n            input:\n              - text\n            maxTokens: %d\n", name, maxTokensPerRequest)
		if !strings.Contains(patch, want) {
			t.Fatalf("模型 %s 要写明 maxTokens（网关的单次上限）：\n期望片段：%q\n实际：\n%s", name, want, patch)
		}
	}
	// 一次请求带上配置里写的上限，网关要放行
	if code, msg := completionLimitError(map[string]any{"max_completion_tokens": json.Number(strconv.Itoa(maxTokensPerRequest))}); code != "" {
		t.Fatalf("按配置上限请求不应被拒：%s %s", code, msg)
	}
}
