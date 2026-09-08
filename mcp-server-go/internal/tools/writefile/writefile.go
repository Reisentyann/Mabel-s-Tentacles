// 文件：mcp-server-go/internal/tools/writefile/writefile.go —— MCP 工具 write_file：写文件 + 内联描述字段随编排机事件异步落库
// 修改：2026-09-08（日期由 fresh-header.ps1 刷新）

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

		// 键空间（owner 隔离批次）：无前缀 = 自己空间自动拼接；~A/ 跨用户写
		// 一律拒绝。覆写授权按行内 owner 判（新路径 = 创建，放行）。
		sc, serr := tools.ScopeWrite(ctx, filePath)
		if serr != nil {
			tools.RecordOperation(ctx, deps.Store, sessionID, "write_file", filePath, "denied", serr.Error(), params)
			return tools.Deny(ctx, "write_file", filePath, serr.Error()), nil
		}
		key := sc.Key

		// 覆写授权：目标已有元数据行时按写矩阵判（新路径 = 创建，放行）
		if denied, reason := tools.CanFile(ctx, deps.Store, key, true); denied {
			tools.RecordOperation(ctx, deps.Store, sessionID, "write_file", key, "denied", reason, params)
			return tools.Deny(ctx, "write_file", key, reason), nil
		}

		// 入库唯一口（intake 域 2026-09-08）：逻辑键 + uuid 派生物理随机路径
		// ——agent 不接触物理布局，盘面不可猜；描述落库归编排机事件（下方 Submit）
		if deps.Manager == nil {
			return tools.ResultError("manager not wired"), nil
		}
		receipt, err := deps.Manager.Write(ctx, key, content)
		if err != nil {
			slog.Error("write_file failed", "path", key, "session", sessionID, "error", err, "duration", time.Since(start).String())
			tools.RecordOperation(ctx, deps.Store, sessionID, "write_file", key, "failed", err.Error(), params)
			return tools.ResultError(err.Error()), nil
		}

		slog.Info("write_file ok", "path", key, "bytes", len(content), "uuid", receipt.UUID, "session", sessionID, "duration", time.Since(start).String())

		// 编排机异步接管 T1（盘写成功即回，agent 不等描述）：agent 顺带
		// 描述字段随事件走，执行器单次 Upsert 落库并喂索引——旧的双 upsert 已灭。
		// Actor/Visibility 为归属与可见性打标（两级模型：缺省 public 共享，
		// 私密是显式选择）。
		// 事件可丢（容灾铁律 2）：队列满由 Submit 内部 WARN + T2 对账兜底。
		if deps.Orch != nil {
			deps.Orch.Submit(core.Event{
				Kind:       core.KindWrite,
				Path:       key,
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

		tools.RecordOperation(ctx, deps.Store, sessionID, "write_file", key, "success", "", params)
		result := map[string]any{"success": true, "uuid": receipt.UUID}
		// 下载地址（2026-09-08）：message 里的链接用代码框包裹——聊天
		// 软件的 Markdown 渲染会剥掉 URL 里的 _xx_（斜体标记）与百分号
		// 转义（QQ 实测：invitation_to_lilywhite → invitationtolilywhite），
		// 代码框内的 URL 原样呈现，用户复制框内链接即完整无损；
		// 入口未配置时不卡回执——降级提示 agent 稍后走读文件/元数据接口自取
		if u := tools.DownloadURL(deps.Cfg, key, receipt.UUID); u != "" {
			result["download_url"] = u
			result["message"] = "Successfully wrote to " + key + ". 下载地址（请把下方代码框内的链接原样发给用户，不要拆开）：\n```\n" + u + "\n```"
		} else {
			result["message"] = "Successfully wrote to " + key + "（下载链接暂不可用：请稍后调用 read_file 或查询文件元数据接口获取下载地址）"
		}
		return tools.Result(result), nil
	})
}
