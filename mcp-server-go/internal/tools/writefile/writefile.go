// 文件：mcp-server-go/internal/tools/writefile/writefile.go —— MCP 工具 write_file：写文件 + 内联描述字段随编排机事件异步落库
// 修改：2026-10-14（日期由 fresh-header.ps1 刷新）

package writefile

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/Reisentyann/Mabel-s-Tentacles/common"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/core"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/repo"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/service"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/tools"
)

func init() {
	tools.Register(register)
	tools.Register(registerUpload)
}

func register(s *server.MCPServer, deps tools.Deps) {
	tool := mcp.NewTool("write_file",
		mcp.WithDescription("Create a new text/code file in your workspace. "+
			"[BOUNDARY] Only for creating NEW text files (existing paths are rejected; to edit an existing file use patch_file or modify_file; to upload binary/images/base64 or import a local file use upload_file). "+
			"Pass title/description/tags/attributes inline so the file is immediately indexed and searchable."),
		mcp.WithString("file_path",
			mcp.Description("Logical path of the new file relative to your workspace, e.g. 'notes/plan.md'. Also accepts 'path'."),
		),
		mcp.WithString("path",
			mcp.Description("Alias for file_path."),
		),
		mcp.WithString("content",
			mcp.Required(),
			mcp.Description("Text content to write to the file."),
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
			mcp.Description("Who can see this file: 'public' (default, anyone can read/edit) or 'private' (only you and admin)."),
		),
		mcp.WithString("attributes",
			mcp.Description(`Optional JSON object of LLM semantic fields, e.g. {"llm-semantic-type":"technical_doc","sp-llm-topic":"architecture"}. Do NOT pass cod-* fields.`),
		),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		filePath, err := tools.GetFilePath(req)
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
			slog.Error("write_file failed", "tool", "write_file", "path", key, "bytes", len(content), "session", sessionID, "error", err, "duration", time.Since(start).String())
			tools.RecordOperation(ctx, deps.Store, sessionID, "write_file", key, "failed", err.Error(), params)
			return tools.ResultError(err.Error()), nil
		}

		slog.Info("write_file ok", "tool", "write_file", "path", key, "bytes", len(content), "uuid", receipt.UUID, "session", sessionID, "duration", time.Since(start).String())

		// 持久化维护意图：盘写成功后先落库记录，进程重启后由 RetryMaintenance 接手。
		// 与后续 Submit 绑定 jobID：worker 正常执行完置 done，失败或丢弃置 retry/ready。
		var jobID int64
		if ms, ok := deps.Store.(repo.MaintenanceStore); ok {
			id, perr := ms.PrepareMaintenance(ctx, receipt.UUID, "write", sessionID, tools.Actor(ctx).Name)
			if perr != nil {
				slog.Warn("write_file prepare maintenance failed", "tool", "write_file", "path", key, "uuid", receipt.UUID, "session", sessionID, "error", perr)
			} else {
				jobID = id
			}
		}

		// 编排机异步接管 T1（盘写成功即回，agent 不等描述）：agent 顺带
		// 描述字段随事件走，执行器单次 Upsert 落库并喂索引——旧的双 upsert 已灭。
		// Actor/Visibility 为归属与可见性打标（两级模型：缺省 public 共享，
		// 私密是显式选择）。
		// 初次描述仍使用既有异步事件链。
		var inlineAttrs map[string]any
		if rawAttrs := req.GetString("attributes", ""); rawAttrs != "" {
			if err := json.Unmarshal([]byte(rawAttrs), &inlineAttrs); err != nil {
				return tools.ResultError("invalid attributes JSON: " + err.Error()), nil
			}
		}

		if deps.Orch != nil {
			deps.Orch.Submit(core.Event{
				Kind: core.KindWrite, Path: key, UUID: receipt.UUID, MaintenanceJobID: jobID, SessionID: sessionID, Actor: tools.Actor(ctx),
				Visibility: req.GetString("visibility", ""),
				Agent:      &core.AgentMeta{Title: common.StrPtr(title), Description: common.StrPtr(description), Tags: tags, FileType: common.StrPtr(fileType), Attributes: inlineAttrs},
			})
		} else if jobID > 0 {
			if ms, ok := deps.Store.(repo.MaintenanceStore); ok {
				_ = ms.FinishMaintenance(ctx, jobID, "ready", "orchestrator not wired")
			}
		}
		tools.RecordOperation(ctx, deps.Store, sessionID, "write_file", key, "success", "", params)
		result := map[string]any{"success": true, "uuid": receipt.UUID}
		// 下载地址（2026-09-08）：message 里的链接用代码框包裹——聊天
		// 软件的 Markdown 渲染会剥掉 URL 里的 _xx_（斜体标记）与百分号
		// 转义（QQ 实测：invitation_to_lilywhite → invitationtolilywhite），
		// 代码框内的 URL 原样呈现，用户复制框内链接即完整无损；
		// 入口未配置时不卡回执——降级提示 agent 稍后走读文件/元数据接口自取
		var dlURL string
		// 优先签发 /d/{code} 极简短链（24小时有效，对朋友转发极度友好，无乱码与断行）
		if deps.Store != nil && deps.Cfg != nil {
			dlBase := strings.TrimRight(deps.Cfg.API.DownloadBaseURL, "/")
			if dlBase == "" {
				dlBase = deps.Cfg.Server.BaseURL
			}
			if shortURL, err := service.IssueShortURL(ctx, deps.Store, dlBase, key, receipt.UUID, 24*time.Hour); err == nil {
				dlURL = shortURL
			}
		}
		// 降级回退到常规票据下载地址
		if dlURL == "" && deps.Manager != nil {
			dlURL = deps.Manager.IssueDownloadURL(key, receipt.UUID, 0)
		}
		if dlURL != "" {
			result["download_url"] = dlURL
			result["message"] = "Successfully wrote to " + key + ". 下载短链（24小时有效，可直接发给朋友）：\n" + dlURL
		} else {
			result["message"] = "Successfully wrote to " + key + "（下载链接暂不可用：请稍后调用 read_file 或查询文件元数据接口获取下载地址）"
		}
		return tools.Result(result), nil
	})
}

func registerUpload(s *server.MCPServer, deps tools.Deps) {
	tool := mcp.NewTool("upload_file",
		mcp.WithDescription("Upload an image or binary/archive file into your workspace, and automatically run the describer engine (extracting image resolution, aspect ratio, color palette, brightness, EXIF, etc.) and index it. "+
			"[BOUNDARY] Provide EITHER 'content_base64' (raw base64 or data:image/...;base64,... URL, max 5MB decoded) OR 'source_path' (a local file path on the server machine to import). "+
			"For plain text files, prefer write_file. Pass title/description/tags/attributes inline so the uploaded image or file is immediately searchable."),
		mcp.WithString("file_path",
			mcp.Description("Target logical path in your workspace (must include extension, e.g. 'images/avatar.png' or ' illustrations/mabel.webp'). Also accepts 'path'."),
		),
		mcp.WithString("path",
			mcp.Description("Alias for file_path."),
		),
		mcp.WithString("content_base64",
			mcp.Description("Base64-encoded file bytes (or full data URL like 'data:image/png;base64,iVBOR...'). Max 5MB after decoding."),
		),
		mcp.WithString("source_path",
			mcp.Description("Alternative to content_base64: absolute or relative path of an existing local file on the host to import into Mabel (admin/master key only for arbitrary host paths)."),
		),
		mcp.WithString("title",
			mcp.Description("Short title of the image/file."),
		),
		mcp.WithString("description",
			mcp.Description("Free-text description of the image/file content."),
		),
		mcp.WithString("tags",
			mcp.Description("Comma-separated tags, e.g. 'illustration,anime,purple'."),
		),
		mcp.WithString("file_type",
			mcp.Description("Optional file type override (e.g. image / text / code / other). Defaults to inferred from extension."),
		),
		mcp.WithString("visibility",
			mcp.Description("'public' (default) or 'private'."),
		),
		mcp.WithString("attributes",
			mcp.Description(`Optional JSON object of LLM semantic fields, e.g. {"llm-semantic-type":"illustration","llm-characters":["梅贝尔"],"llm-style":"赛博机能"}. Do NOT pass cod-* fields.`),
		),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		filePath, err := tools.GetFilePath(req)
		if err != nil {
			return tools.ResultError("invalid file_path: " + err.Error()), nil
		}
		b64 := strings.TrimSpace(req.GetString("content_base64", ""))
		srcPath := strings.TrimSpace(req.GetString("source_path", ""))
		if (b64 == "" && srcPath == "") || (b64 != "" && srcPath != "") {
			return tools.ResultError("must provide exactly one of 'content_base64' or 'source_path'"), nil
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
		var inlineAttrs map[string]any
		if rawAttrs := req.GetString("attributes", ""); rawAttrs != "" {
			if err := json.Unmarshal([]byte(rawAttrs), &inlineAttrs); err != nil {
				return tools.ResultError("invalid attributes JSON: " + err.Error()), nil
			}
		}

		sessionID := tools.SessionID(ctx)
		start := time.Now()
		mode := "base64"
		if srcPath != "" {
			mode = "source_path"
		}
		params := map[string]any{"file_path": filePath, "mode": mode, "has_description": description != "" || len(tags) > 0}

		sc, serr := tools.ScopeWrite(ctx, filePath)
		if serr != nil {
			tools.RecordOperation(ctx, deps.Store, sessionID, "upload_file", filePath, "denied", serr.Error(), params)
			return tools.Deny(ctx, "upload_file", filePath, serr.Error()), nil
		}
		key := sc.Key

		if denied, reason := tools.CanFile(ctx, deps.Store, key, true); denied {
			tools.RecordOperation(ctx, deps.Store, sessionID, "upload_file", key, "denied", reason, params)
			return tools.Deny(ctx, "upload_file", key, reason), nil
		}
		if deps.Manager == nil {
			return tools.ResultError("manager not wired"), nil
		}

		var (
			fileUUID  string
			sizeBytes int64
		)
		if b64 != "" {
			data, derr := DecodeBase64Payload(b64)
			if derr != nil {
				tools.RecordOperation(ctx, deps.Store, sessionID, "upload_file", key, "failed", derr.Error(), params)
				return tools.ResultError("invalid content_base64: " + derr.Error()), nil
			}
			receipt, werr := deps.Manager.WriteBytes(ctx, key, data)
			if werr != nil {
				slog.Error("upload_file write bytes failed", "tool", "upload_file", "path", key, "bytes", len(data), "session", sessionID, "error", werr, "duration", time.Since(start).String())
				tools.RecordOperation(ctx, deps.Store, sessionID, "upload_file", key, "failed", werr.Error(), params)
				return tools.ResultError(werr.Error()), nil
			}
			fileUUID = receipt.UUID
			sizeBytes = receipt.SizeBytes
		} else {
			p := tools.Principal(ctx)
			if p == nil || !p.IsAdmin() {
				reason := "source_path host file import requires admin/master key; use content_base64 instead"
				tools.RecordOperation(ctx, deps.Store, sessionID, "upload_file", key, "denied", reason, params)
				return tools.Deny(ctx, "upload_file", key, reason), nil
			}
			receipt, ierr := deps.Manager.ImportFile(ctx, key, srcPath)
			if ierr != nil {
				slog.Error("upload_file import failed", "tool", "upload_file", "path", key, "source_path", srcPath, "session", sessionID, "error", ierr, "duration", time.Since(start).String())
				tools.RecordOperation(ctx, deps.Store, sessionID, "upload_file", key, "failed", ierr.Error(), params)
				return tools.ResultError(ierr.Error()), nil
			}
			fileUUID = receipt.UUID
			sizeBytes = receipt.SizeBytes
		}

		slog.Info("upload_file ok", "tool", "upload_file", "path", key, "mode", mode, "bytes", sizeBytes, "uuid", fileUUID, "session", sessionID, "duration", time.Since(start).String())

		var jobID int64
		if ms, ok := deps.Store.(repo.MaintenanceStore); ok {
			id, perr := ms.PrepareMaintenance(ctx, fileUUID, "write", sessionID, tools.Actor(ctx).Name)
			if perr != nil {
				slog.Warn("upload_file prepare maintenance failed", "tool", "upload_file", "path", key, "uuid", fileUUID, "session", sessionID, "error", perr)
			} else {
				jobID = id
			}
		}

		if deps.Orch != nil {
			deps.Orch.Submit(core.Event{
				Kind: core.KindWrite, Path: key, UUID: fileUUID, MaintenanceJobID: jobID, SessionID: sessionID, Actor: tools.Actor(ctx),
				Visibility: req.GetString("visibility", ""),
				Agent:      &core.AgentMeta{Title: common.StrPtr(title), Description: common.StrPtr(description), Tags: tags, FileType: common.StrPtr(fileType), Attributes: inlineAttrs},
			})
		} else if jobID > 0 {
			if ms, ok := deps.Store.(repo.MaintenanceStore); ok {
				_ = ms.FinishMaintenance(ctx, jobID, "ready", "orchestrator not wired")
			}
		}

		tools.RecordOperation(ctx, deps.Store, sessionID, "upload_file", key, "success", "", params)
		result := map[string]any{
			"success":      true,
			"uuid":         fileUUID,
			"logical_path": key,
			"size_bytes":   sizeBytes,
		}
		var dlURL string
		if deps.Store != nil && deps.Cfg != nil {
			dlBase := strings.TrimRight(deps.Cfg.API.DownloadBaseURL, "/")
			if dlBase == "" {
				dlBase = deps.Cfg.Server.BaseURL
			}
			if shortURL, err := service.IssueShortURL(ctx, deps.Store, dlBase, key, fileUUID, 24*time.Hour); err == nil {
				dlURL = shortURL
			}
		}
		if dlURL == "" && deps.Manager != nil {
			dlURL = deps.Manager.IssueDownloadURL(key, fileUUID, 0)
		}
		if dlURL != "" {
			result["download_url"] = dlURL
			result["message"] = fmt.Sprintf("Successfully uploaded %s (%d bytes, uuid=%s). 下载短链（24小时有效）：\n%s", key, sizeBytes, fileUUID, dlURL)
		} else {
			result["message"] = fmt.Sprintf("Successfully uploaded %s (%d bytes, uuid=%s)", key, sizeBytes, fileUUID)
		}
		return tools.Result(result), nil
	})
}

// DecodeBase64Payload 支持纯 Base64 字符串或 `data:<mime>;base64,<payload>` 格式。
func DecodeBase64Payload(raw string) ([]byte, error) {
	s := strings.TrimSpace(raw)
	if idx := strings.Index(s, ","); idx != -1 && strings.HasPrefix(strings.ToLower(s[:idx]), "data:") {
		s = s[idx+1:]
	}
	s = strings.NewReplacer("\r", "", "\n", "", " ", "").Replace(s)
	if s == "" {
		return nil, fmt.Errorf("empty base64 payload")
	}
	if data, err := base64.StdEncoding.DecodeString(s); err == nil {
		return data, nil
	}
	return base64.RawStdEncoding.DecodeString(s)
}
