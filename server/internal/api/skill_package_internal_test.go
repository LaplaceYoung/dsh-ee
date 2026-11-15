package api

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

func zipOf(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(body))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

const skillMD = "---\nname: Invoice Check\ndescription: 核对发票\nwhen-to-use: 员工问发票能不能报销时\ndisable-model-invocation: true\n---\n# 步骤\n\n先看抬头。\n"

func TestParseSkillPackage(t *testing.T) {
	d, msg := parseSkillPackage(zipOf(t, map[string]string{
		"invoice/SKILL.md":         skillMD,
		"invoice/rules/limits.csv": "城市,标准\n北京,500\n",
		"invoice/logo.png":         "\x89PNG\x00\x00",
		"__MACOSX/invoice/._x":     "junk",
	}))
	if msg != "" {
		t.Fatal(msg)
	}
	if d.Slug != "invoice-check" || d.Spec.Title != "Invoice Check" || d.Spec.Description != "核对发票" || d.Spec.WhenToUse != "员工问发票能不能报销时" {
		t.Fatalf("frontmatter 没读对：%+v", d)
	}
	if d.Spec.ModelInvocable == nil || *d.Spec.ModelInvocable {
		t.Fatal("disable-model-invocation: true 应变成只能由人调用")
	}
	if !strings.Contains(d.Spec.Body, "先看抬头") || !strings.Contains(d.Spec.Body, "## 附件：rules/limits.csv") || !strings.Contains(d.Spec.Body, "北京,500") {
		t.Fatalf("正文应带上文本附件：%s", d.Spec.Body)
	}
	if len(d.Skipped) != 1 || d.Skipped[0].Path != "logo.png" || d.Skipped[0].Reason != "binary" {
		t.Fatalf("二进制附件应跳过：%+v", d.Skipped)
	}
}

func TestParseSkillPackageRejects(t *testing.T) {
	cases := map[string]struct {
		files map[string]string
		want  string
	}{
		"没有 SKILL.md": {map[string]string{"README.md": "x"}, "没有 SKILL.md"},
		"带 .env":      {map[string]string{"SKILL.md": skillMD, ".env": "A=1"}, "疑似有密钥"},
		"正文里有私钥":      {map[string]string{"SKILL.md": skillMD + "\n-----BEGIN RSA PRIVATE KEY-----\n"}, "疑似有密钥"},
		"YAML 坏了":     {map[string]string{"SKILL.md": "---\nname: [\n---\nbody"}, "不是合法的 YAML"},
	}
	for name, c := range cases {
		if _, msg := parseSkillPackage(zipOf(t, c.files)); !strings.Contains(msg, c.want) {
			t.Errorf("%s：应报「%s」，实际「%s」", name, c.want, msg)
		}
	}
	if _, msg := parseSkillPackage([]byte("not a zip")); msg != "不是有效的 zip 文件" {
		t.Errorf("非 zip：%s", msg)
	}
	// 没有 frontmatter：整篇当正文，标识留给员工填
	d, msg := parseSkillPackage(zipOf(t, map[string]string{"SKILL.md": "只有正文"}))
	if msg != "" || d.Slug != "" || d.Spec.Body != "只有正文" {
		t.Errorf("没有 frontmatter：%+v %s", d, msg)
	}
}
