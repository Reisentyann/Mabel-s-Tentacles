// 文件：mcp-server-go/internal/tools/copyfile/copyfile.go —— MCP 工具 copy_file：内容 + 元数据一起复制（KindCopy 事件喂索引）
// 修改：2026-09-06（日期由 fresh-header.ps1 刷新）

package copyfile

import (
	"context"
	"log/slog"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/core"
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

		// 源读授权：看不到的文件不允许复制（含无元数据的归属不明文件）
		if denied, reason := tools.CanFile(ctx, deps.Store, source, false); denied {
			tools.RecordOperation(ctx, deps.Store, sessionID, "copy_file", source, "denied", reason, map[string]any{"source": source})
			return tools.Deny(ctx, "copy_file", source, reason), nil
		}

		content, err := service.SafeRead(deps.Cfg.DataDir, source)
		if err != nil {
			return tools.ResultError(err.Error()), nil
		}
		if err := service.SafeWrite(deps.Cfg.DataDir, target, string(content)); err != nil {
			return tools.ResultError(err.Error()), nil
		}

		owner := ""
		if p := tools.Principal(ctx); p != nil {
			owner = p.Name
		}
		if deps.Store != nil {
			// 副本归操作者（拿走即拥有，copied_from 保留谱系）
			if err := deps.Store.CopyMetadata(ctx, source, target, owner, sessionID); err != nil {
				// 源文件可能没有元数据，复制失败不致命：KindCopy 事件的执行器
				// 会从盘上重建目标元数据并喂索引（COALESCE 保留 copied_from 谱系列）
				slog.Warn("copy metadata failed, orchestrator will rebuild target meta",
					"source", source, "target", target, "session", sessionID, "error", err)
			}
		}
		// 复制主路径原本的喂食洞（目标 uuid 从不挂索引）由此堵上：
		// 执行器重分析目标 + Upsert + Sink.Update，CopyMetadata 成败与否都覆盖
		if deps.Orch != nil {
			deps.Orch.Submit(core.Event{Kind: core.KindCopy, Path: target, SessionID: sessionID, Actor: core.Actor{Name: owner}})
		}

		slog.Info("copy_file ok", "source", source, "target", target, "bytes", len(content), "session", sessionID, "duration", time.Since(start).String())
		tools.RecordOperation(ctx, deps.Store, sessionID, "copy_file", target, "success", "", map[string]any{"source": source})
		return tools.Result(map[string]any{"success": true, "message": "Successfully copied " + source + " to " + target}), nil
	})
}
