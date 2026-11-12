package openapi

import (
	"fmt"
	"strings"
	"testing"
)

func TestUniqueToolNameBoundsAndCase(t *testing.T) {
	used := map[string]bool{}
	base := strings.Repeat("a", 64)
	first := UniqueToolName(base, used)
	if first != base {
		t.Fatalf("首个名字应保持原样：%q", first)
	}
	for i := 0; i < 1200; i++ {
		name := UniqueToolName(base, used)
		if len(name) > 64 {
			t.Fatalf("工具名超过 64 字节：%q", name)
		}
		if strings.ToLower(name) == strings.ToLower(first) {
			t.Fatalf("工具名重复：%q", name)
		}
	}
	// 大小写不同也视为同一个名字
	used = map[string]bool{}
	_ = UniqueToolName("GetUser", used)
	if got := UniqueToolName("getuser", used); got == "getuser" {
		t.Fatalf("大小写不同应生成后缀：%q", got)
	}
}

func TestParseDeduplicatesLongOperationIDs(t *testing.T) {
	var b strings.Builder
	b.WriteString(`{"openapi":"3.0.0","info":{"title":"x","version":"1"},"paths":{`)
	id := strings.Repeat("a", 64)
	for i := 0; i < 1200; i++ {
		fmt.Fprintf(&b, `"/p%d":{"get":{"operationId":%q}}`, i, id)
		if i < 1199 {
			b.WriteByte(',')
		}
	}
	b.WriteString("}}")
	spec, err := Parse([]byte(b.String()))
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.Tools) != 1200 {
		t.Fatalf("工具数 = %d", len(spec.Tools))
	}
	seen := map[string]bool{}
	for _, tool := range spec.Tools {
		if len(tool.Name) > 64 {
			t.Fatalf("工具名超过 64 字节：%q", tool.Name)
		}
		key := strings.ToLower(tool.Name)
		if seen[key] {
			t.Fatalf("工具名重复：%q", tool.Name)
		}
		seen[key] = true
	}
}
