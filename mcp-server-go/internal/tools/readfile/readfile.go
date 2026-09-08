// 文件：mcp-server-go/internal/tools/readfile/readfile.go —— MCP 工具 read_file：读文件（1MB 截断防上下文撑爆）
// 修改：2026-09-08（日期由 fresh-header.ps1 刷新）

package readfile

import (
	"context"
	"log/slog"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/tools"
)

// maxReadSize 单次返回给调用方的最大内容长度；超出部分截断并标记 truncated，
// 防止大文件把 LLM 上下文撑爆。
const maxReadSize = 1024 * 1024

func init() {
	tools.Register(register)
}

func register(s *server.MCPServer, deps tools.Deps) {
	tool := mcp.NewTool("read_file",
		mcp.WithDescription("Read the text content of a file in the data directory. "+
			"Recommended workflow: call list_data_files first (optionally with keyword q) to check each file's title/description/tags, "+
			"confirm this is really the file you need, then call read_file on it. "+
			"Avoid blind reads: wrong files waste context, and large files come back truncated."),
		mcp.WithString("path",
			mcp.Required(),
			mcp.Description("File path to read, relative to the data directory. Look it up via list_data_files first."),
		),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path, err := req.RequireString("path")
		if err != nil {
			return tools.ResultError("invalid path: " + err.Error()), nil
		}

		sessionID := tools.SessionID(ctx)
		start := time.Now()

		// 键空间（owner 隔离批次）：无前缀 = 自己；~A/ = 跨用户只读寻址
		sc, serr := tools.ScopePath(ctx, path)
		if serr != nil {
			return tools.Deny(ctx, "read_file", path, serr.Error()), nil
		}
		key := sc.Key

		// 读授权：看不到的文件不给读（跨用户寻址走行内 ACL；无元数据 =
		// 归属不明，仅管家/admin 可读）
		if denied, reason := tools.CanFile(ctx, deps.Store, key, false); denied {
			tools.RecordOperation(ctx, deps.Store, sessionID, "read_file", key, "denied", reason, map[string]any{"path": path})
			return tools.Deny(ctx, "read_file", key, reason), nil
		}

		// 取件走管理机（fetch 域逻辑口）：uuid 派生物理路径 + buffer 快路径
		if deps.Manager == nil {
			return tools.ResultError("manager not wired"), nil
		}
		rf, err := deps.Manager.ReadByLogic(ctx, key, maxReadSize)
		if err != nil {
			slog.Error("read_file failed", "path", key, "session", sessionID, "error", err, "duration", time.Since(start).String())
			tools.RecordOperation(ctx, deps.Store, sessionID, "read_file", key, "failed", err.Error(), map[string]any{"path": path})
			return tools.ResultError(err.Error()), nil
		}

		content := rf.Content
		truncated := rf.FileRef.SizeBytes > maxReadSize

		slog.Info("read_file ok", "path", key, "size", len(content), "truncated", truncated, "session", sessionID, "duration", time.Since(start).String())
		tools.RecordOperation(ctx, deps.Store, sessionID, "read_file", key, "success", "", map[string]any{"path": path, "truncated": truncated})

		return tools.Result(map[string]any{
			"success":   true,
			"path":      key,
			"size":      len(content),
			"truncated": truncated,
			"content":   string(content),
		}), nil
	})
}
