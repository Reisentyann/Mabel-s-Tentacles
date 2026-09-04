// 文件：describer-go/text/text.go —— cod-text 插件主编排：建 ctx + 遍历 extractor 注册表（加字段不碰这里）
// 修改：2026-09-04（日期由 fresh-header.ps1 刷新）

// Package text cod-text 插件：文本统计事实（无语义）。
// 字段字典见 docs/元数据字段说明.md 第 4.3 节。
//
// 字段实现采用注册表模式（extractors）：
//   - 一个 extractor 对应一个字段，注册序即产出序（确定）
//   - 所有 extractor 共享 buildCtx 产出的 textCtx（解码/分行/切 rune 只做一次）
//   - 加字段不动 Analyze 主编排，只动对应分类文件 + extractor.go 注册表
//
// 分类文件：ctx.go（共享上下文）/ extractor.go（注册表）/
// stats.go（基础统计）/ structure.go（结构）/ fingerprint.go（行文指纹，P0 待加）。
package text

import (
	"github.com/Reisentyann/Mabel-s-Tentacles/describer-go"
)

func init() { describer.Register(descriptor{}) }

type descriptor struct{}

func (descriptor) Family() string { return "text" }

// FamilyVersion=3：v2 增补 P0 结构量化（4.3.2）与行文指纹（4.3.3）；
// v3 解码链增 UTF-16（BOM 判定 LE/BE）、GBK 要求零替换符（坏序列落
// latin-1 兜底）、has-bom 口径扩展为任意 BOM（UTF-8 / UTF-16）。
// v4 增补 P2 字段 13 个（字段字典 4.3 各节末尾）：
// shebang/final-newline/non-ascii-ratio/upper-ratio/word-count/avg-word-len、
// trailing-space-lines/consecutive-blank-max/indent-style、
// longest-line/minified/timestamp-line-ratio/bracket-balance。
// 存量 cod-text-ver<4 由回填重算。
func (descriptor) FamilyVersion() int { return 4 }

func (descriptor) SPNamespaces() []string { return nil }
func (descriptor) Supports(_ string, _ []byte, b describer.Basic) bool {
	return b.Textish
}

// Analyze 建 ctx（解码/分行/切 rune 只做一次）→ 遍历 extractor 注册表 → 产 cod-text-* 字段。
// 主编排稳定，加字段不碰这里。
func (descriptor) Analyze(in describer.Input, full []byte) (map[string]any, map[string]string) {
	if len(full) == 0 {
		return nil, nil
	}
	ctx := buildCtx(full)
	a := map[string]any{}
	for _, e := range extractors {
		if e.needs != nil && !e.needs(ctx) {
			continue
		}
		if v := e.extract(ctx); v != nil {
			a["cod-text-"+e.name] = v
		}
	}
	return a, nil
}
