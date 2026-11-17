package api

import (
	"errors"

	"github.com/hikariming/dsh-ee/server/internal/store"
)

// errProviderKeyOriginMoved 表示记录里的 API Key 绑定在另一个 origin 上。这类记录来自
// 改址守卫上线前：PATCH 改了 base_url 却没重填 key，旧 key 留在了库里。读取侧不能把它
// 发往当前 base_url（新地址可能是攻击者）：check 返回可读错误，转发路径按配置错误拒绝。
var errProviderKeyOriginMoved = errors.New("provider api key is bound to a different origin")

// providerKeyOriginMoved 报告历史记录里的 key 是否绑在另一个 origin 上。
// api_key_origin 为空表示记录来自加列之前：无法判定，按原逻辑放行（残余窗口见 PR），
// 管理员下次重填 key 时补写 origin。
func providerKeyOriginMoved(p *store.Provider) bool {
	if p == nil || p.APIKeyOrigin == "" {
		return false
	}
	origin, ok := baseURLOrigin(p.BaseURL)
	return !ok || origin != p.APIKeyOrigin
}

// openProviderKey 解密上游 key，并在发送前挡住绑定 origin 不一致的记录。
// 调用方沿用各自现有的「provider 不可用/配置错误」错误壳与故障转移逻辑。
func (s *Server) openProviderKey(p *store.Provider) ([]byte, error) {
	if providerKeyOriginMoved(p) {
		return nil, errProviderKeyOriginMoved
	}
	return s.box.Open(p.APIKeyEnc)
}
