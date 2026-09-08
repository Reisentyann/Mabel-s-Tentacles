// 文件：mcp-server-go/internal/tools/analyzefile/analyzefile.go —— MCP 工具 analyze_file：T3 手动重分析（manager updater 域）
// 修改：2026-09-08（日期由 fresh-header.ps1 刷新）

package analyzefile

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
	tool := mcp.NewTool("analyze_file",
		mcp.WithDescription("Re-run the deterministic describer on an existing file and store fresh cod-* facts (metadata refresh). "+
			"Use this when a file was changed outside write_file/modify_data_file (e.g. by execute_command), or to upgrade old metadata to the current engine version. "+
			"Returns the fact families hit and the newly produced attributes."),
		mcp.WithString("file_path",
			mcp.Required(),
			mcp.Description("Path of the file, relative to the data directory."),
		),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		filePath, err := req.RequireString("file_path")
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

		report, err := deps.Manager.AnalyzeFile(ctx, key)
		if err != nil {
			slog.Error("analyze_file failed", "path", key, "session", sessionID, "error", err, "duration", time.Since(start).String())
			tools.RecordOperation(ctx, deps.Store, sessionID, "analyze_file", key, "failed", err.Error(), map[string]any{"file_path": filePath})
			return tools.ResultError(err.Error()), nil
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
