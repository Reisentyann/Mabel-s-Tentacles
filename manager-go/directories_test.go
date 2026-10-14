// 文件：manager-go/directories_test.go —— UUID 目录测试：同名不串件、空目录、导出权限与半文件恢复
// 修改：2026-10-08（日期由 fresh-header.ps1 刷新）

package manager_test

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	manager "github.com/Reisentyann/Mabel-s-Tentacles/manager-go"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type directoryTestStore struct {
	*fakeStore
	links map[string][]manager.DirectoryEntry
}

type atomicDirectoryTestStore struct{ *directoryTestStore }

func (s *atomicDirectoryTestStore) AtomicDirectoryIntake() {}
func (s *atomicDirectoryTestStore) ReserveMeta(ctx context.Context, key string) (string, error) {
	uuid, err := s.fakeStore.ReserveMeta(ctx, key)
	if err != nil {
		return "", err
	}
	if in, ok := manager.DirectoryIntakeFrom(ctx); ok {
		s.links[in.ParentUUID] = append(s.links[in.ParentUUID], manager.DirectoryEntry{UUID: uuid, Name: in.Name, Kind: "file"})
	}
	if source, ok := manager.DirectoryCopyFrom(ctx); ok {
		for _, row := range s.rows {
			if row.UUID == source {
				s.rows[key].CopiedFrom = row.Path
				break
			}
		}
	}
	return uuid, nil
}
func (s *atomicDirectoryTestStore) MoveUUID(ctx context.Context, uuid, target, parent, name string) (string, error) {
	var source string
	for p, row := range s.rows {
		if row.UUID == uuid {
			source = p
			break
		}
	}
	if source == "" {
		return "", manager.ErrNotFound
	}
	id, err := s.fakeStore.MoveMeta(ctx, source, target)
	if err != nil {
		return "", err
	}
	for p, items := range s.links {
		keep := items[:0]
		for _, item := range items {
			if item.UUID != uuid {
				keep = append(keep, item)
			}
		}
		s.links[p] = keep
	}
	s.links[parent] = append(s.links[parent], manager.DirectoryEntry{UUID: uuid, Name: name, Kind: "file"})
	return id, nil
}

func TestUUIDDirectoryMoveAndCopySameNames(t *testing.T) {
	st := &atomicDirectoryTestStore{&directoryTestStore{fakeStore: newFakeStore(), links: map[string][]manager.DirectoryEntry{}}}
	st.dirs["~alice"] = true
	st.dirs["~alice/dest"] = true
	m := manager.New(st, t.TempDir(), nil, nil, manager.DownloadConfig{})
	ctx := context.Background()
	a, err := m.WriteInDirectory(ctx, "~alice", "same.txt", "original")
	if err != nil {
		t.Fatal(err)
	}
	b, err := m.WriteInDirectory(ctx, "~alice/dest", "same.txt", "existing")
	if err != nil {
		t.Fatal(err)
	}
	copy, err := m.CopyInDirectory(ctx, a.UUID, "~alice/dest", "same.txt")
	if err != nil {
		t.Fatal(err)
	}
	if copy.UUID == a.UUID || st.rows[copy.LogicPath].CopiedFrom != a.LogicPath {
		t.Fatalf("copy=%+v", copy)
	}
	move, err := m.MoveByUUID(ctx, a.UUID, "~alice/dest", "same.txt")
	if err != nil {
		t.Fatal(err)
	}
	if move.UUID != a.UUID {
		t.Fatal("move changed identity")
	}
	for id, want := range map[string]string{a.UUID: "original", b.UUID: "existing", copy.UUID: "original"} {
		f, err := m.Read(ctx, id, 0)
		if err != nil || string(f.Content) != want {
			t.Fatalf("id=%s content=%v err=%v", id, f, err)
		}
	}
	items, err := m.ListDirectory(ctx, "~alice/dest")
	if err != nil || len(items) != 3 {
		t.Fatalf("items=%v err=%v", items, err)
	}
}

func (s *directoryTestStore) AnalysisName(ctx context.Context, uuid string) (string, error) {
	for _, items := range s.links {
		for _, item := range items {
			if item.UUID == uuid {
				return item.Name, nil
			}
		}
	}
	return "", nil
}

func (s *directoryTestStore) DirectoryByUUID(ctx context.Context, id string) (*manager.DirectoryRef, error) {
	return s.DirectoryByPath(ctx, id)
}
func (s *directoryTestStore) DirectoryByPath(ctx context.Context, p string) (*manager.DirectoryRef, error) {
	if !s.dirs[p] {
		return nil, manager.ErrNotFound
	}
	return &manager.DirectoryRef{UUID: p, Path: p}, nil
}
func (s *directoryTestStore) LinkDirectoryFile(ctx context.Context, parent, file, name string) error {
	s.links[parent] = append(s.links[parent], manager.DirectoryEntry{UUID: file, Name: name, Kind: "file"})
	return nil
}
func (s *directoryTestStore) DirectoryEntries(ctx context.Context, id string) ([]manager.DirectoryEntry, error) {
	items := append([]manager.DirectoryEntry{}, s.links[id]...)
	for p := range s.dirs {
		if strings.HasPrefix(p, id+"/") && !strings.Contains(strings.TrimPrefix(p, id+"/"), "/") {
			items = append(items, manager.DirectoryEntry{UUID: p, Name: strings.TrimPrefix(p, id+"/"), Kind: "dir"})
		}
	}
	return items, nil
}

func TestDirectoryImportEmptyAndNested(t *testing.T) {
	st := &directoryTestStore{fakeStore: newFakeStore(), links: map[string][]manager.DirectoryEntry{}}
	st.dirs["~alice"] = true
	m := manager.New(st, t.TempDir(), nil, nil, manager.DownloadConfig{})
	source := t.TempDir()
	if err := os.Mkdir(filepath.Join(source, "empty"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(source, "nested"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "nested", "note.txt"), []byte("payload"), 0644); err != nil {
		t.Fatal(err)
	}
	items, err := m.ImportDirectory(context.Background(), "~alice", source)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("results=%v", items)
	}
	for _, item := range items {
		if item.Error != "" || item.UUID == "" {
			t.Fatalf("result=%+v", item)
		}
	}
	var buf bytes.Buffer
	if err := m.ExportDirectory(context.Background(), "~alice", &buf, func(manager.DirectoryEntry) error { return nil }); err != nil {
		t.Fatal(err)
	}
	z, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, f := range z.File {
		names[f.Name] = true
	}
	if !names["empty/"] || !names["nested/note.txt"] {
		t.Fatalf("zip names=%v", names)
	}
	if _, err := os.Stat(filepath.Join(source, "nested", "note.txt")); err != nil {
		t.Fatal("import must preserve source", err)
	}
}
func (s *directoryTestStore) GetMetaByUUID(ctx context.Context, id string) (*manager.FileRef, error) {
	for _, r := range s.rows {
		if r.UUID == id {
			return &manager.FileRef{UUID: id, Path: r.Path, IsDeleted: r.IsDeleted}, nil
		}
	}
	return nil, nil
}

func TestSameNameUUIDDirectoryExport(t *testing.T) {
	st := &directoryTestStore{fakeStore: newFakeStore(), links: map[string][]manager.DirectoryEntry{}}
	st.dirs["~alice"] = true
	m := manager.New(st, t.TempDir(), nil, nil, manager.DownloadConfig{})
	ctx := context.Background()
	a, err := m.WriteInDirectory(ctx, "~alice", "报告.txt", "first")
	if err != nil {
		t.Fatal(err)
	}
	b, err := m.WriteInDirectory(ctx, "~alice", "报告.txt", "second")
	if err != nil {
		t.Fatal(err)
	}
	if a.UUID == b.UUID {
		t.Fatal("same-name files share identity")
	}
	report, err := m.Audit(ctx)
	if err != nil || len(report.Orphans) != 0 {
		t.Fatalf("normal intake audit=%+v err=%v", report, err)
	}
	for id, want := range map[string]string{a.UUID: "first", b.UUID: "second"} {
		r, err := m.Read(ctx, id, 0)
		if err != nil || string(r.Content) != want {
			t.Fatalf("uuid=%s err=%v", id, err)
		}
	}
	var buf bytes.Buffer
	if err := m.ExportDirectory(ctx, "~alice", &buf, func(manager.DirectoryEntry) error { return nil }); err != nil {
		t.Fatal(err)
	}
	z, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	if len(z.File) != 3 {
		t.Fatalf("archive entries=%d", len(z.File))
	}
	contents := map[string]bool{}
	for _, f := range z.File {
		r, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(r)
		r.Close()
		if err != nil {
			t.Fatal(err)
		}
		contents[string(data)] = true
	}
	if !contents["first"] || !contents["second"] {
		t.Fatal("duplicate-name export lost content")
	}
	if err := m.ExportDirectory(ctx, "~alice", io.Discard, func(manager.DirectoryEntry) error { return errors.New("denied") }); err == nil {
		t.Fatal("export bypassed authorization")
	}
}

func TestRecoveryDoesNotPublishPartial(t *testing.T) {
	m, st, _, dir := newTestManager(t)
	id, err := st.ReserveMeta(context.Background(), "partial.txt")
	if err != nil {
		t.Fatal(err)
	}
	rel, _ := manager.StoragePathOf(id, "partial.txt")
	abs := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs+".partial", []byte("half"), 0644); err != nil {
		t.Fatal(err)
	}
	c, a, err := m.RecoverPendingIntakes(context.Background())
	if err != nil || c != 0 || a != 1 {
		t.Fatalf("completed=%d archived=%d err=%v", c, a, err)
	}
	if _, err := os.Stat(abs); !os.IsNotExist(err) {
		t.Fatal("partial was published")
	}
}

func TestDirectoryRequestReplay(t *testing.T) {
	st := &directoryTestStore{fakeStore: newFakeStore(), links: map[string][]manager.DirectoryEntry{}}
	st.dirs["~alice"] = true
	m := manager.New(st, t.TempDir(), nil, nil, manager.DownloadConfig{})
	ctx := context.Background()
	a, err := m.WriteInDirectoryRequest(ctx, "~alice", "a.txt", "hello", "r1")
	if err != nil {
		t.Fatal(err)
	}
	b, err := m.WriteInDirectoryRequest(ctx, "~alice", "a.txt", "hello", "r1")
	if err != nil || a.UUID != b.UUID {
		t.Fatalf("replay=%v err=%v", b, err)
	}
	for _, tc := range []struct{ name, content string }{{"b.go", "hello"}, {"a.txt", "changed"}} {
		if _, err := m.WriteInDirectoryRequest(ctx, "~alice", tc.name, tc.content, "r1"); err == nil {
			t.Fatal("parameter mismatch accepted")
		}
	}
	if len(st.links["~alice"]) != 1 {
		t.Fatal("retry created duplicate")
	}
	if _, err := m.AnalyzeFile(ctx, a.LogicPath); err != nil {
		t.Fatal(err)
	}
	if st.ups[a.LogicPath].Name != "a.txt" {
		t.Fatalf("analysis lost display name: %+v", st.ups[a.LogicPath])
	}
}
