// 文件：manager-go/manager.go —— 管理机门面：文件位置与谱系的唯一知情者（架构设计.md 第 4 节）
// 修改：2026-10-08（日期由 fresh-header.ps1 刷新）

package manager

import "sync"

// IndexSink 索引喂食钩子：写路径 Upsert 后把 attributes 的 old/new diff
// 喂给索引机（架构设计.md 第 3 节喂食点）。与 indexer-go Indexer 的
// Update 方法同构——结构化类型匹配，装配层把 indexer 实例直接注入
// 即可（索引机批次接线；nil = 喂食跳过）。
type IndexSink interface {
	Update(uuid string, old, new map[string]any)
}

// Manager 协调文件内容和元数据。通过 New 构造，首次使用后不可复制。
type Manager struct {
	intake   sync.RWMutex // 入库持读锁；清理执行持写锁等待在途发布。
	mutation sync.Mutex   // 单实例修改与移动串行。
	store    Store
	dataDir  string                   // 文件系统根（updater 读盘 / placement 改名）
	sink     IndexSink                // 索引喂食钩子（可空：索引机批次前为 nil）
	extMime  func(path string) string // 扩展名→MIME 推导（装配层注入；与 mime_type 顶层列同源，cod-basic-mime-match 的对比口径；nil = 不产该字段）
	buf      *fileBuffer              // 取件缓冲区（fetch 域 Open/Read 的快速路径；纯派生态，可丢可清）
	download DownloadConfig           // 下载票据域配置（Secret/BaseURL；装配层注入，空 = 下载未启用）
}

// New 构造管理机。dataDir 为 data 目录根；sink 与 extMime 允许 nil
// （sink=nil 索引不喂食；extMime=nil 时 cod-basic-mime-match 不产出）。
// dl 为下载票据配置（Secret/BaseURL 空 = 下载地址不产出）。
// 取件缓冲区按默认预算构造（64MB / 单条目 5MB，见 buffer.go）。
func New(st Store, dataDir string, sink IndexSink, extMime func(path string) string, dl DownloadConfig) *Manager {
	return &Manager{
		store:    st,
		dataDir:  dataDir,
		sink:     sink,
		extMime:  extMime,
		buf:      newFileBuffer(defaultBufCapBytes, defaultBufMaxEntry),
		download: dl,
	}
}
