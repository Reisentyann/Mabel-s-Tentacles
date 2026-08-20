package writefile

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
	tool := mcp.NewTool("write_file",
		mcp.WithDescription("Write generated content to a file under the data directory. Use this when the user wants to generate code, write an article, or create a file."),
		mcp.WithString("file_path",
			mcp.Required(),
			mcp.Description("Path of the file to write, relative to the data directory."),
		),
		mcp.WithString("content",
			mcp.Required(),
			mcp.Description("Content to write to the file."),
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

		sessionID := tools.SessionID(ctx)
		params := map[string]any{"file_path": filePath, "content_size": len(content)}

		if err := service.SafeWrite(deps.Cfg.DataDir, filePath, content); err != nil {
			slog.Error("write_file failed", "path", filePath, "error", err)
			tools.RecordOperation(ctx, deps.Store, sessionID, "write_file", filePath, "failed", err.Error(), params)
			return tools.ResultError(err.Error()), nil
		}

		slog.Info("write_file ok", "path", filePath, "bytes", len(content))
		tools.RecordOperation(ctx, deps.Store, sessionID, "write_file", filePath, "success", "", params)
		return tools.Result(map[string]any{"success": true, "message": "Successfully wrote to " + filePath}), nil
	})
}
