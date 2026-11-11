package extension

import (
	"embed"
	"encoding/json"
	"sort"
	"strings"
)

// 规格文件编译在二进制里，离线可用；仓库里 extensions/schemas/ 下是同一份（有测试保证一致）。
//
//go:embed schemas/*.json
var schemaFiles embed.FS

// SchemaInfo 是一份规格的名字和一句话说明。
type SchemaInfo struct {
	Name  string `json:"name"`
	Title string `json:"title"`
}

// Schemas 列出全部规格。
func Schemas() []SchemaInfo {
	out := []SchemaInfo{{Name: "errors", Title: "`dsh-ee extension check` 会报的全部检查项：含义、严重程度、能不能忽略"}}
	entries, _ := schemaFiles.ReadDir("schemas")
	for _, e := range entries {
		raw, err := schemaFiles.ReadFile("schemas/" + e.Name())
		if err != nil {
			continue
		}
		var head struct {
			Title string `json:"title"`
		}
		_ = json.Unmarshal(raw, &head)
		out = append(out, SchemaInfo{Name: strings.TrimSuffix(strings.TrimSuffix(e.Name(), ".json"), ".schema"), Title: head.Title})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Schema 取一份规格的内容；没有这个名字时返回 false。
func Schema(name string) ([]byte, bool) {
	if name == "errors" {
		raw, _ := json.MarshalIndent(map[string]any{"title": "dsh-ee extension check 的检查项", "codes": Codes}, "", "  ")
		return append(raw, '\n'), true
	}
	if strings.ContainsAny(name, "/\\.") {
		return nil, false
	}
	raw, err := schemaFiles.ReadFile("schemas/" + name + ".schema.json")
	if err != nil {
		return nil, false
	}
	return raw, true
}
