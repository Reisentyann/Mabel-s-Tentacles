// 文件：mcp-server-go/core/executor.go —— 统一执行器：盘上读 → describer.Analyze → 读旧合并 → 顶层列推导 → 单次 Upsert → 喂索引
// 修改：2026-09-08（日期由 fresh-header.ps1 刷新）

package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Reisentyann/Mabel-s-Tentacles/common"
	"github.com/Reisentyann/Mabel-s-Tentacles/describer-go"
	_ "github.com/Reisentyann/Mabel-s-Tentacles/describer-go/all" // 插件聚合注册（编排机自带；与 tools 侧重复 blank import 幂等无害）
	"github.com/Reisentyann/Mabel-s-Tentacles/manager-go"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/repo"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/service"
)

// Report 单次执行产出（日志计量与调用方回执复用）。
type Report struct {
	UUID       string   `json:"uuid"`
	Families   []string `json:"families"` // 本次命中插件家族（注册序）
	CodKeys    int      `json:"cod_keys"` // 本次新产出 cod/sp-cod 键数
	Size       int64    `json:"size"`
	Owner      string   `json:"owner"`      // 归属落库口径（权限批次审计）
	Visibility string   `json:"visibility"` // 本次落库的可见性
}

// execute 统一执行器——"描述→落库→喂索引"管线的全仓唯一实现
// （原 tools.RecordFileMeta 的 T1 场景收敛点；manager-go/updater.go 的
// analyze 暂为并行的盘读版，manager 另线维护不动它，T2/T3 接线批次再议共享；
// 盘读三件套 readHead/readLimited/checksumFile 已先行收敛 common）：
//
//	resolve → stat → head 512B + 惰性全量（5MB）→ 流式 checksum
//	→ describer.Analyze → 读旧 attrs → MergeResults（保留 llm-*）
//	→ 顶层列推导 + Agent 顺带字段 → 单次 Upsert（返回 uuid）→ Sink.Update
//
// 幂等：家族整族替换保证，重复执行结果恒等。
// 读旧失败必须失败整个事件——Upsert 的 attributes 是整列覆盖，拿不到旧值
// 就落库会把 llm-* 既有事实抹掉（比丢一次事件严重得多）。
// 文件已删除等竞态：stat 失败返回错误，调用方按容灾立场丢弃
// （管理机 T2 幽灵计数轮次兜底）。
func (o *Orchestrator) execute(ctx context.Context, ev Event) (*Report, error) {
	// KindMove 早退（文件管理域 2026-09-08）：键已由管理机 Move 改好——
	// uuid / 内容 / 描述 / 归属全不变，索引键（uuid）不动，统一描述管线
	// 零触发（intake 域设计红利：Move = 纯 DB 键改）。事件仅记账
	//（lifecycle 日志归 orchestrator 统一记录，谱系 moved_from 在行内）。
	if ev.Kind == KindMove {
		return &Report{UUID: "", Visibility: ev.Visibility}, nil
	}
	// 占位行先行（write/copy 工具层经管理机 Reserve）：行必有 uuid——
	// 物理路径由 uuid 派生（intake 域口径，agent 只见逻辑键）。
	// 旧 attrs 一并前置读取（读-改-写的旧值侧）。
	meta, gerr := o.opts.Store.GetMetadata(ctx, ev.Path)
	if gerr != nil && !errors.Is(gerr, pgx.ErrNoRows) {
		return nil, fmt.Errorf("get metadata: %w", gerr)
	}
	if meta == nil {
		return nil, fmt.Errorf("no meta row for %q (intake first)", ev.Path)
	}
	old := describer.AttrsFromJSON(meta.Attributes)

	storageRel, err := manager.StoragePathOf(meta.UUID, ev.Path)
	if err != nil {
		return nil, err
	}
	abs, err := service.ResolvePath(o.opts.DataDir, storageRel)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("stat: %w", err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("'%s' is a directory", ev.Path)
	}
	head, err := common.ReadHead(abs, describer.MaxHeadBytes)
	if err != nil {
		return nil, fmt.Errorf("read head: %w", err)
	}
	cs, err := common.ChecksumFile(abs)
	if err != nil {
		return nil, fmt.Errorf("checksum: %w", err)
	}

	fileType, mimeType := service.InferFileMeta(ev.Path)
	results := describer.Analyze(describer.Input{
		Path:    ev.Path,
		Head:    head,
		Size:    info.Size(),
		MTime:   info.ModTime(),
		ExtMime: mimeType,
	}, func() ([]byte, error) {
		return common.ReadLimited(abs, describer.MaxFullBytes)
	})

	// 读-改-写：整族合并 cod-*，保留 llm-* / sp-llm-*（旧值已前置读取）
	merged := describer.MergeResults(old, results, time.Now())

	// 顶层列 + agent 顺带字段 → 单次 Upsert（消灭 write_file 的双 upsert）。
	// 归属打标（权限批次 2026-09-06）：仅创建语义（KindWrite）落 owner——
	// modify/copy 事件不动归属（COALESCE 保留原 owner；copy 的归属由
	// CopyMetadata 落操作者）。可见性：写者指定优先；新文件缺省 public
	//（两级模型 2026-09-08：共享为默认，私密是显式选择）。
	size := info.Size()
	visibility := ev.Visibility
	if visibility == "" && ev.Kind == KindWrite {
		visibility = "public"
	}
	upsert := &repo.FileMetadata{
		FilePath:   ev.Path,
		Scope:      service.InferScope(ev.Path),
		FileType:   &fileType,
		MimeType:   &mimeType,
		Extension:  common.StrPtr(service.InferExtension(ev.Path)),
		SizeBytes:  &size,
		Checksum:   &cs,
		SessionID:  common.StrPtr(ev.SessionID),
		Visibility: visibility,
		Attributes: describer.JSONFromAttrs(merged),
	}
	if ev.Kind == KindWrite && ev.Actor.Name != "" {
		owner := ev.Actor.Name
		upsert.OwnerID = &owner
	}
	if ev.Agent != nil {
		if ev.Agent.Title != nil {
			upsert.Title = ev.Agent.Title
		}
		if ev.Agent.Description != nil {
			upsert.Description = ev.Agent.Description
		}
		if ev.Agent.Tags != nil {
			upsert.Tags = ev.Agent.Tags
		}
		if ev.Agent.FileType != nil {
			upsert.FileType = ev.Agent.FileType
		}
	}
	uuid, err := o.opts.Store.UpsertMetadata(ctx, upsert)
	if err != nil {
		return nil, fmt.Errorf("upsert metadata: %w", err)
	}
	if o.opts.Sink != nil {
		o.opts.Sink.Update(uuid, old, merged)
	}

	report := &Report{UUID: uuid, Families: make([]string, 0, len(results)), Size: size,
		Visibility: visibility}
	if meta.OwnerID != nil {
		report.Owner = *meta.OwnerID // 本次落归属（KindWrite）；modify/copy 不动 owner，留空
	}
	for _, r := range results {
		report.Families = append(report.Families, r.Family)
		report.CodKeys += len(r.Attrs)
	}
	return report, nil
}
