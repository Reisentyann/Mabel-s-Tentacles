// 文件：indexer-go/mem.go —— 索引机进程内内存实现：字段级独立桶 + RWMutex 并发保护
// 修改：2026-09-03（日期由 fresh-header.ps1 刷新）

// Package indexer 的 mem.go 是进程内内存实现：field → 桶。
// DB 是唯一事实源，本实现是派生缓存——可丢弃可重建（Rebuild），
// 查询侧故障时上层自动降级走 SQL 检索兜底（search/sql.go 保留）。
package indexer

import (
	"reflect"
	"sort"
	"sync"
)

// memIndexer 进程内内存实现。
// RWMutex：Query 走读锁、Update/Rebuild 走写锁——mcp-server 单进程内
// HTTP/MCP 并发调用下安全（桶内 map 非并发安全，必须由锁保护）。
type memIndexer struct {
	mu      sync.RWMutex
	buckets map[string]bucket // field → 桶
}

// New 构造进程内索引机（空索引，等待 Rebuild 或增量 Update）。
func New() Indexer {
	return &memIndexer{buckets: map[string]bucket{}}
}

// Query 按条件求 uuid 集合：逐条件求桶内命中，And 交集 / Or 并集，
// 结果升序（map 迭代序随机，排序保证确定）。
// 空条件返回空集不报错；从未挂过值的字段视为无命中（空集）；
// 桶型与 Op 不符（如枚举桶收 range）报错，由上层降级 SQL。
func (m *memIndexer) Query(conds []Condition, mode Combine) ([]string, error) {
	if len(conds) == 0 {
		return []string{}, nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	sets := make([]map[string]struct{}, len(conds))
	for i, c := range conds {
		b, ok := m.buckets[c.Field]
		if !ok {
			sets[i] = map[string]struct{}{} // 该字段从未出现可索引值：无命中
			continue
		}
		set, err := b.query(c)
		if err != nil {
			return nil, err
		}
		sets[i] = set
	}

	out := combine(sets, mode)
	res := make([]string, 0, len(out))
	for uuid := range out {
		res = append(res, uuid)
	}
	sort.Strings(res)
	return res, nil
}

// combine 交/并：And 从最小集起步逐集过滤（一旦空集即早退），
// Or 全并。入参集合为桶内部只读集合，不改写。
func combine(sets []map[string]struct{}, mode Combine) map[string]struct{} {
	order := make([]int, len(sets))
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(a, b int) bool { return len(sets[order[a]]) < len(sets[order[b]]) })

	if mode == Or {
		out := map[string]struct{}{}
		for _, i := range order {
			for uuid := range sets[i] {
				out[uuid] = struct{}{}
			}
		}
		return out
	}
	first := sets[order[0]]
	out := make(map[string]struct{}, len(first))
	for uuid := range first {
		out[uuid] = struct{}{}
	}
	for _, i := range order[1:] {
		if len(out) == 0 {
			return out
		}
		set := sets[i]
		for uuid := range out {
			if _, ok := set[uuid]; !ok {
				delete(out, uuid)
			}
		}
	}
	return out
}

// Update 写路径喂食：old/new 为该文件变更前后的 attributes（读-改-写
// 时由编排方各取一份，Upsert 后调用）。逐字段 diff：
//   - 键消失或值变 → 从旧桶移除
//   - 新增或值变 → 挂入新桶（数组字段值变全量重挂：旧元素逐一移除、新元素逐一挂入）
//   - new 为 nil/空 = 整体移除（文件删除路径）
//
// 幂等：重复喂食同一 (uuid, old, new) 结果恒等——桶级 add/remove 幂等
// 加"值没动不碰桶"。不可索引值（nil / 对象 / 无标量元素的数组）静默跳过。
func (m *memIndexer) Update(uuid string, old, new map[string]any) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for f, ov := range old {
		if sameValue(ov, new[f]) {
			continue // 值没动：不碰桶（new 缺键时 new[f]=nil，与 nil 旧值等价跳过）
		}
		m.unmount(f, uuid, ov)
	}
	for f, nv := range new {
		if ov, existed := old[f]; existed && sameValue(ov, nv) {
			continue
		}
		m.mount(f, uuid, nv)
	}
}

// Rebuild 全量重建：清空全部桶后按 all 重新挂载（服务启动时从 DB 载入
// uuid→attributes）。个人库量级秒级；重建持写锁，期间查询方等待。
// 内存实现无失败路径，error 保留给将来外部化实现。
func (m *memIndexer) Rebuild(all map[string]map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.buckets = map[string]bucket{}
	for uuid, attrs := range all {
		for f, v := range attrs {
			m.mount(f, uuid, v)
		}
	}
	return nil
}

// mount 将 uuid 的单字段值挂入桶；桶不存在则按首个值的类型建桶
// （数值→num / 数组→multi / 其余标量→enum）。不可索引值不建桶。
func (m *memIndexer) mount(field, uuid string, v any) {
	if !indexable(v) {
		return
	}
	b, ok := m.buckets[field]
	if !ok {
		b = newBucketFor(v)
		if b == nil {
			return
		}
		m.buckets[field] = b
	}
	b.add(uuid, v)
}

// unmount 将 uuid 的单字段旧值移出桶（桶不存在=从未挂过，no-op）。
func (m *memIndexer) unmount(field, uuid string, v any) {
	if b, ok := m.buckets[field]; ok {
		b.remove(uuid, v)
	}
}

// indexable 可挂载判定：标量，或含 ≥1 个标量元素的数组。
// nil / 对象 / 纯对象数组（如 cod-image-palette）不入索引。
func indexable(v any) bool {
	switch x := v.(type) {
	case []any:
		for _, el := range x {
			if isScalar(el) {
				return true
			}
		}
		return false
	case []string:
		return len(x) > 0
	default:
		return isScalar(v)
	}
}

func isScalar(v any) bool {
	switch v.(type) {
	case float64, float32, int, int64, string, bool:
		return true
	}
	return false
}

// newBucketFor 按首个值型建桶：数值→num、数组→multi、其余标量→enum。
func newBucketFor(v any) bucket {
	switch v.(type) {
	case float64, float32, int, int64:
		return &numBucket{}
	case []any, []string:
		return &multiBucket{m: map[string]map[string]struct{}{}}
	case string, bool:
		return &enumBucket{m: map[string]map[string]struct{}{}}
	}
	return nil
}

// sameValue 归一化深度相等：数值族统一 float64（int 3 == float64 3）、
// []string 与 []any 逐元素对齐（describer 原生口径 vs DB JSON 口径），
// 数组逐元素递归；不可归一的（nil/对象）退回 reflect.DeepEqual。
func sameValue(a, b any) bool {
	na, okA := normalize(a)
	nb, okB := normalize(b)
	if !okA || !okB {
		return reflect.DeepEqual(a, b)
	}
	return reflect.DeepEqual(na, nb)
}

// normalize 归一化：数值族→float64、[]string→[]any（数组逐元素递归）。
// nil 与 map 等复合对象不可归一（ok=false）。
func normalize(v any) (any, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case string, bool:
		return x, true
	case []string:
		out := make([]any, len(x))
		for i, s := range x {
			out[i] = s
		}
		return out, true
	case []any:
		out := make([]any, len(x))
		for i, el := range x {
			if n, ok := normalize(el); ok {
				out[i] = n
			} else {
				out[i] = el
			}
		}
		return out, true
	}
	return v, false
}
