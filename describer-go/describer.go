// 文件：describer-go/describer.go —— 描述引擎本体：Descriptor 接口 + 注册表 + Analyze 编排（basic 恒跑 + 路由 + 共享全量加载）
// 修改：2026-09-04（日期由 fresh-header.ps1 刷新）

// Package describer 是确定性文件描述引擎：字节进、事实出。
// 产出仅允许 cod- 前缀固定字段与 sp-cod- 自由字段，字段字典见仓库
// docs/元数据字段说明.md；模型轨（llm-*）在 llm 子包。
// 本包不做任何 IO（读文件由调用方完成），不做任何语义猜测。
package describer

import (
	"time"
)

// 分析预算（字段字典第 7 节）。截断由引擎在 Analyze 入口统一强制执行——
// 调用方传全量也安全，预算不再依赖调用方自觉（T1 写路径曾直传全量 content，
// 超大文件会让 text 插件全量解码切 rune，内存数倍膨胀）。
const (
	// MaxHeadBytes basic 嗅探预算：前 512B（http.DetectContentType 口径）。
	MaxHeadBytes = 512
	// MaxFullBytes 全量分析预算：text/image 全读上限 5MB。
	MaxFullBytes = 5 << 20
)

// Input 描述输入。Head 为前 512B（超长引擎截断）；Full 可为 nil，此时通过 Loader 惰性加载。
type Input struct {
	Path    string    // 文件相对路径（扩展名 / 文件名模式判定用）
	Head    []byte    // 前 512B（basic 嗅探用；超长由引擎截断）
	Full    []byte    // 全量内容；nil 时非 basic 插件经 Loader 获取（超 5MB 由引擎截断）
	Size    int64     // 文件字节数
	MTime   time.Time // 文件系统修改时间
	ExtMime string    // 扩展名推断的 MIME（调用方传入，用于 mime-match 对比）
}

// Loader 惰性全量读取（调用方实现；5MB 截断由引擎统一执行，调用方无需自截）。
// 出错时需要全量的插件被跳过。
type Loader func() ([]byte, error)

// Result 单个插件家族的产物。Attrs 键均已带完整前缀
// （cod-<family>-<字段> 与 sp-cod-<自由字段>）。Attrs 为 nil 表示该家族
// 本轮已跑但零产出（如图片损坏后解码失败）——仍入列，MergeResults 借此
// 整族清除旧键，防陈旧事实残留与 IsStale 反复重触发。
type Result struct {
	Family  string
	Ver     int // 家族算法版本（FamilyVersion()），合并时落 cod-<family>-ver
	Attrs   map[string]any
	SPPurge []string // 本家族负责的 sp-cod- 前缀（整族替换时一并清除旧键）
}

// Basic basic 插件产出的路由事实，供其余插件 Supports 判定。
type Basic struct {
	Mime    string // 魔数嗅探 MIME
	Textish bool   // 是否文本（无 NUL 字节且控制字符占比低）
}

// Descriptor 硬编码描述器。实现方只允许产出本家族封闭字段集
// （键为代码中的字面量，不存在任意键出口）与 SP 自由字段。
type Descriptor interface {
	Family() string
	// FamilyVersion 家族算法版本（字段字典第 10.1 节）：
	// 新增字段 / 改算法输出 / 删字段 → +1；纯重构不改输出 → 不动。
	FamilyVersion() int
	Supports(path string, head []byte, b Basic) bool
	SPNamespaces() []string
	Analyze(in Input, full []byte) (attrs map[string]any, sp map[string]string)
}

var registry []Descriptor

// Register 由各插件 init() 调用完成自注册（见 all/all.go 集中 blank import）。
func Register(d Descriptor) {
	registry = append(registry, d)
}

// Analyze 运行全部命中插件：basic 恒跑并产出路由事实；
// 其余插件共享一次全量加载。返回各家族结果（注册序，确定）。
// 已跑即入列——零产出的家族以空 Attrs 入列，供合并时清除旧键；
// 全量不可得（加载失败）的家族不入列，旧键保留，重跑时机由 IsStale 判定。
func Analyze(in Input, load Loader) []Result {
	// 预算截断（引擎强制，调用方不可绕过）
	if len(in.Head) > MaxHeadBytes {
		in.Head = in.Head[:MaxHeadBytes]
	}

	var results []Result

	// 1) basic 恒跑（路由事实的来源）。basic 家族唯一：只认首个注册，
	// 防重复注册静默双跑
	var b Basic
	for _, d := range registry {
		if d.Family() != "basic" {
			continue
		}
		attrs, _ := d.Analyze(in, nil)
		if len(attrs) > 0 {
			results = append(results, Result{Family: "basic", Ver: d.FamilyVersion(), Attrs: attrs})
			if v, ok := attrs["cod-basic-mime"].(string); ok {
				b.Mime = v
			}
			if v, ok := attrs["cod-basic-textish"].(bool); ok {
				b.Textish = v
			}
		}
		break
	}

	// 2) 其余插件：Supports 路由 + 共享一次全量加载（5MB 截断在加载点统一执行）
	var full []byte
	loaded := false
	needFull := func() ([]byte, bool) {
		if loaded {
			return full, full != nil
		}
		loaded = true
		data := in.Full
		if data == nil && load != nil {
			if d, err := load(); err == nil {
				data = d
			}
		}
		if len(data) > MaxFullBytes {
			data = data[:MaxFullBytes]
		}
		full = data
		return full, full != nil
	}

	for _, d := range registry {
		if d.Family() == "basic" || !d.Supports(in.Path, in.Head, b) {
			continue
		}
		data, ok := needFull()
		if !ok {
			continue
		}
		attrs, sp := d.Analyze(in, data)
		// SP 自由字段：引擎统一加 sp-cod- 前缀，插件只给原始键
		for k, v := range sp {
			if attrs == nil {
				attrs = map[string]any{}
			}
			attrs["sp-cod-"+k] = v
		}
		// 已跑即入列（含零产出）：MergeResults 对空 Attrs 仍整族清旧键，
		// 防"本轮无事实"时旧键残留（如图片损坏后解码失败）
		results = append(results, Result{Family: d.Family(), Ver: d.FamilyVersion(), Attrs: attrs, SPPurge: d.SPNamespaces()})
	}
	return results
}
