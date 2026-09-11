// 文件：chaos-go/chaos.go —— 混沌机：娱乐功能集合的实例与随机源（零 IO 零 DB 纯库）
// 修改：2026-09-11（日期由 fresh-header.ps1 刷新）

// Package chaos 混沌机：与文件系统无关的娱乐功能集合。
//
// 定位：描述机（字节→事实）、索引机（字段→uuid）、管理机（文件在哪）之外
// 的第四个纯库模块。混沌机不碰盘、不碰 DB、不依赖任何兄弟模块——每个娱乐
// 功能自成一个文件（mabel.go 梅贝尔台词……），可独立调用，也可经 MCP 工具
// 暴露给 agent 陪聊。
//
// 形态：`Chaos` 实例持有独立随机源，运行即产出随机结果；进程级单例走
// `Default()`，测试可 `New()` 建独立实例。
package chaos

import (
	"math/rand/v2"
	"sync"
)

// Chaos 混沌机实例：持有独立随机源，方法并发安全。
type Chaos struct {
	mu  sync.Mutex
	rng *rand.Rand
}

// New 新建混沌机。种子取自全局随机源（math/rand/v2 的 PCG）。
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

// intn 并发安全地取 [0,n) 内的随机整数（n<=0 返回 0）。
func (c *Chaos) intn(n int) int {
	if n <= 0 {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.rng.IntN(n)
}

// shuffle 并发安全洗牌 n 个元素（就地交换）。
func (c *Chaos) shuffle(n int, swap func(i, j int)) {
	if n <= 1 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rng.Shuffle(n, swap)
}
