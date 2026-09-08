// 文件：mcp-server-go/internal/api/router.go —— HTTP API 路由装配：公共路由 + JWT 保护路由 + Server 结构
// 修改：2026-09-08（日期由 fresh-header.ps1 刷新）

package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/Reisentyann/Mabel-s-Tentacles/manager-go"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/core"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/config"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/repo"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/search"
)

type Server struct {
	cfg      *config.Config
	repo     repo.Store
	searcher search.Searcher    // 检索门面 = 编排机（索引优先 → SQL 降级）
	orch     *core.Orchestrator // 编排机：describe 同步入口 + 写路径事件
	manager  *manager.Manager   // updater 域（T2/T3 端点）；nil = 未装配
	limiter  *rateLimiter       // 登录/注册限流（注册开放后的基础防滥用）
}

// Register 把 HTTP API 路由挂到 mux 上。orch 兼任检索门面（实现 search.Searcher）。
func Register(mux *http.ServeMux, cfg *config.Config, st repo.Store, orch *core.Orchestrator, mgr *manager.Manager) {
	s := &Server{cfg: cfg, repo: st, searcher: orch, orch: orch, manager: mgr, limiter: newRateLimiter()}

	// 公共路由（登录/注册限流：撞库与滥注册的第一道闸）
	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("POST /api/auth/login", s.limit("login", 5, time.Minute, s.login))
	mux.HandleFunc("POST /api/auth/refresh", s.limit("refresh", 10, time.Minute, s.refresh))
	mux.HandleFunc("POST /api/auth/logout", s.logout)
	mux.HandleFunc("POST /api/auth/register", s.limit("register", 3, time.Hour, s.register))
	mux.HandleFunc("GET /api/files/download", s.downloadFile) // 自证端点：静态 token（agent 链接）或 JWT

	// 受 JWT 保护
	mux.Handle("GET /api/files", s.requireAuth(http.HandlerFunc(s.listFiles)))
	mux.Handle("POST /api/files/download-zip", s.requireAuth(http.HandlerFunc(s.downloadZip)))
	mux.Handle("GET /api/files/search", s.requireAuth(http.HandlerFunc(s.searchFiles)))
	mux.Handle("GET /api/files/metadata", s.requireAuth(http.HandlerFunc(s.getFileMetadata)))
	mux.Handle("PUT /api/files/metadata", s.requireAuth(http.HandlerFunc(s.describeFile)))
	mux.Handle("POST /api/files/copy", s.requireAuth(http.HandlerFunc(s.copyFile)))
	mux.Handle("POST /api/files/move", s.requireAuth(http.HandlerFunc(s.moveFile)))
	mux.Handle("POST /api/files/analyze", s.requireAuth(http.HandlerFunc(s.analyzeFile)))

	// admin 专属（权限批次 2026-09-06）：全库扫描与账号/组/钥匙管理
	mux.Handle("POST /api/files/backfill", s.requireAdmin(http.HandlerFunc(s.backfillFile)))
	mux.Handle("GET /api/admin/users", s.requireAdmin(http.HandlerFunc(s.listUsers)))
	mux.Handle("PUT /api/admin/users/{username}/active", s.requireAdmin(http.HandlerFunc(s.setUserActive)))
	mux.Handle("POST /api/admin/groups", s.requireAdmin(http.HandlerFunc(s.createGroup)))
	mux.Handle("GET /api/admin/groups", s.requireAdmin(http.HandlerFunc(s.listGroups)))
	mux.Handle("DELETE /api/admin/groups/{id}", s.requireAdmin(http.HandlerFunc(s.deleteGroup)))
	mux.Handle("POST /api/admin/groups/{id}/members", s.requireAdmin(http.HandlerFunc(s.addGroupMember)))
	mux.Handle("DELETE /api/admin/groups/{id}/members/{username}", s.requireAdmin(http.HandlerFunc(s.removeGroupMember)))
	mux.Handle("POST /api/admin/agent-keys", s.requireAdmin(http.HandlerFunc(s.issueAgentKey)))
	mux.Handle("GET /api/admin/agent-keys", s.requireAdmin(http.HandlerFunc(s.listAgentKeys)))
	mux.Handle("DELETE /api/admin/agent-keys/{id}", s.requireAdmin(http.HandlerFunc(s.revokeAgentKey)))

	mux.Handle("GET /api/operations", s.requireAuth(http.HandlerFunc(s.listOperations)))
	mux.Handle("GET /api/commands", s.requireAuth(http.HandlerFunc(s.listCommands)))
	mux.Handle("GET /api/commands/{id}", s.requireAuth(http.HandlerFunc(s.getCommand)))

	// 管理页前端托管（部署批次 2026-09-08）："/" 无方法兜底——已注册的
	// API（方法树）/ SSE / message（默认树具体路径）优先匹配，其余走
	// 此回退：静态文件命中直出，未命中回 index.html（SPA 前端路由自管）。
	// WebDir 空 = 不托管。注意不能用 "GET /"（方法树与 /sse 等无方法
	// 注册跨树冲突 panic——Go 1.22 mux 规则）。
	if s.cfg.Server.WebDir != "" {
		mux.Handle("/", s.spaHandler())
	}
}

// spaHandler 静态目录 SPA 托管：GET/HEAD 静态直出 + 任意路径回
// index.html（前端路由 /manage /login 由其接管）；其余方法 404。
// 路径清洗用 http.Dir 自带语义（.. 归一），双保险再拒显式穿越段。
func (s *Server) spaHandler() http.Handler {
	dir := s.cfg.Server.WebDir
	fs := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		rel := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if rel != "" && !strings.Contains(rel, "..") {
			if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel))); err == nil {
				fs.ServeHTTP(w, r)
				return
			}
		}
		// SPA fallback：前端路由（/manage /login）都由 index.html 接管
		http.ServeFile(w, r, filepath.Join(dir, "index.html"))
	})
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, detail string) {
	writeJSON(w, code, map[string]string{"detail": detail})
}

func decodeJSON(r *http.Request, v any) error {
	return json.NewDecoder(r.Body).Decode(v)
}
