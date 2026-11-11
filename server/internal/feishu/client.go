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
		return fmt.Errorf("飞书接口错误 %d：%s", env.Code, env.Msg)
	}
	return json.Unmarshal(env.Data, output)
}

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
	Membership    struct {
		IsDeleted bool `json:"is_deleted"`
		IsFrozen  bool `json:"is_frozen"`
	} `json:"membership,omitempty"`
}

func (u User) Disabled() bool { return u.Membership.IsDeleted || u.Membership.IsFrozen }

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
