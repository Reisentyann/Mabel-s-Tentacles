// 文件：mcp-server-go/core/search.go —— 检索门面：索引优先（属性过滤 → Index.Query → uuid 批量取件）→ SQL 降级 + 启动全量重建 RebuildIndex
// 修改：2026-09-08（日期由 fresh-header.ps1 刷新）

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
