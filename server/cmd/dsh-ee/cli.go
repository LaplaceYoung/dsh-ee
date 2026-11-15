package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/hikariming/dsh-ee/server/internal/extension"
)

// 不启动服务、不连数据库的子命令：写扩展时用的检查和规格（docs/plugins.md §10、docs/agent-dev.md §3～§4）。

// runCLI 处理子命令。不是子命令时 handled 为 false，由调用方启动服务。
func runCLI(args []string, stdout, stderr io.Writer) (handled bool, code int) {
	// 全局 --lang 在这里统一摘掉并解析一次（可以出现在任意子命令位置），子命令不用认识它。
	args, err := takeCLILang(args)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return true, 2
	}
	if len(args) == 0 {
		return false, 0
	}
	switch args[0] {
	case "version", "--version":
		fmt.Fprintln(stdout, version)
		return true, 0
	case "help", "-h", "--help":
		fmt.Fprint(stdout, cliUsage)
		return true, 0
	case "extension":
		return true, cliExtension(args[1:], stdout, stderr)
	case "directory":
		return true, cliDirectory(args[1:], stdout, stderr)
	case "schema":
		return true, cliSchema(args[1:], stdout, stderr)
	case "config":
		return true, cliConfig(args[1:], stdout, stderr)
	case "package":
		return true, cliPackage(args[1:], stdout, stderr)
	case "doctor":
		return true, cliDoctor(args[1:], stdout, stderr)
	case "admin":
		return true, cliAdmin(args[1:], stdout, stderr)
	}
	return false, 0
}

// flags 把参数分成普通参数和 --json 开关；不认识的开关返回错误。
func flags(args []string) (rest []string, asJSON bool, err error) {
	for _, a := range args {
		switch {
		case a == "--json":
			asJSON = true
		case strings.HasPrefix(a, "-"):
			return nil, false, fmt.Errorf(tr("不认识的参数 %s", "unknown option %s"), a)
		default:
			rest = append(rest, a)
		}
	}
	return rest, asJSON, nil
}

func cliExtension(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		if cmd, ok := extCommands[args[0]]; ok {
			return cmd(args[1:], stdout, stderr)
		}
	}
	if len(args) == 0 || args[0] != "check" {
		fmt.Fprint(stderr, cliUsage)
		return 2
	}
	if slices.ContainsFunc(args[1:], func(a string) bool { return a == "-h" || a == "--help" || a == "help" }) {
		fmt.Fprint(stdout, cliUsage)
		return 0
	}
	rest, asJSON, err := flags(args[1:])
	if err != nil || len(rest) != 1 {
		if err != nil {
			fmt.Fprintln(stderr, err)
		}
		fmt.Fprintln(stderr, tr("用法：dsh-ee extension check <目录> [--json]", "Usage: dsh-ee extension check <dir> [--json]"))
		return 2
	}
	dir := rest[0]
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		fmt.Fprintf(stderr, tr("%s 不是一个目录\n", "%s is not a directory\n"), dir)
		return 2
	}
	res := extension.Check(os.DirFS(dir))
	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		enc.SetEscapeHTML(false)
		_ = enc.Encode(res)
	} else {
		printCheck(stdout, res)
	}
	if !res.OK {
		return 1
	}
	return 0
}

// severityLabel 把检查结果的严重级别翻成当前语言的词（默认中文，既有输出不变）。
func severityLabel(severity string) string {
	switch severity {
	case extension.SevError:
		return tr("错误", "error")
	case extension.SevWarning:
		return tr("提醒", "warning")
	case extension.SevInfo:
		return tr("说明", "info")
	}
	return severity
}

func printCheck(w io.Writer, res *extension.Result) {
	if res.Identity != "" {
		fmt.Fprintf(w, tr("%s（%s）\n", "%s (%s)\n"), res.Identity, res.Kind)
	}
	if len(res.Servers) > 0 {
		fmt.Fprintf(w, tr("请求只会发往：%s\n", "requests go only to: %s\n"), joinText(res.Servers))
	}
	if cr := res.Credential; cr != nil {
		line := map[string]string{
			extension.CredShared:     tr("管理员配的一份共享凭据", "one shared credential configured by an admin"),
			extension.CredUserSecret: tr("员工自己填的凭据，放在 ", "a credential each employee fills in, placed in ") + cr.Header,
			extension.CredEEJWT:      tr("dsh-ee 为当前员工现签的身份令牌（ee_jwt），放在 ", "an identity token minted by dsh-ee for the current employee (ee_jwt), placed in ") + cr.Header,
			extension.CredOAuth:      "oauth",
		}[cr.Mode]
		if cr.Mode == extension.CredEEJWT {
			if cr.Audience != "" {
				line += tr("，aud 是 ", "; aud is ") + cr.Audience
			}
			if len(cr.Claims) > 0 {
				line += tr("，额外带 ", "; extra claims: ") + joinText(cr.Claims)
			}
		}
		if cr.SharedHeader != "" {
			line += tr("；另带一份共享凭据，放在 ", "; a shared credential is also sent in ") + cr.SharedHeader
		}
		fmt.Fprintf(w, tr("调用时的身份：%s\n", "identity on calls: %s\n"), line)
	} else if res.Manifest != nil {
		fmt.Fprintln(w, tr("调用时的身份：不带凭据", "identity on calls: no credential"))
	}
	if len(res.Tools) > 0 {
		fmt.Fprintln(w, tr("工具：", "Tools:"))
		for _, t := range res.Tools {
			kind := tr("写操作，调用前要员工确认", "write operation; the employee must confirm before each call")
			if t.ReadOnly {
				kind = tr("只读", "read-only")
			}
			line := "  " + t.Name
			if t.Method != "" {
				line += fmt.Sprintf("  %s %s", t.Method, t.Path)
			}
			fmt.Fprintf(w, tr("%s  （%s）\n", "%s  (%s)\n"), line, kind)
			who := map[string]string{extension.FillApp: tr("搭应用的人固定", "fixed by the app builder"), extension.FillAdmin: tr("管理员统一设置", "set centrally by the admin")}
			for _, p := range t.Fixed {
				extra := ""
				if len(p.Default) > 0 && string(p.Default) != "null" {
					extra = tr("，预先填 ", ", pre-filled with ") + string(p.Default)
				}
				fmt.Fprintf(w, tr("      %s：%s，模型看不到%s\n", "      %s: %s; not visible to the model%s\n"), p.Name, who[p.Fill], extra)
			}
			if len(t.Keep) > 0 {
				fmt.Fprintf(w, tr("      返回只保留：%s\n", "      returns only: %s\n"), joinText(t.Keep))
			}
		}
	}
	if len(res.Results) > 0 {
		fmt.Fprintln(w)
	}
	for _, d := range res.Results {
		fmt.Fprintf(w, "%s  %s  %s\n      %s\n", severityLabel(d.Severity), d.Code, d.Where, d.Message)
		if d.Fix != "" {
			fmt.Fprintf(w, tr("      改法：%s\n", "      fix: %s\n"), d.Fix)
		}
	}
	fmt.Fprintln(w)
	if res.OK {
		fmt.Fprintf(w, tr("通过：0 个错误，%d 个提醒\n", "passed: 0 errors, %d warnings\n"), res.Warnings)
		if res.Digest != "" {
			fmt.Fprintf(w, tr("sha256：%s\n", "sha256: %s\n"), res.Digest)
		}
	} else {
		fmt.Fprintf(w, tr("没通过：%d 个错误，%d 个提醒\n", "failed: %d errors, %d warnings\n"), res.Errors, res.Warnings)
	}
}

func cliSchema(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, cliUsage)
		return 2
	}
	rest, asJSON, err := flags(args[1:])
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	switch args[0] {
	case "list":
		if len(rest) != 0 {
			fmt.Fprintln(stderr, tr("用法：dsh-ee schema list [--json]", "Usage: dsh-ee schema list [--json]"))
			return 2
		}
		list := extension.Schemas()
		if asJSON {
			enc := json.NewEncoder(stdout)
			enc.SetIndent("", "  ")
			enc.SetEscapeHTML(false)
			_ = enc.Encode(list)
			return 0
		}
		for _, s := range list {
			fmt.Fprintf(stdout, "%-10s %s\n", s.Name, s.Title)
		}
		return 0
	case "get":
		if len(rest) != 1 {
			fmt.Fprintln(stderr, tr("用法：dsh-ee schema get <名字>", "Usage: dsh-ee schema get <name>"))
			return 2
		}
		raw, ok := extension.Schema(rest[0])
		if !ok {
			fmt.Fprintf(stderr, tr("没有名为 %s 的规格；用 dsh-ee schema list 看有哪些\n", "no schema named %s; run dsh-ee schema list to see what is available\n"), rest[0])
			return 1
		}
		_, _ = stdout.Write(raw)
		return 0
	}
	fmt.Fprint(stderr, cliUsage)
	return 2
}
