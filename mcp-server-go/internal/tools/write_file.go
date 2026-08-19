package tools

import (
	"context"
	"encoding/json"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/config"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/service"
)

func RegisterWriteFile(s *server.MCPServer, cfg *config.Config) {
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

		if err := service.SafeWrite(cfg.DataDir, filePath, content); err != nil {
			return jsonError(err.Error()), nil
		}

		return jsonResult(true, "Successfully wrote to "+filePath), nil
	})
}

func jsonResult(success bool, message string) *mcp.CallToolResult {
	b, _ := json.Marshal(map[string]any{"success": success, "message": message})
	return mcp.NewToolResultText(string(b))
}

func jsonError(message string) *mcp.CallToolResult {
	return jsonResult(false, message)
}
