package api_test

// DeepSeek 原生 Responses 透传的集成测试。
//
// 约定：管理端把 provider.vendor 配成 deepseek 时，/v1/responses 与 /responses 直接
// 转发到 <provider.base>/responses，dsh-ee 只做鉴权、模型权限、额度、model 改写、
// 逐字节转发与用量旁路记账；转换层会丢的 web_search/server tools、citations、
// title、image/file 内容块等原样保留。非 DeepSeek provider 仍走 chat completions
// 转换层（由 TestResponsesNonStreamText / TestResponsesToolCall 等回归覆盖）。

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hikariming/dsh-ee/server/internal/api"
	"github.com/hikariming/dsh-ee/server/internal/rbac"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// nativeDeepSeekResponse 是包含转换层会丢字段的 DeepSeek Responses 响应：
// web_search_call item、citations annotations、title、以及 DeepSeek 的 usage 形状。
const nativeDeepSeekResponse = `{"id":"resp_ds_1","object":"response","created_at":1793084908,"status":"completed","title":"企业搜索","model":"deepseek-flash","output":[{"type":"web_search_call","id":"ws_1","status":"completed","action":{"type":"search","queries":["dsh-ee"]}},{"type":"message","id":"msg_1","status":"completed","role":"assistant","content":[{"type":"output_text","text":"搜索结果","annotations":[{"type":"url_citation","url":"https://example.com","title":"来源"}],"logprobs":[]}]}],"parallel_tool_calls":true,"previous_response_id":null,"store":false,"usage":{"input_tokens":120,"output_tokens":30,"total_tokens":150}}`

func TestResponsesNativeDeepSeekPassthrough(t *testing.T) {
	f := newResponsesFixture(t)
	u := f.user(t, "responses-native-pass", nil)
	f.token(t, u.ID, "ee_at_resp_native", []string{rbac.ScopeModels}, nil, nil)
	f.setNativeHandler(func([]byte) (int, string, string) {
		return http.StatusOK, "application/json; charset=utf-8", nativeDeepSeekResponse
	})

	request := map[string]any{
		"model": "native-chat",
		"input": []any{map[string]any{
			"type": "message", "role": "user",
			"content": []any{
				map[string]any{"type": "input_text", "text": "搜一下 dsh-ee"},
				map[string]any{"type": "input_image", "image_url": "https://example.com/a.png"},
			},
		}},
		"tools":        []any{map[string]any{"type": "web_search"}},
		"title":        map[string]any{"enabled": true},
		"safety_check": map[string]any{"level": "low"},
		"stream":       false,
	}

	code, body, header := f.post(t, "ee_at_resp_native", request)
	expect(t, "DeepSeek 原生非流式", code, http.StatusOK, body)
	// 响应逐字节转发：web_search_call / citations / title 都原样保留。
	if !bytes.Equal(body, []byte(nativeDeepSeekResponse)) {
		t.Fatalf("原生非流式响应必须逐字节转发；得到 %s", body)
	}
	if got := header.Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("原生响应的 Content-Type = %q", got)
	}

	// 请求 JSON 语义透传：只改 model，web_search/title/safety_check/image 内容块都保留。
	forwarded := map[string]any{}
	if err := json.Unmarshal(f.lastNativeRequest(t), &forwarded); err != nil {
		t.Fatalf("上游收到的请求不是合法 JSON：%v", err)
	}
	if got := responsesString(t, forwarded, "model"); got != "upstream-native" {
		t.Fatalf("转发给上游的 model = %q，期望 upstream-native", got)
	}
	tools := responsesList(t, forwarded, "tools")
	if len(tools) != 1 || responsesString(t, tools[0].(map[string]any), "type") != "web_search" {
		t.Fatalf("原生透传应保留 web_search 工具：%v", forwarded["tools"])
	}
	if title := responsesObject(t, forwarded, "title"); title["enabled"] != true {
		t.Fatalf("原生透传应保留 title 字段：%v", forwarded["title"])
	}
	if _, ok := forwarded["safety_check"]; !ok {
		t.Fatalf("原生透传应保留未识别的顶层字段：%v", forwarded)
	}
	input := responsesList(t, forwarded, "input")
	content := responsesList(t, input[0].(map[string]any), "content")
	if len(content) != 2 || responsesString(t, content[1].(map[string]any), "type") != "input_image" {
		t.Fatalf("原生透传应保留 image 内容块：%v", content)
	}
	if got := f.lastNativeAuth(t); got != "Bearer "+upstreamSecret {
		t.Fatalf("原生请求应使用上游 key，得到 %q", got)
	}

	// usage 旁路记账：input/output/total 按 DeepSeek 原生 usage 记入月度额度。
	used, err := f.st.MonthTokensByUser(context.Background(), u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if used != 150 {
		t.Fatalf("原生非流式请求的月度用量 = %d，期望 150", used)
	}

	// /responses 别名与主入口一样走原生透传。
	code, aliasBody, _ := f.postPath(t, "/responses", "ee_at_resp_native", request)
	expect(t, "DeepSeek 原生别名", code, http.StatusOK, aliasBody)
	if !bytes.Equal(aliasBody, []byte(nativeDeepSeekResponse)) {
		t.Fatalf("/responses 别名也应逐字节转发原生响应")
	}
	used, err = f.st.MonthTokensByUser(context.Background(), u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if used != 300 {
		t.Fatalf("别名请求后的月度用量 = %d，期望 300", used)
	}
}

func TestResponsesNativeDeepSeekStreamPassthrough(t *testing.T) {
	f := newResponsesFixture(t)
	u := f.user(t, "responses-native-stream", nil)
	f.token(t, u.ID, "ee_at_resp_native_stream", []string{rbac.ScopeModels}, nil, nil)

	// 混合 \n 与 \r\n 行尾、event:/data: 行，验证 dsh-ee 不动任何字节。
	const nativeSSE = "event: response.created\r\ndata: {\"type\":\"response.created\",\"sequence_number\":0,\"response\":{\"id\":\"resp_ds_stream\",\"object\":\"response\",\"status\":\"in_progress\",\"output\":[]}}\r\n\r\nevent: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"sequence_number\":1,\"delta\":\"你好\"}\n\nevent: response.completed\ndata: {\"type\":\"response.completed\",\"sequence_number\":2,\"response\":{\"id\":\"resp_ds_stream\",\"object\":\"response\",\"status\":\"completed\",\"output\":[],\"usage\":{\"input_tokens\":7,\"output_tokens\":11,\"total_tokens\":18}}}\n\n"
	f.setNativeHandler(func([]byte) (int, string, string) {
		return http.StatusOK, "text/event-stream; charset=utf-8", nativeSSE
	})

	code, body, header := f.post(t, "ee_at_resp_native_stream", map[string]any{
		"model": "native-chat", "input": "hi", "stream": true,
	})
	expect(t, "DeepSeek 原生流式", code, http.StatusOK, body)
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("原生流式 Content-Type = %q", ct)
	}
	if !bytes.Equal(body, []byte(nativeSSE)) {
		t.Fatalf("原生 SSE 事件必须逐字节转发；得到 %q", body)
	}
	if got := f.lastNativeAccept(t); got != "text/event-stream" {
		t.Fatalf("原生流式请求的 Accept = %q，期望 text/event-stream", got)
	}
	var forwarded map[string]any
	if err := json.Unmarshal(f.lastNativeRequest(t), &forwarded); err != nil {
		t.Fatal(err)
	}
	if stream, _ := forwarded["stream"].(bool); !stream {
		t.Fatalf("原生透传应保留 stream:true：%v", forwarded)
	}

	// usage 从最后的 response.completed 事件里旁路取到。
	used, err := f.st.MonthTokensByUser(context.Background(), u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if used != 18 {
		t.Fatalf("原生流式请求的月度用量 = %d，期望 18", used)
	}
}

func TestResponsesNativeDeepSeekAuthQuotaAndFailover(t *testing.T) {
	f := newResponsesFixture(t)

	// 1) 原生路径同样要求 Bearer：没有令牌不会打到上游。
	code, body, _ := f.post(t, "", map[string]any{"model": "native-chat", "input": "hi"})
	expect(t, "原生缺 Bearer", code, http.StatusUnauthorized, body)
	if got := f.nativeRequestCount(); got != 0 {
		t.Fatalf("鉴权失败不该打上游，实际 %d 次", got)
	}

	// 2) 额度为 0 的账号：429 且不打上游。
	quotaUser := f.user(t, "responses-native-quota", ptrInt64(0))
	f.token(t, quotaUser.ID, "ee_at_resp_native_quota", []string{rbac.ScopeModels}, nil, nil)
	code, body, _ = f.post(t, "ee_at_resp_native_quota", map[string]any{"model": "native-chat", "input": "hi"})
	expect(t, "原生额度用完", code, http.StatusTooManyRequests, body)
	if got := f.nativeRequestCount(); got != 0 {
		t.Fatalf("额度用完不该打上游，实际 %d 次", got)
	}

	// 3) 令牌模型限制：403 且不打上游。
	u := f.user(t, "responses-native-limits", nil)
	f.token(t, u.ID, "ee_at_native_limited", []string{rbac.ScopeModels}, []string{"other-model"}, nil)
	code, body, _ = f.post(t, "ee_at_native_limited", map[string]any{"model": "native-chat", "input": "hi"})
	expect(t, "原生模型不在令牌范围", code, http.StatusForbidden, body)
	if got := f.nativeRequestCount(); got != 0 {
		t.Fatalf("模型拒绝不该打上游，实际 %d 次", got)
	}

	// 4) 请求体上限仍然生效（16 MiB），原生透传不绕过。
	f.token(t, u.ID, "ee_at_resp_native_body", []string{rbac.ScopeModels}, nil, nil)
	tooLarge := map[string]any{"model": "native-chat", "input": strings.Repeat("a", 16<<20)}
	code, body, _ = f.post(t, "ee_at_resp_native_body", tooLarge)
	expect(t, "原生请求体超限", code, http.StatusRequestEntityTooLarge, body)
	if got := f.nativeRequestCount(); got != 0 {
		t.Fatalf("请求体超限不该打上游，实际 %d 次", got)
	}

	// 5) 故障转移只在同厂商（DeepSeek）之间：第一条 500，第二条原生成功。
	var primaryCalls, secondaryCalls int
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		primaryCalls++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{"error":{"message":"主线路炸了","type":"api_error"}}`)
	}))
	t.Cleanup(primary.Close)
	secondaryPayload := strings.Replace(nativeDeepSeekResponse, `"resp_ds_1"`, `"resp_ds_secondary"`, 1)
	secondary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secondaryCalls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, secondaryPayload)
	}))
	t.Cleanup(secondary.Close)
	f.provider(t, "native-failover-a", "deepseek", primary.URL, 1, "up-failover-a", "native-failover")
	f.provider(t, "native-failover-b", "deepseek", secondary.URL, 2, "up-failover-b", "native-failover")
	code, body, _ = f.post(t, "ee_at_resp_native_body", map[string]any{"model": "native-failover", "input": "hi"})
	expect(t, "原生同厂商故障转移", code, http.StatusOK, body)
	if !bytes.Equal(body, []byte(secondaryPayload)) {
		t.Fatalf("故障转移后应逐字节转发第二条线路的响应；得到 %s", body)
	}
	if primaryCalls != 1 || secondaryCalls != 1 {
		t.Fatalf("故障转移调用次数不对：primary=%d secondary=%d", primaryCalls, secondaryCalls)
	}

	// 6) 不跨协议故障转移：DeepSeek 线路全挂时不回退到 chat 转换层。
	mixedPrimary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{"error":{"message":"原生线路炸了","type":"api_error"}}`)
	}))
	t.Cleanup(mixedPrimary.Close)
	f.provider(t, "native-mixed-a", "deepseek", mixedPrimary.URL, 1, "up-mixed-a", "native-mixed")
	f.provider(t, "native-mixed-b", "custom", f.upstreamURL, 2, "up-mixed-b", "native-mixed")
	chatBefore := f.chatRequestCount()
	code, body, _ = f.post(t, "ee_at_resp_native_body", map[string]any{"model": "native-mixed", "input": "hi"})
	expect(t, "原生失败不跨协议回退", code, http.StatusInternalServerError, body)
	if got := f.chatRequestCount(); got != chatBefore {
		t.Fatalf("原生线路失败后不应回退到 chat 转换上游，chat 请求数 %d → %d", chatBefore, got)
	}
}

// TestResponsesNativeDeepSeekFileAccess 覆盖原生 Responses 透传的文件引用访问边界：
// 文件属于上游共享账号、员工之间不隔离，普通员工在 input 里引用 file_id 会被 403
// file_access_denied 拦截（非流式与流式都在转发前拦截，不打上游）；管理员与机器人
// 账号默认放行；部署开启 DSH_EE_DEEPSEEK_TOOLS_ALLOW_EMPLOYEE 后普通员工放行。
func TestResponsesNativeDeepSeekFileAccess(t *testing.T) {
	f := newResponsesFixture(t)
	f.setNativeHandler(func([]byte) (int, string, string) {
		return http.StatusOK, "application/json", nativeDeepSeekResponse
	})
	employee := f.user(t, "responses-native-files", nil)
	f.token(t, employee.ID, "ee_at_native_files", []string{rbac.ScopeModels}, nil, nil)

	fileRequest := func(stream bool) map[string]any {
		body := map[string]any{
			"model": "native-chat",
			"input": []any{map[string]any{
				"type": "message", "role": "user",
				"content": []any{map[string]any{"type": "input_file", "file_id": "file-abc123"}},
			}},
		}
		if stream {
			body["stream"] = true
		}
		return body
	}
	assertFileDenied := func(t *testing.T, stream bool) {
		t.Helper()
		code, body, _ := f.post(t, "ee_at_native_files", fileRequest(stream))
		expect(t, "普通员工引用 file_id", code, http.StatusForbidden, body)
		errObj := responsesErrorObject(t, body)
		if got := responsesString(t, errObj, "type"); got != "permission_error" {
			t.Fatalf("file_access_denied 的 error.type = %q", got)
		}
		if got := responsesString(t, errObj, "code"); got != "file_access_denied" {
			t.Fatalf("file_access_denied 的 error.code = %q", got)
		}
		if got := f.nativeRequestCount(); got != 0 {
			t.Fatalf("文件引用被拦截时不应打上游，实际 %d 次", got)
		}
	}
	// 非流式 / 流式的 input_file 都被拦截。
	assertFileDenied(t, false)
	assertFileDenied(t, true)

	// input_image 带 file_id 也属于文件引用。
	code, body, _ := f.post(t, "ee_at_native_files", map[string]any{
		"model": "native-chat",
		"input": []any{map[string]any{
			"type": "message", "role": "user",
			"content": []any{map[string]any{"type": "input_image", "file_id": "file-img"}},
		}},
	})
	expect(t, "普通员工引用 image file_id", code, http.StatusForbidden, body)
	if got := responsesString(t, responsesErrorObject(t, body), "code"); got != "file_access_denied" {
		t.Fatalf("image file_id 的 error.code = %q", got)
	}

	// 不带 file_id 的普通员工请求不受影响。
	code, body, _ = f.post(t, "ee_at_native_files", map[string]any{"model": "native-chat", "input": "hi"})
	expect(t, "普通员工不带 file_id", code, http.StatusOK, body)
	if got := f.nativeRequestCount(); got != 1 {
		t.Fatalf("正常请求应打上游 1 次，实际 %d 次", got)
	}

	// 管理员放行：管理员账号本身带管理权限点。
	adminUser, err := f.st.GetUserByUsername(context.Background(), "admin")
	if err != nil {
		t.Fatal(err)
	}
	f.token(t, adminUser.ID, "ee_at_native_admin", []string{rbac.ScopeModels}, nil, nil)
	code, body, _ = f.post(t, "ee_at_native_admin", fileRequest(false))
	expect(t, "管理员引用 file_id", code, http.StatusOK, body)

	// 机器人/服务账号（account_type=bot）默认放行。
	code, body = f.admin.do("POST", "/api/admin/users", map[string]any{
		"username": "responses-bot", "display_name": "Responses Bot", "department": "研发效能部",
		"account_type": "bot", "owner_id": adminUser.ID,
	}, nil)
	expect(t, "创建机器人账号", code, http.StatusCreated, body)
	bot := decode[store.User](t, body)
	code, body = f.admin.do("PATCH", "/api/admin/users/"+itoa(bot.ID)+"/quota", map[string]any{"monthly_token_limit": nil}, nil)
	expect(t, "机器人额度", code, http.StatusOK, body)
	f.token(t, bot.ID, "ee_at_resp_bot", []string{rbac.ScopeModels}, nil, nil)
	code, body, _ = f.post(t, "ee_at_resp_bot", fileRequest(true))
	expect(t, "机器人引用 file_id", code, http.StatusOK, body)

	// 部署开启 DSH_EE_DEEPSEEK_TOOLS_ALLOW_EMPLOYEE 后普通员工放行（权限交给部署方评估）。
	g := newResponsesFixtureWith(t, func(opts *api.Options) { opts.DeepSeekToolsAllowEmployee = true })
	g.setNativeHandler(func([]byte) (int, string, string) {
		return http.StatusOK, "application/json", nativeDeepSeekResponse
	})
	allowedEmployee := g.user(t, "responses-native-files-open", nil)
	g.token(t, allowedEmployee.ID, "ee_at_native_files_open", []string{rbac.ScopeModels}, nil, nil)
	code, body, _ = g.post(t, "ee_at_native_files_open", map[string]any{
		"model": "native-chat",
		"input": []any{map[string]any{
			"type": "message", "role": "user",
			"content": []any{map[string]any{"type": "input_file", "file_id": "file-abc123"}},
		}},
		"stream": true,
	})
	expect(t, "开启开关后员工引用 file_id", code, http.StatusOK, body)
	if got := g.nativeRequestCount(); got != 1 {
		t.Fatalf("开启开关后应打上游 1 次，实际 %d 次", got)
	}
}
