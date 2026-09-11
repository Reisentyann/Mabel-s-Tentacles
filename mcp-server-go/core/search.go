// 文件：mcp-server-go/core/search.go —— 检索门面：索引优先（属性过滤 → Index.Query → uuid 批量取件）→ SQL 降级 + 启动全量重建 RebuildIndex
// 修改：2026-09-11（日期由 fresh-header.ps1 刷新）

package core

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/Reisentyann/Mabel-s-Tentacles/common"
	"github.com/Reisentyann/Mabel-s-Tentacles/describer-go"
	"github.com/Reisentyann/Mabel-s-Tentacles/indexer-go"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/repo"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/search"
)

// 编排机实现 search.Searcher：装配层把它注入 api.Server 即完成检索接线，
// 上层（api/metadata.go）零改动。降级链（检索索引化 2026-09-06 点亮）：
//
//  1. q.Attributes 非空且无 SQL 专长条件（Text/Tags/IncludeDeleted 空）→
//     Index.Query 求命中 uuid 集合（And 交集，标量 eq / 数组 in）
//  2. repo.GetMetadataByUUIDs 一次取齐 → 内存复判（file_type/creator/
//     scope/观察者可见性，与 SQL metaWhere 同口径）→ updated_at DESC 排序分页
//  3. Index 未装配 / Query 出错 / 取件出错 → WARN + 落 Fallback（SQL 兜底）
//  4. Fallback 未装配 → 报错
//
// 关键口径：索引返回空集是合法答案（无命中），不是降级信号；
// 只有错误/未装配才降级。软删行索引不挂载（Rebuild 口径），索引路径
// 结果恒为未软删——IncludeDeleted=true 时索引给不出全集，直接降级 SQL。
func (o *Orchestrator) Search(ctx context.Context, q search.Query) ([]repo.FileMetadata, int, error) {
	if o.opts.Fallback == nil {
		return nil, 0, fmt.Errorf("core: 检索未装配（无 SQL 兜底）")
	}
	if o.opts.Index == nil || !indexable(q) {
		return o.opts.Fallback.Search(ctx, q)
	}

	uuids, err := o.opts.Index.Query(toConditions(q.Attributes), indexer.And)
	if err != nil {
		slog.Warn("index query failed, degrade to SQL", "error", err)
		return o.opts.Fallback.Search(ctx, q)
	}
	return o.collectByUUIDs(ctx, uuids, q)
}

// SearchByConditions 条件数组直查（search_files 工具入口，目录批次
// 2026-09-09）：conds 为显式索引条件（eq/in/gt/lt/range/ne/exists/contains
// 全量，And 交集，检索语言扩展 2026-09-10），绕过 search.Query.Attributes
// 只有 eq/in 的窄口径；q 携带复判条件（FileType/Creator/Scope/观察者）
// 与分页参数，其 Text/Tags/Attributes/IncludeDeleted 不参与（含软删走
// Search 的 SQL 口径）。q.OrderBy 非空时命中集按该属性值排序（asc/desc）——
// 极值/Top-N 由此表达（size=1 即最大/最小），两条路径（索引/降级）同口径。
//
// 降级链与 Search 同构：Index 未装配 / Query 出错 → eq/in 条件折回
// Attributes 走 SQL 兜底；gt/lt/range/ne/exists/contains 是索引专长
// （SQL 的 attributes @> 只有包含语义，装不下这些）→ 索引不可用时直接
// 报错，宁缺毋滥不静默错答。
// 空集 = 合法答案（无命中），不降级。
func (o *Orchestrator) SearchByConditions(ctx context.Context, conds []indexer.Condition, q search.Query) ([]repo.FileMetadata, int, error) {
	if len(conds) == 0 {
		return []repo.FileMetadata{}, 0, nil
	}
	if o.opts.Fallback == nil {
		return nil, 0, fmt.Errorf("core: 检索未装配（无 SQL 兜底）")
	}
	var (
		items []repo.FileMetadata
		total int
		err   error
	)
	if o.opts.Index == nil {
		items, total, err = o.degradeConds(ctx, conds, q)
	} else {
		var uuids []string
		uuids, qerr := o.opts.Index.Query(conds, indexer.And)
		if qerr != nil {
			slog.Warn("index query by conditions failed", "conds", len(conds), "error", qerr)
			items, total, err = o.degradeConds(ctx, conds, q)
		} else {
			items, total, err = o.collectByUUIDs(ctx, uuids, q)
		}
	}
	if err != nil {
		return nil, 0, err
	}
	sortByAttr(items, q)
	return items, total, nil
}

// degradeConds 条件降级：eq/in 折回 Attributes 走 SQL；ne/exists/contains
// 与范围条件一样折不动（@> 装不下）→ 报错（见 SearchByConditions 注释）。
func (o *Orchestrator) degradeConds(ctx context.Context, conds []indexer.Condition, q search.Query) ([]repo.FileMetadata, int, error) {
	attrs := map[string]any{}
	for _, c := range conds {
		switch c.Op {
		case indexer.OpEq:
			attrs[c.Field] = c.Value
		case indexer.OpIn:
			attrs[c.Field] = c.Value
		default:
			return nil, 0, fmt.Errorf("core: 条件（%s）依赖索引机，索引不可用无法降级", c.Op)
		}
	}
	q.Attributes = attrs
	return o.opts.Fallback.Search(ctx, q)
}

// sortByAttr 按属性值排序命中集（极值/Top-N 的支撑，检索语言扩展
// 2026-09-10）：q.OrderBy 空 = 维持调用方次序（updated_at 倒序现状）。
// 值取自行内 attributes JSONB；缺键行恒排末尾（desc 与 asc 皆是——
// 「最大」不可能是缺值的行）；数值比较数值、字符串比字典序，跨型
// 稳定但不建议（排序字段应选数值字段）。
func sortByAttr(items []repo.FileMetadata, q search.Query) {
	if q.OrderBy == "" || len(items) < 2 {
		return
	}
	desc := q.Order != "asc"
	// 行数与行值一次性提取（attributes 解析一次，比较零解析）
	vals := make([]any, len(items))
	for i := range items {
		attrs := describer.AttrsFromJSON(items[i].Attributes)
		vals[i] = attrs[q.OrderBy]
	}
	sort.SliceStable(items, func(a, b int) bool {
		if desc {
			// desc：严格大者在前；等值不动（稳定）；缺键恒最后
			return vals[a] != nil && (vals[b] == nil || attrLess(vals[b], vals[a]))
		}
		// asc：严格小者在前；等值不动；缺键恒最后
		return vals[a] != nil && (vals[b] == nil || attrLess(vals[a], vals[b]))
	})
}

// attrLess 值比较：缺键(nil) 恒小于一切（配合 sortByAttr 的方向处理）；
// 数值族比数值，字符串比字典序，bool false<true；跨型给稳定序
// （数值 < 字符串 < 布尔——无业务含义，只为排序确定性）。
func attrLess(a, b any) bool {
	if a == nil || b == nil {
		return a == nil && b != nil
	}
	fa, aok := numFloat(a)
	fb, bok := numFloat(b)
	if aok && bok {
		return fa < fb
	}
	if aok != bok {
		return aok // 数值 < 非数值
	}
	sa, aStr := a.(string)
	sb, bStr := b.(string)
	if aStr || bStr {
		if aStr != bStr {
			return aStr // 字符串 < 布尔
		}
		return sa < sb
	}
	ba, _ := a.(bool)
	bb, _ := b.(bool)
	return !ba && bb
}

// numFloat 数值族归一（JSON 解析出的 float64 直通；int 族兜底）。
func numFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	}
	return 0, false
}

// collectByUUIDs 索引命中后的取件复判分页（Search / SearchByConditions
// 共享后半段）：uuid 批量取件 → 软删双保险过滤 → 内存复判（与 SQL
// metaWhere 同口径）→ updated_at DESC 排序分页。
func (o *Orchestrator) collectByUUIDs(ctx context.Context, uuids []string, q search.Query) ([]repo.FileMetadata, int, error) {
	if len(uuids) == 0 {
		return []repo.FileMetadata{}, 0, nil // 空集 = 合法答案（无命中），不降级
	}
	metas, err := o.opts.Store.GetMetadataByUUIDs(ctx, uuids)
	if err != nil {
		slog.Warn("fetch by uuids failed, degrade to SQL", "error", err)
		return o.opts.Fallback.Search(ctx, q)
	}

	items := make([]repo.FileMetadata, 0, len(metas))
	for _, m := range metas {
		if m.IsDeleted {
			continue // 防御：Rebuild 口径不含软删，取件侧双保险
		}
		if matchesQuery(q, m) {
			items = append(items, *m)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].UpdatedAt.After(items[j].UpdatedAt) })

	total := len(items)
	page, size := q.Page, q.Size
	if page < 1 {
		page = 1
	}
	if size <= 0 {
		size = 20
	}
	lo := (page - 1) * size
	if lo >= total {
		return []repo.FileMetadata{}, total, nil
	}
	hi := lo + size
	if hi > total {
		hi = total
	}
	return items[lo:hi], total, nil
}

// IndexCatalog 索引字段目录透传（list_index_fields 工具的发现入口）。
// nil = 索引未装配（调用方按"检索降级"口径提示）。
func (o *Orchestrator) IndexCatalog() []indexer.FieldInfo {
	if o.opts.Index == nil {
		return nil
	}
	return o.opts.Index.Catalog()
}

// indexable 索引路径适用判定：属性过滤非空（索引的价值场景）且其余条件
// 均可在取件后内存复判——文本关键词/标签是 SQL LIKE/@> 专长，含软删需要
// 全集口径（索引不挂载软删行），任一存在即降级 SQL。
func indexable(q search.Query) bool {
	return len(q.Attributes) > 0 && q.Text == "" && len(q.Tags) == 0 && !q.IncludeDeleted
}

// toConditions 属性过滤 → 索引条件：标量 → eq；数组 → in（值集合任一
// 命中）。数值范围（gt/lt/range）暂无查询语言入口，将来 API 扩展时映射。
func toConditions(attrs map[string]any) []indexer.Condition {
	conds := make([]indexer.Condition, 0, len(attrs))
	for k, v := range attrs {
		switch val := v.(type) {
		case []any:
			if len(val) == 0 {
				continue
			}
			conds = append(conds, indexer.Condition{Field: k, Op: indexer.OpIn, Value: val})
		case []string:
			if len(val) == 0 {
				continue
			}
			in := make([]any, len(val))
			for i, s := range val {
				in[i] = s
			}
			conds = append(conds, indexer.Condition{Field: k, Op: indexer.OpIn, Value: in})
		default:
			conds = append(conds, indexer.Condition{Field: k, Op: indexer.OpEq, Value: v})
		}
	}
	return conds
}

// matchesQuery 取件后的内存复判（与 SQL metaWhere 同口径）：
// file_type / user_id(creator) / scope / 观察者可见性。
func matchesQuery(q search.Query, m *repo.FileMetadata) bool {
	if q.FileType != "" && common.DerefStr(m.FileType) != q.FileType {
		return false
	}
	if q.Creator != "" && common.DerefStr(m.UserID) != q.Creator {
		return false
	}
	if q.Scope != "" && m.Scope != q.Scope {
		return false
	}
	if q.ViewerName != "" && !q.ViewerAdmin && !visibleTo(q, m) {
		return false
	}
	return true
}

// visibleTo 观察者可见性（authz.CanRead 的检索投影，与 metaWhere $8/$9
// 同口径）：public / 本人 owner / 本组 group。
func visibleTo(q search.Query, m *repo.FileMetadata) bool {
	if m.Visibility == "public" {
		return true
	}
	if common.DerefStr(m.OwnerID) == q.ViewerName {
		return true
	}
	if m.Visibility == "group" && m.GroupID != nil {
		for _, g := range q.ViewerGroups {
			if g == *m.GroupID {
				return true
			}
		}
	}
	return false
}

// 编译期断言：编排机满足检索接口（装配层可直接注入 api.Server）。
var _ search.Searcher = (*Orchestrator)(nil)

// rebuildPageSize 索引重建的分页游标每页行数。
const rebuildPageSize = 200

// RebuildIndex 启动全量重建：分页扫全表（file_path 升序游标，不含软删）
// → uuid→attrs → IndexSource.Rebuild。个人库量级秒级；重建期间检索自动
// 降级 SQL（见 Search）。服务启动时调用一次——DB 是事实源，索引是派生
// 缓存，可丢弃可重建（容灾铁律 2 的落点）。
func (o *Orchestrator) RebuildIndex(ctx context.Context) error {
	if o.opts.Index == nil {
		return fmt.Errorf("core: 索引源未装配")
	}
	start := time.Now()
	all := map[string]map[string]any{}
	since := ""
	for {
		rows, err := o.opts.Store.ListMetadataPage(ctx, since, rebuildPageSize)
		if err != nil {
			return fmt.Errorf("rebuild list page: %w", err)
		}
		if len(rows) == 0 {
			break
		}
		for _, r := range rows {
			since = r.FilePath
			if r.UUID == "" {
				continue
			}
			all[r.UUID] = describer.AttrsFromJSON(r.Attributes)
		}
	}
	if err := o.opts.Index.Rebuild(all); err != nil {
		return fmt.Errorf("rebuild index: %w", err)
	}
	slog.Info("index rebuilt", "files", len(all), "duration", time.Since(start).String())
	return nil
}
