package api

import (
	"slices"
	"strings"
	"testing"
)

// normalizeInputModalities 只做管理端的结构校验与规范化：小写、去重、按 text/image/file 排序，
// text 始终存在；不按模型名硬编码能力，也不枚举其它模态。
func TestNormalizeInputModalities(t *testing.T) {
	ok := []struct {
		raw  []string
		want []string
	}{
		{raw: nil, want: []string{"text"}},
		{raw: []string{}, want: []string{"text"}},
		{raw: []string{"image"}, want: []string{"text", "image"}},
		{raw: []string{" FILE ", "image", "text", "image"}, want: []string{"text", "image", "file"}},
		{raw: []string{"text"}, want: []string{"text"}},
	}
	for _, tc := range ok {
		got, err := normalizeInputModalities(tc.raw)
		if err != nil {
			t.Fatalf("normalizeInputModalities(%v) 返回错误：%v", tc.raw, err)
		}
		if !slices.Equal(got, tc.want) {
			t.Errorf("normalizeInputModalities(%v) = %v，期望 %v", tc.raw, got, tc.want)
		}
	}

	for _, bad := range [][]string{{"audio"}, {"video"}, {"text", "pdf"}} {
		_, err := normalizeInputModalities(bad)
		if err == nil || !strings.Contains(err.Error(), "输入模态") {
			t.Errorf("normalizeInputModalities(%v) 应返回输入模态错误，得到 %v", bad, err)
		}
	}
}
