// 文件：mcp-server-go/internal/tools/copyfile/copyfile.go —— MCP 工具 copy_file：内容 + 元数据一起复制
// 修改：2026-09-05（日期由 fresh-header.ps1 刷新）

package copyfile

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
	tool := mcp.NewTool("copy_file",
		mcp.WithDescription("Copy a file (content and metadata) to a new path."),
		mcp.WithString("source",
			mcp.Required(),
			mcp.Description("Source file path, relative to the data directory."),
		),
		mcp.WithString("target",
			mcp.Required(),
			mcp.Description("Target file path, relative to the data directory."),
		),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		source, err := req.RequireString("source")
		if err != nil {
			return tools.ResultError("invalid source: " + err.Error()), nil
		}
		target, err := req.RequireString("target")
		if err != nil {
			return tools.ResultError("invalid target: " + err.Error()), nil
		}
		if source == target {
			return tools.ResultError("source and target must differ"), nil
		}

		sessionID := tools.SessionID(ctx)
		start := time.Now()

		content, err := service.SafeRead(deps.Cfg.DataDir, source)
		if err != nil {
			return tools.ResultError(err.Error()), nil
		}
		if err := service.SafeWrite(deps.Cfg.DataDir, target, string(content)); err != nil {
			return tools.ResultError(err.Error()), nil
		}

		if deps.Store != nil {
			if err := deps.Store.CopyMetadata(ctx, source, target, sessionID, ""); err != nil {
				// 源文件可能没有元数据，复制失败不致命；回填目标基础元数据，避免检索遗漏
				slog.Warn("copy metadata failed, recording basic meta", "source", source, "target", target, "session", sessionID, "error", err)
				tools.RecordFileMeta(ctx, deps, target, content, sessionID)
			}
		}

		slog.Info("copy_file ok", "source", source, "target", target, "bytes", len(content), "session", sessionID, "duration", time.Since(start).String())
		tools.RecordOperation(ctx, deps.Store, sessionID, "copy_file", target, "success", "", map[string]any{"source": source})
		return tools.Result(map[string]any{"success": true, "message": "Successfully copied " + source + " to " + target}), nil
	})
}
