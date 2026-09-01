package describer

import "strings"

// 共享原语（≈ stdlib image/color 的角色）：各插件与 llm 子包共同使用的
// 确定性小工具。留在根包是因为它们是插件 API 契约的一部分。

// Round2 保留两位小数（数值字段统一精度）。
func Round2(f float64) float64 {
	return float64(int(f*100+0.5)) / 100
}

// Round1 保留一位小数。
func Round1(f float64) float64 {
	return float64(int(f*10+0.5)) / 10
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
