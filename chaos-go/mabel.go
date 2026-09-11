// 文件：chaos-go/mabel.go —— 混沌机功能·梅贝尔台词：台词库解析分组 + 随机取样 + 功能注册
// 修改：2026-09-11（日期由 fresh-header.ps1 刷新）

package chaos

import (
	_ "embed"
	"errors"
	"regexp"
	"strings"
	"sync"
)

// 台词库随二进制编译进模块（部署零额外文件）。正本见 docs/梅贝尔全文本.txt。
//
//go:embed data/mabel_lines.txt
var mabelRaw string

var (
	mabelOnce   sync.Once
	mabelGroups []string
)

// MabelCount 返回台词库总组数（一组 = 一段台词，可能多行）。
func MabelCount() int { return len(mabelLibrary()) }

// MabelLines 用默认混沌机随机取 n 段台词。
func MabelLines(n int) []string { return Default().MabelLines(n) }

// MabelLines 随机取 n 段互不重复的梅贝尔台词。
// n<=0 时随机取 1-5 段；n 上限 5（超出按 5 算）；超过库容量则返回全库。
func (c *Chaos) MabelLines(n int) []string {
	groups := mabelLibrary()
	total := len(groups)
	if total == 0 {
		return nil
	}
	if n <= 0 {
		n = 1 + c.intn(5)
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
	c.shuffle(total, func(i, j int) { idx[i], idx[j] = idx[j], idx[i] })
	out := make([]string, n)
	for i := 0; i < n; i++ {
		out[i] = groups[idx[i]]
	}
	return out
}

// mabelQuote 梅贝尔台词功能（自注册，见 feature.go）。
func init() { Register(mabelQuote{}) }

type mabelQuote struct{}

func (mabelQuote) Name() string { return "mabel_quote" }

func (mabelQuote) Description() string {
	return "Return random Mabel quotes from the built-in line library (a chaos-machine entertainment feature). " +
		"Use it when the user wants flavor, roleplay, or a break — let Mabel 'speak'. " +
		"Each quote is one self-contained segment (may span multiple lines) and already carries its 「」 quotation marks; relay it to the user as-is. " +
		"The library has hundreds of segments; returns 1-5 distinct quotes at random by default, and repeated calls give variety."
}

func (mabelQuote) Params() []Param {
	return []Param{
		{Name: "count", Type: ParamNumber, Description: "Optional number of distinct quotes to return, 1-5. Omit (or pass 0) for a random 1-5.", Default: 0},
	}
}

func (mabelQuote) Run(c *Chaos, p Params) (map[string]any, error) {
	lines := c.MabelLines(IntParam(p, "count", 0))
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

// mabelCodeRe 匹配 RPG Maker 控制码 \c[n]，兼容转录里缺失右括号的 \c[n。
var mabelCodeRe = regexp.MustCompile(`\\c\[[0-9]*\]?`)

// mabelLibrary 懒解析台词库（进程内一次）。
func mabelLibrary() []string {
	mabelOnce.Do(func() { mabelGroups = parseMabel(mabelRaw) })
	return mabelGroups
}

// parseMabel 把台词库按空行切成组：每个非空行块 = 一段台词（可能多行）。
// 逐行右裁空白，组内用 \n 连接，最后清掉控制码与转义残留。
func parseMabel(raw string) []string {
	raw = strings.TrimPrefix(raw, "\ufeff")
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	var groups []string
	var cur []string
	flush := func() {
		if len(cur) == 0 {
			return
		}
		text := cleanMabel(strings.Join(cur, "\n"))
		if text != "" {
			groups = append(groups, text)
		}
		cur = nil
	}
	for _, line := range strings.Split(raw, "\n") {
		if strings.TrimSpace(line) == "" {
			flush()
			continue
		}
		cur = append(cur, strings.TrimRight(line, " \t"))
	}
	flush()
	return groups
}

// cleanMabel 清掉转录残留：控制码 \c[n]、转义叹号 \!。
func cleanMabel(s string) string {
	s = mabelCodeRe.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, `\!`, "!")
	return strings.TrimSpace(s)
}
