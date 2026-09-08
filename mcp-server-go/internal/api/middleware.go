// 文件：mcp-server-go/internal/api/middleware.go —— HTTP 中间件：请求日志（bytes/ip/user）/ 尾斜杠归一 / JWT 鉴权 / 下载 token
// 修改：2026-09-08（日期由 fresh-header.ps1 刷新）

package api

import (
	"context"
	"crypto/hmac"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/Reisentyann/Mabel-s-Tentacles/common"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/authz"
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
			"ip", common.ClientIP(r),
			"duration", time.Since(start).String(),
		}
		if rec.user != "" {
			args = append(args, "user", rec.user)
		}
		slog.Info("http request", args...)
	})
}

// clientIP 已收敛 common.ClientIP（MCP 侧 mcp/auth.go 的同款副本同步退役）。

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

// requireAuth JWT 校验中间件（权限批次 2026-09-06 重做）：
// 解 token → 查库取实时 is_active/role/组 → Principal 进 context
// （下游 authz.CanRead/CanWrite 统一取用）。查库换实时封号——
// admin 停用账号即刻生效，不等 token 过期。
// api.require_auth=false（本地开发显式关闭）时放行并注入匿名管理员主体
// ——下游授权代码路径保持同一形状（与 MCP 空 key 放行同款开发口径）。
func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.cfg.API.RequireAuth {
			p := &authz.Principal{Kind: authz.KindUser, Name: "anonymous", Role: "admin"}
			next.ServeHTTP(w, r.WithContext(authz.WithPrincipal(r.Context(), p)))
			return
		}
		claims, err := service.ParseToken(s.cfg.Security.SecretKey, bearerToken(r))
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid or expired token")
			return
		}
		p := s.loadPrincipal(r.Context(), claims)
		if p == nil {
			slog.Warn("auth rejected: account disabled or missing",
				"user", claims.Subject, "ip", common.ClientIP(r), "path", r.URL.Path)
			writeError(w, http.StatusUnauthorized, "invalid or expired token")
			return
		}
		// 把用户名写回顶层日志记录器，RequestLog 结束后随请求日志一起落盘
		if rec, ok := w.(*statusRecorder); ok {
			rec.user = p.Name
		}
		next.ServeHTTP(w, r.WithContext(authz.WithPrincipal(r.Context(), p)))
	})
}

// loadPrincipal claims → 实时主体（查库）。nil = 拒绝（账号不存在/已停用）。
// 无库兜底（测试/降级）：信 claims 的角色快照。
func (s *Server) loadPrincipal(ctx context.Context, claims *service.Claims) *authz.Principal {
	if s.repo == nil {
		return &authz.Principal{Kind: authz.KindUser, UID: claims.UserID, Name: claims.Subject, Role: claims.Role}
	}
	u, err := s.repo.GetUserByID(ctx, claims.UserID)
	if err != nil || u == nil || !u.IsActive {
		return nil
	}
	gids, gerr := s.repo.UserGroupIDs(ctx, u.ID)
	if gerr != nil {
		slog.Warn("load user groups failed", "user", u.Username, "error", gerr)
		gids = nil
	}
	return &authz.Principal{Kind: authz.KindUser, UID: u.ID, Name: u.Username, Role: u.Role, GroupIDs: gids}
}

// requireAdmin admin 专属端点包装（用户/组/钥匙管理、backfill）。
// 拒绝必须留痕（谁 tried what）——越权尝试是安全审计的第一现场。
func (s *Server) requireAdmin(next http.Handler) http.Handler {
	return s.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := authz.PrincipalFrom(r.Context())
		if p == nil || !p.IsAdmin() {
			slog.Warn("admin access denied",
				"user", authz.PrincipalFrom(r.Context()).Subject(),
				"ip", common.ClientIP(r), "path", r.URL.Path)
			writeError(w, http.StatusForbidden, "admin only")
			return
		}
		next.ServeHTTP(w, r)
	}))
}

// principalOf 请求主体（requireAuth 注入后取用；公开端点自证时可能为 nil）。
func principalOf(r *http.Request) *authz.Principal {
	return authz.PrincipalFrom(r.Context())
}

// checkAccessToken 已退役（2026-09-08 限时票据批次）：静态 ACCESS_TOKEN
// 曾是"通过即跳过全部授权"的万能下载钥匙——链接一旦外泄等于全站任意
// 文件（含私密）永久可下载。下载自证改走 exp+ticket 限时票据（单文件
// 绑定 + 半小时过期，service/ticket.go）。函数体保留仅作口径变更的考古
// 注记，无调用方。
func (s *Server) checkAccessToken(r *http.Request) bool {
	token := s.cfg.API.AccessToken
	if token == "" {
		return true
	}
	return hmac.Equal([]byte(r.URL.Query().Get("token")), []byte(token))
}
