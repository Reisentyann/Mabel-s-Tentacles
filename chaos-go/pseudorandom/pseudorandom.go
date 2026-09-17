// 文件：chaos-go/pseudorandom/pseudorandom.go —— 混沌机组件·伪随机机：用 PRNG 产随机整数（快、本地、非密码学）
// 修改：2026-09-13（日期由 fresh-header.ps1 刷新）

// Package pseudorandom 是混沌机的「伪随机机」组件：功能名 `pseudo_random`。
//
// 与真随机机相对：数据来自混沌机实例的伪随机数发生器（math/rand/v2 的
// PCG），快、零外部依赖、给定种子可复现——用于娱乐（掷点、抽签、随机数），
// **不用于密码学与保密**。想要物理真随机请用 true_random（见 truerandom）。
package pseudorandom

import (
	"fmt"

	"github.com/Reisentyann/Mabel-s-Tentacles/chaos-go"
)

const (
	defaultCount = 1
	maxCount     = 100
	defaultMin   = 1
	defaultMax   = 100
)

func init() { chaos.Register(pseudoRandom{}) }

type pseudoRandom struct{}

func (pseudoRandom) Name() string { return "pseudo_random" }

func (pseudoRandom) Description() string {
	return "Return pseudo-random integers from the chaos machine's PRNG (math/rand/v2 PCG). " +
		"Fast, local, no external dependency, reproducible given the seed — for entertainment, NOT for secrets. " +
		"For physical true randomness use true_random instead. " +
		"Params: count (1-100, default 1), min/max (inclusive integer range, default 1-100)."
}

func (pseudoRandom) Params() []chaos.Param {
	return []chaos.Param{
		{Name: "count", Type: chaos.ParamNumber, Description: fmt.Sprintf("How many integers to return, 1-%d (default %d).", maxCount, defaultCount), Default: defaultCount},
		{Name: "min", Type: chaos.ParamNumber, Description: "Inclusive lower bound (default 1).", Default: defaultMin},
		{Name: "max", Type: chaos.ParamNumber, Description: "Inclusive upper bound (default 100).", Default: defaultMax},
	}
}

func (pr pseudoRandom) Run(c *chaos.Chaos, p chaos.Params) (map[string]any, error) {
	count := chaos.IntParam(p, "count", defaultCount)
	if count < 1 {
		count = 1
	}
	if count > maxCount {
		count = maxCount
	}
	lo := chaos.IntParam(p, "min", defaultMin)
	hi := chaos.IntParam(p, "max", defaultMax)
	if hi < lo {
		lo, hi = hi, lo
	}
	span := hi - lo + 1
	if span <= 0 { // 区间过大导致溢出
		return nil, fmt.Errorf("范围过大：%d..%d", lo, hi)
	}
	values := make([]int, count)
	for i := range values {
		values[i] = lo + c.IntN(span)
	}
	return map[string]any{
		"success": true,
		"source":  "prng:math-rand-v2",
		"min":     lo,
		"max":     hi,
		"values":  values,
	}, nil
}
