// Package openapi 把 OpenAPI 3 文档里的操作转换成 MCP 工具，并按工具调用发出 HTTP 请求。
// 只支持 dsh-ee 需要的部分：本地 $ref、路径 / 查询参数、JSON 请求体。
package openapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Param 是一个路径或查询参数。
type Param struct {
	Name     string `json:"name"`
	In       string `json:"in"`
	Required bool   `json:"required"`
}

// Operation 记下调用时怎么拼请求。
type Operation struct {
	Method string  `json:"method"`
	Path   string  `json:"path"`
	Params []Param `json:"params"`
	// 有 JSON 请求体时，工具参数里的 body 字段就是它
	Body bool `json:"body"`
}

type Tool struct {
	Name        string
	Description string
	InputSchema json.RawMessage
	Op          Operation
	// GET / HEAD 视为只读
	ReadOnly bool
}

type Spec struct {
	Title   string
	BaseURL string
	Tools   []Tool
}

var methods = []string{"get", "post", "put", "patch", "delete"}

// Parse 解析 JSON 或 YAML 的 OpenAPI 3 文档。
func Parse(doc []byte) (*Spec, error) {
	var root map[string]any
	if err := yaml.Unmarshal(doc, &root); err != nil {
		return nil, fmt.Errorf("不是合法的 JSON / YAML：%w", err)
	}
	if v, _ := root["openapi"].(string); !strings.HasPrefix(v, "3.") {
		return nil, errors.New("只支持 OpenAPI 3.x（文档里要有 openapi: 3.x）")
	}
	r := resolver{root: root}
	spec := &Spec{}
	if info, ok := root["info"].(map[string]any); ok {
		spec.Title, _ = info["title"].(string)
	}
	if servers, ok := root["servers"].([]any); ok && len(servers) > 0 {
		if s, ok := servers[0].(map[string]any); ok {
			spec.BaseURL, _ = s["url"].(string)
		}
	}
	paths, _ := root["paths"].(map[string]any)
	pathKeys := make([]string, 0, len(paths))
	for p := range paths {
		pathKeys = append(pathKeys, p)
	}
	sort.Strings(pathKeys)
	used := map[string]bool{}
	for _, p := range pathKeys {
		item, _ := r.deref(paths[p], 0).(map[string]any)
		shared, _ := item["parameters"].([]any)
		for _, m := range methods {
			raw, ok := item[m].(map[string]any)
			if !ok {
				continue
			}
			t, err := r.tool(m, p, raw, shared)
			if err != nil {
				return nil, fmt.Errorf("%s %s：%w", strings.ToUpper(m), p, err)
			}
			t.Name = UniqueToolName(t.Name, used)
			spec.Tools = append(spec.Tools, t)
		}
	}
	if len(spec.Tools) == 0 {
		return nil, errors.New("文档里没有任何操作（paths 为空）")
	}
	return spec, nil
}

// UniqueToolName 返回一个在 used 里没出现过的工具名（大小写不敏感，按数据库 utf8mb4 CI 口径）。
// 重名时追加 _2、_3…；后缀计入 64 字符预算，避免名字超长。
func UniqueToolName(base string, used map[string]bool) string {
	key := strings.ToLower(base)
	if !used[key] {
		used[key] = true
		return base
	}
	for i := 2; ; i++ {
		suffix := "_" + strconv.Itoa(i)
		head := base
		if len(head)+len(suffix) > 64 {
			head = head[:64-len(suffix)]
		}
		candidate := head + suffix
		candidateKey := strings.ToLower(candidate)
		if !used[candidateKey] {
			used[candidateKey] = true
			return candidate
		}
	}
}

var nonName = regexp.MustCompile(`[^A-Za-z0-9]+`)

// ToolName 把 operationId 或「方法_路径」规范成 MCP 工具名。
func ToolName(s string) string {
	s = strings.Trim(nonName.ReplaceAllString(s, "_"), "_")
	if s == "" {
		s = "op"
	}
	return truncate(s, 64)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

type resolver struct{ root map[string]any }

// deref 解析本地 $ref（#/components/...）；超过深度时返回空对象，避免循环引用。
func (r resolver) deref(v any, depth int) any {
	m, ok := v.(map[string]any)
	if !ok {
		return v
	}
	ref, ok := m["$ref"].(string)
	if !ok {
		return v
	}
	if depth > 16 || !strings.HasPrefix(ref, "#/") {
		return map[string]any{}
	}
	var cur any = r.root
	for _, part := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
		part = strings.NewReplacer("~1", "/", "~0", "~").Replace(part)
		cm, ok := cur.(map[string]any)
		if !ok {
			return map[string]any{}
		}
		cur = cm[part]
	}
	return r.deref(cur, depth+1)
}

// inline 递归展开 schema 里的 $ref，得到自包含的 JSON Schema。
// 引用成环（如 Customer.parent 指回 Customer）时，第二次遇到同一个 $ref 就截断成任意对象。
func (r resolver) inline(v any, depth int) any {
	return r.inlineRefs(v, depth, nil)
}

func (r resolver) inlineRefs(v any, depth int, refs []string) any {
	if depth > 64 {
		return map[string]any{"type": "object"}
	}
	if m, ok := v.(map[string]any); ok {
		if ref, ok := m["$ref"].(string); ok {
			if slices.Contains(refs, ref) {
				return map[string]any{"type": "object", "description": "（递归引用 " + ref + "，结构同上）"}
			}
			return r.inlineRefs(r.deref(v, 0), depth+1, append(refs, ref))
		}
	}
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			switch k {
			case "example", "examples", "xml", "externalDocs", "discriminator", "deprecated":
				continue
			}
			out[k] = r.inlineRefs(val, depth+1, refs)
		}
		// OpenAPI 3.0 的 nullable 在 JSON Schema 里没有对应
		delete(out, "nullable")
		return out
	case []any:
		out := make([]any, len(x))
		for i, val := range x {
			out[i] = r.inlineRefs(val, depth+1, refs)
		}
		return out
	default:
		return v
	}
}

func (r resolver) tool(method, path string, op map[string]any, shared []any) (Tool, error) {
	name, _ := op["operationId"].(string)
	if name == "" {
		name = method + "_" + path
	}
	summary, _ := op["summary"].(string)
	desc, _ := op["description"].(string)
	description := strings.TrimSpace(strings.Join(nonEmpty(summary, desc), "\n\n"))
	if description == "" {
		description = strings.ToUpper(method) + " " + path
	}
	props := map[string]any{}
	var required []string
	o := Operation{Method: strings.ToUpper(method), Path: path, Params: []Param{}}
	seen := map[string]bool{}
	own, _ := op["parameters"].([]any)
	for _, list := range [][]any{own, shared} {
		for _, p := range list {
			pm, ok := r.deref(p, 0).(map[string]any)
			if !ok {
				continue
			}
			in, _ := pm["in"].(string)
			pname, _ := pm["name"].(string)
			if pname == "" || (in != "path" && in != "query") || seen[in+":"+pname] {
				continue
			}
			seen[in+":"+pname] = true
			req, _ := pm["required"].(bool)
			if in == "path" {
				req = true
			}
			schema, _ := r.inline(pm["schema"], 0).(map[string]any)
			if schema == nil {
				schema = map[string]any{"type": "string"}
			}
			if d, _ := pm["description"].(string); d != "" {
				schema["description"] = d
			}
			props[pname] = schema
			if req {
				required = append(required, pname)
			}
			o.Params = append(o.Params, Param{Name: pname, In: in, Required: req})
		}
	}
	if rb, ok := r.deref(op["requestBody"], 0).(map[string]any); ok {
		content, _ := rb["content"].(map[string]any)
		for ct, media := range content {
			if !strings.Contains(ct, "json") {
				continue
			}
			mm, _ := media.(map[string]any)
			schema := r.inline(mm["schema"], 0)
			if schema == nil {
				schema = map[string]any{"type": "object"}
			}
			props["body"] = schema
			o.Body = true
			if req, _ := rb["required"].(bool); req {
				required = append(required, "body")
			}
			break
		}
	}
	input := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		sort.Strings(required)
		input["required"] = required
	}
	schema, err := json.Marshal(input)
	if err != nil {
		return Tool{}, err
	}
	return Tool{Name: ToolName(name), Description: description, InputSchema: schema, Op: o, ReadOnly: method == "get"}, nil
}

func nonEmpty(xs ...string) []string {
	var out []string
	for _, x := range xs {
		if strings.TrimSpace(x) != "" {
			out = append(out, strings.TrimSpace(x))
		}
	}
	return out
}

// checkPathValue 拦下会把请求改到别的接口上的路径参数：空值让 /customers/{id} 变成 /customers/（列表接口）；
// 「.」「..」会被上游或它前面的代理折叠掉（/customers/../notes 变成 /notes）。管理员按接口开放工具，
// 模型给的参数不能让一个工具打到另一个接口上。「/」会被转义成 %2F，有的上游会先解码再路由，所以分段后一起查。
func checkPathValue(name, v string) error {
	if v == "" {
		return fmt.Errorf("路径参数 %s 不能为空", name)
	}
	for _, seg := range strings.FieldsFunc(v, func(r rune) bool { return r == '/' || r == '\\' }) {
		if seg == "." || seg == ".." {
			return fmt.Errorf("路径参数 %s 不能是「.」或「..」", name)
		}
	}
	return nil
}

// Call 按工具参数发请求。返回状态码与响应体（最多 limit 字节）。
func Call(ctx context.Context, hc *http.Client, baseURL string, op Operation, args map[string]any, header http.Header, limit int64) (int, []byte, error) {
	path := op.Path
	q := url.Values{}
	for _, p := range op.Params {
		v, ok := args[p.Name]
		if !ok || v == nil {
			if p.Required {
				return 0, nil, fmt.Errorf("缺少必填参数 %s", p.Name)
			}
			continue
		}
		s := scalar(v)
		if p.In == "path" {
			if err := checkPathValue(p.Name, s); err != nil {
				return 0, nil, err
			}
			path = strings.ReplaceAll(path, "{"+p.Name+"}", url.PathEscape(s))
		} else {
			q.Set(p.Name, s)
		}
	}
	u := strings.TrimRight(baseURL, "/") + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	var body io.Reader
	if op.Body {
		if b, ok := args["body"]; ok && b != nil {
			raw, err := json.Marshal(b)
			if err != nil {
				return 0, nil, err
			}
			body = bytes.NewReader(raw)
		}
	}
	req, err := http.NewRequestWithContext(ctx, op.Method, u, body)
	if err != nil {
		return 0, nil, err
	}
	for k, vs := range header {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	req.Header.Set("Accept", "application/json, text/plain;q=0.9, */*;q=0.5")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := hc.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	return resp.StatusCode, out, err
}

func scalar(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case json.Number:
		return x.String()
	case float64:
		return strings.TrimSuffix(strings.TrimRight(fmt.Sprintf("%f", x), "0"), ".")
	case bool:
		if x {
			return "true"
		}
		return "false"
	default:
		b, _ := json.Marshal(x)
		return string(b)
	}
}
