// 文件：mcp-server-go/internal/api/commands.go —— 命令记录端点：分页列表 / 单条详情（Dashboard 数据源）
// 修改：2026-09-03（日期由 fresh-header.ps1 刷新）

package api

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5"
)

func (s *Server) listCommands(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	size, _ := strconv.Atoi(r.URL.Query().Get("size"))
	if size < 1 || size > 100 {
		size = 10
	}

	if s.repo == nil {
		writeJSON(w, http.StatusOK, map[string]any{"items": []any{}, "total": 0, "page": page, "size": size})
		return
	}

	items, total, err := s.repo.ListCommands(r.Context(), page, size)
	if err != nil {
		slog.Error("list commands failed", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total, "page": page, "size": size})
}

func (s *Server) getCommand(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid command id")
		return
	}

	if s.repo == nil {
		writeError(w, http.StatusInternalServerError, "database unavailable")
		return
	}

	command, err := s.repo.GetCommand(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "Command not found")
			return
		}
		slog.Error("get command failed", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, command)
}
