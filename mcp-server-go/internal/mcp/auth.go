// 文件：mcp-server-go/internal/mcp/auth.go —— MCP API Key 中间件：Bearer 常量时间比对（未配置则放行）
// 修改：2026-09-03（日期由 fresh-header.ps1 刷新）

package mcp

import (
	"crypto/subtle"
	"net/http"
)

func AuthMiddleware(next http.Handler, apiKey string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if apiKey == "" {
			next.ServeHTTP(w, r)
			return
		}
		got := r.Header.Get("Authorization")
		want := "Bearer " + apiKey
		if subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
