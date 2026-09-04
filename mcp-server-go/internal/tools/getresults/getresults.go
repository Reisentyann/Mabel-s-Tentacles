// 文件：mcp-server-go/internal/tools/getresults/getresults.go —— MCP 工具 get_results：历史命令结果查询
// 修改：2026-09-03（日期由 fresh-header.ps1 刷新）

package getresults

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
	tool := mcp.NewTool("get_results",
		mcp.WithDescription("Get the history and results of previously executed commands for a user."),
		mcp.WithNumber("user_id",
			mcp.Required(),
			mcp.Description("The identifier of the calling user."),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of results to return (default 10)."),
		),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		userID, err := req.RequireInt("user_id")
		if err != nil {
			return tools.ResultError("invalid user_id: " + err.Error()), nil
		}
		limit := req.GetInt("limit", 10)

		sessionID := tools.SessionID(ctx)
		start := time.Now()

		if deps.Store == nil {
			tools.RecordOperation(ctx, deps.Store, sessionID, "get_results", "", "success", "", map[string]any{"count": 0})
			return tools.Result(map[string]any{"success": true, "data": []any{}}), nil
		}

		results, err := deps.Store.GetCommands(ctx, userID, limit)
		if err != nil {
			slog.Error("get_results failed", "user", userID, "limit", limit, "session", sessionID, "error", err, "duration", time.Since(start).String())
			tools.RecordOperation(ctx, deps.Store, sessionID, "get_results", "", "failed", err.Error(), nil)
			return tools.ResultError(err.Error()), nil
		}

		slog.Info("get_results ok", "user", userID, "limit", limit, "returned", len(results), "session", sessionID, "duration", time.Since(start).String())
		tools.RecordOperation(ctx, deps.Store, sessionID, "get_results", "", "success", "", map[string]any{"count": len(results)})
		return tools.Result(map[string]any{"success": true, "data": results}), nil
	})
}
