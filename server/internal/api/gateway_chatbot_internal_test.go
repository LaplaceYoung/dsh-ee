package api

import (
	"reflect"
	"slices"
	"strings"
	"testing"
)

// upstreamChatBody 只影响发往 DeepSeek 上游的那次请求：默认不注入、注入时保客户端已有值、
// 非 DeepSeek 上游不注入、返回新 map 不改客户端原始 body。
func TestUpstreamChatBody(t *testing.T) {
	serverTools := []any{
		map[string]any{"type": deepseekWebSearchTool},
		map[string]any{"type": deepseekReadURLTool},
	}
	cases := []struct {
		name       string
		opt        Options
		body       map[string]any
		vendor     string
		upstream   string
		wantSafety any // nil 表示不应出现（客户端显式 null 时原样保留）
		wantTitle  any
	}{
		{
			name:     "默认关闭不注入",
			body:     map[string]any{"model": "chat", "messages": []any{}},
			vendor:   deepseekVendor,
			upstream: "deepseek-flash",
		},
		{
			name:       "开启后给 DeepSeek 注入",
			opt:        Options{SafetyCheckLevel: "high", ChatTitle: true},
			body:       map[string]any{"model": "chat", "messages": []any{}, "tools": serverTools},
			vendor:     deepseekVendor,
			upstream:   "deepseek-flash",
			wantSafety: map[string]any{"level": "high"},
			wantTitle:  map[string]any{"enabled": true},
		},
		{
			name: "客户端已传的 safety_check/title 原样保留",
			opt:  Options{SafetyCheckLevel: "high", ChatTitle: true},
			body: map[string]any{
				"model":        "chat",
				"safety_check": map[string]any{"level": "low"},
				"title":        map[string]any{"enabled": false},
			},
			vendor:     deepseekVendor,
			upstream:   "deepseek-flash",
			wantSafety: map[string]any{"level": "low"},
			wantTitle:  map[string]any{"enabled": false},
		},
		{
			name: "客户端显式 null 也保留，不覆盖",
			opt:  Options{SafetyCheckLevel: "high", ChatTitle: true},
			body: map[string]any{
				"model":        "chat",
				"safety_check": nil,
				"title":        nil,
			},
			vendor:   deepseekVendor,
			upstream: "deepseek-flash",
		},
		{
			name:     "非 DeepSeek 上游不注入",
			opt:      Options{SafetyCheckLevel: "high", ChatTitle: true},
			body:     map[string]any{"model": "chat", "messages": []any{}},
			vendor:   "custom",
			upstream: "vendor-chat",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &Server{opts: tc.opt}
			original := make(map[string]any, len(tc.body))
			for k, v := range tc.body {
				original[k] = v
			}
			got := s.upstreamChatBody(tc.body, tc.upstream, tc.vendor)
			if got["model"] != tc.upstream {
				t.Fatalf("上游 model = %v，期望 %q", got["model"], tc.upstream)
			}
			// 客户端字段除 model 外都应原样保留（包括 tools 里的服务端工具）。
			for k, v := range original {
				if k == "model" {
					continue
				}
				if !reflect.DeepEqual(got[k], v) {
					t.Fatalf("客户端字段 %q 被改动：%v → %v", k, v, got[k])
				}
			}
			check := func(key string, want any) {
				t.Helper()
				value, exists := got[key]
				_, inOriginal := original[key]
				if inOriginal {
					// 客户端传了（包括显式 null）就必须原样保留。
					if !exists || !reflect.DeepEqual(value, original[key]) {
						t.Fatalf("客户端已传的 %q 应保留：%v → %v", key, original[key], value)
					}
					return
				}
				if want == nil {
					if exists {
						t.Fatalf("%q 不应出现：%v", key, value)
					}
					return
				}
				if !reflect.DeepEqual(value, want) {
					t.Fatalf("%q = %#v，期望 %#v", key, value, want)
				}
			}
			check("safety_check", tc.wantSafety)
			check("title", tc.wantTitle)
			// 原始 body 不能被改动：故障切换时其它线路拿到的还是客户端原样请求。
			if !reflect.DeepEqual(tc.body, original) {
				t.Fatalf("upstreamChatBody 改动了客户端原始 body：%v → %v", original, tc.body)
			}
			// 返回的 map 是独立副本：改它不影响客户端 body。
			got["extra"] = true
			if _, ok := tc.body["extra"]; ok {
				t.Fatal("upstreamChatBody 返回的应是独立副本")
			}
		})
	}
}

// TestUpstreamChatBodyFailover 校验同一客户端的 body 在不同线路之间不会互相污染：
// 先按 DeepSeek 线路构造（注入），再按非 DeepSeek 线路构造（不注入）。
func TestUpstreamChatBodyFailover(t *testing.T) {
	s := &Server{opts: Options{SafetyCheckLevel: "high", ChatTitle: true}}
	body := map[string]any{"model": "chat", "messages": []any{}}
	deepseekBody := s.upstreamChatBody(body, "deepseek-flash", deepseekVendor)
	otherBody := s.upstreamChatBody(body, "vendor-chat", "custom")
	if _, ok := deepseekBody["safety_check"]; !ok {
		t.Fatal("DeepSeek 线路应注入 safety_check")
	}
	if _, ok := otherBody["safety_check"]; ok {
		t.Fatal("非 DeepSeek 线路不应继承 DeepSeek 的注入字段")
	}
	if _, ok := otherBody["title"]; ok {
		t.Fatal("非 DeepSeek 线路不应继承 title")
	}
	if otherBody["model"] != "vendor-chat" {
		t.Fatalf("非 DeepSeek 线路的 model 不对：%v", otherBody["model"])
	}
}

// TestChatbotCapability 覆盖能力声明：默认不注入但声明透传，注入开启后如实暴露。
func TestChatbotCapability(t *testing.T) {
	off := (&Server{}).chatbotCapability()
	if off.UpstreamVendor != deepseekVendor || !off.Passthrough || !off.Refusal {
		t.Fatalf("默认 chatbot 声明不对：%+v", off)
	}
	if !slices.Equal(off.ServerTools, []string{deepseekWebSearchTool, deepseekReadURLTool}) {
		t.Fatalf("server_tools 不对：%v", off.ServerTools)
	}
	if !off.SafetyCheck.Supported || off.SafetyCheck.Injected || off.SafetyCheck.Level != "" {
		t.Fatalf("默认不应注入 safety_check：%+v", off.SafetyCheck)
	}
	if !off.Title.Supported || off.Title.Injected {
		t.Fatalf("默认不应注入 title：%+v", off.Title)
	}
	for _, field := range []string{"title", "citations", "refusal", "server_tool_use", "server_tool_calls"} {
		if !slices.Contains(off.ResponseFields, field) {
			t.Fatalf("response_fields 缺少 %q：%v", field, off.ResponseFields)
		}
	}
	if !strings.Contains(off.Note, "DeepSeek") || !strings.Contains(off.Note, "透传") {
		t.Fatalf("note 应说明依赖上游且只透传：%q", off.Note)
	}

	on := (&Server{opts: Options{SafetyCheckLevel: "high", ChatTitle: true}}).chatbotCapability()
	if !on.SafetyCheck.Injected || on.SafetyCheck.Level != "high" || !on.Title.Injected {
		t.Fatalf("开启后的注入声明不对：%+v", on)
	}
}
