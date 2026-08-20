package modifydatafile

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
	tool := mcp.NewTool("modify_data_file",
		mcp.WithDescription("Modify an existing file in the data directory. mode='append' appends content, mode='overwrite' replaces the whole file. Use list_data_files first to get a valid file path."),
		mcp.WithString("file_path",
			mcp.Required(),
			mcp.Description("Path of the file to modify, relative to the data directory."),
		),
		mcp.WithString("content",
			mcp.Required(),
			mcp.Description("Content to append or write."),
		),
		mcp.WithString("mode",
			mcp.Description("'append' or 'overwrite' (default 'append')."),
		),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		filePath, err := req.RequireString("file_path")
		if err != nil {
			return tools.ResultError("invalid file_path: " + err.Error()), nil
		}
		content, err := req.RequireString("content")
		if err != nil {
			return tools.ResultError("invalid content: " + err.Error()), nil
		}
		mode := req.GetString("mode", "append")

		sessionID := tools.SessionID(ctx)
		params := map[string]any{"file_path": filePath, "mode": mode, "content_size": len(content)}

		if err := service.SafeModify(deps.Cfg.DataDir, filePath, content, mode); err != nil {
			slog.Error("modify_data_file failed", "path", filePath, "mode", mode, "error", err)
			tools.RecordOperation(ctx, deps.Store, sessionID, "modify_data_file", filePath, "failed", err.Error(), params)
			return tools.ResultError(err.Error()), nil
		}

		slog.Info("modify_data_file ok", "path", filePath, "mode", mode)
		tools.RecordOperation(ctx, deps.Store, sessionID, "modify_data_file", filePath, "success", "", params)
		return tools.Result(map[string]any{"success": true, "message": "Successfully modified " + filePath + " in " + mode + " mode"}), nil
	})
}
