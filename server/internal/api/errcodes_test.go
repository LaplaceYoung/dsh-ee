package api_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/api"
	"github.com/hikariming/dsh-ee/server/internal/rbac"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// apiErrBody 是老 /api/* 错误信封：error 是中文默认文案，code / params 是 P4 五语化新增的可选字段。
type apiErrBody struct {
	Error  string         `json:"error"`
	Code   string         `json:"code"`
	Params map[string]any `json:"params"`
}

func decodeErrBody(t *testing.T, what string, body []byte) apiErrBody {
	t.Helper()
	var e apiErrBody
	if err := json.Unmarshal(body, &e); err != nil {
		t.Fatalf("%s：解析错误信封失败：%v；响应 %s", what, err, body)
	}
	return e
}

// checkErr 断言一次错误响应：error 文案保持不变、code 正确、params 一致（wantParams 为 nil 时
// 要求响应里不出现 params）。
func checkErr(t *testing.T, what string, body []byte, wantErr, wantCode string, wantParams map[string]any) {
	t.Helper()
	e := decodeErrBody(t, what, body)
	if e.Error != wantErr {
		t.Errorf("%s：error 应保持原中文文案 %q，得到 %q；响应 %s", what, wantErr, e.Error, body)
	}
	if e.Code != wantCode {
		t.Errorf("%s：code 应为 %q，得到 %q；响应 %s", what, wantCode, e.Code, body)
	}
	if wantParams == nil {
		if len(e.Params) != 0 {
			t.Errorf("%s：不应有 params，得到 %v", what, e.Params)
		}
		return
	}
	if len(e.Params) != len(wantParams) {
		t.Errorf("%s：params 应为 %v，得到 %v", what, wantParams, e.Params)
	}
	for k, want := range wantParams {
		got, ok := e.Params[k]
		if !ok {
			t.Errorf("%s：params 缺少 %q（得到 %v）", what, k, e.Params)
			continue
		}
		if n, ok := want.(int); ok {
			// JSON 数字解出来是 float64
			if f, ok := got.(float64); !ok || f != float64(n) {
				t.Errorf("%s：params[%q] 应为 %d，得到 %v", what, k, n, got)
			}
			continue
		}
		if got != want {
			t.Errorf("%s：params[%q] 应为 %v，得到 %v", what, k, want, got)
		}
	}
}

// TestLoginErrorCodes 覆盖登录失败、用户名锁定、未登录与会话过期：文案不变，另带 code。
func TestLoginErrorCodes(t *testing.T) {
	ts := newLoginRateLimitServer(t, api.Options{})

	// 密码错误：401 + auth.bad_credentials，中文文案与加 code 之前逐字一致。
	code, body, _ := loginAttempt(t, ts.URL, "198.51.100.60", "admin", "wrong-password")
	expect(t, "密码错误", code, http.StatusUnauthorized, body)
	checkErr(t, "密码错误", body, "用户名或密码错误", "auth.bad_credentials", nil)

	// 同一用户名连续失败 5 次后锁定：429 + auth.too_many_attempts + minutes=15。
	ip := "198.51.100.61"
	for i := 2; i <= 5; i++ {
		code, body, _ = loginAttempt(t, ts.URL, ip, "admin", "wrong-password")
		expect(t, "连续密码错误", code, http.StatusUnauthorized, body)
	}
	code, body, _ = loginAttempt(t, ts.URL, ip, "admin", "admin-password-123")
	expect(t, "用户名锁定", code, http.StatusTooManyRequests, body)
	checkErr(t, "用户名锁定", body, "失败次数过多，请 15 分钟后再试", "auth.too_many_attempts", map[string]any{"minutes": 15})

	// 没有会话：401 + auth.login_required。
	anon := newClient(t, ts.URL)
	code, body = anon.do("GET", "/api/auth/me", nil, nil)
	expect(t, "未登录读 /api/auth/me", code, http.StatusUnauthorized, body)
	checkErr(t, "未登录", body, "请先登录", "auth.login_required", nil)

	// 会话无效（伪造 cookie）：401 + auth.session_expired。
	code, body = anon.do("GET", "/api/auth/me", nil, map[string]string{"Cookie": "dsh_ee_session=bogus"})
	expect(t, "无效会话", code, http.StatusUnauthorized, body)
	checkErr(t, "会话过期", body, "登录已过期，请重新登录", "auth.session_expired", nil)
}

// TestLoginIPRateLimitErrorCode 覆盖按出口 IP 的登录限流：429 + auth.ip_rate_limited，
// params.retry_after 与 Retry-After 头一致。
func TestLoginIPRateLimitErrorCode(t *testing.T) {
	ts := newLoginRateLimitServer(t, api.Options{LoginIPLimit: 1, LoginIPWindow: 15 * time.Minute})
	ip := "198.51.100.62"

	code, body, _ := loginAttempt(t, ts.URL, ip, "admin", "wrong-password")
	expect(t, "第一次密码错误", code, http.StatusUnauthorized, body)
	checkErr(t, "第一次密码错误", body, "用户名或密码错误", "auth.bad_credentials", nil)

	code, body, header := loginAttempt(t, ts.URL, ip, "admin", "admin-password-123")
	expect(t, "IP 限流", code, http.StatusTooManyRequests, body)
	e := decodeErrBody(t, "IP 限流", body)
	if e.Error != "登录失败次数过多，请稍后再试" {
		t.Errorf("IP 限流：error 应保持原中文文案，得到 %q", e.Error)
	}
	if e.Code != "auth.ip_rate_limited" {
		t.Errorf("IP 限流：code 应为 auth.ip_rate_limited，得到 %q", e.Code)
	}
	retry, ok := e.Params["retry_after"].(float64)
	if !ok || retry < 1 || retry > 900 {
		t.Errorf("IP 限流：params.retry_after 应是 1..900 的秒数，得到 %v（响应 %s）", e.Params["retry_after"], body)
	}
	if got, want := header.Get("Retry-After"), strconv.Itoa(int(retry)); got != want {
		t.Errorf("IP 限流：Retry-After 应为 %q，得到 %q", want, got)
	}
}

// TestAuthPermissionAndPasswordErrorCodes 覆盖权限点、改密码相关的稳定语义错误。
func TestAuthPermissionAndPasswordErrorCodes(t *testing.T) {
	f := newPrincipalFixture(t)
	u := f.user(t, "codes-user", nil)
	c := newClient(t, f.ts.URL)
	code, body := c.do("POST", "/api/auth/login", map[string]string{"username": u.Username, "password": "employee-password-1"}, nil)
	expect(t, "员工登录", code, http.StatusOK, body)

	// 普通员工没有用户管理权限：403 + auth.forbidden。
	code, body = c.do("GET", "/api/admin/users", nil, nil)
	expect(t, "员工读用户列表", code, http.StatusForbidden, body)
	checkErr(t, "无权限", body, "没有权限执行这个操作", "auth.forbidden", nil)

	// 原密码不对：400 + auth.bad_old_password。
	code, body = c.do("POST", "/api/auth/password", map[string]string{"old_password": "nope", "new_password": "employee-password-2"}, nil)
	expect(t, "原密码错误", code, http.StatusBadRequest, body)
	checkErr(t, "原密码错误", body, "原密码不正确", "auth.bad_old_password", nil)

	// 新密码太短：400 + auth.weak_password + min=10。
	code, body = c.do("POST", "/api/auth/password", map[string]string{"old_password": "employee-password-1", "new_password": "short"}, nil)
	expect(t, "新密码太短", code, http.StatusBadRequest, body)
	checkErr(t, "新密码太短", body, "密码至少 10 个字符", "auth.weak_password", map[string]any{"min": 10})

	// 新密码超过 72 字节：400 + auth.password_too_long + max=72。
	code, body = c.do("POST", "/api/auth/password", map[string]string{"old_password": "employee-password-1", "new_password": strings.Repeat("a", 73)}, nil)
	expect(t, "新密码太长", code, http.StatusBadRequest, body)
	checkErr(t, "新密码太长", body, "密码不能超过 72 字节", "auth.password_too_long", map[string]any{"max": 72})
}

// TestDeviceErrorCodes 覆盖设备码登录的拒绝：writeErr 迁移的 4 处，以及 RFC 8628 轮询错误
// 新增 code（error 字段仍是协议码，客户端行为不变）。
func TestDeviceErrorCodes(t *testing.T) {
	f := newPrincipalFixture(t)
	u := f.user(t, "device-codes-user", nil)
	loginC := newClient(t, f.ts.URL)
	code, body := loginC.do("POST", "/api/auth/login", map[string]string{"username": u.Username, "password": "employee-password-1"}, nil)
	expect(t, "员工登录", code, http.StatusOK, body)

	anon := newClient(t, f.ts.URL)

	// scope 不受支持。
	code, body = anon.do("POST", "/api/device/code", map[string]any{"client_name": "codes", "scopes": []string{"bogus"}}, nil)
	expect(t, "非法 scope", code, http.StatusBadRequest, body)
	checkErr(t, "非法 scope", body, "scope 不受支持", "device.scope_unsupported", nil)

	// 接入码不存在或已过期。
	code, body = anon.do("POST", "/api/device/code", map[string]any{"client_name": "codes", "setup_code": "nope"}, nil)
	expect(t, "无效接入码", code, http.StatusBadRequest, body)
	checkErr(t, "无效接入码", body, "接入码不存在或已过期，请在 dsh-ee 网页的「开始使用」页重新生成", "device.setup_code_expired", nil)

	// 正常发起设备码。
	code, body = anon.do("POST", "/api/device/code", map[string]any{"client_name": "codes"}, nil)
	expect(t, "发起设备码", code, http.StatusOK, body)
	var dev struct {
		DeviceCode string `json:"device_code"`
		UserCode   string `json:"user_code"`
	}
	if err := json.Unmarshal(body, &dev); err != nil || dev.DeviceCode == "" || dev.UserCode == "" {
		t.Fatalf("设备码响应不对：%v / %s", err, body)
	}

	// 未确认时轮询：error 仍是 authorization_pending，另带 device.pending。
	code, body = anon.do("POST", "/api/device/token", map[string]string{"device_code": dev.DeviceCode}, nil)
	expect(t, "首次轮询", code, http.StatusBadRequest, body)
	checkErr(t, "首次轮询", body, "authorization_pending", "device.pending", nil)

	// 轮询太快：error 仍是 slow_down，另带 device.slow_down。
	code, body = anon.do("POST", "/api/device/token", map[string]string{"device_code": dev.DeviceCode}, nil)
	expect(t, "立即轮询", code, http.StatusBadRequest, body)
	checkErr(t, "立即轮询", body, "slow_down", "device.slow_down", nil)

	// 用户拒绝：error 仍是 access_denied，另带 device.code_denied。
	code, body = loginC.do("POST", "/api/device/"+dev.UserCode, map[string]any{"approve": false}, nil)
	expect(t, "拒绝设备码", code, http.StatusOK, body)
	code, body = anon.do("POST", "/api/device/token", map[string]string{"device_code": dev.DeviceCode}, nil)
	expect(t, "轮询被拒的设备码", code, http.StatusBadRequest, body)
	checkErr(t, "轮询被拒的设备码", body, "access_denied", "device.code_denied", nil)

	// 确认码不存在或已过期（读取与批准各一处）。
	code, body = loginC.do("GET", "/api/device/ZZZZ-ZZZZ", nil, nil)
	expect(t, "读不存在的确认码", code, http.StatusNotFound, body)
	checkErr(t, "读不存在的确认码", body, "确认码不存在或已过期", "device.code_not_found", nil)
	code, body = loginC.do("POST", "/api/device/ZZZZ-ZZZZ", map[string]any{"approve": true}, nil)
	expect(t, "批准不存在的确认码", code, http.StatusBadRequest, body)
	checkErr(t, "批准不存在的确认码", body, "确认码不存在、已处理或已过期", "device.code_not_pending", nil)

	// 设备码被领取第二次：error 仍是 expired_token，另带 device.code_expired。
	code, body = anon.do("POST", "/api/device/code", map[string]any{"client_name": "codes"}, nil)
	expect(t, "再发起设备码", code, http.StatusOK, body)
	if err := json.Unmarshal(body, &dev); err != nil {
		t.Fatal(err)
	}
	code, body = loginC.do("POST", "/api/device/"+dev.UserCode, map[string]any{"approve": true}, nil)
	expect(t, "批准设备码", code, http.StatusOK, body)
	code, body = anon.do("POST", "/api/device/token", map[string]string{"device_code": dev.DeviceCode}, nil)
	expect(t, "领取设备令牌", code, http.StatusOK, body)
	code, body = anon.do("POST", "/api/device/token", map[string]string{"device_code": dev.DeviceCode}, nil)
	expect(t, "重复领取设备令牌", code, http.StatusBadRequest, body)
	checkErr(t, "重复领取设备令牌", body, "expired_token", "device.code_expired", nil)
}

// TestDeviceRateLimitErrorCode 覆盖设备码按 IP 限流：429 仍写 rate_limited，另带
// device.too_many_attempts。单独起服务，避免和其它用例共用限流窗口。
func TestDeviceRateLimitErrorCode(t *testing.T) {
	ts := newLoginRateLimitServer(t, api.Options{})
	anon := newClient(t, ts.URL)
	var lastStatus int
	var lastBody []byte
	for i := 0; i <= 30; i++ { // 每分钟 30 次，第 31 次被限流
		lastStatus, lastBody = anon.do("POST", "/api/device/code", map[string]any{"client_name": "codes"}, nil)
	}
	expect(t, "设备码限流", lastStatus, http.StatusTooManyRequests, lastBody)
	checkErr(t, "设备码限流", lastBody, "rate_limited", "device.too_many_attempts", nil)
}

// TestUnmigratedErrorHasNoCode 确认没迁移的错误响应形状与以前完全一样：只有 error，
// 没有 code / params。前端据此走原来的「显示服务端中文」分支。
func TestUnmigratedErrorHasNoCode(t *testing.T) {
	ts := newLoginRateLimitServer(t, api.Options{})
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/auth/login", bytes.NewReader([]byte("username=admin")))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	expect(t, "表单登录", resp.StatusCode, http.StatusUnsupportedMediaType, body)
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("解析响应失败：%v；响应 %s", err, body)
	}
	if raw["error"] != "请求体必须是 application/json" {
		t.Errorf("error 文案应不变，得到 %v", raw["error"])
	}
	if _, ok := raw["code"]; ok {
		t.Errorf("未迁移的错误响应不应出现 code：%s", body)
	}
	if _, ok := raw["params"]; ok {
		t.Errorf("未迁移的错误响应不应出现 params：%s", body)
	}
}

// ---- 第二批领域：用户 / 角色 / 权限点 / 菜单 / 系统设置 / 审批 ----

// TestUserAndQuotaErrorCodes 覆盖用户管理、部门边界、额度与 API key 的稳定拒绝：
// 中文文案逐字不变，另带本批新增的 code / params。
func TestUserAndQuotaErrorCodes(t *testing.T) {
	f := newPrincipalFixture(t)
	admin := f.admin

	// 用户名格式：400 + user.invalid_username。
	code, body := admin.do("POST", "/api/admin/users", map[string]any{"username": "Bad Name!", "password": "bad-name-pass-1"}, nil)
	expect(t, "用户名格式", code, http.StatusBadRequest, body)
	checkErr(t, "用户名格式", body, "用户名只能包含小写字母、数字和 . _ -，2 到 64 个字符", "user.invalid_username", nil)

	// 重名：409 + user.exists。
	code, body = admin.do("POST", "/api/admin/users", map[string]any{"username": "codes-user", "password": "codes-user-pass-1"}, nil)
	expect(t, "建用户", code, http.StatusCreated, body)
	codesUser := decode[struct {
		ID int64 `json:"id"`
	}](t, body)
	code, body = admin.do("POST", "/api/admin/users", map[string]any{"username": "codes-user", "password": "codes-user-pass-1"}, nil)
	expect(t, "重名用户", code, http.StatusConflict, body)
	checkErr(t, "重名用户", body, "用户名已存在", "user.exists", nil)

	// 用户不存在：改额度 / 改角色两个入口都是 404 + user.not_found。
	code, body = admin.do("PATCH", "/api/admin/users/999999/quota", map[string]any{"monthly_token_limit": 0}, nil)
	expect(t, "改不存在用户的额度", code, http.StatusNotFound, body)
	checkErr(t, "改不存在用户的额度", body, "用户不存在", "user.not_found", nil)
	code, body = admin.do("PUT", "/api/admin/users/999999/roles", map[string]any{"role_ids": []int64{}}, nil)
	expect(t, "改不存在用户的角色", code, http.StatusNotFound, body)
	checkErr(t, "改不存在用户的角色", body, "用户不存在", "user.not_found", nil)

	// 只有「管理用户」权限的经理：分配角色、分配额度、改部门、动上级都要被拒。
	code, body = admin.do("POST", "/api/admin/roles", map[string]any{"code": "codes-user-mgr", "name": "用户经理", "permissions": []string{"users.view", "users.manage"}}, nil)
	expect(t, "建用户经理角色", code, http.StatusCreated, body)
	mgrRole := decode[struct {
		ID int64 `json:"id"`
	}](t, body).ID
	code, body = admin.do("POST", "/api/admin/users", map[string]any{"username": "codes-mgr", "password": "codes-mgr-pass-1", "role_ids": []int64{mgrRole}}, nil)
	expect(t, "建用户经理", code, http.StatusCreated, body)
	mgr := newClient(t, f.ts.URL)
	code, body = mgr.do("POST", "/api/auth/login", map[string]string{"username": "codes-mgr", "password": "codes-mgr-pass-1"}, nil)
	expect(t, "用户经理登录", code, http.StatusOK, body)

	code, body = mgr.do("POST", "/api/admin/users", map[string]any{"username": "codes-role-assign", "password": "codes-role-pass-1", "role_ids": []int64{}}, nil)
	expect(t, "无角色权限分配角色", code, http.StatusForbidden, body)
	checkErr(t, "无角色权限分配角色", body, "分配角色需要「管理角色与权限」权限", "user.roles_assign_forbidden", nil)

	code, body = mgr.do("POST", "/api/admin/users", map[string]any{"username": "codes-quota-assign", "password": "codes-quota-pass-1", "monthly_token_limit": 10}, nil)
	expect(t, "无额度权限设额度", code, http.StatusForbidden, body)
	checkErr(t, "无额度权限设额度", body, "设置月度额度需要「分配额度」权限", "user.quota_assign_forbidden", nil)

	// 自己不能禁用自己、不能改部门；权限更大的上级不能被动。
	code, body = admin.do("GET", "/api/auth/me", nil, nil)
	expect(t, "读当前管理员", code, http.StatusOK, body)
	adminID := decode[struct {
		ID int64 `json:"id"`
	}](t, body).ID
	code, body = admin.do("GET", "/api/admin/users", nil, nil)
	expect(t, "用户列表", code, http.StatusOK, body)
	mgrID := int64(0)
	for _, u := range decode[struct {
		Users []struct {
			ID       int64  `json:"id"`
			Username string `json:"username"`
		} `json:"users"`
	}](t, body).Users {
		if u.Username == "codes-mgr" {
			mgrID = u.ID
		}
	}
	if mgrID == 0 {
		t.Fatal("用户列表里找不到刚建的用户经理")
	}
	code, body = mgr.do("PATCH", "/api/admin/users/"+itoa(mgrID), map[string]any{"disabled": true}, nil)
	expect(t, "管理员禁用自己", code, http.StatusBadRequest, body)
	checkErr(t, "管理员禁用自己", body, "不能禁用自己", "user.cannot_disable_self", nil)
	code, body = mgr.do("PATCH", "/api/admin/users/"+itoa(adminID), map[string]any{"disabled": true}, nil)
	expect(t, "禁用权限更大的用户", code, http.StatusForbidden, body)
	checkErr(t, "禁用权限更大的用户", body, "不能禁用、重置密码或解除权限比自己大的用户的身份绑定", "user.outranked_target", nil)
	code, body = mgr.do("PATCH", "/api/admin/users/"+itoa(mgrID), map[string]any{"department": "新部门"}, nil)
	expect(t, "改自己的部门", code, http.StatusForbidden, body)
	checkErr(t, "改自己的部门", body, "不能修改自己、同级或权限比自己大的用户的部门", "user.department_change_forbidden", nil)

	// 建号参数校验（经理权限就够，都能走到对应分支）。
	code, body = mgr.do("POST", "/api/admin/users", map[string]any{"username": "codes-bad-acct", "password": "codes-bad-acct-1", "account_type": "robot"}, nil)
	expect(t, "非法账号类型", code, http.StatusBadRequest, body)
	checkErr(t, "非法账号类型", body, "account_type 只能是 human 或 bot", "user.invalid_account_type", nil)
	code, body = mgr.do("POST", "/api/admin/users", map[string]any{"username": "codes-bot", "password": "codes-bot-pass-1", "account_type": "bot"}, nil)
	expect(t, "机器人账号设密码", code, http.StatusBadRequest, body)
	checkErr(t, "机器人账号设密码", body, "机器人账号不能设置密码或 LDAP 登录", "user.bot_credentials_forbidden", nil)
	code, body = mgr.do("POST", "/api/admin/users", map[string]any{"username": "codes-ldap-pw", "auth_source": "ldap", "password": "codes-ldap-pw-1"}, nil)
	expect(t, "目录账号设本地密码", code, http.StatusBadRequest, body)
	checkErr(t, "目录账号设本地密码", body, "LDAP / OIDC 账号不设本地密码", "user.external_password_forbidden", nil)
	code, body = mgr.do("POST", "/api/admin/users", map[string]any{"username": "codes-bad-src", "auth_source": "weird"}, nil)
	expect(t, "非法 auth_source", code, http.StatusBadRequest, body)
	checkErr(t, "非法 auth_source", body, "auth_source 只能是 local、ldap 或 oidc", "user.invalid_auth_source", nil)
	code, body = admin.do("POST", "/api/admin/users", map[string]any{"username": "codes-neg", "password": "codes-neg-pass-1", "monthly_token_limit": -1}, nil)
	expect(t, "负数额度", code, http.StatusBadRequest, body)
	checkErr(t, "负数额度", body, "monthly_token_limit 不能为负数", "user.invalid_quota", nil)

	// LDAP 账号的密码由目录管理；本地账号没有身份绑定可解除。
	code, body = admin.do("POST", "/api/admin/users", map[string]any{"username": "codes-ldap", "auth_source": "ldap"}, nil)
	expect(t, "建目录账号", code, http.StatusCreated, body)
	ldapUser := decode[struct {
		ID int64 `json:"id"`
	}](t, body)
	code, body = admin.do("PATCH", "/api/admin/users/"+itoa(ldapUser.ID), map[string]any{"password": "codes-ldap-pass-1"}, nil)
	expect(t, "重置目录账号密码", code, http.StatusBadRequest, body)
	checkErr(t, "重置目录账号密码", body, "LDAP / OIDC 账号的密码由企业目录或统一登录管理，不能在这里重置", "user.password_managed_externally", nil)
	code, body = admin.do("PATCH", "/api/admin/users/"+itoa(codesUser.ID), map[string]any{"unbind_identity": true}, nil)
	expect(t, "解除本地账号身份绑定", code, http.StatusBadRequest, body)
	checkErr(t, "解除本地账号身份绑定", body, "只有企业统一登录（OIDC）的账号有身份绑定可以解除", "user.no_identity_binding", nil)

	// 额度：空补丁 / 负数 / 过期时间在过去。
	code, body = admin.do("PATCH", "/api/admin/users/"+itoa(codesUser.ID)+"/quota", map[string]any{}, nil)
	expect(t, "空额度补丁", code, http.StatusBadRequest, body)
	checkErr(t, "空额度补丁", body, "没有要修改的额度字段", "quota.nothing_to_update", nil)
	code, body = admin.do("PATCH", "/api/admin/users/"+itoa(codesUser.ID)+"/quota", map[string]any{"monthly_token_limit": -1}, nil)
	expect(t, "额度为负数", code, http.StatusBadRequest, body)
	checkErr(t, "额度为负数", body, "monthly_token_limit 不能为负数；0 表示不分配额度，null 表示不限额", "quota.invalid_limit", nil)
	code, body = admin.do("PATCH", "/api/admin/users/"+itoa(codesUser.ID)+"/quota", map[string]any{"quota_expires_at": "2000-01-01T00:00:00Z"}, nil)
	expect(t, "额度过期时间在过去", code, http.StatusBadRequest, body)
	checkErr(t, "额度过期时间在过去", body, "quota_expires_at 必须是将来的时间", "quota.invalid_expiry", nil)

	// API key：空名称 / 不存在 / 代发时用户不存在、已停用。
	code, body = admin.do("POST", "/api/me/keys", map[string]any{"name": ""}, nil)
	expect(t, "空 key 名称", code, http.StatusBadRequest, body)
	checkErr(t, "空 key 名称", body, "name 不能为空", "key.invalid_name", nil)
	code, body = admin.do("DELETE", "/api/me/keys/999999", nil, nil)
	expect(t, "吊销不存在的 key", code, http.StatusNotFound, body)
	checkErr(t, "吊销不存在的 key", body, "key 不存在", "key.not_found", nil)
	code, body = admin.do("POST", "/api/admin/keys", map[string]any{"user_id": 999999, "name": "代发"}, nil)
	expect(t, "给不存在用户代发 key", code, http.StatusBadRequest, body)
	checkErr(t, "给不存在用户代发 key", body, "user_id 对应的用户不存在", "key.user_not_found", nil)
	code, body = admin.do("PATCH", "/api/admin/users/"+itoa(codesUser.ID), map[string]any{"disabled": true}, nil)
	expect(t, "停用用户", code, http.StatusOK, body)
	code, body = admin.do("POST", "/api/admin/keys", map[string]any{"user_id": codesUser.ID, "name": "代发"}, nil)
	expect(t, "给停用用户代发 key", code, http.StatusBadRequest, body)
	checkErr(t, "给停用用户代发 key", body, "该用户已被禁用", "key.user_disabled", nil)
}

// TestRolePermissionMenuErrorCodes 覆盖角色、权限点、菜单的校验与防提权拒绝。
func TestRolePermissionMenuErrorCodes(t *testing.T) {
	f := newPrincipalFixture(t)
	admin := f.admin

	// 角色代码 / 名称校验，重名，未知权限点，角色不存在。
	code, body := admin.do("POST", "/api/admin/roles", map[string]any{"code": "Bad Code", "name": "x"}, nil)
	expect(t, "角色代码格式", code, http.StatusBadRequest, body)
	checkErr(t, "角色代码格式", body, "角色代码以小写字母开头，只含小写字母、数字和 . _ -，2 到 64 个字符", "role.invalid_code", nil)
	code, body = admin.do("POST", "/api/admin/roles", map[string]any{"code": "codes-role", "name": ""}, nil)
	expect(t, "角色名称为空", code, http.StatusBadRequest, body)
	checkErr(t, "角色名称为空", body, "名称不能为空，且不超过 128 个字符", "role.invalid_name", nil)
	code, body = admin.do("POST", "/api/admin/roles", map[string]any{"code": "codes-role", "name": "代码角色", "permissions": []string{}}, nil)
	expect(t, "建角色", code, http.StatusCreated, body)
	code, body = admin.do("POST", "/api/admin/roles", map[string]any{"code": "codes-role", "name": "代码角色", "permissions": []string{}}, nil)
	expect(t, "重名角色", code, http.StatusConflict, body)
	checkErr(t, "重名角色", body, "角色代码已存在", "role.exists", nil)
	code, body = admin.do("POST", "/api/admin/roles", map[string]any{"code": "codes-role2", "name": "代码角色二", "permissions": []string{"no.such.perm"}}, nil)
	expect(t, "角色带未知权限点", code, http.StatusBadRequest, body)
	checkErr(t, "角色带未知权限点", body, "权限点不存在：no.such.perm", "perm.unknown", map[string]any{"code": "no.such.perm"})
	code, body = admin.do("PATCH", "/api/admin/roles/999999", map[string]any{"name": "x"}, nil)
	expect(t, "改不存在的角色", code, http.StatusNotFound, body)
	checkErr(t, "改不存在的角色", body, "角色不存在", "role.not_found", nil)

	// 内置角色不能删；超级管理员角色不能单独配权限。
	code, body = admin.do("GET", "/api/admin/roles", nil, nil)
	expect(t, "角色列表", code, http.StatusOK, body)
	var memberID, superID int64
	superName := ""
	for _, r := range decode[struct {
		Roles []struct {
			ID      int64  `json:"id"`
			Code    string `json:"code"`
			Name    string `json:"name"`
			IsSuper bool   `json:"is_super"`
		} `json:"roles"`
	}](t, body).Roles {
		if r.Code == "member" {
			memberID = r.ID
		}
		if r.IsSuper {
			superID, superName = r.ID, r.Name
		}
	}
	if memberID == 0 || superID == 0 {
		t.Fatalf("角色列表里找不到内置角色：%s", body)
	}
	code, body = admin.do("DELETE", "/api/admin/roles/"+itoa(memberID), nil, nil)
	expect(t, "删内置角色", code, http.StatusConflict, body)
	checkErr(t, "删内置角色", body, "内置角色不能删除", "role.in_use", nil)
	code, body = admin.do("PATCH", "/api/admin/roles/"+itoa(superID), map[string]any{"permissions": []string{}}, nil)
	expect(t, "给超管角色配权限", code, http.StatusBadRequest, body)
	checkErr(t, "给超管角色配权限", body, "超级管理员自动拥有全部权限，不需要单独配置", "role.super_permissions_implicit", nil)

	// 权限点：代码校验 / 重名 / 不存在 / 内置不能删。
	code, body = admin.do("POST", "/api/admin/permissions", map[string]any{"code": "Bad Code", "name": "x"}, nil)
	expect(t, "权限点代码格式", code, http.StatusBadRequest, body)
	checkErr(t, "权限点代码格式", body, "权限点代码以小写字母开头，只含小写字母、数字和 . _ : -，2 到 64 个字符", "perm.invalid_code", nil)
	code, body = admin.do("POST", "/api/admin/permissions", map[string]any{"code": "codes.test.perm", "name": "代码权限点"}, nil)
	expect(t, "建权限点", code, http.StatusCreated, body)
	code, body = admin.do("POST", "/api/admin/permissions", map[string]any{"code": "codes.test.perm", "name": "代码权限点"}, nil)
	expect(t, "重名权限点", code, http.StatusConflict, body)
	checkErr(t, "重名权限点", body, "权限点代码已存在", "perm.exists", nil)
	code, body = admin.do("DELETE", "/api/admin/permissions/no.such.perm", nil, nil)
	expect(t, "删不存在的权限点", code, http.StatusNotFound, body)
	checkErr(t, "删不存在的权限点", body, "权限点不存在", "perm.not_found", nil)
	code, body = admin.do("DELETE", "/api/admin/permissions/users.view", nil, nil)
	expect(t, "删内置权限点", code, http.StatusConflict, body)
	checkErr(t, "删内置权限点", body, "内置权限点不能删除", "perm.in_use", nil)

	// 菜单：标题校验 / 不存在 / 排序引用不存在。
	code, body = admin.do("POST", "/api/admin/menus", map[string]any{"title": "", "kind": "page"}, nil)
	expect(t, "菜单标题为空", code, http.StatusBadRequest, body)
	checkErr(t, "菜单标题为空", body, "标题不能为空，且不超过 64 个字符", "menu.invalid_title", nil)
	code, body = admin.do("DELETE", "/api/admin/menus/999999", nil, nil)
	expect(t, "删不存在的菜单", code, http.StatusNotFound, body)
	checkErr(t, "删不存在的菜单", body, "菜单不存在", "menu.not_found", nil)
	code, body = admin.do("PUT", "/api/admin/menus/order", map[string]any{"items": []map[string]any{{"id": 999999, "sort": 0}}}, nil)
	expect(t, "排序引用不存在的菜单", code, http.StatusBadRequest, body)
	checkErr(t, "排序引用不存在的菜单", body, "菜单不存在：999999", "menu.unknown", map[string]any{"id": 999999})

	// 只有「管理角色」权限的经理：授予越权角色、未知角色、授超管都要被拒。
	code, body = admin.do("POST", "/api/admin/roles", map[string]any{"code": "codes-role-mgr", "name": "角色经理", "permissions": []string{"roles.manage"}}, nil)
	expect(t, "建角色经理角色", code, http.StatusCreated, body)
	roleMgrRole := decode[struct {
		ID int64 `json:"id"`
	}](t, body).ID
	code, body = admin.do("POST", "/api/admin/users", map[string]any{"username": "codes-role-mgr", "password": "codes-role-mgr-pass-1", "role_ids": []int64{roleMgrRole}}, nil)
	expect(t, "建角色经理", code, http.StatusCreated, body)
	code, body = admin.do("POST", "/api/admin/users", map[string]any{"username": "codes-target", "password": "codes-target-pass-1", "role_ids": []int64{}}, nil)
	expect(t, "建目标用户", code, http.StatusCreated, body)
	targetID := decode[struct {
		ID int64 `json:"id"`
	}](t, body).ID
	roleMgr := newClient(t, f.ts.URL)
	code, body = roleMgr.do("POST", "/api/auth/login", map[string]string{"username": "codes-role-mgr", "password": "codes-role-mgr-pass-1"}, nil)
	expect(t, "角色经理登录", code, http.StatusOK, body)

	code, body = roleMgr.do("PUT", "/api/admin/users/"+itoa(targetID)+"/roles", map[string]any{"role_ids": []int64{superID}}, nil)
	expect(t, "授予超管角色", code, http.StatusForbidden, body)
	checkErr(t, "授予超管角色", body, "只有超级管理员能授予「"+superName+"」", "role.grant_super_forbidden", map[string]any{"role": superName})
	code, body = roleMgr.do("PUT", "/api/admin/users/"+itoa(targetID)+"/roles", map[string]any{"role_ids": []int64{999999}}, nil)
	expect(t, "授予不存在的角色", code, http.StatusForbidden, body)
	checkErr(t, "授予不存在的角色", body, "角色不存在：999999", "role.unknown", map[string]any{"id": 999999})

	// 建一个带 users.manage 的角色：角色经理授不出去、也改不了删不了。
	code, body = admin.do("POST", "/api/admin/roles", map[string]any{"code": "codes-grant-rich", "name": "提权角色", "permissions": []string{"users.manage"}}, nil)
	expect(t, "建提权角色", code, http.StatusCreated, body)
	richRole := decode[struct {
		ID int64 `json:"id"`
	}](t, body).ID
	code, body = roleMgr.do("PUT", "/api/admin/users/"+itoa(targetID)+"/roles", map[string]any{"role_ids": []int64{richRole}}, nil)
	expect(t, "授予超出权限的角色", code, http.StatusForbidden, body)
	checkErr(t, "授予超出权限的角色", body, "不能授予超出自己权限的角色「提权角色」", "role.grant_overreach", map[string]any{"role": "提权角色"})
	code, body = roleMgr.do("PATCH", "/api/admin/roles/"+itoa(richRole), map[string]any{"name": "改名"}, nil)
	expect(t, "改超出权限的角色", code, http.StatusForbidden, body)
	checkErr(t, "改超出权限的角色", body, "这个角色有你没有的权限，不能修改", "role.outranked_update", nil)
	code, body = roleMgr.do("DELETE", "/api/admin/roles/"+itoa(richRole), nil, nil)
	expect(t, "删超出权限的角色", code, http.StatusForbidden, body)
	checkErr(t, "删超出权限的角色", body, "这个角色有你没有的权限，不能删除", "role.outranked_delete", nil)
	code, body = roleMgr.do("PATCH", "/api/admin/roles/"+itoa(superID), map[string]any{"name": "改名"}, nil)
	expect(t, "改超管角色", code, http.StatusForbidden, body)
	checkErr(t, "改超管角色", body, "只有超级管理员能修改超级管理员角色", "role.super_modify_forbidden", nil)

	// 角色经理不能改权限比自己大的超管的角色。
	code, body = admin.do("GET", "/api/auth/me", nil, nil)
	expect(t, "读当前管理员", code, http.StatusOK, body)
	adminID := decode[struct {
		ID int64 `json:"id"`
	}](t, body).ID
	code, body = roleMgr.do("PUT", "/api/admin/users/"+itoa(adminID)+"/roles", map[string]any{"role_ids": []int64{roleMgrRole}}, nil)
	expect(t, "改超管角色分配", code, http.StatusForbidden, body)
	checkErr(t, "改超管角色分配", body, "不能修改权限比自己大的用户", "user.outranked_roles", nil)
}

// TestAdminSettingsErrorCodes 覆盖系统设置与管理约束（双人批准、预设）的稳定拒绝。
// TestAdminSettingsErrorCodes 覆盖系统设置与管理约束（双人批准、预设）的稳定拒绝。
func TestAdminSettingsErrorCodes(t *testing.T) {
	f := newPrincipalFixture(t)
	admin := f.admin

	// 只有「管理角色」的经理建分权预设时，没有预设角色的全部权限。
	code, body := admin.do("POST", "/api/admin/roles", map[string]any{"code": "codes-policy-mgr", "name": "约束经理", "permissions": []string{"roles.manage"}}, nil)
	expect(t, "建约束经理角色", code, http.StatusCreated, body)
	policyMgrRole := decode[struct {
		ID int64 `json:"id"`
	}](t, body).ID
	code, body = admin.do("POST", "/api/admin/users", map[string]any{"username": "codes-policy-mgr", "password": "codes-policy-mgr-pass-1", "role_ids": []int64{policyMgrRole}}, nil)
	expect(t, "建约束经理", code, http.StatusCreated, body)
	policyMgr := newClient(t, f.ts.URL)
	code, body = policyMgr.do("POST", "/api/auth/login", map[string]string{"username": "codes-policy-mgr", "password": "codes-policy-mgr-pass-1"}, nil)
	expect(t, "约束经理登录", code, http.StatusOK, body)
	code, body = policyMgr.do("POST", "/api/admin/policy/preset", map[string]any{"name": "separated"}, nil)
	expect(t, "无权限建分权预设", code, http.StatusForbidden, body)
	checkErr(t, "无权限建分权预设", body, "建「系统管理员」要你自己有它的全部权限", "policy.preset_overreach", map[string]any{"role": "系统管理员"})

	// 不存在的预设。
	code, body = admin.do("POST", "/api/admin/policy/preset", map[string]any{"name": "bogus"}, nil)
	expect(t, "不存在的预设", code, http.StatusBadRequest, body)
	checkErr(t, "不存在的预设", body, "没有这个预设：standard / separated", "policy.invalid_preset", nil)

	// 只有「registry.manage」的管理员不能改系统设置段：403 + settings.forbidden + field。
	code, body = admin.do("POST", "/api/admin/roles", map[string]any{"code": "codes-registry-mgr", "name": "源经理", "permissions": []string{"registry.manage"}}, nil)
	expect(t, "建源经理角色", code, http.StatusCreated, body)
	regMgrRole := decode[struct {
		ID int64 `json:"id"`
	}](t, body).ID
	code, body = admin.do("POST", "/api/admin/users", map[string]any{"username": "codes-registry-mgr", "password": "codes-registry-mgr-pass-1", "role_ids": []int64{regMgrRole}}, nil)
	expect(t, "建源经理", code, http.StatusCreated, body)
	regMgr := newClient(t, f.ts.URL)
	code, body = regMgr.do("POST", "/api/auth/login", map[string]string{"username": "codes-registry-mgr", "password": "codes-registry-mgr-pass-1"}, nil)
	expect(t, "源经理登录", code, http.StatusOK, body)
	code, body = regMgr.do("PUT", "/api/admin/settings", map[string]any{"orgName": "新的组织名"}, nil)
	expect(t, "无权限改系统设置", code, http.StatusForbidden, body)
	checkErr(t, "无权限改系统设置", body, "没有权限修改「orgName」设置", "settings.forbidden", map[string]any{"field": "orgName"})

	// 设置值校验失败：400 + settings.invalid_value（具体原因仍在 error 文案里）。
	code, body = admin.do("PUT", "/api/admin/settings", map[string]any{"orgName": strings.Repeat("长", 65)}, nil)
	expect(t, "组织名过长", code, http.StatusBadRequest, body)
	checkErr(t, "组织名过长", body, "组织名称不能超过 64 个字符", "settings.invalid_value", nil)

	// 打开双人批准后，管理约束和预设都要走配置变更。
	policy := map[string]any{"super_disabled": false, "no_self_grant": false, "exclusive": []any{}, "dual_approval": []string{"config"}}
	code, body = admin.do("PUT", "/api/admin/policy", policy, nil)
	expect(t, "打开双人批准", code, http.StatusOK, body)
	code, body = admin.do("PUT", "/api/admin/policy", policy, nil)
	expect(t, "双人批准下改约束", code, http.StatusConflict, body)
	checkErr(t, "双人批准下改约束", body, "开了双人批准：管理约束要通过配置变更提交（dsh-ee config apply，或「字段与访问规则 → 变更请求」）", "policy.dual_required", nil)
	code, body = admin.do("POST", "/api/admin/policy/preset", map[string]any{"name": "standard"}, nil)
	expect(t, "双人批准下用预设", code, http.StatusConflict, body)
	checkErr(t, "双人批准下用预设", body, "开了双人批准：预设要通过配置变更提交", "policy.preset_dual_required", nil)
}

func TestAccessRequestErrorCodes(t *testing.T) {
	f := newPrincipalFixture(t)
	admin := f.admin
	f.user(t, "codes-employee", nil)
	emp := newClient(t, f.ts.URL)
	code, body := emp.do("POST", "/api/auth/login", map[string]string{"username": "codes-employee", "password": "employee-password-1"}, nil)
	expect(t, "员工登录", code, http.StatusOK, body)

	doc := `{"openapi":"3.0.0","info":{"title":"CRM"},"paths":{"/summary":{"get":{"operationId":"summary"}}}}`
	for _, c := range []struct {
		slug, title string
		scope       map[string]any
	}{
		{"codes-open", "代码开放连接", map[string]any{"all": true}},
		{"codes-scoped", "代码受限连接", map[string]any{"departments": []string{"别的部门"}}},
	} {
		code, body = admin.do("POST", "/api/hub", map[string]any{"kind": "connection", "slug": c.slug, "delivery": "auto", "scope": c.scope,
			"spec": map[string]any{"title": c.title, "type": "openapi", "url": "http://127.0.0.1:1", "openapi": doc, "auth": map[string]any{"type": "none"}}}, nil)
		expect(t, "接入连接 "+c.slug, code, http.StatusCreated, body)
	}

	// 没写用途。
	code, body = emp.do("POST", "/api/me/access-requests", map[string]any{"kind": "connection", "slug": "codes-open"}, nil)
	expect(t, "没写用途", code, http.StatusBadRequest, body)
	checkErr(t, "没写用途", body, "请写明用途（最多 500 个字符），审批人据此决定是否开放", "request.purpose_required", nil)

	// 已经能用，不需要申请。
	code, body = emp.do("POST", "/api/me/access-requests", map[string]any{"kind": "connection", "slug": "codes-open", "reason": "想用"}, nil)
	expect(t, "申请已可用对象", code, http.StatusBadRequest, body)
	checkErr(t, "申请已可用对象", body, "你已经可以使用「代码开放连接」了，不需要申请", "request.already_granted", map[string]any{"title": "代码开放连接"})

	// 申请一个没权限的对象，随后用各种不对的状态处理它。
	code, body = emp.do("POST", "/api/me/access-requests", map[string]any{"kind": "connection", "slug": "codes-scoped", "reason": "跨部门协作"}, nil)
	expect(t, "提交申请", code, http.StatusCreated, body)
	reqID := decode[struct {
		ID int64 `json:"id"`
	}](t, body).ID
	code, body = emp.do("POST", "/api/me/access-requests/"+itoa(reqID), map[string]any{"decision": "maybe"}, nil)
	expect(t, "非法 decision", code, http.StatusBadRequest, body)
	checkErr(t, "非法 decision", body, "decision 只能是 approve 或 reject", "request.invalid_decision", nil)
	code, body = emp.do("POST", "/api/me/access-requests/999999", map[string]any{"decision": "approve"}, nil)
	expect(t, "处理不存在的申请", code, http.StatusNotFound, body)
	checkErr(t, "处理不存在的申请", body, "申请不存在或已经处理过", "request.not_found", nil)
	code, body = emp.do("POST", "/api/me/access-requests/"+itoa(reqID), map[string]any{"decision": "approve"}, nil)
	expect(t, "非审批人处理申请", code, http.StatusForbidden, body)
	checkErr(t, "非审批人处理申请", body, "你不是这条申请的审批人", "request.not_approver", nil)

	// 管理员审批通过后，再处理就是「已经处理过」。
	code, body = admin.do("POST", "/api/me/access-requests/"+itoa(reqID), map[string]any{"decision": "approve"}, nil)
	expect(t, "管理员审批通过", code, http.StatusOK, body)
	code, body = emp.do("POST", "/api/me/access-requests/"+itoa(reqID), map[string]any{"decision": "approve"}, nil)
	expect(t, "重复处理已完成的申请", code, http.StatusNotFound, body)
	checkErr(t, "重复处理已完成的申请", body, "申请不存在或已经处理过", "request.not_found", nil)
}

// ---- 第三批领域：令牌管理 / 管理员模式 / 初始化与审核 ----

// TestTokensErrorCodes 覆盖令牌管理（tokens.go）的稳定拒绝：签发校验、归属、类型、
// 部门边界与批量吊销。中文文案逐字不变，另带本批新增的 code / params。
func TestTokensIssueErrorCodes(t *testing.T) {
	f := newPrincipalFixture(t)
	u := f.user(t, "codes-token-user", nil)
	c := newClient(t, f.ts.URL)
	code, body := c.do("POST", "/api/auth/login", map[string]string{"username": u.Username, "password": "employee-password-1"}, nil)
	expect(t, "员工登录", code, http.StatusOK, body)

	// 签发参数：名称 / client_kind / 额度 / 有效期上限 / scope。
	code, body = c.do("POST", "/api/me/tokens", map[string]any{"name": ""}, nil)
	expect(t, "空令牌名", code, http.StatusBadRequest, body)
	checkErr(t, "空令牌名", body, "令牌名称必填，且不能超过 128 个字符", "token.invalid_name", nil)
	code, body = c.do("POST", "/api/me/tokens", map[string]any{"name": "codes", "client_kind": "robot"}, nil)
	expect(t, "非法 client_kind", code, http.StatusBadRequest, body)
	checkErr(t, "非法 client_kind", body, "client_kind 只能是 dsh 或 third_party", "token.invalid_client_kind", nil)
	code, body = c.do("POST", "/api/me/tokens", map[string]any{"name": "codes", "monthly_token_limit": -1}, nil)
	expect(t, "负数额度", code, http.StatusBadRequest, body)
	checkErr(t, "负数额度", body, "monthly_token_limit 不能为负数", "token.invalid_limit", nil)
	code, body = c.do("POST", "/api/me/tokens", map[string]any{"name": "codes", "expires_in_days": 3651}, nil)
	expect(t, "有效期超上限", code, http.StatusBadRequest, body)
	checkErr(t, "有效期超上限", body, "expires_in_days 应在 0–3650 之间，0 表示长期有效", "token.invalid_expiry", map[string]any{"max": 3650})
	code, body = c.do("POST", "/api/me/tokens", map[string]any{"name": "codes", "scopes": []string{rbac.ScopeAdmin}}, nil)
	expect(t, "admin scope", code, http.StatusBadRequest, body)
	checkErr(t, "admin scope", body, "scope 超出账号权限，或包含暂不支持的 admin", "token.invalid_scope", nil)

	// 吊销：id 格式 / 不是自己的按不存在 / 类型不对。
	code, body = c.do("DELETE", "/api/me/tokens/not-an-id", nil, nil)
	expect(t, "非法令牌 id", code, http.StatusBadRequest, body)
	checkErr(t, "非法令牌 id", body, "令牌 id 格式不正确", "token.invalid_id", nil)
	code, body = c.do("DELETE", "/api/me/tokens/at:999999", nil, nil)
	expect(t, "吊销不存在的令牌", code, http.StatusNotFound, body)
	checkErr(t, "吊销不存在的令牌", body, "令牌不存在", "token.not_found", nil)
	code, body = c.do("DELETE", "/api/me/tokens/weird:1", nil, nil)
	expect(t, "令牌类型不对", code, http.StatusBadRequest, body)
	checkErr(t, "令牌类型不对", body, "令牌类型不正确", "token.invalid_kind", nil)

	// 管理员入口：没有管理权限 / 签发给不存在的账号。
	code, body = c.do("GET", "/api/admin/tokens", nil, nil)
	expect(t, "员工读令牌管理", code, http.StatusForbidden, body)
	checkErr(t, "员工读令牌管理", body, "没有令牌管理权限", "token.manage_forbidden", nil)
	code, body = f.admin.do("POST", "/api/admin/tokens", map[string]any{"user_id": 999999, "name": "代发"}, nil)
	expect(t, "给不存在账号签发", code, http.StatusBadRequest, body)
	checkErr(t, "给不存在账号签发", body, "账号不存在", "token.user_not_found", nil)

	// 管理员关闭员工手动生成令牌。
	code, body = f.admin.do("PUT", "/api/admin/settings", map[string]any{"connect": map[string]any{"allowManualTokens": false}}, nil)
	expect(t, "关闭手动令牌", code, http.StatusOK, body)
	code, body = c.do("POST", "/api/me/tokens", map[string]any{"name": "codes-manual"}, nil)
	expect(t, "手动令牌已关闭", code, http.StatusForbidden, body)
	checkErr(t, "手动令牌已关闭", body, "管理员已关闭员工手动生成令牌", "token.manual_disabled", nil)
}

// TestTokensDepartmentErrorCodes 覆盖令牌管理的部门边界：只有「本部门令牌管理」权限的
// 管理员不能签发 / 吊销别的部门账号的令牌；批量吊销的 id 形式与空条件。
func TestTokensDepartmentErrorCodes(t *testing.T) {
	f := newPrincipalFixture(t)

	// 部门令牌管理员：只能签发 / 吊销本部门机器人账号的令牌。
	code, body := f.admin.do("POST", "/api/admin/roles", map[string]any{"code": "codes-dept-tokens", "name": "部门令牌", "permissions": []string{rbac.TokensDeptManage}}, nil)
	expect(t, "建部门令牌角色", code, http.StatusCreated, body)
	deptRole := decode[store.Role](t, body)
	mgr := f.user(t, "codes-dept-mgr", nil)
	code, body = f.admin.do("PUT", "/api/admin/users/"+itoa(mgr.ID)+"/roles", map[string]any{"role_ids": []int64{deptRole.ID}}, nil)
	expect(t, "分配部门令牌角色", code, http.StatusOK, body)
	code, body = f.admin.do("POST", "/api/admin/users", map[string]any{"username": "codes-other-bot", "department": "财务部", "account_type": "bot"}, nil)
	expect(t, "建别部门机器人", code, http.StatusCreated, body)
	otherBot := decode[store.User](t, body)
	otherTok := f.token(t, otherBot.ID, "ee_at_codes_other_bot", []string{rbac.ScopeHub}, nil, nil)
	mc := newClient(t, f.ts.URL)
	code, body = mc.do("POST", "/api/auth/login", map[string]string{"username": mgr.Username, "password": "employee-password-1"}, nil)
	expect(t, "部门令牌管理员登录", code, http.StatusOK, body)
	code, body = mc.do("POST", "/api/admin/tokens", map[string]any{"user_id": otherBot.ID, "name": "跨部门"}, nil)
	expect(t, "签发给别部门机器人", code, http.StatusForbidden, body)
	checkErr(t, "签发给别部门机器人", body, "只能管理本部门机器人账号的令牌", "token.department_scope_forbidden", nil)
	code, body = mc.do("POST", "/api/admin/tokens/revoke", map[string]any{"ids": []string{"bogus"}}, nil)
	expect(t, "批量吊销 id 格式不对", code, http.StatusBadRequest, body)
	checkErr(t, "批量吊销 id 格式不对", body, "批量吊销目前只支持 at:<id> 形式", "token.invalid_bulk_id", nil)
	code, body = mc.do("POST", "/api/admin/tokens/revoke", map[string]any{"ids": []string{"at:999999"}}, nil)
	expect(t, "批量吊销引用不存在的令牌", code, http.StatusBadRequest, body)
	checkErr(t, "批量吊销引用不存在的令牌", body, "令牌不存在：at:999999", "token.unknown", map[string]any{"id": "at:999999"})
	code, body = mc.do("POST", "/api/admin/tokens/revoke", map[string]any{}, nil)
	expect(t, "批量吊销没给条件", code, http.StatusBadRequest, body)
	checkErr(t, "批量吊销没给条件", body, "请指定要吊销的令牌、账号、设备或部门", "token.revoke_filter_required", nil)
	code, body = mc.do("POST", "/api/admin/tokens/revoke", map[string]any{"ids": []string{"at:" + itoa(otherTok.ID)}}, nil)
	expect(t, "吊销别部门机器人的令牌", code, http.StatusForbidden, body)
	checkErr(t, "吊销别部门机器人的令牌", body, "只能吊销本部门机器人账号的令牌", "token.department_revoke_forbidden", nil)
}

// TestAdminModeErrorCodes 覆盖管理员模式（admin_mode.go）的稳定拒绝：没有管理权限、
// 时长不对、密码不对、失败锁定；令牌 id / 归属沿用令牌域的 code。
func TestAdminModeErrorCodes(t *testing.T) {
	f := newPrincipalFixture(t)
	code, body := f.admin.do("GET", "/api/auth/me", nil, nil)
	expect(t, "读当前管理员", code, http.StatusOK, body)
	super := decode[store.User](t, body)

	// 员工没有管理权限：不能开启管理模式，也不能申请管理模式链接。
	emp := f.user(t, "codes-mode-emp", nil)
	empTok := f.token(t, emp.ID, "ee_at_codes_mode_emp", []string{rbac.ScopeHub}, nil, nil)
	ec := newClient(t, f.ts.URL)
	code, body = ec.do("POST", "/api/auth/login", map[string]string{"username": emp.Username, "password": "employee-password-1"}, nil)
	expect(t, "员工登录", code, http.StatusOK, body)
	code, body = f.elevate(t, ec, empTok.ID, "employee-password-1", 1)
	expect(t, "员工开启管理模式", code, http.StatusForbidden, body)
	checkErr(t, "员工开启管理模式", body, "你的账号没有管理权限，不能开启管理模式", "admin_mode.permission_required", nil)
	code, body = ec.do("GET", "/api/me/admin-link", nil, nil)
	expect(t, "员工申请管理模式链接", code, http.StatusForbidden, body)
	checkErr(t, "员工申请管理模式链接", body, "你的账号没有管理权限", "admin_mode.forbidden", nil)

	// 管理员：时长不对 / 令牌不存在 / 密码不对。
	tk := f.token(t, super.ID, "ee_at_codes_mode_admin", []string{rbac.ScopeHub}, nil, nil)
	code, body = f.elevate(t, f.admin, tk.ID, adminPassword, 2)
	expect(t, "非法时长", code, http.StatusBadRequest, body)
	checkErr(t, "非法时长", body, "管理模式只能开 1 小时或 8 小时", "admin_mode.invalid_duration", nil)
	code, body = f.elevate(t, f.admin, tk.ID+9999, adminPassword, 1)
	expect(t, "管理模式下不存在的令牌", code, http.StatusNotFound, body)
	checkErr(t, "管理模式下不存在的令牌", body, "令牌不存在", "token.not_found", nil)
	code, body = f.elevate(t, f.admin, tk.ID, "wrong-password-1", 1)
	expect(t, "管理模式密码不对", code, http.StatusForbidden, body)
	checkErr(t, "管理模式密码不对", body, "密码不对", "admin_mode.bad_password", nil)

	// 与登录共用失败限流：连错 5 次后即使密码正确也 429（含 minutes 参数）。
	for i := 0; i < 5; i++ {
		code, _ = f.elevate(t, f.admin, tk.ID, "wrong-password-1", 1)
	}
	code, body = f.elevate(t, f.admin, tk.ID, adminPassword, 1)
	expect(t, "连续输错后锁定", code, http.StatusTooManyRequests, body)
	checkErr(t, "连续输错后锁定", body, "失败次数过多，请 15 分钟后再试", "admin_mode.too_many_attempts", map[string]any{"minutes": 15})
}

// TestSetupInspectErrorCodes 覆盖初始化 / 审核页（admin_setup.go）的稳定拒绝：
// 非法包名、版本不存在、能力不存在、越权审核。
func TestSetupInspectErrorCodes(t *testing.T) {
	f := newPrincipalFixture(t)
	admin := f.admin

	code, body := admin.do("GET", "/api/admin/review/inspect?name=bad%20name&version=1", nil, nil)
	expect(t, "非法包名", code, http.StatusBadRequest, body)
	checkErr(t, "非法包名", body, "非法的包名", "admin_setup.invalid_package_name", nil)
	code, body = admin.do("GET", "/api/admin/review/inspect?name=no-such-codes-pkg&version=9.9.9", nil, nil)
	expect(t, "版本不存在", code, http.StatusNotFound, body)
	checkErr(t, "版本不存在", body, "版本不存在", "admin_setup.version_not_found", nil)
	code, body = admin.do("GET", "/api/admin/hub/inspect?asset_id=999999", nil, nil)
	expect(t, "能力不存在", code, http.StatusNotFound, body)
	checkErr(t, "能力不存在", body, "能力不存在", "admin_setup.asset_not_found", nil)

	// 只有「管理插件」权限的人不能审核企业连接。
	code, body = admin.do("POST", "/api/admin/roles", map[string]any{"code": "codes-plugins-only", "name": "插件审核", "permissions": []string{rbac.PluginsManage}}, nil)
	expect(t, "建插件审核角色", code, http.StatusCreated, body)
	pluginRole := decode[store.Role](t, body)
	reviewer := f.user(t, "codes-plugins-reviewer", nil)
	code, body = admin.do("PUT", "/api/admin/users/"+itoa(reviewer.ID)+"/roles", map[string]any{"role_ids": []int64{pluginRole.ID}}, nil)
	expect(t, "分配插件审核角色", code, http.StatusOK, body)
	doc := `{"openapi":"3.0.0","info":{"title":"CRM"},"paths":{"/summary":{"get":{"operationId":"summary"}}}}`
	code, body = admin.do("POST", "/api/hub", map[string]any{"kind": "connection", "slug": "codes-inspect-conn", "delivery": "auto",
		"scope": map[string]any{"all": true}, "spec": map[string]any{"title": "代码检查连接", "type": "openapi",
			"url": "http://127.0.0.1:1", "openapi": doc, "auth": map[string]any{"type": "none"}}}, nil)
	expect(t, "建企业连接", code, http.StatusCreated, body)
	connID := decode[struct {
		Asset struct {
			ID int64 `json:"id"`
		} `json:"asset"`
	}](t, body).Asset.ID
	rc := newClient(t, f.ts.URL)
	code, body = rc.do("POST", "/api/auth/login", map[string]string{"username": reviewer.Username, "password": "employee-password-1"}, nil)
	expect(t, "插件审核员登录", code, http.StatusOK, body)
	code, body = rc.do("GET", "/api/admin/hub/inspect?asset_id="+itoa(connID), nil, nil)
	expect(t, "越权审核企业连接", code, http.StatusForbidden, body)
	checkErr(t, "越权审核企业连接", body, "没有权限审核这类能力", "admin_setup.review_forbidden", nil)
}

// TestErrorCodeRegistryMatchesI18n 保证 errcodes.go 的 code 常量集合与
// web/src/i18n/{zh,en}/api-errors.ts 的键集合双向一致：新增 code 忘了补译文、
// 或删了 code 译文还在，都会在这里失败。tsc 的 `satisfies Messages<typeof zh>`
// 只保证 zh / en 两份译文互相对齐，跨到 Go 注册表的这条由本测试兜住。
func TestErrorCodesRegistryMatchesI18n(t *testing.T) {
	zhPath := filepath.Join("..", "..", "..", "web", "src", "i18n", "zh", "api-errors.ts")
	enPath := filepath.Join("..", "..", "..", "web", "src", "i18n", "en", "api-errors.ts")
	if _, err := os.Stat(zhPath); err != nil {
		t.Skip("仓库里没有 web 译文（例如只有 server 子树的检出），跳过注册表一致性检查")
	}

	codes := readCodeRegistry(t)
	codeSet := map[string]bool{}
	for _, code := range codes {
		if codeSet[code] {
			t.Errorf("errcodes.go 里 code %q 定义了两次", code)
		}
		codeSet[code] = true
	}
	if len(codeSet) == 0 {
		t.Fatal("errcodes.go 里没解析出任何 code 常量")
	}

	for _, lang := range []struct{ name, path string }{{"zh", zhPath}, {"en", enPath}} {
		keys := readApiErrorKeys(t, lang.path)
		for code := range codeSet {
			if !keys[code] {
				t.Errorf("%s 译文缺少 code %q 的键路径（errcodes.go 有定义）", lang.name, code)
			}
		}
		for key := range keys {
			if !codeSet[key] {
				t.Errorf("%s 译文里多了 code %q 的键路径（errcodes.go 没有定义）", lang.name, key)
			}
		}
		for code := range codeSet {
			if !strings.Contains(code, ".") || strings.HasPrefix(code, ".") || strings.HasSuffix(code, ".") {
				t.Errorf("code %q 不符合 <domain>.<reason> 命名", code)
			}
		}
	}
}

// readCodeRegistry 从 errcodes.go 的常量定义里取出所有 code 字面量。
func readCodeRegistry(t *testing.T) []string {
	t.Helper()
	f, err := os.Open("errcodes.go")
	if err != nil {
		t.Fatalf("读 errcodes.go：%v", err)
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "code") || strings.HasPrefix(line, "//") {
			continue
		}
		parts := strings.Split(line, `"`)
		if len(parts) != 3 {
			continue
		}
		out = append(out, parts[1])
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("读 errcodes.go：%v", err)
	}
	return out
}

// readApiErrorKeys 把 api-errors.ts 的嵌套对象摊平成 code 键路径（点号分段）。
// 只认「标识符: {」「标识符: "字符串"」两种写法，够这套译文文件用。
func readApiErrorKeys(t *testing.T, path string) map[string]bool {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读 %s：%v", path, err)
	}
	keys := map[string]bool{}
	var stack []string
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "//") || strings.HasPrefix(line, "/*") || strings.HasPrefix(line, "*") {
			continue
		}
		if i := strings.Index(line, ": {"); i > 0 {
			stack = append(stack, strings.TrimSpace(line[:i]))
			continue
		}
		if line == "}," || line == "}" {
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			continue
		}
		if i := strings.Index(line, `: "`); i > 0 {
			keys[strings.Join(append(stack, strings.TrimSpace(line[:i])), ".")] = true
		}
	}
	return keys
}
