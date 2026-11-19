package api_test

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDeviceFlow(t *testing.T) {
	f := newPrincipalFixture(t)
	u := f.user(t, "device-user", nil)
	c := newClient(t, f.ts.URL)
	code, body := c.do("POST", "/api/auth/login", map[string]string{"username": u.Username, "password": "employee-password-1"}, nil)
	expect(t, "员工登录", code, 200, body)

	anonymous := newClient(t, f.ts.URL)
	code, body = anonymous.do("POST", "/api/device/code", map[string]any{
		"client_name": "MacBook", "client_os": "macOS 15", "client_version": "0.2.1-rc.5",
	}, nil)
	expect(t, "发起设备码", code, 200, body)
	var device struct {
		DeviceCode string `json:"device_code"`
		UserCode   string `json:"user_code"`
		URI        string `json:"verification_uri_complete"`
	}
	if err := json.Unmarshal(body, &device); err != nil {
		t.Fatal(err)
	}
	if device.DeviceCode == "" || len(device.UserCode) != 9 || !strings.Contains(device.URI, "/device?code=") {
		t.Fatalf("设备码响应不对：%s", body)
	}
	code, body = anonymous.do("POST", "/api/device/token", map[string]string{"device_code": device.DeviceCode}, nil)
	expect(t, "首次轮询 pending", code, 400, body)
	if !strings.Contains(string(body), "authorization_pending") {
		t.Fatalf("首次轮询状态不对：%s", body)
	}
	code, body = anonymous.do("POST", "/api/device/token", map[string]string{"device_code": device.DeviceCode}, nil)
	expect(t, "立即轮询 slow_down", code, 400, body)
	if !strings.Contains(string(body), "slow_down") {
		t.Fatalf("立即轮询状态不对：%s", body)
	}

	code, body = anonymous.do("GET", "/api/device/"+device.UserCode, nil, nil)
	expect(t, "未登录不能读设备信息", code, 401, body)
	code, body = c.do("GET", "/api/device/"+device.UserCode, nil, nil)
	expect(t, "读取设备信息", code, 200, body)
	if !strings.Contains(string(body), "MacBook") || !strings.Contains(string(body), "macOS 15") {
		t.Fatalf("设备信息不对：%s", body)
	}
	code, body = c.do("POST", "/api/device/"+device.UserCode, map[string]any{"approve": true}, nil)
	expect(t, "批准设备", code, 200, body)
	code, body = anonymous.do("POST", "/api/device/token", map[string]string{"device_code": device.DeviceCode}, nil)
	expect(t, "领取设备令牌", code, 200, body)
	var granted struct {
		AccessToken string `json:"access_token"`
		User        struct {
			Username string `json:"username"`
		} `json:"user"`
	}
	if err := json.Unmarshal(body, &granted); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(granted.AccessToken, "ee_at_") || granted.User.Username != u.Username {
		t.Fatalf("领取结果不对：%s", body)
	}
	code, body = anonymous.do("POST", "/api/device/token", map[string]string{"device_code": device.DeviceCode}, nil)
	expect(t, "令牌只能领取一次", code, 400, body)
	if !strings.Contains(string(body), "expired_token") {
		t.Fatalf("重复领取状态不对：%s", body)
	}
	code, body = anonymous.do("GET", "/v1/models", nil, bearer(granted.AccessToken))
	expect(t, "设备令牌调模型", code, 200, body)
	code, body = anonymous.do("GET", "/api/dsh/catalog", nil, bearer(granted.AccessToken))
	expect(t, "设备令牌读目录", code, 200, body)
	// 设备码是 DSH 的登录方式：签出的令牌默认 client_kind=dsh，写确认沿用 DSH 快捷语义
	code, body = anonymous.do("GET", "/api/dsh/whoami", nil, bearer(granted.AccessToken))
	expect(t, "设备令牌 whoami", code, 200, body)
	if got := decodeAs[struct {
		ClientKind string `json:"client_kind"`
	}](t, body).ClientKind; got != "dsh" {
		t.Fatalf("设备令牌的 client_kind 应为 dsh：%s", body)
	}

	code, body = anonymous.do("POST", "/api/device/code", map[string]any{"client_name": "MacBook"}, nil)
	expect(t, "同设备再次登录", code, 200, body)
	if err := json.Unmarshal(body, &device); err != nil {
		t.Fatal(err)
	}
	code, body = c.do("POST", "/api/device/"+device.UserCode, map[string]any{"approve": true}, nil)
	expect(t, "同设备再次批准", code, 200, body)
	code, body = anonymous.do("GET", "/v1/models", nil, bearer(granted.AccessToken))
	expect(t, "同设备旧令牌被吊销", code, 401, body)
	if !strings.Contains(string(body), "token_revoked") {
		t.Fatalf("旧令牌错误码不对：%s", body)
	}

	code, body = anonymous.do("POST", "/api/device/code", map[string]any{"client_name": "denied"}, nil)
	expect(t, "发起拒绝场景", code, 200, body)
	if err := json.Unmarshal(body, &device); err != nil {
		t.Fatal(err)
	}
	code, body = c.do("POST", "/api/device/"+device.UserCode, map[string]any{"approve": false}, nil)
	expect(t, "拒绝设备", code, 200, body)
	code, body = anonymous.do("POST", "/api/device/token", map[string]string{"device_code": device.DeviceCode}, nil)
	expect(t, "拒绝后 access_denied", code, 400, body)
	if !strings.Contains(string(body), "access_denied") {
		t.Fatalf("拒绝状态不对：%s", body)
	}
}

// 回归：发起与轮询曾共用每 IP 每分钟 10 次的限流，员工 45 秒内没点授权，第 11 次轮询拿到
// rate_limited，DSH 与 setup 都判定登录失败；公司内网多人共用出口 IP 时更早触发。
func TestDevicePollingNotRateLimited(t *testing.T) {
	f := newPrincipalFixture(t)
	u := f.user(t, "slow-approver", nil)
	c := newClient(t, f.ts.URL)
	code, body := c.do("POST", "/api/auth/login", map[string]string{"username": u.Username, "password": "employee-password-1"}, nil)
	expect(t, "员工登录", code, 200, body)

	anonymous := newClient(t, f.ts.URL)
	var codes []struct {
		DeviceCode string `json:"device_code"`
		UserCode   string `json:"user_code"`
	}
	// 同一出口 IP 上 12 台设备同时发起登录
	for i := 0; i < 12; i++ {
		code, body := anonymous.do("POST", "/api/device/code", map[string]any{"client_name": "laptop-" + itoa(int64(i))}, nil)
		expect(t, "发起设备码", code, 200, body)
		codes = append(codes, decode[struct {
			DeviceCode string `json:"device_code"`
			UserCode   string `json:"user_code"`
		}](t, body))
	}
	// 每台轮询 5 次（共 60 次），只允许 authorization_pending / slow_down
	for round := 0; round < 5; round++ {
		for _, d := range codes {
			code, body := anonymous.do("POST", "/api/device/token", map[string]string{"device_code": d.DeviceCode}, nil)
			if code == 429 || strings.Contains(string(body), "rate_limited") {
				t.Fatalf("正常轮询不应被限流（第 %d 轮）：%d %s", round, code, body)
			}
		}
	}
	code, body = c.do("POST", "/api/device/"+codes[0].UserCode, map[string]any{"approve": true}, nil)
	expect(t, "慢慢批准", code, 200, body)
	code, body = anonymous.do("POST", "/api/device/token", map[string]string{"device_code": codes[0].DeviceCode}, nil)
	expect(t, "批准后仍能领取", code, 200, body)

	// 发起登录本身仍有上限
	limited := false
	for i := 0; i < 40 && !limited; i++ {
		code, _ := anonymous.do("POST", "/api/device/code", map[string]any{"client_name": "flood"}, nil)
		limited = code == 429
	}
	if !limited {
		t.Fatal("同一 IP 大量发起设备码应被限流")
	}
}

// 没写 client_name 的客户端（SDK 的 requestDeviceCode 默认就不写）认不出是不是同一台设备：
// 批准第二个时不能把前一个当成「同设备重新登录」吊销掉。
func TestDeviceApproveUnnamedKeepsOtherDevices(t *testing.T) {
	f := newPrincipalFixture(t)
	u := f.user(t, "device-user", nil)
	c := newClient(t, f.ts.URL)
	code, body := c.do("POST", "/api/auth/login", map[string]string{"username": u.Username, "password": "employee-password-1"}, nil)
	expect(t, "员工登录", code, 200, body)
	anonymous := newClient(t, f.ts.URL)
	login := func(what string) string {
		t.Helper()
		code, body := anonymous.do("POST", "/api/device/code", map[string]any{}, nil)
		expect(t, what+"：发起设备码", code, 200, body)
		var device struct {
			DeviceCode string `json:"device_code"`
			UserCode   string `json:"user_code"`
		}
		if err := json.Unmarshal(body, &device); err != nil {
			t.Fatal(err)
		}
		code, body = c.do("GET", "/api/device/"+device.UserCode, nil, nil)
		expect(t, what+"：读设备信息", code, 200, body)
		if strings.Contains(string(body), `"replace_existing":true`) {
			t.Fatalf("%s：认不出设备时不该提示会替换旧登录：%s", what, body)
		}
		code, body = c.do("POST", "/api/device/"+device.UserCode, map[string]any{"approve": true}, nil)
		expect(t, what+"：批准", code, 200, body)
		code, body = anonymous.do("POST", "/api/device/token", map[string]string{"device_code": device.DeviceCode}, nil)
		expect(t, what+"：领令牌", code, 200, body)
		return decodeAs[struct {
			AccessToken string `json:"access_token"`
		}](t, body).AccessToken
	}
	first := login("第一个客户端")
	second := login("第二个客户端")
	for name, tok := range map[string]string{"第一个": first, "第二个": second} {
		code, body := anonymous.do("GET", "/api/dsh/whoami", nil, bearer(tok))
		expect(t, name+"客户端的令牌仍然有效", code, 200, body)
	}
}
