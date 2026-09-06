// 文件：indexer-go/bucket.go —— 三型桶数据结构：每字段独立索引，加字段=加桶（架构设计.md 第 3 节）
// 修改：2026-09-05（日期由 fresh-header.ps1 刷新）

// Package indexer 的 bucket.go 实现三型索引桶与归一化原语。
//
// 桶型由字段首个可索引值决定（数值→num / 标量→enum / 数组→multi）；
// 后续值类型不符时尽力归一（数字字符串可入数值桶、数字可作枚举键），
// 归一失败静默跳过——本包是 best-effort 派生缓存，DB 才是事实源，
// SQL 检索保留兜底，脏值不值得报错中断喂食。
package indexer

import (
	"fmt"
	"sort"
	"strconv"
)

// bucket 单字段的索引桶。每字段独立建桶互不影响——新字段支持索引
// 即注册一个桶，不动其他桶（字段级扩展性的落点）。
//
// 幂等约定：add 同 (uuid, 值) 重复挂载 no-op；remove 不存在的挂载 no-op。
// 同一字段一个 uuid 只应有一个现值（attributes 键唯一）——值变时的
// 先移除后挂入由 memIndexer.Update 的 diff 语义保证。
//
// query 返回的集合是桶内部集合（只读约定：调用方不得改写）——
// And/Or 组合只读不拷贝，个人库量级下省一次大集拷贝。
type bucket interface {
	// add 将 uuid 挂入值对应的位（多值字段一桶多挂）。
	add(uuid string, v any)
	// remove 将 uuid 从值对应的位移除。
	remove(uuid string, v any)
	// query 按条件求命中 uuid 集合（桶型只支持自己语义的 Op）。
	query(cond Condition) (map[string]struct{}, error)
	// stats 键数与挂载数（Stats 自省用；num 桶 keys 按条目计）。
	stats() (keys, mounts int)
}

// enumBucket 枚举 / bool / 单值字符串桶：map[值]→uuid 集合，等值 O(1)。
// 适用：cod-image-family、cod-text-language、cod-text-eol、
// llm-semantic-type、cod-basic-name-pattern、bool 类字段等。
type enumBucket struct {
	m map[string]map[string]struct{}
}

func (b *enumBucket) add(uuid string, v any) {
	k, ok := keyOf(v)
	if !ok {
		return
	}
	set, ok := b.m[k]
	if !ok {
		set = map[string]struct{}{}
		b.m[k] = set
	}
	set[uuid] = struct{}{}
}

func (b *enumBucket) remove(uuid string, v any) {
	k, ok := keyOf(v)
	if !ok {
		return
	}
	set := b.m[k]
	delete(set, uuid)
	if len(set) == 0 {
		delete(b.m, k)
	}
}

func (b *enumBucket) query(cond Condition) (map[string]struct{}, error) {
	return queryKeyed(b.m, cond)
}

func (b *enumBucket) stats() (int, int) {
	return keyedStats(b.m)
}

// multiBucket 数组多值桶：值→uuid 集合，一个文件可挂多个值。
// 适用：tags、llm-characters、cod-text-structure、cod-text-top-keywords
// 等一切 array 字段（OpEq/OpIn 命中任一元素即整文件命中）。
type multiBucket struct {
	m map[string]map[string]struct{}
}

func (b *multiBucket) add(uuid string, v any) {
	for _, el := range sliceOf(v) {
		if k, ok := keyOf(el); ok {
			b.mount(k, uuid)
		}
	}
}

func (b *multiBucket) remove(uuid string, v any) {
	for _, el := range sliceOf(v) {
		if k, ok := keyOf(el); ok {
			b.unmount(k, uuid)
		}
	}
}

func (b *multiBucket) query(cond Condition) (map[string]struct{}, error) {
	return queryKeyed(b.m, cond)
}

func (b *multiBucket) stats() (int, int) {
	return keyedStats(b.m)
}

// keyedStats 键值桶（enum/multi 同构）的计量：键数 + 挂载总数。
func keyedStats(m map[string]map[string]struct{}) (int, int) {
	mounts := 0
	for _, set := range m {
		mounts += len(set)
	}
	return len(m), mounts
}

func (b *multiBucket) mount(k, uuid string) {
	set, ok := b.m[k]
	if !ok {
		set = map[string]struct{}{}
		b.m[k] = set
	}
	set[uuid] = struct{}{}
}

func (b *multiBucket) unmount(k, uuid string) {
	set := b.m[k]
	delete(set, uuid)
	if len(set) == 0 {
		delete(b.m, k)
	}
}

// queryKeyed 键值桶（enum/multi 同构）的查询：eq 直查 / in 并集。
// eq 命中返回桶内部集合（缺键为 nil map，range 安全）。
func queryKeyed(m map[string]map[string]struct{}, cond Condition) (map[string]struct{}, error) {
	switch cond.Op {
	case OpEq:
		k, ok := keyOf(cond.Value)
		if !ok {
			return nil, fmt.Errorf("indexer: 字段 %s 的 eq 条件值不可作键（%T）", cond.Field, cond.Value)
		}
		return m[k], nil
	case OpIn:
		vals := sliceOf(cond.Value)
		if vals == nil {
			return nil, fmt.Errorf("indexer: 字段 %s 的 in 条件值须为数组（%T）", cond.Field, cond.Value)
		}
		out := map[string]struct{}{}
		for _, v := range vals {
			if k, ok := keyOf(v); ok {
				for uuid := range m[k] {
					out[uuid] = struct{}{}
				}
			}
		}
		return out, nil
	}
	return nil, fmt.Errorf("indexer: 键值桶只支持 eq/in，字段 %s 收到 %s", cond.Field, cond.Op)
}

// numBucket 数值桶：(值, uuid) 有序切片，二分范围扫描。
// 适用：cod-text-digit-ratio、cod-image-megapixels、cod-text-time-span、
// cod-image-brightness 等一切 number 字段（OpEq/OpGt/OpLt/OpRange/OpIn）。
// 维护成本：插入 O(n) 挪位（个人库量级足够，将来量大再换 B-tree）。
type numBucket struct {
	sorted []numEntry
}

// numEntry 数值桶条目。val 为归一后的 float64（JSON 数值统一类型）。
type numEntry struct {
	val  float64
	uuid string
}

func (b *numBucket) add(uuid string, v any) {
	f, ok := toFloat(v)
	if !ok {
		return
	}
	i := b.lowerBound(f)
	for j := i; j < len(b.sorted) && b.sorted[j].val == f; j++ {
		if b.sorted[j].uuid == uuid {
			return // 幂等：同值同 uuid 已在桶
		}
	}
	b.sorted = append(b.sorted, numEntry{})
	copy(b.sorted[i+1:], b.sorted[i:])
	b.sorted[i] = numEntry{val: f, uuid: uuid}
}

func (b *numBucket) remove(uuid string, v any) {
	f, ok := toFloat(v)
	if !ok {
		return
	}
	for j := b.lowerBound(f); j < len(b.sorted) && b.sorted[j].val == f; j++ {
		if b.sorted[j].uuid == uuid {
			b.sorted = append(b.sorted[:j], b.sorted[j+1:]...)
			return
		}
	}
}

func (b *numBucket) query(cond Condition) (map[string]struct{}, error) {
	switch cond.Op {
	case OpEq:
		f, ok := toFloat(cond.Value)
		if !ok {
			return nil, errNumValue(cond)
		}
		return b.collect(b.lowerBound(f), b.upperBound(f)), nil
	case OpGt:
		f, ok := toFloat(cond.Value)
		if !ok {
			return nil, errNumValue(cond)
		}
		return b.collect(b.upperBound(f), len(b.sorted)), nil
	case OpLt:
		f, ok := toFloat(cond.Value)
		if !ok {
			return nil, errNumValue(cond)
		}
		return b.collect(0, b.lowerBound(f)), nil
	case OpRange:
		lo, hi, ok := rangeOf(cond.Value)
		if !ok {
			return nil, fmt.Errorf("indexer: 字段 %s 的 range 条件值须为 [lo,hi]（%T）", cond.Field, cond.Value)
		}
		if lo > hi {
			return map[string]struct{}{}, nil // 空区间：无命中
		}
		return b.collect(b.lowerBound(lo), b.upperBound(hi)), nil
	case OpIn:
		vals := sliceOf(cond.Value)
		if vals == nil {
			return nil, fmt.Errorf("indexer: 字段 %s 的 in 条件值须为数组（%T）", cond.Field, cond.Value)
		}
		out := map[string]struct{}{}
		for _, v := range vals {
			if f, ok := toFloat(v); ok {
				for uuid := range b.collect(b.lowerBound(f), b.upperBound(f)) {
					out[uuid] = struct{}{}
				}
			}
		}
		return out, nil
	}
	return nil, fmt.Errorf("indexer: 数值桶只支持 eq/gt/lt/range/in，字段 %s 收到 %s", cond.Field, cond.Op)
}

// stats 数值桶计量：keys 与 mounts 都按条目计（一个条目即一次挂载）。
func (b *numBucket) stats() (int, int) {
	return len(b.sorted), len(b.sorted)
}

// lowerBound 首个 val >= f 的下标；upperBound 首个 val > f 的下标。
func (b *numBucket) lowerBound(f float64) int {
	return sort.Search(len(b.sorted), func(i int) bool { return b.sorted[i].val >= f })
}

func (b *numBucket) upperBound(f float64) int {
	return sort.Search(len(b.sorted), func(i int) bool { return b.sorted[i].val > f })
}

// collect 收集 [from, to) 的 uuid（新建集合，调用方可安全持有）。
func (b *numBucket) collect(from, to int) map[string]struct{} {
	out := make(map[string]struct{}, to-from)
	for i := from; i < to; i++ {
		out[b.sorted[i].uuid] = struct{}{}
	}
	return out
}

// —— 归一化原语（挂载与查询两侧共用，保证同形值同形键）——

// keyOf 标量桶键：string 原样、bool → true/false、数字 → g 格式
// （int 3 与 float64 3 同键；数字误入字符串字段不炸，脏数据友好）。
func keyOf(v any) (string, bool) {
	switch x := v.(type) {
	case string:
		return x, true
	case bool:
		if x {
			return "true", true
		}
		return "false", true
	case float64:
		return strconv.FormatFloat(x, 'g', -1, 64), true
	case int:
		return strconv.FormatFloat(float64(x), 'g', -1, 64), true
	case int64:
		return strconv.FormatFloat(float64(x), 'g', -1, 64), true
	}
	return "", false
}

// toFloat 数值归一：JSON 反序列化数值为 float64，describer 原生产出 int/int64；
// 查询侧同 IsStale.toInt 的宽松风格，接受数字字符串。
func toFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case string:
		f, err := strconv.ParseFloat(x, 64)
		return f, err == nil
	}
	return 0, false
}

// sliceOf 数组展开：[]any / []string → 元素切片；非数组返回 nil
// （空数组返回空切片非 nil，与"不是数组"区分）。
func sliceOf(v any) []any {
	switch x := v.(type) {
	case []any:
		return x
	case []string:
		out := make([]any, len(x))
		for i, s := range x {
			out[i] = s
		}
		return out
	}
	return nil
}

// rangeOf range 条件值：[2]any{lo,hi} 或 []any{lo,hi} 两种形态。
func rangeOf(v any) (float64, float64, bool) {
	var lo, hi any
	switch x := v.(type) {
	case [2]any:
		lo, hi = x[0], x[1]
	case []any:
		if len(x) != 2 {
			return 0, 0, false
		}
		lo, hi = x[0], x[1]
	default:
		return 0, 0, false
	}
	l, ok1 := toFloat(lo)
	h, ok2 := toFloat(hi)
	return l, h, ok1 && ok2
}

func errNumValue(cond Condition) error {
	return fmt.Errorf("indexer: 字段 %s 的 %s 条件值须为数值（%T）", cond.Field, cond.Op, cond.Value)
}
