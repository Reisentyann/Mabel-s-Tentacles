// 文件：mcp-server-go/core/describe.go —— 同步描述入口：LLMStore 闸门 + 单次 Upsert + 喂索引（describe_file 与 HTTP describe 的唯一实现）
// 修改：2026-09-05（日期由 fresh-header.ps1 刷新）

package core

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Reisentyann/Mabel-s-Tentacles/describer-go"
	"github.com/Reisentyann/Mabel-s-Tentacles/describer-go/llm"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/repo"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/service"
)

// DescribeRequest 描述提交入参（MCP describe_file 与 HTTP describe 的
// 参数面收敛；两侧壳层只做解析，语义归这里——消灭原有的两处复制粘贴）。
type DescribeRequest struct {
	Path        string
	Title       *string
	Description *string
	Tags        []string
	FileType    *string
	Mode        string         // "append"：描述追加为下一段；其余 = 覆写
	Attributes  map[string]any // llm 轨输入（过 LLMStore 闸门）；nil/空 = 无
}

// DescribeResult 描述提交回执。
type DescribeResult struct {
	Rejected []string // 被闸门拒绝的键（"key(op): 原因"，回传模型自纠错）
}

// Describe 同步入口——为何同步：拒绝列表必须当场回传给模型自纠错，
// 异步会把校验反馈丢进后台。执行：resolve+stat 存在性 → 读旧 attrs/旧描述
// → append 拼接 → LLMStore.SetMany/Commit（cod-* 只读、受控词表、
// null 墓碑删除、审计戳）→ 单次 Upsert → 喂索引（补上原实现缺的喂食洞：
// llm-* 是可索引字段，改了不喂即索引漂移）。
func (o *Orchestrator) Describe(ctx context.Context, req DescribeRequest, sessionID string) (*DescribeResult, error) {
	start := time.Now()
	abs, err := service.ResolvePath(o.opts.DataDir, req.Path)
	if err != nil {
		return nil, err
	}
	if info, serr := os.Stat(abs); serr != nil || info.IsDir() {
		return nil, fmt.Errorf("'%s' does not exist", req.Path)
	}

	// 读旧：attrs（llm 合并基底 + sink diff 旧值侧）与旧描述（append 拼接）
	var oldAttrs map[string]any
	var oldDesc string
	if m, gerr := o.opts.Store.GetMetadata(ctx, req.Path); gerr == nil && m != nil {
		oldAttrs = describer.AttrsFromJSON(m.Attributes)
		if m.Description != nil {
			oldDesc = *m.Description
		}
	} else if gerr != nil && !errors.Is(gerr, pgx.ErrNoRows) {
		return nil, fmt.Errorf("get metadata: %w", gerr)
	}

	description := ""
	if req.Description != nil {
		description = *req.Description
	}
	if req.Mode == "append" && oldDesc != "" && description != "" {
		description = oldDesc + "\n\n" + description
	}
	var descPtr *string
	if description != "" {
		descPtr = &description
	}

	// llm 语义字段：LLMStore 中间件（唯一写入口）
	attrs := oldAttrs
	var rejected []string
	if len(req.Attributes) > 0 {
		st := llm.OpenLLM()
		st.SetMany(req.Attributes)
		for _, r := range st.Rejected() {
			slog.Warn("describe attribute rejected by llm middleware",
				"path", req.Path, "session", sessionID,
				"key", r.Key, "op", r.Op, "reason", r.Reason)
			rejected = append(rejected, r.Key+"("+r.Op+"): "+r.Reason)
		}
		attrs = st.Commit(oldAttrs, llm.LLMSourceAgent, time.Now())
	}

	meta := &repo.FileMetadata{
		FilePath:    req.Path,
		Title:       req.Title,
		Description: descPtr,
		Tags:        req.Tags,
		FileType:    req.FileType,
		SessionID:   strPtr(sessionID),
		Attributes:  describer.JSONFromAttrs(attrs),
	}
	uuid, err := o.opts.Store.UpsertMetadata(ctx, meta)
	if err != nil {
		slog.Error("describe submit failed",
			"path", req.Path, "session", sessionID,
			"error", err, "duration", time.Since(start).String())
		return nil, fmt.Errorf("upsert metadata: %w", err)
	}
	if o.opts.Sink != nil {
		o.opts.Sink.Update(uuid, oldAttrs, attrs)
	}

	slog.Info("describe submit ok",
		"path", req.Path, "session", sessionID, "mode", req.Mode,
		"rejected", len(rejected), "uuid", uuid,
		"duration", time.Since(start).String())
	return &DescribeResult{Rejected: rejected}, nil
}
