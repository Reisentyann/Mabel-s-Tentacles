package llm

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Reisentyann/Mabel-s-Tentacles/describer-go"
)

// LLMSource 取值约定：agent（MCP 客户端模型）/ ollama:<model>（服务端模型）。
const (
	LLMSourceAgent  = "agent"
	LLMSourceOllama = "ollama"
)

// 审计字段：系统专属。模型不可写、不可删，由 Commit 统一盖戳。
// llm-source 此前允许模型自报来源——等于可伪造，现收紧为系统写入。
var llmAuditFields = map[string]bool{
	"llm-source": true,
	"llm-at":     true,
}

// Rejection 一次被中间件拒绝的操作（Reason 是人话，回传给调用方教育模型）。
type Rejection struct {
	Key    string
	Op     string // set | delete
	Reason string
}

// LLMStore LLM 字段操作中间件：LLM 来源对 attributes 的一切写操作
// （增/删/改）必须经过它，这是唯一写入口（查走 GetMetadata 只读，天然安全）。
//
// 策略（docs/元数据字段说明.md 第 5.3 节）：
//   - cod-* / sp-cod-*：绝对只读区——Set/Delete 一律拒绝，确定性字段只归 describer 引擎
//   - llm-* 固定字段：Set 须命中受控词表与值域；Delete 允许（自己轨道的产物）
//   - llm-source / llm-at：审计字段，模型 Set/Delete 均拒绝，Commit 盖戳
//   - sp-llm-*：自由区，Set/Delete 任意
//   - 其余前缀：拒绝
//
// 值为 nil（JSON null）视为删除墓碑。
type LLMStore struct {
	staged   map[string]any
	dels     map[string]bool
	rejected []Rejection
}

// OpenLLM 创建中间件实例。
func OpenLLM() *LLMStore {
	return &LLMStore{staged: map[string]any{}, dels: map[string]bool{}}
}

// SetMany 批量写入（键排序保证拒绝清单确定性）。
func (s *LLMStore) SetMany(fields map[string]any) {
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		s.Set(k, fields[k])
	}
}

// Set 写入一个字段；value 为 nil 视为删除（JSON null 墓碑）。
func (s *LLMStore) Set(key string, value any) {
	if value == nil {
		s.Delete(key)
		return
	}
	if isCodZone(key) {
		s.reject(key, "set", "cod 区只读：确定性字段由 describer 引擎维护，模型不可写")
		return
	}
	if llmAuditFields[key] {
		s.reject(key, "set", "审计字段由系统写入，模型不可伪造")
		return
	}
	if llmFixedFields[key] {
		v, ok := validateLLMValue(key, value)
		if !ok {
			s.reject(key, "set", "取值不合法：未通过受控词表/值域校验")
			return
		}
		s.staged[key] = v
		return
	}
	if strings.HasPrefix(key, "sp-llm-") {
		s.staged[key] = value
		return
	}
	s.reject(key, "set", "未知前缀：只接受 llm-* 与 sp-llm-*")
}

// Delete 删除一个字段。仅允许 llm-* 与 sp-llm-*；cod 区与审计字段拒绝。
func (s *LLMStore) Delete(key string) {
	if isCodZone(key) {
		s.reject(key, "delete", "cod 区只读：模型不可删除确定性字段")
		return
	}
	if llmAuditFields[key] {
		s.reject(key, "delete", "审计字段系统专属，不可删除")
		return
	}
	if llmFixedFields[key] || strings.HasPrefix(key, "sp-llm-") {
		delete(s.staged, key)
		s.dels[key] = true
		return
	}
	s.reject(key, "delete", "未知前缀：只接受 llm-* 与 sp-llm-*")
}

func isCodZone(key string) bool {
	return strings.HasPrefix(key, "cod-") || strings.HasPrefix(key, "sp-cod-")
}

func (s *LLMStore) reject(key, op, reason string) {
	s.rejected = append(s.rejected, Rejection{Key: key, Op: op, Reason: reason})
}

// Rejected 被拒绝的操作清单（Key+Op 排序，确定性）。
func (s *LLMStore) Rejected() []Rejection {
	out := make([]Rejection, len(s.rejected))
	copy(out, s.rejected)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Key != out[j].Key {
			return out[i].Key < out[j].Key
		}
		return out[i].Op < out[j].Op
	})
	return out
}

// Fields 已暂存的写操作（不含删除）；SanitizeLLM 兼容层用。
func (s *LLMStore) Fields() map[string]any {
	return s.staged
}

// HasChanges 是否存在任何写/删操作。
func (s *LLMStore) HasChanges() bool {
	return len(s.staged) > 0 || len(s.dels) > 0
}

// Commit 应用到 existing：先执行删除、再写入，最后盖审计戳
// （llm-source / llm-at）。无变更时原样返回且不盖戳（避免元数据行无谓抖动）。
// cod-* / sp-cod-* 原样透传——中间件从构造上就不可能碰它们。
func (s *LLMStore) Commit(existing map[string]any, source string, now time.Time) map[string]any {
	if !s.HasChanges() {
		return existing
	}
	merged := make(map[string]any, len(existing)+len(s.staged)+2)
	for k, v := range existing {
		merged[k] = v
	}
	for k := range s.dels {
		delete(merged, k)
	}
	for k, v := range s.staged {
		merged[k] = v
	}
	merged["llm-source"] = source
	merged["llm-at"] = now.Unix()
	return merged
}

// validateLLMValue llm- 固定字段的值域校验（词表枚举 / 数组化 / 截断）。
func validateLLMValue(key string, v any) (any, bool) {
	switch key {
	case "llm-semantic-type":
		s, ok := v.(string)
		if !ok || !semanticTypes[s] {
			return nil, false
		}
		return s, true
	case "llm-characters":
		return toStringSlice(v, 10)
	case "llm-summary":
		return describer.TrimRunes(fmt.Sprint(v), 100), true
	default:
		s := strings.TrimSpace(fmt.Sprint(v))
		if s == "" {
			return nil, false
		}
		return s, true
	}
}
