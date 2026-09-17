// 文件：mcp-server-go/internal/tools/patchfile/patchfile.go —— MCP 工具 patch_file：基于唯一锚点文本的微创局部替换
// 修改：2026-09-17（日期由 fresh-header.ps1 刷新）

package patchfile

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
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
	tool := mcp.NewTool("patch_file",
		mcp.WithDescription("Perform an exact string replacement in a file. Safer and faster than whole-file overwrite for small edits. The old_string must match exactly once in the file."),
		mcp.WithString("file_path",
			mcp.Description("Path of the file to patch (must be in your space). Also accepts 'path'."),
		),
		mcp.WithString("path",
			mcp.Description("Alias for file_path."),
		),
		mcp.WithString("old_string",
			mcp.Required(),
			mcp.Description("Exact text to find and replace. Provide sufficient surrounding context to ensure it is unique."),
		),
		mcp.WithString("new_string",
			mcp.Required(),
			mcp.Description("Replacement text."),
		),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		filePath, err := tools.GetFilePath(req)
		if err != nil {
			return tools.ResultError("invalid file_path: " + err.Error()), nil
		}
		oldString, err := req.RequireString("old_string")
		if err != nil {
			return tools.ResultError("invalid old_string: " + err.Error()), nil
		}
		newString, err := req.RequireString("new_string")
		if err != nil {
			return tools.ResultError("invalid new_string: " + err.Error()), nil
		}
		if oldString == newString {
			return tools.ResultError("old_string and new_string are identical"), nil
		}

		sessionID := tools.SessionID(ctx)
		start := time.Now()
		params := map[string]any{"file_path": filePath, "old_len": len(oldString), "new_len": len(newString)}

		// 键空间检查：只能修改自己空间文件
		sc, serr := tools.ScopeWrite(ctx, filePath)
		if serr != nil {
			tools.RecordOperation(ctx, deps.Store, sessionID, "patch_file", filePath, "denied", serr.Error(), params)
			return tools.Deny(ctx, "patch_file", filePath, serr.Error()), nil
		}
		key := sc.Key

		// 写授权检查
		if denied, reason := tools.CanFile(ctx, deps.Store, key, true); denied {
			tools.RecordOperation(ctx, deps.Store, sessionID, "patch_file", key, "denied", reason, params)
			return tools.Deny(ctx, "patch_file", key, reason), nil
		}

		if deps.Manager == nil {
			return tools.ResultError("manager not wired"), nil
		}

		// 读取当前文件内容（10MB 上限）
		rf, err := deps.Manager.ReadByLogic(ctx, key, 10*1024*1024)
		if err != nil {
			slog.Error("patch_file read failed", "path", key, "session", sessionID, "error", err, "duration", time.Since(start).String())
			tools.RecordOperation(ctx, deps.Store, sessionID, "patch_file", key, "failed", err.Error(), params)
			return tools.ResultError("failed to read file: " + err.Error()), nil
		}

		original := string(rf.Content)
		occurrences := strings.Count(original, oldString)
		if occurrences == 0 {
			msg := "old_string not found in file content"
			slog.Warn("patch_file old_string not found", "path", key, "session", sessionID, "duration", time.Since(start).String())
			tools.RecordOperation(ctx, deps.Store, sessionID, "patch_file", key, "failed", msg, params)
			return tools.ResultError(msg), nil
		}
		if occurrences > 1 {
			msg := fmt.Sprintf("old_string matched %d times; provide more surrounding lines to identify a unique match", occurrences)
			slog.Warn("patch_file old_string ambiguous", "path", key, "count", occurrences, "session", sessionID, "duration", time.Since(start).String())
			tools.RecordOperation(ctx, deps.Store, sessionID, "patch_file", key, "failed", msg, params)
			return tools.ResultError(msg), nil
		}

		// 唯一匹配，执行单次安全替换
		patched := strings.Replace(original, oldString, newString, 1)

		if err := deps.Manager.Modify(ctx, key, patched, "overwrite"); err != nil {
			slog.Error("patch_file modify failed", "path", key, "session", sessionID, "error", err, "duration", time.Since(start).String())
			tools.RecordOperation(ctx, deps.Store, sessionID, "patch_file", key, "failed", err.Error(), params)
			return tools.ResultError("failed to write patched content: " + err.Error()), nil
		}

		// 触发异步元数据刷新
		if deps.Orch != nil {
			deps.Orch.Submit(core.Event{Kind: core.KindModify, Path: key, SessionID: sessionID, Actor: tools.Actor(ctx)})
		}

		slog.Info("patch_file ok", "path", key, "session", sessionID, "duration", time.Since(start).String())
		tools.RecordOperation(ctx, deps.Store, sessionID, "patch_file", key, "success", "", params)
		return tools.Result(map[string]any{"success": true, "message": "Successfully patched " + key}), nil
	})
}
