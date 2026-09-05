// 文件：describer-go/text/structure.go —— 结构字段：title-line/headings/structure + 结构量化计数（table/list/code/quote/checkbox/indent）
// 修改：2026-09-05（日期由 fresh-header.ps1 刷新）

// Package text 内的 structure.go：cod-text 结构类字段。
// 字段字典见 docs/元数据字段说明.md 第 4.3 节结构部分（title-line/headings/structure）
// 与第 4.3.2 节结构量化（计数版）。
package text

import (
	"regexp"
	"strings"

	"github.com/Reisentyann/Mabel-s-Tentacles/describer-go"
)

var (
	reHeadingMD  = regexp.MustCompile(`^#{1,6}\s+\S`)
	reHeadingCN  = regexp.MustCompile(`^\s*(第|卷|序章|尾声)[\s]{0,2}\S{0,20}(章|节|回|卷)`)
	reHeadingCh  = regexp.MustCompile(`(?i)^\s*chapter\s+\w+`)
	reHeadingNum = regexp.MustCompile(`^\s*\d{1,3}[.、]\s*\S`)
	reListItem   = regexp.MustCompile(`^\s*([-*+]|\d{1,2}[.)、])\s+\S`)
	reTableRow   = regexp.MustCompile(`^\s*\|.*\|`)
	reSpace      = regexp.MustCompile(`^\s*$`)

	reOrderedItem = regexp.MustCompile(`^\s*\d{1,2}[.)、]\s+\S`)
	reCheckbox    = regexp.MustCompile(`^\s*[-*+]\s+\[[ xX]\]\s*\S`)
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

func extractTitleLine(ctx *textCtx) any {
	if t := titleLine(ctx.lines); t != "" {
		return t
	}
	return nil
}

func extractHeadings(ctx *textCtx) any {
	hs := headings(ctx.lines)
	if len(hs) == 0 {
		return nil
	}
	return hs
}

func extractStructure(ctx *textCtx) any {
	st := structure(ctx.lines)
	if len(st) == 0 {
		return nil
	}
	return st
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

// quantCounts 结构量化计数（字段字典 4.3.2）。纯计数 0 也是事实（0 也产键）。
type quantCounts struct {
	tableBlocks   int
	tableRows     int
	tableCols     int
	listItems     int
	listNest      int
	orderedN      int
	codeLines     int
	quoteLines    int
	checkboxes    int
	indentMax     int
	trailingSpace int // 行尾空白行（脏文件/lint 指纹）
	blankRunMax   int // 最大连续空行数
	tabIndentN    int // tab 缩进行数
	spaceIndentN  int // 空格缩进行数
}

// countQuant 一次行遍历产出全部结构量化计数。
// fence 围栏内的行只计入 codeLines，不参与其他结构计数。
func countQuant(lines []string) *quantCounts {
	q := &quantCounts{}
	inTable := false
	inFence := false
	blankRun := 0
	for _, raw := range lines {
		l := strings.TrimRight(raw, "\r")
		trimmed := strings.TrimSpace(l)

		// 行尾空白 / 连续空行 / 缩进风格（fence 内外都算：物理行事实）
		if trimmed == "" {
			blankRun++
			if blankRun > q.blankRunMax {
				q.blankRunMax = blankRun
			}
		} else {
			blankRun = 0
			if strings.HasSuffix(l, " ") || strings.HasSuffix(l, "\t") {
				q.trailingSpace++
			}
			switch l[0] {
			case '\t':
				q.tabIndentN++
			case ' ':
				q.spaceIndentN++
			}
		}

		if strings.HasPrefix(trimmed, "```") {
			inFence = !inFence
			continue // 围栏行本身不计
		}
		if inFence {
			q.codeLines++
			continue
		}

		if reTableRow.MatchString(l) {
			q.tableRows++
			if !inTable {
				q.tableBlocks++
			}
			if cols := tableCols(l); cols > q.tableCols {
				q.tableCols = cols
			}
			inTable = true
		} else {
			inTable = false
		}

		if strings.HasPrefix(trimmed, ">") {
			q.quoteLines++
		}

		if reListItem.MatchString(l) {
			q.listItems++
			if reOrderedItem.MatchString(l) {
				q.orderedN++
			}
			if lv := indentLevel(l) + 1; lv > q.listNest {
				q.listNest = lv
			}
		}

		if reCheckbox.MatchString(l) {
			q.checkboxes++
		}

		if trimmed != "" {
			if lv := indentLevel(l); lv > q.indentMax {
				q.indentMax = lv
			}
		}
	}
	return q
}

// tableCols 表格行列数：剥首尾 | 后按 | 分割。
func tableCols(line string) int {
	s := strings.TrimSpace(line)
	s = strings.TrimPrefix(s, "|")
	s = strings.TrimSuffix(s, "|")
	return len(strings.Split(s, "|"))
}

// indentLevel 缩进层级：行首空白量（空格 1、tab 2）÷ 2 取整。
func indentLevel(line string) int {
	n := 0
	for _, r := range line {
		switch r {
		case ' ':
			n++
		case '\t':
			n += 2
		default:
			return n / 2
		}
	}
	return n / 2
}

func extractTableBlocks(ctx *textCtx) any    { return ctx.Quant().tableBlocks }
func extractTableRows(ctx *textCtx) any      { return ctx.Quant().tableRows }
func extractTableCols(ctx *textCtx) any      { return ctx.Quant().tableCols }
func extractListItems(ctx *textCtx) any      { return ctx.Quant().listItems }
func extractListNestingMax(ctx *textCtx) any { return ctx.Quant().listNest }
func extractCodeLines(ctx *textCtx) any      { return ctx.Quant().codeLines }
func extractQuoteLines(ctx *textCtx) any     { return ctx.Quant().quoteLines }
func extractCheckboxes(ctx *textCtx) any     { return ctx.Quant().checkboxes }
func extractIndentMax(ctx *textCtx) any      { return ctx.Quant().indentMax }
func extractTrailingSpaceLines(ctx *textCtx) any {
	return ctx.Quant().trailingSpace
}
func extractConsecutiveBlankMax(ctx *textCtx) any {
	return ctx.Quant().blankRunMax
}

// extractIndentStyle 缩进风格：tab / space / mixed / none（无缩进行）。
func extractIndentStyle(ctx *textCtx) any {
	q := ctx.Quant()
	switch {
	case q.tabIndentN > 0 && q.spaceIndentN > 0:
		return "mixed"
	case q.tabIndentN > 0:
		return "tab"
	case q.spaceIndentN > 0:
		return "space"
	default:
		return "none"
	}
}

func extractOrderedRatio(ctx *textCtx) any {
	q := ctx.Quant()
	if q.listItems == 0 {
		return nil
	}
	return describer.Round2(float64(q.orderedN) / float64(q.listItems))
}
