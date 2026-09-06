// 文件：common/ptr.go —— 指针小件：StrPtr（空串→nil 的 COALESCE 语义）与 DerefStr/DerefInt64（nil→零值）
// 修改：2026-09-06（日期由 fresh-header.ps1 刷新）

package common

// StrPtr 空字符串返回 nil（表示「不覆盖原值」），非空返回指针。
// agent 侧可选字段 → 落库 DTO 指针语义的统一转换口
// （tools.StrPtr / core.strPtr / repo.derefStr 三处同义的收敛点）。
func StrPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// DerefStr nil → 空串。
func DerefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// DerefInt64 nil → 0。
func DerefInt64(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}
