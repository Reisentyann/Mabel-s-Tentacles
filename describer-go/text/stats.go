// Package text 内的 stats.go：cod-text 基础统计字段（无语义、纯计数）。
// 字段字典见 docs/元数据字段说明.md 第 4.3 节基础统计部分。
package text

import (
	"sort"
	"strings"

	"github.com/Reisentyann/Mabel-s-Tentacles/describer-go"
)

func extractEncoding(ctx *textCtx) any { return ctx.encoding }
func extractLines(ctx *textCtx) any    { return len(ctx.lines) }
func extractChars(ctx *textCtx) any    { return len(ctx.runes) }

func extractCjkRatio(ctx *textCtx) any {
	rn := float64(len(ctx.runes))
	if rn == 0 {
		return nil
	}
	return describer.Round2(float64(ctx.cjk) / rn)
}

func extractLanguage(ctx *textCtx) any {
	rn := float64(len(ctx.runes))
	if rn == 0 {
		return "unknown"
	}
	switch {
	case ctx.hasCJK && ctx.hasLatin:
		return "mixed"
	case ctx.kana > 0 && ctx.hasCJK:
		return "ja"
	case ctx.hasCJK:
		return "zh"
	case ctx.hasLatin:
		return "en"
	default:
		return "unknown"
	}
}

func extractBlankRatio(ctx *textCtx) any {
	if len(ctx.lines) == 0 {
		return 0.0
	}
	blank := 0
	for _, l := range ctx.lines {
		if strings.TrimSpace(l) == "" {
			blank++
		}
	}
	return describer.Round2(float64(blank) / float64(len(ctx.lines)))
}

func extractTopKeywords(ctx *textCtx) any {
	kw := topKeywords(ctx.decoded, ctx.runes, ctx.hasCJK, ctx.hasLatin)
	if len(kw) == 0 {
		return nil
	}
	return kw
}

var enStopwords = map[string]bool{
	"a": true, "an": true, "and": true, "are": true, "as": true, "at": true,
	"be": true, "but": true, "by": true, "for": true, "from": true, "has": true,
	"have": true, "he": true, "her": true, "his": true, "i": true, "in": true,
	"is": true, "it": true, "its": true, "of": true, "on": true, "or": true,
	"she": true, "that": true, "the": true, "this": true, "to": true, "was": true,
	"we": true, "were": true, "will": true, "with": true, "you": true, "your": true,
}

// topKeywords 高频词：CJK 用 2-gram 频率统计（无语义、无分词库），
// 拉丁词用停用词过滤。计数并列时按键排序保证确定。
func topKeywords(decoded string, runes []rune, hasCJK, hasLatin bool) []string {
	const maxN = 10
	var items []struct {
		key string
		n   int
	}

	if hasCJK {
		freq := map[string]int{}
		for i := 0; i+1 < len(runes); i++ {
			if describer.IsCJK(runes[i]) && describer.IsCJK(runes[i+1]) {
				freq[string(runes[i:i+2])]++
			}
		}
		for k, n := range freq {
			if n >= 2 {
				items = append(items, struct {
					key string
					n   int
				}{k, n})
			}
		}
	}
	if hasLatin {
		freq := map[string]int{}
		for _, w := range strings.FieldsFunc(strings.ToLower(decoded), func(r rune) bool {
			return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '\'')
		}) {
			if len(w) >= 3 && !enStopwords[w] {
				freq[w]++
			}
		}
		for k, n := range freq {
			if n >= 2 {
				items = append(items, struct {
					key string
					n   int
				}{k, n})
			}
		}
	}
	if len(items) == 0 {
		return nil
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].n != items[j].n {
			return items[i].n > items[j].n
		}
		return items[i].key < items[j].key
	})
	if len(items) > maxN {
		items = items[:maxN]
	}
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.key)
	}
	return out
}
