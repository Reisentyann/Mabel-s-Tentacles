// 文件：indexer-go/indexer.go —— 索引机接口与类型：字段条件 → uuid 纯查询 + Stats 自省（架构设计.md 第 3 节）
// 修改：2026-09-05（日期由 fresh-header.ps1 刷新）

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
	OpEq    Op = "eq"    // 等值（枚举/bool/string）
	OpIn    Op = "in"    // 值集合任一命中（Value 为 []any）
	OpGt    Op = "gt"    // 数值大于
	OpLt    Op = "lt"    // 数值小于
	OpRange Op = "range" // 数值闭区间，Value 为 [2]any{lo, hi}
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
}
