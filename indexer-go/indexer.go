// 文件：indexer-go/indexer.go —— 索引机接口与类型：字段条件 → uuid 纯查询 + Stats 自省（架构设计.md 第 3 节）
// 修改：2026-09-11（日期由 fresh-header.ps1 刷新）

// Package indexer 是字段索引机：纯查询服务——输入字段条件，输出文件 uuid 集合
// （docs/架构设计.md 第 3 节）。uuid 是组件间货币，管理机凭 uuid 取件。
//
// 铁律：本包是 sink——零出调用（不调 repo、不调 manager、不通知任何人）；
// 只被喂食（Update）与查询（Query）。写路径（T1/describe_file/T2）在 Upsert 后
// 由编排方调 Update 做 diff 增量；execute_command 直改文件的绕口由管理机
// updater 的对账兜底（IsStale checksum 漂移 → 重分析 → 喂食），本包不管。
//
// 布局（≈ stdlib hash 的接口/实现分离范式）：
//
//	indexer.go 接口与类型 / mem.go 进程内内存实现 / bucket.go 三型桶数据结构
package indexer

// Op 单字段条件的比较运算。
type Op string

const (
	OpEq       Op = "eq"       // 等值（枚举/bool/string；multi=数组含该元素）
	OpIn       Op = "in"       // 值集合任一命中（Value 为 []any）
	OpGt       Op = "gt"       // 数值大于
	OpLt       Op = "lt"       // 数值小于
	OpRange    Op = "range"    // 数值闭区间，Value 为 [2]any{lo, hi}
	OpNe       Op = "ne"       // 不等于——只圈「有该键且值不同」的行；缺键行用 exists=false
	OpExists   Op = "exists"   // 键存在性（Value 为 bool）：分母为 0 不产键的字段（如 EXIF）由此查「没有 X」
	OpContains Op = "contains" // 子串（Value 为 string）：字符串字段包含；数组字段任一元素包含即中
)

// Condition 单字段条件。Field 为完整键名（cod-* / llm-* / sp-*）。
type Condition struct {
	Field string
	Op    Op
	Value any
}

// Combine 多条件的组合方式。
type Combine int

const (
	And Combine = iota // 全部命中（交集）
	Or                 // 任一命中（并集）
)

// Stats 索引机运行计量（自省接口）：排查"索引与 DB 不一致"时的抓手。
type Stats struct {
	Fields int `json:"fields"` // 桶数（被索引的字段数）
	Keys   int `json:"keys"`   // 全部桶的键/条目总数（enum·multi 的值键数 + num 的条目数）
	Mounts int `json:"mounts"` // 挂载点总数（uuid×值 关联数）
}

// FieldKind 桶型（字段目录用）：决定该字段支持哪些 Op。
type FieldKind string

const (
	KindEnum  FieldKind = "enum"  // 标量等值桶：可查 eq / in / ne / contains / exists
	KindNum   FieldKind = "num"   // 数值桶：可查 eq / in / gt / lt / range / ne / exists
	KindMulti FieldKind = "multi" // 数组多值桶（任一元素命中即整文件命中）：eq / in / ne / contains（元素级）/ exists
)

// CatalogValueLimit 目录单字段的取值列表上限：超过截断并置 Truncated
// （agent 上下文保护——目录是"发现"用的，不是全量导出用的）。
const CatalogValueLimit = 50

// FieldInfo 单字段目录项：告诉查询方"这个字段是什么型、能怎么查、
// 现在有哪些可查的值/值域"。字段目录是 agent 的发现接口——
// 一次 Catalog 调用 = 拿到全部可查字段与取值，随后即可精准 Query。
type FieldInfo struct {
	Field     string    `json:"field"`               // 完整键名（cod-* / llm-* / sp-*）
	Kind      FieldKind `json:"kind"`                // 桶型（决定支持的 Op 集）
	Keys      int       `json:"keys"`                // 键/条目数
	Mounts    int       `json:"mounts"`              // 挂载点数（值×uuid 关联数）
	Values    []string  `json:"values,omitempty"`    // enum/multi：当前全部取值（截断见 Truncated）
	Truncated bool      `json:"truncated,omitempty"` // 取值超上限被截断
	Min       *float64  `json:"min,omitempty"`       // num：值域下界（空桶为 nil）
	Max       *float64  `json:"max,omitempty"`       // num：值域上界
}

// Indexer 索引机接口。进程内内存实现见 mem.go；
// 将来若需多实例外部化，换实现不动调用方。
type Indexer interface {
	// Query 按条件求 uuid 集合。空条件返回空集（不报错）。
	Query(conds []Condition, mode Combine) ([]string, error)
	// Update 写路径喂食：old/new 为该文件变更前后的 attributes，
	// 实现负责逐字段 diff（旧桶移除、新桶挂入）。幂等：重复喂食结果恒等。
	// new 为 nil/空 = 整体移除（文件删除路径）。
	Update(uuid string, old, new map[string]any)
	// Rebuild 全量重建（服务启动时从 DB 载入 uuid→attributes）。
	Rebuild(all map[string]map[string]any) error
	// Stats 返回运行计量（fields/keys/mounts）。纯读操作，运维排障抓手。
	Stats() Stats
	// Catalog 返回字段目录（按字段名升序）：每字段的桶型/键数/挂载数/
	// 取值列表（enum·multi，截断保护）或值域（num min·max）。
	// 查询方的发现接口——先 Catalog 拿目录，再 Query 圈文件。
	Catalog() []FieldInfo
}
