// 文件：common/httpx.go —— HTTP 小件：ClientIP（反代优先 X-Forwarded-For 首段，HTTP 与 MCP 两侧同口径）
// 修改：2026-09-06（日期由 fresh-header.ps1 刷新）

package common

import (
	"net/http"
	"strings"
)

// ClientIP 取真实客户端 IP：反代场景（1Panel 等）优先 X-Forwarded-For 首段，
// 否则 RemoteAddr 去端口。安全审计日志的第一现场要素（撞 key / 越权尝试
// 都靠它对现场）。
func ClientIP(r *http.Request) string {
	if xf := r.Header.Get("X-Forwarded-For"); xf != "" {
		if i := strings.Index(xf, ","); i > 0 {
			return strings.TrimSpace(xf[:i])
		}
		return strings.TrimSpace(xf)
	}
	if i := strings.LastIndex(r.RemoteAddr, ":"); i > 0 {
		return r.RemoteAddr[:i]
	}
	return r.RemoteAddr
}
