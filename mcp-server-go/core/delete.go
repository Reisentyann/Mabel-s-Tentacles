// 文件：mcp-server-go/core/delete.go —— 同步软删除入口：更新 DB is_deleted=true + 从索引机移除
// 修改：2026-10-09（日期由 fresh-header.ps1 刷新）

package core

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

// Delete 同步软删除入口：DB 标记 is_deleted=true，并从内存索引机移除（Update old, nil）。
func (o *Orchestrator) Delete(ctx context.Context, path, sessionID string) error {
	start := time.Now()
	if o.opts.Store == nil {
		return errors.New("store not available")
	}

	meta, err := o.opts.Store.GetMetadata(ctx, path)
	if err != nil {
		return err
	}
	if meta == nil {
		return errors.New("file not found")
	}
	if meta.IsDeleted {
		return errors.New("file is already deleted")
	}

	if err := o.runMaintenance(ctx, meta.UUID, Event{Kind: Kind("delete"), Path: path, SessionID: sessionID}, func() error {
		return o.opts.Store.SoftDeleteMetadata(ctx, path)
	}); err != nil {
		slog.Error("soft delete maintenance failed", "path", path, "uuid", meta.UUID, "session", sessionID, "error", err)
		return err
	}

	slog.Info("file soft deleted", "path", path, "uuid", meta.UUID, "session", sessionID, "duration", time.Since(start).String())
	return nil
}
