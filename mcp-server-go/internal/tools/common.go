// 文件：mcp-server-go/internal/tools/common.go —— 工具共享层：Result 构造 / SessionID / RecordOperation / DownloadURL / StrPtr（T1 管线已迁 core 执行器，2026-09-06 接线批次）
// 修改：2026-09-06（日期由 fresh-header.ps1 刷新）

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

// StrPtr 空字符串返回 nil（表示「不覆盖原值」），非空返回指针。
// agent 侧可选字段 → core.AgentMeta 指针语义的统一转换口。
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
