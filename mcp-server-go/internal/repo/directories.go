// 文件：mcp-server-go/internal/repo/directories.go —— UUID 目录查询与显示名称关联，兼容旧路径文件
// 修改：2026-10-08（日期由 fresh-header.ps1 刷新）

package repo

import (
	"context"
	"errors"
	manager "github.com/Reisentyann/Mabel-s-Tentacles/manager-go"
	"github.com/jackc/pgx/v5"
)

func (s *pgxStore) DirectoryByUUID(ctx context.Context, id string) (*manager.DirectoryRef, error) {
	r := &manager.DirectoryRef{}
	err := s.pool.QueryRow(ctx, `SELECT uuid,file_path FROM logical_directories WHERE uuid=$1 AND NOT is_deleted`, id).Scan(&r.UUID, &r.Path)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, manager.ErrNotFound
	}
	return r, err
}
func (s *pgxStore) DirectoryByPath(ctx context.Context, p string) (*manager.DirectoryRef, error) {
	r := &manager.DirectoryRef{}
	err := s.pool.QueryRow(ctx, `SELECT uuid,file_path FROM logical_directories WHERE file_path=$1 AND NOT is_deleted`, p).Scan(&r.UUID, &r.Path)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, manager.ErrNotFound
	}
	return r, err
}
func (s *pgxStore) LinkDirectoryFile(ctx context.Context, parent, file, name string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `INSERT INTO directory_files(parent_uuid,file_uuid,name) VALUES($1,$2,$3)`, parent, file, name); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE file_metadata SET title=$2 WHERE uuid=$1`, file, name); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s *pgxStore) DirectoryEntries(ctx context.Context, id string) ([]manager.DirectoryEntry, error) {
	dir, err := s.DirectoryByUUID(ctx, id)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT d.file_uuid,d.name,'file' FROM directory_files d JOIN file_metadata f ON f.uuid=d.file_uuid WHERE d.parent_uuid=$1 AND NOT f.is_deleted
	 UNION ALL SELECT uuid,substr(file_path,length($2)+2),'dir' FROM logical_directories WHERE NOT is_deleted AND starts_with(file_path,$2||'/') AND strpos(substr(file_path,length($2)+2),'/')=0
	 UNION ALL SELECT uuid,substr(file_path,length($2)+2),'file' FROM file_metadata f WHERE NOT is_deleted AND starts_with(file_path,$2||'/') AND strpos(substr(file_path,length($2)+2),'/')=0 AND NOT EXISTS(SELECT 1 FROM directory_files d WHERE d.file_uuid=f.uuid)
	 ORDER BY 3,2,1`, id, dir.Path)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []manager.DirectoryEntry{}
	for rows.Next() {
		var r manager.DirectoryEntry
		if err := rows.Scan(&r.UUID, &r.Name, &r.Kind); err != nil {
			return nil, err
		}
		items = append(items, r)
	}
	return items, rows.Err()
}
