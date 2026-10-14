// 文件：mcp-server-go/internal/api/shortlink.go —— 短链下载端点：GET /d/{code} 直流下载与友好提示
// 修改：2026-09-17（日期由 fresh-header.ps1 刷新）

package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Reisentyann/Mabel-s-Tentacles/manager-go"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/service"
)

// createShareLink 为既有文件签发分享短链：GET/POST /api/files/share?path=...
func (s *Server) createShareLink(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		var body struct {
			Path string `json:"path"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		path = body.Path
	}
	if path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	if !s.canActFile(w, r, path, "read", false) {
		return
	}

	m, err := s.repo.GetMetadata(r.Context(), path)
	if err != nil || m == nil {
		writeError(w, http.StatusNotFound, "file not found")
		return
	}

	dlBase := strings.TrimRight(s.cfg.API.DownloadBaseURL, "/")
	if dlBase == "" {
		dlBase = s.cfg.Server.BaseURL
	}

	shortURL, err := service.IssueShortURL(r.Context(), s.repo, dlBase, path, m.UUID, 24*time.Hour)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":   true,
		"short_url": shortURL,
		"path":      path,
	})
}

func (s *Server) downloadShortLink(w http.ResponseWriter, r *http.Request) {
	code := strings.TrimPrefix(r.URL.Path, "/d/")
	if code == "" {
		http.Error(w, "invalid short code", http.StatusBadRequest)
		return
	}

	if s.repo == nil {
		http.Error(w, "database unavailable", http.StatusInternalServerError)
		return
	}

	sl, err := service.ResolveShortLink(r.Context(), s.repo, code)
	if err != nil {
		if errors.Is(err, service.ErrShortLinkExpired) {
			renderExpiredHTML(w)
			return
		}
		renderNotFoundHTML(w)
		return
	}

	// 查找文件物理位置
	rel, serr := manager.StoragePathOf(sl.UUID, sl.FilePath)
	if serr != nil {
		renderNotFoundHTML(w)
		return
	}
	target, rerr := service.ResolvePath(s.cfg.DataDir, rel)
	if rerr != nil {
		renderNotFoundHTML(w)
		return
	}

	info, statErr := os.Stat(target)
	if statErr != nil || info.IsDir() {
		renderNotFoundHTML(w)
		return
	}

	// 递增下载计数
	_ = s.repo.IncrementDownloadCount(r.Context(), sl.FilePath)

	// 下载文件名处理：支持 RFC 5987 UTF-8 编码，防止中文文件名乱码
	fn := filepath.Base(sl.FilePath)
	escapedFn := url.PathEscape(fn)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, fn, escapedFn))

	slog.Info("short link download ok", "code", code, "path", sl.FilePath, "bytes", info.Size())
	http.ServeFile(w, r, target)
}

func renderExpiredHTML(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusGone)
	w.Write([]byte(`<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>下载链接已过期 · 梅贝尔之触</title>
  <style>
    body { background: #14121a; color: #e8e4f0; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; display: flex; align-items: center; justify-content: center; min-height: 100vh; margin: 0; padding: 20px; box-sizing: border-box; }
    .card { background: #1c1926; border: 1px solid #322c42; padding: 36px 28px; border-radius: 12px; text-align: center; max-width: 420px; box-shadow: 0 8px 24px rgba(0,0,0,0.4); }
    .icon { font-size: 42px; margin-bottom: 12px; }
    h2 { color: #f56c6c; margin: 0 0 12px; font-size: 1.25rem; }
    p { color: #9b93ad; font-size: 0.9rem; line-height: 1.6; margin: 0; }
  </style>
</head>
<body>
  <div class="card">
    <div class="icon">⏳</div>
    <h2>下载链接已过期</h2>
    <p>该文件的限时分享短链已超过有效期（默认 24 小时）。如需下载，请联系发送者重新获取分享链接。</p>
  </div>
</body>
</html>`))
}

func renderNotFoundHTML(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	w.Write([]byte(`<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>文件未找到 · 梅贝尔之触</title>
  <style>
    body { background: #14121a; color: #e8e4f0; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; display: flex; align-items: center; justify-content: center; min-height: 100vh; margin: 0; padding: 20px; box-sizing: border-box; }
    .card { background: #1c1926; border: 1px solid #322c42; padding: 36px 28px; border-radius: 12px; text-align: center; max-width: 420px; box-shadow: 0 8px 24px rgba(0,0,0,0.4); }
    .icon { font-size: 42px; margin-bottom: 12px; }
    h2 { color: #b876d9; margin: 0 0 12px; font-size: 1.25rem; }
    p { color: #9b93ad; font-size: 0.9rem; line-height: 1.6; margin: 0; }
  </style>
</head>
<body>
  <div class="card">
    <div class="icon">🐙</div>
    <h2>未找到该文件</h2>
    <p>该短链可能已被移除，或者物理文件已被清理。</p>
  </div>
</body>
</html>`))
}
