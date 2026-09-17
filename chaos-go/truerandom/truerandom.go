// 文件：chaos-go/truerandom/truerandom.go —— 混沌机组件·真随机机：从注入的量子熵源取真随机（不回退）
// 修改：2026-09-11（日期由 fresh-header.ps1 刷新）

// Package truerandom 是混沌机的「真随机机」组件：功能名 `true_random`。
//
// 数据来自物理世界量子层面的真随机（默认济南器件无关量子随机数 DIQRNG 信标，
// 见 jinan.go；备选 ANU，见 anu.go）。熵源由装配层按配置注入 Chaos 实例——
// 本组件不读配置、不发起 HTTP（网络 IO 只在熵源实现里）。
//
// 铁律（用户口径）：**不回退**。未配置熵源或取数失败一律返回 error，
// 绝不拿伪随机/OS 随机冒充量子随机。
package truerandom

import (
	"encoding/hex"
	"fmt"

	"github.com/Reisentyann/Mabel-s-Tentacles/chaos-go"
)

const (
	defaultByteCount = 8
	maxByteCount     = 32 // 济南单脉冲 256bit，上限 32 字节
)

func init() { chaos.Register(trueRandom{}) }

type trueRandom struct{}

func (trueRandom) Name() string { return "true_random" }

func (trueRandom) Description() string {
	return "Return true random bytes from the configured quantum entropy source " +
		"(default: the Jinan device-independent quantum random number / DIQRNG beacon). " +
		"No fallback: if the quantum source is unconfigured or unreachable this returns an error instead of pseudo-random data. " +
		"Response carries source label and pulse metadata (index/timestamp/type/CHSH/NIST)."
}

func (trueRandom) Params() []chaos.Param {
	return []chaos.Param{
		{Name: "count", Type: chaos.ParamNumber, Description: fmt.Sprintf("Number of random bytes to return, 1-%d (default %d).", maxByteCount, defaultByteCount), Default: defaultByteCount},
	}
}

func (tr trueRandom) Run(c *chaos.Chaos, p chaos.Params) (map[string]any, error) {
	src, err := c.Entropy()
	if err != nil {
		return nil, err
	}
	n := chaos.IntParam(p, "count", defaultByteCount)
	if n < 1 {
		n = 1
	}
	if n > maxByteCount {
		n = maxByteCount
	}
	b, err := src.Bytes(n)
	if err != nil {
		return nil, fmt.Errorf("真随机机取数失败：%w", err)
	}
	out := map[string]any{
		"success": true,
		"source":  src.Name(),
		"bytes":   n,
		"hex":     hex.EncodeToString(b),
	}
	if info, ok := src.(chaos.InfoSource); ok {
		out["meta"] = info.Info()
	}
	return out, nil
}
