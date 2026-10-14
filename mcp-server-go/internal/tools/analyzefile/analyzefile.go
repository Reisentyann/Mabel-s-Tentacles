// 文件：mcp-server-go/internal/tools/analyzefile/analyzefile.go —— MCP 工具 analyze_file：T3 手动重分析（manager updater 域）
// 修改：2026-10-14（日期由 fresh-header.ps1 刷新）

package analyzefile

import (
	"context"
	"log/slog"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/Reisentyann/Mabel-s-Tentacles/manager-go"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/tools"
)

func init() {
	tools.Register(register)
}

func register(s *server.MCPServer, deps tools.Deps) {
	tool := mcp.NewTool("analyze_file",
		mcp.WithDescription("Re-run the deterministic describer on an existing file's disk bytes to refresh 'cod-*' physical facts ( preserving all existing 'llm-*' semantic annotations). "+
			"[BOUNDARY] Only needed when a file was changed outside MCP tools (e.g. by execute_command) or after a describer engine upgrade. To add human/LLM tags, descriptions or 'llm-*' attributes, use describe_file instead."),
		mcp.WithString("file_path",
			mcp.Description("Path of the file to analyze, relative to workspace. Also accepts 'path'."),
		),
		mcp.WithString("path",
			mcp.Description("Alias for file_path."),
		),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		filePath, err := tools.GetFilePath(req)
		if err != nil {
			return tools.ResultError("invalid file_path: " + err.Error()), nil
		}
		if deps.Manager == nil {
			return tools.ResultError("manager not available"), nil
		}

		sessionID := tools.SessionID(ctx)
		start := time.Now()

		// 键空间（owner 隔离批次）：无前缀 = 自己；~A/ = 跨用户寻址
		// （重分析是写操作，跨用户由行内 owner 矩阵拒）
		sc, serr := tools.ScopePath(ctx, filePath)
		if serr != nil {
			return tools.Deny(ctx, "analyze_file", filePath, serr.Error()), nil
		}
		key := sc.Key

		// 写授权：重分析改的是文件元数据（整族刷新），owner/admin 之外拒绝
		if denied, reason := tools.CanFile(ctx, deps.Store, key, true); denied {
			tools.RecordOperation(ctx, deps.Store, sessionID, "analyze_file", key, "denied", reason, nil)
			return tools.Deny(ctx, "analyze_file", key, reason), nil
		}

		var (
			report *manager.AnalyzeReport
			aerr   error
		)
		if deps.Orch != nil {
			report, aerr = deps.Orch.AnalyzeFile(ctx, deps.Manager, key)
		} else {
			report, aerr = deps.Manager.AnalyzeFile(ctx, key)
		}
		if aerr != nil {
			slog.Error("analyze_file failed", "path", key, "session", sessionID, "error", aerr, "duration", time.Since(start).String())
			tools.RecordOperation(ctx, deps.Store, sessionID, "analyze_file", key, "failed", aerr.Error(), map[string]any{"file_path": filePath})
			return tools.ResultError(aerr.Error()), nil
		}

		slog.Info("analyze_file ok", "path", key, "session", sessionID, "families", report.Families, "cod_keys", len(report.Attrs), "duration", time.Since(start).String())
		tools.RecordOperation(ctx, deps.Store, sessionID, "analyze_file", key, "success", "", map[string]any{"families": report.Families})
		return tools.Result(map[string]any{
			"success":  true,
			"message":  "Successfully analyzed " + key,
			"path":     report.Path,
			"families": report.Families,
			"attrs":    report.Attrs,
		}), nil
	})
}
