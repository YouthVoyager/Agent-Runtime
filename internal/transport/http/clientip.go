package httpserver

import (
	"net"
	"net/http"
	"strings"
)

// ClientIP 提取客户端真实 IP，优先信任 X-Forwarded-For 首个地址，其次 X-Real-IP，
// 都缺失时回退到 RemoteAddr。审计日志(audit_logs.ip)统一使用该函数提取。
func ClientIP(r *http.Request) string {
	if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); forwarded != "" {
		first := strings.TrimSpace(strings.Split(forwarded, ",")[0])
		if first != "" {
			return first
		}
	}
	if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
		return realIP
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return strings.TrimSpace(r.RemoteAddr)
	}
	return host
}
