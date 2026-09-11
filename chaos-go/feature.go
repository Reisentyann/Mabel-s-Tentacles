// 文件：chaos-go/feature.go —— 混沌机功能注册表：Feature 接口 + 自注册 + 参数读取助手
// 修改：2026-09-11（日期由 fresh-header.ps1 刷新）

package chaos

import (
	"fmt"
	"strconv"
	"sync"
)

// ParamType 参数类型（MCP 工具 schema 用）。混沌机本体不依赖 MCP——
// 由装配层把 Param 翻译成具体工具的入参 schema。
type ParamType string

const (
	ParamString ParamType = "string"
	ParamNumber ParamType = "number"
	ParamBool   ParamType = "bool"
)

// Param 一个功能参数的声明（名字 / 类型 / 说明 / 是否必填 / 默认值）。
type Param struct {
	Name        string
	Type        ParamType
	Description string
	Required    bool
	Default     any
}

// Params 功能调用的入参（JSON 归一样，数字可能是 float64）。
type Params map[string]any

// Feature 混沌机的一个娱乐功能。新增功能 = 新建一个文件实现本接口，
// 在 init() 里 Register——装配层会自动为它生成 MCP 工具，无需改任何接线。
type Feature interface {
	Name() string                                   // 对外唯一名（= MCP 工具名，如 mabel_quote）
	Description() string                            // 给 agent 看的功能说明
	Params() []Param                                // 入参声明（可为空）
	Run(c *Chaos, p Params) (map[string]any, error) // 执行：返回给 agent 的结果体
}

var (
	featureMu    sync.RWMutex
	featureByKey = map[string]Feature{}
	featureList  []Feature
)

// Register 由各功能文件的 init() 调用完成自注册。
// 空名 / 重名即 panic——装配期错误尽早暴露，不留到运行期。
func Register(f Feature) {
	featureMu.Lock()
	defer featureMu.Unlock()
	name := f.Name()
	if name == "" {
		panic("chaos: feature name empty")
	}
	if _, dup := featureByKey[name]; dup {
		panic("chaos: duplicate feature " + name)
	}
	featureByKey[name] = f
	featureList = append(featureList, f)
}

// Features 返回全部已注册功能（注册序稳定）。
func Features() []Feature {
	featureMu.RLock()
	defer featureMu.RUnlock()
	out := make([]Feature, len(featureList))
	copy(out, featureList)
	return out
}

// Lookup 按名取功能。
func Lookup(name string) (Feature, bool) {
	featureMu.RLock()
	defer featureMu.RUnlock()
	f, ok := featureByKey[name]
	return f, ok
}

// Run 执行命名功能（默认混沌机实例）。
func Run(name string, p Params) (map[string]any, error) {
	return Default().Run(name, p)
}

// Run 执行命名功能；功能不存在返回错误。
func (c *Chaos) Run(name string, p Params) (map[string]any, error) {
	f, ok := Lookup(name)
	if !ok {
		return nil, fmt.Errorf("chaos: unknown feature %q", name)
	}
	return f.Run(c, p)
}

// IntParam 从 Params 取整数，兼容 JSON 的 float64 与字符串形态；缺失/坏值取 def。
func IntParam(p Params, key string, def int) int {
	v, ok := p[key]
	if !ok || v == nil {
		return def
	}
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case string:
		if i, err := strconv.Atoi(n); err == nil {
			return i
		}
	}
	return def
}

// StrParam 从 Params 取字符串；缺失取 def。
func StrParam(p Params, key, def string) string {
	if v, ok := p[key].(string); ok {
		return v
	}
	return def
}

// BoolParam 从 Params 取布尔；缺失取 def。
func BoolParam(p Params, key string, def bool) bool {
	if v, ok := p[key].(bool); ok {
		return v
	}
	return def
}
