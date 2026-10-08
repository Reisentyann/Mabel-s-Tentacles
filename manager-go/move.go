// 文件：manager-go/move.go —— 逻辑路径与 UUID 移动、物理发布确认
// 修改：2026-10-08（日期由 fresh-header.ps1 刷新）

package manager

import (
	"context"
	"fmt"
	"os"
	"path"
)

// UUIDMoveStore 以 UUID 原子更新逻辑键、目录关联及移动恢复记录。
type UUIDMoveStore interface {
	MoveUUID(ctx context.Context, uuid, target, parentUUID, name string) (string, error)
}

// MoveReceipt 包含移动前后的逻辑键和物理路径是否变化。
type MoveReceipt struct {
	UUID        string `json:"uuid"`
	From        string `json:"from"`
	To          string `json:"to"`
	StorageMove bool   `json:"storage_move"`
}

// Move 改变文件逻辑键并记录谱系。目标已占用返回 ErrKeyExists。
// 扩展名改变时排他发布新的物理路径，失败由 RecoverPendingMoves 恢复。
func (m *Manager) Move(ctx context.Context, from, to string) (*MoveReceipt, error) {
	m.mutation.Lock()
	defer m.mutation.Unlock()
	if err := validLogicPath(from); err != nil {
		return nil, err
	}
	if err := validLogicPath(to); err != nil {
		return nil, err
	}
	if from == to {
		return nil, fmt.Errorf("error: source and target are the same")
	}
	row, err := m.store.GetMeta(ctx, from)
	if err != nil {
		return nil, fmt.Errorf("get meta: %w", err)
	}
	if row == nil {
		return nil, ErrNotFound
	}
	if row.IsDeleted {
		return nil, ErrDeleted
	}
	if err := m.requirePublished(ctx, row.UUID); err != nil {
		return nil, err
	}
	if err := m.ensureFilePathAvailable(ctx, to); err != nil {
		return nil, err
	}
	if err := m.prepareStorageMove(row.UUID, from, to); err != nil {
		return nil, err
	}
	uuid, err := m.store.MoveMeta(ctx, from, to)
	if err != nil {
		return nil, err
	}
	return m.completeStorageMove(ctx, MoveOperation{UUID: uuid, From: from, To: to})
}

// MoveByUUID 以对象身份移动文件；目标由目录 UUID 与新名称确定。
func (m *Manager) MoveByUUID(ctx context.Context, uuid, parentUUID, name string) (*MoveReceipt, error) {
	m.mutation.Lock()
	defer m.mutation.Unlock()
	if err := validEntryName(name); err != nil {
		return nil, err
	}
	ds, err := m.directoryStore()
	if err != nil {
		return nil, err
	}
	parent, err := ds.DirectoryByUUID(ctx, parentUUID)
	if err != nil {
		return nil, err
	}
	ref, err := m.Locate(ctx, uuid)
	if err != nil {
		return nil, err
	}
	if ref.IsDeleted {
		return nil, ErrDeleted
	}
	if err := m.requirePublished(ctx, uuid); err != nil {
		return nil, err
	}
	target, err := newDirectoryFilePath(parent, name)
	if err != nil {
		return nil, err
	}
	if err := m.ensureFilePathAvailable(ctx, target); err != nil {
		return nil, err
	}
	moveStore, ok := m.store.(UUIDMoveStore)
	if !ok {
		return nil, fmt.Errorf("manager: UUID move unavailable")
	}
	if err := m.prepareStorageMove(uuid, ref.Path, target); err != nil {
		return nil, err
	}
	if _, err := moveStore.MoveUUID(ctx, uuid, target, parentUUID, name); err != nil {
		return nil, err
	}
	return m.completeStorageMove(ctx, MoveOperation{UUID: uuid, From: ref.Path, To: target, ParentUUID: parentUUID, Name: name})
}

// prepareStorageMove 在提交元数据前检查物理路径和恢复能力。
func (m *Manager) prepareStorageMove(uuid, from, to string) error {
	if _, err := StoragePathOf(uuid, from); err != nil {
		return err
	}
	if _, err := StoragePathOf(uuid, to); err != nil {
		return err
	}
	if path.Ext(from) == path.Ext(to) {
		return nil
	}
	if _, ok := m.store.(MoveRecoveryStore); !ok {
		return fmt.Errorf("manager: recoverable extension move unavailable")
	}
	abs, err := m.storageAbs(uuid, from)
	if err != nil {
		return err
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("move source is not regular")
	}
	return nil
}

// completeStorageMove 在元数据提交后发布内容并使缓存失效。
// 调用方必须持有 mutation 锁，且已通过 prepareStorageMove。
func (m *Manager) completeStorageMove(ctx context.Context, op MoveOperation) (*MoveReceipt, error) {
	moved := path.Ext(op.From) != path.Ext(op.To)
	if moved {
		if err := m.finishStorageMove(op); err != nil {
			return nil, fmt.Errorf("move pending recovery: %w", err)
		}
		if err := m.store.(MoveRecoveryStore).CompleteMove(ctx, op.UUID, op.To); err != nil {
			return nil, fmt.Errorf("confirm move: %w", err)
		}
	}
	m.buf.drop(op.UUID)
	return &MoveReceipt{UUID: op.UUID, From: op.From, To: op.To, StorageMove: moved}, nil
}
