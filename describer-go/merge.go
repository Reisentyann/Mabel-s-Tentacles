// 文件：describer-go/merge.go —— cod 轨合并：家族整族替换 + cod-<family>-ver/-at 刷新（纯 JSON 操作）
// 修改：2026-09-03（日期由 fresh-header.ps1 刷新）

package describer

import (
	"encoding/json"
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
// SPPurge 前缀旧键，再写新键，并刷新 cod-<family>-ver / -at
// （版本号见字段字典第 10.1 节，陈旧判定见 IsStale）。
//
// 模型轨（llm-*）不走这里——唯一写入口是 llm 子包的 LLMStore。
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
		if r.Ver > 0 {
			merged["cod-"+r.Family+"-ver"] = r.Ver
		}
		merged["cod-"+r.Family+"-at"] = now.Unix()
	}
	return merged
}
