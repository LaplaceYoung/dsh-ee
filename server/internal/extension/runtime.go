package extension

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"slices"
	"strings"
	"testing/fstest"
)

// 这里是装进 dsh-ee 之后、调用时要用的那部分：把清单叠加到接口文档生成的工具上
// （说明、只读、谁来填参数、只保留哪些字段），以及读上传的扩展包。

// 上传包的上限：zip 本身、解开后每个文件、解开后合计、文件个数。
const (
	MaxPackage   = 8 << 20
	maxFileSize  = 4 << 20
	maxTotalSize = 16 << 20
	maxFiles     = 64
)

// UnderServer 报告 target 是不是落在服务器地址 server 之内（协议、主机相同，路径以它开头）。
func UnderServer(target, server string) bool { return underServer(target, server) }

// CheckFiles 检查内存里的一组文件（路径 → 内容）。
func CheckFiles(files map[string][]byte) *Result {
	m := fstest.MapFS{}
	for name, b := range files {
		m[name] = &fstest.MapFile{Data: b, Mode: 0o444}
	}
	return Check(m)
}

// ReadZip 解开扩展的 zip 包，返回扩展根目录下的文件。清单可以在包的根目录，
// 也可以在唯一的一层目录下（GitHub「下载 ZIP」和 `zip -r ext.zip ext/` 都是这样）。
func ReadZip(b []byte) (map[string][]byte, error) {
	if len(b) > MaxPackage {
		return nil, fmt.Errorf("扩展包不能超过 %d MB", MaxPackage>>20)
	}
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return nil, errors.New("不是合法的 zip 文件")
	}
	all := map[string][]byte{}
	total := 0
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		// 有些 Windows 上的压缩工具用反斜杠分隔路径
		name := path.Clean(strings.TrimPrefix(strings.ReplaceAll(f.Name, "\\", "/"), "./"))
		if !fs.ValidPath(name) || strings.HasPrefix(name, "__MACOSX/") || path.Base(name) == ".DS_Store" {
			continue
		}
		if len(all) >= maxFiles {
			return nil, fmt.Errorf("扩展包里的文件太多（最多 %d 个）；声明式扩展只需要清单和接口文档", maxFiles)
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("读 %s：%w", name, err)
		}
		data, err := io.ReadAll(io.LimitReader(rc, maxFileSize+1))
		rc.Close()
		if err != nil {
			return nil, fmt.Errorf("读 %s：%w", name, err)
		}
		if len(data) > maxFileSize {
			return nil, fmt.Errorf("%s 超过 %d MB", name, maxFileSize>>20)
		}
		if total += len(data); total > maxTotalSize {
			return nil, fmt.Errorf("扩展包解开后超过 %d MB", maxTotalSize>>20)
		}
		all[name] = data
	}
	if _, ok := all[ManifestFile]; ok {
		return all, nil
	}
	// 清单不在根目录：只认唯一一层目录下的
	var root string
	for name := range all {
		if path.Base(name) == ManifestFile && strings.Count(name, "/") == 1 {
			if root != "" {
				return nil, errors.New("扩展包里有不止一份 " + ManifestFile + "，一个包只能装一个扩展")
			}
			root = path.Dir(name) + "/"
		}
	}
	if root == "" {
		return nil, errors.New("扩展包里找不到 " + ManifestFile + "：它要在包的根目录，或唯一的一层目录下")
	}
	out := map[string][]byte{}
	for name, data := range all {
		if rest, ok := strings.CutPrefix(name, root); ok {
			out[rest] = data
		}
	}
	return out, nil
}

// ParseManifest 读清单。调用时用：装进来之前已经检查过了，这里只取值。
func ParseManifest(raw []byte) (*Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("清单读不了：%w", err)
	}
	return &m, nil
}

// Tool 按名字找清单里的工具；没提到就是 nil。
func (m *Manifest) Tool(name string) *Tool {
	for i := range m.Tools {
		if m.Tools[i].Name == name {
			return &m.Tools[i]
		}
	}
	return nil
}

// AdminParam 是安装时管理员要统一设置的一个参数（fill: admin）。同名参数在几个工具上出现时只填一次。
type AdminParam struct {
	// 参数名，请求体里的字段去掉 body. 前缀
	Key     string          `json:"key"`
	Label   string          `json:"label"`
	Type    string          `json:"type,omitempty"`
	Enum    []any           `json:"enum,omitempty"`
	Default json.RawMessage `json:"default,omitempty"`
	Tools   []string        `json:"tools"`
}

// AdminKey 是 fill: admin 参数在安装配置里的键。
func AdminKey(name string) string { return strings.TrimPrefix(name, "body.") }

// AdminParams 列出清单里所有要管理员设置的参数。
func (m *Manifest) AdminParams() []AdminParam {
	var out []AdminParam
	idx := map[string]int{}
	for _, t := range m.Tools {
		for _, p := range t.Params {
			if p.Fill != FillAdmin {
				continue
			}
			key := AdminKey(p.Name)
			if i, ok := idx[key]; ok {
				out[i].Tools = append(out[i].Tools, t.Name)
				continue
			}
			idx[key] = len(out)
			out = append(out, AdminParam{Key: key, Label: p.Label.Any(), Type: p.Type, Enum: p.Enum, Default: p.Default, Tools: []string{t.Name}})
		}
	}
	return out
}

// CheckValue 核对管理员给这个参数填的值：类型要和清单写的一致，有可选值的只能从里面选。
// 返回给填的人看的问题，没问题是空串。
func (p AdminParam) CheckValue(raw json.RawMessage) string {
	name := p.Label
	if name == "" {
		name = p.Key
	}
	if p.Type != "" && !jsonIs(raw, p.Type) {
		return fmt.Sprintf("参数「%s」要填 %s 类型的值", name, p.Type)
	}
	if len(p.Enum) > 0 {
		var v any
		if json.Unmarshal(raw, &v) != nil {
			return fmt.Sprintf("参数「%s」的设置值不是合法的 JSON", name)
		}
		got, _ := json.Marshal(v)
		if !slices.ContainsFunc(p.Enum, func(c any) bool { b, err := json.Marshal(c); return err == nil && bytes.Equal(b, got) }) {
			return fmt.Sprintf("参数「%s」只能从清单给出的选项里选", name)
		}
	}
	return ""
}

// Schema 把清单对一个工具的补充叠加到接口文档生成的参数定义上：
// 不由模型填的参数（fill: app / admin）从定义里拿掉，模型看不到也就没法填；
// 由模型填的参数换上清单里写的说明、可选值。
func (t *Tool) Schema(input json.RawMessage) (json.RawMessage, error) {
	var root map[string]any
	if err := json.Unmarshal(input, &root); err != nil {
		return nil, err
	}
	props, _ := root["properties"].(map[string]any)
	for _, p := range t.Params {
		holder, field := root, p.Name
		hp := props
		if f, ok := strings.CutPrefix(p.Name, "body."); ok {
			body, _ := props["body"].(map[string]any)
			if body == nil {
				continue
			}
			holder, field = body, f
			hp, _ = body["properties"].(map[string]any)
		}
		if hp == nil {
			continue
		}
		if p.Fill == FillApp || p.Fill == FillAdmin {
			delete(hp, field)
			dropRequired(holder, field)
			continue
		}
		schema, _ := hp[field].(map[string]any)
		if schema == nil {
			continue
		}
		if d := p.Description.Any(); d != "" {
			schema["description"] = d
		}
		if len(p.Enum) > 0 {
			schema["enum"] = p.Enum
		}
		if p.Required {
			addRequired(holder, field)
		}
	}
	// 请求体里能填的字段都拿掉了，也就不用再要求模型给 body
	if body, _ := props["body"].(map[string]any); body != nil {
		if bp, ok := body["properties"].(map[string]any); ok && len(bp) == 0 && len(t.fixedBody()) > 0 {
			delete(props, "body")
			dropRequired(root, "body")
		}
	}
	return json.Marshal(root)
}

func (t *Tool) fixedBody() []string {
	var out []string
	for _, p := range t.Params {
		if f, ok := strings.CutPrefix(p.Name, "body."); ok && (p.Fill == FillApp || p.Fill == FillAdmin) {
			out = append(out, f)
		}
	}
	return out
}

func dropRequired(schema map[string]any, name string) {
	req, _ := schema["required"].([]any)
	out := req[:0:0]
	for _, r := range req {
		if r != name {
			out = append(out, r)
		}
	}
	if len(out) == 0 {
		delete(schema, "required")
	} else {
		schema["required"] = out
	}
}

func addRequired(schema map[string]any, name string) {
	req, _ := schema["required"].([]any)
	for _, r := range req {
		if r == name {
			return
		}
	}
	schema["required"] = append(req, name)
}

// Fix 把不由模型填的参数补进调用参数里，覆盖模型给的同名值。
// admin 是安装时管理员设置的值（键见 AdminKey）；没设置就用清单里的 default。
// fill: app 的参数目前取清单里的 default（按应用固定在之后的版本里做）。
func (t *Tool) Fix(args map[string]any, admin map[string]json.RawMessage) error {
	// 请求体里有不由模型填的字段时，body 必须是对象：模型把整个 body 传成字符串或数组，
	// 这里就没法把那些字段换掉或拿掉，不能原样发出去
	if b, ok := args["body"]; ok && b != nil && len(t.fixedBody()) > 0 {
		if _, isObj := b.(map[string]any); !isObj {
			return errors.New("参数 body 要是一个 JSON 对象")
		}
	}
	for _, p := range t.Params {
		if p.Fill != FillApp && p.Fill != FillAdmin {
			continue
		}
		raw := p.Default
		if p.Fill == FillAdmin {
			if v, ok := admin[AdminKey(p.Name)]; ok {
				raw = v
			}
		}
		var val any
		if len(raw) > 0 {
			dec := json.NewDecoder(bytes.NewReader(raw))
			dec.UseNumber()
			if err := dec.Decode(&val); err != nil {
				return fmt.Errorf("参数 %s 的设置值不是合法的 JSON", p.Name)
			}
		}
		field, inBody := strings.CutPrefix(p.Name, "body.")
		if val == nil {
			// 没有值：确保模型也没法替它填
			if inBody {
				if body, ok := args["body"].(map[string]any); ok {
					delete(body, field)
				}
			} else {
				delete(args, p.Name)
			}
			if p.Required {
				who := "管理员"
				if p.Fill == FillApp {
					who = "应用"
				}
				return fmt.Errorf("参数 %s 要由%s设置，但还没有设置", p.Name, who)
			}
			continue
		}
		if inBody {
			body, ok := args["body"].(map[string]any)
			if !ok {
				body = map[string]any{}
				args["body"] = body
			}
			body[field] = val
		} else {
			args[p.Name] = val
		}
	}
	return nil
}

// Keep 只保留返回结果里的这些字段。写第一层的字段名：结果是对象就筛它的字段，是数组就筛每一项。
// 「字段.子字段」再往里一层：那个字段是对象就筛它的子字段，是数组就筛它每一项的子字段——
// {"items": [...], "total": 1} 这样包了一层的列表，写 items.id、items.title、total。
// 不是 JSON 或结构对不上时原样返回（宁可多给，也不丢掉模型需要的内容）。
func Keep(body []byte, keep []string) []byte {
	if len(keep) == 0 {
		return body
	}
	var v any
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	if dec.Decode(&v) != nil {
		return body
	}
	var whole, parents []string
	sub := map[string][]string{}
	for _, k := range keep {
		parent, child, dotted := strings.Cut(k, ".")
		switch {
		case !dotted:
			whole = append(whole, k)
		case parent != "" && child != "" && !strings.Contains(child, "."):
			if sub[parent] == nil {
				parents = append(parents, parent)
			}
			sub[parent] = append(sub[parent], child)
		}
	}
	pick := func(m map[string]any, fields []string) map[string]any {
		out := make(map[string]any, len(fields))
		for _, k := range fields {
			if val, ok := m[k]; ok {
				out[k] = val
			}
		}
		return out
	}
	// each 对一个对象、或数组里的每个对象做同一件事；别的值原样留着
	each := func(x any, f func(map[string]any) map[string]any) any {
		switch y := x.(type) {
		case map[string]any:
			return f(y)
		case []any:
			for i, item := range y {
				if m, ok := item.(map[string]any); ok {
					y[i] = f(m)
				}
			}
		}
		return x
	}
	top := func(m map[string]any) map[string]any {
		out := pick(m, whole)
		for _, parent := range parents {
			if _, kept := out[parent]; kept {
				continue // 整个字段都留了，不用再筛里面
			}
			if x, ok := m[parent]; ok {
				out[parent] = each(x, func(o map[string]any) map[string]any { return pick(o, sub[parent]) })
			}
		}
		return out
	}
	switch v.(type) {
	case []any, map[string]any:
		v = each(v, top)
	default:
		return body
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if enc.Encode(v) != nil {
		return body
	}
	return bytes.TrimRight(buf.Bytes(), "\n")
}
