// 文件：mcp-server-go/internal/logging/logging.go —— slog 初始化（控制台+文件，JSON/Text）+ 日志字段规范
// 修改：2026-09-03（日期由 fresh-header.ps1 刷新）

// Package logging 统一 slog 初始化（控制台 + 可选文件，JSON/Text 双格式）。
//
// 日志字段规范（出 bug 时日志是唯一证词，宁多勿缺）：
//   - 入口：tool（MCP 工具名）或 http request（method+path）
//   - 谁干的：session（MCP 会话）/ user（HTTP JWT 或 user_id）——写操作必带
//   - 操作现场：path / source+target / mode / bytes 等参数
//   - 耗时：duration——性能排查唯一依据
//   - 结果：失败必带 error；成功带关键产出计量（families / cod_keys /
//     returned / exit_code / bytes_out…），字段缺失类 bug 靠它对现场
package logging

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// Setup 初始化 slog：同时输出到控制台（stdout）和日志文件（若 logFile 非空）。
func Setup(level, format, logFile string) {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: lvl}

	var w io.Writer = os.Stdout
	if logFile != "" {
		if dir := filepath.Dir(logFile); dir != "." && dir != "" {
			_ = os.MkdirAll(dir, 0o755)
		}
		if f, err := os.OpenFile(logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
			w = io.MultiWriter(os.Stdout, f)
		}
	}

	var handler slog.Handler
	if format == "text" {
		handler = slog.NewTextHandler(w, opts)
	} else {
		handler = slog.NewJSONHandler(w, opts)
	}

	slog.SetDefault(slog.New(handler))
}
