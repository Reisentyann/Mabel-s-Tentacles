// 文件：indexer-go/mem.go —— 索引机进程内内存实现：字段级独立桶 + RWMutex 并发保护 + Stats 自省
// 修改：2026-09-11（日期由 fresh-header.ps1 刷新）

// Package indexer 的 mem.go 是进程内内存实现：field → 桶。
// DB 是唯一事实源，本实现是派生缓存——可丢弃可重建（Rebuild），
// 查询侧故障时上层自动降级走 SQL 检索兜底（search/sql.go 保留）。
package indexer

import (
	"reflect"
	"sort"
	"strings"
	"sync"
)

// memIndexer 进程内内存实现。
// RWMutex：Query 走读锁、Update/Rebuild 走写锁——mcp-server 单进程内
// HTTP/MCP 并发调用下安全（桶内 map 非并发安全，必须由锁保护）。
//
// 两层结构：
//   - buckets：field → 桶（eq/in/gt/lt/range 的快路径，值寻址 O(1)）
//   - attrs：uuid → 原始 attributes 镜像（ne/exists/contains 的扫描路径，
//     2026-09-10 检索语言扩展批次）——语义需要"键存在性/值不等于/子串"，
//     桶结构装不下（桶只挂可索引值，缺键信息在桶里不可见）；镜像在
//     Update/Rebuild 与桶同步维护，个人库量级内存翻倍可接受
type memIndexer struct {
	mu      sync.RWMutex
	buckets map[string]bucket         // field → 桶
	attrs   map[string]map[string]any // uuid → 原始 attributes（扫描型 op 的支撑）
}

// New 构造进程内索引机（空索引，等待 Rebuild 或增量 Update）。
func New() Indexer {
	return &memIndexer{buckets: map[string]bucket{}, attrs: map[string]map[string]any{}}
}

// Query 按布尔表达式求 uuid 集合（And/Or/Not 任意嵌套，见 expr.go）：
// 递归求值后升序返回（map 迭代序随机，排序保证确定）。
// 空表达式返回空集不报错；从未挂过值的字段视为无命中（空集）；
// 桶型与 Op 不符（如枚举桶收 range）报错，由上层降级 SQL。
func (m *memIndexer) Query(expr Expr) ([]string, error) {
	if expr.IsEmpty() {
		return []string{}, nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	set, err := m.eval(expr)
	if err != nil {
		return nil, err
	}
	res := make([]string, 0, len(set))
	for uuid := range set {
		res = append(res, uuid)
	}
	sort.Strings(res)
	return res, nil
}

// eval 递归求值布尔表达式树（调用方已持读锁）。集合均为桶内只读集或
// 求值新建集，调用方可安全持有。
func (m *memIndexer) eval(e Expr) (map[string]struct{}, error) {
	switch {
	case e.Cond != nil:
		return m.evalLeaf(*e.Cond)
	case e.Not != nil:
		child, err := m.eval(*e.Not)
		if err != nil {
			return nil, err
		}
		return m.complement(child), nil
	case len(e.And) > 0:
		sets := make([]map[string]struct{}, 0, len(e.And))
		for _, c := range e.And {
			s, err := m.eval(c)
			if err != nil {
				return nil, err
			}
			sets = append(sets, s)
		}
		return intersect(sets), nil
	case len(e.Or) > 0:
		sets := make([]map[string]struct{}, 0, len(e.Or))
		for _, c := range e.Or {
			s, err := m.eval(c)
			if err != nil {
				return nil, err
			}
			sets = append(sets, s)
		}
		return union(sets), nil
	default:
		return map[string]struct{}{}, nil // 空表达式（顶层已短路，防御）
	}
}

// evalLeaf 叶子求值：扫描型 op（ne/exists/contains）走 attrs 镜像，
// 其余走字段桶；从未出现可索引值的字段 = 无命中（空集，不报错）。
func (m *memIndexer) evalLeaf(c Condition) (map[string]struct{}, error) {
	switch c.Op {
	case OpNe, OpExists, OpContains:
		return m.scanSet(c), nil
	}
	b, ok := m.buckets[c.Field]
	if !ok {
		return map[string]struct{}{}, nil
	}
	return b.query(c)
}

// complement 索引全集对 child 取补（Not 的求值）：全集 = attrs 镜像里
// 全部 uuid——Not 的语义是"在库内文件里找不命中的"，软删行本就不入索引。
func (m *memIndexer) complement(child map[string]struct{}) map[string]struct{} {
	out := make(map[string]struct{}, len(m.attrs))
	for uuid := range m.attrs {
		if _, ok := child[uuid]; !ok {
			out[uuid] = struct{}{}
		}
	}
	return out
}

// scanSet 扫描型 op 的镜像求集（调用方已持读锁）：
//   - exists：键存在性（Value=bool；true=有键，false=无键——"没有 X 的
//     文件"由此查，缺键信息只在镜像里可见）
//   - ne：有该键且值不同（缺键不算——语义上"无该键"≠"值不等于"；
//     归一化比较，数值族 int 3 ≠ float64 3 不成立）
//   - contains：字符串字段包含子串；数组字段任一元素包含即中
//     （Value 须为 string，解析层已校验）
func (m *memIndexer) scanSet(c Condition) map[string]struct{} {
	out := map[string]struct{}{}
	for uuid, a := range m.attrs {
		v, has := a[c.Field]
		switch c.Op {
		case OpExists:
			want, _ := c.Value.(bool)
			if has == want {
				out[uuid] = struct{}{}
			}
		case OpNe:
			if has && !sameValue(v, c.Value) {
				out[uuid] = struct{}{}
			}
		case OpContains:
			sub, _ := c.Value.(string)
			if has && containsValue(v, sub) {
				out[uuid] = struct{}{}
			}
		}
	}
	return out
}

// containsValue 值的子串判定：字符串 → 直接包含；数组 → 任一字符串元素
// 包含即中；其余类型（数值/bool）无子串语义，不命中。
func containsValue(v any, sub string) bool {
	switch x := v.(type) {
	case string:
		return strings.Contains(x, sub)
	case []any:
		for _, el := range x {
			if s, ok := el.(string); ok && strings.Contains(s, sub) {
				return true
			}
		}
	case []string:
		for _, s := range x {
			if strings.Contains(s, sub) {
				return true
			}
		}
	}
	return false
}

// intersect 交集：从最小集起步逐集过滤（一旦空集即早退）。
// 入参集合为桶内只读集或求值新建集，不改写。
func intersect(sets []map[string]struct{}) map[string]struct{} {
	if len(sets) == 0 {
		return map[string]struct{}{}
	}
	order := make([]int, len(sets))
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(a, b int) bool { return len(sets[order[a]]) < len(sets[order[b]]) })

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

// union 并集：全部集合并入。
func union(sets []map[string]struct{}) map[string]struct{} {
	out := map[string]struct{}{}
	for _, set := range sets {
		for uuid := range set {
			out[uuid] = struct{}{}
		}
	}
	return out
}

// Stats 运行计量：桶数 / 键·条目数 / 挂载点数。读锁遍历，纯自省。
// 排查"索引与 DB 不一致"时的第一抓手（空索引/超预期膨胀一眼可见）。
func (m *memIndexer) Stats() Stats {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s := Stats{Fields: len(m.buckets)}
	for _, b := range m.buckets {
		k, mo := b.stats()
		s.Keys += k
		s.Mounts += mo
	}
	return s
}

// Catalog 字段目录（查询方的发现接口）：按字段名升序，每字段给出
// 桶型/键数/挂载数 + 取值列表（enum·multi，超 CatalogValueLimit 截断
// 置 Truncated）或值域（num 的 min/max）。空索引返回空切片非 nil。
func (m *memIndexer) Catalog() []FieldInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]FieldInfo, 0, len(m.buckets))
	for f, b := range m.buckets {
		info := b.catalog()
		info.Field = f
		info.Keys, info.Mounts = b.stats()
		if len(info.Values) > CatalogValueLimit {
			info.Values = info.Values[:CatalogValueLimit]
			info.Truncated = true
		}
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Field < out[j].Field })
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
	// 镜像同步：new 为 nil/空 = 整体移除；否则整表覆写（镜像存原始值，
	// 含不可挂桶的——exists 的语义就是"键在不在"，与值是否可索引无关）
	if len(new) == 0 {
		delete(m.attrs, uuid)
	} else {
		m.attrs[uuid] = new
	}
}

// Rebuild 全量重建：清空全部桶后按 all 重新挂载（服务启动时从 DB 载入
// uuid→attributes）。个人库量级秒级；重建持写锁，期间查询方等待。
// 内存实现无失败路径，error 保留给将来外部化实现。
func (m *memIndexer) Rebuild(all map[string]map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.buckets = map[string]bucket{}
	m.attrs = map[string]map[string]any{}
	for uuid, attrs := range all {
		for f, v := range attrs {
			m.mount(f, uuid, v)
		}
		if len(attrs) > 0 {
			m.attrs[uuid] = attrs
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
