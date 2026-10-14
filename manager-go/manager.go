// 文件：manager-go/manager.go —— 管理机门面：文件位置与谱系的唯一知情者（架构设计.md 第 4 节）
// 修改：2026-10-14（日期由 fresh-header.ps1 刷新）

package manager

import "sync"

// IndexSink is retained for source compatibility; indexing is coordinated by core.
type IndexSink interface {
	Update(uuid string, old, new map[string]any)
}

// Manager 协调文件内容和元数据。通过 New 构造，首次使用后不可复制。
type Manager struct {
	intake     sync.RWMutex // 入库持读锁；清理执行持写锁等待在途发布。
	mutation   sync.Mutex   // 单实例修改与移动串行。
	store      Store
	dataDir    string // 文件系统根（updater 读盘 / placement 改名）
	sink       IndexSink
	metaUpdate func(fn func() error) error // 可选：逐文件 DB 元数据读改写与索引发布的串行临界区钩子
	extMime    func(path string) string    // 扩展名→MIME 推导（装配层注入；与 mime_type 顶层列同源，cod-basic-mime-match 的对比口径；nil = 不产该字段）
	buf        *fileBuffer                 // 取件缓冲区（fetch 域 Open/Read 的快速路径；纯派生态，可丢可清）
	download   DownloadConfig              // 下载票据域配置（Secret/BaseURL；装配层注入，空 = 下载未启用）
}

// SetMetaUpdateRunner 注入逐文件 DB 元数据读改写与索引发布的串行执行钩子（如编排机的 updateMu）。
// 注入后，T2/T3 的 GetMeta->MergeResults->UpsertMeta->sink.Update 与幽灵 SoftDeleteMeta->sink.Update
// 在同一临界区内执行。
func (m *Manager) SetMetaUpdateRunner(runner func(fn func() error) error) {
	m.metaUpdate = runner
}

func (m *Manager) runMetaUpdate(fn func() error) error {
	if m.metaUpdate != nil {
		return m.metaUpdate(fn)
	}
	return fn()
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
