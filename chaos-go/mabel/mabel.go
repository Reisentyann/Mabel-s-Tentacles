// 文件：chaos-go/mabel/mabel.go —— 混沌机组件·梅贝尔台词：台词库分组 + 随机取样 + 功能自注册
// 修改：2026-09-11（日期由 fresh-header.ps1 刷新）

// Package mabel 是混沌机的「梅贝尔台词机」组件：go:embed 台词库，按空行
// 切段（多行台词算一段/一个 group），随机返回若干段。功能名 `mabel_quote`。
package mabel

import (
	_ "embed"
	"errors"
	"regexp"
	"strings"
	"sync"

	"github.com/Reisentyann/Mabel-s-Tentacles/chaos-go"
)

// 台词库随二进制编译进组件（正本见 docs/梅贝尔全文本.txt）。
//
//go:embed lines.txt
var raw string

var (
	once   sync.Once
	groups []string
)

func init() { chaos.Register(quote{}) }

type quote struct{}

func (quote) Name() string { return "mabel_quote" }

func (quote) Description() string {
	return "Return random Mabel quotes from the built-in line library. " +
		"Use it when the user wants flavor, roleplay, or a break — let Mabel 'speak'. " +
		"Each quote is one self-contained segment (may span multiple lines) and already carries its 「」 quotation marks; relay it to the user as-is. " +
		"The library has hundreds of segments; returns 1-5 distinct quotes at random by default, and repeated calls give variety."
}

func (quote) Params() []chaos.Param {
	return []chaos.Param{
		{Name: "count", Type: chaos.ParamNumber, Description: "Optional number of distinct quotes to return, 1-5. Omit (or pass 0) for a random 1-5.", Default: 0},
	}
}

func (q quote) Run(c *chaos.Chaos, p chaos.Params) (map[string]any, error) {
	lines := pick(c, chaos.IntParam(p, "count", 0))
	if len(lines) == 0 {
		return nil, errors.New("台词库为空")
	}
	return map[string]any{
		"success": true,
		"count":   len(lines),
		"lines":   lines,
		"hint":    "原样发给用户即可（已含「」引号，可整段引用）。",
	}, nil
}

// Count 返回台词库总组数（一组 = 一段台词，可能多行）。
func Count() int { return len(library()) }

// Lines 用默认混沌机随机取 n 段台词。
func Lines(n int) []string { return pick(chaos.Default(), n) }

// pick 随机取 n 段互不重复的台词：n<=0 随机 1-5 段；n 上限 5；超过库容量取全库。
func pick(c *chaos.Chaos, n int) []string {
	lib := library()
	total := len(lib)
	if total == 0 {
		return nil
	}
	if n <= 0 {
		n = 1 + c.IntN(5)
	}
	if n > 5 {
		n = 5
	}
	if n > total {
		n = total
	}
	idx := make([]int, total)
	for i := range idx {
		idx[i] = i
	}
	c.Shuffle(total, func(i, j int) { idx[i], idx[j] = idx[j], idx[i] })
	out := make([]string, n)
	for i := 0; i < n; i++ {
		out[i] = lib[idx[i]]
	}
	return out
}

// codeRe 匹配 RPG Maker 控制码 \c[n]，兼容转录里缺失右括号的 \c[n。
var codeRe = regexp.MustCompile(`\\c\[[0-9]*\]?`)

// library 懒解析台词库（进程内一次）。
func library() []string {
	once.Do(func() { groups = parse(raw) })
	return groups
}

// parse 把台词库按空行切成组：每个非空行块 = 一段台词（可能多行）。
func parse(s string) []string {
	s = strings.TrimPrefix(s, "\ufeff")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	var out []string
	var cur []string
	flush := func() {
		if len(cur) == 0 {
			return
		}
		text := clean(strings.Join(cur, "\n"))
		if text != "" {
			out = append(out, text)
		}
		cur = nil
	}
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) == "" {
			flush()
			continue
		}
		cur = append(cur, strings.TrimRight(line, " \t"))
	}
	flush()
	return out
}

// clean 清掉转录残留：控制码 \c[n]、转义叹号 \!。
func clean(s string) string {
	s = codeRe.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, `\!`, "!")
	return strings.TrimSpace(s)
}
