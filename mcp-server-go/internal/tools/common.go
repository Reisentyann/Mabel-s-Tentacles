package tools

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/url"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

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

// RecordFileMeta 写文件后自动记录元数据（类型/扩展名/大小/校验和/会话），供检索使用。
func RecordFileMeta(ctx context.Context, st repo.Store, filePath string, content []byte, sessionID string) {
	if st == nil {
		return
	}
	ft, mt := service.InferFileMeta(filePath)
	ext := service.InferExtension(filePath)
	cs := service.ChecksumSHA256(content)
	size := int64(len(content))
	meta := &repo.FileMetadata{
		FilePath:  filePath,
		Scope:     "global",
		FileType:  &ft,
		MimeType:  &mt,
		Extension: StrPtr(ext),
		SizeBytes: &size,
		Checksum:  &cs,
		SessionID: StrPtr(sessionID),
	}
	if err := st.UpsertMetadata(ctx, meta); err != nil {
		slog.Warn("record file metadata failed", "path", filePath, "error", err)
	}
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
