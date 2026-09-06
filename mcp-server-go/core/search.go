// 文件：mcp-server-go/core/search.go —— 检索门面：索引优先 → SQL 降级（实现 search.Searcher）+ 启动全量重建 RebuildIndex
// 修改：2026-09-05（日期由 fresh-header.ps1 刷新）

package core

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/Reisentyann/Mabel-s-Tentacles/describer-go"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/repo"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/search"
)

// 编排机实现 search.Searcher：装配层把它注入 api.Server 即完成检索接线，
// 上层（api/metadata.go）零改动。降级链（设计定稿，检索批次接线）：
//
//  1. q.Attributes 非空 → IndexSource.Query 求命中 uuid 集合
//  2. repo 按 uuid 批量取件（GetMetadataByUUIDs 待补）+ 其余条件过滤分页
//  3. Index 未装配 / Query 出错 → WARN + 落 Fallback（SQL 兜底）
//  4. Fallback 未装配 → 报错
//
// 关键口径：索引返回空集是合法答案（无命中），不是降级信号；
// 只有错误/未装配才降级。骨架阶段索引分支未接线，直通 Fallback。
//
// TODO(检索批次)：
//   - q.Attributes → []indexer.Condition 映射（等值 eq / 数组 in）
//   - 文本关键词/标签仍走 SQL（索引机无子串能力，SQL LIKE 兜底口径不变）
//   - IncludeDeleted=true 时整查询降级 SQL（Rebuild 口径不含软删行）
//   - repo 补 GetMetadataByUUIDs（uuid 是组件间货币，取件归 repo）
func (o *Orchestrator) Search(ctx context.Context, q search.Query) ([]repo.FileMetadata, int, error) {
	if o.opts.Fallback == nil {
		return nil, 0, fmt.Errorf("core: 检索未装配（无 SQL 兜底）")
	}
	return o.opts.Fallback.Search(ctx, q)
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
