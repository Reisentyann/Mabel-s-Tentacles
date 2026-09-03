// 文件：mcp-server-go/internal/tools/executecommand/executecommand.go —— MCP 工具 execute_command：Shell 执行 + 记录入库
// 修改：2026-09-03（日期由 fresh-header.ps1 刷新）

package executecommand

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
	tool := mcp.NewTool("execute_command",
		mcp.WithDescription("Execute a shell command. Use this when the user's intent is to run a command or perform an action on the system."),
		mcp.WithString("command",
			mcp.Required(),
			mcp.Description("The shell command to execute."),
		),
		mcp.WithNumber("user_id",
			mcp.Required(),
			mcp.Description("The identifier of the calling user."),
		),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		command, err := req.RequireString("command")
		if err != nil {
			return tools.ResultError("invalid command: " + err.Error()), nil
		}
		userID, err := req.RequireInt("user_id")
		if err != nil {
			return tools.ResultError("invalid user_id: " + err.Error()), nil
		}

		sessionID := tools.SessionID(ctx)
		start := time.Now()

		var commandID int64
		if deps.Store != nil {
			if commandID, err = deps.Store.InsertCommand(ctx, userID, command); err != nil {
				slog.Error("insert command record failed", "command", command, "user", userID, "session", sessionID, "error", err)
				return tools.ResultError("Database error: " + err.Error()), nil
			}
		}

		exitCode, stdout, stderr := service.ExecuteCommand(ctx, command, 0)

		status := "done"
		if exitCode != 0 {
			status = "error"
		}
		if deps.Store != nil {
			if err := deps.Store.UpdateCommand(ctx, commandID, status, stdout, stderr, exitCode); err != nil {
				slog.Error("update command record failed", "command_id", commandID, "session", sessionID, "error", err)
			}
		}

		slog.Info("execute_command finished",
			"command", command, "user", userID, "session", sessionID,
			"exit_code", exitCode, "bytes_out", len(stdout), "bytes_err", len(stderr),
			"duration", time.Since(start).String())

		opStatus := "success"
		if exitCode != 0 {
			opStatus = "failed"
		}
		tools.RecordOperation(ctx, deps.Store, sessionID, "execute_command", "", opStatus, "", map[string]any{"command": command, "exit_code": exitCode})

		return tools.Result(map[string]any{
			"success":   exitCode == 0,
			"exit_code": exitCode,
			"stdout":    stdout,
			"stderr":    stderr,
		}), nil
	})
}
