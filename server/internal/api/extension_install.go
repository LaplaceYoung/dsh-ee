package api

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/hikariming/dsh-ee/server/internal/extension"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

type extensionInstallOption struct {
	Value json.RawMessage `json:"value"`
	Label string          `json:"label"`
}

type extensionInstallField struct {
	Key      string                   `json:"key"`
	Type     string                   `json:"type"`
	Label    string                   `json:"label"`
	Help     string                   `json:"help,omitempty"`
	Required bool                     `json:"required"`
	Default  json.RawMessage          `json:"default,omitempty"`
	Options  []extensionInstallOption `json:"options,omitempty"`
	Enum     []any                    `json:"-"`
	Secret   bool                     `json:"secret,omitempty"`
}

func rawDefault(v json.RawMessage) json.RawMessage {
	if len(v) == 0 || string(v) == "null" {
		return nil
	}
	return append(json.RawMessage(nil), v...)
}

func optionsForParam(p extension.Param) []extensionInstallOption {
	if len(p.Options) > 0 {
		return installOptions(p.Options)
	}
	if len(p.Enum) > 0 {
		raw, _ := json.Marshal(p.Enum)
		return installOptions(raw)
	}
	return nil
}

func installOptions(raw json.RawMessage) []extensionInstallOption {
	var values []json.RawMessage
	if json.Unmarshal(raw, &values) != nil {
		return nil
	}
	out := []extensionInstallOption{}
	for _, value := range values {
		out = append(out, extensionInstallOption{Value: rawDefault(value), Label: fmt.Sprint(jsonValue(value))})
	}
	return out
}

func jsonValue(raw json.RawMessage) any {
	var v any
	_ = json.Unmarshal(raw, &v)
	return v
}

// extensionInstallFields 同时收清单 config 和工具里的 fill:admin；后者不需要作者重复写一份配置字段。
func extensionInstallFields(m *extension.Manifest) []extensionInstallField {
	out := []extensionInstallField{}
	index := map[string]int{}
	add := func(f extensionInstallField) {
		if i, ok := index[f.Key]; ok {
			if out[i].Help == "" {
				out[i].Help = f.Help
			}
			out[i].Required = out[i].Required || f.Required
			if len(out[i].Default) == 0 {
				out[i].Default = f.Default
			}
			return
		}
		index[f.Key] = len(out)
		out = append(out, f)
	}
	for _, c := range m.Config {
		add(extensionInstallField{Key: c.Key, Type: c.Type, Label: c.Label.Any(), Help: c.Help.Any(), Required: c.Required, Default: rawDefault(c.Default), Options: installOptions(c.Options), Secret: c.Type == "secret"})
	}
	for _, t := range m.Tools {
		for _, p := range t.Params {
			if p.Fill != extension.FillAdmin {
				continue
			}
			label := p.Label.Any()
			if label == "" {
				label = p.Description.Any()
			}
			typ := p.Type
			if len(p.Enum) > 0 || len(p.Options) > 0 {
				typ = "select"
			}
			if typ == "integer" {
				typ = "number"
			}
			add(extensionInstallField{Key: p.Name, Type: typ, Label: label, Required: p.Required && len(p.Default) == 0, Default: rawDefault(p.Default), Options: optionsForParam(p), Enum: p.Enum})
		}
	}
	return out
}

type extensionInstallRequest struct {
	Archive      string                     `json:"archive"`
	Server       string                     `json:"server"`
	Slug         string                     `json:"slug,omitempty"`
	Config       map[string]json.RawMessage `json:"config,omitempty"`
	SharedSecret *string                    `json:"shared_secret,omitempty"`
}

func emptyConfigValue(raw json.RawMessage) bool {
	if len(raw) == 0 || string(raw) == "null" {
		return true
	}
	var text string
	return json.Unmarshal(raw, &text) == nil && strings.TrimSpace(text) == ""
}

func validInstallValue(field extensionInstallField, raw json.RawMessage) error {
	if emptyConfigValue(raw) {
		if field.Required {
			return fmt.Errorf("安装配置 %s 必填", field.Key)
		}
		return nil
	}
	var value any
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	if err := dec.Decode(&value); err != nil {
		return fmt.Errorf("安装配置 %s 不是合法 JSON", field.Key)
	}
	ok := true
	switch field.Type {
	case "", "text", "secret", "url", "kb", "app":
		_, ok = value.(string)
	case "select":
		ok = len(field.Options) > 0
	case "boolean":
		_, ok = value.(bool)
	case "number":
		_, ok = value.(json.Number)
	case "array":
		_, ok = value.([]any)
	case "object":
		_, ok = value.(map[string]any)
	}
	if !ok {
		return fmt.Errorf("安装配置 %s 的类型应为 %s", field.Key, field.Type)
	}
	if field.Type == "url" {
		u, err := url.Parse(value.(string))
		if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
			return fmt.Errorf("安装配置 %s 必须是完整且不带账号密码的 http(s) 地址", field.Key)
		}
	}
	if field.Type == "select" {
		allowed := false
		for _, opt := range field.Options {
			if sameExtensionValue(jsonValue(opt.Value), value) {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("安装配置 %s 只能选择清单声明的选项", field.Key)
		}
	}
	if len(field.Enum) > 0 {
		allowed := false
		for _, candidate := range field.Enum {
			if sameExtensionValue(value, candidate) {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("安装配置 %s 不在清单允许的枚举值里", field.Key)
		}
	}
	return nil
}

func resolveExtensionConfig(m *extension.Manifest, input map[string]json.RawMessage) (map[string]json.RawMessage, map[string]json.RawMessage, error) {
	fields := extensionInstallFields(m)
	byKey := map[string]extensionInstallField{}
	for _, f := range fields {
		byKey[f.Key] = f
	}
	for key := range input {
		if _, ok := byKey[key]; !ok {
			return nil, nil, fmt.Errorf("清单没有声明安装配置 %s", key)
		}
	}
	public, secrets := map[string]json.RawMessage{}, map[string]json.RawMessage{}
	for _, f := range fields {
		raw := rawDefault(f.Default)
		if override, ok := input[f.Key]; ok {
			raw = rawDefault(override)
		}
		if err := validInstallValue(f, raw); err != nil {
			return nil, nil, err
		}
		if emptyConfigValue(raw) {
			continue
		}
		if f.Secret {
			secrets[f.Key] = raw
		} else {
			public[f.Key] = raw
		}
	}
	return public, secrets, nil
}

// extensionAssetSpec 把通过检查的清单映射成既有 OpenAPI 连接。身份仍走 #59 的统一实现，不另建一套。
func extensionAssetSpec(res *extension.Result, req extensionInstallRequest) (*assetSpec, error) {
	m := res.Manifest
	if m == nil {
		return nil, errors.New("清单没有解析成功")
	}
	if !slices.Contains(m.Servers, req.Server) {
		return nil, errors.New("请求地址必须是清单 servers 中的一项")
	}
	slug := strings.TrimSpace(req.Slug)
	if slug == "" {
		parts := strings.Split(m.ID, "/")
		slug = parts[len(parts)-1]
	}
	if !slugRe.MatchString(slug) || len(slug) > maxSlug {
		return nil, fmt.Errorf("连接标识 %q 不合法，只能使用小写字母、数字和中划线", slug)
	}
	public, _, err := resolveExtensionConfig(m, req.Config)
	if err != nil {
		return nil, err
	}
	sp := &assetSpec{
		Title: m.Name.Any(), Description: m.Description.Any(), Type: "openapi", URL: strings.TrimRight(req.Server, "/"), OpenAPI: string(res.OpenAPIDoc), TimeoutSec: 30,
		Declarative: m, ExtensionConfig: public, ExtensionDigest: res.Digest,
	}
	sp.Auth = &connAuth{Type: "none"}
	if c := m.Credential; c != nil {
		if c.Shared != nil {
			sp.Auth = &connAuth{Type: c.Shared.Type, Header: c.Shared.Header}
		}
		if c.Mode == extension.CredUserSecret || c.Mode == extension.CredEEJWT {
			ua := &connUserAuth{Mode: c.Mode, Header: c.Header, Audience: c.Audience, Claims: c.Claims, Hint: c.Hint.Any()}
			sp.UserAuth = ua
		}
	}
	if msg := validateSpec(store.KindConnection, sp); msg != "" {
		return nil, errors.New(msg)
	}
	return sp, nil
}

// POST /api/admin/extensions/install：预检 → 配置校验 → 建连接与安装记录 → 保存凭据 → 生成工具。
func (s *Server) handleExtensionInstall(w http.ResponseWriter, r *http.Request) {
	var req extensionInstallRequest
	if !readJSONLimit(w, r, &req, 12<<20) {
		return
	}
	archive, err := decodeBase64(req.Archive)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "扩展 ZIP 的 base64 编码不对："+err.Error())
		return
	}
	arc, err := readExtensionArchive(archive)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	res := arc.check()
	if res.Manifest == nil || !res.OK {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "扩展检查未通过，不能安装", "check": res})
		return
	}
	if res.Manifest.Credential != nil && res.Manifest.Credential.Shared != nil && (req.SharedSecret == nil || strings.TrimSpace(*req.SharedSecret) == "") {
		writeErr(w, http.StatusBadRequest, "这个扩展需要共享凭据")
		return
	}
	sp, err := extensionAssetSpec(res, req)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	v := viewerOf(r)
	a := &store.Asset{Kind: store.KindConnection, Slug: strings.TrimSpace(req.Slug), Title: sp.Title, Description: sp.Description, Category: "extension", OwnerID: &v.user.ID, Scope: store.Scope{All: true}, Delivery: store.DeliveryAuto}
	if a.Slug == "" {
		parts := strings.Split(res.Manifest.ID, "/")
		a.Slug = parts[len(parts)-1]
	}
	rawSpec, _ := json.Marshal(sp)
	if err := s.store.CreateAsset(r.Context(), a, rawSpec, store.StatusPublished, v.user.ID); err != nil {
		if errors.Is(err, store.ErrDuplicate) {
			writeErr(w, http.StatusConflict, "连接标识已存在："+a.Slug)
			return
		}
		s.internalErr(w, r, err)
		return
	}
	cleanup := func() { _ = s.store.DeleteAsset(r.Context(), a.ID) }
	_, secrets, err := resolveExtensionConfig(res.Manifest, req.Config)
	if err != nil {
		cleanup()
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var secretEnc []byte
	if len(secrets) > 0 {
		raw, _ := json.Marshal(secrets)
		secretEnc, err = s.box.Seal(raw)
		if err != nil {
			cleanup()
			s.internalErr(w, r, err)
			return
		}
	}
	publicRaw, _ := json.Marshal(sp.ExtensionConfig)
	in := &store.ExtensionInstallation{AssetID: a.ID, ID: res.Manifest.ID, Version: res.Manifest.Version, Digest: res.Digest, Kind: res.Manifest.Kind,
		Manifest: string(arc.manifestRaw), OpenAPI: string(arc.openapiRaw), Config: string(publicRaw), SecretEnc: secretEnc, CreatedBy: &v.user.ID}
	if err := s.store.CreateExtensionInstallation(r.Context(), in); err != nil {
		cleanup()
		if errors.Is(err, store.ErrDuplicate) {
			writeErr(w, http.StatusConflict, "这个扩展已经安装："+res.Manifest.Identity())
			return
		}
		s.internalErr(w, r, err)
		return
	}
	if req.SharedSecret != nil && strings.TrimSpace(*req.SharedSecret) != "" {
		if err := s.saveConnSecret(r, a.ID, *req.SharedSecret); err != nil {
			cleanup()
			s.internalErr(w, r, err)
			return
		}
	}
	resp := map[string]any{"asset": a, "extension": map[string]any{"id": res.Manifest.ID, "version": res.Manifest.Version, "digest": res.Digest}}
	base, err := s.connectBase(r.Context(), r)
	if err != nil {
		resp["sync_error"] = err.Error()
	} else if n, err := s.syncConnectionTools(r.Context(), a.ID, v, base); err != nil {
		resp["sync_error"] = err.Error()
	} else {
		resp["tools"] = n
	}
	s.audit(r, auditExtensionInstall, res.Manifest.Identity(), map[string]any{"asset_id": a.ID, "digest": res.Digest, "server": sp.URL})
	s.hub.notify()
	writeJSON(w, http.StatusCreated, resp)
}

func decodeBase64(raw string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(strings.TrimSpace(raw))
}
