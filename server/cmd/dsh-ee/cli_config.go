package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/hikariming/dsh-ee/server/internal/api"
	"github.com/hikariming/dsh-ee/server/internal/config"
)

// dsh-ee config …：权限配置即代码（docs/agent-dev.md §6.2）。命令只是 /api/admin/config、/api/admin/access 的客户端，
// 判断和试算都在服务端做；apply 只是提交变更请求，批准永远在网页上由人做。

const configUsage = `用法：dsh-ee config <命令> [参数]

  export [--json]                         导出现在的权限配置（角色、管理约束、字段、规则）
  plan <文件>                              看这份配置生效后会变什么、影响谁（不生效）
  test [<文件>] --tests <测试文件>          跑访问测试；不给配置文件就用现在的配置
  apply <文件> [--note 说明]                提交成变更请求，等管理员在网页上批准
  changes（或 status）                     最近的变更请求和它们的状态
  explain --user 用户 --doc 文档id [--config 文件]
                                          某人能不能看某篇文档、经过了哪些判断
  simulate --users a,b (--kb 知识库 | --docs 1,2) [--config 文件]
                                          一张「谁能看什么」的表

连接参数（也可以用环境变量）：
  --server 地址   或 DSH_EE_URL     dsh-ee 的地址，例如 https://ee.acme.com
  --token 令牌    或 DSH_EE_TOKEN   带 config 范围的令牌（网页「我的令牌」里建）

所有命令都可以加 --json 输出原始结果。plan 有问题、test 有不通过的用例时退出码为 1。
`

type cliOpts struct {
	pos    []string
	vals   map[string]string
	asJSON bool
}

// parseOpts 把 --键 值 / --键=值 / --json 分出来。
func parseOpts(args []string, valued ...string) (*cliOpts, error) {
	o := &cliOpts{vals: map[string]string{}}
	takes := map[string]bool{}
	for _, k := range valued {
		takes[k] = true
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--json" {
			o.asJSON = true
			continue
		}
		if !strings.HasPrefix(a, "--") || a == "-" {
			o.pos = append(o.pos, a)
			continue
		}
		k, v, has := strings.Cut(strings.TrimPrefix(a, "--"), "=")
		if !takes[k] {
			return nil, fmt.Errorf("不认识的参数 --%s", k)
		}
		if !has {
			if i+1 >= len(args) {
				return nil, fmt.Errorf("--%s 后面要跟一个值", k)
			}
			i++
			v = args[i]
		}
		o.vals[k] = v
	}
	return o, nil
}

type eeClient struct {
	base, token string
	hc          *http.Client
}

func newEEClient(o *cliOpts) (*eeClient, error) {
	base := strings.TrimRight(firstNonEmpty(o.vals["server"], os.Getenv("DSH_EE_URL")), "/")
	token := firstNonEmpty(o.vals["token"], os.Getenv("DSH_EE_TOKEN"))
	if base == "" {
		return nil, errors.New("不知道 dsh-ee 的地址：加 --server https://… 或设置环境变量 DSH_EE_URL")
	}
	if u, err := url.Parse(base); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("dsh-ee 的地址 %q 不对，要像 https://ee.acme.com 这样", base)
	}
	if token == "" {
		return nil, errors.New("没有令牌：在网页「我的令牌」里建一个带 config 范围的令牌，用 --token 或环境变量 DSH_EE_TOKEN 传进来")
	}
	return &eeClient{base: base, token: token, hc: &http.Client{Timeout: 2 * time.Minute}}, nil
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// do 发请求；非 2xx 时把服务端的 error 字段变成错误。返回原始响应体。
func (c *eeClient) do(method, path string, body any) ([]byte, int, error) {
	var rd io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		rd = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, c.base+path, rd)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("X-DshEE-Client", "dsh-ee-cli/"+version)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("连不上 %s：%w", c.base, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	if resp.StatusCode >= 300 {
		var e struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(raw, &e) == nil && e.Error != "" {
			return raw, resp.StatusCode, fmt.Errorf("dsh-ee 返回 HTTP %d：%s", resp.StatusCode, e.Error)
		}
		return raw, resp.StatusCode, fmt.Errorf("dsh-ee 返回 HTTP %d：%s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return raw, resp.StatusCode, nil
}

func readInput(path string) (string, error) {
	if path == "-" {
		raw, err := io.ReadAll(os.Stdin)
		return string(raw), err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("读不了 %s：%w", path, err)
	}
	return string(raw), nil
}

// accessPart 从配置文件里取出 fields 和 rules（explain / simulate 只看这两部分）。
func accessPart(text string) (map[string]any, error) {
	var doc struct {
		Fields []map[string]any `yaml:"fields"`
		Rules  []map[string]any `yaml:"rules"`
	}
	if err := yaml.Unmarshal([]byte(text), &doc); err != nil {
		return nil, fmt.Errorf("配置文件不是合法的 YAML：%w", err)
	}
	if doc.Fields == nil {
		doc.Fields = []map[string]any{}
	}
	if doc.Rules == nil {
		doc.Rules = []map[string]any{}
	}
	return map[string]any{"fields": doc.Fields, "rules": doc.Rules}, nil
}

func printRawJSON(w io.Writer, raw []byte) {
	var v any
	if json.Unmarshal(raw, &v) != nil {
		_, _ = w.Write(raw)
		return
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func cliConfig(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprint(stdout, configUsage)
		if len(args) == 0 {
			return 2
		}
		return 0
	}
	cmd, rest := args[0], args[1:]
	if cmd == "status" {
		cmd = "changes"
	}
	valued := []string{"server", "token"}
	switch cmd {
	case "test":
		valued = append(valued, "tests")
	case "apply":
		valued = append(valued, "note")
	case "explain":
		valued = append(valued, "user", "doc", "config")
	case "simulate":
		valued = append(valued, "users", "kb", "docs", "config")
	case "export", "plan", "changes":
	default:
		fmt.Fprintf(stderr, "不认识的命令 config %s\n\n", cmd)
		fmt.Fprint(stderr, configUsage)
		return 2
	}
	o, err := parseOpts(rest, valued...)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	c, err := newEEClient(o)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	fail := func(err error) int {
		fmt.Fprintln(stderr, err)
		return 1
	}
	switch cmd {
	case "export":
		path := "/api/admin/config"
		if o.asJSON {
			path += "?format=json"
		}
		raw, _, err := c.do("GET", path, nil)
		if err != nil {
			return fail(err)
		}
		_, _ = stdout.Write(raw)
		return 0

	case "plan":
		if len(o.pos) != 1 {
			fmt.Fprintln(stderr, "用法：dsh-ee config plan <文件>")
			return 2
		}
		text, err := readInput(o.pos[0])
		if err != nil {
			return fail(err)
		}
		raw, _, err := c.do("POST", "/api/admin/config/plan", map[string]string{"config": text})
		if err != nil {
			return fail(err)
		}
		var plan planOut
		_ = json.Unmarshal(raw, &plan)
		if o.asJSON {
			printRawJSON(stdout, raw)
		} else {
			printPlan(stdout, &plan)
		}
		if !plan.OK || len(plan.Problems) > 0 {
			return 1
		}
		return 0

	case "test":
		if o.vals["tests"] == "" || len(o.pos) > 1 {
			fmt.Fprintln(stderr, "用法：dsh-ee config test [<配置文件>] --tests <测试文件>")
			return 2
		}
		tests, err := readInput(o.vals["tests"])
		if err != nil {
			return fail(err)
		}
		body := map[string]string{"tests": tests}
		if len(o.pos) == 1 {
			if body["config"], err = readInput(o.pos[0]); err != nil {
				return fail(err)
			}
		}
		raw, _, err := c.do("POST", "/api/admin/config/test", body)
		if err != nil {
			return fail(err)
		}
		var res struct {
			OK       bool     `json:"ok"`
			Problems []string `json:"problems"`
			Results  []struct {
				Case   int      `json:"case"`
				As     string   `json:"as"`
				OK     bool     `json:"ok"`
				Errors []string `json:"errors"`
			} `json:"results"`
		}
		_ = json.Unmarshal(raw, &res)
		if o.asJSON {
			printRawJSON(stdout, raw)
		} else {
			for _, p := range res.Problems {
				fmt.Fprintf(stdout, "✗ 配置有问题：%s\n", p)
			}
			pass := 0
			for _, r := range res.Results {
				if r.OK {
					pass++
					fmt.Fprintf(stdout, "✓ 第 %d 条（%s）\n", r.Case, r.As)
					continue
				}
				fmt.Fprintf(stdout, "✗ 第 %d 条（%s）\n", r.Case, r.As)
				for _, e := range r.Errors {
					fmt.Fprintf(stdout, "    %s\n", e)
				}
			}
			fmt.Fprintf(stdout, "\n%d 条里通过 %d 条\n", len(res.Results), pass)
		}
		if !res.OK {
			return 1
		}
		return 0

	case "apply":
		if len(o.pos) != 1 {
			fmt.Fprintln(stderr, "用法：dsh-ee config apply <文件> [--note 说明]")
			return 2
		}
		text, err := readInput(o.pos[0])
		if err != nil {
			return fail(err)
		}
		raw, _, err := c.do("POST", "/api/admin/config/changes", map[string]string{"config": text, "note": o.vals["note"]})
		if err != nil {
			if raw != nil && !o.asJSON {
				var bad struct {
					Plan *planOut `json:"plan"`
				}
				if json.Unmarshal(raw, &bad) == nil && bad.Plan != nil {
					printPlan(stderr, bad.Plan)
				}
			}
			return fail(err)
		}
		if o.asJSON {
			printRawJSON(stdout, raw)
			return 0
		}
		var res struct {
			Change struct {
				ID int64 `json:"id"`
			} `json:"change_request"`
			Plan    planOut `json:"plan"`
			Message string  `json:"message"`
		}
		_ = json.Unmarshal(raw, &res)
		printPlan(stdout, &res.Plan)
		fmt.Fprintf(stdout, "\n已提交变更请求 #%d：%s\n网页查看：%s/admin/fields?tab=changes\n", res.Change.ID, res.Message, c.base)
		return 0

	case "changes":
		raw, _, err := c.do("GET", "/api/admin/config/changes", nil)
		if err != nil {
			return fail(err)
		}
		if o.asJSON {
			printRawJSON(stdout, raw)
			return 0
		}
		var res struct {
			Items []struct {
				ID        int64     `json:"id"`
				Title     string    `json:"title"`
				Status    string    `json:"status"`
				CreatedAt time.Time `json:"created_at"`
			} `json:"items"`
		}
		_ = json.Unmarshal(raw, &res)
		if len(res.Items) == 0 {
			fmt.Fprintln(stdout, "还没有变更请求")
		}
		for _, it := range res.Items {
			fmt.Fprintf(stdout, "#%-5d %-10s %s  %s\n", it.ID, it.Status, it.CreatedAt.Local().Format("2006-01-02 15:04"), it.Title)
		}
		return 0

	case "explain":
		doc, err := strconv.ParseInt(o.vals["doc"], 10, 64)
		if o.vals["user"] == "" || err != nil {
			fmt.Fprintln(stderr, "用法：dsh-ee config explain --user 用户 --doc 文档id [--config 文件]")
			return 2
		}
		body := map[string]any{"user": o.vals["user"], "doc": doc}
		if p := o.vals["config"]; p != "" {
			text, err := readInput(p)
			if err != nil {
				return fail(err)
			}
			if body["config"], err = accessPart(text); err != nil {
				return fail(err)
			}
		}
		raw, _, err := c.do("POST", "/api/admin/access/explain", body)
		if err != nil {
			return fail(err)
		}
		if o.asJSON {
			printRawJSON(stdout, raw)
			return 0
		}
		var res struct {
			Allowed bool   `json:"allowed"`
			User    string `json:"user"`
			Title   string `json:"title"`
			Steps   []struct {
				Stage  string `json:"stage"`
				Result string `json:"result"`
				Reason string `json:"reason"`
			} `json:"steps"`
		}
		_ = json.Unmarshal(raw, &res)
		verdict := "看不到"
		if res.Allowed {
			verdict = "能看到"
		}
		fmt.Fprintf(stdout, "%s %s《%s》\n", res.User, verdict, res.Title)
		for i, st := range res.Steps {
			fmt.Fprintf(stdout, "  %d. [%s] %s：%s\n", i+1, st.Result, st.Stage, st.Reason)
		}
		return 0

	case "simulate":
		users := splitList(o.vals["users"])
		if len(users) == 0 || (o.vals["kb"] == "" && o.vals["docs"] == "") {
			fmt.Fprintln(stderr, "用法：dsh-ee config simulate --users a,b (--kb 知识库 | --docs 1,2) [--config 文件]")
			return 2
		}
		body := map[string]any{"users": users, "kb": o.vals["kb"]}
		var docs []int64
		for _, d := range splitList(o.vals["docs"]) {
			id, err := strconv.ParseInt(d, 10, 64)
			if err != nil {
				fmt.Fprintf(stderr, "文档 id %q 不是数字\n", d)
				return 2
			}
			docs = append(docs, id)
		}
		if len(docs) > 0 {
			body["docs"] = docs
		}
		if p := o.vals["config"]; p != "" {
			text, err := readInput(p)
			if err != nil {
				return fail(err)
			}
			if body["config"], err = accessPart(text); err != nil {
				return fail(err)
			}
		}
		raw, _, err := c.do("POST", "/api/admin/access/simulate", body)
		if err != nil {
			return fail(err)
		}
		// 表的结构以服务端为准，这里原样给出
		printRawJSON(stdout, raw)
		return 0
	}
	return 2
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

type planOut struct {
	OK       bool     `json:"ok"`
	Problems []string `json:"problems"`
	Changes  []string `json:"changes"`
	Notes    []string `json:"notes"`
	Impact   []struct {
		User   string   `json:"user"`
		KB     string   `json:"kb"`
		Lose   []string `json:"lose"`
		Gain   []string `json:"gain"`
		Recent []string `json:"recently_read_lost"`
	} `json:"impact"`
	PermImpact []struct {
		User string   `json:"user"`
		Lose []string `json:"lose"`
		Gain []string `json:"gain"`
	} `json:"perm_impact"`
	Summary string `json:"summary"`
}

func printPlan(w io.Writer, p *planOut) {
	if p.Summary != "" {
		fmt.Fprintln(w, p.Summary)
	}
	for _, s := range p.Problems {
		fmt.Fprintf(w, "✗ %s\n", s)
	}
	if len(p.Changes) > 0 {
		fmt.Fprintln(w, "\n会变的：")
		for _, s := range p.Changes {
			fmt.Fprintf(w, "  · %s\n", s)
		}
	}
	if len(p.PermImpact) > 0 {
		fmt.Fprintln(w, "\n权限变化：")
		for _, im := range p.PermImpact {
			if len(im.Gain) > 0 {
				fmt.Fprintf(w, "  %s 多了：%s\n", im.User, strings.Join(im.Gain, "、"))
			}
			if len(im.Lose) > 0 {
				fmt.Fprintf(w, "  %s 少了：%s\n", im.User, strings.Join(im.Lose, "、"))
			}
		}
	}
	if len(p.Impact) > 0 {
		fmt.Fprintln(w, "\n能看的文档变化：")
		for _, im := range p.Impact {
			if len(im.Lose) > 0 {
				fmt.Fprintf(w, "  %s 在「%s」里将看不到 %d 篇：%s\n", im.User, im.KB, len(im.Lose), clip(im.Lose, 5))
			}
			if len(im.Recent) > 0 {
				fmt.Fprintf(w, "    其中最近看过的：%s\n", clip(im.Recent, 5))
			}
			if len(im.Gain) > 0 {
				fmt.Fprintf(w, "  %s 在「%s」里将多看到 %d 篇：%s\n", im.User, im.KB, len(im.Gain), clip(im.Gain, 5))
			}
		}
	}
	for _, s := range p.Notes {
		fmt.Fprintf(w, "注：%s\n", s)
	}
}

func clip(xs []string, n int) string {
	if len(xs) <= n {
		return strings.Join(xs, "、")
	}
	return strings.Join(xs[:n], "、") + fmt.Sprintf(" 等 %d 篇", len(xs))
}

const adminUsage = `用法：dsh-ee admin recover

  recover   在服务器上生成一个 1 小时有效的恢复账号（有全部管理权限点，不靠超级管理员）。
            把自己锁在外面时用；要在能连到数据库的服务器上执行，读和服务一样的环境变量。审计里留记录。
`

func cliAdmin(args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 || args[0] != "recover" {
		if len(args) == 1 && (args[0] == "help" || args[0] == "-h" || args[0] == "--help") {
			fmt.Fprint(stdout, adminUsage)
			return 0
		}
		fmt.Fprint(stderr, adminUsage)
		return 2
	}
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(stderr, "读不了配置：", err)
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	st, err := openStore(ctx, cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		fmt.Fprintln(stderr, "连不上数据库：", err)
		return 1
	}
	defer st.Close()
	if err := st.Migrate(ctx); err != nil {
		fmt.Fprintln(stderr, "数据库迁移失败：", err)
		return 1
	}
	user, pass, expires, err := api.Recover(ctx, st)
	if err != nil {
		// 审计失败时账号已经建好了，凭据还在返回值里：必须照常打出来，同时显式告警并非零退出，
		// 不能只报「生成失败」把事情糊过去（否则库里会多出一个没人知道凭据的无审计全权账号）。
		if user == "" || pass == "" || expires.IsZero() {
			fmt.Fprintln(stderr, "生成恢复账号失败：", err)
			return 1
		}
		fmt.Fprintln(stderr, "⚠️  严重警告：恢复账号已创建，但审计记录没有写入（audit_logs）。")
		fmt.Fprintln(stderr, "请立即人工记录下面的凭据与到期时间，并排查审计表。原因：", err)
	}
	fmt.Fprintf(stdout, "已生成恢复账号（%s 前有效，到期自动停用）：\n  用户名：%s\n  密码：  %s\n用它登录网页，把管理角色重新分好后就不要再用。\n", expires.Local().Format("2006-01-02 15:04"), user, pass)
	if err != nil {
		return 1
	}
	return 0
}
