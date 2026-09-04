// 文件：describer-go/primitives.go —— 共享原语（≈ image/color）：Round2/Round1/TrimRunes/IsCJK/IsKana/ToSlashLower
// 修改：2026-09-03（日期由 fresh-header.ps1 刷新）

package describer

import (
	"math"
	"strings"
)

// 共享原语（≈ stdlib image/color 的角色）：各插件与 llm 子包共同使用的
// 确定性小工具。留在根包是因为它们是插件 API 契约的一部分。

// Round2 保留两位小数（数值字段统一精度）。math.Round 半值远离零：
// 对非负数与旧实现（int(f*100+0.5) 截断）等价，负数方向修正。
func Round2(f float64) float64 {
	return math.Round(f*100) / 100
}

// Round1 保留一位小数。
func Round1(f float64) float64 {
	return math.Round(f*10) / 10
}

// UTF16BOM 判定 UTF-16 BOM：FF FE → LE（little=true），FE FF → BE。
// UTF-32 LE 的 FF FE 00 00 不在此列（4 字节 BOM，维持二进制判定）。
// basic 的 textish 闸门与 text 的解码链共用本判定。
func UTF16BOM(head []byte) (little, ok bool) {
	if len(head) < 2 {
		return false, false
	}
	switch {
	case head[0] == 0xFF && head[1] == 0xFE:
		if len(head) >= 4 && head[2] == 0x00 && head[3] == 0x00 {
			return false, false
		}
		return true, true
	case head[0] == 0xFE && head[1] == 0xFF:
		return false, true
	}
	return false, false
}

// TrimRunes 按 rune 截断（多值字段防膨胀）。
func TrimRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}

// IsCJK 判定中日韩字符（含兼容区与谚文）。
func IsCJK(r rune) bool {
	return (r >= 0x4E00 && r <= 0x9FFF) || // CJK 统一表意
		(r >= 0x3400 && r <= 0x4DBF) || // 扩展 A
		(r >= 0xF900 && r <= 0xFAFF) || // 兼容表意
		(r >= 0xAC00 && r <= 0xD7AF) // 谚文
}

// IsKana 判定平假名/片假名（日语标志）。
func IsKana(r rune) bool {
	return r >= 0x3040 && r <= 0x30FF
}

// ToSlashLower 键名规范化：小写 + 下划线转中划线。
func ToSlashLower(s string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(s)), "_", "-")
}
