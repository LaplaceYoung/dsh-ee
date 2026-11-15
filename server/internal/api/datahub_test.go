package api_test

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/hikariming/dsh-ee/server/internal/api"
	"github.com/hikariming/dsh-ee/server/internal/datahub"
	"github.com/hikariming/dsh-ee/server/internal/secret"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// openTestDataHub 在测试库旁边建一个 <库名>_data，测试结束连同只读账号一起删掉。
func openTestDataHub(t *testing.T, st *store.Store, box *secret.Box) *datahub.Hub {
	t.Helper()
	var db string
	if err := st.DB().QueryRowContext(context.Background(), "SELECT DATABASE()").Scan(&db); err != nil {
		t.Fatal(err)
	}
	cfg, err := mysql.ParseDSN(strings.Replace(os.Getenv("DSH_EE_TEST_DSN"), ")/", ")/"+db+"_data", 1))
	if err != nil {
		t.Fatal(err)
	}
	h, err := api.OpenDataHub(context.Background(), st, box, cfg.FormatDSN(), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		h.DropDatabase(context.Background())
		h.DropReader(context.Background())
		h.Close()
	})
	return h
}

func (c *client) uploadTable(fields map[string]string, filename string, data []byte) (int, []byte) {
	c.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range fields {
		_ = mw.WriteField(k, v)
	}
	w, _ := mw.CreateFormFile("file", filename)
	_, _ = w.Write(data)
	_ = mw.Close()
	req, _ := http.NewRequest("POST", c.base+"/api/admin/data/tables", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := c.hc.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b
}

func testXLSX(t *testing.T, rows [][]string) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(name, body string) {
		w, _ := zw.Create(name)
		_, _ = w.Write([]byte(body))
	}
	add("xl/workbook.xml", `<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="S" sheetId="1" r:id="rId1"/></sheets></workbook>`)
	add("xl/_rels/workbook.xml.rels", `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Target="worksheets/sheet1.xml"/></Relationships>`)
	var b strings.Builder
	b.WriteString("<worksheet><sheetData>")
	for _, r := range rows {
		b.WriteString("<row>")
		for _, c := range r {
			b.WriteString(`<c t="inlineStr"><is><t>` + c + `</t></is></c>`)
		}
		b.WriteString("</row>")
	}
	b.WriteString("</sheetData></worksheet>")
	add("xl/worksheets/sheet1.xml", b.String())
	_ = zw.Close()
	return buf.Bytes()
}

// 数据中台（M15）：上传表、数据集（参数、自动参数、范围）、只读账号、AI 工具与工作台数据源。
func TestDataHub(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if _, err := api.Bootstrap(ctx, st, "admin", "admin-password-123"); err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	box, _ := secret.NewBox(key)
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))

	// 没开数据中台：页面能打开、说明原因，上传返回 503
	off := httptest.NewServer(api.New(st, box, api.Options{DataDir: t.TempDir()}, quiet).Handler())
	offAdmin := newClient(t, off.URL)
	code, body := offAdmin.do("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "admin-password-123"}, nil)
	expect(t, "登录", code, 200, body)
	code, body = offAdmin.do("GET", "/api/admin/data", nil, nil)
	if code != 200 || !strings.Contains(string(body), `"enabled":false`) {
		t.Fatalf("没开时应说明：%d %s", code, body)
	}
	code, body = offAdmin.uploadTable(map[string]string{"name": "x", "title": "x", "mode": "create"}, "x.csv", []byte("a\n1\n"))
	expect(t, "没开时上传", code, 503, body)
	off.Close()

	hub := openTestDataHub(t, st, box)
	if hub.ReadErr != "" {
		t.Fatalf("只读账号应自动建好：%s", hub.ReadErr)
	}
	ts := httptest.NewServer(api.New(st, box, api.Options{DataDir: t.TempDir(), DataHub: hub}, quiet).Handler())
	defer ts.Close()
	admin := newClient(t, ts.URL)
	code, body = admin.do("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "admin-password-123"}, nil)
	expect(t, "管理员登录", code, 200, body)
	webs := map[string]*client{}
	for _, u := range []struct{ name, display, dept string }{{"zhou", "周航", "销售部"}, {"he", "何静", "财务部"}} {
		code, body = admin.do("POST", "/api/admin/users", map[string]any{"username": u.name, "password": u.name + "-password-1", "display_name": u.display, "department": u.dept}, nil)
		expect(t, "建用户", code, 201, body)
		c := newClient(t, ts.URL)
		code, body = c.do("POST", "/api/auth/login", map[string]string{"username": u.name, "password": u.name + "-password-1"}, nil)
		expect(t, "登录", code, 200, body)
		webs[u.name] = c
	}
	zhou := webs["zhou"]

	// ---- 上传 ----
	csv := "\xEF\xBB\xBF客户,负责人,年金额,续约日期\n华东精密,周航,\"120,000\",2026-12-01\n瑞丰银行,林娜,80000,2027/01/15\n启明软件,周航,30000,2026-11-20\n"
	code, body = admin.uploadTable(map[string]string{"name": "Bad-Name", "title": "x", "mode": "create"}, "c.csv", []byte(csv))
	expect(t, "表名不合法", code, 400, body)
	code, body = zhou.uploadTable(map[string]string{"name": "customers", "title": "客户", "mode": "create"}, "c.csv", []byte(csv))
	expect(t, "员工不能上传", code, 403, body)
	code, body = admin.uploadTable(map[string]string{"name": "customers", "title": "客户主数据", "mode": "create"}, "客户.csv", []byte(csv))
	expect(t, "上传 CSV", code, 200, body)
	if !strings.Contains(string(body), `"row_count":3`) || !strings.Contains(string(body), `{"name":"年金额","type":"number"}`) || !strings.Contains(string(body), `{"name":"续约日期","type":"date"}`) {
		t.Fatalf("建表结果：%s", body)
	}
	code, body = admin.uploadTable(map[string]string{"name": "customers", "title": "x", "mode": "create"}, "c.csv", []byte(csv))
	expect(t, "重复建表", code, 409, body)
	code, body = admin.uploadTable(map[string]string{"name": "customers", "mode": "append"}, "c.csv", []byte("客户,负责人\n新客户,周航\n"))
	expect(t, "追加时列不一样", code, 400, body)
	code, body = admin.uploadTable(map[string]string{"name": "customers", "mode": "append"}, "more.xlsx", testXLSX(t, [][]string{{"负责人", "客户", "年金额", "续约日期"}, {"林娜", "金穗粮油", "50000", "2027-03-01"}}))
	expect(t, "追加 xlsx（列顺序不同）", code, 200, body)
	if !strings.Contains(string(body), `"row_count":4`) || !strings.Contains(string(body), `"title":"客户主数据"`) {
		t.Fatalf("追加后：%s", body)
	}
	code, body = admin.do("GET", "/api/admin/data/tables/customers", nil, nil)
	expect(t, "预览表", code, 200, body)
	if !strings.Contains(string(body), "金穗粮油") || strings.Contains(string(body), "_row") {
		t.Fatalf("预览：%s", body)
	}
	code, body = admin.uploadTable(map[string]string{"name": "staff", "title": "人员", "mode": "create"}, "s.csv", []byte("姓名,部门\n周航,销售部\n何静,财务部\n陈一,财务部\n"))
	expect(t, "上传人员表", code, 200, body)

	// ---- 只读：只读账号写不了，SQL 也只能是一条 SELECT ----
	if _, err := hub.Query(ctx, "DELETE FROM customers", nil); err == nil {
		t.Fatal("只读账号不应能删数据")
	}
	if _, err := hub.Query(ctx, "CREATE TABLE evil (a int)", nil); err == nil {
		t.Fatal("只读账号不应能建表")
	}
	ds := func(slug, sql string, extra map[string]any) map[string]any {
		m := map[string]any{"slug": slug, "title": slug, "sql": sql}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}
	code, body = admin.do("POST", "/api/admin/data/datasets", ds("bad", "UPDATE customers SET 年金额 = 0", nil), nil)
	expect(t, "数据集只能是 SELECT", code, 400, body)
	code, body = admin.do("POST", "/api/admin/data/datasets", ds("bad", "SELECT * FROM nope", nil), nil)
	expect(t, "SQL 写错了保存时就报", code, 400, body)
	code, body = admin.do("POST", "/api/admin/data/run", map[string]any{"sql": "SELECT 客户 AS name, 年金额 AS arr FROM customers WHERE 年金额 >= :min ORDER BY arr DESC", "values": map[string]any{"min": 60000.0}}, nil)
	expect(t, "试运行", code, 200, body)
	if !strings.Contains(string(body), `{"arr":120000,"name":"华东精密"}`) || strings.Contains(string(body), "启明软件") {
		t.Fatalf("试运行结果：%s", body)
	}

	// ---- 数据集：参数、自动参数、范围 ----
	code, body = admin.do("POST", "/api/admin/data/datasets", ds("my-customers", "SELECT 客户 AS name, 年金额 AS arr, 续约日期 AS renew_at FROM customers WHERE 负责人 = :me_name ORDER BY renew_at", map[string]any{"title": "我负责的客户"}), nil)
	expect(t, "建「我负责的客户」", code, 201, body)
	myDS := decodeAs[store.DataSet](t, body)
	if !strings.Contains(string(body), `"auto":true`) {
		t.Fatalf("me_name 应标为自动参数：%s", body)
	}
	code, body = admin.do("POST", "/api/admin/data/datasets", ds("big-customers", "SELECT 客户 AS name FROM customers WHERE 年金额 >= :min", map[string]any{"params": []map[string]any{{"name": "min", "title": "最低金额", "default": "50000"}}}), nil)
	expect(t, "带默认参数的数据集", code, 201, body)
	code, body = admin.do("POST", "/api/admin/data/datasets", ds("dept-staff", "SELECT 姓名 AS name FROM staff WHERE 部门 = :me_department", map[string]any{"scope": map[string]any{"departments": []string{"财务部"}}}), nil)
	expect(t, "只给财务部的数据集", code, 201, body)
	code, body = admin.do("POST", "/api/admin/data/datasets", ds("my-customers", "SELECT 1", nil), nil)
	expect(t, "重复标识", code, 409, body)

	code, body = zhou.do("GET", "/api/data/datasets", nil, nil)
	expect(t, "周航能用的数据集", code, 200, body)
	if !strings.Contains(string(body), "my-customers") || strings.Contains(string(body), "dept-staff") {
		t.Fatalf("范围：%s", body)
	}

	// 数据集覆盖保存：范围与 SQL 都是整段覆盖的，PUT 必须带来 updated_at，旧快照 409
	putDS := func(token string) (int, []byte) {
		m := ds("my-customers", "SELECT 客户 AS name, 年金额 AS arr, 续约日期 AS renew_at FROM customers WHERE 负责人 = :me_name ORDER BY renew_at",
			map[string]any{"title": "我负责的客户"})
		if token != "" {
			m["updated_at"] = token
		}
		return admin.do("PUT", "/api/admin/data/datasets/my-customers", m, nil)
	}
	code, body = putDS("")
	expect(t, "缺 updated_at 改数据集", code, 400, body)
	time.Sleep(2 * time.Millisecond) // updated_at 是毫秒精度，隔开两次写保证版本不同
	code, body = putDS(myDS.UpdatedAt.UTC().Format(time.RFC3339Nano))
	expect(t, "更新数据集", code, 200, body)
	code, body = putDS(myDS.UpdatedAt.UTC().Format(time.RFC3339Nano))
	expect(t, "旧快照改数据集", code, 409, body)
	if !strings.Contains(string(body), "数据集已被其他人修改，请刷新后重试") {
		t.Fatalf("数据集 409 文案不对：%s", body)
	}

	// ---- DSH 里：data__dataset ----
	tokenOf := func(c *client) dshClient {
		code, body := c.do("POST", "/api/me/npm-tokens", map[string]any{"name": "dsh", "access": "read"}, nil)
		expect(t, "接入令牌", code, 201, body)
		return dshClient{t: t, base: ts.URL, token: decodeAs[struct{ Token string }](t, body).Token}
	}
	dz, dh := tokenOf(zhou), tokenOf(webs["he"])
	if names := dz.toolNames(); !slices.Contains(names, "data__dataset") {
		t.Fatalf("应有 data__dataset：%v", names)
	}
	call := func(d dshClient, args map[string]any) (map[string]any, bool, string) {
		r := d.rpc("tools/call", map[string]any{"name": "data__dataset", "arguments": args})
		res := r["result"].(map[string]any)
		text := res["content"].([]any)[0].(map[string]any)["text"].(string)
		isErr, _ := res["isError"].(bool)
		var out map[string]any
		_ = json.Unmarshal([]byte(text), &out)
		return out, isErr, text
	}
	if out, isErr, text := call(dz, map[string]any{"name": "my-customers"}); isErr || out["total"] != float64(2) || !strings.Contains(text, "启明软件") || strings.Contains(text, "瑞丰银行") {
		t.Fatalf("me_name 应只查到周航的客户：%s", text)
	}
	if out, _, text := call(dz, map[string]any{"name": "big-customers"}); out["total"] != float64(3) {
		t.Fatalf("默认参数 50000：%s", text)
	}
	if out, _, text := call(dz, map[string]any{"name": "big-customers", "params": map[string]any{"min": 100000}}); out["total"] != float64(1) {
		t.Fatalf("传参数：%s", text)
	}
	if _, isErr, _ := call(dz, map[string]any{"name": "dept-staff"}); !isErr {
		t.Fatal("周航不在财务部，不能用 dept-staff")
	}
	if out, isErr, text := call(dh, map[string]any{"name": "dept-staff"}); isErr || out["total"] != float64(2) {
		t.Fatalf("何静看到财务部的人：%s", text)
	}

	// ---- 工作台数据源 {"dataset": …} ----
	wb := func(dataset string) map[string]any {
		return map[string]any{"panes": []map[string]any{{"id": "main", "type": "panel", "title": "客户", "widgets": []map[string]any{
			{"type": "table", "title": "我负责的客户", "source": map[string]any{"dataset": dataset}, "rows": "items", "columns": []map[string]any{{"field": "name", "title": "客户"}}},
		}}}}
	}
	app := func(dataset string) map[string]any {
		return map[string]any{"kind": "app", "slug": "my-book", "delivery": "auto", "spec": map[string]any{"title": "客户本", "description": "x", "persona": "x", "web": true, "workbench": wb(dataset)}}
	}
	code, body = admin.do("POST", "/api/hub", app("nope"), nil)
	expect(t, "引用不存在的数据集", code, 400, body)
	code, body = admin.do("POST", "/api/hub", app("my-customers"), nil)
	expect(t, "用数据集的应用", code, 201, body)
	code, body, _ = dz.do("GET", "/api/dsh/catalog", nil, nil)
	expect(t, "目录", code, 200, body)
	if !strings.Contains(string(body), `"connection":"data","tool":"dataset","args":{"name":"my-customers","params":{}}`) {
		t.Fatalf("工作台的数据集数据源应换成 data__dataset：%s", body)
	}
	code, body = zhou.do("POST", "/api/me/apps/my-book/source", map[string]any{"pane": "main", "widget": 0}, nil)
	expect(t, "网页版取数", code, 200, body)
	if !strings.Contains(string(body), "华东精密") || strings.Contains(string(body), "瑞丰银行") {
		t.Fatalf("网页版的数据集：%s", body)
	}

	// ---- 替换与删除 ----
	code, body = admin.uploadTable(map[string]string{"name": "customers", "mode": "replace"}, "c.csv", []byte("客户,负责人,年金额,续约日期\n只剩一个,周航,1,2026-01-01\n"))
	expect(t, "替换", code, 200, body)
	if out, _, text := call(dz, map[string]any{"name": "my-customers"}); out["total"] != float64(1) {
		t.Fatalf("替换后：%s", text)
	}
	code, body = admin.do("DELETE", "/api/admin/data/datasets/big-customers", nil, nil)
	expect(t, "删数据集", code, 204, body)
	code, body = admin.do("DELETE", "/api/admin/data/tables/staff", nil, nil)
	expect(t, "删表", code, 204, body)
	code, body = admin.do("GET", "/api/admin/data", nil, nil)
	expect(t, "总览", code, 200, body)
	if strings.Contains(string(body), "big-customers") || strings.Contains(string(body), `"name":"staff"`) || !strings.Contains(string(body), `"read_error":""`) {
		t.Fatalf("删除后：%s", body)
	}
	code, body = admin.do("GET", "/api/admin/audit?action=data.&limit=100", nil, nil)
	expect(t, "审计", code, 200, body)
	for _, a := range []string{"data.upload", "data.dataset_create", "data.dataset_delete", "data.drop"} {
		if !strings.Contains(string(body), a) {
			t.Fatalf("审计里应有 %s", a)
		}
	}
}

// 示例公司在数据中台开着时：导入客户主数据、人员和三个数据集，周航的周报应用带上数据集工作台；清除时一起删。
func TestDemoDataHub(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if _, err := api.Bootstrap(ctx, st, "admin", "admin-password-123"); err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	box, _ := secret.NewBox(key)
	hub := openTestDataHub(t, st, box)
	ts := httptest.NewServer(api.New(st, box, api.Options{DataDir: t.TempDir(), DataHub: hub}, slog.New(slog.NewTextHandler(io.Discard, nil))).Handler())
	defer ts.Close()
	admin := newClient(t, ts.URL)
	code, body := admin.do("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "admin-password-123"}, nil)
	expect(t, "登录", code, 200, body)
	code, body = admin.do("POST", "/api/admin/demo", nil, nil)
	expect(t, "导入示例", code, 200, body)
	var res struct {
		Items    []struct{ Kind, Slug, Result, Error string }
		Accounts []struct{ Username, Password string }
	}
	_ = json.Unmarshal(body, &res)
	kinds := map[string]int{}
	for _, it := range res.Items {
		if it.Result != "created" {
			t.Fatalf("%s %s 没建成：%s", it.Kind, it.Slug, it.Error)
		}
		kinds[it.Kind]++
	}
	if kinds["data_table"] != 2 || kinds["data_set"] != 3 {
		t.Fatalf("应建 2 张表、3 个数据集：%v", kinds)
	}
	var pw string
	for _, a := range res.Accounts {
		if a.Username == "demo.zhou.hang" {
			pw = a.Password
		}
	}
	zhou := newClient(t, ts.URL)
	code, body = zhou.do("POST", "/api/auth/login", map[string]string{"username": "demo.zhou.hang", "password": pw}, nil)
	expect(t, "周航登录", code, 200, body)
	code, body = zhou.do("POST", "/api/me/apps/zhouhang-weekly/source", map[string]any{"pane": "main", "widget": 0}, nil)
	expect(t, "周报应用的数据集工作台", code, 200, body)
	if !strings.Contains(string(body), "华东精密制造") {
		t.Fatalf("周航负责的客户里应有华东精密制造：%s", body)
	}
	code, body = admin.do("DELETE", "/api/admin/demo", nil, nil)
	expect(t, "清除示例", code, 200, body)
	if !strings.Contains(string(body), `"data":5`) {
		t.Fatalf("应删 5 项数据中台的示例：%s", body)
	}
	code, body = admin.do("GET", "/api/admin/data", nil, nil)
	expect(t, "清除后", code, 200, body)
	if strings.Contains(string(body), "customers") || strings.Contains(string(body), "staff") {
		t.Fatalf("清除后不应还有示例表和数据集：%s", body)
	}
}
