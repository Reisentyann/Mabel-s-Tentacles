// 文件：mcp-server-go/internal/search/conditions.go —— 索引条件解析：JSON 条件数组 → []indexer.Condition（工具层 / HTTP 层共用的一份语义）
// 修改：2026-09-11（日期由 fresh-header.ps1 刷新）

// search 包的条件入口：agent 侧条件语法（JSON 数组 [{field,op,value}]，
// op ∈ eq/in/gt/lt/range/ne/exists/contains）在此归一为 indexer.Condition。
// MCP 工具 search_files 与 HTTP GET /api/files/search?cond= 共用本解析——
// 一处语义，两个入口，永不漂移（op 白名单、错误话术同源）。
package search

import (
	"encoding/json"
	"fmt"

	"github.com/Reisentyann/Mabel-s-Tentacles/indexer-go"
)

// condArg agent 侧条件形态（JSON）；解析后映射为 indexer.Condition。
type condArg struct {
	Field string `json:"field"`
	Op    string `json:"op"`
	Value any    `json:"value"`
}

// validOp op 白名单（与索引机桶级/镜像级语义一致）。
var validOp = map[string]bool{
	"eq": true, "in": true, "gt": true, "lt": true, "range": true,
	"ne": true, "exists": true, "contains": true,
}

// ParseConditions 解析条件 JSON（文本字节或原生数组序列化产物均可）：
// 坏结构 / 空 / 坏 op / 缺 value → 人话错误（教 agent 自纠错，提示先
// list_index_fields 查字段目录）。合法 → []indexer.Condition（And 交集
// 语义，由编排机 SearchByConditions 执行）。
func ParseConditions(raw any) ([]indexer.Condition, error) {
	if raw == nil {
		return nil, fmt.Errorf("conditions is required, e.g. [{\"field\":\"cod-text-language\",\"op\":\"eq\",\"value\":\"zh\"}]（先用 list_index_fields 查可用字段）")
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
	var args []condArg
	if err := json.Unmarshal(buf, &args); err != nil {
		return nil, fmt.Errorf("conditions 须为 JSON 数组 [{\"field\",\"op\",\"value\"}]: %v", err)
	}
	if len(args) == 0 {
		return nil, fmt.Errorf("conditions 为空：至少一个条件（先用 list_index_fields 查可用字段与取值）")
	}
	conds := make([]indexer.Condition, 0, len(args))
	for i, a := range args {
		if a.Field == "" {
			return nil, fmt.Errorf("第 %d 个条件缺 field", i+1)
		}
		if !validOp[a.Op] {
			return nil, fmt.Errorf("第 %d 个条件 op=%q 不合法（支持 eq/in/gt/lt/range/ne/exists/contains）", i+1, a.Op)
		}
		if a.Value == nil {
			return nil, fmt.Errorf("第 %d 个条件缺 value", i+1)
		}
		if err := checkValueShape(i+1, a); err != nil {
			return nil, err
		}
		conds = append(conds, indexer.Condition{Field: a.Field, Op: indexer.Op(a.Op), Value: a.Value})
	}
	return conds, nil
}

// checkValueShape 新 op 的 value 形状校验（入口即拒，教自纠错）：
// exists 须 bool、contains 须 string、ne 须标量。eq/in/gt/lt/range 的
// 值域校验归索引机桶级（既有口径不动）。
func checkValueShape(i int, a condArg) error {
	switch a.Op {
	case "exists":
		if _, ok := a.Value.(bool); !ok {
			return fmt.Errorf("第 %d 个条件 exists 的 value 须为 true/false", i)
		}
	case "contains":
		if _, ok := a.Value.(string); !ok {
			return fmt.Errorf("第 %d 个条件 contains 的 value 须为字符串（要找的子串）", i)
		}
	case "ne":
		switch a.Value.(type) {
		case string, float64, bool:
		default:
			return fmt.Errorf("第 %d 个条件 ne 的 value 须为标量（字符串/数字/布尔）", i)
		}
	}
	return nil
}
