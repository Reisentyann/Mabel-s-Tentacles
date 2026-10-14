// 文件：mcp-server-go/internal/tools/getresults/getresults.go —— MCP 工具 get_results：历史命令结果查询
// 修改：2026-09-17（日期由 fresh-header.ps1 刷新）

package getresults

import (
	"context"
	"log/slog"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/repo"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/tools"
)

func init() {
	tools.Register(register)
}

func register(s *server.MCPServer, deps tools.Deps) {
	handler := func(toolName string) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			limit := req.GetInt("limit", 10)
			if limit < 1 || limit > 100 {
				limit = 10
			}

			userID := req.GetInt("user_id", 0)
			p := tools.Principal(ctx)
			if userID == 0 && p != nil && p.UID > 0 {
				userID = int(p.UID)
			}

			sessionID := tools.SessionID(ctx)
			start := time.Now()

			if deps.Store == nil {
				tools.RecordOperation(ctx, deps.Store, sessionID, toolName, "", "success", "", map[string]any{"count": 0})
				return tools.Result(map[string]any{"success": true, "data": []any{}}), nil
			}

			var results []repo.CommandResult
			var err error
			if userID > 0 {
				results, err = deps.Store.GetCommands(ctx, userID, limit)
			} else if p != nil && p.IsAdmin() {
				// 管理员/管家在未指定特定用户时，查阅全局最新命令历史
				results, _, err = deps.Store.ListCommands(ctx, 1, limit)
			} else {
				results, err = deps.Store.GetCommands(ctx, 0, limit)
			}

			if err != nil {
				slog.Error(toolName+" failed", "user", userID, "limit", limit, "session", sessionID, "error", err, "duration", time.Since(start).String())
				tools.RecordOperation(ctx, deps.Store, sessionID, toolName, "", "failed", err.Error(), nil)
				return tools.ResultError(err.Error()), nil
			}

			slog.Info(toolName+" ok", "user", userID, "limit", limit, "returned", len(results), "session", sessionID, "duration", time.Since(start).String())
			tools.RecordOperation(ctx, deps.Store, sessionID, toolName, "", "success", "", map[string]any{"count": len(results)})
			return tools.Result(map[string]any{"success": true, "data": results}), nil
		}
	}

	toolOpts := []mcp.ToolOption{
		mcp.WithDescription("Get execution history and output of shell commands. Pass limit (default 10). user_id is automatically inferred."),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of results to return (default 10, max 100)."),
		),
		mcp.WithNumber("user_id",
			mcp.Description("Optional user identifier (automatically inferred if omitted)."),
		),
	}

	s.AddTool(mcp.NewTool("get_command_history", toolOpts...), handler("get_command_history"))
	s.AddTool(mcp.NewTool("get_results", toolOpts...), handler("get_results"))
}
