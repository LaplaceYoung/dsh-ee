package registry

import (
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"
)

// Version 是一个可以对外提供的版本：清单 + 我们自己的下载地址。
type Version struct {
	Version   string
	Manifest  map[string]any
	Integrity string
	Shasum    string
	Time      time.Time
	// 非空表示已弃用（npm deprecate），客户端安装时会提示。
	Deprecated string
}

// Packument 是 npm 的包文档：一个包的全部可见版本。
type Packument struct {
	Name     string
	Versions []Version
	// 发布者设置的标签；Encode 时会去掉指向不可见版本的标签，并保证有 latest。
	DistTags map[string]string
}

// 精简文档（Accept: application/vnd.npm.install-v1+json）保留的版本字段，与 npm 官方 registry 一致。
var corgiFields = []string{
	"name", "version", "deprecated", "dependencies", "optionalDependencies", "devDependencies",
	"bundleDependencies", "bundledDependencies", "peerDependencies", "peerDependenciesMeta", "bin", "directories",
	"dist", "engines", "_hasShrinkwrap", "hasInstallScript", "cpu", "os", "libc", "funding", "acceptDependencies",
}

// 上游清单里与上游 registry 绑定、换了地址就失效或会误导客户端的字段。
var droppedFields = []string{"_resolved", "_from", "_shasum", "_integrity", "_downloads", "_cached", "_contentLength"}

// Encode 生成 packument JSON。tarballBase 形如 https://dsh.acme.internal/npm（不带结尾斜杠）。
func (p *Packument) Encode(tarballBase string, abbreviated bool) ([]byte, error) {
	versions := map[string]any{}
	times := map[string]string{}
	var names []string
	var created, modified time.Time
	for _, v := range p.Versions {
		doc := map[string]any{}
		for k, val := range v.Manifest {
			doc[k] = val
		}
		for _, k := range droppedFields {
			delete(doc, k)
		}
		doc["name"] = p.Name
		doc["version"] = v.Version
		doc["_id"] = p.Name + "@" + v.Version
		dist := map[string]any{
			"integrity": v.Integrity,
			"shasum":    v.Shasum,
			"tarball":   TarballURL(tarballBase, p.Name, v.Version),
		}
		// 保留上游 dist 里与地址无关的字段。signatures 签的是「名字@版本:integrity」，和 tarball 地址无关，
		// integrity 又是原样透传的，所以照样能用 npm 的公钥验过：corepack 经本源下载 pnpm 时要验这个，缺了就装不上。
		// attestations 指向上游 registry 的地址，去掉。
		if old, ok := v.Manifest["dist"].(map[string]any); ok {
			for _, k := range []string{"fileCount", "unpackedSize"} {
				if x, ok := old[k]; ok {
					dist[k] = x
				}
			}
			// 只在 integrity 和签名时的一致时带上，免得给别的内容配上不相干的签名
			if x, ok := old["signatures"]; ok && v.Integrity != "" && old["integrity"] == v.Integrity {
				dist["signatures"] = x
			}
		}
		doc["dist"] = dist
		if v.Deprecated != "" {
			doc["deprecated"] = v.Deprecated
		} else {
			delete(doc, "deprecated")
		}
		if abbreviated {
			if hasInstallScript(doc) {
				doc["hasInstallScript"] = true
			}
			slim := map[string]any{}
			for _, k := range corgiFields {
				if x, ok := doc[k]; ok {
					slim[k] = x
				}
			}
			doc = slim
		}
		versions[v.Version] = doc
		names = append(names, v.Version)
		t := v.Time.UTC()
		times[v.Version] = t.Format(time.RFC3339Nano)
		if created.IsZero() || t.Before(created) {
			created = t
		}
		if t.After(modified) {
			modified = t
		}
	}
	if modified.IsZero() {
		modified = time.Now().UTC()
		created = modified
	}

	tags := map[string]string{}
	for tag, v := range p.DistTags {
		if _, ok := versions[v]; ok {
			tags[tag] = v
		}
	}
	if _, ok := tags["latest"]; !ok && len(names) > 0 {
		tags["latest"] = latestOf(names)
	}

	out := map[string]any{
		"name":      p.Name,
		"dist-tags": tags,
		"versions":  versions,
	}
	if abbreviated {
		out["modified"] = modified.Format(time.RFC3339Nano)
	} else {
		out["_id"] = p.Name
		times["created"] = created.Format(time.RFC3339Nano)
		times["modified"] = modified.Format(time.RFC3339Nano)
		out["time"] = times
		// 顶层的描述类字段取 latest 版本的
		if latest := tags["latest"]; latest != "" {
			for _, v := range p.Versions {
				if v.Version != latest {
					continue
				}
				for _, k := range []string{"description", "license", "homepage", "repository", "keywords", "author", "bugs", "readme"} {
					if x, ok := v.Manifest[k]; ok {
						out[k] = x
					}
				}
			}
		}
	}
	return json.Marshal(out)
}

func hasInstallScript(doc map[string]any) bool {
	if b, ok := doc["hasInstallScript"].(bool); ok && b {
		return true
	}
	scripts, _ := doc["scripts"].(map[string]any)
	for _, s := range installScripts {
		if v, _ := scripts[s].(string); strings.TrimSpace(v) != "" {
			return true
		}
	}
	return false
}

// TarballURL 是我们对外的 tarball 地址：<base>/<name>/-/<file>，作用域包的 / 不转义（与 npm 官方一致）。
func TarballURL(base, name, version string) string {
	return base + "/" + escapeName(name) + "/-/" + url.PathEscape(TarballFile(name, version))
}

func escapeName(name string) string {
	if sc := Scope(name); sc != "" {
		return url.PathEscape(sc) + "/" + url.PathEscape(strings.TrimPrefix(name, sc+"/"))
	}
	return url.PathEscape(name)
}

// ---- 上游文档 ----

// UpstreamDoc 是上游 registry 返回的完整 packument，版本文档保持原始 JSON，按需解析。
type UpstreamDoc struct {
	Name     string                     `json:"name"`
	DistTags map[string]string          `json:"dist-tags"`
	Versions map[string]json.RawMessage `json:"versions"`
	Time     map[string]string          `json:"time"`
}

// UpstreamVersion 是上游某个版本的下载信息。
type UpstreamVersion struct {
	Version   string
	Manifest  map[string]any
	Parsed    *Manifest
	Tarball   string
	Integrity string
	Shasum    string
	Time      time.Time
}

func ParseUpstreamDoc(b []byte) (*UpstreamDoc, error) {
	var d UpstreamDoc
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, fmt.Errorf("Upstream packument is not valid JSON: %w", err)
	}
	if d.Versions == nil {
		d.Versions = map[string]json.RawMessage{}
	}
	return &d, nil
}

// VersionNames 返回全部版本号（升序）。
func (d *UpstreamDoc) VersionNames() []string {
	out := make([]string, 0, len(d.Versions))
	for v := range d.Versions {
		out = append(out, v)
	}
	SortVersions(out)
	return out
}

// Version 解析某个版本；不存在或文档损坏时返回 nil。
func (d *UpstreamDoc) Version(v string) *UpstreamVersion {
	raw, ok := d.Versions[v]
	if !ok {
		return nil
	}
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return nil
	}
	uv := &UpstreamVersion{Version: v, Manifest: m, Parsed: manifestFromMap(m)}
	if dist, ok := m["dist"].(map[string]any); ok {
		uv.Tarball = str(dist["tarball"])
		uv.Integrity = str(dist["integrity"])
		uv.Shasum = str(dist["shasum"])
	}
	if t, err := time.Parse(time.RFC3339Nano, d.Time[v]); err == nil {
		uv.Time = t
	}
	return uv
}

// MergeVersions 合并两组版本：同一版本号以 local 为准，按版本号排序。
func MergeVersions(upstream, local []Version) []Version {
	seen := map[string]bool{}
	out := slices.Clone(local)
	for _, v := range local {
		seen[v.Version] = true
	}
	for _, v := range upstream {
		if !seen[v.Version] {
			out = append(out, v)
		}
	}
	slices.SortFunc(out, func(a, b Version) int { return compareVersions(a.Version, b.Version) })
	return out
}
