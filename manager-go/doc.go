// 文件：manager-go/doc.go —— 包职责、依赖边界与文件生命周期约定
// 修改：2026-10-08（日期由 fresh-header.ps1 刷新）

// Package manager 管理文件内容、UUID 位置、逻辑目录和文件谱系。
// 调用方负责授权与事件投递；Manager 通过 Store 访问元数据，
// 通过 describer 分析内容，不依赖 HTTP、MCP 或数据库实现。
//
// 文件以 UUID 标识，物理路径由 UUID 和逻辑键的扩展名派生。
// 目录中的显示名称独立于逻辑键，允许同名文件。
// 写入先占位再发布内容；改变扩展名的移动通过持久化记录恢复。
// 启动时应先调用 RecoverPendingMoves，再调用 RecoverPendingIntakes，
// 完成后再接收请求和启动扫描。
//
// Manager 的修改、移动和打开阶段在单实例内串行协调。
// Manager 在首次使用后不可复制；此协调不提供跨实例事务保证。
// Open 返回的内容流由调用方关闭，Read 返回独立的内容字节。
package manager
