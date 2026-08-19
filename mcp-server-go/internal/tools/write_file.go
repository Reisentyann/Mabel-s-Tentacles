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

func RegisterWriteFile(s *server.MCPServer, cfg *config.Config, st *store.Store) {
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
			return jsonError("invalid file_path: " + err.Error()), nil
		}
		content, err := req.RequireString("content")
		if err != nil {
			return jsonError("invalid content: " + err.Error()), nil
		}

		sessionID := sessionIDFromContext(ctx)
		params := map[string]any{"file_path": filePath, "content_size": len(content)}

		if err := service.SafeWrite(cfg.Server.DataDir, filePath, content); err != nil {
			slog.Error("write_file failed", "path", filePath, "error", err)
			recordOperation(ctx, st, sessionID, "write_file", filePath, "failed", err.Error(), params)
			return jsonError(err.Error()), nil
		}

		slog.Info("write_file ok", "path", filePath, "bytes", len(content))
		recordOperation(ctx, st, sessionID, "write_file", filePath, "success", "", params)
		return jsonResult(map[string]any{"success": true, "message": "Successfully wrote to " + filePath}), nil
	})
}
