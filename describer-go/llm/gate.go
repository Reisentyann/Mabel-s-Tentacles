// 文件：describer-go/llm/gate.go —— 模型轨前缀闸门：llm 受控词表 / sp-llm 放行 / 其余拒绝
// 修改：2026-09-03（日期由 fresh-header.ps1 刷新）

// Package llm 是模型轨：LLM 来源对 attributes 的一切写操作的前缀闸门与
// LLMStore 中间件（唯一写入口）。策略见 docs/元数据字段说明.md 第 5 节。
package llm

import (
	"fmt"
	"sort"
	"strings"
)

// llm 固定字段受控词表（docs/元数据字段说明.md 第 5 节）。
// 除这些键与 sp-llm-* 前缀外，模型轨提交的一切键都会被拒绝。
// 注意 llm-source 在词表内但属审计字段：LLMStore 会先拦截并系统盖戳。
var llmFixedFields = map[string]bool{
	"llm-semantic-type": true,
	"llm-tone":          true,
	"llm-characters":    true,
	"llm-action":        true,
	"llm-style":         true,
	"llm-summary":       true,
	"llm-source":        true, // 审计字段：仅 LLMStore.Commit 可写
}

// llm-semantic-type 受控枚举（模糊类型词表）。
var semanticTypes = map[string]bool{
	"novel":         true,
	"game_guide":    true,
	"technical_doc": true,
	"note":          true,
	"log":           true,
	"meme":          true,
	"illustration":  true,
	"photo":         true,
	"screenshot":    true,
	"code_artifact": true,
	"data":          true,
	"other":         true,
}

// SanitizeLLM 兼容层：等价于 OpenLLM().SetMany(in) 后取暂存字段与被拒键。
// 新代码应直接使用 LLMStore（支持 null 墓碑删除与 Rejection 原因回传）。
func SanitizeLLM(in map[string]any) (map[string]any, []string) {
	st := OpenLLM()
	st.SetMany(in)
	rej := st.Rejected()
	dropped := make([]string, 0, len(rej))
	for _, r := range rej {
		dropped = append(dropped, r.Key)
	}
	sort.Strings(dropped)
	return st.Fields(), dropped
}
func toStringSlice(v any, max int) ([]string, bool) {
	switch arr := v.(type) {
	case []string:
		out := make([]string, 0, len(arr))
		for _, s := range arr {
			if s = strings.TrimSpace(s); s != "" {
				out = append(out, s)
			}
			if len(out) >= max {
				break
			}
		}
		return out, true
	case []any:
		out := make([]string, 0, len(arr))
		for _, item := range arr {
			s := strings.TrimSpace(fmt.Sprint(item))
			if s != "" {
				out = append(out, s)
			}
			if len(out) >= max {
				break
			}
		}
		return out, true
	case string:
		parts := strings.Split(arr, ",")
		return toStringSlice(parts, max)
	}
	return nil, false
}
