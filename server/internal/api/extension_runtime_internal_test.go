package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hikariming/dsh-ee/server/internal/extension"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

func TestDeclarativeToolSchemaAndParams(t *testing.T) {
	m := &extension.Manifest{ID: "acme/tasks", Version: "1.0.0", Tools: []extension.Tool{{
		Name: "create_item", ReadOnly: false,
		Params: []extension.Param{
			{Name: "body.priority", Type: "string", Fill: extension.FillAdmin, Default: json.RawMessage(`"中"`), Options: json.RawMessage(`["中","高"]`)},
			{Name: "body.tenant", Type: "string", Fill: extension.FillAdmin},
			{Name: "body.count", Type: "integer", Fill: extension.FillApp, Default: json.RawMessage(`2`), Enum: []any{float64(1), float64(2)}},
			{Name: "body.choice", Type: "string", Enum: []any{"safe"}},
		},
	}}}
	tool := &store.ConnectionTool{Name: "create_item", InputSchema: json.RawMessage(`{
		"type":"object",
		"properties":{
			"title":{"type":"string"},
			"body":{"type":"object","required":["priority","tenant","count"],"properties":{
				"title":{"type":"string"},"priority":{"type":"string"},"tenant":{"type":"string"},"count":{"type":"integer"}
			}}
		},
		"required":["body"]
	}`)}
	if err := declarativeToolSchema(tool, m); err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if err := json.Unmarshal(tool.InputSchema, &schema); err != nil {
		t.Fatal(err)
	}
	body := schema["properties"].(map[string]any)["body"].(map[string]any)
	props := body["properties"].(map[string]any)
	for _, fixed := range []string{"priority", "tenant", "count"} {
		if _, exists := props[fixed]; exists {
			t.Fatalf("固定参数仍对模型可见：%s；schema=%s", fixed, tool.InputSchema)
		}
	}
	if _, exists := body["required"]; exists {
		t.Fatalf("固定参数全部移除后不应留下空 required：%v", body["required"])
	}
	// title 还能由模型填，所以 root body 仍要保留，否则随便一个无 body 的请求也能绕过业务必填规则。
	if len(schema["required"].([]any)) != 1 {
		t.Fatalf("body 本身仍应要求：%v", schema["required"])
	}

	config := map[string]json.RawMessage{
		"body.priority": json.RawMessage(`"高"`),
		"body.tenant":   json.RawMessage(`"secret-tenant"`),
	}
	args := map[string]any{"body": map[string]any{"title": "new", "priority": "伪造", "tenant": "伪造", "count": 999, "choice": "safe"}}
	if err := extensionParams(m, "create_item", config, args); err != nil {
		t.Fatal(err)
	}
	bodyArgs := args["body"].(map[string]any)
	if bodyArgs["priority"] != "高" || bodyArgs["tenant"] != "secret-tenant" || bodyArgs["count"] != float64(2) {
		t.Fatalf("固定参数没有按配置覆盖：%v", bodyArgs)
	}
	// 数字枚举用 json.Number 与 float64 做规范化比较，不能因为表示不同而误拒合法配置。
	args["body"] = map[string]any{"title": "new", "choice": "不允许"}
	if err := extensionParams(m, "create_item", config, args); err == nil {
		t.Fatal("枚举外的值应被拒绝")
	}
}

func TestExtensionOptionalEnumAllowsMissing(t *testing.T) {
	m := &extension.Manifest{Tools: []extension.Tool{{Name: "list", Params: []extension.Param{
		{Name: "body.choice", Enum: []any{"safe"}},
		{Name: "body.unset", Fill: extension.FillAdmin},
	}}}}
	args := map[string]any{"body": map[string]any{"unset": "模型伪造"}}
	if err := extensionParams(m, "list", nil, args); err != nil {
		t.Fatalf("可选枚举缺省不应被拒绝：%v", err)
	}
	if _, exists := args["body"].(map[string]any)["unset"]; exists {
		t.Fatal("未配置的固定参数应删除伪造值，而不是发给上游")
	}
}

func TestExtensionSelectConfigValidation(t *testing.T) {
	field := extensionInstallField{Key: "body.count", Type: "select", Options: installOptions(json.RawMessage(`[1,2]`)), Enum: []any{float64(1), float64(2)}}
	if err := validInstallValue(field, json.RawMessage(`2`)); err != nil {
		t.Fatalf("数字枚举 value 应按值归一化通过：%v", err)
	}
	if err := validInstallValue(field, json.RawMessage(`3`)); err == nil {
		t.Fatal("枚举外的安装配置必须在安装时拒绝")
	}
}

func TestTrimExtensionResult(t *testing.T) {
	input := `[{"id":1,"title":"ok","secret":"不能给模型"},{"id":2,"title":"second","secret":"x"}]`
	got, err := trimExtensionResult(input, []string{"id", "title"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "secret") || !strings.Contains(got, "second") {
		t.Fatalf("keep 没有真正删除返回字段：%s", got)
	}
	var items []map[string]any
	if err := json.Unmarshal([]byte(got), &items); err != nil || len(items) != 2 || len(items[0]) != 2 {
		t.Fatalf("keep 后的结构错误：%s err=%v", got, err)
	}
	if _, err := trimExtensionResult("not-json", []string{"id"}); err == nil {
		t.Fatal("无法过滤的非 JSON 响应不能原样泄漏给模型")
	}
}

func TestReadExtensionArchiveSafety(t *testing.T) {
	manifest := []byte(`{"id":"acme/demo","version":"1.0.0","name":"demo","kind":"declarative","openapi":"openapi.yaml","servers":["http://127.0.0.1"]}`)
	document := []byte("openapi: 3.0.0\ninfo: {title: demo}\npaths: {}\n")
	build := func(names map[string][]byte) []byte {
		var b bytes.Buffer
		zw := zip.NewWriter(&b)
		for name, data := range names {
			w, _ := zw.Create(name)
			_, _ = w.Write(data)
		}
		_ = zw.Close()
		return b.Bytes()
	}
	good := build(map[string][]byte{extension.ManifestFile: manifest, "openapi.yaml": document})
	arc, err := readExtensionArchive(good)
	if err != nil || len(arc.manifestRaw) == 0 || len(arc.openapiRaw) == 0 {
		t.Fatalf("正常 ZIP 应读出清单与文档：%+v err=%v", arc, err)
	}
	for name, files := range map[string]map[string][]byte{
		"路径穿越": {extension.ManifestFile: manifest, "../openapi.yaml": document},
		"绝对路径": {extension.ManifestFile: manifest, "/openapi.yaml": document},
	} {
		if _, err := readExtensionArchive(build(files)); err == nil {
			t.Fatalf("%s应被拒绝", name)
		}
	}
}
