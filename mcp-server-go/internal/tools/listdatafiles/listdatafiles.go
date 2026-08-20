package listdatafiles

import (
	"context"
	"log/slog"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/service"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/tools"
)

func init() {
	tools.Register(register)
}

func register(s *server.MCPServer, deps tools.Deps) {
	tool := mcp.NewTool("list_data_files",
		mcp.WithDescription("List all files under the data directory."),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		sessionID := tools.SessionID(ctx)

		files, err := service.SafeList(deps.Cfg.DataDir)
		if err != nil {
			slog.Error("list_data_files failed", "error", err)
			tools.RecordOperation(ctx, deps.Store, sessionID, "list_data_files", "", "failed", err.Error(), nil)
			return tools.ResultError(err.Error()), nil
		}

		slog.Info("list_data_files ok", "count", len(files))
		tools.RecordOperation(ctx, deps.Store, sessionID, "list_data_files", "", "success", "", map[string]any{"count": len(files)})
		return tools.Result(map[string]any{"success": true, "files": files}), nil
	})
}
