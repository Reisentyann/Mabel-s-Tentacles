// 文件：mcp-server-go/core/orchestrator.go —— 编排机门面：生命周期事件 + 异步队列 + worker 池（三机之上的统一编排层骨架）
// 修改：2026-09-05（日期由 fresh-header.ps1 刷新）

// Package core 是编排机：把描述机（describer-go，字节进事实出）/ 索引机
// （indexer-go，条件→uuid）/ 管理机（manager-go，位置与谱系）的编排，
// 从散落在各 MCP/HTTP 入口的各自手搓，收拢成唯一管线。三机职责与
// docs/架构设计.md 的组件约定不变——本包只管流程：事件进来，事实落库，
// 索引喂饱，调用方拿回执。
//
// 容灾三铁律（2026-09-05 定稿）：
//  1. 源文件不能丢——盘写由调用方同步完成并硬失败；编排失败永不碰盘
//  2. 描述可丢可重建——事件可丢（队列溢出/进程崩溃），启动 RebuildIndex
//     + 管理机 T2 对账轮次兜底，最终一致（DB 是事实源，索引与描述皆派生）
//  3. 降级不阻塞——队列满丢弃告警、索引故障降级 SQL、sink 喂食失败只记日志
//
// 异步口径：写路径盘写成功即返回，描述→落库→喂索引入队后跑；事件不携带
// 文件内容，worker 处理时盘上重读——后写胜出（同路径连续写只对终态负责），
// 队列零内容滞留。
//
// 布局：orchestrator.go 门面与队列 / executor.go 统一执行器 /
// describe.go 同步描述入口 / search.go 检索门面与索引重建。
//
// 现状为骨架（2026-09-05），细节按批次落地（接线点各文件 TODO 标注）：
//   - tools/main/api 接线：Deps 增 Orch，write/modify/copy 切 Submit
//   - write_file 双 upsert 合一（Agent 顺带字段并入执行器单次 Upsert）
//   - copy 主路径喂食：repo.CopyMetadata 补返回 uuid 后由编排机直接喂
//   - describe_file / HTTP describe 切 Describe（消灭两处复制粘贴）
//   - 检索索引化：uuid 批量取件 + 降级链（见 search.go）
//   - T2/T3 与 manager-go 共享执行器核心（manager 另线维护，本包不动它）
package core

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Reisentyann/Mabel-s-Tentacles/indexer-go"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/repo"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/search"
)

// Sink 索引喂食最小面：indexer-go 的 Indexer.Update 与 manager-go 的
// IndexSink 结构同构——装配层把 indexer 实例直接注入即可，本包不绑定
// 兄弟模块的具体类型。nil = 索引未装配，喂食跳过。
type Sink interface {
	Update(uuid string, old, new map[string]any)
}

// IndexSource 索引查询最小面（indexer-go Indexer 的结构子集）：
// 检索门面（search.go）与启动重建（RebuildIndex）用。nil = 检索降级 SQL。
type IndexSource interface {
	Query(conds []indexer.Condition, mode indexer.Combine) ([]string, error)
	Rebuild(all map[string]map[string]any) error
}

// Store 编排机所需的最小存储面（repo.Store 的结构子集，pgx 实现直接注入；
// 与 repo 同模块，直接复用其 DTO，不另造影子结构）。
type Store interface {
	GetMetadata(ctx context.Context, filePath string) (*repo.FileMetadata, error)
	UpsertMetadata(ctx context.Context, m *repo.FileMetadata) (uuid string, err error)
	ListMetadataPage(ctx context.Context, sincePath string, limit int) ([]repo.FileMetadata, error)
}

// Kind 生命周期事件种类。提交即表示盘上内容已是终态（调用方先完成盘写）。
type Kind string

const (
	KindWrite   Kind = "write"   // 创建/覆写
	KindModify  Kind = "modify"  // 追加/覆写修改
	KindCopy    Kind = "copy"    // 复制完成（目标路径视角）
	KindAnalyze Kind = "analyze" // 显式重分析（预留：T3 接线批次）
)

// Event 生命周期事件。不携带文件内容——worker 处理时盘上重读（后写胜出，
// 零内容滞留）；Agent 为 agent 随写顺带提供的顶层描述字段，与执行器产出
// 合并为单次 Upsert（消灭 write_file 场景的第二次 upsert）。
type Event struct {
	Kind      Kind
	Path      string
	Agent     *AgentMeta
	SessionID string
}

// AgentMeta agent 顶层描述字段（COALESCE 语义：nil = 不覆盖既有值；
// Tags 非 nil（含空切片）= 覆盖）。
type AgentMeta struct {
	Title       *string
	Description *string
	Tags        []string
	FileType    *string
}

// Options 编排机构造参数。
type Options struct {
	DataDir  string          // 文件系统根（必填：resolve + 盘上重读）
	Store    Store           // 最小存储面（必填）
	Sink     Sink            // 索引喂食（nil = 跳过）
	Index    IndexSource     // 索引查询（nil = 检索直接降级 SQL）
	Fallback search.Searcher // SQL 兜底检索（Search 的降级终点；nil = Search 报错）
	QueueCap int             // 事件队列容量（默认 256；满 = 丢弃 + WARN）
	Workers  int             // worker 数（默认 1；盘上重读 + 后写胜出，多 worker 亦安全）
}

// Orchestrator 编排机。
type Orchestrator struct {
	opts   Options
	queue  chan Event
	mu     sync.RWMutex // 保护 closed：Submit 持读锁非阻塞发送 / Stop 持写锁关闭，杜绝向已闭通道发送
	closed bool
	wg     sync.WaitGroup
	startO sync.Once
	stopO  sync.Once

	submitted atomic.Int64
	processed atomic.Int64
	dropped   atomic.Int64
}

// New 构造编排机（校验必填项）。Start 前 Submit 会缓冲在队列里，
// Start 后由 worker 消费。
func New(opts Options) (*Orchestrator, error) {
	if opts.DataDir == "" {
		return nil, fmt.Errorf("core: DataDir 必填")
	}
	if opts.Store == nil {
		return nil, fmt.Errorf("core: Store 必填")
	}
	if opts.QueueCap <= 0 {
		opts.QueueCap = 256
	}
	if opts.Workers <= 0 {
		opts.Workers = 1
	}
	return &Orchestrator{opts: opts, queue: make(chan Event, opts.QueueCap)}, nil
}

// Start 起 worker 池（幂等）。ctx 取消只中断单次执行中的慢操作（DB 调用），
// 队列排空与 worker 退出由 Stop 负责。
func (o *Orchestrator) Start(ctx context.Context) {
	o.startO.Do(func() {
		o.wg.Add(o.opts.Workers)
		for i := 0; i < o.opts.Workers; i++ {
			go o.work(ctx)
		}
	})
}

// Stop 关停（幂等）：封队列（此后 Submit 返回 false）→ worker 排空存量
// 事件 → 等全部退出。优雅收尾尽力而为；排空期间的失败按容灾立场丢弃。
func (o *Orchestrator) Stop() {
	o.stopO.Do(func() {
		o.mu.Lock()
		o.closed = true
		close(o.queue)
		o.mu.Unlock()
		o.wg.Wait()
	})
}

// Submit 提交生命周期事件（非阻塞）。false = 队列满被丢弃或已关停——
// 容灾立场：事件可丢，管理机 T2 对账轮次兜底重建，调用方无需补偿。
func (o *Orchestrator) Submit(ev Event) bool {
	o.mu.RLock()
	defer o.mu.RUnlock()
	if o.closed {
		return false
	}
	select {
	case o.queue <- ev:
		o.submitted.Add(1)
		return true
	default:
		o.dropped.Add(1)
		slog.Warn("orchestrate queue full, event dropped",
			"kind", ev.Kind, "path", ev.Path, "session", ev.SessionID)
		return false
	}
}

// Stats 运行计量（自省）。
type Stats struct {
	Submitted int64 `json:"submitted"` // 累计入队
	Processed int64 `json:"processed"` // 累计处理（含失败——失败也算处理完）
	Dropped   int64 `json:"dropped"`   // 累计丢弃（队列满 / 已关停）
	InFlight  int64 `json:"in_flight"` // 当前排队数 = Submitted - Processed - Dropped
}

// Stats 返回运行计量。
func (o *Orchestrator) Stats() Stats {
	s := Stats{
		Submitted: o.submitted.Load(),
		Processed: o.processed.Load(),
		Dropped:   o.dropped.Load(),
	}
	s.InFlight = s.Submitted - s.Processed - s.Dropped
	return s
}

// work 单 worker 主循环：排空式消费（Stop 关队列后处理完存量即退出）。
// 失败只记日志不重试——事件可丢立场（铁律 2），T2 对账兜底。
func (o *Orchestrator) work(ctx context.Context) {
	defer o.wg.Done()
	for ev := range o.queue {
		start := time.Now()
		report, err := o.execute(ctx, ev)
		if err != nil {
			slog.Warn("orchestrate event failed",
				"kind", ev.Kind, "path", ev.Path, "session", ev.SessionID,
				"error", err, "duration", time.Since(start).String())
			o.processed.Add(1)
			continue
		}
		slog.Info("file lifecycle recorded",
			"kind", ev.Kind, "path", ev.Path, "session", ev.SessionID,
			"uuid", report.UUID, "families", report.Families,
			"cod_keys", report.CodKeys, "size", report.Size,
			"duration", time.Since(start).String())
		o.processed.Add(1)
	}
}
