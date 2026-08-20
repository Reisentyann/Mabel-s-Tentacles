package api

import (
	"crypto/hmac"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/config"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/store"
)

type Server struct {
	cfg   *config.Config
	store store.Store
}

// Register 把 HTTP API 路由挂到 mux 上。
func Register(mux *http.ServeMux, cfg *config.Config, st store.Store) {
	s := &Server{cfg: cfg, store: st}
	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("GET /api/files", s.listFiles)
	mux.HandleFunc("GET /api/files/download", s.downloadFile)
	mux.HandleFunc("POST /api/files/download-zip", s.downloadZip)
	mux.HandleFunc("GET /api/operations", s.listOperations)
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

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// checkAccessToken 校验下载接口的 query token，配置为空时不校验。
func (s *Server) checkAccessToken(r *http.Request) bool {
	token := s.cfg.API.AccessToken
	if token == "" {
		return true
	}
	return hmac.Equal([]byte(r.URL.Query().Get("token")), []byte(token))
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, detail string) {
	writeJSON(w, code, map[string]string{"detail": detail})
}
