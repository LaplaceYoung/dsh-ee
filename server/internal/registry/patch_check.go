package registry

import (
	"strings"

	"gopkg.in/yaml.v3"
)

// checkBundlePatch 真正解析 dsh.bundle.patch：YAML 数组、行结构、id 唯一、
// 插入行 name 合法，并且至少一行引用本包。读端/profile 加载失败大多来自这里。
func checkBundlePatch(m *Manifest, info *TarballInfo) *ScanItem {
	if m == nil || !m.IsPlugin || m.BundlePatch == "" || !safeRelPath(m.BundlePatch) {
		return nil
	}
	rel := strings.TrimPrefix(m.BundlePatch, "./")
	raw, ok := info.YAML[rel]
	if !ok {
		if info.Files[rel] {
			return patchItem(LevelFail, map[string]any{"path": rel, "reason": "unreadable"})
		}
		return nil // bundle 检查项已经报 file-missing
	}
	var doc any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return patchItem(LevelFail, map[string]any{"path": rel, "reason": "yaml", "error": err.Error()})
	}
	rows, ok := doc.([]any)
	if !ok {
		return patchItem(LevelFail, map[string]any{"path": rel, "reason": "not-array"})
	}
	if len(rows) == 0 {
		return patchItem(LevelFail, map[string]any{"path": rel, "reason": "empty"})
	}
	seen := map[string]int{}
	own := false
	for i, rawRow := range rows {
		rowNo := i + 1
		row, ok := rawRow.(map[string]any)
		if !ok {
			return patchItem(LevelFail, map[string]any{"path": rel, "reason": "row-not-object", "row": rowNo})
		}
		if id, ok := row["id"]; ok {
			if item := recordPatchID(id, rowNo, rel, seen); item != nil {
				return item
			}
		}
		if name, ok := row["name"].(string); ok && strings.TrimSpace(name) == m.Name {
			own = true
		}
		if rawInsert, ok := row["insert"]; ok {
			list, ok := rawInsert.([]any)
			if !ok {
				return patchItem(LevelFail, map[string]any{"path": rel, "reason": "insert-not-array", "row": rowNo})
			}
			for j, rawChild := range list {
				childNo := j + 1
				child, ok := rawChild.(map[string]any)
				if !ok {
					return patchItem(LevelFail, map[string]any{"path": rel, "reason": "row-not-object", "row": rowNo, "insert": childNo})
				}
				id, ok := child["id"]
				if !ok {
					return patchItem(LevelFail, map[string]any{"path": rel, "reason": "missing-id", "row": rowNo, "insert": childNo})
				}
				if item := recordPatchID(id, rowNo, rel, seen); item != nil {
					return item
				}
				name, _ := child["name"].(string)
				if strings.TrimSpace(name) == "" {
					return patchItem(LevelFail, map[string]any{"path": rel, "reason": "missing-name", "row": rowNo, "insert": childNo})
				}
				if strings.TrimSpace(name) == m.Name {
					own = true
				}
				if cfg, ok := child["config"]; ok {
					if _, ok := cfg.(map[string]any); !ok {
						return patchItem(LevelWarn, map[string]any{"path": rel, "reason": "config-not-map", "name": name})
					}
				}
			}
		}
	}
	if !own {
		// 组合包可能只插入依赖/配置行；名字不匹配说明 profile 很可能找不到插件，
		// 但不同组合方式有例外，先作为警告提示，不阻断发布。
		return patchItem(LevelWarn, map[string]any{"path": rel, "reason": "name-mismatch", "name": m.Name})
	}
	return patchItem(LevelPass, map[string]any{"path": rel, "rows": len(rows)})
}

func recordPatchID(raw any, rowNo int, patchPath string, seen map[string]int) *ScanItem {
	id, ok := raw.(string)
	if !ok || strings.TrimSpace(id) == "" {
		return patchItem(LevelFail, map[string]any{"path": patchPath, "reason": "missing-id", "row": rowNo})
	}
	if first, exists := seen[id]; exists {
		return patchItem(LevelFail, map[string]any{"path": patchPath, "reason": "duplicate-id", "id": id, "row": rowNo, "first": first})
	}
	seen[id] = rowNo
	return nil
}

func patchItem(level string, params map[string]any) *ScanItem {
	return &ScanItem{Key: "patch", Level: level, Params: params}
}
