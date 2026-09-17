// 文件：mcp-server-go/core/delete.go —— 同步软删除入口：更新 DB is_deleted=true + 从索引机移除
// 修改：2026-09-17（日期由 fresh-header.ps1 刷新）

package core

import (
	"context"
	"encoding/json"
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

	if err := o.opts.Store.SoftDeleteMetadata(ctx, path); err != nil {
		slog.Error("soft delete failed", "path", path, "session", sessionID, "error", err)
		return err
	}

	if o.opts.Sink != nil && len(meta.Attributes) > 0 {
		var oldAttrs map[string]any
		if err := json.Unmarshal(meta.Attributes, &oldAttrs); err == nil && len(oldAttrs) > 0 {
			o.opts.Sink.Update(meta.UUID, oldAttrs, nil)
		}
	}

	slog.Info("file soft deleted", "path", path, "uuid", meta.UUID, "session", sessionID, "duration", time.Since(start).String())
	return nil
}
