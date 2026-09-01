// Package text cod-text 插件：文本统计事实（无语义）。
// 字段字典见 docs/元数据字段说明.md 第 4.3 节。
package text

import (
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"

	"github.com/Reisentyann/Mabel-s-Tentacles/describer-go"
)

func init() { describer.Register(descriptor{}) }

type descriptor struct{}

func (descriptor) Family() string                                { return "text" }
func (descriptor) SPNamespaces() []string                        { return nil }
func (descriptor) Supports(_ string, _ []byte, b describer.Basic) bool {
	return b.Textish
}

func (descriptor) Analyze(in describer.Input, full []byte) (map[string]any, map[string]string) {
	if len(full) == 0 {
		return nil, nil
	}
	a := map[string]any{}

	decoded, enc := decodeText(full)
	a["cod-text-encoding"] = enc

	lines := strings.Split(decoded, "\n")
	a["cod-text-lines"] = len(lines)
	runes := []rune(decoded)
	a["cod-text-chars"] = len(runes)

	cjk, kana, latin := 0, 0, 0
	for _, r := range runes {
		switch {
		case describer.IsCJK(r):
			cjk++
		case describer.IsKana(r):
			kana++
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'):
			latin++
		}
	}
	rn := float64(len(runes))
	a["cod-text-cjk-ratio"] = describer.Round2(float64(cjk) / rn)
	a["cod-text-language"] = language(cjk, kana, latin, rn)

	a["cod-text-top-keywords"] = topKeywords(decoded, runes, cjk > 0, latin > 0)

	if t := titleLine(lines); t != "" {
		a["cod-text-title-line"] = t
	}
	if hs := headings(lines); len(hs) > 0 {
		a["cod-text-headings"] = hs
	}
	if st := structure(lines); len(st) > 0 {
		a["cod-text-structure"] = st
	}
	a["cod-text-blank-ratio"] = describer.Round2(blankRatio(lines))

	return a, nil
}

// decodeText 解码：严格 UTF-8 优先，其次 GBK，兜底 latin-1。
func decodeText(b []byte) (string, string) {
	if utf8.Valid(b) {
		return string(b), "utf-8"
	}
	if dec, err := simplifiedchinese.GBK.NewDecoder().Bytes(b); err == nil && utf8.Valid(dec) {
		return string(dec), "gbk"
	}
	// latin-1：每字节直接映射 U+0000-U+00FF
	r := make([]rune, len(b))
	for i, x := range b {
		r[i] = rune(x)
	}
	return string(r), "latin-1"
}

func language(cjk, kana, latin int, total float64) string {
	if total == 0 {
		return "unknown"
	}
	hasCJK := float64(cjk)/total > 0.05
	hasLatin := float64(latin)/total > 0.2
	switch {
	case hasCJK && hasLatin:
		return "mixed"
	case kana > 0 && hasCJK:
		return "ja"
	case hasCJK:
		return "zh"
	case hasLatin:
		return "en"
	default:
		return "unknown"
	}
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
				freq[string(runes[i : i+2])]++
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

var (
	reHeadingMD  = regexp.MustCompile(`^#{1,6}\s+\S`)
	reHeadingCN  = regexp.MustCompile(`^\s*(第|卷|序章|尾声)[\s]{0,2}\S{0,20}(章|节|回|卷)`)
	reHeadingCh  = regexp.MustCompile(`(?i)^\s*chapter\s+\w+`)
	reHeadingNum = regexp.MustCompile(`^\s*\d{1,3}[.、]\s*\S`)
	reListItem   = regexp.MustCompile(`^\s*([-*+]|\d{1,2}[.)、])\s+\S`)
	reTableRow   = regexp.MustCompile(`^\s*\|.*\|`)
	reSpace      = regexp.MustCompile(`^\s*$`)
)

func isHeading(line string) bool {
	return reHeadingMD.MatchString(line) || reHeadingCN.MatchString(line) ||
		reHeadingCh.MatchString(line) || reHeadingNum.MatchString(line)
}

// headingText 标题正文：剥掉 markdown # 前缀，其余原样。
func headingText(line string) string {
	s := strings.TrimSpace(line)
	if reHeadingMD.MatchString(s) {
		return strings.TrimSpace(strings.TrimLeft(s, "#"))
	}
	return s
}

// titleLine 标题猜测：前 50 行里首个标题行，否则首个非空行；截 80 字符。
func titleLine(lines []string) string {
	limit := len(lines)
	if limit > 50 {
		limit = 50
	}
	first := ""
	for i := 0; i < limit; i++ {
		l := strings.TrimRight(lines[i], "\r")
		if reSpace.MatchString(l) {
			continue
		}
		if isHeading(l) {
			return describer.TrimRunes(headingText(l), 80)
		}
		if first == "" {
			first = l
		}
	}
	if first == "" {
		return ""
	}
	return describer.TrimRunes(strings.TrimSpace(first), 80)
}

// headings 前 8 个标题（markdown #、中文章节、Chapter、数字编号）。
func headings(lines []string) []string {
	var out []string
	for _, l := range lines {
		if isHeading(strings.TrimRight(l, "\r")) {
			out = append(out, describer.TrimRunes(headingText(l), 80))
			if len(out) >= 8 {
				break
			}
		}
	}
	return out
}

// structure 结构检测（多选）。
func structure(lines []string) []string {
	var out []string
	if len(lines) >= 2 && strings.TrimSpace(lines[0]) == "---" {
		for i := 1; i < len(lines); i++ {
			if strings.TrimSpace(lines[i]) == "---" {
				out = append(out, "frontmatter")
				break
			}
		}
	}
	listN, tableN, fenceN := 0, 0, 0
	for _, l := range lines {
		l = strings.TrimRight(l, "\r")
		if reListItem.MatchString(l) {
			listN++
		}
		if reTableRow.MatchString(l) {
			tableN++
		}
		if strings.HasPrefix(strings.TrimSpace(l), "```") {
			fenceN++
		}
	}
	if listN >= 3 {
		out = append(out, "lists")
	}
	if fenceN >= 2 {
		out = append(out, "code-blocks")
	}
	if tableN >= 2 {
		out = append(out, "tables")
	}
	for _, l := range lines {
		if isHeading(l) {
			out = append(out, "headings")
			break
		}
	}
	return out
}

func blankRatio(lines []string) float64 {
	if len(lines) == 0 {
		return 0
	}
	blank := 0
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			blank++
		}
	}
	return float64(blank) / float64(len(lines))
}
