// 文件：mcp-server-go/internal/tools/writefile/writefile.go —— MCP 工具 write_file：写文件 + 内联描述字段随编排机事件异步落库
// 修改：2026-09-06（日期由 fresh-header.ps1 刷新）

package writefile

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/Reisentyann/Mabel-s-Tentacles/common"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/core"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/service"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/tools"
)

func init() {
	tools.Register(register)
}

func register(s *server.MCPServer, deps tools.Deps) {
	tool := mcp.NewTool("write_file",
		mcp.WithDescription("Write generated content to a file under the data directory. Pass title/description/tags so the file can be found later via search; without a description the file may become unfindable. Use this when the user wants to generate code, write an article, or create a file."),
		mcp.WithString("file_path",
			mcp.Required(),
			mcp.Description("Path of the file to write, relative to the data directory."),
		),
		mcp.WithString("content",
			mcp.Required(),
			mcp.Description("Content to write to the file."),
		),
		mcp.WithString("title",
			mcp.Description("Short title of the file, helps searchability."),
		),
		mcp.WithString("description",
			mcp.Description("Free-text description of the file content, enables keyword search later."),
		),
		mcp.WithString("tags",
			mcp.Description("Comma-separated tags, e.g. 'report,red'."),
		),
		mcp.WithString("file_type",
			mcp.Description("File type, e.g. text / image / code / other. Defaults to inferred from extension."),
		),
		mcp.WithString("visibility",
			mcp.Description("Who can see this file: 'private' (default, only you), 'public' (everyone can read), or 'group' (members of its group)."),
		),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		filePath, err := req.RequireString("file_path")
		if err != nil {
			return tools.ResultError("invalid file_path: " + err.Error()), nil
		}
		content, err := req.RequireString("content")
		if err != nil {
			return tools.ResultError("invalid content: " + err.Error()), nil
		}
		title := req.GetString("title", "")
		description := req.GetString("description", "")
		fileType := req.GetString("file_type", "")

		var tags []string
		if raw := req.GetString("tags", ""); raw != "" {
			for _, t := range strings.Split(raw, ",") {
				if t = strings.TrimSpace(t); t != "" {
					tags = append(tags, t)
				}
			}
		}

		sessionID := tools.SessionID(ctx)
		start := time.Now()
		params := map[string]any{"file_path": filePath, "content_size": len(content), "has_description": description != "" || len(tags) > 0}

		// 覆写授权：目标已有元数据行时按写矩阵判（新路径 = 创建，放行）
		if denied, reason := tools.CanFile(ctx, deps.Store, filePath, true); denied {
			tools.RecordOperation(ctx, deps.Store, sessionID, "write_file", filePath, "denied", reason, params)
			return tools.Deny(ctx, "write_file", filePath, reason), nil
		}

		if err := service.SafeWrite(deps.Cfg.DataDir, filePath, content); err != nil {
			slog.Error("write_file failed", "path", filePath, "session", sessionID, "error", err, "duration", time.Since(start).String())
			tools.RecordOperation(ctx, deps.Store, sessionID, "write_file", filePath, "failed", err.Error(), params)
			return tools.ResultError(err.Error()), nil
		}

		slog.Info("write_file ok", "path", filePath, "bytes", len(content), "session", sessionID, "duration", time.Since(start).String())

		// 编排机异步接管 T1（盘写成功即回，agent 不等描述）：agent 顺带
		// 描述字段随事件走，执行器单次 Upsert 落库并喂索引——旧的双 upsert 已灭。
		// Actor/Visibility 为权限批次的归属与可见性打标（默认 private）。
		// 事件可丢（容灾铁律 2）：队列满由 Submit 内部 WARN + T2 对账兜底。
		if deps.Orch != nil {
			deps.Orch.Submit(core.Event{
				Kind:       core.KindWrite,
				Path:       filePath,
				SessionID:  sessionID,
				Actor:      tools.Actor(ctx),
				Visibility: req.GetString("visibility", ""),
				Agent: &core.AgentMeta{
					Title:       common.StrPtr(title),
					Description: common.StrPtr(description),
					Tags:        tags,
					FileType:    common.StrPtr(fileType),
				},
			})
		}

		tools.RecordOperation(ctx, deps.Store, sessionID, "write_file", filePath, "success", "", params)
		result := map[string]any{"success": true, "message": "Successfully wrote to " + filePath}
		if u := tools.DownloadURL(deps.Cfg, filePath); u != "" {
			result["download_url"] = u
		}
		return tools.Result(result), nil
	})
}
