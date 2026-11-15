package config

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"unicode"
)

// CSP 的三种模式（DSH_EE_CSP）。默认 enforce：不设置时下发内置的基线策略；
// off 显式关闭完整策略（只保留 frame-ancestors 嵌入收口）；report 只报告不阻断。
// 见 server/internal/api/security_headers.go。
const (
	CSPOff     = "off"
	CSPReport  = "report"
	CSPEnforce = "enforce"
)

// CSPDefaultMode 是不设置 DSH_EE_CSP 时生效的模式：默认启用基线策略。
// 旧版本默认 off，升级后行为会变；要保留旧行为显式设置 DSH_EE_CSP=off。
const CSPDefaultMode = CSPEnforce

// parseCSPMode 解析 DSH_EE_CSP：off | report | enforce，大小写与首尾空白不敏感；
// 空值（未设置）取 CSPDefaultMode。非法值让启动失败（fail closed），避免把写错的
// "enforce" 静默当成 off。
func parseCSPMode(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "":
		return CSPDefaultMode, nil
	case CSPOff:
		return CSPOff, nil
	case CSPReport:
		return CSPReport, nil
	case CSPEnforce:
		return CSPEnforce, nil
	default:
		return "", fmt.Errorf("DSH_EE_CSP 只能是 off / report / enforce，当前为 %q", raw)
	}
}

// validateCSPReportURI 校验 DSH_EE_CSP_REPORT_URI：必须是不带空白/控制字符/指令分隔符的
// http(s) 绝对地址。report-uri 直接拼进响应头，含分号、引号或换行会破坏 CSP 甚至造成头注入，
// 所以启动时直接拒绝（fail closed）。
func validateCSPReportURI(raw string) error {
	for _, r := range raw {
		if r < 0x20 || r == 0x7f || unicode.IsSpace(r) || strings.ContainsRune(";,\"'<>", r) {
			return fmt.Errorf("DSH_EE_CSP_REPORT_URI 不能包含空白、控制字符或 ; , \" ' < >：%q", raw)
		}
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errors.New("DSH_EE_CSP_REPORT_URI 必须是 http(s)://host[:port][/path] 形式")
	}
	return nil
}
