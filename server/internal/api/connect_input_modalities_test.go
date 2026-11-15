package api

import (
	"context"
	"strings"
	"testing"

	"github.com/hikariming/dsh-ee/server/internal/store"
)

// dsh-connect 生成的模型配置要把输入模态写成 llm-pi-ai 的 input 列表：
// text 始终有；声明 image 才写 image；file 不在 DSH 0.2.x 的模态词表里，不能写入。
func TestConnectModelInputModalities(t *testing.T) {
	f := newReleaseFixture(t, "notify")
	ctx := context.Background()
	provider := &store.Provider{Name: "modality", BaseURL: "http://127.0.0.1:8093", APIKeyEnc: []byte{1}, APIKeyHint: "…1", Enabled: true}
	if err := f.st.CreateProvider(ctx, provider); err != nil {
		t.Fatal(err)
	}
	models := []*store.Model{
		{ProviderID: provider.ID, Name: "company-vision", UpstreamModel: "vendor-vision", Enabled: true, InputModalities: []string{"text", "image"}},
		{ProviderID: provider.ID, Name: "company-text", UpstreamModel: "vendor-text", Enabled: true, InputModalities: []string{"text"}},
		{ProviderID: provider.ID, Name: "company-docs", UpstreamModel: "vendor-docs", Enabled: true, InputModalities: []string{"text", "file"}},
	}
	for _, m := range models {
		if err := f.st.CreateModel(ctx, m); err != nil {
			t.Fatal(err)
		}
	}

	_, files, _, err := f.s.buildConnect(ctx, "http://127.0.0.1:8092", "1.0.1")
	if err != nil {
		t.Fatal(err)
	}
	patch := string(files["cordis.patch.yml"])

	for _, want := range []string{
		"          - id: 'company-vision'\n            input:\n              - text\n              - image\n",
		"          - id: 'company-text'\n            input:\n              - text\n",
		"          - id: 'company-docs'\n            input:\n              - text\n",
	} {
		if !strings.Contains(patch, want) {
			t.Fatalf("cordis.patch.yml 缺少模型输入模态：\n期望片段：%q\n实际：\n%s", want, patch)
		}
	}
	// file 不能出现在 DSH 模型配置里（DSH 0.2.x 的模态词表只有 text/image，否则配置校验失败）。
	if strings.Contains(patch, "              - file\n") {
		t.Fatalf("cordis.patch.yml 不应写入 file 模态：\n%s", patch)
	}
	// 默认模型与原有网关配置不受影响。
	if !strings.Contains(patch, "- id: agent-default-model") || !strings.Contains(patch, "X-DshEE-Host: !!js") {
		t.Fatalf("模型网关的既有配置被破坏：\n%s", patch)
	}
}

// DSH 的 input 只接受 text/image：connectInputModalities 对其它值一律丢弃，text 始终存在。
func TestConnectInputModalities(t *testing.T) {
	cases := []struct {
		raw  []string
		want []string
	}{
		{raw: nil, want: []string{"text"}},
		{raw: []string{"text", "image"}, want: []string{"text", "image"}},
		{raw: []string{"image"}, want: []string{"text", "image"}},
		{raw: []string{"text", "file"}, want: []string{"text"}},
		{raw: []string{"file"}, want: []string{"text"}},
		{raw: []string{"IMAGE", " text "}, want: []string{"text", "image"}},
	}
	for _, tc := range cases {
		got := connectInputModalities(tc.raw)
		if strings.Join(got, ",") != strings.Join(tc.want, ",") {
			t.Errorf("connectInputModalities(%v) = %v，期望 %v", tc.raw, got, tc.want)
		}
	}
}
