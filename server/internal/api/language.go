package api

import (
	"context"

	"github.com/hikariming/dsh-ee/server/internal/i18n"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// orgLanguage 返回组织默认语言（规范短码 zh/en/es/pt/hi）。
// 设置读失败时按 zh：消息发送不能因为读不到设置就失败或阻塞业务。
func (s *Server) orgLanguage(ctx context.Context) string {
	set, err := s.appSettings(ctx)
	if err != nil {
		return i18n.Zh
	}
	return i18n.Resolve("", set.DefaultLanguage)
}

// userLanguage 是这名用户的接收语言：用户偏好 → 组织默认 → zh。
// 用户偏好为空（跟随组织）或历史脏值时，由 i18n.Resolve 继续往下一层找。
func (s *Server) userLanguage(ctx context.Context, u *store.User) string {
	if u == nil {
		return s.orgLanguage(ctx)
	}
	return i18n.Resolve(u.Language, s.orgLanguage(ctx))
}
