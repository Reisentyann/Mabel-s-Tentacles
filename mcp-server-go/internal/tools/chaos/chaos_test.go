// 文件：mcp-server-go/internal/tools/chaos/chaos_test.go —— JMComic 来源元数据适配测试
// 修改：2026-09-23（日期由 fresh-header.ps1 刷新）

package chaos

import "testing"

func TestJMAttributes(t *testing.T) {
	attrs := jmAttributes(map[string]any{
		"id":            float64(123456),
		"type":          "album",
		"title":         "示例本子",
		"author":        []any{"作者甲"},
		"tags":          []any{"标签甲", "标签乙"},
		"actors":        []string{"角色甲"},
		"works":         []any{"原创"},
		"page_count":    float64(24),
		"episode_count": float64(2),
		"episodes": []any{
			map[string]any{"id": "1001", "title": "第一话"},
		},
	})
	if attrs["sp-cod-jm-id"] != "123456" || attrs["sp-cod-jm-type"] != "album" {
		t.Fatalf("identity attrs = %#v", attrs)
	}
	if got, ok := attrs["sp-cod-jm-author"].([]string); !ok || len(got) != 1 || got[0] != "作者甲" {
		t.Fatalf("author attrs = %#v", attrs["sp-cod-jm-author"])
	}
	if got, ok := attrs["sp-cod-jm-episode-ids"].([]string); !ok || len(got) != 1 || got[0] != "1001" {
		t.Fatalf("episode attrs = %#v", attrs["sp-cod-jm-episode-ids"])
	}
}

func TestJMDescription(t *testing.T) {
	got := jmDescription(map[string]any{"id": "123", "title": "示例"})
	if got != "JMComic 123 · 示例" {
		t.Fatalf("description = %q", got)
	}
}
