// 文件：mcp-server-go/internal/api/audit.go —— 管理员只读盘库对账端点
// 修改：2026-10-08（日期由 fresh-header.ps1 刷新）

package api

import (
	"log/slog"
	"net/http"
	"time"
)

func (s *Server) auditFiles(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	if s.manager == nil {
		writeError(w, http.StatusServiceUnavailable, "manager not available")
		return
	}
	report, err := s.manager.Audit(r.Context())
	if err != nil {
		slog.Error("admin audit failed", "method", r.Method, "path", r.URL.Path, "user", principalOf(r).Subject(), "error", err, "duration", time.Since(start).String())
		writeError(w, http.StatusInternalServerError, "audit failed")
		return
	}
	slog.Info("admin audit ok", "user", principalOf(r).Subject(), "orphans", len(report.Orphans), "ghosts", len(report.Ghosts), "duplicate_groups", len(report.DupChecksums), "duration", time.Since(start).String())
	writeJSON(w, http.StatusOK, report)
}
