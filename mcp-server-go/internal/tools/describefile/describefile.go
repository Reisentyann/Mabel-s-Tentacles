// 文件：mcp-server-go/internal/tools/describefile/describefile.go —— MCP 工具 describe_file：描述三件套 + llm 字段（编排机同步入口，拒因当场回传）
// 修改：2026-09-06（日期由 fresh-header.ps1 刷新）

package describefile

import (
	"context"
	"encoding/json"
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

		if deps.Orch == nil {
			return tools.ResultError("orchestrator unavailable"), nil
		}

		// 编排机同步入口（与 HTTP describe 同一份实现，原复制粘贴已灭）：
		// 存在性校验 → LLMStore 闸门（cod-* 只读 / 受控词表 / null 墓碑）→
		// 单次 Upsert → 喂索引（llm-* 是可索引字段，原先不喂的漂移洞已堵）。
		// 同步是因为拒绝列表必须当场回传给模型自纠错。
		attrs := map[string]any{}
		if raw := req.GetString("attributes", ""); raw != "" {
			if err := json.Unmarshal([]byte(raw), &attrs); err != nil {
				return tools.ResultError("invalid attributes JSON: " + err.Error()), nil
			}
		}
		res, derr := deps.Orch.Describe(ctx, core.DescribeRequest{
			Path:        filePath,
			Title:       tools.StrPtr(title),
			Description: tools.StrPtr(description),
			Tags:        tags,
			FileType:    tools.StrPtr(fileType),
			Mode:        mode,
			Attributes:  attrs,
		}, sessionID)
		if derr != nil {
			slog.Error("describe_file failed",
				"path", filePath, "session", sessionID,
				"error", derr, "duration", time.Since(start).String())
			return tools.ResultError(derr.Error()), nil
		}

		slog.Info("describe_file ok",
			"path", filePath, "mode", mode, "rejected", len(res.Rejected),
			"session", sessionID, "duration", time.Since(start).String())
		tools.RecordOperation(ctx, deps.Store, sessionID, "describe_file", filePath, "success", "", map[string]any{"description": description, "tags": tags, "file_type": fileType, "mode": mode})
		result := map[string]any{"success": true, "message": "Successfully described " + filePath}
		if len(res.Rejected) > 0 {
			result["rejected"] = res.Rejected // 回传拒绝原因，模型可自纠错
		}
		return tools.Result(result), nil
	})
}
