// 文件：manager-go/cleanup_test.go —— 清理归档的身份、发布状态、冲突与重复执行验收
// 修改：2026-10-08（日期由 fresh-header.ps1 刷新）

package manager_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Reisentyann/Mabel-s-Tentacles/manager-go"
)

func cleanupFixture(t *testing.T) (*manager.Manager, *fakeStore, string, *manager.WriteReceipt, manager.CleanupOptions) {
	t.Helper()
	m, st, _, root := newTestManager(t)
	r, err := m.Write(context.Background(), "saved.txt", "payload")
	if err != nil {
		t.Fatal(err)
	}
	st.refs[r.UUID] = &manager.FileRef{UUID: r.UUID, Path: r.LogicPath}
	abs := filepath.Join(root, filepath.FromSlash(r.StorageRel))
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(abs, old, old); err != nil {
		t.Fatal(err)
	}
	return m, st, root, r, manager.CleanupOptions{ArchiveRoot: t.TempDir()}
}

func TestCleanupArchivesPartialByDayAndCanRepeat(t *testing.T) {
	m, _, root, receipt, options := cleanupFixture(t)
	ctx := context.Background()
	p, err := m.PlanCleanup(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Candidates) != 1 {
		t.Fatalf("plan: %+v", p)
	}
	r, err := m.ApplyCleanup(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	wantBatch := filepath.Join(p.Options.ArchiveRoot, "管理机清理", p.CreatedAt.Format("2006-01-02"), p.ID)
	if r.BatchDir != wantBatch || len(r.Results) != 1 || r.Results[0].Status != "archived" {
		t.Fatalf("report: %+v", r)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(receipt.StorageRel)+".partial")); !os.IsNotExist(err) {
		t.Fatalf("partial remains: %v", err)
	}
	b, err := os.ReadFile(r.Results[0].Destination)
	if err != nil || string(b) != "payload" {
		t.Fatalf("archive: %q %v", b, err)
	}
	read, err := m.ReadByLogic(ctx, receipt.LogicPath, 0)
	if err != nil || string(read.Content) != "payload" {
		t.Fatalf("formal read: %+v %v", read, err)
	}
	repeat, err := m.ApplyCleanup(ctx, p)
	if err != nil || repeat.Results[0].Status != "skipped" {
		t.Fatalf("repeat: %+v %v", repeat, err)
	}
}

func TestCleanupRechecksPendingAndDestinationConflict(t *testing.T) {
	for _, pending := range []bool{true, false} {
		t.Run(map[bool]string{true: "pending", false: "conflict"}[pending], func(t *testing.T) {
			m, st, root, receipt, options := cleanupFixture(t)
			p, err := m.PlanCleanup(context.Background(), options)
			if err != nil {
				t.Fatal(err)
			}
			if pending {
				st.pending[receipt.LogicPath] = manager.IntakeOperation{LogicPath: receipt.LogicPath, UUID: receipt.UUID}
			} else {
				dst := filepath.Join(options.ArchiveRoot, "管理机清理", p.CreatedAt.Format("2006-01-02"), p.ID, "data", filepath.FromSlash(p.Candidates[0].Path)+".archive")
				if err := os.MkdirAll(dst, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dst, "content"), []byte("existing"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			r, err := m.ApplyCleanup(context.Background(), p)
			if err != nil || r.Results[0].Status != "skipped" {
				t.Fatalf("report: %+v %v", r, err)
			}
			if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(receipt.StorageRel)+".partial")); err != nil {
				t.Fatal("source changed", err)
			}
		})
	}
}

func TestCleanupReportsUnownedAndDifferentPartial(t *testing.T) {
	m, _, root, _, options := cleanupFixture(t)
	for _, name := range []string{"orphan.txt", ".modify-evidence", "unknown.partial"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("evidence"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	p, err := m.PlanCleanup(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Candidates) != 1 || len(p.Reported) != 3 {
		t.Fatalf("plan: %+v", p)
	}
	if _, err := m.PlanCleanup(context.Background(), manager.CleanupOptions{ArchiveRoot: root}); err == nil {
		t.Fatal("archive root overlaps data")
	}
	if _, err := m.PlanCleanup(context.Background(), manager.CleanupOptions{ArchiveRoot: options.ArchiveRoot, MinAge: -time.Second}); err == nil {
		t.Fatal("negative age accepted")
	}
}

type blockingCleanupIntakeStore struct {
	*fakeStore
	entered chan struct{}
	release chan struct{}
}

func (s *blockingCleanupIntakeStore) ReserveMeta(ctx context.Context, key string) (string, error) {
	close(s.entered)
	<-s.release
	return s.fakeStore.ReserveMeta(ctx, key)
}

func TestCleanupWaitsForActiveIntake(t *testing.T) {
	st := &blockingCleanupIntakeStore{fakeStore: newFakeStore(), entered: make(chan struct{}), release: make(chan struct{})}
	m := manager.New(st, t.TempDir(), nil, nil, manager.DownloadConfig{})
	p, err := m.PlanCleanup(context.Background(), manager.CleanupOptions{ArchiveRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	writeDone := make(chan error, 1)
	go func() { _, err := m.Write(context.Background(), "active.txt", "payload"); writeDone <- err }()
	<-st.entered
	cleanupDone := make(chan error, 1)
	go func() { _, err := m.ApplyCleanup(context.Background(), p); cleanupDone <- err }()
	select {
	case err := <-cleanupDone:
		close(st.release)
		<-writeDone
		t.Fatalf("cleanup did not wait for intake: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(st.release)
	if err := <-writeDone; err != nil {
		t.Fatal(err)
	}
	if err := <-cleanupDone; err != nil {
		t.Fatal(err)
	}
}
