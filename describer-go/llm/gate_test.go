package llm

import (
	"reflect"
	"testing"
)

func TestSanitizeLLM(t *testing.T) {
	in := map[string]any{
		"llm-semantic-type": "game_guide",      // 合法枚举
		"llm-tone":          "暖橙温馨",          // 合法自由短语
		"llm-characters":    []any{"梅贝尔", "铃仙"}, // 数组
		"llm-summary":       "一句话",             //
		"sp-llm-游戏名":        "狼人杀",            // 自由字段放行
		"llm-semantic-type-x": "bad",            // 未定义 llm- 字段
		"cod-image-width":     64,               // 模型试图写 cod 前缀
		"llm-foo":             "x",              // 未定义 llm- 字段
		"llm-semantic-type2":  "novel",          // 未定义
	}
	out, dropped := SanitizeLLM(in)
	if out["llm-semantic-type"] != "game_guide" {
		t.Fatalf("semantic-type should pass, got %v", out["llm-semantic-type"])
	}
	if out["sp-llm-游戏名"] != "狼人杀" {
		t.Fatal("sp-llm-* should pass")
	}
	if chars, ok := out["llm-characters"].([]string); !ok || !reflect.DeepEqual(chars, []string{"梅贝尔", "铃仙"}) {
		t.Fatalf("characters should be []string, got %#v", out["llm-characters"])
	}
	wantDropped := []string{"cod-image-width", "llm-foo", "llm-semantic-type-x", "llm-semantic-type2"}
	if !reflect.DeepEqual(dropped, wantDropped) {
		t.Fatalf("dropped = %v, want %v", dropped, wantDropped)
	}
}

func TestSanitizeLLMRejectsBadEnum(t *testing.T) {
	out, dropped := SanitizeLLM(map[string]any{"llm-semantic-type": "科幻小说"})
	if _, ok := out["llm-semantic-type"]; ok {
		t.Fatal("invalid semantic-type should be dropped")
	}
	if len(dropped) != 1 || dropped[0] != "llm-semantic-type" {
		t.Fatalf("dropped = %v", dropped)
	}
}

func TestSanitizeLLMTruncates(t *testing.T) {
	long := make([]rune, 300)
	for i := range long {
		long[i] = '字'
	}
	chars := make([]any, 15)
	for i := range chars {
		chars[i] = "角色"
	}
	out, _ := SanitizeLLM(map[string]any{
		"llm-summary":    string(long),
		"llm-characters": chars,
	})
	if got := len([]rune(out["llm-summary"].(string))); got != 100 {
		t.Fatalf("summary should truncate to 100 runes, got %d", got)
	}
	if got := len(out["llm-characters"].([]string)); got != 10 {
		t.Fatalf("characters should cap at 10, got %d", got)
	}
}
