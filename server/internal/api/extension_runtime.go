package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/hikariming/dsh-ee/server/internal/extension"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

func manifestTool(m *extension.Manifest, name string) *extension.Tool {
	if m == nil {
		return nil
	}
	for i := range m.Tools {
		if m.Tools[i].Name == name {
			return &m.Tools[i]
		}
	}
	return nil
}

func removeSchemaRequired(raw any, names ...string) []any {
	list, _ := raw.([]any)
	blocked := map[string]bool{}
	for _, name := range names {
		blocked[name] = true
	}
	out := []any{}
	for _, item := range list {
		name, _ := item.(string)
		if !blocked[name] {
			out = append(out, item)
		}
	}
	return out
}

func removeRequired(target map[string]any, names ...string) {
	if target == nil {
		return
	}
	remaining := removeSchemaRequired(target["required"], names...)
	if len(remaining) == 0 {
		delete(target, "required")
	} else {
		target["required"] = remaining
	}
}

func mapStringAny(raw json.RawMessage, out *map[string]any) error {
	if len(raw) == 0 {
		*out = map[string]any{}
		return nil
	}
	return json.Unmarshal(raw, out)
}

// declarativeToolSchema 把 fill:app/admin 从模型可见的参数里删掉。服务端仍会按清单补上这些值。
func declarativeToolSchema(t *store.ConnectionTool, m *extension.Manifest) error {
	dt := manifestTool(m, t.Name)
	if dt == nil {
		return nil
	}
	if desc := dt.Description.Any(); desc != "" {
		t.Description = desc
	}
	t.ReadOnly = dt.ReadOnly
	t.Enabled = t.ReadOnly
	var schema map[string]any
	if err := mapStringAny(t.InputSchema, &schema); err != nil {
		return err
	}
	properties, _ := schema["properties"].(map[string]any)
	for _, p := range dt.Params {
		if p.Fill == "" || p.Fill == extension.FillModel {
			continue
		}
		if strings.HasPrefix(p.Name, "body.") {
			body, _ := properties["body"].(map[string]any)
			bodyProps, _ := body["properties"].(map[string]any)
			name := strings.TrimPrefix(p.Name, "body.")
			delete(bodyProps, name)
			removeRequired(body, name)
			// 整个 body 都由管理员/应用固定时，不该再要求模型传一个空对象才能通过 JSON Schema 校验。
			if len(bodyProps) == 0 {
				removeRequired(schema, "body")
			}
		} else if properties != nil {
			delete(properties, p.Name)
			removeRequired(schema, p.Name)
		}
	}
	raw, err := json.Marshal(schema)
	if err != nil {
		return err
	}
	t.InputSchema = raw
	return nil
}

// applyDeclarativeTools 在存进库之前覆盖只读标记、说明与参数 schema。普通 OpenAPI 连接不进来。
func applyDeclarativeTools(tools []*store.ConnectionTool, m *extension.Manifest) error {
	if m == nil {
		return nil
	}
	for _, tool := range tools {
		if err := declarativeToolSchema(tool, m); err != nil {
			return fmt.Errorf("处理声明式工具 %s：%w", tool.Name, err)
		}
	}
	return nil
}

func requiredRaw(raw json.RawMessage) bool {
	if len(raw) == 0 || string(raw) == "null" {
		return false
	}
	return true
}

func setExtensionArg(args map[string]any, name string, value any) error {
	if strings.HasPrefix(name, "body.") {
		field := strings.TrimPrefix(name, "body.")
		body := map[string]any{}
		if existing, ok := args["body"]; ok && existing != nil {
			var valid bool
			body, valid = existing.(map[string]any)
			if !valid {
				return errors.New("body 必须是 JSON 对象")
			}
		}
		body[field] = value
		args["body"] = body
		return nil
	}
	args[name] = value
	return nil
}

func sameExtensionValue(a, b any) bool {
	if reflect.DeepEqual(a, b) {
		return true
	}
	aRaw, aErr := json.Marshal(a)
	bRaw, bErr := json.Marshal(b)
	return aErr == nil && bErr == nil && string(aRaw) == string(bRaw)
}

func deleteExtensionArg(args map[string]any, name string) {
	if strings.HasPrefix(name, "body.") {
		if body, ok := args["body"].(map[string]any); ok {
			delete(body, strings.TrimPrefix(name, "body."))
		}
		return
	}
	delete(args, name)
}

// extensionParams 按管理员配置和清单默认值补齐参数，并覆盖模型可能手动伪造的固定参数。
func extensionParams(m *extension.Manifest, toolName string, config map[string]json.RawMessage, args map[string]any) error {
	t := manifestTool(m, toolName)
	if t == nil {
		return nil
	}
	for _, p := range t.Params {
		if p.Fill == "" || p.Fill == extension.FillModel {
			continue
		}
		raw := rawDefault(p.Default)
		if p.Fill == extension.FillAdmin {
			if configured, ok := config[p.Name]; ok {
				raw = configured
			}
		}
		// fill:app 当前允许的就是清单的默认绑定；后续由具体应用覆盖，而不是模型或全局安装配置覆盖。
		if p.Fill == extension.FillApp {
			raw = rawDefault(p.Default)
		}
		if !requiredRaw(raw) {
			if p.Required {
				return fmt.Errorf("工具 %s 的参数 %s 需要由 %s 填，但还没有值", toolName, p.Name, p.Fill)
			}
			// 固定参数未配置时应视为未提供，不能把模型伪造的值或 null 带给上游。
			deleteExtensionArg(args, p.Name)
			continue
		}
		typ := p.Type
		if typ == "integer" {
			typ = "number"
		}
		field := extensionInstallField{Key: p.Name, Type: typ, Options: installOptions(p.Options)}
		if err := validInstallValue(field, raw); err != nil {
			return fmt.Errorf("工具 %s 的参数 %s：%w", toolName, p.Name, err)
		}
		if err := setExtensionArg(args, p.Name, jsonValue(raw)); err != nil {
			return err
		}
	}
	// 只读/枚举限制同时适用于模型自己填写的参数；不能通过直接构造 MCP 请求绕过清单。
	for _, p := range t.Params {
		if len(p.Enum) == 0 {
			continue
		}
		var actual any
		var exists bool
		if strings.HasPrefix(p.Name, "body.") {
			body, _ := args["body"].(map[string]any)
			actual, exists = body[strings.TrimPrefix(p.Name, "body.")]
		} else {
			actual, exists = args[p.Name]
		}
		if !exists || actual == nil {
			// 可选枚举参数缺省时不验证；OpenAPI 层仍负责真正的必填校验。
			continue
		}
		valid := false
		for _, candidate := range p.Enum {
			if sameExtensionValue(actual, candidate) {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("工具 %s 的参数 %s 不在允许的枚举值里", toolName, p.Name)
		}
	}
	return nil
}

// extensionRuntimeConfig 合并公开固定值与安装时 a加密的 secret 配置。只有实际调用时才解密。
func (s *Server) extensionRuntimeConfig(ctx context.Context, assetID int64, sp *assetSpec) (map[string]json.RawMessage, error) {
	if sp.Declarative == nil {
		return nil, nil
	}
	merged := map[string]json.RawMessage{}
	for key, value := range sp.ExtensionConfig {
		merged[key] = value
	}
	enc, err := s.store.ExtensionSecretConfig(ctx, assetID)
	if errors.Is(err, store.ErrNotFound) {
		return merged, nil
	}
	if err != nil {
		return nil, err
	}
	if len(enc) == 0 {
		return merged, nil
	}
	raw, err := s.box.Open(enc)
	if err != nil {
		return nil, fmt.Errorf("解密扩展安装配置（主密钥是否换过？）：%w", err)
	}
	var secrets map[string]json.RawMessage
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&secrets); err != nil {
		return nil, err
	}
	for key, value := range secrets {
		merged[key] = value
	}
	return merged, nil
}

func trimExtensionResult(text string, keep []string) (string, error) {
	if len(keep) == 0 {
		return text, nil
	}
	dec := json.NewDecoder(strings.NewReader(text))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return "", errors.New("声明式 keep 只支持 JSON 响应，不能把未过滤的内容交给模型：" + err.Error())
	}
	var trim func(any) any
	trim = func(x any) any {
		switch v := x.(type) {
		case []any:
			out := make([]any, len(v))
			for i, item := range v {
				out[i] = trim(item)
			}
			return out
		case map[string]any:
			out := map[string]any{}
			for _, key := range keep {
				if item, ok := v[key]; ok {
					out[key] = item
				}
			}
			return out
		default:
			return x
		}
	}
	raw, err := json.Marshal(trim(value))
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func manifestKeep(m *extension.Manifest, toolName string) []string {
	t := manifestTool(m, toolName)
	if t == nil {
		return nil
	}
	return t.Keep
}
