// 文件：mcp-server-go/internal/tools/statfile/statfile.go —— MCP 工具 get_file_info / stat_file：直查单文件完整元数据与事实属性
// 修改：2026-09-17（日期由 fresh-header.ps1 刷新）

package statfile

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/Reisentyann/Mabel-s-Tentacles/common"
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

			sessionID := tools.SessionID(ctx)
			start := time.Now()

			// 键空间寻址解析
			sc, serr := tools.ScopePath(ctx, filePath)
			if serr != nil {
				return tools.Deny(ctx, toolName, filePath, serr.Error()), nil
			}
			key := sc.Key

			// 读授权判定
			if denied, reason := tools.CanFile(ctx, deps.Store, key, false); denied {
				tools.RecordOperation(ctx, deps.Store, sessionID, toolName, key, "denied", reason, map[string]any{"file_path": filePath})
				return tools.Deny(ctx, toolName, key, reason), nil
			}

			if deps.Store == nil {
				return tools.ResultError("store not wired"), nil
			}

			m, err := deps.Store.GetMetadata(ctx, key)
			if err != nil {
				slog.Error(toolName+" get metadata failed", "path", key, "session", sessionID, "error", err, "duration", time.Since(start).String())
				tools.RecordOperation(ctx, deps.Store, sessionID, toolName, key, "failed", err.Error(), map[string]any{"file_path": filePath})
				return tools.ResultError("file not found: " + key), nil
			}
			if m == nil {
				slog.Warn(toolName+" not found", "path", key, "session", sessionID, "duration", time.Since(start).String())
				tools.RecordOperation(ctx, deps.Store, sessionID, toolName, key, "failed", "file not found", map[string]any{"file_path": filePath})
				return tools.ResultError("file not found: " + key), nil
			}
			if m.IsDeleted {
				slog.Warn(toolName+" file is deleted", "path", key, "session", sessionID, "duration", time.Since(start).String())
				tools.RecordOperation(ctx, deps.Store, sessionID, toolName, key, "failed", "file has been deleted", map[string]any{"file_path": filePath})
				return tools.ResultError("file has been deleted: " + key), nil
			}

			var attrs map[string]any
			if len(m.Attributes) > 0 {
				_ = json.Unmarshal(m.Attributes, &attrs)
			}
			if attrs == nil {
				attrs = map[string]any{}
			}

			data := map[string]any{
				"uuid":         m.UUID,
				"logical_path": m.FilePath,
				"size_bytes":   common.DerefInt64(m.SizeBytes),
				"checksum":     common.DerefStr(m.Checksum),
				"scope":        m.Scope,
				"visibility":   m.Visibility,
				"owner_id":     common.DerefStr(m.OwnerID),
				"title":        common.DerefStr(m.Title),
				"description":  common.DerefStr(m.Description),
				"tags":         m.Tags,
				"file_type":    common.DerefStr(m.FileType),
				"mime_type":    common.DerefStr(m.MimeType),
				"extension":    common.DerefStr(m.Extension),
				"created_at":   m.CreatedAt.Format(time.RFC3339),
				"updated_at":   m.UpdatedAt.Format(time.RFC3339),
				"lineage": map[string]any{
					"copied_from": common.DerefStr(m.CopiedFrom),
					"moved_from":  common.DerefStr(m.MovedFrom),
				},
				"attributes": attrs,
			}

			slog.Info(toolName+" ok", "path", key, "uuid", m.UUID, "session", sessionID, "duration", time.Since(start).String())
			tools.RecordOperation(ctx, deps.Store, sessionID, toolName, key, "success", "", map[string]any{"file_path": filePath})
			return tools.Result(map[string]any{"success": true, "data": data}), nil
		}
	}

	toolOpts := []mcp.ToolOption{
		mcp.WithDescription("Get full metadata and computed deterministic facts (90+ attributes) for a specific file. Returns size, timestamps, tags, lineage and attribute map."),
		mcp.WithString("file_path",
			mcp.Description("Logical file path, relative to workspace or ~username/path. Also accepts 'path'."),
		),
		mcp.WithString("path",
			mcp.Description("Alias for file_path."),
		),
	}

	s.AddTool(mcp.NewTool("get_file_info", toolOpts...), handler("get_file_info"))
	s.AddTool(mcp.NewTool("stat_file", toolOpts...), handler("stat_file"))
}
