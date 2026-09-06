// 文件：manager-go/fetch_test.go —— 取件域接口轮契约测试：stub 语义钉死（未实现错误 + 零值返回不 panic；空入空出即时生效）
// 修改：2026-09-06（日期由 fresh-header.ps1 刷新）

package manager_test

import (
	"context"
	"testing"

	"github.com/Reisentyann/Mabel-s-Tentacles/manager-go"
)

// TestFetchStubs 接口轮钉面：四个入口全部返回未实现错误与零值结果
// （实现批次落地后本用例改写为真语义断言）。
func TestFetchStubs(t *testing.T) {
	m, _, _, _ := newTestManager(t)
	ctx := context.Background()

	if ref, err := m.Locate(ctx, "u1"); err == nil || ref != nil {
		t.Fatalf("Locate stub = (%v, %v), want (nil, err)", ref, err)
	}
	if of, err := m.Open(ctx, "u1"); err == nil || of != nil {
		t.Fatalf("Open stub = (%v, %v), want (nil, err)", of, err)
	}
	if rf, err := m.Read(ctx, "u1", 0); err == nil || rf != nil {
		t.Fatalf("Read stub = (%v, %v), want (nil, err)", rf, err)
	}
	if refs, err := m.LocateMany(ctx, []string{"u1"}); err == nil || refs != nil {
		t.Fatalf("LocateMany stub = (%v, %v), want (nil, err)", refs, err)
	}
}

// TestLocateManyEmptyPinned 空入空出：零依赖语义接口轮即生效（不返回
// 未实现错误——空集合没有可取的东西）。
func TestLocateManyEmptyPinned(t *testing.T) {
	m, _, _, _ := newTestManager(t)
	refs, err := m.LocateMany(context.Background(), nil)
	if err != nil {
		t.Fatalf("LocateMany(nil) err = %v, want nil", err)
	}
	if len(refs) != 0 {
		t.Fatalf("LocateMany(nil) = %v, want empty map", refs)
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
