// 文件：mcp-server-go/internal/repo/shortlinks.go —— short_links 表：下载短码存取
// 修改：2026-09-17（日期由 fresh-header.ps1 刷新）

package repo

import (
	"context"
	"time"

	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/service"
)

func (s *pgxStore) CreateShortLink(ctx context.Context, code, filePath, uuid string, expiresAt time.Time) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO short_links (code, file_path, uuid, expires_at)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (code) DO UPDATE SET file_path = EXCLUDED.file_path, uuid = EXCLUDED.uuid, expires_at = EXCLUDED.expires_at`,
		code, filePath, uuid, expiresAt,
	)
	return err
}

func (s *pgxStore) GetShortLink(ctx context.Context, code string) (*service.ShortLink, error) {
	var sl service.ShortLink
	err := s.pool.QueryRow(ctx,
		`SELECT code, file_path, uuid, expires_at, created_at
		 FROM short_links WHERE code = $1`,
		code,
	).Scan(&sl.Code, &sl.FilePath, &sl.UUID, &sl.ExpiresAt, &sl.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &sl, nil
}
