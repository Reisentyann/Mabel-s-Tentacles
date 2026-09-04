// 文件：mcp-server-go/internal/search/search.go —— 检索抽象：Query 结构 + Searcher 接口（与 SQL 实现解耦）
// 修改：2026-09-03（日期由 fresh-header.ps1 刷新）

package search

import (
	"context"

	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/repo"
)

// Query 检索条件，与具体检索后端无关。
type Query struct {
	Text           string         // 关键词（匹配描述/文件路径）
	Tags           []string       // 标签（AND 包含）
	FileType       string         // 文件类型
	Creator        string         // 创建者
	Scope          string         // 分区：global / user / game（游戏室预留分区，空 = 不过滤）
	Attributes     map[string]any // 属性过滤（如 color）
	IncludeDeleted bool           // 是否含已删除
	Page           int
	Size           int
}

// Searcher 检索接口。当前实现是 SQLSearcher（PostgreSQL 关键词/标签/属性检索），
// 后续接入 RAG / 向量检索等，只需新增一个实现并在 main 里替换，上层无需改动。
type Searcher interface {
	Search(ctx context.Context, q Query) ([]repo.FileMetadata, int, error)
}
