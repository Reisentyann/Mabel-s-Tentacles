// 文件：describer-go/text/fingerprint.go —— 行文指纹字段：行长/最长行/minified/数字/时间戳/时间戳行占比/括号配平/重复/段落/句子/对话/标点/多样性/实体计数/eol/bom
// 修改：2026-09-05（日期由 fresh-header.ps1 刷新）

// Package text 内的 fingerprint.go：cod-text 行文指纹字段（文体侧写，纯统计）。
// 字段字典见 docs/元数据字段说明.md 第 4.3.3 节。
// 全部共享 textCtx（decoded/lines/runes），一遍 rune + 一遍行 + 正则扫描。
package text

import (
	"math"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/Reisentyann/Mabel-s-Tentacles/describer-go"
)

// fpCounts 行文指纹计数（4.3.3）。spanOK/paraOK/sentOK/bracketOK=false 时对应键不产。
type fpCounts struct {
	avgLineLen    float64
	lineLenStd    float64
	longestLine   int
	effLines      int // 有效行数（末尾伪影空行除外），minified 判定用
	digitRatio    float64
	tsCount       int
	tsSpan        int64
	spanOK        bool
	tsLineRatio   float64
	tsLineOK      bool
	repeatRatio   float64
	paragraphs    int
	avgParaLen    float64
	paraOK        bool
	sentences     int
	avgSentLen    float64
	sentOK        bool
	dialogRatio   float64
	punctDensity  float64
	charDiversity float64
	bracketBal    float64
	bracketOK     bool
	urlN          int
	emailN        int
	mentionN      int
	hashtagN      int
	emojiN        int
	linkN         int
	eol           string
}

var (
	reURL       = regexp.MustCompile(`https?://[^\s)>\]]+`)
	reEmail     = regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`)
	reMention   = regexp.MustCompile(`(?:^|\s)@[^\s]+`)
	reHashtag   = regexp.MustCompile(`(?:^|\s)#[^\s#]+`)
	reLink      = regexp.MustCompile(`\[[^\]]*\]\([^)]+\)`)
	reTimestamp = regexp.MustCompile(`\d{4}-\d{2}-\d{2}(?:[T ]\d{2}:\d{2}(?::\d{2})?)?`)
	reTsLine    = regexp.MustCompile(`^\s*\d{4}-\d{2}-\d{2}`)
)

var tsLayouts = []string{
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05",
	"2006-01-02T15:04",
	"2006-01-02 15:04",
	"2006-01-02",
}

// countFP 产出全部行文指纹计数（一遍 rune 遍历 + 一遍行遍历 + 正则扫描）。
func countFP(c *textCtx) *fpCounts {
	fp := &fpCounts{}
	decoded, runes := c.decoded, c.runes
	rn := float64(len(runes))

	// —— rune 遍历：数字 / 标点 / 字符多样性 / 句数 / 对话 / emoji / 括号配平 ——
	distinct := map[rune]struct{}{}
	digitN, punctN, dialogChars, emojiN := 0, 0, 0, 0
	sents, prevEnd := 0, false
	inDialog := false
	var parenO, parenC, braceO, braceC, brackO, brackC int
	for _, r := range runes {
		distinct[r] = struct{}{}
		if unicode.IsDigit(r) {
			digitN++
		}
		if unicode.IsPunct(r) {
			punctN++
		}
		if isEmoji(r) {
			emojiN++
		}
		if isSentenceEnd(r) {
			if !prevEnd {
				sents++
			}
			prevEnd = true
		} else {
			prevEnd = false
		}
		switch r {
		case '“', '「', '『', '‘':
			inDialog = true
		case '”', '」', '』', '’':
			inDialog = false
		case '(':
			parenO++
		case ')':
			parenC++
		case '{':
			braceO++
		case '}':
			braceC++
		case '[':
			brackO++
		case ']':
			brackC++
		default:
			if inDialog {
				dialogChars++
			}
		}
	}
	fp.emojiN = emojiN
	fp.sentences = sents
	if total := parenO + parenC + braceO + braceC + brackO + brackC; total > 0 {
		paired := min(parenO, parenC) + min(braceO, braceC) + min(brackO, brackC)
		fp.bracketOK = true
		fp.bracketBal = describer.Round2(2 * float64(paired) / float64(total))
	}
	if rn > 0 {
		fp.digitRatio = describer.Round2(float64(digitN) / rn)
		fp.punctDensity = describer.Round2(float64(punctN) / rn)
		fp.charDiversity = describer.Round2(float64(len(distinct)) / rn)
		fp.dialogRatio = describer.Round2(float64(dialogChars) / rn)
	}

	// —— 行遍历：行长统计 ——
	// 末尾空段是文件以 \n 结尾时 Split 的伪影，不是真实行，跳过。
	if len(c.lines) > 0 {
		var sum, sumSq float64
		n := 0
		for i, raw := range c.lines {
			l := strings.TrimRight(raw, "\r")
			if i == len(c.lines)-1 && strings.TrimSpace(l) == "" {
				continue
			}
			w := len([]rune(l))
			if w > fp.longestLine {
				fp.longestLine = w
			}
			sum += float64(w)
			sumSq += float64(w) * float64(w)
			n++
		}
		fp.effLines = n
		if n > 0 {
			mean := sum / float64(n)
			fp.avgLineLen = describer.Round2(mean)
			fp.lineLenStd = describer.Round2(math.Sqrt(math.Max(0, sumSq/float64(n)-mean*mean)))
		}
	}

	// —— 行遍历：重复行（非空行中出现 ≥2 次的行占比）——
	nonBlank := 0
	freq := map[string]int{}
	for _, raw := range c.lines {
		l := strings.TrimRight(raw, "\r")
		if strings.TrimSpace(l) == "" {
			continue
		}
		nonBlank++
		freq[l]++
	}
	if nonBlank > 0 {
		repeated := 0
		for _, n := range freq {
			if n >= 2 {
				repeated += n
			}
		}
		fp.repeatRatio = describer.Round2(float64(repeated) / float64(nonBlank))
	}

	// —— 行遍历：段落（空行分隔的非空行连续段）——
	paraN, paraChars, inPara := 0, 0, false
	for _, raw := range c.lines {
		if strings.TrimSpace(strings.TrimRight(raw, "\r")) == "" {
			inPara = false
			continue
		}
		if !inPara {
			paraN++
			inPara = true
		}
		paraChars += len([]rune(strings.TrimRight(raw, "\r")))
	}
	fp.paragraphs = paraN
	if paraN > 0 {
		fp.paraOK = true
		fp.avgParaLen = describer.Round2(float64(paraChars) / float64(paraN))
	}
	if sents > 0 {
		fp.sentOK = true
		fp.avgSentLen = describer.Round2(rn / float64(sents))
	}

	// —— 正则扫描：实体计数 ——
	fp.urlN = len(reURL.FindAllString(decoded, -1))
	fp.emailN = len(reEmail.FindAllString(decoded, -1))
	fp.mentionN = len(reMention.FindAllString(decoded, -1))
	fp.hashtagN = len(reHashtag.FindAllString(decoded, -1))
	fp.linkN = len(reLink.FindAllString(decoded, -1))

	// —— 时间戳：计数 + 最早-最晚跨度 ——
	matches := reTimestamp.FindAllString(decoded, -1)
	fp.tsCount = len(matches)
	var minT, maxT time.Time
	for _, m := range matches {
		for _, layout := range tsLayouts {
			if t, err := time.Parse(layout, m); err == nil {
				if minT.IsZero() || t.Before(minT) {
					minT = t
				}
				if maxT.IsZero() || t.After(maxT) {
					maxT = t
				}
				break
			}
		}
	}
	if !minT.IsZero() && !maxT.IsZero() {
		fp.spanOK = true
		fp.tsSpan = maxT.Unix() - minT.Unix()
	}

	// —— 时间戳行占比（日志规整度指纹）——
	tsLines := 0
	for _, raw := range c.lines {
		if reTsLine.MatchString(strings.TrimRight(raw, "\r")) {
			tsLines++
		}
	}
	if fp.effLines > 0 {
		fp.tsLineOK = true
		fp.tsLineRatio = describer.Round2(float64(tsLines) / float64(fp.effLines))
	}

	// —— 换行符类型 ——
	crlf := strings.Count(decoded, "\r\n")
	lf := strings.Count(decoded, "\n") - crlf
	switch {
	case crlf > 0 && lf > 0:
		fp.eol = "mixed"
	case crlf > 0:
		fp.eol = "crlf"
	case lf > 0:
		fp.eol = "lf"
	default:
		fp.eol = "none"
	}

	return fp
}

// isSentenceEnd 句末字符（。！？.!?，连续句末算 1 句）。
func isSentenceEnd(r rune) bool {
	switch r {
	case '。', '！', '？', '.', '!', '?':
		return true
	}
	return false
}

// isEmoji 常用 emoji 区段（字段字典 4.3.3 口径）。
func isEmoji(r rune) bool {
	return (r >= 0x1F300 && r <= 0x1FAFF) || // 符号与象形（含补充与扩展）
		(r >= 0x2600 && r <= 0x27BF) || // 杂项符号 / 装饰符号
		(r >= 0x1F1E6 && r <= 0x1F1FF) // 区域指示符（旗帜）
}

func extractAvgLineLen(ctx *textCtx) any      { return ctx.FP().avgLineLen }
func extractLineLenStd(ctx *textCtx) any      { return ctx.FP().lineLenStd }
func extractDigitRatio(ctx *textCtx) any      { return ctx.FP().digitRatio }
func extractTimestampCount(ctx *textCtx) any  { return ctx.FP().tsCount }
func extractRepeatLineRatio(ctx *textCtx) any { return ctx.FP().repeatRatio }
func extractParagraphs(ctx *textCtx) any      { return ctx.FP().paragraphs }
func extractSentences(ctx *textCtx) any       { return ctx.FP().sentences }
func extractDialogRatio(ctx *textCtx) any     { return ctx.FP().dialogRatio }
func extractPunctDensity(ctx *textCtx) any    { return ctx.FP().punctDensity }
func extractCharDiversity(ctx *textCtx) any   { return ctx.FP().charDiversity }
func extractURLCount(ctx *textCtx) any        { return ctx.FP().urlN }
func extractEmailCount(ctx *textCtx) any      { return ctx.FP().emailN }
func extractMentionCount(ctx *textCtx) any    { return ctx.FP().mentionN }
func extractHashtagCount(ctx *textCtx) any    { return ctx.FP().hashtagN }
func extractEmojiCount(ctx *textCtx) any      { return ctx.FP().emojiN }
func extractLinkCount(ctx *textCtx) any       { return ctx.FP().linkN }
func extractEOL(ctx *textCtx) any             { return ctx.FP().eol }
func extractHasBOM(ctx *textCtx) any          { return ctx.hasBOM }

func extractTimeSpan(ctx *textCtx) any {
	fp := ctx.FP()
	if !fp.spanOK {
		return nil
	}
	return fp.tsSpan
}

func extractAvgParaLen(ctx *textCtx) any {
	fp := ctx.FP()
	if !fp.paraOK {
		return nil
	}
	return fp.avgParaLen
}

func extractAvgSentLen(ctx *textCtx) any {
	fp := ctx.FP()
	if !fp.sentOK {
		return nil
	}
	return fp.avgSentLen
}

func extractLongestLine(ctx *textCtx) any { return ctx.FP().longestLine }

// extractMinified 压缩单行文件判定：有效行数 ≤3 且最长行 >500 rune
// （minified js/json / data URI dump；纯计数口径，确定性）。
func extractMinified(ctx *textCtx) any {
	fp := ctx.FP()
	return fp.effLines > 0 && fp.effLines <= 3 && fp.longestLine > 500
}

func extractTimestampLineRatio(ctx *textCtx) any {
	fp := ctx.FP()
	if !fp.tsLineOK {
		return nil
	}
	return fp.tsLineRatio
}

// extractBracketBalance 括号配平率：Σmin(开,闭)/Σ(开+闭)×2，全配平=1；
// 无任何括号不产键（1.0 与"无括号"不可区分时宁可缺失）。
func extractBracketBalance(ctx *textCtx) any {
	fp := ctx.FP()
	if !fp.bracketOK {
		return nil
	}
	return fp.bracketBal
}
