// 文件：mcp-server-go/internal/search/conditions.go —— 索引条件解析：JSON 条件语法 → indexer.Expr 布尔表达式树（工具层 / HTTP 层共用的一份语义）
// 修改：2026-09-11（日期由 fresh-header.ps1 刷新）

// search 包的条件入口：agent 侧条件语法在此归一为 indexer.Expr。
// MCP 工具 search_files 与 HTTP GET /api/files/search?cond= 共用本解析——
// 一处语义，两个入口，永不漂移（op 白名单、错误话术同源）。
//
// 语法（检索语言扩展 2026-09-11，布尔组合）：
//
//	扁平数组   [{"field","op","value"}, ...]         = 全部 And（旧口径，向后兼容）
//	与组合     {"and":[<node>, ...]}
//	或组合     {"or": [<node>, ...]}
//	非组合     {"not": <node>}
//
// 其中 <node> 是叶子 {"field","op","value"} 或另一个组合节点，可任意嵌套——
// "(中文 或 英文) 且 (长文 且 非日志)" 这类真实检索意图直接表达。
package search

import (
	"encoding/json"
	"fmt"

	"github.com/Reisentyann/Mabel-s-Tentacles/indexer-go"
)

// validOp op 白名单（与索引机桶级/镜像级语义一致）。
var validOp = map[string]bool{
	"eq": true, "in": true, "gt": true, "lt": true, "range": true,
	"ne": true, "exists": true, "contains": true,
}

// ParseConditions 解析条件语法 → indexer.Expr 表达式树。
// 坏结构 / 空 / 坏 op / 缺 value → 人话错误（教 agent 自纠错，提示先
// list_index_fields 查字段目录）。空表达式（无任何条件）报错——
// 调用方应在解析前就拦掉"空查询"，不把它当"全集"。
func ParseConditions(raw any) (indexer.Expr, error) {
	v, err := normalizeJSON(raw)
	if err != nil {
		return indexer.Expr{}, err
	}
	expr, perr := parseNode(v)
	if perr != nil {
		return indexer.Expr{}, perr
	}
	if expr.IsEmpty() {
		return indexer.Expr{}, fmt.Errorf("conditions 为空：至少一个条件（先用 list_index_fields 查可用字段与取值）")
	}
	return expr, nil
}

// normalizeJSON 把入参归一成 JSON 解码后的通用结构（map/[]any/标量）：
// 文本（MCP 字符串入参 / HTTP query 值）→ 直接解码；已结构化（原生数组、
// json.RawMessage 等）→ Marshal 往返一次，保证类型形态统一。
func normalizeJSON(raw any) (any, error) {
	if raw == nil {
		return nil, fmt.Errorf(`conditions is required, e.g. [{"field":"cod-text-language","op":"eq","value":"zh"}]（先用 list_index_fields 查可用字段）`)
	}
	var buf []byte
	switch x := raw.(type) {
	case string:
		buf = []byte(x)
	case []byte:
		buf = x
	default:
		b, err := json.Marshal(x)
		if err != nil {
			return nil, fmt.Errorf("conditions 序列化失败: %v", err)
		}
		buf = b
	}
	var v any
	if err := json.Unmarshal(buf, &v); err != nil {
		return nil, fmt.Errorf("conditions 须为 JSON（数组 = 全部 And；或 {and/or/not} 组合）: %v", err)
	}
	return v, nil
}

// parseNode 递归解析一个节点：数组 = And；对象 = 叶子或组合。
func parseNode(v any) (indexer.Expr, error) {
	switch x := v.(type) {
	case []any:
		children, err := parseList(x)
		if err != nil {
			return indexer.Expr{}, err
		}
		return indexer.And(children...), nil
	case map[string]any:
		return parseObject(x)
	default:
		return indexer.Expr{}, fmt.Errorf("条件项须为对象 {field,op,value} 或组合 {and/or/not}，收到 %T", v)
	}
}

// parseObject 解析对象节点：含 and/or/not 键即组合，否则视为叶子。
func parseObject(m map[string]any) (indexer.Expr, error) {
	combos := 0
	for _, k := range []string{"and", "or", "not"} {
		if _, ok := m[k]; ok {
			combos++
		}
	}
	if combos == 0 {
		return parseLeaf(m)
	}
	if combos > 1 || len(m) > 1 {
		return indexer.Expr{}, fmt.Errorf("组合节点只能有一个键（and / or / not），不能与 field/op/value 混用")
	}

	if v, ok := m["and"]; ok {
		children, err := parseListValue(v)
		if err != nil {
			return indexer.Expr{}, fmt.Errorf("and %v", err)
		}
		return indexer.And(children...), nil
	}
	if v, ok := m["or"]; ok {
		children, err := parseListValue(v)
		if err != nil {
			return indexer.Expr{}, fmt.Errorf("or %v", err)
		}
		return indexer.Or(children...), nil
	}
	child, err := parseNode(m["not"])
	if err != nil {
		return indexer.Expr{}, fmt.Errorf("not: %w", err)
	}
	return indexer.Not(child), nil
}

// parseListValue 组合键值须为数组（and/or 的子条件列表）。
func parseListValue(v any) ([]indexer.Expr, error) {
	arr, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("须为数组（元素为条件或组合），收到 %T", v)
	}
	return parseList(arr)
}

// parseList 解析子节点列表；空数组报错（组合至少一个条件）。
func parseList(arr []any) ([]indexer.Expr, error) {
	if len(arr) == 0 {
		return nil, fmt.Errorf("数组为空：至少一个条件")
	}
	children := make([]indexer.Expr, 0, len(arr))
	for i, it := range arr {
		c, err := parseNode(it)
		if err != nil {
			return nil, fmt.Errorf("第 %d 项：%w", i+1, err)
		}
		children = append(children, c)
	}
	return children, nil
}

// parseLeaf 解析叶子：field + op + value 三键齐全，op 白名单 + 值形状校验。
func parseLeaf(m map[string]any) (indexer.Expr, error) {
	field, _ := m["field"].(string)
	if field == "" {
		return indexer.Expr{}, fmt.Errorf("叶子条件缺 field（或用 and/or/not 组合）")
	}
	op, _ := m["op"].(string)
	if !validOp[op] {
		return indexer.Expr{}, fmt.Errorf("字段 %s 的 op=%q 不合法（支持 eq/in/gt/lt/range/ne/exists/contains）", field, op)
	}
	value, has := m["value"]
	if !has || value == nil {
		return indexer.Expr{}, fmt.Errorf("字段 %s 缺 value", field)
	}
	if err := checkValueShape(field, op, value); err != nil {
		return indexer.Expr{}, err
	}
	return indexer.Leaf(field, indexer.Op(op), value), nil
}

// checkValueShape 新 op 的 value 形状校验（入口即拒，教自纠错）：
// exists 须 bool、contains 须 string、ne 须标量。eq/in/gt/lt/range 的
// 值域校验归索引机桶级（既有口径不动）。
func checkValueShape(field, op string, value any) error {
	switch op {
	case "exists":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("字段 %s 的 exists 的 value 须为 true/false", field)
		}
	case "contains":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("字段 %s 的 contains 的 value 须为字符串（要找的子串）", field)
		}
	case "ne":
		switch value.(type) {
		case string, float64, bool:
		default:
			return fmt.Errorf("字段 %s 的 ne 的 value 须为标量（字符串/数字/布尔）", field)
		}
	}
	return nil
}
