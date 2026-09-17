// 文件：mcp-server-go/internal/service/shortlink.go —— 短链服务：6 位 Base62 短码签发、有效期管理与解析
// 修改：2026-09-17（日期由 fresh-header.ps1 刷新）

package service

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"
)

var (
	ErrShortLinkNotFound = errors.New("short link not found")
	ErrShortLinkExpired  = errors.New("short link expired")
)

type ShortLink struct {
	Code      string    `json:"code"`
	FilePath  string    `json:"file_path"`
	UUID      string    `json:"uuid"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

type ShortLinkStore interface {
	CreateShortLink(ctx context.Context, code, filePath, uuid string, expiresAt time.Time) error
	GetShortLink(ctx context.Context, code string) (*ShortLink, error)
}

const (
	base62Chars         = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	defaultShortCodeLen = 6
	defaultShortLinkTTL = 24 * time.Hour
)

// GenerateShortCode 生成指定长度的 Base62 密码学安全伪随机字符串。
func GenerateShortCode(length int) (string, error) {
	if length <= 0 {
		length = defaultShortCodeLen
	}
	maxIdx := big.NewInt(int64(len(base62Chars)))
	var sb strings.Builder
	sb.Grow(length)
	for i := 0; i < length; i++ {
		idx, err := rand.Int(rand.Reader, maxIdx)
		if err != nil {
			return "", err
		}
		sb.WriteByte(base62Chars[idx.Int64()])
	}
	return sb.String(), nil
}

// IssueShortURL 为目标文件生成 /d/{code} 短链并存库（默认 24 小时有效）。
func IssueShortURL(ctx context.Context, st ShortLinkStore, baseURL, logicPath, uuid string, ttl time.Duration) (string, error) {
	if st == nil {
		return "", errors.New("store not available")
	}
	if ttl <= 0 {
		ttl = defaultShortLinkTTL
	}
	expiresAt := time.Now().Add(ttl)

	var code string
	var err error
	for attempts := 0; attempts < 3; attempts++ {
		code, err = GenerateShortCode(defaultShortCodeLen)
		if err != nil {
			return "", fmt.Errorf("generate short code: %w", err)
		}
		if err = st.CreateShortLink(ctx, code, logicPath, uuid, expiresAt); err == nil {
			break
		}
	}
	if err != nil {
		return "", fmt.Errorf("create short link: %w", err)
	}

	base := strings.TrimRight(baseURL, "/")
	return base + "/d/" + code, nil
}

// ResolveShortLink 校验并解析短码，过期或不存在返回哨兵错误。
func ResolveShortLink(ctx context.Context, st ShortLinkStore, code string) (*ShortLink, error) {
	if st == nil {
		return nil, errors.New("store not available")
	}
	sl, err := st.GetShortLink(ctx, code)
	if err != nil {
		return nil, ErrShortLinkNotFound
	}
	if time.Now().After(sl.ExpiresAt) {
		return nil, ErrShortLinkExpired
	}
	return sl, nil
}
