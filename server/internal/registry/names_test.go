package registry

import (
	"strings"
	"testing"
)

// TestNameRulesConsistent 守住「能发布」与「能读取/审核」不是两套互相矛盾的规则。
//
// 历史 bug：ValidPublishName 允许连续点（发布端 regex 里有 "."），ValidName 又显式
// 拒绝 ".." ⇒ 网页上传 @company/a..b 能入库并占用版本号，之后 npm GET 与网页审核
// 一律 400，包永远 pending、既不可读也不可审。
//
// 规则：发布端只允许比读端更严（新包全小写、不能以下划线开头），绝不能更松；
// 连续点在两侧都拒绝。
func TestNameRulesConsistent(t *testing.T) {
	type nameCase struct {
		name     string
		readable bool // ValidName（npm 读、审核、上游同步、索引读端）是否接受
		// publishOnlyReject 非空表示读端接受、发布端有意更严。必须写明原因；
		// 除此之外两个函数对同一个名字的结论必须一致。
		publishOnlyReject string
	}
	cases := []nameCase{
		// 正常包名：读写都接受
		{name: "left-pad", readable: true},
		{name: "@company/hello", readable: true},
		{name: "@company/x.y_z~1", readable: true},
		{name: "a.b", readable: true},
		{name: "@company/a.b.c", readable: true},
		{name: "a.", readable: true},
		{name: strings.Repeat("a", maxNameLength), readable: true},

		// 发布端有意收紧（npm 新包规则；历史包名如 JSONStream 仍可读）
		{name: "JSONStream", readable: true, publishOnlyReject: "npm 历史包名允许大写，新发布要求全小写（与 DSH 索引读端 PACKAGE_NAME 一致）"},
		{name: "@Company/hello", readable: true, publishOnlyReject: "作用域同样要求全小写"},
		{name: "_legacy", readable: true, publishOnlyReject: "npm 历史包名允许前导下划线，新发布不允许"},
		{name: "@company/_legacy", readable: true, publishOnlyReject: "同上（作用域内也同样收紧）"},

		// 连续点：读写都拒绝（本次修复的核心）
		{name: "a..b", readable: false},
		{name: "@company/a..b", readable: false},
		{name: "a...b", readable: false},
		{name: "@company/a...b", readable: false},
		{name: "a..", readable: false},
		{name: "..a", readable: false},
		{name: "@company/..a", readable: false},
		{name: "@company/a..b..c", readable: false},
		{name: "..", readable: false},

		// 两侧都不接受的名字（字符集、作用域、长度、保留位置）
		{name: "", readable: false},
		{name: ".", readable: false},
		{name: ".hidden", readable: false},
		{name: "~tilde", readable: false},
		{name: "-dash", readable: false},
		{name: "a b", readable: false},
		{name: "a/b", readable: false},
		{name: "a/../b", readable: false},
		{name: "../x", readable: false},
		{name: "@company", readable: false},
		{name: "@/x", readable: false},
		{name: "@company/", readable: false},
		{name: "@company/a/b", readable: false},
		{name: "包名", readable: false},
		{name: strings.Repeat("a", maxNameLength+1), readable: false},
	}

	for _, tc := range cases {
		readable := ValidName(tc.name)
		if readable != tc.readable {
			t.Errorf("ValidName(%q) = %v，期望 %v", tc.name, readable, tc.readable)
		}
		wantPublish := tc.readable && tc.publishOnlyReject == ""
		publishable := ValidPublishName(tc.name)
		if publishable != wantPublish {
			t.Errorf("ValidPublishName(%q) = %v，期望 %v（读端接受=%v，发布端收紧原因=%q）",
				tc.name, publishable, wantPublish, tc.readable, tc.publishOnlyReject)
		}
		// 核心不变量：发布端绝不允许比读端更松，否则就是「能发布但读不到」。
		if publishable && !readable {
			t.Errorf("能发布但读不到：ValidPublishName(%q)=true 而 ValidName(%q)=false", tc.name, tc.name)
		}
	}
}

// TestPublishNameNeverLooserThanReadName 用穷举短包名把上面的不变量做成性质测试：
// 任何发布端接受的名字，读端必须也接受；任何含连续点的名字两端口径都必须拒绝。
// 这样以后再改其中一侧的正则，就不会又悄悄漂移出第三套规则。
func TestPublishNameNeverLooserThanReadName(t *testing.T) {
	const alphabet = ".a@/-_~A0"
	var walk func(prefix string, depth int)
	walk = func(prefix string, depth int) {
		if prefix != "" {
			publishable, readable := ValidPublishName(prefix), ValidName(prefix)
			if publishable && !readable {
				t.Fatalf("能发布但读不到：ValidPublishName(%q)=true 而 ValidName(%q)=false", prefix, prefix)
			}
			if strings.Contains(prefix, "..") && (publishable || readable) {
				t.Fatalf("连续点必须两侧都拒绝：ValidPublishName(%q)=%v，ValidName(%q)=%v",
					prefix, publishable, prefix, readable)
			}
		}
		if depth == 0 {
			return
		}
		for i := 0; i < len(alphabet); i++ {
			walk(prefix+string(alphabet[i]), depth-1)
		}
	}
	walk("", 5)
}
