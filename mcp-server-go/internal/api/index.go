// 文件：mcp-server-go/internal/api/index.go —— 索引机 HTTP 端点：字段目录发现（GET /api/index/fields）+ 检索条件参数（searchFiles 的 cond 扩展）
// 修改：2026-09-11（日期由 fresh-header.ps1 刷新）

// 索引机能力的 HTTP 面（目录批次 2026-09-09，与 MCP 工具层同源）：
// 字段目录给前端/HTTP 工具做发现；cond 参数让 GET /api/files/search
// 从"color 一个硬编码属性"升级为全量条件语法（与 search_files 工具
// 同一份解析——internal/search.ParseConditions，语义永不漂移）。
package api

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/search"
)

// indexFields GET /api/index/fields?prefix=cod-text —— 索引机字段目录
// （HTTP 侧发现入口，对齐 MCP list_index_fields）。观察者口径取舍：目录
// 是字段聚合视图（字段名/桶型/取值枚举），不投影文件行——取值列表可能
// 含观察者不可见文件的值（语言/标签类枚举，非内容级敏感面）；
// 行级可见性由检索端点（cond 查询）的复判保证。
func (s *Server) indexFields(w http.ResponseWriter, r *http.Request) {
	begin := time.Now()
	prefix := r.URL.Query().Get("prefix")

	if s.orch == nil {
		writeError(w, http.StatusInternalServerError, "orchestrator unavailable")
		return
	}
	cat := s.orch.IndexCatalog()
	if cat == nil {
		slog.Warn("index fields unavailable: index not wired", "duration", time.Since(begin).String())
		writeError(w, http.StatusServiceUnavailable, "索引机未装配（检索走 SQL 兜底，字段目录不可用）")
		return
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

	slog.Info("index fields ok", "prefix", prefix, "fields", len(fields), "duration", time.Since(begin).String())
	writeJSON(w, http.StatusOK, map[string]any{
		"fields": fields,
		"guide":  search.TheGuide(), // 通用规则 + 自然语言→查询速查（契约的跨字段半边）
		"stats": map[string]any{
			"fields": len(fields),
			"keys":   keys,
			"mounts": mounts,
		},
		"hint": "GET /api/files/search?cond=[{field,op,value}]（URL 编码）；kind=enum/multi 可 eq/in，kind=num 可 eq/in/gt/lt/range（value=[lo,hi]）；字段含义看 desc、取值分档看 bench（自然语言→查询值的翻译）",
	})
}
