package describer

import (
	"fmt"
	"sort"
	"strings"
)

// llm 固定字段受控词表（docs/元数据字段说明.md 第 5 节）。
// 除这些键与 sp-llm-* 前缀外，模型轨提交的一切键都会被丢弃。
var llmFixedFields = map[string]bool{
	"llm-semantic-type": true,
	"llm-tone":          true,
	"llm-characters":    true,
	"llm-action":        true,
	"llm-style":         true,
	"llm-summary":       true,
	"llm-source":        true,
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

// SanitizeLLM 模型轨前缀闸门：只放行受控 llm- 字段与 sp-llm-* 自由字段。
// 返回净化结果与被丢弃的键（调用方记 WARN）。闸门只验前缀与取值域，
// 不验完整性——任意子集都合法（可空性原则）。
func SanitizeLLM(in map[string]any) (map[string]any, []string) {
	out := make(map[string]any, len(in))
	var dropped []string
	for k, v := range in {
		if llmFixedFields[k] {
			switch k {
			case "llm-semantic-type":
				s, ok := v.(string)
				if !ok || !semanticTypes[s] {
					dropped = append(dropped, k)
					continue
				}
				out[k] = s
			case "llm-characters":
				arr, ok := toStringSlice(v, 10)
				if !ok {
					dropped = append(dropped, k)
					continue
				}
				out[k] = arr
			case "llm-summary":
				out[k] = TrimRunes(fmt.Sprint(v), 100)
			default:
				s := strings.TrimSpace(fmt.Sprint(v))
				if s == "" {
					dropped = append(dropped, k)
					continue
				}
				out[k] = s
			}
			continue
		}
		if strings.HasPrefix(k, "sp-llm-") {
			out[k] = v // 自由字段：任意放行
			continue
		}
		dropped = append(dropped, k)
	}
	sort.Strings(dropped) // 确定性输出（日志与测试友好）
	return out, dropped
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
