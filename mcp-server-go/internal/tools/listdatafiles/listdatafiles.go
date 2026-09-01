package listdatafiles

import (
	"context"
	"log/slog"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/repo"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/service"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/tools"
)

func init() {
	tools.Register(register)
}

func register(s *server.MCPServer, deps tools.Deps) {
	tool := mcp.NewTool("list_data_files",
		mcp.WithDescription("List files under the data directory with brief metadata, paginated to avoid flooding the context. Each item shows path/title/description/file_type/size/tags and has_description, so you can spot files lacking a description and maintain them via describe_file."),
		mcp.WithNumber("page",
			mcp.Description("Page number, 1-based (default 1)."),
		),
		mcp.WithNumber("size",
			mcp.Description("Page size (default 20, max 100)."),
		),
		mcp.WithString("q",
			mcp.Description("Optional keyword to filter by file path (substring match)."),
		),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		page := req.GetInt("page", 1)
		if page < 1 {
			page = 1
		}
		size := req.GetInt("size", 20)
		if size < 1 || size > 100 {
			size = 20
		}
		q := req.GetString("q", "")

		sessionID := tools.SessionID(ctx)

		all, err := service.SafeList(deps.Cfg.DataDir)
		if err != nil {
			slog.Error("list_data_files failed", "error", err)
			tools.RecordOperation(ctx, deps.Store, sessionID, "list_data_files", "", "failed", err.Error(), nil)
			return tools.ResultError(err.Error()), nil
		}

		// 可选：按路径关键词过滤
		filtered := all
		if q != "" {
			filtered = filtered[:0]
			for _, p := range all {
				if strings.Contains(p, q) {
					filtered = append(filtered, p)
				}
			}
		}

		total := len(filtered)
		start := (page - 1) * size
		if start >= total {
			tools.RecordOperation(ctx, deps.Store, sessionID, "list_data_files", "", "success", "", map[string]any{"count": 0, "page": page, "total": total})
			return tools.Result(map[string]any{
				"success": true,
				"page":    page,
				"size":    size,
				"total":   total,
				"files":   []any{},
			}), nil
		}
		end := start + size
		if end > total {
			end = total
		}
		pagePaths := filtered[start:end]

		// 批量联表：仅查本页 N 条元数据，避免全表扫描
		var metas map[string]*repo.FileMetadata
		if deps.Store != nil {
			metas, _ = deps.Store.GetMetadataByPaths(ctx, pagePaths)
		}

		items := make([]map[string]any, 0, len(pagePaths))
		for _, p := range pagePaths {
			m := metas[p]
			if m != nil && m.IsDeleted {
				continue // 软删文件不列出
			}
			item := map[string]any{
				"path":            p,
				"has_description": false,
			}
			if m != nil {
				title := ptrStr(m.Title)
				desc := ptrStr(m.Description)
				tags := m.Tags
				if tags == nil {
					tags = []string{}
				}
				item["title"] = title
				item["description"] = desc
				item["file_type"] = ptrStr(m.FileType)
				item["size_bytes"] = ptrInt64(m.SizeBytes)
				item["tags"] = tags
				item["updated_at"] = m.UpdatedAt
				item["has_description"] = title != "" || desc != "" || len(tags) > 0
			}
			items = append(items, item)
		}

		slog.Info("list_data_files ok", "page", page, "returned", len(items), "total", total)
		tools.RecordOperation(ctx, deps.Store, sessionID, "list_data_files", "", "success", "", map[string]any{"count": len(items), "page": page, "total": total})
		return tools.Result(map[string]any{
			"success": true,
			"page":    page,
			"size":    size,
			"total":   total,
			"files":   items,
		}), nil
	})
}

func ptrStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func ptrInt64(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}
