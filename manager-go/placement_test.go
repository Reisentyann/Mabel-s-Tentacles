// 文件：manager-go/placement_test.go —— 位置域 L1：Resolve 薄壳 + 逻辑口回执与 uuid 口同口径
// 修改：2026-09-11（日期由 fresh-header.ps1 刷新）

package manager_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Reisentyann/Mabel-s-Tentacles/manager-go"
)

// TestResolve 位置域薄壳（P0 收口）：Resolve(ctx,uuid) = Locate(uuid).Path；
// 无行 → ErrNotFound（与 Locate 同哨兵）。
func TestResolve(t *testing.T) {
	m, st, _, _ := newTestManager(t)
	putRef(st, "u-ok", "小说/第一章.txt", false)

	got, err := m.Resolve(context.Background(), "u-ok")
	if err != nil || got != "小说/第一章.txt" {
		t.Fatalf("Resolve = (%q, %v), want 小说/第一章.txt", got, err)
	}
	if _, err := m.Resolve(context.Background(), "u-none"); !errors.Is(err, manager.ErrNotFound) {
		t.Fatalf("Resolve missing err = %v, want ErrNotFound", err)
	}
}

// TestOpenByLogicReceipt 逻辑口回执补全（P0）：Scope/MimeType/SizeBytes 与
// uuid 口同口径，不再只回 UUID/Path。
func TestOpenByLogicReceipt(t *testing.T) {
	m, st, _, dir := newTestManager(t)
	// 逻辑口走 GetMeta（rows 事实源）；盘面按 uuid 派生位铺（openRef 只从派生路径读）。
	st.rows["b/文.txt"] = &manager.MetaRow{
		Path: "b/文.txt", UUID: "u-receipt", Scope: "user",
		MimeType: "text/plain", SizeBytes: 3,
	}
	putStored(t, dir, "u-receipt", "b/文.txt", "abc")

	of, err := m.OpenByLogic(context.Background(), "b/文.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer of.Content.Close()
	if of.Scope != "user" || of.MimeType != "text/plain" {
		t.Fatalf("OpenByLogic ref = %+v, want scope/mime carried", of.FileRef)
	}
	if of.SizeBytes != 3 {
		t.Fatalf("OpenByLogic SizeBytes = %d, want 3", of.SizeBytes)
	}
	if _, err := m.OpenByLogic(context.Background(), "无此行.txt"); !errors.Is(err, manager.ErrNotFound) {
		t.Fatalf("OpenByLogic missing err = %v, want ErrNotFound", err)
	}
}
