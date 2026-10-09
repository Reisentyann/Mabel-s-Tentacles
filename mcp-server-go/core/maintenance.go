// 文件：mcp-server-go/core/maintenance.go —— 文件修改与移动门面：先记录意图、维护文件、统一分析并恢复重试
// 修改：2026-10-09（日期由 fresh-header.ps1 刷新）

package core

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/Reisentyann/Mabel-s-Tentacles/describer-go"
	"github.com/Reisentyann/Mabel-s-Tentacles/manager-go"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/repo"
)

type managerIndexSink struct{ orch *Orchestrator }

func (s managerIndexSink) Update(uuid string, old, next map[string]any) {
	s.orch.updateMu.Lock()
	defer s.orch.updateMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	rows, err := s.orch.opts.Store.GetMetadataByUUIDs(ctx, []string{uuid})
	if err != nil {
		slog.Error("manager index notification failed", "uuid", uuid, "error", err)
		return
	}
	meta := rows[uuid]
	if meta == nil {
		return
	}
	next = describer.AttrsFromJSON(meta.Attributes)
	if meta.IsDeleted {
		next = nil
	}
	if s.orch.opts.Sink != nil {
		s.orch.opts.Sink.Update(uuid, old, next)
	}
}

// ManagerIndexSink adapts manager T2/T3 updates through the orchestrator's
// single serialization point; manager-go remains unaware of the indexer.
func (o *Orchestrator) ManagerIndexSink() manager.IndexSink { return managerIndexSink{orch: o} }

// runMaintenance 串行保护维护、分析、人工标注及恢复。单实例限定。
func (o *Orchestrator) runMaintenance(ctx context.Context, uuid string, ev Event, change func() error) error {
	o.updateMu.Lock()
	defer o.updateMu.Unlock()
	st, ok := o.opts.Store.(repo.MaintenanceStore)
	if !ok {
		return fmt.Errorf("core: durable maintenance not wired")
	}
	start := time.Now()
	id, err := st.PrepareMaintenance(ctx, uuid, string(ev.Kind), ev.SessionID, ev.Actor.Name)
	if err != nil {
		return fmt.Errorf("prepare maintenance: %w", err)
	}
	if err = change(); err != nil {
		// 发布可能已完成而返回错误。保留任务并重读事实，绝不重放文件修改。
		// Never discard intent based on an uncertain read. Retrying only re-reads
		// current DB state; it never repeats append/move/delete side effects.
		stateCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		stateErr := st.FinishMaintenance(stateCtx, id, "ready", err.Error())
		cancel()
		slog.Error("file maintenance failed", "uuid", uuid, "path", ev.Path, "kind", ev.Kind, "session", ev.SessionID, "user", ev.Actor.Name, "job", id, "error", err, "state_error", stateErr, "duration", time.Since(start).String())
		return err
	}
	// prepared 也可恢复，ready 写失败不会丢失已完成文件操作。
	if err := st.FinishMaintenance(ctx, id, "ready", ""); err != nil {
		return fmt.Errorf("file maintenance completed; job %d awaits recovery: %w", id, err)
	}
	return o.processMaintenance(ctx, st, repo.MaintenanceJob{ID: id, UUID: uuid, Kind: string(ev.Kind), Session: ev.SessionID, User: ev.Actor.Name})
}

func (o *Orchestrator) processMaintenance(ctx context.Context, st repo.MaintenanceStore, job repo.MaintenanceJob) error {
	rows, err := o.opts.Store.GetMetadataByUUIDs(ctx, []string{job.UUID})
	if err == nil {
		meta := rows[job.UUID]
		if meta == nil {
			err = fmt.Errorf("maintenance uuid not found")
		} else if meta.IsDeleted {
			if o.opts.Sink != nil {
				o.opts.Sink.Update(job.UUID, nil, nil)
			}
		} else {
			// 按 UUID 读到当前键后执行，绝不重放追加/移动。
			if pending, ok := o.opts.Store.(interface {
				IsIntakePending(context.Context, string) (bool, error)
			}); ok {
				var yes bool
				yes, err = pending.IsIntakePending(ctx, job.UUID)
				if err == nil && yes {
					err = manager.ErrPending
				}
			}
			if err == nil {
				_, err = o.executeUnlocked(ctx, Event{Kind: KindAnalyze, Path: meta.FilePath, SessionID: job.Session, Actor: Actor{Name: job.User}})
			}
		}
	}
	if err != nil {
		stateCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		stateErr := st.FinishMaintenance(stateCtx, job.ID, "retry", err.Error())
		cancel()
		slog.Error("maintenance refresh failed", "uuid", job.UUID, "job", job.ID, "kind", job.Kind, "session", job.Session, "user", job.User, "error", err, "state_error", stateErr)
		return fmt.Errorf("file operation completed; maintenance job %d awaits retry: %w", job.ID, err)
	}
	if err := st.FinishMaintenance(ctx, job.ID, "done", ""); err != nil {
		stateCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		stateErr := st.FinishMaintenance(stateCtx, job.ID, "retry", err.Error())
		slog.Error("maintenance completion failed", "job", job.ID, "uuid", job.UUID, "error", err, "state_error", stateErr)
		return fmt.Errorf("metadata refreshed; maintenance job %d completion pending: %w", job.ID, err)
	}
	slog.Info("maintenance refreshed", "uuid", job.UUID, "job", job.ID, "kind", job.Kind, "session", job.Session, "user", job.User)
	return nil
}

// RetryMaintenance 一轮最多处理 limit 项；三次失败保留记录待人工处理。
func (o *Orchestrator) RetryMaintenance(ctx context.Context, limit int) error {
	o.updateMu.Lock()
	defer o.updateMu.Unlock()
	st, ok := o.opts.Store.(repo.MaintenanceStore)
	if !ok {
		return nil
	}
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	jobs, err := st.PendingMaintenance(ctx, limit)
	if err != nil {
		return err
	}
	for _, job := range jobs {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := o.processMaintenance(ctx, st, job); err != nil {
			slog.Warn("maintenance retry deferred", "job", job.ID, "uuid", job.UUID, "error", err)
		}
	}
	return nil
}

// RecoverMaintenanceClaims releases claims left by a previous process.
func (o *Orchestrator) RecoverMaintenanceClaims(ctx context.Context) error {
	st, ok := o.opts.Store.(repo.MaintenanceStore)
	if !ok {
		return fmt.Errorf("core: durable maintenance not wired")
	}
	return st.RecoverMaintenanceClaims(ctx)
}

// NotifyMaintenance 在现有文件发布后持久化刷新任务；由调用方传入 UUID。
func (o *Orchestrator) NotifyMaintenance(ctx context.Context, uuid, kind, path, session string, actor Actor) error {
	return o.runMaintenance(ctx, uuid, Event{Kind: Kind(kind), Path: path, SessionID: session, Actor: actor}, func() error { return nil })
}

func (o *Orchestrator) ModifyFile(ctx context.Context, mgr *manager.Manager, path, content, mode, session string, actor Actor) error {
	if mgr == nil {
		return fmt.Errorf("manager not wired")
	}
	meta, err := o.opts.Store.GetMetadata(ctx, path)
	if err != nil {
		return err
	}
	if meta == nil {
		return fmt.Errorf("file not found")
	}
	return o.runMaintenance(ctx, meta.UUID, Event{Kind: KindModify, Path: path, SessionID: session, Actor: actor}, func() error { return mgr.Modify(ctx, path, content, mode) })
}

func (o *Orchestrator) MoveFile(ctx context.Context, mgr *manager.Manager, from, to, uuid, parent, name, session string, actor Actor) (*manager.MoveReceipt, error) {
	if mgr == nil {
		return nil, fmt.Errorf("manager not wired")
	}
	if uuid == "" {
		meta, err := o.opts.Store.GetMetadata(ctx, from)
		if err != nil {
			return nil, err
		}
		if meta == nil {
			return nil, fmt.Errorf("file not found")
		}
		uuid = meta.UUID
	}
	if from != "" {
		if meta, err := o.opts.Store.GetMetadata(ctx, from); err == nil && meta != nil && meta.UUID == uuid {
			from = meta.FilePath
		}
	}
	var receipt *manager.MoveReceipt
	err := o.runMaintenance(ctx, uuid, Event{Kind: KindMove, Path: from, SessionID: session, Actor: actor}, func() (err error) {
		if parent != "" {
			receipt, err = mgr.MoveByUUID(ctx, uuid, parent, name)
		} else {
			receipt, err = mgr.Move(ctx, from, to)
		}
		return err
	})
	return receipt, err
}
