// 文件：mcp-server-go/internal/tools/common.go —— 工具共享层：Result 构造 / SessionID / RecordOperation / RecordFileMeta（T1 管线）/ DownloadURL
// 修改：2026-09-03（日期由 fresh-header.ps1 刷新）

package tools

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/Reisentyann/Mabel-s-Tentacles/describer-go"
	_ "github.com/Reisentyann/Mabel-s-Tentacles/describer-go/all"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/config"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/repo"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/service"
)

func Result(v map[string]any) *mcp.CallToolResult {
	b, _ := json.Marshal(v)
	return mcp.NewToolResultText(string(b))
}

func ResultError(message string) *mcp.CallToolResult {
	return Result(map[string]any{"success": false, "message": message})
}

func SessionID(ctx context.Context) string {
	if s := server.ClientSessionFromContext(ctx); s != nil {
		return s.SessionID()
	}
	return ""
}

func RecordOperation(ctx context.Context, st repo.Store, sessionID, tool, filePath, status, errMsg string, params map[string]any) {
	if st == nil {
		return
	}
	if err := st.RecordOperation(ctx, sessionID, tool, filePath, status, errMsg, params); err != nil {
		slog.Warn("record operation failed", "tool", tool, "error", err)
	}
}

// RecordFileMeta 写文件后自动记录元数据：确定性描述（describer-go 全插件流水线，
// cod-* 事实）+ 技术元数据（类型/大小/校验和/会话/分区）。
// attributes 走读-改-写合并，保留 llm-* / sp-* 既有字段（docs/元数据字段说明.md）；
// game/ 前缀的文件自动归入 game 分区（游戏室预留命名空间）。
func RecordFileMeta(ctx context.Context, st repo.Store, filePath string, content []byte, sessionID string) {
	if st == nil {
		return
	}
	start := time.Now()
	now := start

	// 确定性描述：字节进、事实出（head 512B 嗅探 + 全量分析）
	head := content
	if len(head) > 512 {
		head = head[:512]
	}
	results := describer.Analyze(describer.Input{
		Path:    filePath,
		Head:    head,
		Full:    content,
		Size:    int64(len(content)),
		MTime:   now,
		ExtMime: extMime(filePath),
	}, nil)

	// 读-改-写：整族合并 cod-*，保留 llm-* / sp-*（单会话写单文件，竞态窗口极小）
	var existing map[string]any
	if m, err := st.GetMetadata(ctx, filePath); err == nil && m != nil {
		existing = describer.AttrsFromJSON(m.Attributes)
	}
	merged := describer.MergeResults(existing, results, now)

	ft, mt := service.InferFileMeta(filePath)
	ext := service.InferExtension(filePath)
	cs := service.ChecksumSHA256(content)
	size := int64(len(content))
	meta := &repo.FileMetadata{
		FilePath:   filePath,
		Scope:      service.InferScope(filePath),
		FileType:   &ft,
		MimeType:   &mt,
		Extension:  StrPtr(ext),
		SizeBytes:  &size,
		Checksum:   &cs,
		SessionID:  StrPtr(sessionID),
		Attributes: describer.JSONFromAttrs(merged),
	}
	if err := st.UpsertMetadata(ctx, meta); err != nil {
		slog.Warn("record file metadata failed",
			"path", filePath, "session", sessionID,
			"families", familyNames(results), "error", err,
			"duration", time.Since(start).String())
		return
	}
	slog.Info("file metadata recorded",
		"path", filePath,
		"session", sessionID,
		"scope", meta.Scope,
		"families", familyNames(results),
		"cod_keys", attrKeyCount(results),
		"size", size,
		"duration", time.Since(start).String())
}

// familyNames 本次分析命中的插件家族（路由正确与否的第一现场）。
func familyNames(results []describer.Result) []string {
	out := make([]string, 0, len(results))
	for _, r := range results {
		out = append(out, r.Family)
	}
	return out
}

// attrKeyCount 本次新产出的 cod/sp-cod 键总数（字段缺失类 bug 的对照基线）。
func attrKeyCount(results []describer.Result) int {
	n := 0
	for _, r := range results {
		n += len(r.Attrs)
	}
	return n
}

// extMime 扩展名推断的 MIME（供 cod-basic-mime-match 对比嗅探结果）。
func extMime(path string) string {
	_, mt := service.InferFileMeta(path)
	return mt
}

// StrPtr 空字符串返回 nil（表示「不覆盖原值」），非空返回指针。
func StrPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// DownloadURL 构造文件的对外下载地址。download_base_url 未配置时返回空串（不返回下载链接）。
func DownloadURL(cfg *config.Config, filePath string) string {
	base := strings.TrimRight(cfg.API.DownloadBaseURL, "/")
	if base == "" {
		return ""
	}
	u := base + "/api/files/download?path=" + url.QueryEscape(filePath)
	if cfg.API.AccessToken != "" {
		u += "&token=" + url.QueryEscape(cfg.API.AccessToken)
	}
	return u
}
