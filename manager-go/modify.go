// 文件：manager-go/modify.go —— 既有文件的追加、覆盖与原子替换
// 修改：2026-10-08（日期由 fresh-header.ps1 刷新）

package manager

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Modify 修改既有文件，mode 为 append 或 overwrite。
func (m *Manager) Modify(ctx context.Context, logicPath, content, mode string) error {
	m.mutation.Lock()
	defer m.mutation.Unlock()
	ref, err := m.locateByLogic(ctx, logicPath)
	if err != nil {
		return err
	}
	return m.modifyRef(ctx, ref, content, mode)
}

// ModifyByUUID 直接以 UUID 定位并修改文件。
func (m *Manager) ModifyByUUID(ctx context.Context, uuid, content, mode string) error {
	m.mutation.Lock()
	defer m.mutation.Unlock()
	ref, err := m.Locate(ctx, uuid)
	if err != nil {
		return err
	}
	if ref.IsDeleted {
		return ErrDeleted
	}
	return m.modifyRef(ctx, ref, content, mode)
}

// modifyRef 要求调用方持有 mutation 锁。
func (m *Manager) modifyRef(ctx context.Context, ref *FileRef, content, mode string) error {
	logicPath := ref.Path
	if err := validLogicPath(logicPath); err != nil {
		return err
	}
	if len(content) > maxIntakeBytes {
		return fmt.Errorf("security error: file content exceeds 5MB limit")
	}
	if mode != "append" && mode != "overwrite" {
		return fmt.Errorf("error: invalid mode '%s', must be 'append' or 'overwrite'", mode)
	}
	if ref.IsDeleted {
		return ErrDeleted
	}
	if err := m.requirePublished(ctx, ref.UUID); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	abs, err := m.storageAbs(ref.UUID, logicPath)
	if err != nil {
		return err
	}
	in, err := os.Open(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrGhost
		}
		return fmt.Errorf("open original: %w", err)
	}
	info, err := in.Stat()
	if err != nil {
		in.Close()
		return err
	}
	if !info.Mode().IsRegular() {
		in.Close()
		return fmt.Errorf("modify requires regular file")
	}
	out, err := os.CreateTemp(filepath.Dir(abs), ".modify-*")
	if err != nil {
		in.Close()
		return err
	}
	// 未发布暂存文件保留供对账，正式文件不截断。
	if mode == "append" {
		_, err = io.Copy(out, in)
	}
	closeErr := in.Close()
	if err == nil {
		_, err = io.WriteString(out, content)
	}
	if err == nil {
		err = out.Chmod(info.Mode().Perm())
	}
	if err == nil {
		err = out.Sync()
	}
	err = errors.Join(err, closeErr, out.Close())
	if err != nil {
		return fmt.Errorf("prepare modification: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := replaceFile(out.Name(), abs); err != nil {
		return fmt.Errorf("publish modification: %w", err)
	}
	m.buf.drop(ref.UUID)
	return nil
}
