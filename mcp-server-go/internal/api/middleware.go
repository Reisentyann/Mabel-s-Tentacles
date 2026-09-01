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

// RequestLog 是顶层的结构化请求日志中间件。
func RequestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		slog.Info("http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration", time.Since(start).String(),
		)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
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
