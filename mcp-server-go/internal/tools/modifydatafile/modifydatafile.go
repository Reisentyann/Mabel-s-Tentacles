// 文件：mcp-server-go/internal/tools/modifydatafile/modifydatafile.go —— MCP 工具 modify_data_file：append/overwrite + 编排机事件异步刷新元数据
// 修改：2026-09-17（日期由 fresh-header.ps1 刷新）

package modifydatafile

import (
	"context"
	"log/slog"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/core"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/tools"
)

func init() {
	tools.Register(register)
}

func register(s *server.MCPServer, deps tools.Deps) {
	handler := func(toolName string) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			filePath, err := tools.GetFilePath(req)
			if err != nil {
				return tools.ResultError("invalid file_path: " + err.Error()), nil
			}
			content, err := req.RequireString("content")
			if err != nil {
				return tools.ResultError("invalid content: " + err.Error()), nil
			}
			mode := req.GetString("mode", "append")

			sessionID := tools.SessionID(ctx)
			start := time.Now()
			params := map[string]any{"file_path": filePath, "mode": mode, "content_size": len(content)}

			// 键空间（owner 隔离批次）：modify 仅自己空间（跨用户写禁止）
			sc, serr := tools.ScopeWrite(ctx, filePath)
			if serr != nil {
				tools.RecordOperation(ctx, deps.Store, sessionID, toolName, filePath, "denied", serr.Error(), params)
				return tools.Deny(ctx, toolName, filePath, serr.Error()), nil
			}
			key := sc.Key

			// 写授权：modify 是对既有文件的动手操作，owner/组内/admin 之外拒绝
			if denied, reason := tools.CanFile(ctx, deps.Store, key, true); denied {
				tools.RecordOperation(ctx, deps.Store, sessionID, toolName, key, "denied", reason, params)
				return tools.Deny(ctx, toolName, key, reason), nil
			}

			// 修改走管理机（intake 域 Modify：逻辑键 → uuid 派生物理路径）
			if deps.Manager == nil {
				return tools.ResultError("manager not wired"), nil
			}
			if err := deps.Manager.Modify(ctx, key, content, mode); err != nil {
				slog.Error(toolName+" failed", "path", key, "mode", mode, "session", sessionID, "error", err, "duration", time.Since(start).String())
				tools.RecordOperation(ctx, deps.Store, sessionID, toolName, key, "failed", err.Error(), params)
				return tools.ResultError(err.Error()), nil
			}

			// 元数据刷新走编排机事件（异步）：worker 盘上重读终态重算
			// size/checksum/描述（后写胜出），此处不再整读文件，agent 即写即回
			if deps.Orch != nil {
				deps.Orch.Submit(core.Event{Kind: core.KindModify, Path: key, SessionID: sessionID, Actor: tools.Actor(ctx)})
			}

			slog.Info(toolName+" ok", "path", key, "mode", mode, "session", sessionID, "duration", time.Since(start).String())
			tools.RecordOperation(ctx, deps.Store, sessionID, toolName, key, "success", "", params)
			return tools.Result(map[string]any{"success": true, "message": "Successfully modified " + key + " in " + mode + " mode"}), nil
		}
	}

	toolOpts := []mcp.ToolOption{
		mcp.WithDescription("Modify an existing file. mode='append' appends content to the end of the file, mode='overwrite' replaces the whole file. Pass file_path (or path)."),
		mcp.WithString("file_path",
			mcp.Description("Path of the file to modify, relative to your workspace (e.g. 'notes/todo.txt')."),
		),
		mcp.WithString("path",
			mcp.Description("Alias for file_path."),
		),
		mcp.WithString("content",
			mcp.Required(),
			mcp.Description("Content to append or write."),
		),
		mcp.WithString("mode",
			mcp.Description("'append' or 'overwrite' (default 'append')."),
		),
	}

	// 注册主工具 modify_file + 兼容旧别名 modify_data_file
	s.AddTool(mcp.NewTool("modify_file", toolOpts...), handler("modify_file"))
	s.AddTool(mcp.NewTool("modify_data_file", toolOpts...), handler("modify_data_file"))
}
