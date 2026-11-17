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
	"github.com/hikariming/dsh-ee/server/internal/i18n"
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

// feishuNotifyText 异步给一名接收者发消息；消息失败只写日志，不改变已经落库的审批/申请结果。
// 文案按「接收者用户语言 → 组织默认语言 → zh」渲染（见 userLanguage）。
func (s *Server) feishuNotifyText(userID int64, key string, params map[string]string) {
	ctx := context.WithoutCancel(context.Background())
	client, _, err := s.feishuNotificationClient(ctx)
	if err != nil || client == nil {
		return
	}
	u, err := s.store.GetUser(ctx, userID)
	if err != nil || u.ExternalDN == "" || u.AccountType != store.AccountHuman || u.Disabled {
		return
	}
	if _, err := client.SendText(ctx, feishuOpenID(u), feishu.Text(s.userLanguage(ctx, u), key, params)); err != nil {
		s.log.Warn("发送飞书结果通知失败", "user_id", userID, "err", err)
	}
}

// notifyFeishuResult 把审批结果发给申请人/作者，语言取接收者自己的偏好。
func (s *Server) notifyFeishuResult(ownerID *int64, title, action string) {
	if ownerID == nil || *ownerID <= 0 {
		return
	}
	key := map[string]string{"approve": feishu.MsgResultApproved, "reject": feishu.MsgResultRejected}[action]
	params := map[string]string{"title": title}
	if key == "" {
		key = feishu.MsgResultOther
		params["status"] = action
	}
	s.feishuNotifyText(*ownerID, key, params)
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
				params := map[string]string{
					"title": b.Title, "query": g.Query, "count": strconv.Itoa(g.Count),
					"id": strconv.FormatInt(b.ID, 10),
				}
				if _, err := client.SendText(ctx, feishuOpenID(u), feishu.Text(s.userLanguage(ctx, u), feishu.MsgKBGap, params)); err != nil {
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

// escapeFeishuMarkdown 转义进入卡片 markdown 正文的用户/管理员自由文本。
// 申请理由、用户名、资产标题等值是员工可写的，卡片却是审批人信任的 bot 消息；
// 不转义就能在正文里造出 [点我](https://evil) 这样的可点链接、![](https://attacker/px)
// 这样会自动加载的图片（钓鱼、IP/UA 泄露），以及伪造的加粗/代码块/标题/引用。
//
// 用反斜杠逐字符转义飞书 markdown 的语法字符：即使渲染器不消费反斜杠，
// 插入的 \ 也会把 "]("、"!["、"**"、"```" 等触发序列拆开，语法不会被解析；
// 换行不转义，保留多行理由的排版。< 与 & 改用 HTML 实体：卡片 markdown
// 支持 <a>/<at>/<font> 等 HTML 标签，反斜杠挡不住标签解析，实体才是飞书
// 官方文档给出的「按字面显示特殊字符」的方式。转义后的文本仍按原样显示。
func escapeFeishuMarkdown(s string) string {
	if s == "" {
		return s
	}
	// 普通文本（不含下面这些字符）原样返回，卡片显示不受影响。
	if !strings.ContainsAny(s, "\\`*_{}[]()#+-.!>|<&~") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '<':
			b.WriteString("&lt;")
		case '&':
			b.WriteString("&amp;")
		case '\\', '`', '*', '_', '{', '}', '[', ']', '(', ')', '#', '+', '-', '.', '!', '>', '|', '~':
			b.WriteByte('\\')
			b.WriteByte(s[i])
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
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

// feishuCardCopy 是一张审批卡的文案键与参数：标题和正文里混着对象数据（技能标题、版本号、网页路径），
// 所以要等确定了收件人语言再替换参数。卡片 JSON 结构不变，只换文本值。
type feishuCardCopy struct {
	TitleKey     string
	TitleParams  map[string]string
	DetailKey    string
	DetailParams map[string]string
	// DetailFreeText 列出 DetailParams 里来自用户/管理员自由填写的参数名。正文是 markdown，
	// 这些值必须先转义；固定文案、枚举、版本号、站内路径等结构化参数不转义。
	// 标题（TitleParams）渲染进 plain_text，不解析 markdown，所以不需要转义。
	DetailFreeText []string
}

// detailText 渲染卡片 markdown 正文。只有 DetailFreeText 里列出的自由文本参数会被转义，
// 消息模板里的固定文案和结构化参数保持原样，避免把模板本身或版本号转义坏。
func (c feishuCardCopy) detailText(lang string) string {
	if len(c.DetailFreeText) == 0 {
		return feishu.Text(lang, c.DetailKey, c.DetailParams)
	}
	params := make(map[string]string, len(c.DetailParams))
	for k, v := range c.DetailParams {
		params[k] = v
	}
	for _, k := range c.DetailFreeText {
		if v, ok := params[k]; ok {
			params[k] = escapeFeishuMarkdown(v)
		}
	}
	return feishu.Text(lang, c.DetailKey, params)
}

func (s *Server) sendFeishuApproval(ctx context.Context, kind, objectKey string, assetID, subID int64, ownerID *int64, copy feishuCardCopy, operators []int64) {
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
		u, err := s.store.GetUser(ctx, operator)
		if err != nil || u.ExternalDN == "" || u.Disabled {
			continue
		}
		// 每张卡片按收件人自己的语言渲染：审批人设了语言就用它的，否则跟组织默认。
		lang := s.userLanguage(ctx, u)
		actionID := secret.Token()
		doc := feishuCardDocument(feishu.Text(lang, copy.TitleKey, copy.TitleParams), copy.detailText(lang))
		actions := doc["body"].(map[string]any)["components"].([]any)
		actions[1] = map[string]any{"tag": "action", "actions": []any{
			feishuButton(feishu.Text(lang, feishu.MsgCardButtonApprove, nil), kind, "approve", actionID),
			feishuButton(feishu.Text(lang, feishu.MsgCardButtonReject, nil), kind, "reject", actionID),
		}}
		doc["body"].(map[string]any)["components"] = actions
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
	copy := feishuCardCopy{
		TitleKey:    feishu.MsgCardHubTitle,
		TitleParams: map[string]string{"title": a.Title},
		DetailKey:   feishu.MsgCardHubDetail,
		DetailParams: map[string]string{
			"kind": a.Kind, "version": strconv.Itoa(version),
			"path": "/assets/" + strconv.FormatInt(a.ID, 10),
		},
	}
	s.sendFeishuApproval(context.WithoutCancel(context.Background()), store.FeishuActionHubVersion, strconv.FormatInt(a.ID, 10)+":"+strconv.Itoa(version), a.ID, int64(version), a.OwnerID, copy, s.feishuReviewers(context.WithoutCancel(context.Background())))
}

func (s *Server) feishuReviewScope(a *store.Asset) {
	if a.PendingScope == nil || a.ScopeRequestedAt == nil {
		return
	}
	copy := feishuCardCopy{
		TitleKey:     feishu.MsgCardScopeTitle,
		TitleParams:  map[string]string{"title": a.Title},
		DetailKey:    feishu.MsgCardScopeDetail,
		DetailParams: map[string]string{"title": a.Title, "path": "/assets/" + strconv.FormatInt(a.ID, 10)},
		DetailFreeText: []string{
			"title", // 资产标题由作者自由填写
		},
	}
	// 卡片动作里带上发卡时那份申请的快照标识（毫秒数，复用 sub_id）：
	// 回调审批时必须和当前 pending_scope 的标识一致，否则点到的是已被替换的旧申请。
	s.sendFeishuApproval(context.WithoutCancel(context.Background()), store.FeishuActionScope, strconv.FormatInt(a.ID, 10)+":scope", a.ID, a.ScopeRequestedAt.UnixMilli(), a.OwnerID, copy, s.feishuReviewers(context.WithoutCancel(context.Background())))
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
	copy := feishuCardCopy{
		TitleKey:    feishu.MsgCardAccessTitle,
		TitleParams: map[string]string{"title": q.Title},
		DetailKey:   feishu.MsgCardAccessDetail,
		DetailParams: map[string]string{
			"applicant": q.Username, "reason": q.Reason, "title": q.Title,
		},
		DetailFreeText: []string{
			"applicant", "reason", "title", // 用户名、申请理由、资产/应用标题都由用户填写
		},
	}
	s.sendFeishuApproval(ctx, store.FeishuActionAccess, strconv.FormatInt(q.ID, 10), q.AssetID, q.ID, ownerPtr(q.UserID), copy, ids)
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
		return viewer{}, feishuErr(feishu.MsgToastOperatorNotProvisioned, nil)
	}
	if u.Disabled || u.AccountType != store.AccountHuman {
		return viewer{}, feishuErr(feishu.MsgToastOperatorDisabled, nil)
	}
	access, err := s.store.UserAccess(ctx, u.ID)
	if err != nil {
		return viewer{}, err
	}
	return viewer{user: u, access: access}, nil
}

// errFeishuForbidden：点按钮的人没有这条审批的权限。卡片留给有权限的人，不作废。
var errFeishuForbidden = errors.New("没有权限")

// feishuMessageError 是带五语消息键的错误：Error() 给日志渲染中文，
// 回给点按钮的人的 toast 按操作者语言渲染（见 feishuErrorText）。
type feishuMessageError struct {
	key    string
	params map[string]string
	// forbidden：这个人没权限，卡片要留着给有权限的人，不能作废。
	forbidden bool
}

func (e *feishuMessageError) Error() string {
	return feishu.Text(i18n.Zh, e.key, e.params)
}

func (e *feishuMessageError) Unwrap() error {
	if e.forbidden {
		return errFeishuForbidden
	}
	return nil
}

// feishuErr 构造一条按语言渲染的用户可见错误。
func feishuErr(key string, params map[string]string) error {
	return &feishuMessageError{key: key, params: params}
}

// feishuForbidden 构造一条「没权限」错误；errors.Is(err, errFeishuForbidden) 仍然成立。
func feishuForbidden(key string) error {
	return &feishuMessageError{key: key, forbidden: true}
}

// feishuErrorText 把错误按语言渲染成 toast 文本；不是五语错误的（数据库、第三方错误）
// 原样返回，保持和原来一样能把原因带给操作人。
func feishuErrorText(lang string, err error) string {
	var msg *feishuMessageError
	if errors.As(err, &msg) {
		return feishu.Text(lang, msg.key, msg.params)
	}
	return err.Error()
}

func (s *Server) applyFeishuDecision(ctx context.Context, v viewer, action *store.FeishuCardAction, decision string) (string, *int64, error) {
	var owner *int64
	switch action.Kind {
	case store.FeishuActionHubVersion:
		a, err := s.store.GetAsset(ctx, action.AssetID)
		if err != nil {
			return "", owner, err
		}
		if !v.manages(a.Kind) {
			return "", owner, feishuForbidden(feishu.MsgToastNoReviewPermAsset)
		}
		to := store.StatusRejected
		if decision == "approve" {
			to = store.StatusPublished
		}
		ok, err := s.store.SetAssetVersionStatus(ctx, a.ID, int(action.SubID), []string{store.StatusPending}, to, v.user.ID, "from_feishu")
		if err != nil || !ok {
			return "", owner, feishuErr(feishu.MsgToastVersionTaken, nil)
		}
		return a.Title, a.OwnerID, nil
	case store.FeishuActionScope:
		a, err := s.store.GetAsset(ctx, action.AssetID)
		if err != nil {
			return "", owner, err
		}
		if !v.manages(a.Kind) {
			return "", owner, feishuForbidden(feishu.MsgToastNoScopePerm)
		}
		// 卡片里记的是发卡时那份申请的快照标识；申请已被负责人替换时 Resolve 返回冲突，
		// 不能拿旧卡片去消费新申请（sub_id 为 0 的升级前旧卡同样按冲突处理，失效关闭）。
		ok, err := s.store.ResolveAssetScope(ctx, a.ID, decision == "approve", "from_feishu", time.UnixMilli(action.SubID).UTC())
		if err != nil || !ok {
			return "", owner, feishuErr(feishu.MsgToastScopeTaken, nil)
		}
		return a.Title, a.OwnerID, nil
	case store.FeishuActionNpmVersion:
		return s.applyFeishuNpmDecision(ctx, v, action, decision)
	case store.FeishuActionAccess:
		q, err := s.store.GetPendingAccessRequest(ctx, action.SubID)
		if err != nil {
			return "", owner, feishuErr(feishu.MsgToastRequestTaken, nil)
		}
		allowed, err := s.canApprove(ctx, v, q)
		if err != nil {
			return "", owner, err
		}
		if !allowed {
			return "", owner, feishuForbidden(feishu.MsgToastNotApprover)
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
		return "", owner, feishuErr(feishu.MsgToastUnknownAction, nil)
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
	// 从这里开始请求确实来自飞书：办不成的原因回给点按钮的人。
	// 还没定位到操作人时（卡片失效/对不上）按组织默认语言回 toast；定位到之后用操作者自己的语言。
	orgLang := s.orgLanguage(r.Context())
	hash := secret.Hash(actionID)
	action, err := s.store.FeishuCardAction(r.Context(), hash)
	if notFound(err) {
		feishuToast(w, "error", feishu.Text(orgLang, feishu.MsgToastCardInvalid, nil))
		return
	}
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	if action.MessageID != event.MessageID {
		feishuToast(w, "error", feishu.Text(orgLang, feishu.MsgToastCardMismatch, nil))
		return
	}
	if action.Status != store.FeishuActionPending {
		feishuToast(w, "info", feishu.Text(orgLang, feishu.MsgToastAlreadyHandled, nil))
		return
	}
	if time.Now().After(action.ExpiresAt) {
		feishuToast(w, "error", feishu.Text(orgLang, feishu.MsgToastCardExpired, nil))
		return
	}
	operator, err := s.feishuOperator(r.Context(), event)
	if err != nil {
		feishuToast(w, "error", feishuErrorText(orgLang, err))
		return
	}
	// 到这个人的语言已经能确定了：后面的 toast 都用它。
	lang := s.userLanguage(r.Context(), operator.user)
	if err := s.store.BeginFeishuCardAction(r.Context(), hash, event.EventID); err != nil {
		feishuToast(w, "info", feishu.Text(lang, feishu.MsgToastAlreadyHandled, nil))
		return
	}
	title, ownerID, err := s.applyFeishuDecision(r.Context(), operator, action, decision)
	if errors.Is(err, errFeishuForbidden) {
		// 回调按飞书要求一律回 200（见 feishuToast），失败只记日志让动作留在 processing 可查；
		// 不改返回码去换飞书重试，避免破坏既有幂等语义和给点按钮的人看的原因。
		if rerr := s.store.ReleaseFeishuCardAction(r.Context(), hash); rerr != nil {
			s.log.Warn("记录飞书卡片动作状态失败", "kind", action.Kind, "object", action.ObjectKey, "status", store.FeishuActionPending, "err", rerr)
		}
		feishuToast(w, "error", feishuErrorText(lang, err))
		return
	}
	if err != nil {
		// 终态写不进去只影响这张卡片后续不能再点（停在 processing），审批本身已经失败，记录后继续告诉用户原因。
		if ferr := s.store.FinishFeishuCardAction(r.Context(), hash, store.FeishuActionFailed, err.Error()); ferr != nil {
			s.log.Warn("记录飞书卡片动作状态失败", "kind", action.Kind, "object", action.ObjectKey, "status", store.FeishuActionFailed, "err", ferr)
		}
		s.log.Warn("飞书审批失败", "kind", action.Kind, "err", err)
		feishuToast(w, "error", feishuErrorText(lang, err))
		return
	}
	if err := s.store.FinishFeishuCardAction(r.Context(), hash, store.FeishuActionDone, decision); err != nil {
		s.internalErr(w, r, err)
		return
	}
	if action.Kind == store.FeishuActionAccess {
		s.auditAs(r, operator.user, "access.request_"+decision, title, map[string]any{"via": "feishu"})
	} else {
		detail := map[string]any{"via": "feishu"}
		if action.Kind == store.FeishuActionScope {
			// 与网页审批一致：记下实际被消费的那份申请的快照标识（卡片动作里校验过的值）。
			detail["requested_at"] = time.UnixMilli(action.SubID).UTC()
		}
		s.auditAs(r, operator.user, "review."+decision, title, detail)
	}
	s.notifyFeishuResult(ownerID, title, decision)
	success := feishu.MsgToastDecisionRejected
	if decision == "approve" {
		success = feishu.MsgToastDecisionApproved
	}
	feishuToast(w, "success", feishu.Text(lang, success, map[string]string{"title": title}))
}
