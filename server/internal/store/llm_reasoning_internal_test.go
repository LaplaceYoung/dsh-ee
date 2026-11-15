package store

import (
	"slices"
	"testing"
)

// 逗号分隔列表（思考强度 / 输入模态）的解析；空值必须返回空切片，JSON 里才是 [] 而不是 null。
func TestSplitJoinEfforts(t *testing.T) {
	cases := []struct {
		raw  string
		want []string
	}{
		{raw: "", want: []string{}},
		{raw: " , ", want: []string{}},
		{raw: "low", want: []string{"low"}},
		{raw: "low,high,max", want: []string{"low", "high", "max"}},
		{raw: " low , high ", want: []string{"low", "high"}},
	}
	for _, tc := range cases {
		if got := splitCSV(tc.raw); !slices.Equal(got, tc.want) {
			t.Errorf("splitCSV(%q) = %v，期望 %v", tc.raw, got, tc.want)
		}
	}
	if got := joinCSV([]string{"low", "high"}); got != "low,high" {
		t.Errorf("joinCSV = %q，期望 low,high", got)
	}
	if got := joinCSV(nil); got != "" {
		t.Errorf("joinCSV(nil) = %q，期望空串", got)
	}
}

// 输入模态按 text/image/file 规范顺序整理；text 始终存在，未知值丢弃（API 层会先拒掉）。
func TestNormalizeModalityList(t *testing.T) {
	cases := []struct {
		raw  []string
		want []string
	}{
		{raw: nil, want: []string{"text"}},
		{raw: []string{}, want: []string{"text"}},
		{raw: []string{"image"}, want: []string{"text", "image"}},
		{raw: []string{"FILE", "image", "text", "image"}, want: []string{"text", "image", "file"}},
		{raw: []string{"file"}, want: []string{"text", "file"}},
		{raw: []string{"audio"}, want: []string{"text"}},
	}
	for _, tc := range cases {
		if got := normalizeModalityList(tc.raw); !slices.Equal(got, tc.want) {
			t.Errorf("normalizeModalityList(%v) = %v，期望 %v", tc.raw, got, tc.want)
		}
	}
}
