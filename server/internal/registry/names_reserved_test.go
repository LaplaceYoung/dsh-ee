package registry

import (
	"strings"
	"testing"
)

// TestNamesReserved 守住 npm 的保留名：node_modules、favicon.ico 在 npm 侧
// （validate-npm-package-name 的 exclusionList，以及 npm publish 实际走的
// @npmcli/package-json fixName）是硬拒绝，大小写不敏感，且只比较整名。
//
// 发布端与读取端必须一致拒绝，否则又会是「能发布但读不到」的一类不一致。
func TestNamesReserved(t *testing.T) {
	// 保留名（大小写不敏感）两侧都必须拒绝
	for _, name := range []string{
		"node_modules", "NODE_MODULES", "Node_Modules", "nOdE_mOdUlEs",
		"favicon.ico", "FAVICON.ICO", "Favicon.Ico", "favicon.ICO",
	} {
		if !isReservedName(name) {
			t.Errorf("isReservedName(%q) = false，应为 npm 保留名", name)
		}
		if ValidPublishName(name) {
			t.Errorf("ValidPublishName(%q) = true，npm 保留名不能发布", name)
		}
		if ValidName(name) {
			t.Errorf("ValidName(%q) = true，npm 保留名读端也必须拒绝", name)
		}
	}

	// 只比较整名：近似名不是保留名；npm 只限制无作用域的保留名，scoped 包允许。
	for _, name := range []string{
		"node-modules", "node_modules2", "node_modules.js", "my_node_modules", "node_modules-extra",
		"favicon", "favicon.ico.png", "favicon_ico",
		"@acme/node_modules", "@acme/favicon.ico", "@a1/node_modules",
		"left-pad", "@acme/x",
	} {
		if isReservedName(name) {
			t.Errorf("isReservedName(%q) = true，保留名只按整名比较", name)
		}
		if !ValidPublishName(name) {
			t.Errorf("ValidPublishName(%q) = false，保留名的近似名/scoped 名应可发布", name)
		}
		if !ValidName(name) {
			t.Errorf("ValidName(%q) = false，保留名的近似名/scoped 名应可读取", name)
		}
	}

	// 发布端的大写收紧不因保留名判断而改变：读端仍接受历史写法的 scoped 名，
	// 发布端因「新包全小写」拒绝它（与保留名无关）。
	if !ValidName("@Acme/Node_Modules") {
		t.Error("ValidName(@Acme/Node_Modules) 应接受（历史包名允许大写）")
	}
	if ValidPublishName("@Acme/Node_Modules") {
		t.Error("ValidPublishName(@Acme/Node_Modules) 应拒绝（新包要求全小写）")
	}
}

// TestPublishNameReservedConsistent 是保留名的小规模性质测试（不重复全量穷举）：
// 发布端接受的名字读端必须也接受，且保留名在两侧的结论完全一致。
func TestPublishNameReservedConsistent(t *testing.T) {
	cases := []string{"node_modules", "NODE_MODULES", "node_modules2", "my-node_modules"}
	for _, base := range []string{"node_modules", "favicon.ico"} {
		for _, name := range []string{base, strings.ToUpper(base), strings.ToLower(base)} {
			cases = append(cases, name)
		}
		for _, prefix := range []string{"", "x", "node_", "favicon."} {
			for _, suffix := range []string{"", "2", "-x", ".js", "_x"} {
				cases = append(cases, prefix+base+suffix)
			}
		}
		for _, scope := range []string{"@acme/", "@a1/", "@Acme/"} {
			cases = append(cases, scope+base)
		}
	}
	for _, name := range cases {
		publishable, readable := ValidPublishName(name), ValidName(name)
		if publishable && !readable {
			t.Errorf("能发布但读不到：ValidPublishName(%q)=true 而 ValidName(%q)=false", name, name)
		}
		if isReservedName(name) && (publishable || readable) {
			t.Errorf("保留名两侧都必须拒绝：ValidPublishName(%q)=%v，ValidName(%q)=%v",
				name, publishable, name, readable)
		}
	}
}
