// 文件：manager-go/manager.go —— 管理机门面：文件位置与谱系的唯一知情者（架构设计.md 第 4 节）
// 修改：2026-09-05（日期由 fresh-header.ps1 刷新）

// Package manager 是文件生命周期的编排层与信息权威（docs/架构设计.md 第 4 节）：
// 文件在哪（位置）、文件之间的关系（谱系）只有它知道，其他组件一律问它，
// 不摸文件系统、不查谱系表。
//
// 铁律：
//   - DB 交互归 repo——manager 编排 repo（调用），不拥有（不写 SQL）
//   - 自动只巡检报告，动手必须显式（move_file）；路径是 agent 的组织语言+元数据唯一键
//   - 编排循环发生在时间轮次（timer/启动驱动），不发生在调用栈里——绝不自旋
//
// 布局（≈ stdlib database/sql 的门面 + 按域平铺范式）：
//
//	manager.go 门面与构造 / placement.go 位置域 / lineage.go 谱系域 /
//	updater.go 更新回填域（T2/T3）/ download.go 下载票据域 / audit.go 对账域
package manager

import (
	"context"
	"encoding/json"
)

// MetaRow updater 域的元数据读视图（manager 自己的 DTO，repo 侧写适配——
// 依赖倒置：不 import repo 的 FileMetadata，交接文档 4.3 DTO 归属）。
type MetaRow struct {
	Path       string          // 文件相对路径（元数据唯一键）
	Checksum   string          // 顶层列 checksum（空 = 无 / 未算）
	Attributes json.RawMessage // cod / llm / sp 全量属性
}

// MetaRecord updater 域的元数据写视图：T2/T3 重分析后的落库载荷。
// file_type / mime_type / extension / scope 等顶层列的推导归装配层适配器。
type MetaRecord struct {
	Path       string
	SizeBytes  int64
	Checksum   string
	Attributes json.RawMessage
}

// Store manager 所需的最小存储面（依赖倒置，io.Reader 模式）：updater 域批次。
// mcp-server-go 的 repo 实现满足签名后由装配层注入（repo/manager_adapter.go），
// manager 不 import 任何兄弟模块——同级模块只允许被上层 require，
// 不允许反向依赖装配层（依赖方向铁律：HTTP/MCP → manager → repo + describer + indexer）。
// TODO（placement/audit/download/lineage 域实现时扩充方法与配套 DTO）：
// GetMetadataByUUID / 元数据全量列接口 / lineage 表 / 幽灵计数批量接口 …
type Store interface {
	// ListMetaPage 按 Path 升序的游标分页：sincePath 之后（不含）limit 条，
	// 不含软删。T2 回填的扫描入口，可中断续跑。
	ListMetaPage(ctx context.Context, sincePath string, limit int) ([]MetaRow, error)
	// GetMeta 读单文件元数据（读-改-写的旧值侧）；无行返回 (nil, nil)。
	GetMeta(ctx context.Context, path string) (*MetaRow, error)
	// UpsertMeta 写回重分析结果，返回该行 uuid（组件间货币，索引挂载键）。
	UpsertMeta(ctx context.Context, rec MetaRecord) (string, error)
	// MarkMissing 盘上缺失计数 +1，返回累计轮次（连续 3 轮触发软删除，
	// 字段字典 10.4；文件重新出现走 UpsertMeta 时清零）。
	MarkMissing(ctx context.Context, path string) (int, error)
	// SoftDeleteMeta 软删除（打标记不物理删，元数据可追溯）。
	SoftDeleteMeta(ctx context.Context, path string) error
}

// IndexSink 索引喂食钩子：写路径 Upsert 后把 attributes 的 old/new diff
// 喂给索引机（架构设计.md 第 3 节喂食点）。与 indexer-go Indexer 的
// Update 方法同构——结构化类型匹配，装配层把 indexer 实例直接注入
// 即可（索引机批次接线；nil = 喂食跳过）。
type IndexSink interface {
	Update(uuid string, old, new map[string]any)
}

// Manager 管理机。依赖按域逐步扩展。
type Manager struct {
	store   Store
	dataDir string                   // 文件系统根（updater 读盘 / placement 改名）
	sink    IndexSink                // 索引喂食钩子（可空：索引机批次前为 nil）
	extMime func(path string) string // 扩展名→MIME 推导（装配层注入；与 mime_type 顶层列同源，cod-basic-mime-match 的对比口径；nil = 不产该字段）
}

// New 构造管理机。dataDir 为 data 目录根；sink 与 extMime 允许 nil
// （sink=nil 索引不喂食；extMime=nil 时 cod-basic-mime-match 不产出）。
func New(st Store, dataDir string, sink IndexSink, extMime func(path string) string) *Manager {
	return &Manager{store: st, dataDir: dataDir, sink: sink, extMime: extMime}
}
