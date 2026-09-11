// 文件：mcp-server-go/internal/search/guide.go —— 检索指南数据面：通用规则 + 自然语言→查询速查（随字段目录响应下发，agent 唯一入口即自足）
// 修改：2026-09-11（日期由 fresh-header.ps1 刷新）

// guide 的定位：基准契约里「不随字段走」的那一半。逐字段的含义与分档
// 在 bench.go（目录项的 desc/bench）；跨字段的口径规则与速查表在这——
// 两者拼起来，agent 一次 list_index_fields / GET /api/index/fields
// 就拿到完整契约，不依赖任何本地文档。
//
// 维护：与 docs/检索契约.md 第 0 节 / 第 8 节同步改（三处同步链：
// 字典 → bench.go + 本文件 → 基准文档）。
package search

// QuickEntry 速查表行：我想要 → 查询条件。
type QuickEntry struct {
	Want string `json:"want"`
	Cond string `json:"cond"`
}

// Guide 检索指南（目录响应的 guide 字段）。
type Guide struct {
	Rules    []string     `json:"rules"`     // 通用口径规则
	QuickRef []QuickEntry `json:"quick_ref"` // 自然语言 → 查询速查
}

// guideData 单例数据（静态契约，进程内共享一份）。
var guideData = Guide{
	Rules: []string{
		"op 与桶型适配：enum 可 eq/in；num 可 eq/in/gt/lt/range（range 的 value=[lo,hi]）；multi（数组字段）eq/in 任一元素命中即中（并集语义，不是全含）",
		"比率类字段值域 0-1，不是百分数：gt 0.5 = 超过一半",
		"纯计数字段 0 也产键：eq 0 是常用的'找没有 X 的文件'（如 checkboxes eq 0 = 干净文件）",
		"比率分母为 0 不产键（如无拉丁字母的文件没有 upper-ratio）：查不到 ≠ 值为 0",
		"优先级：目录的 min/max 与 values 是当前库实测，优先于 bench 先验分档（本库 lines max=21 时，gt 15 即本库长文）",
		"bench 分档是经验锚点不是精确真理：边界附近放宽一档再复筛",
		"llm-* 字段只有经过 LLM 描述的文件才有——目录里它出现与否即覆盖率信号",
		"软删行不挂索引（目录与查询都不含）；sp-* 自由区长尾不进基准表，靠目录 values 发现",
	},
	QuickRef: []QuickEntry{
		{Want: "长文章", Cond: `[{"field":"cod-text-lines","op":"gt","value":200}]（本库口径看目录 max 调低门槛）`},
		{Want: "中文为主的文件", Cond: `[{"field":"cod-text-cjk-ratio","op":"gt","value":0.5}]（比 language=zh 宽，混排也收）`},
		{Want: "小说 / 对话体", Cond: `[{"field":"cod-text-dialog-ratio","op":"gt","value":0.15}]`},
		{Want: "待办清单", Cond: `[{"field":"cod-text-checkboxes","op":"gt","value":0}]`},
		{Want: "日志文件", Cond: `[{"field":"cod-text-timestamp-line-ratio","op":"gt","value":0.5}]`},
		{Want: "数据文件（非文学）", Cond: `[{"field":"cod-text-digit-ratio","op":"gt","value":0.3}]`},
		{Want: "高清大图", Cond: `[{"field":"cod-image-megapixels","op":"gt","value":2}]`},
		{Want: "暗色调图", Cond: `[{"field":"cod-image-dark","op":"eq","value":true}]`},
		{Want: "扁平插画（排除照片）", Cond: `[{"field":"cod-image-flat-ratio","op":"gt","value":0.9}]`},
		{Want: "没有待办的干净文件", Cond: `[{"field":"cod-text-checkboxes","op":"eq","value":0}]`},
	},
}

// TheGuide 返回检索指南（静态契约的只读副本）。
func TheGuide() Guide {
	out := Guide{
		Rules:    make([]string, len(guideData.Rules)),
		QuickRef: make([]QuickEntry, len(guideData.QuickRef)),
	}
	copy(out.Rules, guideData.Rules)
	copy(out.QuickRef, guideData.QuickRef)
	return out
}
