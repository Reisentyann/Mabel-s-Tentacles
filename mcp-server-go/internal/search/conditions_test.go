// 文件：mcp-server-go/internal/search/conditions_test.go —— 条件语法解析单测：扁平数组 / and·or·not 嵌套 / 错误话术
// 修改：2026-09-11（日期由 fresh-header.ps1 刷新）

package search

import (
	"strings"
	"testing"

	"github.com/Reisentyann/Mabel-s-Tentacles/indexer-go"
)

func TestParseConditionsFlatArray(t *testing.T) {
	raw := `[{"field":"cod-text-language","op":"eq","value":"zh"},
	         {"field":"cod-text-lines","op":"gt","value":50}]`
	expr, err := ParseConditions(raw)
	if err != nil {
		t.Fatal(err)
	}
	leaves, ok := expr.Leaves()
	if !ok || len(leaves) != 2 {
		t.Fatalf("扁平数组应解析为 And 两叶子: %+v ok=%v", leaves, ok)
	}
	if leaves[0].Field != "cod-text-language" || leaves[1].Op != indexer.OpGt {
		t.Fatalf("叶子错位: %+v", leaves)
	}
}

func TestParseConditionsNested(t *testing.T) {
	raw := `{"and":[
	  {"field":"cod-text-cjk-ratio","op":"gt","value":0.5},
	  {"or":[
	    {"field":"cod-text-lines","op":"gt","value":200},
	    {"field":"cod-text-dialog-ratio","op":"gt","value":0.15}
	  ]},
	  {"not":{"field":"cod-basic-textish","op":"eq","value":false}}
	]}`
	expr, err := ParseConditions(raw)
	if err != nil {
		t.Fatal(err)
	}
	if expr.IsEmpty() || expr.Count() != 4 {
		t.Fatalf("Count = %d, want 4", expr.Count())
	}
	// 含 or/not → 不可降级
	if _, ok := expr.Leaves(); ok {
		t.Fatal("含 or/not 不应可摊平降级")
	}
	// 结构断言：顶层 And，含 or 与 not 子节点
	if len(expr.And) != 3 {
		t.Fatalf("顶层 And 子节点 = %d, want 3", len(expr.And))
	}
	if len(expr.And[1].Or) != 2 || expr.And[2].Not == nil {
		t.Fatalf("嵌套结构不符: %+v", expr)
	}
}

func TestParseConditionsTopLeafAndNotList(t *testing.T) {
	// 顶层单叶子（便捷）
	expr, err := ParseConditions(`{"field":"cod-text-lines","op":"gt","value":1}`)
	if err != nil || expr.Cond == nil {
		t.Fatalf("顶层单叶子解析失败: %+v err=%v", expr, err)
	}
	// not 收数组 = 非(全体 And)
	expr, err = ParseConditions(`{"not":[{"field":"a","op":"eq","value":1},{"field":"b","op":"eq","value":2}]}`)
	if err != nil || expr.Not == nil || len(expr.Not.And) != 2 {
		t.Fatalf("not 收数组应解析为 非(And): %+v err=%v", expr, err)
	}
	// 原生结构化入参（MCP GetArguments 可能给 []any / map）
	expr, err = ParseConditions([]any{map[string]any{"field": "a", "op": "eq", "value": float64(1)}})
	if err != nil || expr.Count() != 1 {
		t.Fatalf("原生数组入参解析失败: %+v err=%v", expr, err)
	}
}

func TestParseConditionsErrors(t *testing.T) {
	cases := []struct {
		name string
		raw  any
		want string
	}{
		{"nil", nil, "required"},
		{"空串", "", "须为 JSON"},
		{"空数组", `[]`, "空"},
		{"空 or", `{"or":[]}`, "空"},
		{"坏 op", `[{"field":"a","op":"like","value":"x"}]`, "不合法"},
		{"缺 field", `[{"op":"eq","value":1}]`, "缺 field"},
		{"缺 value", `[{"field":"a","op":"eq"}]`, "缺 value"},
		{"and 混 field", `{"and":[],"field":"a"}`, "只能有一个键"},
		{"双重组合", `{"and":[],"or":[]}`, "只能有一个键"},
		{"exists 非 bool", `[{"field":"a","op":"exists","value":"yes"}]`, "true/false"},
		{"contains 非串", `[{"field":"a","op":"contains","value":1}]`, "字符串"},
		{"not 收标量", `{"not":42}`, "条件项须为"},
	}
	for _, c := range cases {
		_, err := ParseConditions(c.raw)
		if err == nil {
			t.Errorf("%s: 期望报错", c.name)
			continue
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: 错误话术 %q 不含 %q", c.name, err.Error(), c.want)
		}
	}
}
