// 文件：manager-go/audit_test.go —— 对账域测试：盘库差异、软删保护、分页、取消与只读语义
// 修改：2026-10-08（日期由 fresh-header.ps1 刷新）

package manager_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	manager "github.com/Reisentyann/Mabel-s-Tentacles/manager-go"
)

func TestAuditDifferences(t *testing.T) {
	root := t.TempDir()
	st := newFakeStore()
	m := manager.New(st, root, nil, nil, manager.DownloadConfig{})
	write := func(rel string) {
		t.Helper()
		abs := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte("same"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 205; i++ {
		p := fmt.Sprintf("notes/%03d.txt", i)
		u := fmt.Sprintf("ab-%03d", i)
		st.rows[p] = &manager.MetaRow{Path: p, UUID: u}
		rel, _ := manager.StoragePathOf(u, p)
		write(rel)
	}
	st.rows["notes/000.txt"].Checksum = "same-hash"
	st.rows["notes/001.txt"].Checksum = "same-hash"
	st.rows["trash.txt"] = &manager.MetaRow{Path: "trash.txt", UUID: "cd-trash", IsDeleted: true, Checksum: "same-hash"}
	write("cd/cd-trash.txt")
	st.rows["missing.txt"] = &manager.MetaRow{Path: "missing.txt", UUID: "ef-missing", MissingRounds: 2}
	write("unknown.txt")
	write("wrong/ab-000.txt") // UUID 相同但位置错误，仍是孤儿。
	write("ab/ab-000.md")     // UUID 相同但扩展名错误，仍是孤儿。
	report, err := m.Audit(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(report.Orphans, []string{"ab/ab-000.md", "unknown.txt", "wrong/ab-000.txt"}) {
		t.Fatalf("orphans: %+v", report.Orphans)
	}
	if !reflect.DeepEqual(report.Ghosts, []manager.AuditGhost{{Path: "missing.txt", UUID: "ef-missing", MissingRounds: 2}}) {
		t.Fatalf("ghosts: %+v", report.Ghosts)
	}
	if !reflect.DeepEqual(report.DupChecksums, []manager.DupGroup{{Checksum: "same-hash", Paths: []string{"notes/000.txt", "notes/001.txt"}}}) {
		t.Fatalf("duplicates: %+v", report.DupChecksums)
	}
	if len(st.ups) != 0 || len(st.missing) != 0 || len(st.softDeleted) != 0 {
		t.Fatal("audit modified metadata")
	}
	again, err := m.Audit(context.Background())
	if err != nil || !reflect.DeepEqual(report, again) {
		t.Fatalf("repeat audit: %+v, %v", again, err)
	}
	if _, err := os.Stat(filepath.Join(root, "unknown.txt")); err != nil {
		t.Fatal("audit changed disk", err)
	}
}

func TestAuditCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	m := manager.New(newFakeStore(), t.TempDir(), nil, nil, manager.DownloadConfig{})
	if _, err := m.Audit(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
}

func TestAuditMissingRootIsError(t *testing.T) {
	m := manager.New(newFakeStore(), filepath.Join(t.TempDir(), "missing"), nil, nil, manager.DownloadConfig{})
	if _, err := m.Audit(context.Background()); err == nil {
		t.Fatal("missing root reported as clean")
	}
}
