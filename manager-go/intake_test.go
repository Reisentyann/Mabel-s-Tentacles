// 文件：manager-go/intake_test.go —— 入库域 L1：新建重名保护、外部文件导入、修改与逻辑视图
// 修改：2026-10-08（日期由 fresh-header.ps1 刷新）

package manager_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Reisentyann/Mabel-s-Tentacles/manager-go"
)

// TestIntakeWriteStorageDerived 入库落盘在 uuid 派生位（<uuid前2>/<uuid><ext>）：
// 盘面无明文逻辑名——物理布局对 agent 不可猜（防猜口径的落点）。
func TestIntakeWriteStorageDerived(t *testing.T) {
	m, st, _, dir := newTestManager(t)
	r, err := m.Write(context.Background(), "小说/第一章.txt", "正文")
	if err != nil {
		t.Fatal(err)
	}
	if r.UUID == "" || r.LogicPath != "小说/第一章.txt" {
		t.Fatalf("receipt = %+v", r)
	}
	want := filepath.Join(dir, filepath.FromSlash(r.StorageRel))
	b, err := os.ReadFile(want)
	if err != nil {
		t.Fatalf("storage file missing at derived path %q: %v", r.StorageRel, err)
	}
	if string(b) != "正文" {
		t.Fatalf("storage content = %q", b)
	}
	// 盘面没有明文逻辑名（只有派生随机名）
	if _, err := os.Stat(filepath.Join(dir, "小说")); !os.IsNotExist(err) {
		t.Fatal("logic dir must not exist on disk")
	}
	// 行的 uuid 与回执一致（Reserve 占位行幂等）
	if st.uuids["小说/第一章.txt"] != r.UUID {
		t.Fatalf("row uuid = %q, receipt uuid = %q", st.uuids["小说/第一章.txt"], r.UUID)
	}
}

// TestIntakeWriteConflict 重名不改变原内容、UUID、属性或缺失计数。
func TestIntakeWriteConflict(t *testing.T) {
	m, st, _, dir := newTestManager(t)
	ctx := context.Background()
	r1, err := m.Write(ctx, "a.txt", "v1")
	if err != nil {
		t.Fatal(err)
	}
	st.rows["a.txt"].Attributes = []byte(`{"llm-description":"keep"}`)
	st.missing["a.txt"] = 2
	for _, deleted := range []bool{false, true} {
		st.rows["a.txt"].IsDeleted = deleted
		if r, err := m.Write(ctx, "a.txt", "v2-longer"); r != nil || !errors.Is(err, manager.ErrKeyExists) {
			t.Fatalf("deleted=%v receipt=%+v err=%v", deleted, r, err)
		}
		b, err := os.ReadFile(filepath.Join(dir, r1.StorageRel))
		if err != nil || string(b) != "v1" || st.uuids["a.txt"] != r1.UUID || st.missing["a.txt"] != 2 || string(st.rows["a.txt"].Attributes) != `{"llm-description":"keep"}` {
			t.Fatalf("conflict changed old file: content=%q err=%v", b, err)
		}
	}
}

func TestImportConflictPreservesSource(t *testing.T) {
	m, st, _, dir := newTestManager(t)
	ctx := context.Background()
	r, err := m.Write(ctx, "existing.zip", "old")
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "new.zip")
	if err := os.WriteFile(source, []byte("new"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, deleted := range []bool{false, true} {
		st.rows["existing.zip"].IsDeleted = deleted
		if _, err := m.ImportFile(ctx, "existing.zip", source); !errors.Is(err, manager.ErrKeyExists) {
			t.Fatalf("got %v", err)
		}
		for p, want := range map[string]string{source: "new", filepath.Join(dir, r.StorageRel): "old"} {
			b, err := os.ReadFile(p)
			if err != nil || string(b) != want {
				t.Fatalf("%s = %q, %v", p, b, err)
			}
		}
	}
}

// atomicIntakeStore 模拟数据库唯一约束；两台 Manager 共享同一占位存储。
type atomicIntakeStore struct {
	*fakeStore
	mu sync.Mutex
}

func (s *atomicIntakeStore) ReserveMeta(ctx context.Context, key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.fakeStore.ReserveMeta(ctx, key)
}

func TestConcurrentIntakeConflict(t *testing.T) {
	st := &atomicIntakeStore{fakeStore: newFakeStore()}
	dir := t.TempDir()
	managers := []*manager.Manager{
		manager.New(st, dir, nil, nil, manager.DownloadConfig{}),
		manager.New(st, dir, nil, nil, manager.DownloadConfig{}),
	}
	start := make(chan struct{})
	results := make(chan error, 16)
	for i := 0; i < 16; i++ {
		go func(i int) {
			<-start
			_, err := managers[i%2].Write(context.Background(), "race.txt", "winner")
			results <- err
		}(i)
	}
	close(start)
	winners := 0
	for i := 0; i < 16; i++ {
		err := <-results
		if err == nil {
			winners++
		} else if !errors.Is(err, manager.ErrKeyExists) {
			t.Fatal(err)
		}
	}
	if winners != 1 {
		t.Fatalf("successful writers = %d", winners)
	}
}

func TestModifyDeletedDoesNotWrite(t *testing.T) {
	m, st, _, dir := newTestManager(t)
	r, err := m.Write(context.Background(), "deleted.txt", "old")
	if err != nil {
		t.Fatal(err)
	}
	st.rows["deleted.txt"].IsDeleted = true
	for _, mode := range []string{"append", "overwrite"} {
		if err := m.Modify(context.Background(), "deleted.txt", "new", mode); !errors.Is(err, manager.ErrDeleted) {
			t.Fatal(err)
		}
	}
	b, err := os.ReadFile(filepath.Join(dir, r.StorageRel))
	if err != nil || string(b) != "old" {
		t.Fatalf("content=%q err=%v", b, err)
	}
}

// TestImportFile 管理机外部文件接入口：下载器只提供源文件路径，管理机
// 负责占位、uuid 派生物理位与文件转移，不要求调用方拼接 data 布局。
func TestImportFile(t *testing.T) {
	m, st, _, dir := newTestManager(t)
	source := filepath.Join(t.TempDir(), "download.zip")
	if err := os.WriteFile(source, []byte("zip-payload"), 0o644); err != nil {
		t.Fatal(err)
	}

	r, err := m.ImportFile(context.Background(), "comics/download.zip", source)
	if err != nil {
		t.Fatal(err)
	}
	if r.UUID == "" || r.LogicPath != "comics/download.zip" || r.SizeBytes != int64(len("zip-payload")) {
		t.Fatalf("import receipt = %+v", r)
	}
	if st.uuids[r.LogicPath] != r.UUID {
		t.Fatalf("row uuid = %q, receipt uuid = %q", st.uuids[r.LogicPath], r.UUID)
	}
	stored := filepath.Join(dir, filepath.FromSlash(r.StorageRel))
	b, err := os.ReadFile(stored)
	if err != nil {
		t.Fatalf("stored import missing: %v", err)
	}
	if string(b) != "zip-payload" {
		t.Fatalf("stored import = %q", b)
	}
}

func TestFailedIntakeReleasesName(t *testing.T) {
	for _, importing := range []bool{false, true} {
		t.Run(fmt.Sprint(importing), func(t *testing.T) {
			st := newFakeStore()
			root := filepath.Join(t.TempDir(), "blocked")
			if err := os.WriteFile(root, []byte("obstruction"), 0644); err != nil {
				t.Fatal(err)
			}
			m := manager.New(st, root, nil, nil, manager.DownloadConfig{})
			source := filepath.Join(t.TempDir(), "source.txt")
			if err := os.WriteFile(source, []byte("payload"), 0644); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel() // 归档不应继承已取消请求。
			var err error
			if importing {
				_, err = m.ImportFile(ctx, "retry.txt", source)
			} else {
				_, err = m.Write(ctx, "retry.txt", "payload")
			}
			if err == nil {
				t.Fatal("expected disk error")
			}
			if st.uuids["retry.txt"] != "" {
				t.Fatal("failed name remains occupied")
			}
			if err := os.Rename(root, root+".saved"); err != nil {
				t.Fatal(err)
			}
			if importing {
				_, err = m.ImportFile(context.Background(), "retry.txt", source)
			} else {
				_, err = m.Write(context.Background(), "retry.txt", "payload")
			}
			if err != nil {
				t.Fatal("retry failed", err)
			}
			r, err := m.ReadByLogic(context.Background(), "retry.txt", 0)
			if err != nil || string(r.Content) != "payload" {
				t.Fatalf("retry read: %+v %v", r, err)
			}
		})
	}
}

func TestRecoverPendingIntakes(t *testing.T) {
	m, st, _, dir := newTestManager(t)
	for _, tc := range []struct {
		path string
		file bool
	}{{"stored.txt", true}, {"missing.txt", false}} {
		uuid, err := st.ReserveMeta(context.Background(), tc.path)
		if err != nil {
			t.Fatal(err)
		}
		if tc.file {
			rel, _ := manager.StoragePathOf(uuid, tc.path)
			abs := filepath.Join(dir, filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(abs, []byte("saved"), 0644); err != nil {
				t.Fatal(err)
			}
		}
	}
	completed, archived, err := m.RecoverPendingIntakes(context.Background())
	if err != nil || completed != 1 || archived != 1 {
		t.Fatalf("completed=%d archived=%d err=%v", completed, archived, err)
	}
	if len(st.pending) != 0 || st.uuids["missing.txt"] != "" {
		t.Fatalf("pending=%v uuids=%v", st.pending, st.uuids)
	}
}

// TestIntakeModifyModes Modify 双模：append 追加 / overwrite 覆写；
// 无行拒绝（修改不是创建入口）。
func TestIntakeModifyModes(t *testing.T) {
	m, _, _, _ := newTestManager(t)
	ctx := context.Background()
	if _, err := m.Write(ctx, "m.txt", "abc"); err != nil {
		t.Fatal(err)
	}
	if err := m.Modify(ctx, "m.txt", "def", "append"); err != nil {
		t.Fatal(err)
	}
	rf, err := m.ReadByLogic(ctx, "m.txt", 0)
	if err != nil {
		t.Fatal(err)
	}
	if string(rf.Content) != "abcdef" {
		t.Fatalf("after append = %q, want abcdef", rf.Content)
	}
	if err := m.Modify(ctx, "m.txt", "xyz", "overwrite"); err != nil {
		t.Fatal(err)
	}
	rf, err = m.ReadByLogic(ctx, "m.txt", 0)
	if err != nil {
		t.Fatal(err)
	}
	if string(rf.Content) != "xyz" {
		t.Fatalf("after overwrite = %q, want xyz", rf.Content)
	}
	if err := m.Modify(ctx, "无此文件.txt", "x", "append"); err == nil {
		t.Fatal("modify must reject missing row")
	}
}

// TestIntakeValidLogicPath 逻辑键校验：拒空 / 前导分隔符 / 盘符 /
// ..、.、空段；正常多段通过。
func TestIntakeValidLogicPath(t *testing.T) {
	m, _, _, _ := newTestManager(t)
	ctx := context.Background()
	for _, bad := range []string{"", "/abs.txt", "\\win.txt", "C:/x", "a/./b", "a/../b", "a//b", "a/", "../x"} {
		if _, err := m.Write(ctx, bad, "x"); err == nil {
			t.Fatalf("Write(%q) should be rejected", bad)
		}
	}
	if _, err := m.Write(ctx, "正常/多段/文件.txt", "x"); err != nil {
		t.Fatalf("valid multi-segment path rejected: %v", err)
	}
}

// TestIntakeLogicViews 逻辑视图：LogicPaths 字典序全集 / LogicTree 段建树
// （目录在前稳定序、叶子带计量）。
func TestIntakeLogicViews(t *testing.T) {
	m, _, _, _ := newTestManager(t)
	ctx := context.Background()
	for _, p := range []string{"b/2.txt", "a/1.txt", "top.txt"} {
		if _, err := m.Write(ctx, p, "内容-"+p); err != nil {
			t.Fatal(err)
		}
	}
	paths, err := m.LogicPaths(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(paths, ","); got != "a/1.txt,b/2.txt,top.txt" {
		t.Fatalf("LogicPaths = %q", got)
	}

	tree, err := m.LogicTree(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(tree) != 3 || tree[0].Name != "a" || tree[0].Type != "dir" || tree[1].Name != "b" || tree[2].Name != "top.txt" {
		t.Fatalf("tree order = %s, want dirs a,b first then top.txt", names(tree))
	}
	if tree[0].Children[0].Path != "a/1.txt" {
		t.Fatalf("tree child path = %q", tree[0].Children[0].Path)
	}
}

func TestCreateDirectoryAndLogicTree(t *testing.T) {
	m, st, _, _ := newTestManager(t)
	ctx := context.Background()
	for _, dir := range []string{"空目录", "空目录/深层"} {
		if err := m.CreateDirectory(ctx, dir); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.CreateDirectory(ctx, "自动父/子"); err != nil {
		t.Fatal(err)
	}
	if !movedDirExists(st, "自动父") || !movedDirExists(st, "自动父/子") {
		t.Fatal("parent directories were not persisted")
	}
	if err := m.CreateDirectory(ctx, "空目录"); !errors.Is(err, manager.ErrDirectoryExists) {
		t.Fatalf("duplicate directory = %v", err)
	}
	if _, err := m.Write(ctx, "文件.txt", "data"); err != nil {
		t.Fatal(err)
	}
	if err := m.CreateDirectory(ctx, "文件.txt"); !errors.Is(err, manager.ErrKeyExists) {
		t.Fatalf("file-directory collision = %v", err)
	}
	if _, err := m.Write(ctx, "空目录", "data"); !errors.Is(err, manager.ErrKeyExists) {
		t.Fatalf("directory-file collision = %v", err)
	}
	if _, err := m.Write(ctx, "文件.txt/child.txt", "data"); err == nil {
		t.Fatal("file cannot be a parent")
	}
	tree, err := m.LogicTree(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := names(tree); got != "空目录,自动父,文件.txt" || len(tree[0].Children) != 1 || tree[0].Children[0].Path != "空目录/深层" {
		t.Fatalf("tree = %+v", tree)
	}
}

func movedDirExists(st *fakeStore, path string) bool { return st.dirs[path] }

// names 测试便捷：节点名列表。
func names(nodes []*manager.LogicNode) string {
	parts := make([]string, 0, len(nodes))
	for _, n := range nodes {
		parts = append(parts, n.Name)
	}
	return strings.Join(parts, ",")
}

// TestStoragePathOfShape 派生规则钉死：前 2 位分层 + 保留扩展名；
// 畸形 uuid 拒绝。
func TestStoragePathOfShape(t *testing.T) {
	rel, err := manager.StoragePathOf("a3f9b2c1-0000-0000-0000-000000000000", "小说/第一章.txt")
	if err != nil {
		t.Fatal(err)
	}
	if rel != "a3/a3f9b2c1-0000-0000-0000-000000000000.txt" {
		t.Fatalf("storage rel = %q", rel)
	}
	if _, err := manager.StoragePathOf("u", "x.txt"); err == nil {
		t.Fatal("short uuid must be rejected")
	}
}

// TestMovePureKeyChange 纯键改（ext 不变）：uuid 不变、物理位不动、
// 零盘操作——intake 域设计红利的兑现口径；谱系 moved_from 记原键。
func TestMovePureKeyChange(t *testing.T) {
	m, st, _, dir := newTestManager(t)
	ctx := context.Background()
	r, err := m.Write(ctx, "小说/旧名.txt", "正文")
	if err != nil {
		t.Fatal(err)
	}

	mv, err := m.Move(ctx, "小说/旧名.txt", "小说/新名.txt")
	if err != nil {
		t.Fatal(err)
	}
	if mv.UUID != r.UUID {
		t.Fatalf("uuid changed on move: %q → %q", r.UUID, mv.UUID)
	}
	if mv.StorageMove {
		t.Fatal("same ext must be pure DB rename (zero disk ops)")
	}
	// 谱系：行迁到新键 + moved_from 记原键
	if st.uuids["小说/新名.txt"] != r.UUID || st.uuids["小说/旧名.txt"] != "" {
		t.Fatal("key space must migrate")
	}
	if row := st.rows["小说/新名.txt"]; row == nil || row.MovedFrom != "小说/旧名.txt" {
		t.Fatalf("row after move = %+v (moved_from missing?)", row)
	}
	if st.rows["小说/旧名.txt"] != nil {
		t.Fatal("old key row must be gone")
	}
	// 内容随键可达（uuid 派生位不变）
	rf, err := m.ReadByLogic(ctx, "小说/新名.txt", 0)
	if err != nil || string(rf.Content) != "正文" {
		t.Fatalf("read after move = %q err=%v", rf.Content, err)
	}
	_ = dir
}

// TestMoveExtChangeRename ext 变化：物理位随派生规则 rename（uuid 分层
// 目录不变，同卷原子）；rename 失败键改回滚（行是事实源口径由调用方保证，
// 本用例钉正路径）。
func TestMoveExtChangeRename(t *testing.T) {
	m, st, _, dir := newTestManager(t)
	ctx := context.Background()
	r, err := m.Write(ctx, "笔记.txt", "内容")
	if err != nil {
		t.Fatal(err)
	}

	mv, err := m.Move(ctx, "笔记.txt", "笔记.md")
	if err != nil {
		t.Fatal(err)
	}
	if !mv.StorageMove {
		t.Fatal("ext change must rename stored file")
	}
	if mv.UUID != r.UUID {
		t.Fatalf("uuid changed on move: %q → %q", r.UUID, mv.UUID)
	}
	// 新派生位有文件、旧派生位没有
	newRel, _ := manager.StoragePathOf(r.UUID, "笔记.md")
	if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(newRel))); err != nil {
		t.Fatalf("new storage missing: %v", err)
	}
	oldRel, _ := manager.StoragePathOf(r.UUID, "笔记.txt")
	if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(oldRel))); !os.IsNotExist(err) {
		t.Fatal("old storage must be gone after rename")
	}
	_ = st
}

// TestMoveGuards 拒绝口径：无行 / 目标占用 / 同键 / 软删行。
func TestMoveGuards(t *testing.T) {
	m, st, _, _ := newTestManager(t)
	ctx := context.Background()
	if _, err := m.Write(ctx, "a.txt", "x"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Write(ctx, "b.txt", "y"); err != nil {
		t.Fatal(err)
	}

	if _, err := m.Move(ctx, "无此文件.txt", "z.txt"); err == nil {
		t.Fatal("missing row must be rejected")
	}
	if _, err := m.Move(ctx, "a.txt", "b.txt"); err == nil {
		t.Fatal("occupied target must be rejected")
	}
	if _, err := m.Move(ctx, "a.txt", "a.txt"); err == nil {
		t.Fatal("same key must be rejected")
	}
	// 软删行拒取内容口径（fake 状态直改，同 BackfillGhost 用例模式）
	st.softDeleted["a.txt"] = true
	st.rows["a.txt"].IsDeleted = true
	if _, err := m.Move(ctx, "a.txt", "c.txt"); err == nil {
		t.Fatal("soft-deleted row must be rejected")
	}
	_ = st
}
