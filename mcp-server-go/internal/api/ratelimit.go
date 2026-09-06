// 文件：mcp-server-go/internal/api/ratelimit.go —— 内存滑动窗限流（注册开放后的基础防滥用：登录/注册按 IP 维度）
// 修改：2026-09-06（日期由 fresh-header.ps1 刷新）

package api

import (
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/Reisentyann/Mabel-s-Tentacles/common"
)

// rateLimiter 进程内滑动窗限流（个人库量级足够；多实例部署再外部化）。
// key = 桶名 + 客户端 IP；命中窗口内的记录数超限即拒绝。
type rateLimiter struct {
	mu   sync.Mutex
	hits map[string][]time.Time
}

func newRateLimiter() *rateLimiter {
	return &rateLimiter{hits: map[string][]time.Time{}}
}

// allow 记录一次命中并判定是否放行（同时顺手清理过期记录，防 map 膨胀）。
func (l *rateLimiter) allow(key string, max int, window time.Duration) bool {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	old := l.hits[key]
	kept := old[:0]
	for _, t := range old {
		if now.Sub(t) < window {
			kept = append(kept, t)
		}
	}
	if len(kept) >= max {
		l.hits[key] = kept
		return false
	}
	l.hits[key] = append(kept, now)
	return true
}

// limit 包装 handler：超限返回 429 + WARN 日志（埋日志：限流命中是
// 撞库/滥注册的第一现场，必须留痕）。
func (s *Server) limit(bucket string, max int, window time.Duration, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := bucket + "|" + common.ClientIP(r)
		if !s.limiter.allow(key, max, window) {
			slog.Warn("rate limited",
				"bucket", bucket, "ip", common.ClientIP(r), "path", r.URL.Path,
				"max", max, "window", window.String())
			writeError(w, http.StatusTooManyRequests, "too many requests, try again later")
			return
		}
		next(w, r)
	}
}
