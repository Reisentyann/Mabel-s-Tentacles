// 文件：describer-go/merge_test.go —— 合并语义测试：整族替换 / sp 清除 / llm 保留 / ver 写入 / JSON 往返
// 修改：2026-09-03（日期由 fresh-header.ps1 刷新）

package describer

import (
	"testing"
	"time"
)

func TestMergeResultsFamilyReplace(t *testing.T) {
	existing := map[string]any{
		"cod-image-width":   1,
		"cod-image-family":  "old",
		"sp-cod-exif-lens":  "50mm",
		"sp-cod-png-prompt": "old prompt",
		"llm-tone":          "保留",
		"cod-text-lines":    5,
		"cod-image-at":      int64(1),
	}
	results := []Result{{
		Family:  "image",
		Ver:     1,
		Attrs:   map[string]any{"cod-image-width": 64},
		SPPurge: []string{"sp-cod-exif-", "sp-cod-png-"},
	}}
	now := time.Unix(1700000000, 0)
	m := MergeResults(existing, results, now)

	if m["cod-image-width"] != 64 {
		t.Fatalf("family field should be replaced, got %v", m["cod-image-width"])
	}
	if _, ok := m["cod-image-family"]; ok {
		t.Fatal("stale family field cod-image-family should be purged")
	}
	if _, ok := m["sp-cod-exif-lens"]; ok {
		t.Fatal("stale sp-cod-exif-* should be purged")
	}
	if _, ok := m["sp-cod-png-prompt"]; ok {
		t.Fatal("stale sp-cod-png-* should be purged")
	}
	if m["llm-tone"] != "保留" {
		t.Fatal("llm field must survive cod family replace")
	}
	if m["cod-text-lines"] != 5 {
		t.Fatal("other cod family must survive")
	}
	if m["cod-image-at"] != now.Unix() {
		t.Fatalf("cod-image-at should refresh, got %v", m["cod-image-at"])
	}
	if m["cod-image-ver"] != 1 {
		t.Fatalf("cod-image-ver should be written, got %v", m["cod-image-ver"])
	}
}

func TestMergeResultsPurgesOnEmptyFamily(t *testing.T) {
	// 零产出家族（如解码失败的 image）：空 Attrs 仍整族清旧键 + 刷 ver/at，
	// 防陈旧事实残留与 IsStale 反复重触发（配 Analyze 的"已跑即入列"）
	existing := map[string]any{
		"cod-image-width":   32,
		"sp-cod-png-prompt": "旧参数",
		"cod-image-ver":     1,
		"cod-image-at":      int64(1),
		"llm-tone":          "保留",
	}
	results := []Result{{
		Family:  "image",
		Ver:     1,
		Attrs:   nil,
		SPPurge: []string{"sp-cod-exif-", "sp-cod-png-"},
	}}
	now := time.Unix(1700000000, 0)
	m := MergeResults(existing, results, now)

	if _, ok := m["cod-image-width"]; ok {
		t.Fatal("empty family output must purge old cod-image-width")
	}
	if _, ok := m["sp-cod-png-prompt"]; ok {
		t.Fatal("empty family output must purge old sp-cod-png-prompt")
	}
	if m["cod-image-ver"] != 1 {
		t.Fatalf("cod-image-ver = %v, want 1", m["cod-image-ver"])
	}
	if m["cod-image-at"] != now.Unix() {
		t.Fatalf("cod-image-at = %v, want %d", m["cod-image-at"], now.Unix())
	}
	if m["llm-tone"] != "保留" {
		t.Fatal("llm field must survive empty family purge")
	}
}

func TestJSONRoundTrip(t *testing.T) {
	if m := AttrsFromJSON(nil); m == nil || len(m) != 0 {
		t.Fatalf("nil JSON should give empty map, got %#v", m)
	}
	m := AttrsFromJSON(JSONFromAttrs(map[string]any{"a": 1.0}))
	if v, ok := m["a"].(float64); !ok || v != 1 {
		t.Fatalf("round trip failed: %#v", m)
	}
}
