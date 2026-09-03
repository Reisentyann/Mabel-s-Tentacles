// 文件：mcp-server-go/internal/service/metadata.go —— 元数据推断：InferFileMeta / InferExtension / InferScope / ChecksumSHA256
// 修改：2026-09-03（日期由 fresh-header.ps1 刷新）

package service

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strings"
)

// InferScope 按保留命名空间推导文件分区：game/<分组>/<房间>/ 前缀的文件属于 game 分区，
// 其余为 global（UpsertMetadata 对空值兜底为 global）。游戏室功能预留此命名空间。
func InferScope(path string) string {
	if strings.HasPrefix(strings.ToLower(path), "game/") {
		return "game"
	}
	return ""
}

// InferExtension 返回文件扩展名（含点，小写），无扩展名返回空串。
func InferExtension(path string) string {
	return strings.ToLower(filepath.Ext(path))
}

// ChecksumSHA256 计算内容的 SHA-256 哈希（十六进制），用于去重/完整性校验。
func ChecksumSHA256(content []byte) string {
	h := sha256.Sum256(content)
	return hex.EncodeToString(h[:])
}

// InferFileMeta 根据文件扩展名推断 file_type 和 mime_type。
func InferFileMeta(path string) (fileType, mimeType string) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".txt", ".md", ".log":
		return "text", "text/plain"
	case ".html", ".htm":
		return "text", "text/html"
	case ".json":
		return "text", "application/json"
	case ".csv":
		return "text", "text/csv"
	case ".png":
		return "image", "image/png"
	case ".jpg", ".jpeg":
		return "image", "image/jpeg"
	case ".gif":
		return "image", "image/gif"
	case ".svg":
		return "image", "image/svg+xml"
	case ".webp":
		return "image", "image/webp"
	case ".go":
		return "code", "text/plain"
	case ".py":
		return "code", "text/x-python"
	case ".js":
		return "code", "text/javascript"
	case ".ts":
		return "code", "text/typescript"
	case ".sh":
		return "code", "text/x-sh"
	case ".java":
		return "code", "text/plain"
	case ".c", ".h", ".cpp", ".hpp":
		return "code", "text/plain"
	case ".zip":
		return "archive", "application/zip"
	case ".pdf":
		return "document", "application/pdf"
	case ".mp4":
		return "video", "video/mp4"
	case ".mp3":
		return "audio", "audio/mpeg"
	default:
		return "other", "application/octet-stream"
	}
}
