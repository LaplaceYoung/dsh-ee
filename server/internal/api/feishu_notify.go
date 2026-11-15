package api

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/feishu"
	"github.com/hikariming/dsh-ee/server/internal/oidc"
	"github.com/hikariming/dsh-ee/server/internal/rbac"
	"github.com/hikariming/dsh-ee/server/internal/secret"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

const feishuCardTTL = 30 * time.Minute

func (s *Server) feishuNotificationClient(ctx context.Context) (*feishu.Client, *oidc.Config, error) {
	cfg, err := s.loadOIDC(ctx)
	if err != nil {
		return nil, nil, err
	}
	if !cfg.Enabled || cfg.Preset != "feishu" || !cfg.FeishuNotify {
		return nil, cfg, nil
	}
	if cfg.FeishuAPIBase == "" || cfg.ClientSecret == "" {
		return nil, nil, errors.New("开启飞书通知需要填写通讯录 API 地址与 App Secret")
	}
	return feishu.NewClient(cfg.FeishuAPIBase, cfg.ClientID, cfg.ClientSecret, s.oidcHTTP().HTTP), cfg, nil
}

func feishuOpenID(u *store.User) string { return strings.TrimPrefix(u.ExternalDN, "feishu:") }

// feishuNotifyText 异步发结果；消息失败只写日志，不改变已经落库的审批/申请结果。
func (s *Server) feishuNotifyText(userID int64, markdown string) {
	client, cfg, err := s.feishuNotificationClient(context.WithoutCancel(context.Background()))
	if err != nil || client == nil {
		return
	}
	u, err := s.store.GetUser(context.WithoutCancel(context.Background()), userID)
	if err != nil || u.ExternalDN == "" || u.AccountType != store.AccountHuman || u.Disabled {
		return
	}
	_ = cfg
	if _, err := client.SendText(context.WithoutCancel(context.Background()), feishuOpenID(u), markdown); err != nil {
		s.log.Warn("发送飞书结果通知失败", "user_id", userID, "err", err)
	}
}

func (s *Server) notifyFeishuResult(ownerID *int64, title, action string) {
	if ownerID == nil || *ownerID <= 0 {
		return
	}
	text := map[string]string{"approve": "已通过", "reject": "已驳回"}[action]
	if text == "" {
		text = action
	}
	s.feishuNotifyText(*ownerID, "审批结果："+title+" · "+text)
}

// feishuNotifyKBGaps 在组织同步节奏上通知真正待补的检索词。通知位置与 g.LastAt 绑定，重启后不重复轰炸维护者。
func (s *Server) feishuNotifyKBGaps(ctx context.Context) {
	client, _, err := s.feishuNotificationClient(ctx)
	if err != nil || client == nil {
		return
	}
	bases, err := s.store.ListKBases(ctx)
	if err != nil {
		return
	}
	ids := make([]int64, len(bases))
	for i, b := range bases {
		ids[i] = b.ID
	}
	computed, err := s.kbGaps(ctx, ids)
	if err != nil {
		s.log.Warn("汇总飞书待补文档提醒失败", "err", err)
		return
	}
	previous := map[string]time.Time{}
	_, _ = s.store.GetSettingJSON(ctx, "feishu_gap_notices", &previous)
	changed := false
	for _, b := range bases {
		for _, g := range computed[b.ID] {
			if !g.LastAt.After(previous[strconv.FormatInt(b.ID, 10)+":"+g.Key]) {
				continue
			}
			users, err := s.store.ListUsers(ctx)
			if err != nil {
				continue
			}
			for _, u := range users {
				access, err := s.store.UserAccess(ctx, u.ID)
				maintains := err == nil && (access.Has(rbac.KnowledgeManage) || b.Maintainers.Allows(u.ID, access.RoleIDs))
				if !maintains || u.Disabled || u.ExternalDN == "" {
					continue
				}
				text := "知识库「" + b.Title + "」有待补文档：员工检索「" + g.Query + "」没有找到答案（" + strconv.Itoa(g.Count) + " 次）。请到 /knowledge/" + strconv.FormatInt(b.ID, 10) + " 处理。"
				if _, err := client.SendText(ctx, feishuOpenID(u), text); err != nil {
					s.log.Warn("发送飞书待补文档提醒失败", "kb", b.Slug, "err", err)
					continue
				}
				previous[strconv.FormatInt(b.ID, 10)+":"+g.Key] = g.LastAt
				changed = true
			}
		}
	}
	if changed {
		_ = s.store.PutSettingJSON(ctx, "feishu_gap_notices", previous, 0)
	}
}

func feishuCardDocument(title, detail string) map[string]any {
	return map[string]any{
		"schema": "2.0", "config": map[string]any{"width_mode": "compact"},
		"header": map[string]any{"template": "blue", "title": map[string]any{"tag": "plain_text", "content": title}},
		"body": map[string]any{"direction": "vertical", "components": []any{
			map[string]any{"tag": "markdown", "content": detail},
			map[string]any{"tag": "action", "actions": []any{}},
		}},
	}
}

func feishuButton(label, kind, decision, actionID string) map[string]any {
	buttonType := "default"
	if decision == "approve" {
		buttonType = "primary"
	}
	if decision == "reject" {
		buttonType = "danger"
	}
	if kind == "link" {
		buttonType = "default"
	}
	return map[string]any{
		"tag": "button", "text": map[string]any{"tag": "plain_text", "content": label}, "type": buttonType,
		"value":     map[string]any{"action_id": actionID, "decision": decision},
		"behaviors": []any{map[string]any{"type": "callback"}},
	}
}

func (s *Server) sendFeishuApproval(ctx context.Context, kind, objectKey string, assetID, subID int64, ownerID *int64, title, detail string, operators []int64) {
	client, _, err := s.feishuNotificationClient(ctx)
	if err != nil || client == nil {
		return
	}
	cfg, err := s.loadOIDC(ctx)
	if err != nil || !cfg.FeishuNotifyActions {
		return
	}
	// 真正回调批准之前先尝试发卡片。卡片 id 是回调要核对的另一个边界，不能只按 object key 接受。
	// 多审批人要全部登记完再一次性写入；逐条写会把前一个人的卡片标记成 superseded。
	var cards []store.FeishuCardActionInput
	for _, operator := range operators {
		if operator <= 0 {
			continue
		}
		actionID := secret.Token()
		doc := feishuCardDocument(title, detail)
		actions := doc["body"].(map[string]any)["components"].([]any)
		actions[1] = map[string]any{"tag": "action", "actions": []any{
			feishuButton("通过", kind, "approve", actionID),
			feishuButton("驳回", kind, "reject", actionID),
		}}
		doc["body"].(map[string]any)["components"] = actions
		u, err := s.store.GetUser(ctx, operator)
		if err != nil || u.ExternalDN == "" || u.Disabled {
			continue
		}
		messageID, err := client.SendInteractive(ctx, feishuOpenID(u), doc)
		if err != nil {
			s.log.Warn("发送飞书审批卡失败", "kind", kind, "object", objectKey, "err", err)
			continue
		}
		cards = append(cards, store.FeishuCardActionInput{ActionHash: secret.Hash(actionID), MessageID: messageID, ExpiresAt: time.Now().Add(feishuCardTTL)})
	}
	if len(cards) > 0 {
		if err := s.store.SendFeishuCardActions(ctx, kind, objectKey, assetID, subID, cards); err != nil {
			s.log.Warn("登记飞书审批动作失败", "kind", kind, "object", objectKey, "err", err)
		}
	}
}

func (s *Server) feishuReviewers(ctx context.Context) []int64 {
	users, err := s.store.ListUsers(ctx)
	if err != nil {
		return nil
	}
	out := []int64{}
	for _, u := range users {
		a, err := s.store.UserAccess(ctx, u.ID)
		if err == nil && a.Has(rbac.PluginsManage) && !u.Disabled && u.ExternalDN != "" {
			out = append(out, u.ID)
		}
	}
	return out
}

// 审批卡的发出：运营事件先调用这里。hub_version 对应一个版本，scope_request 对应待扩大的范围。
func (s *Server) feishuReviewAsset(a *store.Asset, version int) {
	if a.Kind != store.KindSkill && a.Kind != store.KindPrompt && a.Kind != store.KindApp {
		return // 本批先覆盖资产与范围；npm 插件审核留到下一批。
	}
	title := a.Title + " · 待审核"
	detail := "类型：" + a.Kind + "\n版本：v" + strconv.Itoa(version) + "\n请核对改动后决定。网页：" + "/assets/" + strconv.FormatInt(a.ID, 10)
	s.sendFeishuApproval(context.WithoutCancel(context.Background()), store.FeishuActionHubVersion, strconv.FormatInt(a.ID, 10)+":"+strconv.Itoa(version), a.ID, int64(version), a.OwnerID, title, detail, s.feishuReviewers(context.WithoutCancel(context.Background())))
}

func (s *Server) feishuReviewScope(a *store.Asset) {
	if a.PendingScope == nil {
		return
	}
	detail := "申请扩大「" + a.Title + "」的可用范围。网页：" + "/assets/" + strconv.FormatInt(a.ID, 10)
	s.sendFeishuApproval(context.WithoutCancel(context.Background()), store.FeishuActionScope, strconv.FormatInt(a.ID, 10)+":scope", a.ID, 0, a.OwnerID, a.Title+" · 范围分享申请", detail, s.feishuReviewers(context.WithoutCancel(context.Background())))
}

func (s *Server) feishuAccessRequest(ctx context.Context, q *store.AccessRequest) {
	users, err := s.store.ListUsers(ctx)
	if err != nil {
		return
	}
	var reviewers []int64
	for _, u := range users {
		access, lookupErr := s.store.UserAccess(ctx, u.ID)
		if lookupErr != nil || u.Disabled || u.ExternalDN == "" {
			continue
		}
		allowed, approvalErr := s.canApprove(ctx, viewer{user: u, access: access}, q)
		if approvalErr == nil && allowed {
			reviewers = append(reviewers, u.ID)
		}
	}
	ids := []int64{}
	for _, r := range reviewers {
		ids = append(ids, r)
	}
	detail := "申请人：" + q.Username + "\n用途：" + q.Reason + "\n请确认是否开放「" + q.Title + "」的必要权限。"
	s.sendFeishuApproval(ctx, store.FeishuActionAccess, strconv.FormatInt(q.ID, 10), q.AssetID, q.ID, ownerPtr(q.UserID), q.Title+" · 权限申请", detail, ids)
}

func ownerPtr(id int64) *int64 { return &id }

// GET /api/admin/auth/feishu/workspace-url?next=/a/<slug>：给开发者配置工作台首页的入口。
// 飞书用户会经标准 OIDC 回到这个入口并建立网页会话；next 不由 Feishu 或模型控制，本服务只接受站内路径。
func (s *Server) handleFeishuWorkspaceURL(w http.ResponseWriter, r *http.Request) {
	next := strings.TrimSpace(r.URL.Query().Get("next"))
	if next == "" {
		next = "/go"
	}
	if next != "/go" && !strings.HasPrefix(next, "/a/") {
		writeErr(w, http.StatusBadRequest, "飞书工作台链接只能打开 /a/<应用标识>，或 /go")
		return
	}
	if webNext(next) == "" {
		writeErr(w, http.StatusBadRequest, "next 不是合法的站内应用路径")
		return
	}
	cfg, ok := s.oidcEnabled(r.Context())
	if !ok || cfg.Preset != "feishu" {
		writeErr(w, http.StatusBadRequest, "请先开启飞书统一登录")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"url": s.publicBase(r) + "/api/auth/oidc/start?next=" + url.QueryEscape(next), "next": next})
}

// feishuCardEvent 是回调里用得上的几项，从飞书的 schema 2.0 回调里取出来。
type feishuCardEvent struct {
	Type        string
	EventID     string
	OperatorID  string
	MessageID   string
	ActionTag   string
	ActionValue json.RawMessage
}

// feishuCallback 是飞书发来的回调原文（card.action.trigger，schema 2.0）。
// 配置回调地址时飞书先发一次 url_verification（type / token / challenge 在最外层）。
// 见 https://open.feishu.cn/document/feishu-cards/card-callback-communication
type feishuCallback struct {
	Type      string `json:"type"`
	Token     string `json:"token"`
	Challenge string `json:"challenge"`
	Encrypt   string `json:"encrypt"`
	Header    struct {
		EventID   string `json:"event_id"`
		Token     string `json:"token"`
		EventType string `json:"event_type"`
		AppID     string `json:"app_id"`
	} `json:"header"`
	Event struct {
		Operator struct {
			OpenID string `json:"open_id"`
		} `json:"operator"`
		Action struct {
			Tag   string          `json:"tag"`
			Value json.RawMessage `json:"value"`
		} `json:"action"`
		Context struct {
			OpenMessageID string `json:"open_message_id"`
		} `json:"context"`
	} `json:"event"`
}

// feishuTokenOK 用常数时间比对 Verification Token。没配置的一律不过。
func feishuTokenOK(want, got string) bool {
	return want != "" && subtle.ConstantTimeCompare([]byte(want), []byte(got)) == 1
}

func feishuEventAction(raw json.RawMessage) (actionID, decision string, err error) {
	var value struct {
		ActionID string `json:"action_id"`
		Decision string `json:"decision"`
	}
	var direct any
	if err := json.Unmarshal(raw, &direct); err != nil {
		return "", "", errors.New("action_value 不是合法 JSON")
	}
	if text, ok := direct.(string); ok {
		raw = json.RawMessage(text)
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", "", errors.New("action_value 结构无法解析")
	}
	return strings.TrimSpace(value.ActionID), strings.TrimSpace(value.Decision), nil
}

func (s *Server) feishuOperator(ctx context.Context, event feishuCardEvent) (viewer, error) {
	u, err := s.store.GetUserByExternalDN(ctx, store.AuthOIDC, feishu.ExternalSubject(event.OperatorID))
	if err != nil {
		return viewer{}, errors.New("飞书回调操作人未在 dsh-ee 开通账号")
	}
	if u.Disabled || u.AccountType != store.AccountHuman {
		return viewer{}, errors.New("飞书回调操作人已被禁用")
	}
	access, err := s.store.UserAccess(ctx, u.ID)
	if err != nil {
		return viewer{}, err
	}
	return viewer{user: u, access: access}, nil
}

// errFeishuForbidden：点按钮的人没有这条审批的权限。卡片留给有权限的人，不作废。
var errFeishuForbidden = errors.New("没有权限")

func (s *Server) applyFeishuDecision(ctx context.Context, v viewer, action *store.FeishuCardAction, decision string) (string, *int64, error) {
	var owner *int64
	switch action.Kind {
	case store.FeishuActionHubVersion:
		a, err := s.store.GetAsset(ctx, action.AssetID)
		if err != nil {
			return "", owner, err
		}
		if !v.manages(a.Kind) {
			return "", owner, fmt.Errorf("%w：你没有审核这个版本的权限", errFeishuForbidden)
		}
		to := store.StatusRejected
		if decision == "approve" {
			to = store.StatusPublished
		}
		ok, err := s.store.SetAssetVersionStatus(ctx, a.ID, int(action.SubID), []string{store.StatusPending}, to, v.user.ID, "from_feishu")
		if err != nil || !ok {
			return "", owner, errors.New("这个版本已被其他人处理或不是待审核状态")
		}
		return a.Title, a.OwnerID, nil
	case store.FeishuActionScope:
		a, err := s.store.GetAsset(ctx, action.AssetID)
		if err != nil {
			return "", owner, err
		}
		if !v.manages(a.Kind) {
			return "", owner, fmt.Errorf("%w：你没有处理范围分享的权限", errFeishuForbidden)
		}
		ok, err := s.store.ResolveAssetScope(ctx, a.ID, decision == "approve", "from_feishu")
		if err != nil || !ok {
			return "", owner, errors.New("范围分享已被其他人处理")
		}
		return a.Title, a.OwnerID, nil
	case store.FeishuActionNpmVersion:
		return s.applyFeishuNpmDecision(ctx, v, action, decision)
	case store.FeishuActionAccess:
		q, err := s.store.GetPendingAccessRequest(ctx, action.SubID)
		if err != nil {
			return "", owner, errors.New("这条申请已被其他人处理")
		}
		allowed, err := s.canApprove(ctx, v, q)
		if err != nil {
			return "", owner, err
		}
		if !allowed {
			return "", owner, fmt.Errorf("%w：你不是这条申请的审批人", errFeishuForbidden)
		}
		// 和网页审批同一条规则：先用 CAS 把 pending 改成终态抢占，抢到了才去授权。
		// 每个审批人各有一张卡片，所以「飞书 reject + 网页 approve」同时到是真实场景，不能先授权后改状态。
		status := "rejected"
		if decision == "approve" {
			status = "approved"
		}
		if err := s.store.ResolveAccessRequest(ctx, q.ID, status, v.user.ID, "from_feishu"); err != nil {
			if notFound(err) {
				return "", owner, errors.New("这条申请已被其他人处理")
			}
			return "", owner, err
		}
		// 抢占成功后授权；授权失败退回待处理，卡片留失败终态并说明可以重试。
		if decision == "approve" {
			if err := s.grantAccess(ctx, q); err != nil {
				if rerr := s.store.ReopenAccessRequest(ctx, q.ID); rerr != nil {
					s.log.Error("飞书审批授权失败后退回申请失败", "id", q.ID, "err", rerr)
				}
				return "", owner, fmt.Errorf("授权失败，申请已退回待处理，可以重试：%w", err)
			}
		}
		return q.Title, ownerPtr(q.UserID), nil
	default:
		return "", owner, errors.New("未知的审批动作")
	}
}

// feishuDecrypt 解开加密的回调：密钥是 Encrypt Key 的 SHA-256，AES-256-CBC，密文前 16 字节是 IV，PKCS#7 填充。
// 见 https://open.feishu.cn/document/server-docs/event-subscription-guide/event-subscription-configure-/encrypt-key-encryption-configuration-case
func feishuDecrypt(encryptKey, encrypted string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil || len(raw) < 2*aes.BlockSize || len(raw)%aes.BlockSize != 0 {
		return nil, errors.New("密文格式不对")
	}
	key := sha256.Sum256([]byte(encryptKey))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	plain := make([]byte, len(raw)-aes.BlockSize)
	cipher.NewCBCDecrypter(block, raw[:aes.BlockSize]).CryptBlocks(plain, raw[aes.BlockSize:])
	pad := int(plain[len(plain)-1])
	if pad == 0 || pad > aes.BlockSize || pad > len(plain) {
		return nil, errors.New("解不开：Encrypt Key 不对")
	}
	for _, b := range plain[len(plain)-pad:] {
		if int(b) != pad {
			return nil, errors.New("解不开：Encrypt Key 不对")
		}
	}
	return plain[:len(plain)-pad], nil
}

// feishuSignatureOK 核对飞书的请求签名：sha256(timestamp + nonce + Encrypt Key + 原始请求体) 的十六进制。
func feishuSignatureOK(r *http.Request, encryptKey string, body []byte) bool {
	h := sha256.New()
	h.Write([]byte(r.Header.Get("X-Lark-Request-Timestamp") + r.Header.Get("X-Lark-Request-Nonce") + encryptKey))
	h.Write(body)
	want := hex.EncodeToString(h.Sum(nil))
	return subtle.ConstantTimeCompare([]byte(want), []byte(strings.ToLower(r.Header.Get("X-Lark-Signature")))) == 1
}

// feishuToast 是给点按钮的人看的结果。飞书要求回调一律回 HTTP 200，内容只有 toast；
// 回别的状态码，飞书客户端只会显示一句通用的「出错了」，看不到原因。
func feishuToast(w http.ResponseWriter, kind, content string) {
	writeJSON(w, http.StatusOK, map[string]any{"toast": map[string]any{"type": kind, "content": content}})
}

// POST /api/auth/feishu/card：卡片操作回调。无需登录网页，但要先确认请求是飞书发来的
// （Verification Token；填了 Encrypt Key 的还要能解开、签名对得上），
// 之后 action id、消息号、操作者、当前权限、状态和过期都要通过。
// 来源不对的回 4xx；来源对了以后，审批办不成的原因用 toast 告诉点按钮的人。
func (s *Server) handleFeishuCard(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "飞书回调读不了")
		return
	}
	// 飞书事件还有 create_time / tenant_key / host 等附加字段，不能用 DisallowUnknownFields 拒掉。
	var cb feishuCallback
	if err := json.Unmarshal(body, &cb); err != nil {
		writeErr(w, http.StatusBadRequest, "飞书回调 JSON 格式错误")
		return
	}
	cfg, err := s.loadOIDC(r.Context())
	if err != nil || !cfg.Enabled || cfg.Preset != "feishu" || !cfg.FeishuNotifyActions {
		writeErr(w, http.StatusNotFound, "飞书卡片回调未开启")
		return
	}
	// 这是公开接口：先确认请求是飞书发来的，再看内容。操作人身份取自回调，来源不对就不能信
	switch {
	case cb.Encrypt != "" && cfg.FeishuEncryptKey == "":
		writeErr(w, http.StatusBadRequest, "回调是加密的：请在 dsh-ee 的飞书设置里填上 Encrypt Key，或在飞书开发者后台清空它")
		return
	case cb.Encrypt == "" && cfg.FeishuEncryptKey != "":
		// 配了 Encrypt Key 飞书就只发加密的；明文的不是飞书发的
		writeErr(w, http.StatusForbidden, "回调没有加密，不是这个飞书应用发来的")
		return
	case cb.Encrypt != "":
		// 带了签名的要对得上（配置回调地址时的验证请求不带签名）
		if r.Header.Get("X-Lark-Signature") != "" && !feishuSignatureOK(r, cfg.FeishuEncryptKey, body) {
			writeErr(w, http.StatusForbidden, "回调签名不对")
			return
		}
		plain, err := feishuDecrypt(cfg.FeishuEncryptKey, cb.Encrypt)
		if err != nil {
			writeErr(w, http.StatusForbidden, "回调解不开：Encrypt Key 和飞书后台的不一致")
			return
		}
		cb = feishuCallback{}
		if err := json.Unmarshal(plain, &cb); err != nil {
			writeErr(w, http.StatusForbidden, "回调解不开：Encrypt Key 和飞书后台的不一致")
			return
		}
	}
	if cb.Type == "url_verification" {
		if !feishuTokenOK(cfg.FeishuVerificationToken, cb.Token) {
			writeErr(w, http.StatusForbidden, "Verification Token 不对")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"challenge": cb.Challenge})
		return
	}
	if !feishuTokenOK(cfg.FeishuVerificationToken, cb.Header.Token) || (cb.Header.AppID != "" && cb.Header.AppID != cfg.ClientID) {
		writeErr(w, http.StatusForbidden, "回调不是这个飞书应用发来的（Verification Token 或 App ID 不对）")
		return
	}
	event := feishuCardEvent{
		Type: cb.Header.EventType, EventID: cb.Header.EventID, OperatorID: cb.Event.Operator.OpenID,
		MessageID: cb.Event.Context.OpenMessageID, ActionTag: cb.Event.Action.Tag, ActionValue: cb.Event.Action.Value,
	}
	if event.Type != "card.action.trigger" || event.ActionTag != "button" || event.OperatorID == "" || event.MessageID == "" || event.EventID == "" {
		writeErr(w, http.StatusBadRequest, "不是有效的卡片按钮事件")
		return
	}
	actionID, decision, actionErr := feishuEventAction(event.ActionValue)
	if actionErr != nil || actionID == "" || (decision != "approve" && decision != "reject") {
		writeErr(w, http.StatusBadRequest, "这不是 dsh-ee 发出的审批按钮")
		return
	}
	// 从这里开始请求确实来自飞书：办不成的原因回给点按钮的人
	hash := secret.Hash(actionID)
	action, err := s.store.FeishuCardAction(r.Context(), hash)
	if notFound(err) {
		feishuToast(w, "error", "这张卡片已经失效，请到 dsh-ee 网页上处理")
		return
	}
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	if action.MessageID != event.MessageID {
		feishuToast(w, "error", "卡片和审批对不上，请到 dsh-ee 网页上处理")
		return
	}
	if action.Status != store.FeishuActionPending {
		feishuToast(w, "info", "这条审批已经处理过了")
		return
	}
	if time.Now().After(action.ExpiresAt) {
		feishuToast(w, "error", "这张卡片已经过期，请到 dsh-ee 网页上处理")
		return
	}
	operator, err := s.feishuOperator(r.Context(), event)
	if err != nil {
		feishuToast(w, "error", err.Error())
		return
	}
	if err := s.store.BeginFeishuCardAction(r.Context(), hash, event.EventID); err != nil {
		feishuToast(w, "info", "这条审批已经处理过了")
		return
	}
	title, ownerID, err := s.applyFeishuDecision(r.Context(), operator, action, decision)
	if errors.Is(err, errFeishuForbidden) {
		_ = s.store.ReleaseFeishuCardAction(r.Context(), hash)
		feishuToast(w, "error", err.Error())
		return
	}
	if err != nil {
		_ = s.store.FinishFeishuCardAction(r.Context(), hash, store.FeishuActionFailed, err.Error())
		s.log.Warn("飞书审批失败", "kind", action.Kind, "err", err)
		feishuToast(w, "error", err.Error())
		return
	}
	if err := s.store.FinishFeishuCardAction(r.Context(), hash, store.FeishuActionDone, decision); err != nil {
		s.internalErr(w, r, err)
		return
	}
	if action.Kind == store.FeishuActionAccess {
		s.auditAs(r, operator.user, "access.request_"+decision, title, map[string]any{"via": "feishu"})
	} else {
		s.auditAs(r, operator.user, "review."+decision, title, map[string]any{"via": "feishu"})
	}
	s.notifyFeishuResult(ownerID, title, decision)
	feishuToast(w, "success", map[string]string{"approve": "已通过", "reject": "已驳回"}[decision]+"："+title)
}
