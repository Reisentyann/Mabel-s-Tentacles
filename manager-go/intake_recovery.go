// 文件：manager-go/intake_recovery.go —— 入库中断恢复与失败占位归档
// 修改：2026-10-08（日期由 fresh-header.ps1 刷新）

package manager

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"
)

// IntakeOperation 表示已取得 UUID 但尚未确认发布的入库记录。
type IntakeOperation struct {
	LogicPath string
	UUID      string
}

// RecoverPendingIntakes 确认已完整落盘的普通文件，归档其他未完成占位。
// 必须在启动接收请求和扫描之前执行。
func (m *Manager) RecoverPendingIntakes(ctx context.Context) (completed, archived int, err error) {
	items, err := m.store.ListPendingIntakes(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("list pending intakes: %w", err)
	}
	for _, item := range items {
		if err := ctx.Err(); err != nil {
			return completed, archived, err
		}
		abs, err := m.storageAbs(item.UUID, item.LogicPath)
		if err != nil {
			return completed, archived, err
		}
		info, statErr := os.Lstat(abs)
		if statErr == nil && info.Mode().IsRegular() {
			if err := m.store.CompleteIntake(ctx, item.LogicPath, item.UUID); err != nil {
				return completed, archived, err
			}
			completed++
			continue
		}
		if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
			return completed, archived, statErr
		}
		if err := m.store.ArchiveFailedIntake(ctx, item.LogicPath, item.UUID); err != nil {
			return completed, archived, err
		}
		archived++
	}
	return completed, archived, nil
}

// recoverFailedIntake 在请求取消后仍尝试归档，保留 UUID、扩展名及失败内容。
func (m *Manager) recoverFailedIntake(ctx context.Context, logicPath, uuid string, resultErr *error) {
	if *resultErr == nil {
		return
	}
	recoveryCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := m.store.ArchiveFailedIntake(recoveryCtx, logicPath, uuid); err != nil {
		*resultErr = errors.Join(*resultErr, fmt.Errorf("archive failed intake (path remains reserved): %w", err))
	}
}
