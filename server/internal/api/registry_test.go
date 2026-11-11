package api_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/api"
	"github.com/hikariming/dsh-ee/server/internal/registry"
	"github.com/hikariming/dsh-ee/server/internal/secret"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// ---- 测试用的包 ----

// makeTgz 打一个 npm pack 格式的 tarball：package/package.json + 其它文件。
func makeTgz(t *testing.T, pkg map[string]any, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	pj, _ := json.Marshal(pkg)
	all := map[string]string{"package.json": string(pj)}
	for k, v := range files {
		all[k] = v
	}
	for name, body := range all {
		if err := tw.WriteHeader(&tar.Header{Name: "package/" + name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		_, _ = tw.Write([]byte(body))
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func digests(b []byte) (integrity, shasum string) {
	s512 := sha512.Sum512(b)
	s1 := sha1.Sum(b)
	return "sha512-" + base64.StdEncoding.EncodeToString(s512[:]), hex.EncodeToString(s1[:])
}

// publishBody 模拟 npm publish 发出的请求体。
func publishBody(name, version string, tgz []byte, pkg map[string]any, tag string) map[string]any {
	integrity, shasum := digests(tgz)
	vdoc := map[string]any{}
	for k, v := range pkg {
		vdoc[k] = v
	}
	vdoc["dist"] = map[string]any{"integrity": integrity, "shasum": shasum, "tarball": "http://x/" + name + "/-/x.tgz"}
	if tag == "" {
		tag = "latest"
	}
	return map[string]any{
		"_id": name, "name": name, "dist-tags": map[string]string{tag: version},
		"versions": map[string]any{version: vdoc},
		"_attachments": map[string]any{registry.TarballFile(name, version): map[string]any{
			"content_type": "application/octet-stream", "data": base64.StdEncoding.EncodeToString(tgz), "length": len(tgz),
		}},
	}
}

// fakeRegistry 是一个假的上游 npm registry，记录每个包被请求了几次。
type fakeRegistry struct {
	*httptest.Server
	mu    sync.Mutex
	docs  map[string]map[string]any
	tgz   map[string][]byte
	hits  map[string]int
	feeds []map[string]any
}

func newFakeRegistry(t *testing.T) *fakeRegistry {
	f := &fakeRegistry{docs: map[string]map[string]any{}, tgz: map[string][]byte{}, hits: map[string]int{}}
	f.Server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeRegistry) add(t *testing.T, pkg map[string]any, files map[string]string) {
	name, version := pkg["name"].(string), pkg["version"].(string)
	tgz := makeTgz(t, pkg, files)
	integrity, shasum := digests(tgz)
	file := registry.TarballFile(name, version)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tgz[name+"/"+file] = tgz
	doc := f.docs[name]
	if doc == nil {
		doc = map[string]any{"name": name, "dist-tags": map[string]any{}, "versions": map[string]any{}, "time": map[string]any{}}
		f.docs[name] = doc
	}
	v := map[string]any{}
	for k, val := range pkg {
		v[k] = val
	}
	v["dist"] = map[string]any{"integrity": integrity, "shasum": shasum, "tarball": f.URL + "/" + name + "/-/" + file}
	doc["versions"].(map[string]any)[version] = v
	doc["dist-tags"].(map[string]any)["latest"] = version
	doc["time"].(map[string]any)[version] = time.Now().UTC().Format(time.RFC3339)
}

func (f *fakeRegistry) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.URL.Path == "/-/ping" {
		_, _ = io.WriteString(w, "{}")
		return
	}
	if r.URL.Path == "/feed.json" {
		_ = json.NewEncoder(w).Encode(map[string]any{"format": 1, "plugins": f.feeds})
		return
	}
	p := strings.TrimPrefix(r.URL.Path, "/")
	if i := strings.Index(p, "/-/"); i >= 0 {
		name := p[:i]
		f.hits[name]++
		if b, ok := f.tgz[name+"/"+p[i+3:]]; ok {
			_, _ = w.Write(b)
			return
		}
		http.NotFound(w, r)
		return
	}
	f.hits[p]++
	doc, ok := f.docs[p]
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(doc)
}

func (f *fakeRegistry) hitCount(name string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.hits[name]
}

func pluginPkg(name, version string, extra map[string]any) map[string]any {
	p := map[string]any{"name": name, "version": version, "description": "测试插件 " + name, "license": "MIT",
		"dsh": map[string]any{"bundle": map[string]any{"patch": "cordis.patch.yml"}}, "engines": map[string]any{"dsh": ">=0.2.0"}}
	for k, v := range extra {
		p[k] = v
	}
	return p
}

var bundleFile = map[string]string{"cordis.patch.yml": "- {}\n"}

func newRegistryServer(t *testing.T, st *store.Store, up *fakeRegistry) *httptest.Server {
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	box, _ := secret.NewBox(key)
	srv := api.New(st, box, api.Options{
		DataDir:     t.TempDir(),
		NpmUpstream: up.Client(),
		FeedURL:     up.URL + "/feed.json",
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts
}

func decodeAs[T any](t *testing.T, b []byte) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("解析响应：%v；%s", err, b)
	}
	return v
}

func waitJob(t *testing.T, c *client, id int64) store.SyncJob {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		code, body := c.do("GET", "/api/admin/upstream/jobs/"+itoa(id), nil, nil)
		expect(t, "查任务", code, 200, body)
		j := decodeAs[store.SyncJob](t, body)
		if j.Status != "running" {
			return j
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("任务 %d 超时", id)
	return store.SyncJob{}
}

type packument struct {
	DistTags map[string]string `json:"dist-tags"`
	Versions map[string]struct {
		Name    string `json:"name"`
		Version string `json:"version"`
		Dist    struct {
			Tarball   string `json:"tarball"`
			Integrity string `json:"integrity"`
		} `json:"dist"`
		Deprecated string `json:"deprecated"`
	} `json:"versions"`
}

func TestRegistry(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if _, err := api.Bootstrap(ctx, st, "admin", "admin-password-123"); err != nil {
		t.Fatal(err)
	}
	up := newFakeRegistry(t)
	up.add(t, map[string]any{"name": "left-pad", "version": "1.0.0", "license": "MIT"}, nil)
	up.add(t, map[string]any{"name": "left-pad", "version": "1.3.0", "license": "MIT"}, nil)
	up.add(t, map[string]any{"name": "is-number", "version": "7.0.0"}, nil)
	up.add(t, map[string]any{"name": "@deepseek-ai/dsh-core", "version": "0.2.1"}, nil)
	up.add(t, pluginPkg("dsh-plugin-cool", "2.0.0", map[string]any{
		"dependencies":     map[string]any{"left-pad": "^1.0.0"},
		"peerDependencies": map[string]any{"@deepseek-ai/dsh-core": "^0.2.0"},
	}), bundleFile)
	// 企业作用域的同名包挂在上游：dsh-ee 永远不该去请求它（依赖混淆）
	up.add(t, map[string]any{"name": "@acme/evil", "version": "9.9.9"}, nil)
	up.feeds = []map[string]any{{"name": "dsh-plugin-cool", "title": "Cool 插件", "metadata": map[string]any{"category": "tools", "labels": []string{"featured"}, "sortKeys": map[string]any{"stars": 120, "score": 88}}, "versions": []map[string]any{{"version": "2.0.0", "publishTimestamp": 1790000000}}}}

	ts := newRegistryServer(t, st, up)
	admin := newClient(t, ts.URL)
	code, body := admin.do("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "admin-password-123"}, nil)
	expect(t, "管理员登录", code, 200, body)

	// 设置：主作用域 @acme，自定义上游指向假 registry，按需回源，要求令牌
	code, body = admin.do("GET", "/api/settings", nil, nil)
	expect(t, "读设置", code, 200, body)
	set := decodeAs[map[string]any](t, body)
	set["scope"] = "@acme"
	set["orgName"] = "ACME"
	set["upstream"].(map[string]any)["registry"] = "custom"
	set["upstream"].(map[string]any)["customRegistry"] = up.URL
	code, body = admin.do("PUT", "/api/admin/settings", set, nil)
	expect(t, "保存设置", code, 200, body)
	bad := decodeAs[map[string]any](t, body)
	bad["scope"] = "acme"
	code, body = admin.do("PUT", "/api/admin/settings", bad, nil)
	expect(t, "非法作用域", code, 400, body)

	// 员工 alice：可发布令牌与只读令牌
	code, body = admin.do("POST", "/api/admin/users", map[string]any{"username": "alice", "password": "alice-password-1", "department": "研发效能部"}, nil)
	expect(t, "建员工", code, 201, body)
	alice := newClient(t, ts.URL)
	code, body = alice.do("POST", "/api/auth/login", map[string]string{"username": "alice", "password": "alice-password-1"}, nil)
	expect(t, "员工登录", code, 200, body)
	code, body = alice.do("POST", "/api/me/npm-tokens", map[string]string{"name": "laptop", "access": "publish"}, nil)
	expect(t, "建可发布令牌", code, 201, body)
	pubTok := decodeAs[map[string]any](t, body)["token"].(string)
	code, body = alice.do("POST", "/api/me/npm-tokens", map[string]string{"name": "ci", "access": "read"}, nil)
	expect(t, "建只读令牌", code, 201, body)
	readTok := decodeAs[map[string]any](t, body)["token"].(string)
	if !strings.HasPrefix(pubTok, "ee_npm_") {
		t.Fatalf("令牌前缀不对：%s", pubTok)
	}
	npm := func(tok string) map[string]string { return map[string]string{"Authorization": "Bearer " + tok} }
	anon := newClient(t, ts.URL)

	t.Run("读需要令牌", func(t *testing.T) {
		code, body := anon.do("GET", "/npm/left-pad", nil, nil)
		expect(t, "匿名读", code, 401, body)
		code, body = anon.do("GET", "/npm/left-pad", nil, npm("ee_npm_bogus"))
		expect(t, "错误令牌", code, 401, body)
		code, body = anon.do("GET", "/npm/left-pad", nil, npm("sk-ee-not-npm"))
		expect(t, "非 npm 令牌", code, 401, body)
	})

	t.Run("回源上游普通依赖", func(t *testing.T) {
		code, body := anon.do("GET", "/npm/left-pad", nil, npm(readTok))
		expect(t, "回源 packument", code, 200, body)
		p := decodeAs[packument](t, body)
		if p.DistTags["latest"] != "1.3.0" || len(p.Versions) != 2 {
			t.Fatalf("packument 不对：%s", body)
		}
		tb := p.Versions["1.3.0"].Dist.Tarball
		if tb != ts.URL+"/npm/left-pad/-/left-pad-1.3.0.tgz" {
			t.Fatalf("tarball 地址没有改写成 dsh-ee：%s", tb)
		}
		code, body = anon.do("GET", strings.TrimPrefix(tb, ts.URL), nil, npm(readTok))
		expect(t, "回源 tarball", code, 200, body)
		if integ, _ := digests(body); integ != p.Versions["1.3.0"].Dist.Integrity {
			t.Fatal("下载的 tarball 与 integrity 不一致")
		}
		hits := up.hitCount("left-pad")
		code, _ = anon.do("GET", strings.TrimPrefix(tb, ts.URL), nil, npm(readTok))
		expect(t, "再次下载", code, 200, nil)
		if up.hitCount("left-pad") != hits {
			t.Fatal("第二次下载应该用本地缓存，不再访问上游")
		}
		// 精简文档
		req, _ := http.NewRequest("GET", ts.URL+"/npm/left-pad", nil)
		req.Header.Set("Authorization", "Bearer "+readTok)
		req.Header.Set("Accept", "application/vnd.npm.install-v1+json; q=1.0, application/json; q=0.8")
		resp, err := http.DefaultClient.Do(req)
		if err != nil || resp.Header.Get("Content-Type") != "application/vnd.npm.install-v1+json" {
			t.Fatalf("精简文档：%v %s", err, resp.Header.Get("Content-Type"))
		}
		resp.Body.Close()
	})

	t.Run("企业作用域永不回源", func(t *testing.T) {
		code, body := anon.do("GET", "/npm/@acme%2fevil", nil, npm(readTok))
		expect(t, "企业作用域里不存在的包", code, 404, body)
		code, body = anon.do("GET", "/npm/@acme/evil/-/evil-9.9.9.tgz", nil, npm(readTok))
		expect(t, "企业作用域里不存在的 tarball", code, 404, body)
		if n := up.hitCount("@acme/evil") + up.hitCount("@acme%2Fevil"); n != 0 {
			t.Fatalf("企业作用域的包去上游请求了 %d 次", n)
		}
	})

	t.Run("上游插件未审核不可见", func(t *testing.T) {
		code, body := anon.do("GET", "/npm/dsh-plugin-cool", nil, npm(readTok))
		expect(t, "未审核的上游插件", code, 404, body)
		code, body = anon.do("GET", "/npm/dsh-plugin-cool/-/dsh-plugin-cool-2.0.0.tgz", nil, npm(readTok))
		expect(t, "未审核的上游插件 tarball", code, 404, body)
	})

	hello := pluginPkg("@acme/hello", "1.0.0", map[string]any{"dsh-ee": map[string]any{"title": "你好插件", "category": "tools", "kind": "skill"}, "dependencies": map[string]any{"left-pad": "^1.0.0"}})
	helloTgz := makeTgz(t, hello, bundleFile)

	t.Run("发布到企业作用域", func(t *testing.T) {
		code, body := anon.do("PUT", "/npm/@acme%2fhello", publishBody("@acme/hello", "1.0.0", helloTgz, hello, ""), npm(readTok))
		expect(t, "只读令牌发布", code, 403, body)
		code, body = anon.do("PUT", "/npm/@acme%2fhello", publishBody("@acme/hello", "1.0.0", helloTgz, hello, ""), nil)
		expect(t, "匿名发布", code, 401, body)

		other := pluginPkg("@other/x", "1.0.0", nil)
		code, body = anon.do("PUT", "/npm/@other%2fx", publishBody("@other/x", "1.0.0", makeTgz(t, other, bundleFile), other, ""), npm(pubTok))
		expect(t, "发布到别的作用域", code, 403, body)
		plain := pluginPkg("unscoped-thing", "1.0.0", nil)
		code, body = anon.do("PUT", "/npm/unscoped-thing", publishBody("unscoped-thing", "1.0.0", makeTgz(t, plain, bundleFile), plain, ""), npm(pubTok))
		expect(t, "发布无作用域的包", code, 403, body)

		// 元数据说 1.0.0，tarball 里写的是 1.0.1
		lie := pluginPkg("@acme/hello", "1.0.1", nil)
		code, body = anon.do("PUT", "/npm/@acme%2fhello", publishBody("@acme/hello", "1.0.0", makeTgz(t, lie, bundleFile), hello, ""), npm(pubTok))
		expect(t, "清单与 tarball 不一致", code, 400, body)
		// 声明的摘要对不上
		pb := publishBody("@acme/hello", "1.0.0", helloTgz, hello, "")
		pb["versions"].(map[string]any)["1.0.0"].(map[string]any)["dist"].(map[string]any)["integrity"] = "sha512-AAAA"
		code, body = anon.do("PUT", "/npm/@acme%2fhello", pb, npm(pubTok))
		expect(t, "摘要不一致", code, 400, body)

		req, _ := json.Marshal(publishBody("@acme/hello", "1.0.0", helloTgz, hello, ""))
		r, _ := http.NewRequest("PUT", ts.URL+"/npm/@acme%2fhello", bytes.NewReader(req))
		r.Header.Set("Authorization", "Bearer "+pubTok)
		r.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		expect(t, "发布", resp.StatusCode, 201, b)
		if !strings.Contains(resp.Header.Get("npm-notice"), "pending review") {
			t.Fatalf("待审核时应带 npm-notice：%q", resp.Header.Get("npm-notice"))
		}
		code, body = anon.do("PUT", "/npm/@acme%2fhello", publishBody("@acme/hello", "1.0.0", helloTgz, hello, ""), npm(pubTok))
		expect(t, "重复发布同一版本", code, 409, body)

		code, body = anon.do("GET", "/npm/@acme%2fhello", nil, npm(readTok))
		expect(t, "待审核的版本不可见", code, 404, body)
		code, body = anon.do("GET", "/npm/@acme/hello/-/hello-1.0.0.tgz", nil, npm(readTok))
		expect(t, "待审核的 tarball 不可下载", code, 404, body)
		code, body = alice.do("GET", "/api/me/packages", nil, nil)
		expect(t, "我的发布", code, 200, body)
		if !strings.Contains(string(body), `"status":"pending"`) {
			t.Fatalf("我的发布里应有待审核版本：%s", body)
		}
	})

	t.Run("审核通过后可安装", func(t *testing.T) {
		code, body := alice.do("POST", "/api/admin/review", map[string]string{"name": "@acme/hello", "version": "1.0.0", "action": "approve"}, nil)
		expect(t, "员工不能审核", code, 403, body)
		code, body = admin.do("GET", "/api/admin/review", nil, nil)
		expect(t, "审核队列", code, 200, body)
		q := decodeAs[struct {
			Items []struct {
				Name      string `json:"name"`
				Version   string `json:"version"`
				Publisher string `json:"publisher"`
				Scan      []registry.ScanItem
			} `json:"items"`
		}](t, body)
		if len(q.Items) != 1 || q.Items[0].Name != "@acme/hello" || q.Items[0].Publisher != "alice" {
			t.Fatalf("审核队列不对：%s", body)
		}
		levels := map[string]string{}
		for _, it := range q.Items[0].Scan {
			levels[it.Key] = it.Level
		}
		if levels["bundle"] != "pass" || levels["secrets"] != "pass" || levels["install-scripts"] != "pass" {
			t.Fatalf("自动检查结果不对：%v", levels)
		}
		code, body = admin.do("POST", "/api/admin/review", map[string]string{"name": "@acme/hello", "version": "1.0.0", "action": "approve"}, nil)
		expect(t, "通过", code, 200, body)

		code, body = anon.do("GET", "/npm/@acme%2fhello", nil, npm(readTok))
		expect(t, "上架后的 packument", code, 200, body)
		p := decodeAs[packument](t, body)
		if p.DistTags["latest"] != "1.0.0" || p.Versions["1.0.0"].Dist.Tarball != ts.URL+"/npm/@acme/hello/-/hello-1.0.0.tgz" {
			t.Fatalf("packument 不对：%s", body)
		}
		code, body = anon.do("GET", "/npm/@acme/hello/-/hello-1.0.0.tgz", nil, npm(readTok))
		expect(t, "下载", code, 200, body)
		if !bytes.Equal(body, helloTgz) {
			t.Fatal("下载的 tarball 与发布的不一致")
		}
		code, body = alice.do("GET", "/api/catalog", nil, nil)
		expect(t, "能力市场", code, 200, body)
		cat := decodeAs[struct {
			Plugins []map[string]any `json:"plugins"`
		}](t, body)
		if len(cat.Plugins) != 1 || cat.Plugins[0]["title"] != "你好插件" || cat.Plugins[0]["origin"] != "internal" || cat.Plugins[0]["installs"].(float64) != 1 || cat.Plugins[0]["department"] != "研发效能部" {
			t.Fatalf("能力市场不对：%s", body)
		}
		code, body = alice.do("GET", "/api/catalog/"+strings.ReplaceAll("@acme/hello", "/", "%2F"), nil, nil)
		expect(t, "能力详情", code, 200, body)
	})

	t.Run("DSH 索引", func(t *testing.T) {
		code, body := anon.do("GET", "/index/dsh-index.json", nil, nil)
		expect(t, "索引不需要令牌", code, 200, body)
		idx := decodeAs[registry.Index](t, body)
		if idx.Format != 1 || len(idx.Plugins) != 1 || idx.Plugins[0].Name != "@acme/hello" || idx.Plugins[0].Versions[0].Source["type"] != "npm" {
			t.Fatalf("索引不对：%s", body)
		}
		if idx.Plugins[0].Metadata.Category != "tools" || idx.Plugins[0].Metadata.Labels[0] != "internal" || !strings.Contains(idx.Title["zh"], "ACME") {
			t.Fatalf("索引元数据不对：%s", body)
		}
	})

	t.Run("新版本待审核时 latest 不变；驳回后版本号不能重用", func(t *testing.T) {
		v2 := pluginPkg("@acme/hello", "1.1.0", nil)
		code, body := anon.do("PUT", "/npm/@acme%2fhello", publishBody("@acme/hello", "1.1.0", makeTgz(t, v2, bundleFile), v2, ""), npm(pubTok))
		expect(t, "发布 1.1.0", code, 201, body)
		code, body = anon.do("GET", "/npm/@acme%2fhello", nil, npm(readTok))
		expect(t, "packument", code, 200, body)
		if p := decodeAs[packument](t, body); p.DistTags["latest"] != "1.0.0" || len(p.Versions) != 1 {
			t.Fatalf("待审核的 1.1.0 不应出现：%s", body)
		}
		code, body = admin.do("POST", "/api/admin/review", map[string]string{"name": "@acme/hello", "version": "1.1.0", "action": "reject"}, nil)
		expect(t, "驳回要写原因", code, 400, body)
		code, body = admin.do("POST", "/api/admin/review", map[string]string{"name": "@acme/hello", "version": "1.1.0", "action": "reject", "note": "缺少说明"}, nil)
		expect(t, "驳回", code, 200, body)
		code, body = anon.do("PUT", "/npm/@acme%2fhello", publishBody("@acme/hello", "1.1.0", makeTgz(t, v2, bundleFile), v2, ""), npm(pubTok))
		expect(t, "重用被驳回的版本号", code, 409, body)
	})

	t.Run("下架与恢复", func(t *testing.T) {
		code, body := admin.do("POST", "/api/admin/review", map[string]string{"name": "@acme/hello", "action": "unpublish", "note": "安全问题"}, nil)
		expect(t, "下架整个包", code, 200, body)
		code, body = anon.do("GET", "/npm/@acme%2fhello", nil, npm(readTok))
		expect(t, "下架后不可见", code, 404, body)
		code, body = anon.do("GET", "/npm/@acme/hello/-/hello-1.0.0.tgz", nil, npm(readTok))
		expect(t, "下架后不可下载", code, 404, body)
		code, body = anon.do("GET", "/index/dsh-index.json", nil, nil)
		if idx := decodeAs[registry.Index](t, body); len(idx.Plugins) != 0 {
			t.Fatalf("下架后索引里不应有：%s", body)
		}
		code, body = admin.do("POST", "/api/admin/review", map[string]string{"name": "@acme/hello", "version": "1.0.0", "action": "restore"}, nil)
		expect(t, "恢复", code, 200, body)
		code, body = anon.do("GET", "/npm/@acme%2fhello", nil, npm(readTok))
		expect(t, "恢复后可见", code, 200, body)
	})

	t.Run("dist-tag、弃用与不支持的 unpublish", func(t *testing.T) {
		code, body := anon.do("PUT", "/npm/-/package/@acme%2fhello/dist-tags/stable", "1.0.0", npm(pubTok))
		expect(t, "设置标签", code, 200, body)
		code, body = anon.do("GET", "/npm/-/package/@acme%2fhello/dist-tags", nil, npm(readTok))
		expect(t, "读标签", code, 200, body)
		if tags := decodeAs[map[string]string](t, body); tags["stable"] != "1.0.0" || tags["latest"] != "1.0.0" {
			t.Fatalf("标签不对：%s", body)
		}
		code, body = anon.do("PUT", "/npm/@acme%2fhello", map[string]any{"name": "@acme/hello", "versions": map[string]any{"1.0.0": map[string]any{"deprecated": "请升级"}}}, npm(pubTok))
		expect(t, "deprecate", code, 200, body)
		code, body = anon.do("GET", "/npm/@acme%2fhello", nil, npm(readTok))
		if p := decodeAs[packument](t, body); p.Versions["1.0.0"].Deprecated != "请升级" {
			t.Fatalf("弃用说明没生效：%s", body)
		}
		code, body = anon.do("DELETE", "/npm/@acme%2fhello/-rev/1", nil, npm(pubTok))
		expect(t, "npm unpublish", code, 405, body)
		code, body = anon.do("GET", "/npm/-/whoami", nil, npm(readTok))
		expect(t, "whoami", code, 200, body)
		if !strings.Contains(string(body), "alice") {
			t.Fatalf("whoami：%s", body)
		}
	})

	t.Run("npm login", func(t *testing.T) {
		code, body := anon.do("PUT", "/npm/-/user/org.couchdb.user:alice", map[string]string{"name": "alice", "password": "wrong-password"}, nil)
		expect(t, "错误密码", code, 401, body)
		code, body = anon.do("PUT", "/npm/-/user/org.couchdb.user:alice", map[string]string{"name": "alice", "password": "alice-password-1"}, nil)
		expect(t, "npm login", code, 201, body)
		tok := decodeAs[map[string]any](t, body)["token"].(string)
		code, body = anon.do("GET", "/npm/@acme%2fhello", nil, npm(tok))
		expect(t, "用 login 拿到的令牌读", code, 200, body)
	})

	t.Run("上游预载插件与依赖闭包", func(t *testing.T) {
		code, body := admin.do("GET", "/api/admin/upstream/feed", nil, nil)
		expect(t, "dshfind 目录", code, 200, body)
		if !strings.Contains(string(body), "dsh-plugin-cool") {
			t.Fatalf("目录里应有 dsh-plugin-cool：%s", body)
		}
		code, body = admin.do("POST", "/api/admin/upstream/sync", map[string]any{"names": []string{"@acme/hello"}}, nil)
		expect(t, "不能预载企业作用域", code, 400, body)
		code, body = admin.do("POST", "/api/admin/upstream/sync", map[string]any{"names": []string{"dsh-plugin-cool"}}, nil)
		expect(t, "预载", code, 202, body)
		j := waitJob(t, admin, decodeAs[store.SyncJob](t, body).ID)
		if j.Status != "succeeded" {
			t.Fatalf("预载失败：%+v", j)
		}
		pkg, err := st.GetPackage(ctx, "dsh-plugin-cool")
		if err != nil || pkg.DepCount == nil || *pkg.DepCount != 2 || pkg.Title != "Cool 插件" || pkg.UpstreamScore == nil || *pkg.UpstreamScore != 88 {
			t.Fatalf("预载根的统计不对：%+v %v", pkg, err)
		}
		v, _ := st.GetVersion(ctx, "dsh-plugin-cool", "2.0.0")
		if v.Status != "pending" {
			t.Fatalf("预载的插件应进入待审核，得到 %s", v.Status)
		}
		if dv, err := st.GetVersion(ctx, "@deepseek-ai/dsh-core", "0.2.1"); err != nil || dv.Status != "published" {
			t.Fatalf("peer 依赖应镜像并上架：%v %v", dv, err)
		}
		code, body = admin.do("POST", "/api/admin/review", map[string]string{"name": "dsh-plugin-cool", "version": "2.0.0", "action": "approve"}, nil)
		expect(t, "通过上游插件", code, 200, body)
		code, body = anon.do("GET", "/npm/dsh-plugin-cool", nil, npm(readTok))
		expect(t, "审核后的上游插件", code, 200, body)
		code, body = anon.do("GET", "/index/dsh-index.json", nil, nil)
		if idx := decodeAs[registry.Index](t, body); len(idx.Plugins) != 2 {
			t.Fatalf("索引应有 2 个插件：%s", body)
		}

		// dshfind 目录落后于 npm 时，以上游 registry 的 latest 为准，旧的目录版本不算「有新版」
		up.mu.Lock()
		up.feeds[0]["versions"] = []map[string]any{{"version": "1.0.0", "publishTimestamp": 1780000000}}
		up.mu.Unlock()
		code, body = admin.do("GET", "/api/admin/upstream/feed?refresh=1", nil, nil)
		expect(t, "dshfind 目录", code, 200, body)
		type feedRow struct {
			Name           string `json:"name"`
			UpstreamLatest string `json:"upstream_latest"`
			LocalVersion   string `json:"local_version"`
			Newer          bool   `json:"newer"`
		}
		feed := decodeAs[struct{ Plugins []feedRow }](t, body)
		if len(feed.Plugins) != 1 || feed.Plugins[0].UpstreamLatest != "2.0.0" || feed.Plugins[0].LocalVersion != "2.0.0" || feed.Plugins[0].Newer {
			t.Fatalf("应按 npm latest 判断新版本：%s", body)
		}
	})

	t.Run("只用已预载的包", func(t *testing.T) {
		code, body := admin.do("GET", "/api/settings", nil, nil)
		s := decodeAs[map[string]any](t, body)
		s["registry"].(map[string]any)["proxyMode"] = "preloaded"
		code, body = admin.do("PUT", "/api/admin/settings", s, nil)
		expect(t, "切到只用已预载", code, 200, body)
		code, body = anon.do("GET", "/npm/is-number", nil, npm(readTok))
		expect(t, "没预载的包", code, 404, body)
		code, body = anon.do("GET", "/npm/left-pad", nil, npm(readTok))
		expect(t, "已镜像的依赖", code, 200, body)
		if p := decodeAs[packument](t, body); len(p.Versions) != 1 || p.Versions["1.3.0"].Version != "1.3.0" {
			t.Fatalf("只用已预载时只返回本地版本：%s", body)
		}
	})

	t.Run("离线包导出与导入", func(t *testing.T) {
		req, _ := http.NewRequest("GET", ts.URL+"/api/admin/upstream/export", nil)
		resp, err := admin.hc.Do(req)
		if err != nil || resp.StatusCode != 200 {
			t.Fatalf("导出：%v %d", err, resp.StatusCode)
		}
		bundle, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		st2 := openTestStore(t)
		if _, err := api.Bootstrap(ctx, st2, "admin", "admin-password-123"); err != nil {
			t.Fatal(err)
		}
		ts2 := newRegistryServer(t, st2, up)
		admin2 := newClient(t, ts2.URL)
		code, body := admin2.do("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "admin-password-123"}, nil)
		expect(t, "第二个实例登录", code, 200, body)
		code, body = admin2.do("POST", "/api/admin/upstream/import", nil, map[string]string{"Content-Type": "application/x-tar"})
		expect(t, "空请求体", code, 400, body)
		r2, _ := http.NewRequest("POST", ts2.URL+"/api/admin/upstream/import", bytes.NewReader(bundle))
		r2.Header.Set("Content-Type", "application/x-tar")
		resp, err = admin2.hc.Do(r2)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		expect(t, "导入", resp.StatusCode, 200, b)
		res := decodeAs[struct {
			Result registry.ImportResult `json:"result"`
		}](t, b)
		if res.Result.Added != 3 || res.Result.Pending != 1 {
			t.Fatalf("导入结果不对：%s", b)
		}
		if v, err := st2.GetVersion(ctx, "dsh-plugin-cool", "2.0.0"); err != nil || v.Status != "pending" {
			t.Fatalf("导入的插件应待审核：%v %v", v, err)
		}
		if v, err := st2.GetVersion(ctx, "left-pad", "1.3.0"); err != nil || v.Status != "published" {
			t.Fatalf("导入的依赖应直接可用：%v %v", v, err)
		}
	})

	t.Run("员工申请引入", func(t *testing.T) {
		up.add(t, pluginPkg("dsh-plugin-new", "0.1.0", nil), bundleFile)
		code, body := admin.do("GET", "/api/settings", nil, nil)
		s := decodeAs[map[string]any](t, body)
		s["registry"].(map[string]any)["proxyMode"] = "cache"
		code, body = admin.do("PUT", "/api/admin/settings", s, nil)
		expect(t, "切回按需回源", code, 200, body)
		code, body = alice.do("POST", "/api/me/import-requests", map[string]string{"name": "dsh-plugin-new", "reason": "团队要用"}, nil)
		expect(t, "申请引入", code, 202, body)
		j := waitJob(t, admin, decodeAs[store.SyncJob](t, body).ID)
		if j.Status != "succeeded" {
			t.Fatalf("申请引入失败：%+v", j)
		}
		code, body = admin.do("GET", "/api/admin/review", nil, nil)
		if !strings.Contains(string(body), "dsh-plugin-new") || !strings.Contains(string(body), `"publisher":"alice"`) {
			t.Fatalf("审核队列里应有 alice 申请的插件：%s", body)
		}
	})

	// 插件藏在依赖里：员工申请引入一个普通包，它依赖的社区插件不能跟着直接上架
	t.Run("依赖里的插件也要审核", func(t *testing.T) {
		up.add(t, pluginPkg("dsh-plugin-hidden", "1.0.0", map[string]any{"dependencies": map[string]any{"is-number": "^7.0.0"}}), bundleFile)
		up.add(t, map[string]any{"name": "carrier", "version": "1.0.0", "license": "MIT",
			"dependencies": map[string]any{"dsh-plugin-hidden": "^1.0.0", "left-pad": "^1.0.0"}}, nil)
		code, body := alice.do("POST", "/api/me/import-requests", map[string]string{"name": "carrier", "reason": "顺带一个插件"}, nil)
		expect(t, "申请引入", code, 202, body)
		if j := waitJob(t, admin, decodeAs[store.SyncJob](t, body).ID); j.Status != "succeeded" {
			t.Fatalf("申请引入失败：%+v", j)
		}
		if v, err := st.GetVersion(ctx, "dsh-plugin-hidden", "1.0.0"); err != nil || v.Status != store.StatusPending {
			t.Fatalf("依赖里的插件应待审核：%+v %v", v, err)
		}
		// 插件自己的普通依赖照常镜像，审核通过后才装得上
		if v, err := st.GetVersion(ctx, "is-number", "7.0.0"); err != nil || v.Status != store.StatusPublished {
			t.Fatalf("插件的普通依赖应直接可用：%+v %v", v, err)
		}
		code, body = anon.do("GET", "/index/dsh-index.json", nil, nil)
		expect(t, "插件索引", code, 200, body)
		if strings.Contains(string(body), "dsh-plugin-hidden") {
			t.Fatalf("没审核的插件不能出现在插件索引里：%s", body)
		}
		code, body = admin.do("GET", "/api/admin/review", nil, nil)
		if !strings.Contains(string(body), "dsh-plugin-hidden") {
			t.Fatalf("审核队列里应有依赖带进来的插件：%s", body)
		}
	})

	t.Run("整包恢复不带出被驳回和没审过的版本", func(t *testing.T) {
		review := func(what, version, action, note string) {
			t.Helper()
			code, body := admin.do("POST", "/api/admin/review", map[string]string{"name": "@acme/yank", "version": version, "action": action, "note": note}, nil)
			expect(t, what, code, 200, body)
		}
		for _, v := range []string{"1.0.0", "1.1.0", "1.2.0"} {
			pkg := pluginPkg("@acme/yank", v, nil)
			code, body := anon.do("PUT", "/npm/@acme%2fyank", publishBody("@acme/yank", v, makeTgz(t, pkg, bundleFile), pkg, ""), npm(pubTok))
			expect(t, "发布 "+v, code, 201, body)
		}
		review("通过 1.0.0", "1.0.0", "approve", "")
		review("驳回 1.1.0", "1.1.0", "reject", "有安装脚本")
		// 1.2.0 还没审。整包下架再整包恢复：只有上过架的 1.0.0 回来
		review("下架整个包", "", "unpublish", "先停用")
		review("恢复整个包", "", "restore", "")
		code, body := anon.do("GET", "/npm/@acme%2fyank", nil, npm(readTok))
		expect(t, "恢复后的 packument", code, 200, body)
		if p := decodeAs[packument](t, body); len(p.Versions) != 1 || p.DistTags["latest"] != "1.0.0" {
			t.Fatalf("整包恢复后只应有 1.0.0：versions=%d latest=%s", len(p.Versions), p.DistTags["latest"])
		}
		code, body = admin.do("POST", "/api/admin/review", map[string]string{"name": "@acme/yank", "action": "restore"}, nil)
		expect(t, "没有上过架又待恢复的版本了", code, 409, body)
		// 没上过架的版本要逐个恢复，那是对这一个版本的决定
		review("单独恢复 1.2.0", "1.2.0", "restore", "")
		code, body = anon.do("GET", "/npm/@acme%2fyank", nil, npm(readTok))
		if p := decodeAs[packument](t, body); len(p.Versions) != 2 {
			t.Fatalf("单独恢复的 1.2.0 应上架：%s", body)
		}
	})
}
