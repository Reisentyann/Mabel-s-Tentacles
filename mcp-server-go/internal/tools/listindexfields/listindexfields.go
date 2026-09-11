// 文件：mcp-server-go/internal/tools/listindexfields/listindexfields.go —— MCP 索引查询双工具：list_index_fields（字段目录发现）+ search_files（条件查询）
// 修改：2026-09-11（日期由 fresh-header.ps1 刷新）

// 索引机对 agent 的两个查询入口同包落地（目录批次 2026-09-09）：
//   - list_index_fields 字段目录：一次调用拿到全部可查字段 + 桶型
//     （决定 op 集）+ 当前取值/值域。agent 的标准用法：先拿目录（一次
//     发现，可缓存）→ search_files 按字段值精准圈文件，两次调用封顶。
//     字段名是描述机的产出键（cod-<family>-<fact> 前缀规律），目录随
//     库内文件动态增减——新文件带新字段即自动出现，无需注册。
//   - search_files 条件查询：conditions JSON 数组（5 种 op 全量），
//     走编排机 SearchByConditions（索引优先 → eq/in 可降级 SQL）。
//     条件解析与 HTTP 层共用 internal/search.ParseConditions，语义永不漂移。
package listindexfields

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/Reisentyann/Mabel-s-Tentacles/common"
	"github.com/Reisentyann/Mabel-s-Tentacles/describer-go"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/repo"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/search"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/tools"
)

func init() {
	tools.Register(register)
	tools.Register(registerSearch)
}

func register(s *server.MCPServer, deps tools.Deps) {
	tool := mcp.NewTool("list_index_fields",
		mcp.WithDescription("Discover what the indexer can query right now: the catalog of indexed fields with their "+
			"kind (enum: try op eq/in; num: try op eq/in/gt/lt/range; multi: array-valued, eq/in match any element), "+
			"current values (enum/multi) or value range min..max (num), plus per-field desc (what it measures) "+
			"and bench (calibration tiers translating natural language like 'long article' or 'dialogue-heavy' into query values), "+
			"plus a guide (cross-field rules and a want-to-query quick reference). "+
			"Call this once before search_files to learn usable field names, meanings and value scales, then cache it; refresh only when searches miss. "+
			"Field names follow the describer pattern cod-<family>-<fact> (e.g. cod-text-language, cod-code-lang, cod-image-megapixels) or llm-* (model-supplied)."),
		mcp.WithString("prefix",
			mcp.Description("Optional field-name prefix filter to narrow the catalog, e.g. 'cod-text' or 'cod-image'."),
		),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		sessionID := tools.SessionID(ctx)
		begin := time.Now()
		prefix := req.GetString("prefix", "")

		if deps.Orch == nil {
			return tools.ResultError("orchestrator not wired"), nil
		}
		cat := deps.Orch.IndexCatalog()
		if cat == nil {
			slog.Warn("list_index_fields unavailable: index not wired", "session", sessionID, "duration", time.Since(begin).String())
			tools.RecordOperation(ctx, deps.Store, sessionID, "list_index_fields", "", "failed", "index not wired", map[string]any{"prefix": prefix})
			return tools.ResultError("索引机未装配（检索走 SQL 兜底，字段目录不可用）"), nil
		}

		fields := make([]map[string]any, 0, len(cat))
		keys, mounts := 0, 0
		for _, fi := range cat {
			if prefix != "" && !strings.HasPrefix(fi.Field, prefix) {
				continue
			}
			item := map[string]any{
				"field":  fi.Field,
				"kind":   string(fi.Kind),
				"keys":   fi.Keys,
				"mounts": fi.Mounts,
			}
			if b, ok := search.Bench(fi.Field); ok {
				item["desc"] = b.Desc   // 字段含义（量的是什么）
				item["bench"] = b.Bench // 分档基准（自然语言→查询值）
			}
			if fi.Values != nil {
				item["values"] = fi.Values
			}
			if fi.Truncated {
				item["truncated"] = true
			}
			if fi.Min != nil {
				item["min"] = *fi.Min
			}
			if fi.Max != nil {
				item["max"] = *fi.Max
			}
			fields = append(fields, item)
			keys += fi.Keys
			mounts += fi.Mounts
		}

		slog.Info("list_index_fields ok", "prefix", prefix, "fields", len(fields),
			"keys", keys, "mounts", mounts, "session", sessionID, "duration", time.Since(begin).String())
		tools.RecordOperation(ctx, deps.Store, sessionID, "list_index_fields", "", "success", "", map[string]any{"prefix": prefix, "fields": len(fields)})
		return tools.Result(map[string]any{
			"success": true,
			"fields":  fields,
			"guide":   search.TheGuide(), // 通用规则 + 自然语言→查询速查（契约的跨字段半边）
			"stats": map[string]any{
				"fields": len(fields),
				"keys":   keys,
				"mounts": mounts,
			},
			"hint": "用 search_files 按 field+op+value 查询；kind=enum/multi 可 eq/in，kind=num 可 eq/in/gt/lt/range（value=[lo,hi]）；多个条件默认 And，要 or/not 用 {and/or/not} 组合树嵌套；desc=字段含义，bench=分档基准（多长算长/多高算高）",
		}), nil
	})
}

// registerSearch MCP 工具 search_files：索引机条件查询——字段条件 →
// 命中文件列表（分页）。观察者口径与 HTTP 检索一致（非 admin 只命中
// 自己读得到的文件）；结果剥自己空间的 ~<主体名>/ 前缀（agent 寻址
// 语言：无前缀 = 自己空间，可直接 read_file）。
func registerSearch(s *server.MCPServer, deps tools.Deps) {
	tool := mcp.NewTool("search_files",
		mcp.WithDescription("Search files by indexed attribute conditions (the indexer's query entry). "+
			"conditions is EITHER a flat JSON array (all items AND-ed) like "+
			"[{\"field\":\"cod-text-language\",\"op\":\"eq\",\"value\":\"zh\"},{\"field\":\"cod-text-lines\",\"op\":\"gt\",\"value\":50}], "+
			"OR a boolean tree {\"and\":[...]} / {\"or\":[...]} / {\"not\":{...}} that nests arbitrarily — e.g. (Chinese OR English) AND long: "+
			"{\"and\":[{\"or\":[{\"field\":\"cod-text-cjk-ratio\",\"op\":\"gt\",\"value\":0.5},{\"field\":\"cod-text-language\",\"op\":\"eq\",\"value\":\"en\"}]},{\"field\":\"cod-text-lines\",\"op\":\"gt\",\"value\":200}]}. "+
			"Use or/not for alternatives and exclusions. "+
			"ops: eq (equals), in (value is an array, any match), gt / lt (numeric compare), range (value is [lo,hi]), "+
			"ne (not equals — only rows having the key; for rows missing the key use exists), "+
			"exists (value true/false — key presence; fields that are absent when a denominator is zero, like EXIF on generated images, are queried this way), "+
			"contains (substring on string fields; on array fields any element containing it matches). "+
			"Field names come from the describer (prefix pattern cod-<family>-<fact>, e.g. cod-text-language, cod-code-lang, cod-image-megapixels; llm-* for model-supplied tags). "+
			"Ordering: pass order_by=<field> and order=asc|desc (default desc) to sort hits by that value — with size=1 you get THE max/min file directly, no binary searching. "+
			"Best practice: before your first search, call list_index_fields once to learn usable fields, their meanings (desc), value calibrations (bench) and current values/ranges, then cache that catalog for the session — skip the discovery call if you already have it; refresh only when a search misses unexpectedly. "+
			"Returns paginated brief metadata (path/title/description/tags); read_file to fetch content."),
		mcp.WithString("conditions",
			mcp.Required(),
			mcp.Description(`Conditions: a flat JSON array (all AND-ed), e.g. [{"field":"cod-text-language","op":"eq","value":"zh"}], OR a boolean tree {"and":[...]} / {"or":[...]} / {"not":{...}} (nestable). Each leaf: {field, op, value}.`),
		),
		mcp.WithString("file_type",
			mcp.Description("Optional filter by file type, e.g. text / image / code."),
		),
		mcp.WithString("creator",
			mcp.Description("Optional filter by creator username."),
		),
		mcp.WithString("scope",
			mcp.Description("Optional filter by scope: global / user / game."),
		),
		mcp.WithNumber("page",
			mcp.Description("Page number, 1-based (default 1)."),
		),
		mcp.WithNumber("size",
			mcp.Description("Page size (default 20, max 100)."),
		),
		mcp.WithString("order_by",
			mcp.Description("Optional: sort hits by this attribute field's value (e.g. cod-text-lines for the longest file, cod-image-megapixels for the biggest image). Use with size=1 to get THE max/min file — no binary searching needed."),
		),
		mcp.WithString("order",
			mcp.Description("Sort direction with order_by: asc or desc (default desc = largest first)."),
		),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		sessionID := tools.SessionID(ctx)
		begin := time.Now()

		page := req.GetInt("page", 1)
		if page < 1 {
			page = 1
		}
		size := req.GetInt("size", 20)
		if size < 1 || size > 100 {
			size = 20
		}
		params := map[string]any{"page": page, "size": size}

		if deps.Orch == nil {
			return tools.ResultError("orchestrator not wired"), nil
		}

		expr, cerr := search.ParseConditions(rawOf(req))
		if cerr != nil {
			tools.RecordOperation(ctx, deps.Store, sessionID, "search_files", "", "failed", cerr.Error(), params)
			return tools.ResultError(cerr.Error()), nil
		}
		params["conditions"] = expr.Count()

		q := search.Query{
			FileType: req.GetString("file_type", ""),
			Creator:  req.GetString("creator", ""),
			Scope:    req.GetString("scope", ""),
			OrderBy:  req.GetString("order_by", ""),
			Order:    req.GetString("order", ""),
			Page:     page,
			Size:     size,
		}
		if q.Order != "" && q.Order != "asc" && q.Order != "desc" {
			tools.RecordOperation(ctx, deps.Store, sessionID, "search_files", "", "failed", "order 只认 asc / desc", params)
			return tools.ResultError("order 只认 asc / desc（order_by 配套参数）"), nil
		}
		if p := tools.Principal(ctx); p != nil && !p.IsAdmin() {
			q.ViewerName = p.Name
			q.ViewerGroups = p.GroupIDs
		}

		items, total, err := deps.Orch.SearchByConditions(ctx, expr, q)
		if err != nil {
			slog.Error("search_files failed", "conds", expr.Count(), "session", sessionID, "error", err, "duration", time.Since(begin).String())
			tools.RecordOperation(ctx, deps.Store, sessionID, "search_files", "", "failed", err.Error(), params)
			return tools.ResultError(err.Error()), nil
		}

		var viewer string
		if p := tools.Principal(ctx); p != nil {
			viewer = p.Name
		}
		files := make([]map[string]any, 0, len(items))
		for i := range items {
			files = append(files, briefOf(&items[i], viewer, q.OrderBy))
		}

		slog.Info("search_files ok", "conds", expr.Count(), "returned", len(files), "total", total,
			"page", page, "size", size, "viewer", viewer, "session", sessionID, "duration", time.Since(begin).String())
		tools.RecordOperation(ctx, deps.Store, sessionID, "search_files", "", "success", "", params)
		return tools.Result(map[string]any{
			"success": true,
			"page":    page,
			"size":    size,
			"total":   total,
			"files":   files,
		}), nil
	})
}

// rawOf 工具入参的 conditions 原值（string 或原生数组，由
// search.ParseConditions 统一归一）。
func rawOf(req mcp.CallToolRequest) any {
	return req.GetArguments()["conditions"]
}

// briefOf 命中行的简略视图（与 list_data_files 同字段面，path 剥掉
// 自己空间的 owner 前缀）。orderBy 非空时附 order_value（该行的排序
// 属性值——极值/Top-N 场景 agent 要向用户报告"最大是多少"）。
func briefOf(m *repo.FileMetadata, viewer, orderBy string) map[string]any {
	tags := m.Tags
	if tags == nil {
		tags = []string{}
	}
	out := map[string]any{
		"path":        stripOwnPrefix(m.FilePath, viewer),
		"title":       common.DerefStr(m.Title),
		"description": common.DerefStr(m.Description),
		"file_type":   common.DerefStr(m.FileType),
		"size_bytes":  common.DerefInt64(m.SizeBytes),
		"tags":        tags,
		"updated_at":  m.UpdatedAt,
	}
	if orderBy != "" {
		out["order_value"] = describer.AttrsFromJSON(m.Attributes)[orderBy]
	}
	return out
}

// stripOwnPrefix 剥自己空间的 ~<viewer>/ 前缀（自己空间无前缀寻址）；
// 他人空间 / 匿名观察保留原键。
func stripOwnPrefix(path, viewer string) string {
	if viewer == "" {
		return path
	}
	prefix := "~" + viewer + "/"
	if strings.HasPrefix(path, prefix) {
		return strings.TrimPrefix(path, prefix)
	}
	return path
}
