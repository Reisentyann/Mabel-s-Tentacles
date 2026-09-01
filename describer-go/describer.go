// Package describer 是确定性文件描述引擎：字节进、事实出。
// 产出仅允许 cod- 前缀固定字段与 sp-cod- 自由字段，字段字典见仓库
// docs/元数据字段说明.md；模型轨（llm-*）在 llm 子包。
// 本包不做任何 IO（读文件由调用方完成），不做任何语义猜测。
package describer

import (
	"time"
)

// Input 描述输入。Head 为前 512B；Full 可为 nil，此时通过 Loader 惰性加载。
type Input struct {
	Path    string    // 文件相对路径（扩展名 / 文件名模式判定用）
	Head    []byte    // 前 512B（basic 嗅探用）
	Full    []byte    // 全量内容；nil 时非 basic 插件经 Loader 获取
	Size    int64     // 文件字节数
	MTime   time.Time // 文件系统修改时间
	ExtMime string    // 扩展名推断的 MIME（调用方传入，用于 mime-match 对比）
}

// Loader 惰性全量读取（调用方实现，含 5MB 截断职责）。出错时需要全量的插件被跳过。
type Loader func() ([]byte, error)

// Result 单个插件家族的产物。Attrs 键均已带完整前缀
// （cod-<family>-<字段> 与 sp-cod-<自由字段>），nil 字段族不会出现键。
type Result struct {
	Family  string
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
func Analyze(in Input, load Loader) []Result {
	var results []Result

	// 1) basic 恒跑（路由事实的来源）
	var b Basic
	for _, d := range registry {
		if d.Family() != "basic" {
			continue
		}
		attrs, _ := d.Analyze(in, nil)
		if len(attrs) == 0 {
			continue
		}
		results = append(results, Result{Family: "basic", Attrs: attrs})
		if v, ok := attrs["cod-basic-mime"].(string); ok {
			b.Mime = v
		}
		if v, ok := attrs["cod-basic-textish"].(bool); ok {
			b.Textish = v
		}
	}

	// 2) 其余插件：Supports 路由 + 共享一次全量加载
	var full []byte
	loaded := false
	needFull := func() ([]byte, bool) {
		if loaded {
			return full, full != nil
		}
		loaded = true
		if in.Full != nil {
			full = in.Full
			return full, true
		}
		if load != nil {
			if data, err := load(); err == nil {
				full = data
				return full, true
			}
		}
		return nil, false
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
		if len(attrs) > 0 {
			results = append(results, Result{Family: d.Family(), Attrs: attrs, SPPurge: d.SPNamespaces()})
		}
	}
	return results
}
