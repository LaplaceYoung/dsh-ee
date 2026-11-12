package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/hikariming/dsh-ee/server/internal/rbac"
	"github.com/hikariming/dsh-ee/server/internal/registry"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// npmCardObject 是可变状态很少的对象键：一个包内，版本号不可覆写、不可重复发布。
// 右侧的 @ 才是包名与 semver 的分隔点；@scope 里的 @ 属于包名。
func npmCardObject(name, version string) string { return name + "@" + version }

func npmCardObjectParts(key string) (name, version string, err error) {
	at := strings.LastIndex(key, "@")
	if at <= 0 || at+1 >= len(key) {
		return "", "", errors.New("插件审批对象不是 包名@版本")
	}
	name, version = key[:at], key[at+1:]
	if !registry.ValidName(name) || !registry.ValidVersion(version) {
		return "", "", errors.New("插件审批对象中的包名或版本号不合法")
	}
	return name, version, nil
}

// feishuNpmPublish 在 npm CLI 与网页上传提交同一个待审版本时共享卡片入口。
// 发送失败只记录日志，不影响 registry.Publish 已经返回的发布结果。
func (s *Server) feishuNpmPublish(r *http.Request, result *registry.PublishResult, publisherID int64) {
	if r == nil || result == nil || result.Status != store.StatusPending || publisherID <= 0 {
		return
	}
	ctx := context.WithoutCancel(r.Context())
	publisher := "未知发布者"
	if u, err := s.store.GetUser(ctx, publisherID); err == nil {
		publisher = u.Username
	}
	nameAndVersion := npmCardObject(result.Name, result.Version)
	detail := "包名：" + result.Name + "\n版本：" + result.Version + "\n发布者：" + publisher + "\n访问：" + s.publicBase(r) + "/plugins/" + url.PathEscape(result.Name)
	s.sendFeishuApproval(ctx, store.FeishuActionNpmVersion, nameAndVersion, 0, 0, ownerPtr(publisherID), nameAndVersion+" · 插件待审核", detail, s.feishuReviewers(ctx))
}

// feishuNpmResultFromWeb 补齐网页审核后的发布者通知；卡片审核的 owner 会在 applyFeishuDecision 返回。
func (s *Server) feishuNpmResultFromWeb(ctx context.Context, name, version, decision string) {
	if version == "" || (decision != "approve" && decision != "reject") {
		return
	}
	v, err := s.store.GetVersion(ctx, name, version)
	if err != nil || v.PublisherID == nil {
		return
	}
	s.notifyFeishuResult(v.PublisherID, npmCardObject(name, version), decision)
}

// applyFeishuNpmDecision 只在一个版本仍待审、操作者当前仍有审核权限时改变它的状态。
// 不能拿一张很久以前发的卡片绕过被收回的权限，也不能把网页上已经审过的版本再处理一遍。
func (s *Server) applyFeishuNpmDecision(ctx context.Context, v viewer, action *store.FeishuCardAction, decision string) (string, *int64, error) {
	if !v.access.Has(rbac.PluginsManage) {
		return "", nil, fmt.Errorf("%w：你没有审核 NPM 插件版本的权限", errFeishuForbidden)
	}
	name, version, err := npmCardObjectParts(action.ObjectKey)
	if err != nil {
		return "", nil, err
	}
	pending, err := s.store.GetVersion(ctx, name, version)
	if err != nil {
		return "", nil, errors.New("插件版本不存在")
	}
	if pending.Status != store.StatusPending {
		return "", nil, errors.New("这个插件版本已被处理或不是待审核状态")
	}
	// registry.Review 规定驳回必须有备注。飞书本批是即时按钮，没有伪造文本框填写的原因，
	// 所以只记录继承自既有资产卡片的来源，来源和实际操作者也单独写审计。
	note := ""
	if decision == "reject" {
		note = "from_feishu"
	}
	n, err := s.registry.Review(ctx, v.user.ID, name, version, decision, note)
	if err != nil || n != 1 {
		return "", nil, errors.New("这个插件版本已被处理或不是待审核状态")
	}
	return npmCardObject(name, version), pending.PublisherID, nil
}
