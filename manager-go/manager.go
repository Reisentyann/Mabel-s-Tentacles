// 文件：manager-go/manager.go —— 管理机门面：文件位置与谱系的唯一知情者（架构设计.md 第 4 节）
// 修改：2026-09-03（日期由 fresh-header.ps1 刷新）

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

// Store manager 所需的最小存储面（依赖倒置，io.Reader 模式）：
// mcp-server-go 的 repo 实现满足签名后由装配层注入，manager 不 import
// 任何兄弟模块——同级模块只允许被上层 require，不允许反向依赖装配层。
// TODO（各域实现时扩充方法与配套 DTO，repo 侧做适配）。
type Store interface {
	// GetMetadataByUUID / UpsertMetadata / SoftDeleteMetadata / ListAllMetadata ...
}

// Manager 管理机。依赖按域逐步扩展（骨架期先钉存储面）。
type Manager struct {
	store Store
	// TODO（各域实现批次接入）：
	//   dataDir string          // placement/audit 的文件系统根
	//   idx     indexer.Indexer // updater 回填后喂食索引机（require indexer-go，同级 replace）
	//   analyzer 入口            // updater 调 describer.Analyze（require describer-go，同级 replace）
	//   secret  []byte          // download 票据签名密钥
}

// New 构造管理机。
func New(st Store) *Manager {
	return &Manager{store: st}
}
