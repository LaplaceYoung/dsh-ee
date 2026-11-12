package extension

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

func exampleFiles(t *testing.T) map[string][]byte {
	t.Helper()
	files := map[string][]byte{}
	for _, name := range []string{ManifestFile, "openapi.yaml", "README.md"} {
		b, err := os.ReadFile(exampleDir + "/" + name)
		if err != nil {
			t.Fatal(err)
		}
		files[name] = b
	}
	return files
}

func zipOf(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, b := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write(b)
	}
	zw.Close()
	return buf.Bytes()
}

// 包可以是扩展目录本身，也可以多包一层目录（GitHub 的「下载 ZIP」）。
func TestReadZip(t *testing.T) {
	files := exampleFiles(t)
	want := CheckFiles(files)
	if !want.OK {
		t.Fatalf("示例应通过检查：%+v", want.Results)
	}
	nested := map[string][]byte{"__MACOSX/._x": []byte("x")}
	for name, b := range files {
		nested["demo-declarative/"+name] = b
	}
	for what, z := range map[string][]byte{"根目录": zipOf(t, files), "一层目录": zipOf(t, nested)} {
		got, err := ReadZip(z)
		if err != nil {
			t.Fatalf("%s：%v", what, err)
		}
		if res := CheckFiles(got); !res.OK || res.Digest != want.Digest {
			t.Fatalf("%s：解开后的检查结果应和目录一样：%+v", what, res.Results)
		}
	}
	two := map[string][]byte{"a/" + ManifestFile: files[ManifestFile], "b/" + ManifestFile: files[ManifestFile]}
	if _, err := ReadZip(zipOf(t, two)); err == nil || !strings.Contains(err.Error(), "不止一份") {
		t.Fatalf("两份清单应拒收：%v", err)
	}
	if _, err := ReadZip(zipOf(t, map[string][]byte{"x/y/" + ManifestFile: files[ManifestFile]})); err == nil {
		t.Fatal("清单埋得太深应拒收")
	}
	if _, err := ReadZip([]byte("not a zip")); err == nil {
		t.Fatal("不是 zip 应报错")
	}
}

func exampleManifest(t *testing.T) *Manifest {
	t.Helper()
	m, err := ParseManifest(exampleFiles(t)[ManifestFile])
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestAdminParams(t *testing.T) {
	ps := exampleManifest(t).AdminParams()
	if len(ps) != 1 || ps[0].Key != "priority" || string(ps[0].Default) != `"中"` || !reflect.DeepEqual(ps[0].Tools, []string{"create_ticket"}) {
		t.Fatalf("示例只有一个管理员设置的参数 priority：%+v", ps)
	}
}

// 不由模型填的参数从定义里拿掉；模型填的换上清单里的说明和可选值。
func TestToolSchema(t *testing.T) {
	m := exampleManifest(t)
	input := json.RawMessage(`{"type":"object","properties":{"body":{"type":"object","properties":{"title":{"type":"string"},"category":{"type":"string"},"priority":{"type":"string","enum":["高","中","低"]}},"required":["title","priority"]}},"required":["body"]}`)
	out, err := m.Tool("create_ticket").Schema(input)
	if err != nil {
		t.Fatal(err)
	}
	var s struct {
		Properties struct {
			Body struct {
				Properties map[string]map[string]any `json:"properties"`
				Required   []string                  `json:"required"`
			} `json:"body"`
		} `json:"properties"`
	}
	json.Unmarshal(out, &s)
	body := s.Properties.Body
	if _, ok := body.Properties["priority"]; ok {
		t.Fatalf("priority 由管理员设置，模型不该看到：%s", out)
	}
	if !reflect.DeepEqual(body.Required, []string{"title"}) {
		t.Fatalf("priority 也要从 required 里去掉：%s", out)
	}
	cat := body.Properties["category"]
	if cat["description"] == nil || len(cat["enum"].([]any)) != 7 {
		t.Fatalf("category 应换上清单的说明和可选值：%s", out)
	}

	// 请求体里只有不由模型填的字段时，整个 body 都不用模型给
	only := &Tool{Params: []Param{{Name: "body.priority", Fill: FillAdmin}}}
	out, _ = only.Schema(json.RawMessage(`{"type":"object","properties":{"body":{"type":"object","properties":{"priority":{"type":"string"}}}},"required":["body"]}`))
	if strings.Contains(string(out), "body") {
		t.Fatalf("body 里没有模型能填的字段，应整个拿掉：%s", out)
	}
}

func TestToolFix(t *testing.T) {
	tool := exampleManifest(t).Tool("create_ticket")
	// 模型想自己定优先级：被管理员的设置覆盖
	args := map[string]any{"body": map[string]any{"title": "VPN 连不上", "priority": "高"}}
	if err := tool.Fix(args, map[string]json.RawMessage{"priority": json.RawMessage(`"低"`)}); err != nil {
		t.Fatal(err)
	}
	if got := args["body"].(map[string]any)["priority"]; got != "低" {
		t.Fatalf("应使用管理员设置的值：%v", got)
	}
	// 管理员没设置：用清单的 default；模型没给 body 也补上
	args = map[string]any{}
	if err := tool.Fix(args, nil); err != nil {
		t.Fatal(err)
	}
	if got := args["body"].(map[string]any)["priority"]; got != "中" {
		t.Fatalf("应使用清单的 default：%v", got)
	}
	// 没值的必填参数报错；不必填的没值时，模型给的也要拿掉
	req := &Tool{Params: []Param{{Name: "project", Fill: FillApp, Required: true}}}
	if err := req.Fix(map[string]any{}, nil); err == nil || !strings.Contains(err.Error(), "应用") {
		t.Fatalf("必填的应用参数没设置应报错：%v", err)
	}
	opt := &Tool{Params: []Param{{Name: "project", Fill: FillApp}}}
	args = map[string]any{"project": "B"}
	if err := opt.Fix(args, nil); err != nil || args["project"] != nil {
		t.Fatalf("模型不能替不由它填的参数填值：%v %v", err, args)
	}
}

func TestKeep(t *testing.T) {
	keep := []string{"id", "title"}
	cases := map[string]string{
		`[{"id":1,"title":"a","secret":"x"},{"id":2}]`: `[{"id":1,"title":"a"},{"id":2}]`,
		`{"id":1,"title":"a","notes":"long"}`:          `{"id":1,"title":"a"}`,
		`"plain"`:                                      `"plain"`,
		`not json`:                                     `not json`,
		`{"id":12345678901234567890}`:                  `{"id":12345678901234567890}`,
	}
	for in, want := range cases {
		if got := string(Keep([]byte(in), keep)); got != want {
			t.Errorf("Keep(%s) = %s，期望 %s", in, got, want)
		}
	}
	// 包了一层的列表：items.<字段> 筛每一项，total 整个留；整个字段留了就不再筛里面
	nested := []struct {
		keep     []string
		in, want string
	}{
		{[]string{"items.id", "items.title", "total"}, `{"items":[{"id":1,"title":"中文<b>","body":"long"}],"total":1,"page":2}`, `{"items":[{"id":1,"title":"中文<b>"}],"total":1}`},
		{[]string{"data.name"}, `{"data":{"name":"a","secret":"x"},"code":0}`, `{"data":{"name":"a"}}`},
		{[]string{"items", "items.id"}, `{"items":[{"id":1,"x":2}]}`, `{"items":[{"id":1,"x":2}]}`},
		{[]string{"items.id"}, `[{"items":[{"id":1,"x":2}],"n":1}]`, `[{"items":[{"id":1}]}]`},
		{[]string{"items.id"}, `{"items":"not a list"}`, `{"items":"not a list"}`},
	}
	for _, c := range nested {
		if got := string(Keep([]byte(c.in), c.keep)); got != c.want {
			t.Errorf("Keep(%s, %v) = %s，期望 %s", c.in, c.keep, got, c.want)
		}
	}
	if got := string(Keep([]byte(`{"a":1}`), nil)); got != `{"a":1}` {
		t.Errorf("没写 keep 应原样返回：%s", got)
	}
}

// 请求体里有不由模型填的字段时，body 传成字符串或数组不放行。
func TestToolFixRejectsNonObjectBody(t *testing.T) {
	tool := &Tool{Name: "create", Params: []Param{{Name: "body.priority", Fill: FillAdmin}}}
	for _, body := range []any{`{"priority":"高"}`, []any{1}} {
		if err := tool.Fix(map[string]any{"body": body}, nil); err == nil {
			t.Errorf("body 是 %T 时应拒绝", body)
		}
	}
	if err := tool.Fix(map[string]any{"body": map[string]any{"title": "x"}}, nil); err != nil {
		t.Fatal(err)
	}
	// 没有固定的请求体字段时不管 body 的样子
	if err := (&Tool{Name: "search"}).Fix(map[string]any{"body": "text"}, nil); err != nil {
		t.Fatal(err)
	}
}

func TestAdminParamCheckValue(t *testing.T) {
	p := AdminParam{Key: "level", Label: "级别", Type: "string", Enum: []any{"低", "中"}}
	for raw, ok := range map[string]bool{`"低"`: true, `"高"`: false, `1`: false} {
		if msg := p.CheckValue(json.RawMessage(raw)); (msg == "") != ok {
			t.Errorf("CheckValue(%s) = %q", raw, msg)
		}
	}
	if msg := (AdminParam{Key: "n", Type: "integer"}).CheckValue(json.RawMessage(`1.5`)); msg == "" {
		t.Error("1.5 不是整数")
	}
	if msg := (AdminParam{Key: "any"}).CheckValue(json.RawMessage(`{"a":1}`)); msg != "" {
		t.Errorf("没写类型的不限：%s", msg)
	}
}

// 用反斜杠分隔路径的 zip（一些 Windows 上的压缩工具）也认。
func TestReadZipBackslash(t *testing.T) {
	files := exampleFiles(t)
	win := map[string][]byte{}
	for name, b := range files {
		win[`demo-declarative\`+name] = b
	}
	got, err := ReadZip(zipOf(t, win))
	if err != nil {
		t.Fatal(err)
	}
	if res := CheckFiles(got); !res.OK {
		t.Fatalf("反斜杠路径的包应通过检查：%+v", res.Results)
	}
}
