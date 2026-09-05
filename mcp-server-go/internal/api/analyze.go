// 文件：mcp-server-go/internal/api/analyze.go —— T2/T3 端点：POST analyze（单文件重分析）/ POST backfill（一轮批量回填）
// 修改：2026-09-05（日期由 fresh-header.ps1 刷新）

package api

import (
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/service"
)

type analyzeRequest struct {
	Path string `json:"path"`
}

// analyzeFile T3 手动入口：单文件重分析并返回本次事实产出
// （字段字典 10.3；manager updater 域）。
func (s *Server) analyzeFile(w http.ResponseWriter, r *http.Request) {
	var req analyzeRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	if s.manager == nil {
		writeError(w, http.StatusServiceUnavailable, "manager not available")
		return
	}

	// 与 describeFile 同款前置：路径校验 + 存在性（404 语义先于 manager 执行）
	target, err := service.ResolvePath(s.cfg.DataDir, req.Path)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if info, err := os.Stat(target); err != nil || info.IsDir() {
		writeError(w, http.StatusNotFound, "file not found")
		return
	}

	start := time.Now()
	report, err := s.manager.AnalyzeFile(r.Context(), req.Path)
	if err != nil {
		slog.Error("analyze file failed", "path", req.Path, "error", err, "duration", time.Since(start).String())
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	slog.Info("analyze file ok", "path", req.Path, "families", report.Families, "cod_keys", len(report.Attrs), "duration", time.Since(start).String())
	writeJSON(w, http.StatusOK, map[string]any{
		"success":  true,
		"path":     report.Path,
		"families": report.Families,
		"attrs":    report.Attrs,
	})
}

// backfillFile T2 手动触发一轮批量回填（启动后台轮询之外的即时入口；
// batch 取 describe.backfill.batch 配置）。幂等可重复调用。
func (s *Server) backfillFile(w http.ResponseWriter, r *http.Request) {
	if s.manager == nil {
		writeError(w, http.StatusServiceUnavailable, "manager not available")
		return
	}
	start := time.Now()
	n, err := s.manager.Backfill(r.Context(), s.cfg.Describe.Backfill.Batch)
	if err != nil {
		slog.Error("backfill failed", "error", err, "analyzed", n, "duration", time.Since(start).String())
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	slog.Info("backfill ok", "analyzed", n, "duration", time.Since(start).String())
	writeJSON(w, http.StatusOK, map[string]any{
		"success":  true,
		"message":  "backfill round done",
		"analyzed": n,
	})
}
