package text

import (
	"strings"
	"testing"

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
