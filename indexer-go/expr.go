// 文件：indexer-go/expr.go —— 布尔表达式树：叶子条件 + and/or/not 组合与求值支撑
// 修改：2026-09-11（日期由 fresh-header.ps1 刷新）

// Package indexer 的布尔组合语言（2026-09-11）：
//
// 早期查询入口只吃「扁平条件数组 = 全部 And」，Or 虽在桶组合层实现却没有
// 入口——agent 无法表达"或/分组"。本文件把查询入口升级为表达式树：
//
//	叶子   Leaf(field, op, value)          —— 单字段条件（原 Condition）
//	与     And(child1, child2, ...)        —— 全部命中（交集）
//	或     Or(child1, child2, ...)         —— 任一命中（并集）
//	非     Not(child)                      —— 子表达式不命中（补集）
//
// 树可任意嵌套，天然支持 "(A 或 B) 且 (C 且非 D)" 这类真实检索意图。
// 空表达式（零叶子、零组合）= 空集，与本包"空条件无命中"的既有口径一致。
package indexer

// Expr 布尔条件表达式：恰好一种形态有效——
//   - 叶子：Cond != nil（Field/Op/Value 三键）
//   - 与：len(And) > 0，全部子表达式命中
//   - 或：len(Or) > 0，任一子表达式命中
//   - 非：Not != nil，子表达式不命中（在索引全集上取补集）
type Expr struct {
	Cond *Condition // 叶子条件
	And  []Expr     // 与组合
	Or   []Expr     // 或组合
	Not  *Expr      // 非组合
}

// Leaf 构造叶子表达式（单字段条件）。
func Leaf(field string, op Op, value any) Expr {
	return Expr{Cond: &Condition{Field: field, Op: op, Value: value}}
}

// And 构造与组合（全部命中）。
func And(children ...Expr) Expr { return Expr{And: children} }

// Or 构造或组合（任一命中）。
func Or(children ...Expr) Expr { return Expr{Or: children} }

// Not 构造非组合（子表达式不命中）。
func Not(child Expr) Expr { return Expr{Not: &child} }

// IsEmpty 空表达式判定（零叶子零组合）——调用方据此短路成空集，
// 不把空查询误当"全集"。
func (e Expr) IsEmpty() bool {
	return e.Cond == nil && len(e.And) == 0 && len(e.Or) == 0 && e.Not == nil
}

// Count 叶子条件总数（含嵌套组合，日志/诊断用）。
func (e Expr) Count() int {
	switch {
	case e.Cond != nil:
		return 1
	case e.Not != nil:
		return e.Not.Count()
	case len(e.And) > 0:
		n := 0
		for _, c := range e.And {
			n += c.Count()
		}
		return n
	case len(e.Or) > 0:
		n := 0
		for _, c := range e.Or {
			n += c.Count()
		}
		return n
	default:
		return 0
	}
}

// Leaves 把纯 And 树摊平成叶子条件（供 SQL 降级判断）：
// 只有「And 套 And 套叶子」的结构可摊平；一旦遇到 Or / Not 返回 ok=false
// （SQL 的 attributes @> 只有包含语义，装不下或与非）。
// 空表达式返回 (nil, true)。
func (e Expr) Leaves() ([]Condition, bool) {
	switch {
	case e.Cond != nil:
		return []Condition{*e.Cond}, true
	case e.Not != nil || len(e.Or) > 0:
		return nil, false
	case len(e.And) > 0:
		out := make([]Condition, 0, len(e.And))
		for _, c := range e.And {
			ls, ok := c.Leaves()
			if !ok {
				return nil, false
			}
			out = append(out, ls...)
		}
		return out, true
	default:
		return nil, true
	}
}
