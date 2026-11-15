package api

import (
	"net/http"
	"strings"

	"github.com/hikariming/dsh-ee/server/internal/i18n"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// PUT /api/me/preferences {language}：保存当前用户的语言偏好。
//
// language 只接受规范短码 zh / en / es / pt / hi；zh-CN、zh_CN、en-US 这类浏览器写法
// 会先归一成短码再保存。空串表示「跟随组织默认语言」（settings.defaultLanguage），
// 飞书消息渲染时按「用户语言 → 组织默认 → zh」取值。
//
// 目前只有语言一项，端点命名保留 preferences 以便以后放其它个人界面偏好。
func (s *Server) handlePutMyPreferences(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Language string `json:"language"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	raw := strings.TrimSpace(req.Language)
	lang := i18n.Normalize(raw)
	if raw != "" && lang == "" {
		writeErr(w, http.StatusBadRequest, "语言只能是 zh、en、es、pt、hi，或留空跟随组织默认")
		return
	}
	u := currentUser(r)
	if err := s.store.UpdateUser(r.Context(), u.ID, store.UserPatch{Language: &lang}); err != nil {
		s.internalErr(w, r, err)
		return
	}
	// currentUser 是请求上下文里的指针：同一请求后续处理看到的是新值。
	u.Language = lang
	writeJSON(w, http.StatusOK, map[string]any{"language": lang})
}
