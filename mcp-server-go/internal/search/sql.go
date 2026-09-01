package search

import (
	"context"

	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/repo"
)

// SQLSearcher 基于 PostgreSQL 的关键词/标签/属性检索实现。
type SQLSearcher struct {
	store repo.Store
}

func NewSQLSearcher(store repo.Store) *SQLSearcher {
	return &SQLSearcher{store: store}
}

func (s *SQLSearcher) Search(ctx context.Context, q Query) ([]repo.FileMetadata, int, error) {
	return s.store.SearchFiles(ctx, repo.FileSearch{
		Query:          q.Text,
		Tags:           q.Tags,
		FileType:       q.FileType,
		Creator:        q.Creator,
		Attributes:     q.Attributes,
		IncludeDeleted: q.IncludeDeleted,
		Page:           q.Page,
		Size:           q.Size,
	})
}
