// Package feishu 提供飞书登录与组织同步需要的最小通讯录客户端。
// 只用标准库；client secret 只在取 tenant token 的请求体里使用，不进入错误信息或日志。
package feishu

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
)

const apiLimit = 1 << 20

type Client struct {
	HTTP    *http.Client
	BaseURL string
	ID      string
	Secret  string
}

func NewClient(baseURL, id, secret string, hc *http.Client) *Client {
	return &Client{HTTP: hc, BaseURL: strings.TrimRight(baseURL, "/"), ID: id, Secret: secret}
}

type envelope struct {
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

func (c *Client) call(ctx context.Context, method, endpoint string, reqBody any, output any, token string) error {
	var body io.Reader
	if reqBody != nil {
		raw, err := json.Marshal(reqBody)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+endpoint, body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, apiLimit+1))
	if err != nil {
		return err
	}
	if len(raw) > apiLimit {
		return errors.New("飞书响应超过 1 MB")
	}
	if resp.StatusCode != http.StatusOK {
		// 业务错误（比如 41050 没有这个员工的权限）飞书用 HTTP 400 加 code 返回，把 code 留给调用方判断
		var env envelope
		if json.Unmarshal(raw, &env) == nil && env.Code != 0 {
			return &APIError{Status: resp.StatusCode, Code: env.Code, Msg: env.Msg}
		}
		return fmt.Errorf("飞书接口返回 %d：%s", resp.StatusCode, clip(string(raw), 200))
	}
	if strings.Contains(endpoint, "tenant_access_token/internal") {
		return json.Unmarshal(raw, output)
	}
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return err
	}
	if env.Code != 0 {
		return &APIError{Status: resp.StatusCode, Code: env.Code, Msg: env.Msg}
	}
	return json.Unmarshal(env.Data, output)
}

// APIError 是飞书接口返回的业务错误。
type APIError struct {
	Status int
	Code   int
	Msg    string
}

func (e *APIError) Error() string { return fmt.Sprintf("飞书接口错误 %d：%s", e.Code, e.Msg) }

// 查单个员工时用得上的错误码（contact/v3/users/:user_id）。
const (
	// CodeNoUserAuthority：应用的通讯录权限范围里没有这个员工
	CodeNoUserAuthority = 41050
	// CodeUserIDInvalid：这个 open_id 查不到
	CodeUserIDInvalid = 41012
)

func (c *Client) tenantToken(ctx context.Context) (string, error) {
	var out struct {
		Code              int    `json:"code"`
		Msg               string `json:"msg"`
		TenantAccessToken string `json:"tenant_access_token"`
		Expire            int    `json:"expire"`
	}
	err := c.call(ctx, http.MethodPost, "/open-apis/auth/v3/tenant_access_token/internal", map[string]string{"app_id": c.ID, "app_secret": c.Secret}, &out, "")
	if err != nil {
		return "", err
	}
	if out.Code != 0 {
		return "", fmt.Errorf("获取飞书 tenant token 失败（%d）：%s", out.Code, out.Msg)
	}
	if out.TenantAccessToken == "" {
		return "", errors.New("飞书没有返回 tenant token")
	}
	return out.TenantAccessToken, nil
}

type Department struct {
	ID       string `json:"open_department_id"`
	Name     string `json:"name"`
	ParentID string `json:"parent_department_id,omitempty"`
}

type User struct {
	OpenID        string   `json:"open_id"`
	Name          string   `json:"name"`
	Email         string   `json:"email"`
	EmployeeNo    string   `json:"employee_no"`
	DepartmentIDs []string `json:"department_ids"`
	// 飞书通讯录里的员工状态：冻结、离职、主动退出
	Status struct {
		IsFrozen   bool `json:"is_frozen"`
		IsResigned bool `json:"is_resigned"`
		IsExited   bool `json:"is_exited"`
	} `json:"status,omitempty"`
}

// Disabled 报告飞书是否明确说这个人不能再用了（冻结、离职或退出）。
func (u User) Disabled() bool { return u.Status.IsFrozen || u.Status.IsResigned || u.Status.IsExited }

// UserState 是单独向飞书确认一名员工时得到的结论。
type UserState int

const (
	// UserUnknown：飞书查不到这个 open_id，说不清是删了还是别的原因
	UserUnknown UserState = iota
	// UserActive：还在职，只是这次没出现在目录里
	UserActive
	// UserGone：飞书明确说冻结、离职或退出了
	UserGone
	// UserOutOfScope：应用的通讯录权限范围里没有这个人，看不到他的状态
	UserOutOfScope
)

// UserState 单独查一名员工。目录里没看到一个人，不等于他离职了：也可能是应用的通讯录权限范围
// 只覆盖了一部分员工。按目录禁用账号之前用它确认。
func (c *Client) UserState(ctx context.Context, openID string) (UserState, error) {
	token, err := c.tenantToken(ctx)
	if err != nil {
		return UserUnknown, err
	}
	var out struct {
		User User `json:"user"`
	}
	err = c.call(ctx, http.MethodGet, "/open-apis/contact/v3/users/"+url.PathEscape(openID)+"?user_id_type=open_id", nil, &out, token)
	var apiErr *APIError
	switch {
	case errors.As(err, &apiErr) && apiErr.Code == CodeNoUserAuthority:
		return UserOutOfScope, nil
	case errors.As(err, &apiErr) && apiErr.Code == CodeUserIDInvalid:
		return UserUnknown, nil
	case err != nil:
		return UserUnknown, err
	case out.User.Disabled():
		return UserGone, nil
	}
	return UserActive, nil
}

type Directory struct {
	Departments map[string]string
	Users       []User
}

type page struct {
	HasMore   bool   `json:"has_more"`
	PageToken string `json:"page_token"`
}

type departmentPage struct {
	Items []Department `json:"items"`
	page
}

type userPage struct {
	Items []User `json:"items"`
	page
}

func (c *Client) departments(ctx context.Context, token, parent, pageToken string) (*departmentPage, error) {
	q := url.Values{"department_id": {parent}, "fetch_child": {"true"}, "recursive": {"false"}, "page_size": {"50"}, "page_token": {pageToken}}
	var out departmentPage
	if err := c.call(ctx, http.MethodGet, "/open-apis/contact/v3/departments/parentchildren?"+q.Encode(), nil, &out, token); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) users(ctx context.Context, token, dept, pageToken string) (*userPage, error) {
	q := url.Values{"department_id": {dept}, "page_size": {"50"}, "page_token": {pageToken}, "user_id_type": {"open_id"}}
	var out userPage
	if err := c.call(ctx, http.MethodGet, "/open-apis/contact/v3/users/find_by_department?"+q.Encode(), nil, &out, token); err != nil {
		return nil, err
	}
	return &out, nil
}

// Directory 完整拉取当前组织。任何分页错误或 envelope 非零都直接失败，调用方不会用半份目录更新员工。
func (c *Client) Directory(ctx context.Context) (*Directory, error) {
	token, err := c.tenantToken(ctx)
	if err != nil {
		return nil, err
	}
	out := &Directory{Departments: map[string]string{"0": ""}, Users: []User{}}
	departments := []string{"0"}
	seenDepartments := map[string]bool{"0": true}
	seenUsers := map[string]bool{}
	for next := 0; next < len(departments); next++ {
		dept := departments[next]
		pageToken := ""
		for {
			users, err := c.users(ctx, token, dept, pageToken)
			if err != nil {
				return nil, err
			}
			for _, u := range users.Items {
				if u.OpenID == "" || seenUsers[u.OpenID] {
					continue
				}
				seenUsers[u.OpenID] = true
				out.Users = append(out.Users, u)
			}
			if !users.HasMore || users.PageToken == "" {
				break
			}
			pageToken = users.PageToken
		}
		pageToken = ""
		for {
			children, err := c.departments(ctx, token, dept, pageToken)
			if err != nil {
				return nil, err
			}
			for _, child := range children.Items {
				if child.ID == "" || seenDepartments[child.ID] {
					continue
				}
				seenDepartments[child.ID] = true
				out.Departments[child.ID] = child.Name
				departments = append(departments, child.ID)
			}
			if !children.HasMore || children.PageToken == "" {
				break
			}
			pageToken = children.PageToken
		}
	}
	slices.SortStableFunc(out.Users, func(a, b User) int { return strings.Compare(a.OpenID, b.OpenID) })
	return out, nil
}

func ExternalSubject(openID string) string { return "feishu:" + openID }

func Clip(text string, n int) string { return clip(text, n) }

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}
