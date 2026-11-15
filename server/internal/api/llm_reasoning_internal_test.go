package api

import (
	"slices"
	"strings"
	"testing"
)

// normalizeReasoningEfforts 只做管理端的结构校验与规范化：去空白、转小写、去重保序；
// 具体有哪些 effort 值由上游模型决定，这里不枚举（不硬编码模型或强度白名单）。
func TestNormalizeReasoningEfforts(t *testing.T) {
	ok := []struct {
		raw  []string
		want []string
	}{
		{raw: nil, want: []string{}},
		{raw: []string{}, want: []string{}},
		{raw: []string{" low ", "", "HIGH", "high", "max"}, want: []string{"low", "high", "max"}},
		{raw: []string{"minimal", "low", "medium", "high", "xhigh", "max", "ultra"}, want: []string{"minimal", "low", "medium", "high", "xhigh", "max", "ultra"}},
	}
	for _, tc := range ok {
		got, err := normalizeReasoningEfforts(tc.raw)
		if err != nil {
			t.Fatalf("normalizeReasoningEfforts(%v) 返回错误：%v", tc.raw, err)
		}
		if !slices.Equal(got, tc.want) {
			t.Errorf("normalizeReasoningEfforts(%v) = %v，期望 %v", tc.raw, got, tc.want)
		}
	}

	bad := []string{
		"has space",             // 中间有空格
		"1high",                 // 数字开头
		"high!",                 // 非法字符
		"-high",                 // 连字符开头
		strings.Repeat("a", 33), // 单项超过列宽设计
	}
	for _, item := range bad {
		if _, err := normalizeReasoningEfforts([]string{item}); err == nil {
			t.Errorf("normalizeReasoningEfforts(%q) 应返回错误", item)
		}
	}
	// 逗号拼接后超过列宽也要拒绝，而不是等数据库报错。
	long := []string{strings.Repeat("a", 31), strings.Repeat("b", 31), strings.Repeat("c", 31), strings.Repeat("d", 31), strings.Repeat("e", 10)}
	if _, err := normalizeReasoningEfforts(long); err == nil {
		t.Errorf("超过 128 字符的思考强度列表应返回错误")
	}
}
