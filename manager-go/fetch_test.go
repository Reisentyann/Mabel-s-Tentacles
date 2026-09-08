// 文件：manager-go/fetch_test.go —— 取件域 L1：Locate/LocateMany/Open/Read 语义（三哨兵 + 软删照报 + buffer 命中/失效/旁路 + 限读）
// 修改：2026-09-08（日期由 fresh-header.ps1 刷新）

package manager_test

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/Reisentyann/Mabel-s-Tentacles/manager-go"
)

// putRef 测试便捷：往 fakeStore 塞一条取件事实（uuid ↔ 逻辑路径）。
func putRef(st *fakeStore, uuid, rel string, deleted bool) {
	st.refs[uuid] = &manager.FileRef{
		UUID: uuid, Path: rel, Scope: "global",
		SizeBytes: int64(len(uuid)), MimeType: "text/plain",
		IsDeleted: deleted,
	}
}

// putStored 测试便捷：把文件内容放到 uuid 派生的物理存储位
// （intake 域口径：<uuid前2>/<uuid><ext>——openRef 只从派生路径读）。
func putStored(t *testing.T, dir, uuid, logicPath, content string) {
	t.Helper()
	rel, err := manager.StoragePathOf(uuid, logicPath)
	if err != nil {
		t.Fatal(err)
	}
	abs := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestLocate 单取：有行回执 / 无行 ErrNotFound / 软删照报（IsDeleted=true）。
func TestLocate(t *testing.T) {
	m, st, _, dir := newTestManager(t)
	putRef(st, "u-have", "a/有.txt", false)
	putRef(st, "u-del", "a/删.txt", true)
	putStored(t, dir, "u-have", "a/有.txt", "x") // Locate 不做盘上检查，但顺手铺盘面

	ref, err := m.Locate(context.Background(), "u-have")
	if err != nil || ref == nil {
		t.Fatalf("Locate ok-case = (%v, %v), want ref", ref, err)
	}
	if ref.Path != "a/有.txt" || ref.IsDeleted {
		t.Fatalf("Locate ref = %+v", ref)
	}

	if _, err := m.Locate(context.Background(), "u-none"); !errors.Is(err, manager.ErrNotFound) {
		t.Fatalf("Locate missing err = %v, want ErrNotFound", err)
	}

	ref, err = m.Locate(context.Background(), "u-del")
	if err != nil || !ref.IsDeleted {
		t.Fatalf("Locate deleted = (%v, %v), want 照报 IsDeleted", ref, err)
	}
}

// TestLocateMany 批量：缺的不入 map / 软删照报 / 空入空出。
func TestLocateMany(t *testing.T) {
	m, st, _, _ := newTestManager(t)
	putRef(st, "u1", "one.txt", false)
	putRef(st, "u2", "two.txt", true)

	refs, err := m.LocateMany(context.Background(), []string{"u1", "u-miss", "u2"})
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 2 {
		t.Fatalf("LocateMany len = %d, want 2（缺失不入 map）", len(refs))
	}
	if refs["u1"].IsDeleted || !refs["u2"].IsDeleted {
		t.Fatalf("LocateMany refs = %+v", refs)
	}

	empty, err := m.LocateMany(context.Background(), nil)
	if err != nil || len(empty) != 0 {
		t.Fatalf("LocateMany(nil) = (%v, %v), want empty", empty, err)
	}
}

// TestOpenSemantics Open 三哨兵：ErrNotFound / ErrDeleted / ErrGhost，
// 以及正常路径的内容直出。
func TestOpenSemantics(t *testing.T) {
	m, st, _, dir := newTestManager(t)
	putRef(st, "u-del", "删.txt", true)
	putRef(st, "u-ghost", "幽灵.txt", false)
	putRef(st, "u-ok", "正常.txt", false)
	putStored(t, dir, "u-ok", "正常.txt", "梅贝尔的触手书房")

	// "u-really-none" 未塞入 refs → DB 无行 → ErrNotFound
	if _, err := m.Open(context.Background(), "u-really-none"); !errors.Is(err, manager.ErrNotFound) {
		t.Fatalf("Open none err = %v, want ErrNotFound", err)
	}
	if _, err := m.Open(context.Background(), "u-del"); !errors.Is(err, manager.ErrDeleted) {
		t.Fatalf("Open deleted err = %v, want ErrDeleted", err)
	}
	if _, err := m.Open(context.Background(), "u-ghost"); !errors.Is(err, manager.ErrGhost) {
		t.Fatalf("Open ghost err = %v, want ErrGhost", err)
	}

	of, err := m.Open(context.Background(), "u-ok")
	if err != nil {
		t.Fatal(err)
	}
	defer of.Content.Close()
	b, err := io.ReadAll(of.Content)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "梅贝尔的触手书房" {
		t.Fatalf("Open content = %q", b)
	}
	if of.Path != "正常.txt" {
		t.Fatalf("Open ref path = %q", of.Path)
	}
}

// TestOpenBufferHitAndInvalidate buffer 闭环：首次盘读入缓 → 二次命中
// 免盘读（改文件时间戳不漂的前提下）→ 盘上改动（size 漂）→ 新鲜度
// 失效重读新内容（execute_command 绕口的最终一致收敛点）。
func TestOpenBufferHitAndInvalidate(t *testing.T) {
	m, st, _, dir := newTestManager(t)
	putRef(st, "ub", "buf.txt", false)
	putStored(t, dir, "ub", "buf.txt", "v1-content")
	rel, _ := manager.StoragePathOf("ub", "buf.txt")
	abs := filepath.Join(dir, filepath.FromSlash(rel))

	if _, err := m.Open(context.Background(), "ub"); err != nil {
		t.Fatal(err)
	}
	if n, _ := m.BufferStats(); n != 1 {
		t.Fatalf("after first Open, buffer entries = %d, want 1", n)
	}

	// 命中路径：同内容同状态二次 Open，仍取得到
	of, err := m.Open(context.Background(), "ub")
	if err != nil {
		t.Fatal(err)
	}
	of.Content.Close()

	// 盘上改动（长度变化 → size 漂移）→ get 失效 → 重读新内容
	if err := os.WriteFile(abs, []byte("v2-longer-content"), 0o644); err != nil {
		t.Fatal(err)
	}
	rf, err := m.Read(context.Background(), "ub", 0)
	if err != nil {
		t.Fatal(err)
	}
	if string(rf.Content) != "v2-longer-content" {
		t.Fatalf("after disk change, Read = %q, want v2（新鲜度失效重读）", rf.Content)
	}
}

// TestReadLimit Read 限读：limit>0 截断 / limit<=0 全量。
func TestReadLimit(t *testing.T) {
	m, st, _, dir := newTestManager(t)
	putRef(st, "ub", "limit.txt", false)
	putStored(t, dir, "ub", "limit.txt", "0123456789")

	rf, err := m.Read(context.Background(), "ub", 4)
	if err != nil {
		t.Fatal(err)
	}
	if string(rf.Content) != "0123" {
		t.Fatalf("Read(4) = %q, want 0123", rf.Content)
	}

	rf, err = m.Read(context.Background(), "ub", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rf.Content) != 10 {
		t.Fatalf("Read(0) len = %d, want 10（全量）", len(rf.Content))
	}
}

// TestFileRefSemanticsDoc 哨兵错误各自独立（errors.Is 分支判定的前提）。
func TestFileRefSemanticsDoc(t *testing.T) {
	sentinels := []error{manager.ErrNotFound, manager.ErrDeleted, manager.ErrGhost}
	for i, a := range sentinels {
		for j, b := range sentinels {
			if i != j && a == b {
				t.Fatalf("sentinels %d and %d must be distinct", i, j)
			}
		}
	}
}
