package describer

import (
	"encoding/json"
	"sort"
	"strings"
	"time"
)

// AttrsFromJSON 反序列化 attributes（nil / 空 → 空 map，绝不返回 nil）。
func AttrsFromJSON(raw json.RawMessage) map[string]any {
	m := map[string]any{}
	if len(raw) == 0 {
		return m
	}
	_ = json.Unmarshal(raw, &m)
	return m
}

// JSONFromAttrs 序列化 attributes（空 map → "{}"）。
func JSONFromAttrs(m map[string]any) json.RawMessage {
	if m == nil || len(m) == 0 {
		return json.RawMessage("{}")
	}
	b, err := json.Marshal(m)
	if err != nil {
		return json.RawMessage("{}")
	}
	return b
}

// MergeResults 把一次 Analyze 的全部家族结果整族合并进 existing
// （读-改-写由调用方负责持久化）。每个家族：先清 cod-<family>-* 旧键与
// SPPurge 前缀旧键，再写新键，并刷新 cod-<family>-at 时间戳。
func MergeResults(existing map[string]any, results []Result, now time.Time) map[string]any {
	merged := make(map[string]any, len(existing)+16)
	for k, v := range existing {
		merged[k] = v
	}
	for _, r := range results {
		codPrefix := "cod-" + r.Family + "-"
		for k := range merged {
			if strings.HasPrefix(k, codPrefix) {
				delete(merged, k)
			}
		}
		for _, p := range r.SPPurge {
			for k := range merged {
				if strings.HasPrefix(k, p) {
					delete(merged, k)
				}
			}
		}
		for k, v := range r.Attrs {
			merged[k] = v
		}
		merged["cod-"+r.Family+"-at"] = now.Unix()
	}
	return merged
}

// MergeLLM 模型轨合并：llm- 固定字段只覆盖本次提供的键（COALESCE 语义，
// 未提供的保留旧值），sp-llm-* 键级写入，刷新 llm-at。
// 入参 attrs 必须先过 SanitizeLLM。
func MergeLLM(existing map[string]any, attrs map[string]any, now time.Time) map[string]any {
	merged := make(map[string]any, len(existing)+len(attrs))
	for k, v := range existing {
		merged[k] = v
	}
	for k, v := range attrs {
		merged[k] = v
	}
	merged["llm-at"] = now.Unix()
	return merged
}

// LLMSourceAgent / LLMSourceOllama llm-source 取值约定。
const (
	LLMSourceAgent  = "agent"
	LLMSourceOllama = "ollama"
)

// SortedKeys 确定性遍历辅助（测试与日志用）。
func SortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
