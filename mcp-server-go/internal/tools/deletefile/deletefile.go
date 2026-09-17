// 文件：mcp-server-go/internal/tools/deletefile/deletefile.go —— MCP 工具 delete_file：主动软删除文件并从索引移除
// 修改：2026-09-17（日期由 fresh-header.ps1 刷新）

package deletefile

import (
	"context"
	"log/slog"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/tools"
)

func init() {
	tools.Register(register)
}

func register(s *server.MCPServer, deps tools.Deps) {
	tool := mcp.NewTool("delete_file",
		mcp.WithDescription("Soft-delete a file from your workspace. The file is unlinked from active listings and indexes, while remaining recoverable in audit records."),
		mcp.WithString("file_path",
			mcp.Description("Path of the file to delete (must be in your own space). Also accepts 'path'."),
		),
		mcp.WithString("path",
			mcp.Description("Alias for file_path."),
		),
		mcp.WithString("reason",
			mcp.Description("Optional reason for deletion, recorded in audit logs."),
		),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		filePath, err := tools.GetFilePath(req)
		if err != nil {
			return tools.ResultError("invalid file_path: " + err.Error()), nil
		}
		reason := req.GetString("reason", "")

		sessionID := tools.SessionID(ctx)
		start := time.Now()
		params := map[string]any{"file_path": filePath, "reason": reason}

		// 键空间：只能删除自己空间的文件（跨用户删除禁止）
		sc, serr := tools.ScopeWrite(ctx, filePath)
		if serr != nil {
			tools.RecordOperation(ctx, deps.Store, sessionID, "delete_file", filePath, "denied", serr.Error(), params)
			return tools.Deny(ctx, "delete_file", filePath, serr.Error()), nil
		}
		key := sc.Key

		// 写授权：删除是对既有文件的终态操作，需写权限
		if denied, authReason := tools.CanFile(ctx, deps.Store, key, true); denied {
			tools.RecordOperation(ctx, deps.Store, sessionID, "delete_file", key, "denied", authReason, params)
			return tools.Deny(ctx, "delete_file", key, authReason), nil
		}

		if deps.Orch != nil {
			if err := deps.Orch.Delete(ctx, key, sessionID); err != nil {
				slog.Error("delete_file orch failed", "path", key, "session", sessionID, "error", err, "duration", time.Since(start).String())
				tools.RecordOperation(ctx, deps.Store, sessionID, "delete_file", key, "failed", err.Error(), params)
				return tools.ResultError(err.Error()), nil
			}
		} else if deps.Store != nil {
			if err := deps.Store.SoftDeleteMetadata(ctx, key); err != nil {
				slog.Error("delete_file store failed", "path", key, "session", sessionID, "error", err, "duration", time.Since(start).String())
				tools.RecordOperation(ctx, deps.Store, sessionID, "delete_file", key, "failed", err.Error(), params)
				return tools.ResultError(err.Error()), nil
			}
		} else {
			return tools.ResultError("no backend available for deletion"), nil
		}

		slog.Info("delete_file ok", "path", key, "reason", reason, "session", sessionID, "duration", time.Since(start).String())
		tools.RecordOperation(ctx, deps.Store, sessionID, "delete_file", key, "success", "", params)
		return tools.Result(map[string]any{"success": true, "message": "Successfully deleted " + key}), nil
	})
}
