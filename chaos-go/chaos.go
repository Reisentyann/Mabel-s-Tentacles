// 文件：chaos-go/chaos.go —— 混沌机核心：实例 + 随机源 + 可选真随机熵源注入
// 修改：2026-09-11（日期由 fresh-header.ps1 刷新）

// Package chaos 混沌机：与文件系统无关的娱乐功能集合。
//
// 定位：描述机（字节→事实）、索引机（字段→uuid）、管理机（文件在哪）之外
// 的第四个纯库模块。混沌机不碰盘、不碰 DB、不依赖任何兄弟模块——每个娱乐
// 功能自成**一个文件夹**（mabel/ 梅贝尔台词、truerandom/ 真随机机……），
// 在 init() 里 `Register` 自注册，装配层遍历注册表即可自动挂载。
//
// 形态：`Chaos` 实例持有随机源与可选的真随机熵源，运行即产出随机结果；
// 进程级单例走 `Default()`，测试可 `New()` 建独立实例。
package chaos

import (
	"errors"
	"math/rand/v2"
	"sync"
)

// EntropySource 真随机熵源（可选注入）：Bytes 返回 n 字节真随机数据。
//
// 语义铁律：**不回退**——取数失败必须返回 error，调用方如实报错，
// 绝不拿伪随机/OS 随机冒充量子随机。来源标注由 Name 提供
// （如 "quantum:cn-jinan"）。
type EntropySource interface {
	Name() string
	Bytes(n int) ([]byte, error)
}

// InfoSource 可选能力：熵源自述元数据（脉冲号/时间/CHSH 等）。
// 实现则在回执里带出，便于使用者判断数据来源与新鲜度。
type InfoSource interface {
	Info() map[string]any
}

// ErrNoEntropy 未注入真随机熵源（真随机机未配置）。
var ErrNoEntropy = errors.New("chaos: 真随机熵源未配置")

// Chaos 混沌机实例：持有独立随机源（并发安全）与可选真随机熵源。
type Chaos struct {
	mu  sync.Mutex
	rng *rand.Rand

	emu     sync.RWMutex
	entropy EntropySource
}

// New 新建混沌机。随机种子取自全局随机源（math/rand/v2 的 PCG）。
func New() *Chaos {
	return &Chaos{rng: rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))}
}

var (
	defaultOnce  sync.Once
	defaultChaos *Chaos
)

// Default 返回进程级混沌机单例。
func Default() *Chaos {
	defaultOnce.Do(func() { defaultChaos = New() })
	return defaultChaos
}

// IntN 并发安全地取 [0,n) 内的随机整数（n<=0 返回 0）。功能组件共用。
func (c *Chaos) IntN(n int) int {
	if n <= 0 {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.rng.IntN(n)
}

// Shuffle 并发安全洗牌 n 个元素（就地交换）。功能组件共用。
func (c *Chaos) Shuffle(n int, swap func(i, j int)) {
	if n <= 1 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rng.Shuffle(n, swap)
}

// SetEntropySource 注入真随机熵源（装配层启动时调用；nil = 清除）。
func (c *Chaos) SetEntropySource(s EntropySource) {
	c.emu.Lock()
	defer c.emu.Unlock()
	c.entropy = s
}

// Entropy 取已注入的熵源；未配置返回 ErrNoEntropy。
func (c *Chaos) Entropy() (EntropySource, error) {
	c.emu.RLock()
	defer c.emu.RUnlock()
	if c.entropy == nil {
		return nil, ErrNoEntropy
	}
	return c.entropy, nil
}
