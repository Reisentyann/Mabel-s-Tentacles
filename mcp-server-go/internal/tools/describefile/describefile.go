package describefile

import (
	"context"
	"log/slog"
	"os"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/repo"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/service"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/tools"
)

func init() {
	tools.Register(register)
}

func register(s *server.MCPServer, deps tools.Deps) {
	tool := mcp.NewTool("describe_file",
		mcp.WithDescription("Add a description, tags, and type to an existing file so it can be searched later."),
		mcp.WithString("file_path",
			mcp.Required(),
			mcp.Description("Path of the file, relative to the data directory."),
		),
		mcp.WithString("title",
			mcp.Description("Short title of the file."),
		),
		mcp.WithString("description",
			mcp.Description("Free-text description of the file."),
		),
		mcp.WithString("tags",
			mcp.Description("Comma-separated tags, e.g. 'report,red'."),
		),
		mcp.WithString("file_type",
			mcp.Description("File type, e.g. text / image / code / other."),
		),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		filePath, err := req.RequireString("file_path")
		if err != nil {
			return tools.ResultError("invalid file_path: " + err.Error()), nil
		}
		title := req.GetString("title", "")
		description := req.GetString("description", "")
		fileType := req.GetString("file_type", "")

		var tags []string
		if raw := req.GetString("tags", ""); raw != "" {
			for _, t := range strings.Split(raw, ",") {
				if t = strings.TrimSpace(t); t != "" {
					tags = append(tags, t)
				}
			}
		}

		sessionID := tools.SessionID(ctx)

		target, err := service.ResolvePath(deps.Cfg.DataDir, filePath)
		if err != nil {
			return tools.ResultError(err.Error()), nil
		}
		if info, err := os.Stat(target); err != nil || info.IsDir() {
			return tools.ResultError("error: file '" + filePath + "' does not exist"), nil
		}

		meta := &repo.FileMetadata{
			FilePath:    filePath,
			Title:       tools.StrPtr(title),
			Description: tools.StrPtr(description),
			Tags:        tags,
			FileType:    tools.StrPtr(fileType),
			SessionID:   tools.StrPtr(sessionID),
		}
		if deps.Store != nil {
			if err := deps.Store.UpsertMetadata(ctx, meta); err != nil {
				slog.Error("describe_file failed", "path", filePath, "error", err)
				return tools.ResultError("Database error: " + err.Error()), nil
			}
		}

		slog.Info("describe_file ok", "path", filePath)
		tools.RecordOperation(ctx, deps.Store, sessionID, "describe_file", filePath, "success", "", map[string]any{"description": description, "tags": tags, "file_type": fileType})
		return tools.Result(map[string]any{"success": true, "message": "Successfully described " + filePath}), nil
	})
}
