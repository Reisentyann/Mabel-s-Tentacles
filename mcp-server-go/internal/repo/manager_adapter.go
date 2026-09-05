// 文件：mcp-server-go/internal/repo/manager_adapter.go —— manager.Store 适配器：repo 存取 → manager 最小面（DTO 转换 + 顶层列推导归装配侧）
// 修改：2026-09-05（日期由 fresh-header.ps1 刷新）

package repo

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/Reisentyann/Mabel-s-Tentacles/manager-go"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/service"
)

// ManagerStore 把 repo.Store 适配成 manager.Store——依赖倒置的装配侧：
// manager 定义最小面与 DTO（不 import repo），repo 满足签名后由装配层注入
// （架构设计.md 第 4 节 / 交接文档 4.3 DTO 归属）。
// 顶层列推导（file_type / mime_type / extension / scope）在这里做，
// manager 不感知推导规则。
type ManagerStore struct {
	st Store
}

// NewManagerStore 构造适配器。
func NewManagerStore(st Store) *ManagerStore {
	return &ManagerStore{st: st}
}

func (a *ManagerStore) ListMetaPage(ctx context.Context, sincePath string, limit int) ([]manager.MetaRow, error) {
	items, err := a.st.ListMetadataPage(ctx, sincePath, limit)
	if err != nil {
		return nil, err
	}
	out := make([]manager.MetaRow, 0, len(items))
	for i := range items {
		out = append(out, toMetaRow(&items[i]))
	}
	return out, nil
}

func (a *ManagerStore) GetMeta(ctx context.Context, path string) (*manager.MetaRow, error) {
	m, err := a.st.GetMetadata(ctx, path)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil // manager 约定：无行 = (nil, nil)
		}
		return nil, err
	}
	row := toMetaRow(m)
	return &row, nil
}

func (a *ManagerStore) UpsertMeta(ctx context.Context, rec manager.MetaRecord) (string, error) {
	ft, mt := service.InferFileMeta(rec.Path)
	ext := service.InferExtension(rec.Path)
	size, cs := rec.SizeBytes, rec.Checksum
	uuid, err := a.st.UpsertMetadata(ctx, &FileMetadata{
		FilePath:   rec.Path,
		Scope:      service.InferScope(rec.Path),
		FileType:   &ft,
		MimeType:   &mt,
		Extension:  &ext,
		SizeBytes:  &size,
		Checksum:   &cs,
		Attributes: rec.Attributes,
	})
	if err != nil {
		return "", fmt.Errorf("upsert metadata: %w", err)
	}
	return uuid, nil
}

func (a *ManagerStore) MarkMissing(ctx context.Context, path string) (int, error) {
	return a.st.MarkMissingRound(ctx, path)
}

func (a *ManagerStore) SoftDeleteMeta(ctx context.Context, path string) error {
	return a.st.SoftDeleteMetadata(ctx, path)
}

func toMetaRow(m *FileMetadata) manager.MetaRow {
	return manager.MetaRow{
		Path:       m.FilePath,
		Checksum:   derefStr(m.Checksum),
		Attributes: m.Attributes,
	}
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
