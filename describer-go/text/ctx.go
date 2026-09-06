// 文件：describer-go/text/ctx.go —— textCtx 共享上下文：解码/分行/切 rune 一次构造 + Quant/FP 惰性预计算
// 修改：2026-09-05（日期由 fresh-header.ps1 刷新）

// Package text 内的 ctx.go 定义文本分析的共享上下文。
// 字段字典见 docs/元数据字段说明.md 第 4.3 节。
package text

import (
	"bytes"
	"strings"
	"unicode/utf16"
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
	upper     int          // 大写拉丁字母数（upper-ratio 用）
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
	cjk, kana, latin, upper := 0, 0, 0, 0
	for _, r := range runes {
		switch {
		case describer.IsCJK(r):
			cjk++
		case describer.IsKana(r):
			kana++
		case r >= 'A' && r <= 'Z':
			latin++
			upper++
		case r >= 'a' && r <= 'z':
			latin++
		}
	}
	rn := float64(len(runes))
	_, bom16 := describer.UTF16BOM(full)
	hasBOM := (len(full) >= 3 && full[0] == 0xEF && full[1] == 0xBB && full[2] == 0xBF) || bom16
	return &textCtx{
		decoded:  decoded,
		encoding: enc,
		lines:    lines,
		runes:    runes,
		cjk:      cjk,
		kana:     kana,
		latin:    latin,
		upper:    upper,
		hasCJK:   rn > 0 && float64(cjk)/rn > 0.05,
		hasLatin: rn > 0 && float64(latin)/rn > 0.2,
		hasBOM:   hasBOM,
	}
}

// decodeText 解码：带 BOM 的 UTF-16 → 严格 UTF-8 → GBK（零替换符才认领）→ latin-1 兜底。
// FF FE / FE FF 本就是非法 UTF-8 序列，先判 UTF-16 可免被 GBK 误收。
func decodeText(b []byte) (string, string) {
	if little, ok := describer.UTF16BOM(b); ok {
		if little {
			return decodeUTF16(b[2:], true), "utf-16le"
		}
		return decodeUTF16(b[2:], false), "utf-16be"
	}
	if utf8.Valid(b) {
		return string(b), "utf-8"
	}
	// GBK 只在整段干净解码时才认领：x/text 对坏序列常以 U+FFFD 代替而非
	// 报错，不查会把乱码误标成 gbk（此时 latin-1 兜底更诚实）。
	if dec, err := simplifiedchinese.GBK.NewDecoder().Bytes(b); err == nil &&
		utf8.Valid(dec) && !bytes.ContainsRune(dec, utf8.RuneError) {
		return string(dec), "gbk"
	}
	// latin-1：每字节直接映射 U+0000-U+00FF
	r := make([]rune, len(b))
	for i, x := range b {
		r[i] = rune(x)
	}
	return string(r), "latin-1"
}

// decodeUTF16 解码 UTF-16（BOM 已剥）：按字节序组 uint16 再经 utf16.Decode
// 处理代理对；奇尾字节（截断的半字）忽略。
func decodeUTF16(b []byte, little bool) string {
	us := make([]uint16, 0, len(b)/2)
	for i := 0; i+1 < len(b); i += 2 {
		if little {
			us = append(us, uint16(b[i])|uint16(b[i+1])<<8)
		} else {
			us = append(us, uint16(b[i+1])|uint16(b[i])<<8)
		}
	}
	return string(utf16.Decode(us))
}
