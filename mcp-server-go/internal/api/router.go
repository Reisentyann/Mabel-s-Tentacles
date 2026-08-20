package api

import (
	"encoding/json"
	"net/http"

	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/config"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/repo"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/search"
)

type Server struct {
	cfg      *config.Config
	repo     repo.Store
	searcher search.Searcher
}

// Register 把 HTTP API 路由挂到 mux 上。
func Register(mux *http.ServeMux, cfg *config.Config, st repo.Store, searcher search.Searcher) {
	s := &Server{cfg: cfg, repo: st, searcher: searcher}

	// 公共路由
	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("POST /api/auth/login", s.login)
	mux.HandleFunc("POST /api/auth/refresh", s.refresh)
	mux.HandleFunc("POST /api/auth/logout", s.logout)
	mux.HandleFunc("POST /api/auth/register", s.register)
	mux.HandleFunc("GET /api/files/download", s.downloadFile)

	// 受 JWT 保护
	mux.Handle("GET /api/files", s.requireAuth(http.HandlerFunc(s.listFiles)))
	mux.Handle("POST /api/files/download-zip", s.requireAuth(http.HandlerFunc(s.downloadZip)))
	mux.Handle("GET /api/files/search", s.requireAuth(http.HandlerFunc(s.searchFiles)))
	mux.Handle("GET /api/files/metadata", s.requireAuth(http.HandlerFunc(s.getFileMetadata)))
	mux.Handle("PUT /api/files/metadata", s.requireAuth(http.HandlerFunc(s.describeFile)))
	mux.Handle("POST /api/files/copy", s.requireAuth(http.HandlerFunc(s.copyFile)))
	mux.Handle("GET /api/operations", s.requireAuth(http.HandlerFunc(s.listOperations)))
	mux.Handle("GET /api/commands", s.requireAuth(http.HandlerFunc(s.listCommands)))
	mux.Handle("GET /api/commands/{id}", s.requireAuth(http.HandlerFunc(s.getCommand)))
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
