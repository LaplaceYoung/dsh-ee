package registry

import (
	"sort"
	"strings"

	"github.com/Masterminds/semver/v3"
)

// 当前 dsh-ee v0.1 的推荐宿主：DSH 0.2.2-rc.1 使用 Cordis 4.1.0。
// DSH 升到新的 Cordis 线时改这里，并同步测试/文案。
const recommendedCordisVersion = "4.1.0"

// Cordis 由 DSH 宿主提供。插件如果把 Cordis 或 Cordis 插件打进 dependencies，
// 安装后会多出一份独立的 Cordis，服务注入和生命周期会各走各的；这是 DSH 插件
// 最常见的一类运行时故障，也直接拖慢启动/热重载。预检只提示，不替作者改包。
func checkCordisDependency(m *Manifest) *ScanItem {
	if m == nil {
		return nil
	}
	var bad []string
	seen := map[string]bool{}
	for _, deps := range []map[string]string{m.Dependencies, m.OptionalDependencies} {
		for name := range deps {
			if seen[name] || !isHostCordisPackage(name) {
				continue
			}
			seen[name] = true
			bad = append(bad, name)
		}
	}
	if len(bad) == 0 {
		return nil
	}
	sort.Strings(bad)
	if len(bad) > 8 {
		bad = append(bad[:8:8], "…")
	}
	return &ScanItem{Key: "cordis", Level: LevelWarn, Params: map[string]any{"packages": strings.Join(bad, "、")}}
}

func isHostCordisPackage(name string) bool {
	return name == "cordis" || name == "@deepseek-ai/cordis" || strings.HasPrefix(name, "@deepseek-ai/cordis-")
}

// 有些插件把 Cordis 写成 peerDependency 但范围排除了当前宿主使用的 4.1.0；
// 安装时 pnpm 会报 peer 不匹配，或运行时拿到不兼容的 Cordis API。
func checkCordisPeerRange(m *Manifest) *ScanItem {
	if m == nil {
		return nil
	}
	raw, ok := m.PeerDependencies["@deepseek-ai/cordis"]
	if !ok || strings.TrimSpace(raw) == "" {
		return nil
	}
	constraint, err := semver.NewConstraint(raw)
	if err != nil {
		return &ScanItem{Key: "cordis-peer", Level: LevelWarn, Params: map[string]any{"range": raw, "want": recommendedCordisVersion}}
	}
	if constraint.Check(semver.MustParse(recommendedCordisVersion)) {
		return nil
	}
	return &ScanItem{Key: "cordis-peer", Level: LevelWarn, Params: map[string]any{"range": raw, "want": recommendedCordisVersion}}
}
