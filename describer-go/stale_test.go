// 文件：describer-go/stale_test.go —— IsStale 单元测试：新鲜不陈旧 / 四条陈旧路径 / JSON float64 兼容
// 修改：2026-09-03（日期由 fresh-header.ps1 刷新）

package describer

import (
	"testing"
	"time"
)

func TestIsStale(t *testing.T) {
	now := time.Unix(1700000000, 0)
	fresh := map[string]any{
		"cod-text-ver": 2,
		"cod-text-at":  float64(now.Unix()),
	}

	if IsStale(fresh, "text", 2, "abc", "abc", now) {
		t.Fatal("fresh metadata must not be stale")
	}
	if !IsStale(map[string]any{}, "text", 2, "abc", "abc", time.Time{}) {
		t.Fatal("missing ver (old data) must be stale")
	}

	oldVer := map[string]any{
		"cod-text-ver": 1,
		"cod-text-at":  float64(now.Unix()),
	}
	if !IsStale(oldVer, "text", 2, "abc", "abc", now) {
		t.Fatal("older family version must be stale")
	}

	if !IsStale(fresh, "text", 2, "abc", "xyz", now) {
		t.Fatal("checksum mismatch must be stale")
	}
	if IsStale(fresh, "text", 2, "abc", "", now) {
		t.Fatal("checksum skipped when curChecksum empty")
	}

	later := now.Add(time.Hour)
	if !IsStale(fresh, "text", 2, "abc", "abc", later) {
		t.Fatal("mtime newer than cod-text-at must be stale")
	}

	// JSON 反序列化路径：数值为 float64
	jsonish := map[string]any{
		"cod-text-ver": float64(2),
		"cod-text-at":  float64(now.Unix()),
	}
	if IsStale(jsonish, "text", 2, "abc", "abc", now) {
		t.Fatal("float64 ver/at (JSON path) must be readable")
	}
}
