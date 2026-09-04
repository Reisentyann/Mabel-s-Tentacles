// 文件：describer-go/text/text_test.go —— cod-text 单元测试：zh/gbk/en/frontmatter + 量化计数 + 指纹 + 时间戳 + eol/bom + P2 字段
// 修改：2026-09-04（日期由 fresh-header.ps1 刷新）

package text

import (
	"strings"
	"testing"
	"unicode/utf16"

	"golang.org/x/text/encoding/simplifiedchinese"

	"github.com/Reisentyann/Mabel-s-Tentacles/describer-go"
)

const zhSample = `# 深夜来电

梅贝尔整理着书架上的旧文件，铃仙在旁边做记录。
触手轻轻卷起一张地图。

- 项目一
- 项目二
- 项目三

数据与文件都井井有条。`

func zeroInput() describer.Input {
	return describer.Input{Path: "sample.txt"}
}

func TestAnalyzeZh(t *testing.T) {
	d := descriptor{}
	attrs, _ := d.Analyze(zeroInput(), []byte(zhSample))

	if attrs["cod-text-encoding"] != "utf-8" {
		t.Fatalf("encoding = %v", attrs["cod-text-encoding"])
	}
	if attrs["cod-text-language"] != "zh" {
		t.Fatalf("language = %v, want zh", attrs["cod-text-language"])
	}
	lines := attrs["cod-text-lines"].(int)
	if lines != strings.Count(zhSample, "\n")+1 {
		t.Fatalf("lines = %d", lines)
	}
	hs := attrs["cod-text-headings"].([]string)
	if len(hs) == 0 || hs[0] != "深夜来电" {
		t.Fatalf("headings = %#v, want first 深夜来电", hs)
	}
	if attrs["cod-text-title-line"] != "深夜来电" {
		t.Fatalf("title-line = %v", attrs["cod-text-title-line"])
	}
	st := attrs["cod-text-structure"].([]string)
	hasList, hasHeading := false, false
	for _, s := range st {
		if s == "lists" {
			hasList = true
		}
		if s == "headings" {
			hasHeading = true
		}
	}
	if !hasList || !hasHeading {
		t.Fatalf("structure = %#v, want lists+headings", st)
	}
	ratio := attrs["cod-text-cjk-ratio"].(float64)
	if ratio <= 0.3 || ratio >= 1 {
		t.Fatalf("cjk-ratio = %v, want (0.3,1)", ratio)
	}
	if _, ok := attrs["cod-text-top-keywords"]; !ok {
		t.Fatal("zh keywords should exist (梅贝尔/铃仙 bigrams)")
	}
}

func TestAnalyzeGBK(t *testing.T) {
	src := "中文测试内容，中文测试内容。"
	gbk, err := simplifiedchinese.GBK.NewEncoder().Bytes([]byte(src))
	if err != nil {
		t.Fatalf("encode gbk: %v", err)
	}
	d := descriptor{}
	attrs, _ := d.Analyze(zeroInput(), gbk)
	if attrs["cod-text-encoding"] != "gbk" {
		t.Fatalf("encoding = %v, want gbk", attrs["cod-text-encoding"])
	}
	if attrs["cod-text-chars"].(int) != len([]rune(src)) {
		t.Fatalf("chars = %v, want %d", attrs["cod-text-chars"], len([]rune(src)))
	}
	if attrs["cod-text-language"] != "zh" {
		t.Fatalf("language = %v, want zh (decoded)", attrs["cod-text-language"])
	}
}

func TestAnalyzeEn(t *testing.T) {
	src := "the quick brown fox jumps over the lazy dog\n" +
		"the fox and the dog run again and again\n\nbinary data here\n"
	d := descriptor{}
	attrs, _ := d.Analyze(zeroInput(), []byte(src))
	if attrs["cod-text-language"] != "en" {
		t.Fatalf("language = %v, want en", attrs["cod-text-language"])
	}
	kw := attrs["cod-text-top-keywords"].([]string)
	if len(kw) == 0 {
		t.Fatal("en keywords should exist")
	}
	for _, w := range kw {
		switch w {
		case "the", "and", "over":
			t.Fatalf("stopword leaked: %s", w)
		}
	}
	if attrs["cod-text-blank-ratio"].(float64) <= 0 {
		t.Fatal("blank ratio should be > 0")
	}
}

func TestAnalyzeBadGBKFallsToLatin1(t *testing.T) {
	// 坏 GBK（0x81 领字节 + 无效尾字节 0x30）：x/text 以 U+FFFD 代替而非
	// 报错，零替换符校验须把它打回 latin-1 兜底，不得误标 gbk。
	d := descriptor{}
	attrs, _ := d.Analyze(zeroInput(), []byte("hello \x81\x30 world"))
	if attrs["cod-text-encoding"] != "latin-1" {
		t.Fatalf("encoding = %v, want latin-1", attrs["cod-text-encoding"])
	}
	if attrs["cod-text-language"] != "en" {
		t.Fatalf("language = %v, want en", attrs["cod-text-language"])
	}
}

// utf16Bytes 构造带 BOM 的 UTF-16 字节（代理对经 utf16.Encode 展开）。
func utf16Bytes(s string, little bool) []byte {
	out := []byte{0xFE, 0xFF}
	if little {
		out = []byte{0xFF, 0xFE}
	}
	for _, u := range utf16.Encode([]rune(s)) {
		if little {
			out = append(out, byte(u), byte(u>>8))
		} else {
			out = append(out, byte(u>>8), byte(u))
		}
	}
	return out
}

func TestAnalyzeUTF16(t *testing.T) {
	src := "# UTF-16 标题\n\n梅贝尔整理文件。\n"
	d := descriptor{}
	for _, tc := range []struct {
		little bool
		enc    string
	}{
		{true, "utf-16le"},
		{false, "utf-16be"},
	} {
		attrs, _ := d.Analyze(zeroInput(), utf16Bytes(src, tc.little))
		if attrs["cod-text-encoding"] != tc.enc {
			t.Fatalf("encoding = %v, want %s", attrs["cod-text-encoding"], tc.enc)
		}
		if attrs["cod-text-language"] != "zh" {
			t.Fatalf("language = %v, want zh (decoded)", attrs["cod-text-language"])
		}
		if attrs["cod-text-has-bom"] != true {
			t.Fatalf("has-bom = %v, want true", attrs["cod-text-has-bom"])
		}
		if attrs["cod-text-title-line"] != "UTF-16 标题" {
			t.Fatalf("title-line = %v", attrs["cod-text-title-line"])
		}
		if attrs["cod-text-eol"] != "lf" {
			t.Fatalf("eol = %v, want lf (decoded)", attrs["cod-text-eol"])
		}
		if attrs["cod-text-lines"].(int) != 4 {
			t.Fatalf("lines = %v, want 4 (decoded)", attrs["cod-text-lines"])
		}
	}
}

func TestFrontmatter(t *testing.T) {
	src := "---\ntitle: hi\n---\n\n正文"
	d := descriptor{}
	attrs, _ := d.Analyze(zeroInput(), []byte(src))
	st := attrs["cod-text-structure"].([]string)
	found := false
	for _, s := range st {
		if s == "frontmatter" {
			found = true
		}
	}
	if !found {
		t.Fatalf("frontmatter not detected: %#v", st)
	}
}

func TestQuantCounts(t *testing.T) {
	src := "# 标题\n\n" +
		"- 项一\n- 项二\n  - 嵌套项\n1. 有序项\n\n" +
		"| a | b |\n|---|---|\n| 1 | 2 |\n\n" +
		"> 引用行\n\n" +
		"- [ ] 待办\n- [x] 完成\n\n" +
		"```\nline1\nline2\n```\n"
	d := descriptor{}
	attrs, _ := d.Analyze(zeroInput(), []byte(src))

	if attrs["cod-text-table-blocks"].(int) != 1 {
		t.Fatalf("table-blocks = %v", attrs["cod-text-table-blocks"])
	}
	if attrs["cod-text-table-rows"].(int) != 3 {
		t.Fatalf("table-rows = %v", attrs["cod-text-table-rows"])
	}
	if attrs["cod-text-table-cols"].(int) != 2 {
		t.Fatalf("table-cols = %v", attrs["cod-text-table-cols"])
	}
	if attrs["cod-text-list-items"].(int) != 6 {
		t.Fatalf("list-items = %v, want 6", attrs["cod-text-list-items"])
	}
	if attrs["cod-text-list-nesting-max"].(int) != 2 {
		t.Fatalf("list-nesting-max = %v", attrs["cod-text-list-nesting-max"])
	}
	if attrs["cod-text-ordered-ratio"].(float64) != 0.17 {
		t.Fatalf("ordered-ratio = %v, want 0.17", attrs["cod-text-ordered-ratio"])
	}
	if attrs["cod-text-code-lines"].(int) != 2 {
		t.Fatalf("code-lines = %v", attrs["cod-text-code-lines"])
	}
	if attrs["cod-text-quote-lines"].(int) != 1 {
		t.Fatalf("quote-lines = %v", attrs["cod-text-quote-lines"])
	}
	if attrs["cod-text-checkboxes"].(int) != 2 {
		t.Fatalf("checkboxes = %v", attrs["cod-text-checkboxes"])
	}
	if attrs["cod-text-indent-max"].(int) != 1 {
		t.Fatalf("indent-max = %v", attrs["cod-text-indent-max"])
	}
}

func TestFingerprint(t *testing.T) {
	src := "第一段。第二句！\n\n“对话内容”叙述。\n复读行\n复读行\n"
	d := descriptor{}
	attrs, _ := d.Analyze(zeroInput(), []byte(src))

	if attrs["cod-text-sentences"].(int) != 3 {
		t.Fatalf("sentences = %v, want 3", attrs["cod-text-sentences"])
	}
	if attrs["cod-text-dialog-ratio"].(float64) != 0.14 {
		t.Fatalf("dialog-ratio = %v, want 0.14", attrs["cod-text-dialog-ratio"])
	}
	if attrs["cod-text-repeat-line-ratio"].(float64) != 0.5 {
		t.Fatalf("repeat-line-ratio = %v, want 0.5", attrs["cod-text-repeat-line-ratio"])
	}
	if attrs["cod-text-paragraphs"].(int) != 2 {
		t.Fatalf("paragraphs = %v, want 2", attrs["cod-text-paragraphs"])
	}
	if attrs["cod-text-avg-para-len"].(float64) != 11.5 {
		t.Fatalf("avg-para-len = %v, want 11.5", attrs["cod-text-avg-para-len"])
	}
	if attrs["cod-text-eol"] != "lf" {
		t.Fatalf("eol = %v, want lf", attrs["cod-text-eol"])
	}
	if attrs["cod-text-has-bom"] != false {
		t.Fatalf("has-bom = %v", attrs["cod-text-has-bom"])
	}
	if attrs["cod-text-digit-ratio"].(float64) != 0 {
		t.Fatalf("digit-ratio = %v", attrs["cod-text-digit-ratio"])
	}
}

func TestTimestampSpan(t *testing.T) {
	src := "2026-08-27 23:02:02 start\n2026-08-27 23:05:12 end\n"
	d := descriptor{}
	attrs, _ := d.Analyze(zeroInput(), []byte(src))

	if attrs["cod-text-timestamp-count"].(int) != 2 {
		t.Fatalf("timestamp-count = %v", attrs["cod-text-timestamp-count"])
	}
	if attrs["cod-text-time-span"].(int64) != 190 {
		t.Fatalf("time-span = %v, want 190", attrs["cod-text-time-span"])
	}
}

func TestEOLAndBOM(t *testing.T) {
	d := descriptor{}

	attrs, _ := d.Analyze(zeroInput(), []byte("行一\r\n行二\r\n"))
	if attrs["cod-text-eol"] != "crlf" {
		t.Fatalf("eol = %v, want crlf", attrs["cod-text-eol"])
	}

	bom := append([]byte{0xEF, 0xBB, 0xBF}, []byte("BOM 文本")...)
	attrs, _ = d.Analyze(zeroInput(), bom)
	if attrs["cod-text-has-bom"] != true {
		t.Fatalf("has-bom = %v, want true", attrs["cod-text-has-bom"])
	}
}

func TestP2Minified(t *testing.T) {
	d := descriptor{}
	// 单行 600 字符 → minified
	attrs, _ := d.Analyze(zeroInput(), []byte(strings.Repeat("a", 600)+"\n"))
	if attrs["cod-text-minified"] != true {
		t.Fatalf("minified = %v, want true (1 行 600 字符)", attrs["cod-text-minified"])
	}
	if attrs["cod-text-longest-line"].(int) != 600 {
		t.Fatalf("longest-line = %v, want 600", attrs["cod-text-longest-line"])
	}
	// 普通多行短行 → 非 minified
	attrs, _ = d.Analyze(zeroInput(), []byte("line one\nline two\nline three\n"))
	if attrs["cod-text-minified"] != false {
		t.Fatalf("minified = %v, want false", attrs["cod-text-minified"])
	}
	// 行多但行长不够 → 非 minified
	attrs, _ = d.Analyze(zeroInput(), []byte(strings.Repeat("x", 600)+"\n"+strings.Repeat("y", 600)+"\n"+strings.Repeat("z", 600)+"\n"+strings.Repeat("w", 600)+"\n"))
	if attrs["cod-text-minified"] != false {
		t.Fatalf("minified = %v, want false (4 行超出 ≤3 口径)", attrs["cod-text-minified"])
	}
}

func TestP2Shebang(t *testing.T) {
	d := descriptor{}
	attrs, _ := d.Analyze(zeroInput(), []byte("#!/bin/bash\nls -la\n"))
	if attrs["cod-text-shebang"] != "#!/bin/bash" {
		t.Fatalf("shebang = %v", attrs["cod-text-shebang"])
	}
	attrs, _ = d.Analyze(zeroInput(), []byte("no script here\n"))
	if _, ok := attrs["cod-text-shebang"]; ok {
		t.Fatal("无 shebang 不应产键")
	}
}

func TestP2IndentStyle(t *testing.T) {
	d := descriptor{}
	cases := map[string]string{
		"def a():\n\treturn 1\n":      "tab",
		"if x:\n    y = 1\n":          "space",
		"plain\nlines\n":              "none",
		"if x:\n\tif y:\n        z\n": "mixed",
	}
	for src, want := range cases {
		attrs, _ := d.Analyze(zeroInput(), []byte(src))
		if got := attrs["cod-text-indent-style"]; got != want {
			t.Errorf("indent-style(%q) = %v, want %q", src, got, want)
		}
	}
}

func TestP2BracketBalance(t *testing.T) {
	d := descriptor{}
	cases := map[string]float64{
		"{} () []\n": 1.0,
		"() ((\n":    0.5,
		"{ (\n":      0.0,
	}
	for src, want := range cases {
		attrs, _ := d.Analyze(zeroInput(), []byte(src))
		if got := attrs["cod-text-bracket-balance"].(float64); got != want {
			t.Errorf("bracket-balance(%q) = %v, want %v", src, got, want)
		}
	}
	attrs, _ := d.Analyze(zeroInput(), []byte("无括号文本\n"))
	if _, ok := attrs["cod-text-bracket-balance"]; ok {
		t.Fatal("无括号不应产键")
	}
}

func TestP2TimestampLineRatio(t *testing.T) {
	d := descriptor{}
	src := "2026-01-01 10:00:00 event a\n2026-01-01 10:00:01 event b\nplain line\n"
	attrs, _ := d.Analyze(zeroInput(), []byte(src))
	if got := attrs["cod-text-timestamp-line-ratio"].(float64); got != 0.67 {
		t.Fatalf("timestamp-line-ratio = %v, want 0.67 (2/3)", got)
	}
}

func TestP2TextStats(t *testing.T) {
	d := descriptor{}

	attrs, _ := d.Analyze(zeroInput(), []byte("abc\n"))
	if attrs["cod-text-final-newline"] != true {
		t.Fatalf("final-newline = %v, want true", attrs["cod-text-final-newline"])
	}
	attrs, _ = d.Analyze(zeroInput(), []byte("abc"))
	if attrs["cod-text-final-newline"] != false {
		t.Fatalf("final-newline = %v, want false", attrs["cod-text-final-newline"])
	}

	// non-ascii-ratio：hello 世界 = 8 rune（含空格）中 2 个 CJK → 0.25
	attrs, _ = d.Analyze(zeroInput(), []byte("hello 世界"))
	if got := attrs["cod-text-non-ascii-ratio"].(float64); got != 0.25 {
		t.Fatalf("non-ascii-ratio = %v, want 0.25", got)
	}

	// upper-ratio：HELLO world = 10 字母中 5 大写 → 0.5
	attrs, _ = d.Analyze(zeroInput(), []byte("HELLO world"))
	if got := attrs["cod-text-upper-ratio"].(float64); got != 0.5 {
		t.Fatalf("upper-ratio = %v, want 0.5", got)
	}
	// 无拉丁字母不产
	attrs, _ = d.Analyze(zeroInput(), []byte("中文内容"))
	if _, ok := attrs["cod-text-upper-ratio"]; ok {
		t.Fatal("无拉丁字母不应产 upper-ratio")
	}

	// word-count：拉丁词 1 + CJK 4 = 5
	attrs, _ = d.Analyze(zeroInput(), []byte("hello 世界你好"))
	if got := attrs["cod-text-word-count"].(int); got != 5 {
		t.Fatalf("word-count = %v, want 5", got)
	}

	// avg-word-len：(5+2)/2 = 3.5
	attrs, _ = d.Analyze(zeroInput(), []byte("hello hi"))
	if got := attrs["cod-text-avg-word-len"].(float64); got != 3.5 {
		t.Fatalf("avg-word-len = %v, want 3.5", got)
	}

	// trailing-space-lines：1 行行尾空白
	attrs, _ = d.Analyze(zeroInput(), []byte("abc \ndef\n"))
	if got := attrs["cod-text-trailing-space-lines"].(int); got != 1 {
		t.Fatalf("trailing-space-lines = %v, want 1", got)
	}

	// consecutive-blank-max：3 连空行
	attrs, _ = d.Analyze(zeroInput(), []byte("a\n\n\n\nb\n"))
	if got := attrs["cod-text-consecutive-blank-max"].(int); got != 3 {
		t.Fatalf("consecutive-blank-max = %v, want 3", got)
	}
}
