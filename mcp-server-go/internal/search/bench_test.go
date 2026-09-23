// 文件：mcp-server-go/internal/search/bench_test.go —— 基准注册表 L1：收录完备性抽查 + 词条质量底线
// 修改：2026-09-23（日期由 fresh-header.ps1 刷新）

package search_test

import (
	"strings"
	"testing"

	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/search"
)

// TestBenchRegistryQuality 词条质量底线：无空 Desc / 无空 Bench / 键无空白。
func TestBenchRegistryQuality(t *testing.T) {
	if n := search.BenchLen(); n < 90 {
		t.Fatalf("registry = %d entries, want >= 90 (cod-basic/text/image/code + jm + llm families)", n)
	}
	for field, b := range benchAll(t) {
		if strings.TrimSpace(field) != field || field == "" {
			t.Fatalf("field key %q has whitespace trouble", field)
		}
		if b.Desc == "" || strings.TrimSpace(b.Desc) == "" {
			t.Fatalf("%s: empty Desc", field)
		}
		if b.Bench == "" || strings.TrimSpace(b.Bench) == "" {
			t.Fatalf("%s: empty Bench", field)
		}
	}
}

// TestBenchKeyFieldsPresent 关键抽查：agent 最常用的字段必须在表
// （目录批次实测高频：语言/长度/对话/勾选框/代码语言/图像档位/llm 类型）。
func TestBenchKeyFieldsPresent(t *testing.T) {
	want := []string{
		"cod-text-language", "cod-text-lines", "cod-text-chars", "cod-text-cjk-ratio",
		"cod-text-dialog-ratio", "cod-text-checkboxes", "cod-text-structure",
		"cod-text-digit-ratio", "cod-text-title-line",
		"cod-code-lang", "cod-code-todo-count",
		"cod-image-megapixels", "cod-image-brightness", "cod-image-family",
		"cod-basic-entropy", "cod-basic-mime-match",
		"llm-semantic-type", "llm-characters",
		"sp-cod-jm-id", "sp-cod-jm-tags", "sp-cod-jm-page_count",
	}
	for _, f := range want {
		if _, ok := search.Bench(f); !ok {
			t.Errorf("key field %s missing from bench registry", f)
		}
	}
}

// TestBenchUnknownField 自由区口径：未收录键返回 ok=false（sp-* 长尾不枚举是设计）。
func TestBenchUnknownField(t *testing.T) {
	if _, ok := search.Bench("sp-llm-某个自由观察"); ok {
		t.Fatal("sp-* free-zone keys must not be in the registry")
	}
	if _, ok := search.Bench("cod-nope-never"); ok {
		t.Fatal("unknown field must return ok=false")
	}
}

// TestGuideContract 指南契约：规则与速查表的规模底线 + 无空词条 +
// 速查条件必须是可被 ParseConditions 接受的合法语法。
func TestGuideContract(t *testing.T) {
	g := search.TheGuide()
	if len(g.Rules) < 8 {
		t.Fatalf("guide rules = %d, want >= 8", len(g.Rules))
	}
	if len(g.QuickRef) < 10 {
		t.Fatalf("guide quick_ref = %d, want >= 10", len(g.QuickRef))
	}
	for i, r := range g.Rules {
		if strings.TrimSpace(r) == "" {
			t.Fatalf("guide rules[%d] empty", i)
		}
	}
	for i, e := range g.QuickRef {
		if strings.TrimSpace(e.Want) == "" || strings.TrimSpace(e.Cond) == "" {
			t.Fatalf("guide quick_ref[%d] has empty side: %+v", i, e)
		}
		cond := e.Cond
		if i := strings.Index(cond, "（"); i >= 0 { // 剥中文括号里的备注，只留纯条件
			cond = strings.TrimSpace(cond[:i])
		}
		if _, err := search.ParseConditions(cond); err != nil {
			t.Errorf("quick_ref[%d] %q 不是合法条件: %v", i, cond, err)
		}
	}
}

// benchAll 全表快照（经 All() 只读导出）。
func benchAll(t *testing.T) map[string]search.FieldBench {
	t.Helper()
	return search.All()
}
