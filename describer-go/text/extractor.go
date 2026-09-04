// 文件：describer-go/text/extractor.go —— cod-text 字段注册表：一个字段一个 extractor，注册序即产出序
// 修改：2026-09-04（日期由 fresh-header.ps1 刷新）

// Package text 内的 extractor.go 定义字段注册表：一个字段一个 extractor，
// 注册序即产出序（确定）。加字段不动 text.go 主编排，只动本文件 + 对应分类文件。
//
// 字段字典见 docs/元数据字段说明.md 第 4.3 节（4.3.1 基础统计 / 4.3.2 结构量化 /
// 4.3.3 行文指纹；P1 候选见第 4.7 节）。
package text

// extractor 是单个字段的提取器：一个函数对应一个字段。
//
//   - name：字段名（不含 cod-text- 前缀，引擎最终键为 cod-text-<name>）
//   - needs：路由，nil = 恒算；非 nil 返回 false 时该文件不产此字段
//   - extract：在共享 ctx 上算字段值；返回 nil 表示该文件不产此键
//
// 加字段标准动作（OCP，对扩展开放/对修改封闭）：
//  1. 在对应分类文件（stats.go/structure.go/fingerprint.go）写 extractXxx 函数
//  2. 在本文件 extractors 注册表对应分类块加一行
//  3. 字段字典 docs/元数据字段说明.md 对应小节加字段（候选字段则从 4.6/4.7 迁入 4.3）
//  4. test/测试规则.md 断言表加一条
//  5. FamilyVersion +1（版本号机制见字段字典第 10 节）
//
// 不动 text.go 主编排，不动其他分类文件。
type extractor struct {
	name    string
	needs   func(*textCtx) bool
	extract func(*textCtx) any
}

// extractors 是 cod-text 家族的字段注册表。
// 按分类分块，加新字段在对应块末尾追加一行即可。
var extractors = []extractor{
	// —— 基础统计（stats.go，4.3.1）——
	{name: "encoding", extract: extractEncoding},
	{name: "lines", extract: extractLines},
	{name: "chars", extract: extractChars},
	{name: "cjk-ratio", extract: extractCjkRatio},
	{name: "language", extract: extractLanguage},
	{name: "blank-ratio", extract: extractBlankRatio},
	{name: "top-keywords", extract: extractTopKeywords},
	{name: "shebang", extract: extractShebang},
	{name: "final-newline", extract: extractFinalNewline},
	{name: "non-ascii-ratio", extract: extractNonASCIIRatio},
	{name: "upper-ratio", extract: extractUpperRatio},
	{name: "word-count", extract: extractWordCount},
	{name: "avg-word-len", extract: extractAvgWordLen},

	// —— 结构（structure.go，4.3.1）——
	{name: "title-line", extract: extractTitleLine},
	{name: "headings", extract: extractHeadings},
	{name: "structure", extract: extractStructure},

	// —— 结构量化（structure.go，4.3.2）——
	{name: "table-blocks", extract: extractTableBlocks},
	{name: "table-rows", extract: extractTableRows},
	{name: "table-cols", extract: extractTableCols},
	{name: "list-items", extract: extractListItems},
	{name: "list-nesting-max", extract: extractListNestingMax},
	{name: "ordered-ratio", extract: extractOrderedRatio},
	{name: "code-lines", extract: extractCodeLines},
	{name: "quote-lines", extract: extractQuoteLines},
	{name: "checkboxes", extract: extractCheckboxes},
	{name: "indent-max", extract: extractIndentMax},
	{name: "trailing-space-lines", extract: extractTrailingSpaceLines},
	{name: "consecutive-blank-max", extract: extractConsecutiveBlankMax},
	{name: "indent-style", extract: extractIndentStyle},

	// —— 行文指纹（fingerprint.go，4.3.3）——
	{name: "avg-line-len", extract: extractAvgLineLen},
	{name: "line-len-std", extract: extractLineLenStd},
	{name: "digit-ratio", extract: extractDigitRatio},
	{name: "timestamp-count", extract: extractTimestampCount},
	{name: "time-span", extract: extractTimeSpan},
	{name: "repeat-line-ratio", extract: extractRepeatLineRatio},
	{name: "paragraphs", extract: extractParagraphs},
	{name: "avg-para-len", extract: extractAvgParaLen},
	{name: "sentences", extract: extractSentences},
	{name: "avg-sent-len", extract: extractAvgSentLen},
	{name: "dialog-ratio", extract: extractDialogRatio},
	{name: "punct-density", extract: extractPunctDensity},
	{name: "char-diversity", extract: extractCharDiversity},
	{name: "url-count", extract: extractURLCount},
	{name: "email-count", extract: extractEmailCount},
	{name: "mention-count", extract: extractMentionCount},
	{name: "hashtag-count", extract: extractHashtagCount},
	{name: "emoji-count", extract: extractEmojiCount},
	{name: "link-count", extract: extractLinkCount},
	{name: "eol", extract: extractEOL},
	{name: "has-bom", extract: extractHasBOM},
	{name: "longest-line", extract: extractLongestLine},
	{name: "minified", extract: extractMinified},
	{name: "timestamp-line-ratio", extract: extractTimestampLineRatio},
	{name: "bracket-balance", extract: extractBracketBalance},
}
