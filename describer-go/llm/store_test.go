package llm

import (
	"testing"
	"time"
)

func TestCommitCOALESCE(t *testing.T) {
	// 未提供的 llm 字段保留旧值（COALESCE 语义，经中间件验证）
	existing := map[string]any{
		"llm-tone":    "旧基调",
		"llm-summary": "旧总结",
		"sp-llm-x":    "旧",
	}
	st := OpenLLM()
	st.Set("llm-tone", "新基调")
	st.Set("llm-action", "新动作")
	merged := st.Commit(existing, LLMSourceAgent, time.Unix(1700000000, 0))

	if merged["llm-tone"] != "新基调" {
		t.Fatalf("provided llm field should overwrite, got %v", merged["llm-tone"])
	}
	if merged["llm-summary"] != "旧总结" {
		t.Fatal("unprovided llm field must keep old value (COALESCE)")
	}
	if merged["llm-action"] != "新动作" {
		t.Fatal("new llm field should be written")
	}
	if merged["sp-llm-x"] != "旧" {
		t.Fatal("sp-llm must survive")
	}
}

func TestCodZoneUntouchable(t *testing.T) {
	st := OpenLLM()
	st.Set("cod-image-width", 999)
	st.Set("sp-cod-exif-lens", "改为 50mm")
	st.Delete("cod-image-family")
	st.Delete("sp-cod-png-prompt")

	if rej := st.Rejected(); len(rej) != 4 {
		t.Fatalf("all 4 cod ops must be rejected, got %d", len(rej))
	}
	for _, r := range st.Rejected() {
		if r.Reason == "" {
			t.Fatalf("rejection must carry reason: %+v", r)
		}
	}

	// Commit 后 cod 区原样透传
	existing := map[string]any{
		"cod-image-width":   64,
		"cod-image-family":  "orange",
		"sp-cod-png-prompt": "keep me",
		"llm-tone":          "暖橙",
	}
	merged := st.Commit(existing, LLMSourceAgent, time.Unix(1700000000, 0))
	if merged["cod-image-width"] != 64 {
		t.Fatal("cod field must never be writable by LLM")
	}
	if merged["cod-image-family"] != "orange" {
		t.Fatal("cod field must never be deletable by LLM")
	}
	if merged["sp-cod-png-prompt"] != "keep me" {
		t.Fatal("sp-cod field must never be deletable by LLM")
	}
}

func TestAuditFieldsSystemOnly(t *testing.T) {
	st := OpenLLM()
	st.Set("llm-source", "ollama:fake-model") // 伪造来源
	st.Delete("llm-at")
	if rej := st.Rejected(); len(rej) != 2 {
		t.Fatalf("audit field ops must be rejected, got %d", len(rej))
	}

	st2 := OpenLLM()
	st2.Set("llm-tone", "冷蓝")
	merged := st2.Commit(map[string]any{}, LLMSourceAgent, time.Unix(1700000000, 0))
	if merged["llm-source"] != LLMSourceAgent {
		t.Fatalf("llm-source must be system-stamped, got %v", merged["llm-source"])
	}
	if merged["llm-at"] != int64(1700000000) {
		t.Fatalf("llm-at must be system-stamped, got %v", merged["llm-at"])
	}
}

func TestNullTombstoneDeletes(t *testing.T) {
	st := OpenLLM()
	st.SetMany(map[string]any{
		"sp-llm-游戏名": nil, // JSON null = 删除墓碑
		"llm-tone":   "新基调",
	})
	existing := map[string]any{
		"sp-llm-游戏名": "狼人杀",
		"sp-llm-进度":  "第3章",
		"llm-tone":   "旧基调",
	}
	merged := st.Commit(existing, LLMSourceAgent, time.Unix(1700000000, 0))
	if _, ok := merged["sp-llm-游戏名"]; ok {
		t.Fatal("null tombstone must delete sp-llm key")
	}
	if merged["sp-llm-进度"] != "第3章" {
		t.Fatal("unrelated sp-llm key must survive")
	}
	if merged["llm-tone"] != "新基调" {
		t.Fatal("set alongside tombstone must apply")
	}
}

func TestDeleteLLMFixedField(t *testing.T) {
	st := OpenLLM()
	st.Delete("llm-summary")
	merged := st.Commit(map[string]any{"llm-summary": "旧总结", "llm-tone": "保留"}, LLMSourceAgent, time.Unix(1700000000, 0))
	if _, ok := merged["llm-summary"]; ok {
		t.Fatal("llm fixed field should be deletable (its own track)")
	}
	if merged["llm-tone"] != "保留" {
		t.Fatal("other llm field must survive")
	}
}

func TestNoOpCommitDoesNotStamp(t *testing.T) {
	st := OpenLLM()
	existing := map[string]any{"llm-at": int64(1), "llm-source": "old"}
	merged := st.Commit(existing, LLMSourceAgent, time.Unix(1700000000, 0))
	if merged["llm-at"] != int64(1) {
		t.Fatal("no-op commit must not touch llm-at")
	}
	if len(merged) != 2 {
		t.Fatalf("no-op commit must return as-is, got %#v", merged)
	}
}

func TestSanitizeLLMRejectsForgedSource(t *testing.T) {
	out, dropped := SanitizeLLM(map[string]any{
		"llm-source": "ollama:spoof",
		"llm-tone":   "ok",
	})
	if _, ok := out["llm-source"]; ok {
		t.Fatal("SanitizeLLM must reject forged llm-source")
	}
	if out["llm-tone"] != "ok" {
		t.Fatal("legit field should pass")
	}
	if len(dropped) != 1 || dropped[0] != "llm-source" {
		t.Fatalf("dropped = %v", dropped)
	}
}

func TestRejectionListDeterministic(t *testing.T) {
	st := OpenLLM()
	st.Set("zzz-unknown", 1)
	st.Set("cod-a", 1)
	st.Delete("cod-b")
	rej := st.Rejected()
	if len(rej) != 3 {
		t.Fatalf("want 3 rejections, got %d", len(rej))
	}
	if rej[0].Key != "cod-a" || rej[1].Key != "cod-b" || rej[2].Key != "zzz-unknown" {
		t.Fatalf("rejections must be key-sorted: %+v", rej)
	}
}
