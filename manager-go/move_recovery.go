// 文件：manager-go/move_recovery.go —— 扩展名移动恢复：排他发布目标并确认持久化移动记录
// 修改：2026-10-14（日期由 fresh-header.ps1 刷新）

package manager

import (
	"context"
	"errors"
	"fmt"
	"os"
)

// MoveOperation 是与元数据移动同事务保存的恢复记录。
type MoveOperation struct{ UUID, From, To, ParentUUID, Name string }

// MoveRecoveryStore 提供移动记录的枚举与发布确认。
type MoveRecoveryStore interface {
	ListPendingMoves(context.Context) ([]MoveOperation, error)
	CompleteMove(ctx context.Context, uuid, target string) error
}

// finishStorageMove 保留旧目录项作为恢复证据；目标存在必须是同一文件。
func (m *Manager) finishStorageMove(op MoveOperation) error {
	oldAbs, err := m.storageAbs(op.UUID, op.From)
	if err != nil {
		return err
	}
	newAbs, err := m.storageAbs(op.UUID, op.To)
	if err != nil {
		return err
	}
	oldInfo, err := os.Lstat(oldAbs)
	if err != nil {
		return fmt.Errorf("move source: %w", err)
	}
	if !oldInfo.Mode().IsRegular() {
		return fmt.Errorf("move source is not regular")
	}
	if err := os.Link(oldAbs, newAbs); err != nil {
		newInfo, statErr := os.Lstat(newAbs)
		if statErr != nil || !newInfo.Mode().IsRegular() || !os.SameFile(oldInfo, newInfo) {
			return fmt.Errorf("publish move without overwrite: %w", errors.Join(err, statErr))
		}
	}
	return nil
}

// RecoverPendingMoves 必须在启动接收请求和扫描前执行；确认失败仍保留记录。
func (m *Manager) RecoverPendingMoves(ctx context.Context) (int, error) {
	ops, err := m.RecoverPendingMoveOps(ctx)
	return len(ops), err
}

// RecoverPendingMoveOps 恢复未完成的移动操作并返回已恢复的操作列表（供编排机生成维护刷新任务）。
func (m *Manager) RecoverPendingMoveOps(ctx context.Context) ([]MoveOperation, error) {
	m.mutation.Lock()
	defer m.mutation.Unlock()
	s, ok := m.store.(MoveRecoveryStore)
	if !ok {
		return nil, nil
	}
	ops, err := s.ListPendingMoves(ctx)
	if err != nil {
		return nil, err
	}
	recovered := make([]MoveOperation, 0, len(ops))
	for _, op := range ops {
		if err := ctx.Err(); err != nil {
			return recovered, err
		}
		if err := m.finishStorageMove(op); err != nil {
			return recovered, err
		}
		if err := s.CompleteMove(ctx, op.UUID, op.To); err != nil {
			return recovered, err
		}
		m.buf.drop(op.UUID)
		recovered = append(recovered, op)
	}
	return recovered, nil
}
