package service

import (
	"regexp"
	"strings"
)

var iqURLCredentials = regexp.MustCompile(`(?i)(https?|socks5h?)://[^\s/@]+@`)
var iqBearer = regexp.MustCompile(`(?i)\bBearer\s+[^\s,;"']+`)
var iqSecretValue = regexp.MustCompile(`(?i)((?:"|')?(?:access_token|refresh_token|id_token|api_key|api-key|password|authorization|cookie|set-cookie)(?:"|')?\s*[:=]\s*)(?:"[^"\r\n]*"|'[^'\r\n]*'|[^\s,;&]+)`)

// Keep the useful failure cause while removing authentication material from
// the compact account-list preview. Historical results are left unchanged.
func IQErrorSummary(message string) string {
	message = iqURLCredentials.ReplaceAllString(message, "${1}://[REDACTED]@")
	message = iqBearer.ReplaceAllString(message, "Bearer [REDACTED]")
	message = iqSecretValue.ReplaceAllString(message, "${1}[REDACTED]")
	message = strings.Join(strings.Fields(sanitizeErrorMessage(message)), " ")
	if message == "" {
		return "请求失败，请查看测试详情"
	}
	runes := []rune(message)
	if len(runes) > 512 {
		return string(runes[:512]) + "…（详见测试记录）"
	}
	return message
}
