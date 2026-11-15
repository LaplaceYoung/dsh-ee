package api

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
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/secret"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

var (
	vendorRe    = regexp.MustCompile(`^[a-z0-9-]{1,32}$`)
	modelNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/@+-]{0,127}$`)
)

func validBaseURL(raw string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.RawQuery != "" || u.Fragment != "" {
		return "", false
	}
	return strings.TrimRight(u.String(), "/"), true
}

func cleanModels(ms []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range ms {
		m = strings.TrimSpace(m)
		if m != "" && !seen[m] {
			seen[m] = true
			out = append(out, m)
		}
	}
	return out
}

// ---- 网关请求边界 ----

// maxTokensPerRequest 是一次请求允许申请的最大输出 token 数。
//
// 网关按上游返回的 token 用量记账，而一次请求最终用掉多少只有等生成结束才知道；
// 如果放任客户端把 max_tokens / max_completion_tokens 填成任意大的数，单次调用就
// 可能烧掉一整个月的额度。管理端按「万 token」为单位分配月度额度（典型先给 10 万），
// 这里取 32768：单次请求最多占掉最小典型额度的三分之一，同时覆盖长文生成、长代码
// 和推理模型常见的长输出，正常调用不会受影响。超限直接 400，不做静默截断。
const maxTokensPerRequest = 32768

// maxChoicesPerRequest 是一次请求允许的候选数（OpenAI 的 n 参数）。
// 额度和记账都按一次请求统计，n>1 会成倍放大消耗，且各上游对多候选的支持不一致，
// 因此只接受 n=1（或不传）。
const maxChoicesPerRequest = 1

// completionLimitError 校验生成类参数，返回 OpenAI 风格的错误码和给用户看的文案；
// 参数合法时返回空字符串。body 必须是用 UseNumber 解码出来的，数值是 json.Number。
func completionLimitError(body map[string]any) (code, msg string) {
	for _, field := range []string{"max_tokens", "max_completion_tokens"} {
		v, present, ok := intParam(body, field)
		if !present {
			continue
		}
		switch {
		case !ok || v < 1:
			return "invalid_" + field, field + " 必须是正整数"
		case v > maxTokensPerRequest:
			return field + "_too_large", fmt.Sprintf("%s 超过单次上限 %d，请调小", field, maxTokensPerRequest)
		}
	}
	// OpenAI 兼容客户端可能只传 n；显式 0/负数/非整数都不该转发给上游。
	n, present, ok := intParam(body, "n")
	switch {
	case !present:
		return "", ""
	case !ok || n < 1:
		return "invalid_n", "n 必须是正整数"
	case n > maxChoicesPerRequest:
		return "n_not_supported", fmt.Sprintf("当前不支持 n>%d 的多候选请求，请把 n 设为 %d 或不传", maxChoicesPerRequest, maxChoicesPerRequest)
	}
	return "", ""
}

// intParam 读取 body 里的整数参数。present 表示字段存在且不是 null；
// ok 表示它确实是一个能解析成 int64 的整数。
func intParam(body map[string]any, field string) (v int64, present, ok bool) {
	raw, exists := body[field]
	if !exists || raw == nil {
		return 0, false, false
	}
	num, isNumber := raw.(json.Number)
	if !isNumber {
		return 0, true, false
	}
	v, err := strconv.ParseInt(num.String(), 10, 64)
	if err != nil {
		return 0, true, false
	}
	return v, true, true
}

// ---- 上游 ----

func (s *Server) handleListProviders(w http.ResponseWriter, r *http.Request) {
	ps, err := s.store.ListProviders(r.Context(), false)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"providers": nonNil(ps)})
}

type providerReq struct {
	Name     string `json:"name"`
	Vendor   string `json:"vendor"`
	BaseURL  string `json:"base_url"`
	APIKey   string `json:"api_key"`
	Priority *int   `json:"priority"`
	Enabled  *bool  `json:"enabled"`
	// 可选：顺手建几个对外名与上游名相同的模型
	Models []string `json:"models"`
}

func (s *Server) handleCreateProvider(w http.ResponseWriter, r *http.Request) {
	var req providerReq
	if !readJSON(w, r, &req) {
		return
	}
	base, ok := validBaseURL(req.BaseURL)
	if !ok {
		writeErr(w, http.StatusBadRequest, "base_url 必须是 http(s) 地址，例如 https://api.deepseek.com")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len([]rune(req.Name)) > 64 || req.APIKey == "" {
		writeErr(w, http.StatusBadRequest, "name（不超过 64 个字符）和 api_key 不能为空")
		return
	}
	if req.Vendor == "" {
		req.Vendor = "custom"
	}
	if !vendorRe.MatchString(req.Vendor) {
		writeErr(w, http.StatusBadRequest, "vendor 只能包含小写字母、数字和 -")
		return
	}
	models := cleanModels(req.Models)
	for _, m := range models {
		if !modelNameRe.MatchString(m) {
			writeErr(w, http.StatusBadRequest, "模型名不合法："+m)
			return
		}
	}
	enc, err := s.box.Seal([]byte(req.APIKey))
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	p := &store.Provider{Name: req.Name, Vendor: req.Vendor, BaseURL: base, APIKeyEnc: enc, APIKeyHint: secret.Hint(req.APIKey), Priority: 100, Enabled: true}
	if req.Priority != nil {
		p.Priority = *req.Priority
	}
	if req.Enabled != nil {
		p.Enabled = *req.Enabled
	}
	if err := s.store.CreateProvider(r.Context(), p); err != nil {
		if errors.Is(err, store.ErrDuplicate) {
			writeErr(w, http.StatusConflict, "名称已存在")
			return
		}
		s.internalErr(w, r, err)
		return
	}
	if len(models) > 0 {
		if err := s.store.SyncProviderModels(r.Context(), p.ID, models); err != nil {
			s.internalErr(w, r, err)
			return
		}
	}
	created, err := s.store.GetProvider(r.Context(), p.ID)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	s.audit(r, auditProviderCreate, created.Name, map[string]any{"id": created.ID, "base_url": created.BaseURL, "vendor": created.Vendor})
	s.kickConnect()
	writeJSON(w, http.StatusCreated, created)
}

type updateProviderReq struct {
	Vendor   *string  `json:"vendor"`
	BaseURL  *string  `json:"base_url"`
	APIKey   *string  `json:"api_key"`
	Priority *int     `json:"priority"`
	Enabled  *bool    `json:"enabled"`
	Models   []string `json:"models"` // 兼容旧接口：整体替换为同名模型
}

func (s *Server) handleUpdateProvider(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req updateProviderReq
	if !readJSON(w, r, &req) {
		return
	}
	p := store.ProviderPatch{Priority: req.Priority, Enabled: req.Enabled}
	if req.Vendor != nil {
		if !vendorRe.MatchString(*req.Vendor) {
			writeErr(w, http.StatusBadRequest, "vendor 只能包含小写字母、数字和 -")
			return
		}
		p.Vendor = req.Vendor
	}
	if req.BaseURL != nil {
		base, ok := validBaseURL(*req.BaseURL)
		if !ok {
			writeErr(w, http.StatusBadRequest, "base_url 必须是 http(s) 地址")
			return
		}
		p.BaseURL = &base
	}
	if req.APIKey != nil {
		if *req.APIKey == "" {
			writeErr(w, http.StatusBadRequest, "api_key 不能为空")
			return
		}
		enc, err := s.box.Seal([]byte(*req.APIKey))
		if err != nil {
			s.internalErr(w, r, err)
			return
		}
		hint := secret.Hint(*req.APIKey)
		p.APIKeyEnc, p.APIKeyHint = enc, &hint
	}
	if err := s.store.UpdateProvider(r.Context(), id, p); err != nil {
		if notFound(err) {
			writeErr(w, http.StatusNotFound, "上游不存在")
			return
		}
		s.internalErr(w, r, err)
		return
	}
	if req.Models != nil {
		ms := cleanModels(req.Models)
		if len(ms) == 0 {
			writeErr(w, http.StatusBadRequest, "models 不能为空")
			return
		}
		if err := s.store.SyncProviderModels(r.Context(), id, ms); err != nil {
			s.internalErr(w, r, err)
			return
		}
	}
	pr, err := s.store.GetProvider(r.Context(), id)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	s.audit(r, auditProviderUpdate, pr.Name, map[string]any{"id": id, "base_url": pr.BaseURL, "enabled": pr.Enabled, "key_changed": req.APIKey != nil})
	s.kickConnect()
	writeJSON(w, http.StatusOK, pr)
}

func (s *Server) handleDeleteProvider(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	target := "#" + itoa64(id)
	if pv, err := s.store.GetProvider(r.Context(), id); err == nil {
		target = pv.Name
	}
	if err := s.store.DeleteProvider(r.Context(), id); err != nil {
		if notFound(err) {
			writeErr(w, http.StatusNotFound, "上游不存在")
			return
		}
		s.internalErr(w, r, err)
		return
	}
	s.audit(r, auditProviderDelete, target, map[string]any{"id": id})
	s.kickConnect()
	w.WriteHeader(http.StatusNoContent)
}

// ---- 连通性检查 ----

type checkResult struct {
	OK        bool     `json:"ok"`
	Status    int      `json:"status"`
	LatencyMS int64    `json:"latency_ms"`
	Message   string   `json:"message"`
	Models    []string `json:"models"`
}

// checkProvider 先请求 {base}/models；上游不提供这个接口（404/405）时，
// 用一个已配置的模型发一次 max_tokens=1 的对话做验证。
func (s *Server) checkProvider(ctx context.Context, p *store.Provider, fallbackModel string) checkResult {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	key, err := s.box.Open(p.APIKeyEnc)
	if err != nil {
		return checkResult{Message: "企业 key 解密失败，请重新填写"}
	}
	start := time.Now()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, p.BaseURL+"/models", nil)
	req.Header.Set("Authorization", "Bearer "+string(key))
	resp, err := s.upstream.Do(req)
	res := checkResult{Models: []string{}}
	if err != nil {
		res.Message = "连不上上游：" + shortErr(err)
		return res
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	resp.Body.Close()
	res.Status = resp.StatusCode
	res.LatencyMS = time.Since(start).Milliseconds()

	switch {
	case resp.StatusCode == http.StatusOK:
		var list struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if json.Unmarshal(body, &list) == nil {
			for _, m := range list.Data {
				if m.ID != "" {
					res.Models = append(res.Models, m.ID)
				}
			}
		}
		sort.Strings(res.Models)
		res.OK = true
		res.Message = fmt.Sprintf("连接正常，上游提供 %d 个模型", len(res.Models))
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		res.Message = "鉴权失败：企业 key 无效或没有权限"
	case (resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusMethodNotAllowed) && fallbackModel != "":
		return s.checkByChat(ctx, p, key, fallbackModel)
	default:
		res.Message = fmt.Sprintf("上游返回 HTTP %d：%s", resp.StatusCode, snippet(body))
	}
	return res
}

func (s *Server) checkByChat(ctx context.Context, p *store.Provider, key []byte, model string) checkResult {
	start := time.Now()
	payload, _ := json.Marshal(map[string]any{
		"model": model, "max_tokens": 1, "messages": []map[string]string{{"role": "user", "content": "ping"}},
	})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, p.BaseURL+"/chat/completions", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+string(key))
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.upstream.Do(req)
	res := checkResult{Models: []string{}}
	if err != nil {
		res.Message = "连不上上游：" + shortErr(err)
		return res
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()
	res.Status, res.LatencyMS = resp.StatusCode, time.Since(start).Milliseconds()
	if resp.StatusCode == http.StatusOK {
		res.OK = true
		res.Message = "上游没有模型列表接口，已用 " + model + " 做了一次最小对话验证"
	} else {
		res.Message = fmt.Sprintf("用 %s 验证失败，HTTP %d：%s", model, resp.StatusCode, snippet(body))
	}
	return res
}

func snippet(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len([]rune(s)) > 160 {
		s = string([]rune(s)[:160]) + "…"
	}
	if s == "" {
		return "（无响应内容）"
	}
	return s
}

func shortErr(err error) string {
	var ue *url.Error
	if errors.As(err, &ue) {
		err = ue.Err
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "超时"
	}
	return err.Error()
}

func (s *Server) handleCheckProvider(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	p, err := s.store.GetProvider(r.Context(), id)
	if notFound(err) {
		writeErr(w, http.StatusNotFound, "上游不存在")
		return
	}
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	fallback := ""
	models, err := s.store.ListModels(r.Context())
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	for _, m := range models {
		if m.ProviderID == id {
			fallback = m.UpstreamModel
			break
		}
	}
	res := s.checkProvider(r.Context(), p, fallback)
	if err := s.store.SetProviderCheck(r.Context(), id, res.OK, res.Message); err != nil {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// ---- 模型 ----

func (s *Server) handleListModels(w http.ResponseWriter, r *http.Request) {
	ms, err := s.store.ListModels(r.Context())
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"models": nonNil(ms)})
}

type createModelsReq struct {
	ProviderID int64 `json:"provider_id"`
	// 批量添加：每项的对外名为空时与上游名相同；能力声明可省略，之后在模型编辑里补。
	Items []struct {
		UpstreamModel     string   `json:"upstream_model"`
		Name              string   `json:"name"`
		DisplayName       string   `json:"display_name"`
		SupportsReasoning bool     `json:"supports_reasoning"`
		ReasoningEfforts  []string `json:"reasoning_efforts"`
		InputModalities   []string `json:"input_modalities"`
	} `json:"items"`
}

// reasoningEffortRe 只做结构校验：具体有没有某个强度由上游模型决定，不在这里枚举值。
var reasoningEffortRe = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)

// maxReasoningEffortsLen 与 models.reasoning_efforts 列宽一致（逗号分隔后的总长度）。
const maxReasoningEffortsLen = 128

// normalizeReasoningEfforts 规范管理端填写的思考强度：去空白、转小写、去重并保持顺序。
func normalizeReasoningEfforts(raw []string) ([]string, error) {
	out := make([]string, 0, len(raw))
	seen := map[string]bool{}
	for _, item := range raw {
		item = strings.ToLower(strings.TrimSpace(item))
		if item == "" {
			continue
		}
		if !reasoningEffortRe.MatchString(item) {
			return nil, errors.New("思考强度格式不合法：" + item + "（小写字母开头，只含小写字母、数字、下划线或连字符）")
		}
		if seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	if len(strings.Join(out, ",")) > maxReasoningEffortsLen {
		return nil, errors.New("思考强度列表过长，逗号分隔后不能超过 128 个字符")
	}
	return out, nil
}

// inputModalityOrder 是输入模态的规范顺序；text 是默认值且始终保留。
var inputModalityOrder = []string{"text", "image", "file"}

var inputModalitySet = map[string]bool{"text": true, "image": true, "file": true}

// maxInputModalitiesLen 与 models.input_modalities 列宽一致（逗号分隔后的总长度）。
const maxInputModalitiesLen = 128

// normalizeInputModalities 规范管理端填写的输入模态：小写、去重、按 text/image/file 排序，
// text 始终存在；空表示只接受文本。只做结构校验，不按模型名校验/硬编码模态。
func normalizeInputModalities(raw []string) ([]string, error) {
	seen := map[string]bool{}
	for _, item := range raw {
		item = strings.ToLower(strings.TrimSpace(item))
		if item == "" {
			continue
		}
		if !inputModalitySet[item] {
			return nil, errors.New("输入模态只支持 text / image / file：" + item)
		}
		seen[item] = true
	}
	seen["text"] = true
	out := make([]string, 0, len(inputModalityOrder))
	for _, item := range inputModalityOrder {
		if seen[item] {
			out = append(out, item)
		}
	}
	if len(strings.Join(out, ",")) > maxInputModalitiesLen {
		return nil, errors.New("输入模态列表过长")
	}
	return out, nil
}

func (s *Server) handleCreateModels(w http.ResponseWriter, r *http.Request) {
	var req createModelsReq
	if !readJSON(w, r, &req) {
		return
	}
	if _, err := s.store.GetProvider(r.Context(), req.ProviderID); err != nil {
		if notFound(err) {
			writeErr(w, http.StatusBadRequest, "上游不存在")
			return
		}
		s.internalErr(w, r, err)
		return
	}
	if len(req.Items) == 0 || len(req.Items) > 500 {
		writeErr(w, http.StatusBadRequest, "一次添加 1 到 500 个模型")
		return
	}
	var created []*store.Model
	var skipped []string
	for _, it := range req.Items {
		efforts, err := normalizeReasoningEfforts(it.ReasoningEfforts)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		modalities, err := normalizeInputModalities(it.InputModalities)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		m := &store.Model{
			ProviderID:        req.ProviderID,
			UpstreamModel:     strings.TrimSpace(it.UpstreamModel),
			Name:              strings.TrimSpace(it.Name),
			DisplayName:       strings.TrimSpace(it.DisplayName),
			Enabled:           true,
			SupportsReasoning: it.SupportsReasoning,
			ReasoningEfforts:  efforts,
			InputModalities:   modalities,
		}
		if m.Name == "" {
			m.Name = m.UpstreamModel
		}
		if !modelNameRe.MatchString(m.UpstreamModel) || !modelNameRe.MatchString(m.Name) {
			writeErr(w, http.StatusBadRequest, "模型名不合法："+m.UpstreamModel)
			return
		}
		if err := s.store.CreateModel(r.Context(), m); err != nil {
			if errors.Is(err, store.ErrDuplicate) {
				skipped = append(skipped, m.Name)
				continue
			}
			s.internalErr(w, r, err)
			return
		}
		created = append(created, m)
	}
	if len(created) > 0 {
		names := make([]string, len(created))
		for i, m := range created {
			names[i] = m.Name
		}
		s.audit(r, auditModelChange, strings.Join(names, ", "), map[string]any{"op": "create", "count": len(created)})
		s.kickConnect()
	}
	writeJSON(w, http.StatusCreated, map[string]any{"created": nonNil(created), "skipped": nonNil(skipped)})
}

type updateModelReq struct {
	Name              *string   `json:"name"`
	UpstreamModel     *string   `json:"upstream_model"`
	DisplayName       *string   `json:"display_name"`
	Enabled           *bool     `json:"enabled"`
	SupportsReasoning *bool     `json:"supports_reasoning"`
	ReasoningEfforts  *[]string `json:"reasoning_efforts"`
	InputModalities   *[]string `json:"input_modalities"`
}

func (s *Server) handleUpdateModel(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req updateModelReq
	if !readJSON(w, r, &req) {
		return
	}
	for _, v := range []*string{req.Name, req.UpstreamModel} {
		if v != nil {
			*v = strings.TrimSpace(*v)
			if !modelNameRe.MatchString(*v) {
				writeErr(w, http.StatusBadRequest, "模型名不合法："+*v)
				return
			}
		}
	}
	if req.DisplayName != nil {
		d := strings.TrimSpace(*req.DisplayName)
		req.DisplayName = &d
	}
	var efforts []string
	if req.ReasoningEfforts != nil {
		var err error
		if efforts, err = normalizeReasoningEfforts(*req.ReasoningEfforts); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	var modalities []string
	if req.InputModalities != nil {
		var err error
		if modalities, err = normalizeInputModalities(*req.InputModalities); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	patch := store.ModelPatch{
		Name: req.Name, UpstreamModel: req.UpstreamModel, DisplayName: req.DisplayName, Enabled: req.Enabled,
		SupportsReasoning: req.SupportsReasoning,
	}
	if req.ReasoningEfforts != nil {
		patch.ReasoningEfforts = &efforts
	}
	if req.InputModalities != nil {
		patch.InputModalities = &modalities
	}
	err := s.store.UpdateModel(r.Context(), id, patch)
	switch {
	case notFound(err):
		writeErr(w, http.StatusNotFound, "模型不存在")
		return
	case errors.Is(err, store.ErrDuplicate):
		writeErr(w, http.StatusConflict, "这个上游已有同名的对外模型")
		return
	case err != nil:
		s.internalErr(w, r, err)
		return
	}
	m, err := s.store.GetModel(r.Context(), id)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	s.audit(r, auditModelChange, m.Name, map[string]any{
		"op": "update", "id": id, "upstream_model": m.UpstreamModel, "enabled": m.Enabled,
		"supports_reasoning": m.SupportsReasoning, "reasoning_efforts": m.ReasoningEfforts,
		"input_modalities": m.InputModalities,
	})
	s.kickConnect()
	writeJSON(w, http.StatusOK, m)
}

func (s *Server) handleDeleteModel(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	target := "#" + itoa64(id)
	if m, err := s.store.GetModel(r.Context(), id); err == nil {
		target = m.Name
	}
	if err := s.store.DeleteModel(r.Context(), id); err != nil {
		if notFound(err) {
			writeErr(w, http.StatusNotFound, "模型不存在")
			return
		}
		s.internalErr(w, r, err)
		return
	}
	s.audit(r, auditModelChange, target, map[string]any{"op": "delete", "id": id})
	s.kickConnect()
	w.WriteHeader(http.StatusNoContent)
}
