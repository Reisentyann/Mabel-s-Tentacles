// 文件：manager-go/buffer.go —— 取件缓冲区：uuid → 内容快照的容量受限 LRU 缓存（fetch 域 Open/Read 的快速路径，纯派生态）
// 修改：2026-09-08（日期由 fresh-header.ps1 刷新）

// buffer 域职责（fetch 域的支撑机制，非独立域）：其他组件不摸盘（铁律），
// 取内容统一走管理机；buffer 让热文件命中免盘读——"安全且快速"的落点。
//
// 立场（钉死）：
//   - 安全：统一出入口 + 快照语义——put 时内容复制入缓，盘上后续改动
//     不影响已入缓条目；新鲜度由 stat 校验兜底（mtime/size 漂 → 条目
//     失效重读，execute_command 绕口由此收敛，最终一致）
//   - 快速：命中 O(1)；容量满按 LRU 逐出
//   - 派生态：缓冲随时可丢可清（重启 / 内存压力 / 手动清空），丢了
//     大不了盘上重读——容灾铁律 2 的口径，不做持久化
//   - 大文件旁路：size > maxEntry 的文件不入缓（Open 直流），防单条目
//     挤占整个容量；maxEntry 与引擎全量分析预算同口径（describer.MaxFullBytes）
//   - 不服务下载（2026-09-08 立场钉死）：HTTP 下载是大流量一次性直流，
//     入缓只添磁盘读写与 LRU 污染——下载端点不经 buffer/fetch，本缓存
//     只服务 agent 反复读小文件（read_file / 检索取件）的快速路径
//
// 并发：RWMutex——get 的 map 查走读锁、stat 对拍在锁外（IO 不占锁），
// put/drop 走写锁；get 返回共享底层数组的只读切片，调用方约定不改写。
package manager

import (
	"container/list"
	"os"
	"sync"
	"time"

	"github.com/Reisentyann/Mabel-s-Tentacles/describer-go"
)

// 取件缓冲区默认预算。
const (
	// defaultBufCapBytes 总容量 64MB（个人库量级宽裕；内存紧张可调小，
	// 派生态丢了无损）。
	defaultBufCapBytes = 64 << 20
	// defaultBufMaxEntry 单条目上限 5MB（= describer.MaxFullBytes 单一来源：
	// 值得缓存的分析粒度；超此尺寸的大文件直流不入缓）。
	defaultBufMaxEntry = describer.MaxFullBytes
)

// bufEntry 单个缓冲条目（put 时快照：path/size/modTime 记录入缓时刻的
// 盘上状态，get 时 stat 对拍——漂了即失效）。uuid 冗余存一份供 LRU
// 逐出时反查 map 键。
type bufEntry struct {
	uuid    string
	path    string // 绝对路径（stat 对拍用）
	size    int64
	modTime time.Time
	content []byte
}

// fileBuffer 取件缓冲区（uuid 键，LRU 逐出）。
type fileBuffer struct {
	mu       sync.RWMutex
	capBytes int64                    // 总容量（字节）
	maxEntry int64                    // 单条目上限（字节，超限拒入）
	entries  map[string]*list.Element // uuid → element（Value = *bufEntry）
	lru      *list.List               // Front = 最近使用，Back = 逐出候选
	used     int64                    // 当前内容字节占用
}

// newFileBuffer 构造缓冲区（容量与单条目上限由 New 用默认常量注入，
// 将来配置化走装配层参数）。
func newFileBuffer(capBytes, maxEntry int64) *fileBuffer {
	return &fileBuffer{
		capBytes: capBytes,
		maxEntry: maxEntry,
		entries:  map[string]*list.Element{},
		lru:      list.New(),
	}
}

// get 命中且新鲜 → (内容, true)；未命中 / stat 失败 / 新鲜度漂移
// （size/modTime 对拍不一致）→ 顺带 drop 后返回 (nil, false)。
// stat 在读锁外做（IO 不占锁）；条目引用与内容切片取出后即与锁解耦，
// 并发逐出也安全（Go 切片引用保活，快照语义不受影响）。
// 返回的是共享底层数组的只读切片，调用方约定不改写。
func (b *fileBuffer) get(uuid string) ([]byte, bool) {
	b.mu.RLock()
	el, ok := b.entries[uuid]
	if !ok {
		b.mu.RUnlock()
		return nil, false
	}
	e := el.Value.(*bufEntry)
	content, path, size, mod := e.content, e.path, e.size, e.modTime
	b.mu.RUnlock()

	info, err := os.Stat(path)
	if err != nil || info.Size() != size || !info.ModTime().Equal(mod) {
		b.drop(uuid) // 盘上状态不明 / 已漂移：条目失效（no-op 安全）
		return nil, false
	}

	b.mu.Lock() // 命中提升 LRU 位次（条目可能已被并发逐出，重查）
	if el, ok := b.entries[uuid]; ok {
		b.lru.MoveToFront(el)
	}
	b.mu.Unlock()
	return content, true
}

// put 入缓（调用方已持有内容，put 复制为快照——盘上后续改动不影响
// 已入缓条目）；size > maxEntry 拒入（大文件旁路，Open 直流）；
// 容量超限按 LRU 逐出最久未用条目；同 uuid 重复 put = 新快照覆盖旧值。
func (b *fileBuffer) put(uuid string, e bufEntry) {
	if e.size <= 0 || e.size > b.maxEntry || e.size > b.capBytes {
		return // 超限旁路：不入缓
	}
	snap := make([]byte, len(e.content))
	copy(snap, e.content)
	e.content = snap
	e.uuid = uuid

	b.mu.Lock()
	defer b.mu.Unlock()
	if old, ok := b.entries[uuid]; ok { // 覆盖：先归还旧占用
		b.used -= old.Value.(*bufEntry).size
		b.lru.Remove(old)
	}
	for b.used+e.size > b.capBytes && b.lru.Len() > 0 {
		back := b.lru.Back()
		v := back.Value.(*bufEntry)
		b.used -= v.size
		delete(b.entries, v.uuid)
		b.lru.Remove(back)
	}
	if b.used+e.size > b.capBytes {
		return // 防御：逐空仍装不下（size<=capBytes 已保证不至此）
	}
	b.entries[uuid] = b.lru.PushFront(&e)
	b.used += e.size
}

// drop 单条目失效（新鲜度漂移 / 文件删除路径）；不存在为 no-op。
func (b *fileBuffer) drop(uuid string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if el, ok := b.entries[uuid]; ok {
		b.used -= el.Value.(*bufEntry).size
		b.lru.Remove(el)
		delete(b.entries, uuid)
	}
}

// stats 自省计量（条目数 / 占用字节）——运维排障抓手，口径对齐索引机 Stats。
func (b *fileBuffer) stats() (entries int, bytes int64) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.lru.Len(), b.used
}

// BufferStats 取件缓冲区自省（条目数 / 占用字节）——运维排障抓手，
// 口径对齐索引机 Stats（派生态：数值仅作参考，丢了无损）。
func (m *Manager) BufferStats() (entries int, bytes int64) {
	return m.buf.stats()
}
