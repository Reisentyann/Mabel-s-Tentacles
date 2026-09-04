// 文件：mcp-server-go/internal/api/middleware.go —— HTTP 中间件：请求日志（bytes/ip/user）/ 尾斜杠归一 / JWT 鉴权 / 下载 token
// 修改：2026-09-03（日期由 fresh-header.ps1 刷新）

package api

import (
	"context"
	"crypto/hmac"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/service"
)

// TrailingSlash 归一化：非根路径去掉末尾斜杠，避免 ServeMux 精确匹配导致 404。
func TrailingSlash(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" && strings.HasSuffix(r.URL.Path, "/") {
			r.URL.Path = strings.TrimRight(r.URL.Path, "/")
		}
		next.ServeHTTP(w, r)
	})
}

// RequestLog 是顶层的结构化请求日志中间件（字段规范见 internal/logging 包注释）。
// user 由内层 requireAuth 写入 statusRecorder（未鉴权路径为空则不记）。
func RequestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		args := []any{
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"bytes", rec.bytes,
			"ip", clientIP(r),
			"duration", time.Since(start).String(),
		}
		if rec.user != "" {
			args = append(args, "user", rec.user)
		}
		slog.Info("http request", args...)
	})
}

// clientIP 取真实客户端 IP：反代场景（1Panel 等）优先 X-Forwarded-For 首段，
// 否则 RemoteAddr 去端口。
func clientIP(r *http.Request) string {
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

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
	user   string // requireAuth 解析到的 JWT subject，供 RequestLog 落日志
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(p []byte) (int, error) {
	n, err := r.ResponseWriter.Write(p)
	r.bytes += n
	return n, err
}

func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) > 7 && strings.EqualFold(h[:7], "Bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

// requireAuth JWT 校验中间件，校验通过后把 claims 放入 context。
// 当 config 里 api.require_auth=false（开发阶段）时直接放行，不做鉴权。
func (s *Server) requireAuth(next http.Handler) http.Handler {
	if !s.cfg.API.RequireAuth {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, err := service.ParseToken(s.cfg.Security.SecretKey, bearerToken(r))
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid or expired token")
			return
		}
		// 把用户名写回顶层日志记录器，RequestLog 结束后随请求日志一起落盘
		if rec, ok := w.(*statusRecorder); ok {
			rec.user = claims.Subject
		}
		next.ServeHTTP(w, r.WithContext(contextWithUser(r.Context(), claims)))
	})
}

type ctxKey int

const userKey ctxKey = 0

func contextWithUser(ctx context.Context, claims *service.Claims) context.Context {
	return context.WithValue(ctx, userKey, claims)
}

func userFromContext(ctx context.Context) (*service.Claims, bool) {
	claims, ok := ctx.Value(userKey).(*service.Claims)
	return claims, ok
}

// checkAccessToken 校验下载接口的 query token，配置为空时不校验。
func (s *Server) checkAccessToken(r *http.Request) bool {
	token := s.cfg.API.AccessToken
	if token == "" {
		return true
	}
	return hmac.Equal([]byte(r.URL.Query().Get("token")), []byte(token))
}
