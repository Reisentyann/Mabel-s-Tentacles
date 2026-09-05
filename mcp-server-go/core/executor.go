// 文件：mcp-server-go/core/executor.go —— 统一执行器：盘上读 → describer.Analyze → 读旧合并 → 顶层列推导 → 单次 Upsert → 喂索引
// 修改：2026-09-05（日期由 fresh-header.ps1 刷新）

package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Reisentyann/Mabel-s-Tentacles/describer-go"
	_ "github.com/Reisentyann/Mabel-s-Tentacles/describer-go/all" // 插件聚合注册（编排机自带；与 tools 侧重复 blank import 幂等无害）
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/repo"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/service"
)

// Report 单次执行产出（日志计量与调用方回执复用）。
type Report struct {
	UUID     string   `json:"uuid"`
	Families []string `json:"families"` // 本次命中插件家族（注册序）
	CodKeys  int      `json:"cod_keys"` // 本次新产出 cod/sp-cod 键数
	Size     int64    `json:"size"`
}

// execute 统一执行器——"描述→落库→喂索引"管线的全仓唯一实现
// （原 tools.RecordFileMeta 的 T1 场景收敛点；manager-go/updater.go 的
// analyze 暂为并行的盘读版，manager 另线维护不动它，T2/T3 接线批次再议共享）：
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
	abs, err := service.ResolvePath(o.opts.DataDir, ev.Path)
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
	head, err := readHead(abs)
	if err != nil {
		return nil, fmt.Errorf("read head: %w", err)
	}
	cs, err := checksumFile(abs)
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
		return readLimited(abs, describer.MaxFullBytes)
	})

	// 读-改-写：整族合并 cod-*，保留 llm-* / sp-llm-*（无行 = 空开始）
	var old map[string]any
	if m, gerr := o.opts.Store.GetMetadata(ctx, ev.Path); gerr == nil && m != nil {
		old = describer.AttrsFromJSON(m.Attributes)
	} else if gerr != nil && !errors.Is(gerr, pgx.ErrNoRows) {
		return nil, fmt.Errorf("get metadata: %w", gerr)
	}
	merged := describer.MergeResults(old, results, time.Now())

	// 顶层列 + agent 顺带字段 → 单次 Upsert（消灭 write_file 的双 upsert）
	size := info.Size()
	meta := &repo.FileMetadata{
		FilePath:   ev.Path,
		Scope:      service.InferScope(ev.Path),
		FileType:   &fileType,
		MimeType:   &mimeType,
		Extension:  strPtr(service.InferExtension(ev.Path)),
		SizeBytes:  &size,
		Checksum:   &cs,
		SessionID:  strPtr(ev.SessionID),
		Attributes: describer.JSONFromAttrs(merged),
	}
	if ev.Agent != nil {
		if ev.Agent.Title != nil {
			meta.Title = ev.Agent.Title
		}
		if ev.Agent.Description != nil {
			meta.Description = ev.Agent.Description
		}
		if ev.Agent.Tags != nil {
			meta.Tags = ev.Agent.Tags
		}
		if ev.Agent.FileType != nil {
			meta.FileType = ev.Agent.FileType
		}
	}
	uuid, err := o.opts.Store.UpsertMetadata(ctx, meta)
	if err != nil {
		return nil, fmt.Errorf("upsert metadata: %w", err)
	}
	if o.opts.Sink != nil {
		o.opts.Sink.Update(uuid, old, merged)
	}

	report := &Report{UUID: uuid, Families: make([]string, 0, len(results)), Size: size}
	for _, r := range results {
		report.Families = append(report.Families, r.Family)
		report.CodKeys += len(r.Attrs)
	}
	return report, nil
}

// strPtr 空串→nil（COALESCE 语义：nil 不覆盖既有值）。tools.StrPtr 同义，
// 但本包不得 import tools（tools 接线后将依赖本包，反向 import 成环）。
func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// —— 盘读助手（与 manager-go/updater.go 同款；对侧 unexported 无法复用，
// 暂为受控重复，T2/T3 共享批次统一）——

// readHead 读文件前 512B（describer.MaxHeadBytes 口径；短文件按实际长度）。
func readHead(abs string) ([]byte, error) {
	f, err := os.Open(abs)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	buf := make([]byte, describer.MaxHeadBytes)
	n, err := io.ReadFull(f, buf)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return nil, err
	}
	return buf[:n], nil
}

// readLimited 读文件前 limit 字节（Loader 全量预算；引擎内部仍会再截，双保险）。
func readLimited(abs string, limit int64) ([]byte, error) {
	f, err := os.Open(abs)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, limit))
}

// checksumFile 全文件流式 SHA-256（checksum 是完整性事实，必须覆盖全文件，
// 不受 5MB 分析预算影响）。
func checksumFile(abs string) (string, error) {
	f, err := os.Open(abs)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
