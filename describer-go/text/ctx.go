// 文件：describer-go/text/ctx.go —— textCtx 共享上下文：解码/分行/切 rune 一次构造 + Quant/FP 惰性预计算
// 修改：2026-09-03（日期由 fresh-header.ps1 刷新）

// Package text 内的 ctx.go 定义文本分析的共享上下文。
// 字段字典见 docs/元数据字段说明.md 第 4.3 节。
package text

import (
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"

	"github.com/Reisentyann/Mabel-s-Tentacles/describer-go"
)

// textCtx 是一次文本分析的共享中间产物。buildCtx 只构造一次，
// 所有 extractor 读取同一个 ctx，避免每个字段重复解码/分行/切 rune。
// Quant()/FP() 为惰性预计算（Analyze 单线程内使用，无需加锁）：
// 结构量化与行文指纹两组计数各自只在首次访问时遍历一次。
// 这是 cod-text 家族"共享一次全量加载"的落地点。
type textCtx struct {
	decoded   string       // 解码后的 UTF-8 文本
	encoding  string       // utf-8 / gbk / latin-1
	lines     []string     // 按 \n 分行（\r 保留，由 extractor 自行 TrimRight）
	runes     []rune       // decoded 的 rune 切片
	cjk       int          // CJK 字符数
	kana      int          // 假名数（日语标志）
	latin     int          // 拉丁字母数
	hasCJK    bool         // cjk 占比 > 5%
	hasLatin  bool         // latin 字母占比 > 20%
	hasBOM    bool         // 头三字节 EF BB BF
	quant     *quantCounts // 惰性缓存（Quant）
	quantDone bool
	fp        *fpCounts // 惰性缓存（FP）
	fpDone    bool
}

// Quant 返回结构量化计数（4.3.2），首次调用触发一次行遍历。
func (c *textCtx) Quant() *quantCounts {
	if !c.quantDone {
		c.quantDone = true
		c.quant = countQuant(c.lines)
	}
	return c.quant
}

// FP 返回行文指纹计数（4.3.3），首次调用触发一遍 rune + 一遍行 + 正则扫描。
func (c *textCtx) FP() *fpCounts {
	if !c.fpDone {
		c.fpDone = true
		c.fp = countFP(c)
	}
	return c.fp
}

// buildCtx 解码 + 分行 + 切 rune + 字符分类计数，一次构造全 extractor 共享。
func buildCtx(full []byte) *textCtx {
	decoded, enc := decodeText(full)
	lines := strings.Split(decoded, "\n")
	runes := []rune(decoded)
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
	return &textCtx{
		decoded:  decoded,
		encoding: enc,
		lines:    lines,
		runes:    runes,
		cjk:      cjk,
		kana:     kana,
		latin:    latin,
		hasCJK:   rn > 0 && float64(cjk)/rn > 0.05,
		hasLatin: rn > 0 && float64(latin)/rn > 0.2,
		hasBOM:   len(full) >= 3 && full[0] == 0xEF && full[1] == 0xBB && full[2] == 0xBF,
	}
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
