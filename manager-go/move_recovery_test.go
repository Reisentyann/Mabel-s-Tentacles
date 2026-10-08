// 文件：manager-go/move_recovery_test.go —— 移动中断恢复：发布前后重复恢复及目标冲突保护
// 修改：2026-10-08（日期由 fresh-header.ps1 刷新）

package manager_test

import (
	"context"
	manager "github.com/Reisentyann/Mabel-s-Tentacles/manager-go"
	"os"
	"path/filepath"
	"testing"
)

func TestMoveRecoveryInterrupted(t *testing.T) {
	for _, published := range []bool{false, true} {
		t.Run(map[bool]string{false: "before publish", true: "after publish"}[published], func(t *testing.T) {
			m, st, _, dir := newTestManager(t)
			ctx := context.Background()
			r, err := m.Write(ctx, "old.txt", "payload")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := st.MoveMeta(ctx, "old.txt", "new.md"); err != nil {
				t.Fatal(err)
			}
			oldAbs := filepath.Join(dir, filepath.FromSlash(r.StorageRel))
			rel, _ := manager.StoragePathOf(r.UUID, "new.md")
			newAbs := filepath.Join(dir, filepath.FromSlash(rel))
			if published {
				if err := os.Link(oldAbs, newAbs); err != nil {
					t.Fatal(err)
				}
			}
			m = manager.New(st, dir, nil, nil, manager.DownloadConfig{})
			n, err := m.RecoverPendingMoves(ctx)
			if err != nil || n != 1 {
				t.Fatalf("n=%d err=%v", n, err)
			}
			n, err = m.RecoverPendingMoves(ctx)
			if err != nil || n != 0 {
				t.Fatalf("replay n=%d err=%v", n, err)
			}
			f, err := m.ReadByLogic(ctx, "new.md", 0)
			if err != nil || string(f.Content) != "payload" {
				t.Fatalf("read=%v err=%v", f, err)
			}
		})
	}
}

func TestMoveRecoveryDoesNotOverwriteConflict(t *testing.T) {
	m, st, _, dir := newTestManager(t)
	ctx := context.Background()
	r, err := m.Write(ctx, "old.txt", "original")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.MoveMeta(ctx, "old.txt", "new.md"); err != nil {
		t.Fatal(err)
	}
	rel, _ := manager.StoragePathOf(r.UUID, "new.md")
	abs := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.WriteFile(abs, []byte("unrelated"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := m.RecoverPendingMoves(ctx); err == nil {
		t.Fatal("conflict accepted")
	}
	b, err := os.ReadFile(abs)
	if err != nil || string(b) != "unrelated" {
		t.Fatal("conflict overwritten")
	}
	if len(st.moves) != 1 {
		t.Fatal("recovery evidence lost")
	}
}
