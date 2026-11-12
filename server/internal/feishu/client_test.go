package feishu

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDirectoryPaginationAndFailures(t *testing.T) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/open-apis/auth/v3/tenant_access_token/internal", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]string
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req["app_id"] != "cli-x" || req["app_secret"] != "sec-x" {
			http.Error(w, "bad app", http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "tenant_access_token": "tenant-x"})
	})
	mux.HandleFunc("/open-apis/contact/v3/departments/parentchildren", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tenant-x" {
			w.WriteHeader(401)
			return
		}
		parent := r.URL.Query().Get("department_id")
		items := []map[string]any{}
		if parent == "0" {
			items = []map[string]any{
				{"open_department_id": "dep-rd", "name": "研发部"},
				{"open_department_id": "dep-sales", "name": "销售部"},
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"items": items, "has_more": false}})
	})
	mux.HandleFunc("/open-apis/contact/v3/users/find_by_department", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tenant-x" {
			w.WriteHeader(401)
			return
		}
		page := r.URL.Query().Get("page_token")
		if page == "done" || page == "next-sales" {
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"items": []map[string]any{}, "has_more": false}})
			return
		}
		dept := r.URL.Query().Get("department_id")
		var items []map[string]any
		if dept == "0" {
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"items": []map[string]any{}, "has_more": false}})
			return
		}
		switch {
		case dept == "dep-rd" && page == "":
			items = []map[string]any{{"open_id": "ou-alice", "name": "员工甲", "email": "alice@example.com", "department_ids": []string{"dep-rd"}}}
		case dept == "dep-rd" && page == "next-rd":
			items = []map[string]any{{"open_id": "ou-bob", "name": "员工乙", "email": "bob@example.com", "department_ids": []string{"dep-rd"}}}
		case dept == "dep-sales" && page == "":
			items = []map[string]any{{"open_id": "ou-carol", "name": "员工丙", "email": "carol@example.com", "department_ids": []string{"dep-sales"}}}
		default:
			t.Errorf("没有预期的部门/分页：dept=%s page=%s", dept, page)
		}
		out := map[string]any{"items": items, "has_more": false}
		if dept == "dep-rd" && page == "" {
			out["has_more"], out["page_token"] = true, "next-rd"
		} else if dept == "dep-sales" && page == "" {
			out["has_more"], out["page_token"] = true, "next-sales"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": out})
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	client := NewClient(server.URL, "cli-x", "sec-x", server.Client())
	dir, err := client.Directory(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(dir.Users) != 3 || dir.Users[0].OpenID != "ou-alice" || dir.Users[2].OpenID != "ou-carol" {
		t.Fatalf("分页/排序没有拉全员工：%+v", dir.Users)
	}
	if dir.Departments["dep-rd"] != "研发部" || dir.Departments["dep-sales"] != "销售部" {
		t.Fatalf("部门名称映射不对：%+v", dir.Departments)
	}
}

func TestDirectoryRejectsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal" {
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "tenant_access_token": "tenant-x"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 40001, "msg": "missing scope"})
	}))
	defer server.Close()
	dir, err := NewClient(server.URL, "cli-x", "sec-x", server.Client()).Directory(context.Background())
	if err == nil || dir != nil {
		t.Fatalf("飞书 envelope 失败应中止，不应返回半份目录：dir=%+v err=%v", dir, err)
	}
}

func TestExternalSubjectAndDisabled(t *testing.T) {
	if got := feishuSubjectForTest("ou-x"); got != "feishu:ou-x" {
		t.Fatalf("外部身份前缀不对：%s", got)
	}
	var user User
	user.Status.IsFrozen = true
	if !user.Disabled() {
		t.Fatal("冻结员工应识别为禁用")
	}
}

func feishuSubjectForTest(id string) string { return ExternalSubject(id) }
