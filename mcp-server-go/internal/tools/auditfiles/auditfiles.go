// 文件：mcp-server-go/internal/tools/auditfiles/auditfiles.go —— MCP 工具 audit_files：管理员只读盘库对账
// 修改：2026-10-08（日期由 fresh-header.ps1 刷新）

package auditfiles

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
	s.AddTool(mcp.NewTool("audit_files",
		mcp.WithDescription("Admin-only read-only audit of metadata and derived storage files. Reports orphan files, ghost metadata, and duplicate checksums."),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()
		p := tools.Principal(ctx)
		if p == nil || !p.IsAdmin() {
			return tools.Deny(ctx, "audit_files", "", "仅管理员可执行对账"), nil
		}
		if deps.Manager == nil {
			return tools.ResultError("manager not available"), nil
		}
		report, err := deps.Manager.Audit(ctx)
		if err != nil {
			slog.Error("audit_files failed", "tool", "audit_files", "session", tools.SessionID(ctx), "error", err, "duration", time.Since(start).String())
			tools.RecordOperation(ctx, deps.Store, tools.SessionID(ctx), "audit_files", "", "failed", err.Error(), nil)
			return tools.ResultError(err.Error()), nil
		}
		slog.Info("audit_files ok", "tool", "audit_files", "session", tools.SessionID(ctx), "orphans", len(report.Orphans), "ghosts", len(report.Ghosts), "duplicate_groups", len(report.DupChecksums), "duration", time.Since(start).String())
		tools.RecordOperation(ctx, deps.Store, tools.SessionID(ctx), "audit_files", "", "success", "", map[string]any{
			"orphans": len(report.Orphans), "ghosts": len(report.Ghosts), "duplicate_groups": len(report.DupChecksums),
		})
		return tools.Result(map[string]any{"success": true, "report": report}), nil
	})
}
