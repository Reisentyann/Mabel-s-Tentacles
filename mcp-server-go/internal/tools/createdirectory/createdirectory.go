// 文件：mcp-server-go/internal/tools/createdirectory/createdirectory.go —— MCP 工具 create_directory：创建持久化逻辑空目录
// 修改：2026-10-08（日期由 fresh-header.ps1 刷新）

package createdirectory

import (
	"context"
	"log/slog"
	"time"

	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/tools"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func init() { tools.Register(register) }

func register(s *server.MCPServer, deps tools.Deps) {
	s.AddTool(mcp.NewTool("create_directory", mcp.WithDescription("Create an empty persistent directory in your workspace. It is visible in the logical file tree and does not create a physical data directory."), mcp.WithString("path", mcp.Required(), mcp.Description("Directory path relative to your workspace."))), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path, err := req.RequireString("path")
		if err != nil {
			return tools.ResultError("invalid path: " + err.Error()), nil
		}
		sc, err := tools.ScopeWrite(ctx, path)
		if err != nil {
			return tools.Deny(ctx, "create_directory", path, err.Error()), nil
		}
		if deps.Manager == nil {
			return tools.ResultError("manager not wired"), nil
		}
		start := time.Now()
		if err := deps.Manager.CreateDirectory(ctx, sc.Key); err != nil {
			slog.Error("create_directory failed", "tool", "create_directory", "path", sc.Key, "session", tools.SessionID(ctx), "error", err, "duration", time.Since(start).String())
			tools.RecordOperation(ctx, deps.Store, tools.SessionID(ctx), "create_directory", sc.Key, "failed", err.Error(), nil)
			return tools.ResultError(err.Error()), nil
		}
		slog.Info("create_directory ok", "tool", "create_directory", "path", sc.Key, "session", tools.SessionID(ctx), "duration", time.Since(start).String())
		tools.RecordOperation(ctx, deps.Store, tools.SessionID(ctx), "create_directory", sc.Key, "success", "", nil)
		ref, err := deps.Manager.DirectoryAt(ctx, sc.Key)
		if err != nil {
			return tools.ResultError(err.Error()), nil
		}
		return tools.Result(map[string]any{"success": true, "path": sc.Key, "uuid": ref.UUID}), nil
	})
}
