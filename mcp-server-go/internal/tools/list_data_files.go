package tools

import (
	"context"
	"log/slog"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/config"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/service"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/store"
)

func RegisterListDataFiles(s *server.MCPServer, cfg *config.Config, st *store.Store) {
	tool := mcp.NewTool("list_data_files",
		mcp.WithDescription("List all files under the data directory."),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		sessionID := sessionIDFromContext(ctx)

		files, err := service.SafeList(cfg.Server.DataDir)
		if err != nil {
			slog.Error("list_data_files failed", "error", err)
			recordOperation(ctx, st, sessionID, "list_data_files", "", "failed", err.Error(), nil)
			return jsonError(err.Error()), nil
		}

		slog.Info("list_data_files ok", "count", len(files))
		recordOperation(ctx, st, sessionID, "list_data_files", "", "success", "", map[string]any{"count": len(files)})
		return jsonResult(map[string]any{"success": true, "files": files}), nil
	})
}
