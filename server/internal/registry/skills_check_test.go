package registry

import (
	"bytes"
	"testing"
)

func skillsFixture(t *testing.T, files map[string]string) []ScanItem {
	t.Helper()
	base := map[string]string{
		"package/package.json":     `{"name":"dsh-demo","version":"1.0.0","dsh":{"bundle":{"patch":"./cordis.patch.yml"}}}`,
		"package/cordis.patch.yml": "- insert:\n    - id: dsh-demo\n      name: dsh-demo\n",
	}
	for name, content := range files {
		base[name] = content
	}
	data := tgz(t, base)
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

func TestCheckSkillsVisibility(t *testing.T) {
	withSkills := skillsFixture(t, map[string]string{
		"package/skills/one/SKILL.md": "# one\n",
		"package/skills/two/SKILL.md": "# two\n",
		"package/skills/two/ref.md":   "ref\n",
	})
	item := findScan(withSkills, "skills")
	if item == nil || item.Level != LevelWarn || item.Params["count"] != 2 {
		t.Fatalf("应提示包内 2 个技能：%+v", item)
	}
	if files, _ := item.Params["files"].(string); files != "skills/one/SKILL.md、skills/two/SKILL.md" {
		t.Fatalf("技能路径不对：%q", files)
	}
	without := findScan(skillsFixture(t, nil), "skills")
	if without == nil || without.Level != LevelPass {
		t.Fatalf("没有 SKILL.md 时应 pass：%+v", without)
	}
}

func findScan(items []ScanItem, key string) *ScanItem {
	for i := range items {
		if items[i].Key == key {
			return &items[i]
		}
	}
	return nil
}
