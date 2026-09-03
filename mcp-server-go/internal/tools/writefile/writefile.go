// 文件：mcp-server-go/internal/tools/writefile/writefile.go —— MCP 工具 write_file：写文件 + 内联描述 + T1 元数据
// 修改：2026-09-03（日期由 fresh-header.ps1 刷新）

package writefile

import (
	"context"
	"log/slog"
	"strings"
	"time"

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
	tool := mcp.NewTool("write_file",
		mcp.WithDescription("Write generated content to a file under the data directory. Pass title/description/tags so the file can be found later via search; without a description the file may become unfindable. Use this when the user wants to generate code, write an article, or create a file."),
		mcp.WithString("file_path",
			mcp.Required(),
			mcp.Description("Path of the file to write, relative to the data directory."),
		),
		mcp.WithString("content",
			mcp.Required(),
			mcp.Description("Content to write to the file."),
		),
		mcp.WithString("title",
			mcp.Description("Short title of the file, helps searchability."),
		),
		mcp.WithString("description",
			mcp.Description("Free-text description of the file content, enables keyword search later."),
		),
		mcp.WithString("tags",
			mcp.Description("Comma-separated tags, e.g. 'report,red'."),
		),
		mcp.WithString("file_type",
			mcp.Description("File type, e.g. text / image / code / other. Defaults to inferred from extension."),
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
		start := time.Now()
		params := map[string]any{"file_path": filePath, "content_size": len(content), "has_description": description != "" || len(tags) > 0}

		if err := service.SafeWrite(deps.Cfg.DataDir, filePath, content); err != nil {
			slog.Error("write_file failed", "path", filePath, "session", sessionID, "error", err, "duration", time.Since(start).String())
			tools.RecordOperation(ctx, deps.Store, sessionID, "write_file", filePath, "failed", err.Error(), params)
			return tools.ResultError(err.Error()), nil
		}

		slog.Info("write_file ok", "path", filePath, "bytes", len(content), "session", sessionID, "duration", time.Since(start).String())
		tools.RecordFileMeta(ctx, deps.Store, filePath, []byte(content), sessionID)

		// 内联描述：AI 传了任意描述字段就一次性落库，避免事后文件找不到。
		// UpsertMetadata 用 COALESCE，仅覆盖非空字段，不影响 RecordFileMeta 已写的技术元数据。
		if deps.Store != nil && (title != "" || description != "" || len(tags) > 0 || fileType != "") {
			meta := &repo.FileMetadata{
				FilePath:    filePath,
				Title:       tools.StrPtr(title),
				Description: tools.StrPtr(description),
				Tags:        tags,
				FileType:    tools.StrPtr(fileType),
				SessionID:   tools.StrPtr(sessionID),
			}
			if err := deps.Store.UpsertMetadata(ctx, meta); err != nil {
				slog.Warn("write_file upsert description failed", "path", filePath, "session", sessionID, "error", err)
			}
		}

		tools.RecordOperation(ctx, deps.Store, sessionID, "write_file", filePath, "success", "", params)
		result := map[string]any{"success": true, "message": "Successfully wrote to " + filePath}
		if u := tools.DownloadURL(deps.Cfg, filePath); u != "" {
			result["download_url"] = u
		}
		return tools.Result(result), nil
	})
}
