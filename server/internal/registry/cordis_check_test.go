package registry

import (
	"bytes"
	"testing"
)

func cordisFixture(t *testing.T, pkg string) []ScanItem {
	t.Helper()
	data := tgz(t, map[string]string{
		"package/package.json":     pkg,
		"package/cordis.patch.yml": "- insert:\n    - id: demo\n      name: dsh-demo\n",
	})
	info, err := InspectTarball(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	m, err := ParseManifest(info.PackageJSON)
	if err != nil {
		t.Fatal(err)
	}
	return Check(m, info, int64(len(data)), nil)
}

func TestCheckCordisPeerRange(t *testing.T) {
	cases := []struct {
		name string
		peer string
		want bool
	}{
		{"caret includes 4.1", "^4.0.6", false},
		{"tilde excludes 4.1", "~4.0.6", true},
		{"exact old", "4.0.6", true},
		{"invalid range", "not-a-range", true},
		{"any", "*", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pkg := `{"name":"dsh-demo","version":"1.0.0","peerDependencies":{"@deepseek-ai/cordis":"` + tc.peer + `"},"dsh":{"bundle":{"patch":"./cordis.patch.yml"}}}`
			item := findScan(cordisFixture(t, pkg), "cordis-peer")
			if tc.want && (item == nil || item.Level != LevelWarn) {
				t.Fatalf("应提示 Cordis 版本范围：%+v", item)
			}
			if !tc.want && item != nil {
				t.Fatalf("兼容范围不应提示：%+v", item)
			}
		})
	}
}

func TestCheckCordisDependency(t *testing.T) {
	cases := []struct {
		name string
		pkg  string
		want bool
	}{
		{
			name: "direct cordis dependency",
			pkg:  `{"name":"dsh-demo","version":"1.0.0","dependencies":{"@deepseek-ai/cordis":"^4.1.0"},"dsh":{"bundle":{"patch":"./cordis.patch.yml"}}}`,
			want: true,
		},
		{
			name: "cordis plugin dependency",
			pkg:  `{"name":"dsh-demo","version":"1.0.0","optionalDependencies":{"@deepseek-ai/cordis-plugin-timer":"~1.2.0"},"dsh":{"bundle":{"patch":"./cordis.patch.yml"}}}`,
			want: true,
		},
		{
			name: "peer is fine",
			pkg:  `{"name":"dsh-demo","version":"1.0.0","peerDependencies":{"@deepseek-ai/cordis":"^4.1.0"},"dsh":{"bundle":{"patch":"./cordis.patch.yml"}}}`,
			want: false,
		},
		{
			name: "unscoped cordis is also host",
			pkg:  `{"name":"dsh-demo","version":"1.0.0","dependencies":{"cordis":"^4.0.0"},"dsh":{"bundle":{"patch":"./cordis.patch.yml"}}}`,
			want: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			item := findScan(cordisFixture(t, tc.pkg), "cordis")
			if tc.want {
				if item == nil || item.Level != LevelWarn {
					t.Fatalf("应提示 Cordis 依赖：%+v", item)
				}
			} else if item != nil {
				t.Fatalf("peer 依赖不应提示：%+v", item)
			}
		})
	}
}
