// 文件：manager-go/intake_test.go —— 入库域 L1：uuid 派生物理位 / 覆写幂等 / Modify 双模 / 逻辑键校验 / 逻辑视图
// 修改：2026-09-08（日期由 fresh-header.ps1 刷新）

package manager_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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

// TestIntakeWriteOverwriteIdempotent 同逻辑键重写 = 覆写：uuid 不变
// （物理位稳定）、内容替换。
func TestIntakeWriteOverwriteIdempotent(t *testing.T) {
	m, _, _, _ := newTestManager(t)
	ctx := context.Background()
	r1, err := m.Write(ctx, "a.txt", "v1")
	if err != nil {
		t.Fatal(err)
	}
	r2, err := m.Write(ctx, "a.txt", "v2-longer")
	if err != nil {
		t.Fatal(err)
	}
	if r1.UUID != r2.UUID {
		t.Fatalf("uuid changed on overwrite: %q → %q", r1.UUID, r2.UUID)
	}
	if r2.SizeBytes != int64(len("v2-longer")) {
		t.Fatalf("receipt size = %d", r2.SizeBytes)
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
