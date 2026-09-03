// 文件：mcp-server-go/internal/tools/describefile/describefile.go —— MCP 工具 describe_file：描述三件套 + llm 字段（过 LLMStore 闸门）
// 修改：2026-09-03（日期由 fresh-header.ps1 刷新）

package describefile

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/Reisentyann/Mabel-s-Tentacles/describer-go"
	"github.com/Reisentyann/Mabel-s-Tentacles/describer-go/llm"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/repo"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/service"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/tools"
)

func init() {
	tools.Register(register)
}

func register(s *server.MCPServer, deps tools.Deps) {
	tool := mcp.NewTool("describe_file",
		mcp.WithDescription("Add description, tags, and semantic attributes to an existing file so it can be searched later. "+
			"attributes accepts a JSON object: llm-* fixed fields (llm-semantic-type in [novel,game_guide,technical_doc,note,log,meme,illustration,photo,screenshot,code_artifact,data,other], "+
			"llm-tone, llm-characters, llm-action, llm-style, llm-summary) and sp-llm-* free-form fields; set a value to null to delete that key. "+
			"cod-* fields are read-only engine facts and are always rejected."),
		mcp.WithString("file_path",
			mcp.Required(),
			mcp.Description("Path of the file, relative to the data directory."),
		),
		mcp.WithString("title",
			mcp.Description("Short title of the file."),
		),
		mcp.WithString("description",
			mcp.Description("Free-text description of the file."),
		),
		mcp.WithString("tags",
			mcp.Description("Comma-separated tags, e.g. 'report,red'."),
		),
		mcp.WithString("file_type",
			mcp.Description("File type, e.g. text / image / code / other."),
		),
		mcp.WithString("mode",
			mcp.Description("'append' adds the description as a new paragraph after the existing one; 'replace' (default) overwrites."),
		),
		mcp.WithString("attributes",
			mcp.Description(`Optional JSON object of LLM semantic fields, e.g. {"llm-semantic-type":"novel","llm-characters":["梅贝尔"],"sp-llm-游戏名":"狼人杀"}.`),
		),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		filePath, err := req.RequireString("file_path")
		if err != nil {
			return tools.ResultError("invalid file_path: " + err.Error()), nil
		}
		title := req.GetString("title", "")
		description := req.GetString("description", "")
		fileType := req.GetString("file_type", "")
		mode := req.GetString("mode", "replace")

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

		target, err := service.ResolvePath(deps.Cfg.DataDir, filePath)
		if err != nil {
			return tools.ResultError(err.Error()), nil
		}
		if info, err := os.Stat(target); err != nil || info.IsDir() {
			return tools.ResultError("error: file '" + filePath + "' does not exist"), nil
		}

		// 既有元数据：追加模式与 llm 字段合并都需要
		var existing map[string]any
		var oldDesc string
		if deps.Store != nil {
			if m, err := deps.Store.GetMetadata(ctx, filePath); err == nil && m != nil {
				existing = describer.AttrsFromJSON(m.Attributes)
				if m.Description != nil {
					oldDesc = *m.Description
				}
			}
		}
		if mode == "append" && oldDesc != "" && description != "" {
			description = oldDesc + "\n\n" + description
		}

		// llm 语义字段：LLMStore 中间件（唯一写入口）
		// cod-* 只读 / 审计字段系统专属 / 受控词表 / null 墓碑删除
		var rejectedList []string
		if raw := req.GetString("attributes", ""); raw != "" {
			var in map[string]any
			if err := json.Unmarshal([]byte(raw), &in); err != nil {
				return tools.ResultError("invalid attributes JSON: " + err.Error()), nil
			}
			st := llm.OpenLLM()
			st.SetMany(in)
			for _, r := range st.Rejected() {
				slog.Warn("describe_file attribute rejected by llm middleware",
					"path", filePath, "session", sessionID, "key", r.Key, "op", r.Op, "reason", r.Reason)
				rejectedList = append(rejectedList, r.Key+"("+r.Op+"): "+r.Reason)
			}
			existing = st.Commit(existing, llm.LLMSourceAgent, time.Now())
		}

		meta := &repo.FileMetadata{
			FilePath:    filePath,
			Title:       tools.StrPtr(title),
			Description: tools.StrPtr(description),
			Tags:        tags,
			FileType:    tools.StrPtr(fileType),
			SessionID:   tools.StrPtr(sessionID),
			Attributes:  describer.JSONFromAttrs(existing),
		}
		if deps.Store != nil {
			if err := deps.Store.UpsertMetadata(ctx, meta); err != nil {
				slog.Error("describe_file failed", "path", filePath, "session", sessionID, "error", err, "duration", time.Since(start).String())
				return tools.ResultError("Database error: " + err.Error()), nil
			}
		}

		slog.Info("describe_file ok", "path", filePath, "mode", mode, "rejected", len(rejectedList), "session", sessionID, "duration", time.Since(start).String())
		tools.RecordOperation(ctx, deps.Store, sessionID, "describe_file", filePath, "success", "", map[string]any{"description": description, "tags": tags, "file_type": fileType, "mode": mode})
		result := map[string]any{"success": true, "message": "Successfully described " + filePath}
		if len(rejectedList) > 0 {
			result["rejected"] = rejectedList // 回传拒绝原因，模型可自纠错
		}
		return tools.Result(result), nil
	})
}
