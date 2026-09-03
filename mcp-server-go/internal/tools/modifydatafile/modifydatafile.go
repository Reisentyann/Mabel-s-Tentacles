// 文件：mcp-server-go/internal/tools/modifydatafile/modifydatafile.go —— MCP 工具 modify_data_file：append/overwrite + 元数据刷新
// 修改：2026-09-03（日期由 fresh-header.ps1 刷新）

package modifydatafile

import (
	"context"
	"log/slog"
	"time"

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
		start := time.Now()
		params := map[string]any{"file_path": filePath, "mode": mode, "content_size": len(content)}

		if err := service.SafeModify(deps.Cfg.DataDir, filePath, content, mode); err != nil {
			slog.Error("modify_data_file failed", "path", filePath, "mode", mode, "session", sessionID, "error", err, "duration", time.Since(start).String())
			tools.RecordOperation(ctx, deps.Store, sessionID, "modify_data_file", filePath, "failed", err.Error(), params)
			return tools.ResultError(err.Error()), nil
		}

		// 刷新元数据：append/overwrite 后 size 与 checksum 会变化，读取整文件重算
		if full, rerr := service.SafeRead(deps.Cfg.DataDir, filePath); rerr == nil {
			tools.RecordFileMeta(ctx, deps.Store, filePath, full, sessionID)
		} else {
			slog.Warn("refresh metadata after modify failed", "path", filePath, "session", sessionID, "error", rerr)
		}

		slog.Info("modify_data_file ok", "path", filePath, "mode", mode, "session", sessionID, "duration", time.Since(start).String())
		tools.RecordOperation(ctx, deps.Store, sessionID, "modify_data_file", filePath, "success", "", params)
		return tools.Result(map[string]any{"success": true, "message": "Successfully modified " + filePath + " in " + mode + " mode"}), nil
	})
}
