package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/api"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// npmDistTagFixture 是一个只有 @acme 作用域、发布即上架（admissionMode=open）、
// 已经发布过 @acme/hello@1.0.0 的 npm 源。
type npmDistTagFixture struct {
	st      *store.Store
	ts      string
	anon    *client
	pubTok  string
	readTok string
}

func newNpmDistTagFixture(t *testing.T) *npmDistTagFixture {
	t.Helper()
	st := openTestStore(t)
	ctx := context.Background()
	if _, err := api.Bootstrap(ctx, st, "admin", "admin-password-123"); err != nil {
		t.Fatal(err)
	}
	up := newFakeRegistry(t)
	ts := newRegistryServer(t, st, up)
	admin := newClient(t, ts.URL)
	code, body := admin.do("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "admin-password-123"}, nil)
	expect(t, "管理员登录", code, 200, body)

	// 只用企业作用域，上游指到假 registry（托管包不会回源）；发布即上架，
	// 这样新版本的 latest 会直接出现在 packument 里，能通过 npm 的 dist-tags 接口读回。
	code, body = admin.do("GET", "/api/settings", nil, nil)
	expect(t, "读设置", code, 200, body)
	set := decodeAs[map[string]any](t, body)
	set["scope"] = "@acme"
	set["orgName"] = "ACME"
	set["admissionMode"] = "open"
	set["upstream"].(map[string]any)["registry"] = "custom"
	set["upstream"].(map[string]any)["customRegistry"] = up.URL
	code, body = admin.do("PUT", "/api/admin/settings", set, nil)
	expect(t, "保存设置", code, 200, body)

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

	anon := newClient(t, ts.URL)
	code, body = anon.do("PUT", "/npm/@acme%2fhello", publishBody("@acme/hello", "1.0.0", npmHelloTgz(t, "1.0.0"), pluginPkg("@acme/hello", "1.0.0", nil), ""), npmAuth(pubTok))
	expect(t, "发布 1.0.0", code, 201, body)

	code, body = anon.do("GET", "/npm/-/package/@acme%2fhello/dist-tags", nil, npmAuth(readTok))
	expect(t, "初读标签", code, 200, body)
	if tags := decodeAs[map[string]string](t, body); tags["latest"] != "1.0.0" {
		t.Fatalf("初读标签不对：%s", body)
	}
	return &npmDistTagFixture{st: st, ts: ts.URL, anon: anon, pubTok: pubTok, readTok: readTok}
}

func npmHelloTgz(t *testing.T, version string) []byte {
	t.Helper()
	return makeTgz(t, pluginPkg("@acme/hello", version, nil), bundleFile)
}

func npmAuth(tok string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + tok}
}

// rawNpmRequest 在 goroutine 里发 npm 请求：不用 client.do（它失败时会调 t.Fatalf，
// 而 t.Fatalf 只能从测试自己的 goroutine 调）。
func rawNpmRequest(base, method, path string, body any, token string) (int, []byte) {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, base+path, rd)
	if err != nil {
		return 0, []byte(err.Error())
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, []byte(err.Error())
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, data
}

// waitLockWaiting 等这个测试库里出现 n 个「在等行锁」的事务：两个并发操作都已经
// 卡在 npm_packages 那一行的锁上，再放行测试自己持有的锁，交错顺序就是确定的，
// 不依赖毫秒级的调度竞态。
//
// TiDB 直接用 information_schema.tidb_trx 读状态；其它 MySQL 协议数据库没有这张表，
// 退化成等一小会儿（两个请求都发出去之后仍在行锁上，顺序同样由锁队列决定）。
func waitLockWaiting(t *testing.T, st *store.Store, n int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		var waiting int
		err := st.DB().QueryRowContext(context.Background(),
			"SELECT COUNT(*) FROM information_schema.tidb_trx WHERE STATE = 'LockWaiting' AND DB = DATABASE()").Scan(&waiting)
		if err != nil {
			time.Sleep(500 * time.Millisecond)
			return
		}
		if waiting >= n {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("等 %d 个事务卡在 npm_packages 行锁上超时（当前 %d 个）", n, waiting)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// 发布 2.0.0 与「设置 stable=1.0.0」并发：发布事务在 npm_packages 行上持锁时，
// 设标签必须等它提交后再基于新 map 写回。修前 SetDistTag 是先读旧 map 再整表 UPDATE，
// 会卡在发布事务的后面、用旧 map 把刚发布的 latest=2.0.0 覆盖回 1.0.0。
func TestNpmDistTagConcurrentPublish(t *testing.T) {
	f := newNpmDistTagFixture(t)
	ctx := context.Background()
	const name = "@acme/hello"

	// 测试自己持住包行：发布请求会卡在 PublishHosted 的 SELECT ... FOR UPDATE 上排队。
	tx, err := f.st.DB().BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	var raw string
	if err := tx.QueryRowContext(ctx, "SELECT dist_tags FROM npm_packages WHERE name = ? FOR UPDATE", name).Scan(&raw); err != nil {
		t.Fatalf("锁住包行：%v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback() })

	v2 := pluginPkg(name, "2.0.0", nil)
	v2Body := publishBody(name, "2.0.0", npmHelloTgz(t, "2.0.0"), v2, "")
	pubCh := make(chan struct {
		code int
		body []byte
	}, 1)
	go func() {
		code, body := rawNpmRequest(f.ts, "PUT", "/npm/@acme%2fhello", v2Body, f.pubTok)
		pubCh <- struct {
			code int
			body []byte
		}{code, body}
	}()
	waitLockWaiting(t, f.st, 1)

	tagCh := make(chan struct {
		code int
		body []byte
	}, 1)
	go func() {
		code, body := rawNpmRequest(f.ts, "PUT", "/npm/-/package/@acme%2fhello/dist-tags/stable", "1.0.0", f.pubTok)
		tagCh <- struct {
			code int
			body []byte
		}{code, body}
	}()
	waitLockWaiting(t, f.st, 2)

	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	pub, tag := <-pubCh, <-tagCh
	expect(t, "并发发布 2.0.0", pub.code, 201, pub.body)
	expect(t, "并发设置 stable 标签", tag.code, 200, tag.body)

	// 最终状态：两个操作都成功，最新版本与用户标签都必须还在。
	code, body := f.anon.do("GET", "/npm/-/package/@acme%2fhello/dist-tags", nil, npmAuth(f.readTok))
	expect(t, "读标签", code, 200, body)
	tags := decodeAs[map[string]string](t, body)
	if tags["latest"] != "2.0.0" || tags["stable"] != "1.0.0" {
		t.Fatalf("并发发布与设标签互相覆盖：latest=%q stable=%q（%s）", tags["latest"], tags["stable"], body)
	}
	pkg, err := f.st.GetPackage(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	if pkg.DistTags["latest"] != "2.0.0" || pkg.DistTags["stable"] != "1.0.0" {
		t.Fatalf("库里的 dist-tags 被覆盖：%v", pkg.DistTags)
	}
}

// 单线程的 set / delete / 校验行为保持 npm 语义不变。
func TestNpmDistTagSetDeleteAndValidation(t *testing.T) {
	f := newNpmDistTagFixture(t)
	read := npmAuth(f.readTok)
	pub := npmAuth(f.pubTok)

	code, body := f.anon.do("PUT", "/npm/-/package/@acme%2fhello/dist-tags/stable", "1.0.0", pub)
	expect(t, "设置 stable", code, 200, body)
	if tags := decodeAs[map[string]string](t, body); tags["stable"] != "1.0.0" || tags["latest"] != "1.0.0" {
		t.Fatalf("设置后返回的标签不对：%s", body)
	}
	code, body = f.anon.do("GET", "/npm/-/package/@acme%2fhello/dist-tags", nil, read)
	expect(t, "读标签", code, 200, body)
	if tags := decodeAs[map[string]string](t, body); tags["stable"] != "1.0.0" || tags["latest"] != "1.0.0" {
		t.Fatalf("读回的标签不对：%s", body)
	}

	// --delete：删掉指定标签，latest 与其它标签不动
	code, body = f.anon.do("DELETE", "/npm/-/package/@acme%2fhello/dist-tags/stable", nil, pub)
	expect(t, "删除 stable", code, 200, body)
	if tags := decodeAs[map[string]string](t, body); tags["stable"] != "" || tags["latest"] != "1.0.0" {
		t.Fatalf("删除后返回的标签不对：%s", body)
	}
	code, body = f.anon.do("GET", "/npm/-/package/@acme%2fhello/dist-tags", nil, read)
	expect(t, "读标签", code, 200, body)
	if tags := decodeAs[map[string]string](t, body); tags["stable"] != "" {
		t.Fatalf("stable 应已被删除：%s", body)
	}

	// latest 不能删
	code, body = f.anon.do("DELETE", "/npm/-/package/@acme%2fhello/dist-tags/latest", nil, pub)
	expect(t, "删除 latest", code, 400, body)

	// 指向不存在的版本：404，且不落库
	code, body = f.anon.do("PUT", "/npm/-/package/@acme%2fhello/dist-tags/beta", "9.9.9", pub)
	expect(t, "指向不存在的版本", code, 404, body)
	code, body = f.anon.do("GET", "/npm/-/package/@acme%2fhello/dist-tags", nil, read)
	expect(t, "读标签", code, 200, body)
	if tags := decodeAs[map[string]string](t, body); tags["beta"] != "" {
		t.Fatalf("校验失败的标签不应落库：%s", body)
	}

	// 非法标签名
	code, body = f.anon.do("PUT", "/npm/-/package/@acme%2fhello/dist-tags/1.0.0", "1.0.0", pub)
	expect(t, "非法标签名", code, 400, body)
}
