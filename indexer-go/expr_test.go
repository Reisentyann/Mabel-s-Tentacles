// 文件：indexer-go/expr_test.go —— 布尔表达式树单测：嵌套 and/or/not 求值 / 补集 / Leaves 降级判定
// 修改：2026-09-11（日期由 fresh-header.ps1 刷新）

package indexer

import (
	"reflect"
	"testing"
)

// TestQueryNestedBoolean 布尔组合语言（2026-09-11）：任意嵌套 and/or/not。
// 样例库同 sampleAll：u1(zh,420) / u2(en,12) / u3(zh,88)。
func TestQueryNestedBoolean(t *testing.T) {
	ix := New()
	if err := ix.Rebuild(sampleAll()); err != nil {
		t.Fatal(err)
	}
	zh := Leaf("cod-text-language", OpEq, "zh")
	en := Leaf("cod-text-language", OpEq, "en")
	long := Leaf("cod-text-lines", OpGt, 100)

	// (zh 且 lines>100) 或 en → u1 / u2
	got := mustQueryExpr(t, ix, Or(And(zh, long), en))
	if !reflect.DeepEqual(got, []string{"u1", "u2"}) {
		t.Fatalf("(zh ∧ long) ∨ en → %v, want [u1 u2]", got)
	}

	// zh 且 非(long) → u3
	got = mustQueryExpr(t, ix, And(zh, Not(long)))
	if !reflect.DeepEqual(got, []string{"u3"}) {
		t.Fatalf("zh ∧ ¬long → %v, want [u3]", got)
	}

	// 顶层非：非(zh) → 全集里的 u2（补集只认索引全集）
	got = mustQueryExpr(t, ix, Not(zh))
	if !reflect.DeepEqual(got, []string{"u2"}) {
		t.Fatalf("¬zh → %v, want [u2]", got)
	}

	// 非(键存在)：字段谁都没有 → 补集 = 全集
	got = mustQueryExpr(t, ix, Not(Leaf("cod-image-taken-at", OpExists, true)))
	if !reflect.DeepEqual(got, []string{"u1", "u2", "u3"}) {
		t.Fatalf("¬exists 从未出现字段 → %v, want 全集", got)
	}

	// 深度嵌套：((zh ∨ en) ∧ (lines<100 ∨ lines>400)) → u2(12) / u3(88) / u1(420)
	got = mustQueryExpr(t, ix, And(
		Or(zh, en),
		Or(Leaf("cod-text-lines", OpLt, 100), Leaf("cod-text-lines", OpGt, 400)),
	))
	if !reflect.DeepEqual(got, []string{"u1", "u2", "u3"}) {
		t.Fatalf("深度嵌套 → %v, want [u1 u2 u3]", got)
	}

	// 空组合恒等元：And() = 空集（口径：空表达式无命中）
	if got := mustQueryExpr(t, ix, And()); len(got) != 0 {
		t.Fatalf("空 And → %v, want 空集", got)
	}
}

// mustQueryExpr 表达式直查助手。
func mustQueryExpr(t *testing.T, ix Indexer, expr Expr) []string {
	t.Helper()
	out, err := ix.Query(expr)
	if err != nil {
		t.Fatalf("Query(%+v) 出错: %v", expr, err)
	}
	return out
}

// TestExprLeaves SQL 降级判定：纯 And 树可摊平成叶子；遇到 or/not 不可降级。
func TestExprLeaves(t *testing.T) {
	zh := Leaf("cod-text-language", OpEq, "zh")
	long := Leaf("cod-text-lines", OpGt, 100)

	// 纯 And（含嵌套 And）→ 全摊平，保持顺序
	flat, ok := And(And(zh, long), Leaf("cod-basic-textish", OpEq, true)).Leaves()
	if !ok || len(flat) != 3 {
		t.Fatalf("纯 And Leaves = (%v, %v), want 3 项 true", flat, ok)
	}
	if flat[0].Field != "cod-text-language" || flat[2].Field != "cod-basic-textish" {
		t.Fatalf("摊平顺序错: %+v", flat)
	}

	// 单叶子
	if flat, ok := zh.Leaves(); !ok || len(flat) != 1 {
		t.Fatalf("单叶子 Leaves = (%v, %v)", flat, ok)
	}

	// or / not 不可降级
	if _, ok := Or(zh, long).Leaves(); ok {
		t.Fatal("Or 不应可降级")
	}
	if _, ok := Not(zh).Leaves(); ok {
		t.Fatal("Not 不应可降级")
	}
	// And 里混 or → 也不可降级
	if _, ok := And(zh, Or(long, Leaf("x", OpEq, "y"))).Leaves(); ok {
		t.Fatal("And 含 Or 不应可降级")
	}

	// 空表达式 → 零叶子，可降级
	if flat, ok := (Expr{}).Leaves(); !ok || len(flat) != 0 {
		t.Fatalf("空表达式 Leaves = (%v, %v), want (nil, true)", flat, ok)
	}
}
