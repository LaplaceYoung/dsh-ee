package registry

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func patchFixture(t *testing.T, patch string) (*Manifest, *TarballInfo, int64) {
	t.Helper()
	pkg := `{"name":"dsh-demo","version":"1.0.0","dsh":{"bundle":{"patch":"./cordis.patch.yml"}},"engines":{"dsh":">=0.2.0 <0.3.0"}}`
	data := tgz(t, map[string]string{
		"package/package.json":     pkg,
		"package/cordis.patch.yml": patch,
	})
	info, err := InspectTarball(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	m, err := ParseManifest(info.PackageJSON)
	if err != nil {
		t.Fatal(err)
	}
	return m, info, int64(len(data))
}

func patchItemOf(t *testing.T, items []ScanItem) *ScanItem {
	t.Helper()
	for i := range items {
		if items[i].Key == "patch" {
			return &items[i]
		}
	}
	return nil
}

func TestCheckBundlePatch(t *testing.T) {
	cases := []struct {
		name   string
		patch  string
		level  string
		reason string
	}{
		{
			name:  "正常插入",
			patch: "- insert:\n    - id: dsh-demo\n      name: dsh-demo\n",
			level: LevelPass,
		},
		{
			name:   "缺少本包名",
			patch:  "- insert:\n    - id: other\n      name: dsh-other\n",
			level:  LevelWarn,
			reason: "name-mismatch",
		},
		{
			name:   "重复 row id",
			patch:  "- insert:\n    - id: row\n      name: dsh-demo\n    - id: row\n      name: dsh-demo\n",
			level:  LevelFail,
			reason: "duplicate-id",
		},
		{
			name:   "缺少 row id",
			patch:  "- insert:\n    - name: dsh-demo\n",
			level:  LevelFail,
			reason: "missing-id",
		},
		{
			name:   "非 YAML",
			patch:  "- insert: [\n",
			level:  LevelFail,
			reason: "yaml",
		},
		{
			name:   "顶层不是数组",
			patch:  "insert:\n  - id: dsh-demo\n",
			level:  LevelFail,
			reason: "not-array",
		},
		{
			name:   "插入不是数组",
			patch:  "- insert:\n    id: dsh-demo\n",
			level:  LevelFail,
			reason: "insert-not-array",
		},
		{
			name:   "插入行缺 name",
			patch:  "- insert:\n    - id: dsh-demo\n",
			level:  LevelFail,
			reason: "missing-name",
		},
		{
			name:   "config 不是 mapping",
			patch:  "- insert:\n    - id: dsh-demo\n      name: dsh-demo\n      config: 1\n",
			level:  LevelWarn,
			reason: "config-not-map",
		},
		{
			name:  "顶层 patch 行加自己的 insert",
			patch: "- id: ui-sidebar\n  disabled: false\n- insert:\n    - id: dsh-demo\n      name: dsh-demo\n",
			level: LevelPass,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, info, size := patchFixture(t, tc.patch)
			item := patchItemOf(t, Check(m, info, size, nil))
			if item == nil {
				t.Fatalf("缺少 patch 检查项：%+v", Check(m, info, size, nil))
			}
			if item.Level != tc.level {
				t.Fatalf("level = %s，期望 %s（%+v）", item.Level, tc.level, item.Params)
			}
			if tc.reason != "" && fmt.Sprint(item.Params["reason"]) != tc.reason {
				t.Fatalf("reason = %v，期望 %s", item.Params["reason"], tc.reason)
			}
			if tc.level == LevelPass && !strings.Contains(fmt.Sprint(item.Params["path"]), "cordis.patch.yml") {
				t.Fatalf("pass 结果应带 patch 路径：%+v", item.Params)
			}
		})
	}
}
