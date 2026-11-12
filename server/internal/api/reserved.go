package api

// reservedConnSlug 是网关/系统已经占用的前缀或系统连接名。
// 连接工具名是 <slug>__<tool>：slug=home 会命中 home__ 内置工具，
// slug=ee 会命中 ee_ Agent 工具，所以不能给用户连接用。
func reservedConnSlug(slug string) bool {
	switch slug {
	case kbConn, kbQAName, appConn, dataConn, homeConn, "ee":
		return true
	}
	return false
}
