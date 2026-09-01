package describer

import (
	"testing"
	"time"
)

func TestMergeResultsFamilyReplace(t *testing.T) {
	existing := map[string]any{
		"cod-image-width":    1,
		"cod-image-family":   "old",
		"sp-cod-exif-lens":   "50mm",
		"sp-cod-png-prompt":  "old prompt",
		"llm-tone":           "保留",
		"cod-text-lines":     5,
		"cod-image-at":       int64(1),
	}
	results := []Result{{
		Family:  "image",
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
}

func TestMergeLLMCOALESCE(t *testing.T) {
	existing := map[string]any{
		"llm-tone":    "旧基调",
		"llm-summary": "旧总结",
		"sp-llm-x":    "旧",
		"cod-at-x":    1,
	}
	attrs, _ := SanitizeLLM(map[string]any{"llm-tone": "新基调", "llm-action": "新动作"})
	m := MergeLLM(existing, attrs, time.Unix(1700000000, 0))

	if m["llm-tone"] != "新基调" {
		t.Fatalf("provided llm field should overwrite, got %v", m["llm-tone"])
	}
	if m["llm-summary"] != "旧总结" {
		t.Fatal("unprovided llm field must keep old value (COALESCE)")
	}
	if m["llm-action"] != "新动作" {
		t.Fatal("new llm field should be written")
	}
	if m["sp-llm-x"] != "旧" {
		t.Fatal("sp-llm must survive")
	}
	if _, ok := m["llm-at"]; !ok {
		t.Fatal("llm-at should be refreshed")
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
