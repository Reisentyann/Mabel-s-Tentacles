// 文件：chaos-go/all/all.go —— 混沌机组件聚合：blank import 触发各组件自注册
// 修改：2026-09-21（日期由 fresh-header.ps1 刷新）

// Package all 集中 blank import 所有混沌机组件子包，触发各自的 init()
// 自注册。新增组件：新建子包后，在这里加一行 import。
package all

import (
	_ "github.com/Reisentyann/Mabel-s-Tentacles/chaos-go/jmcomic"
	_ "github.com/Reisentyann/Mabel-s-Tentacles/chaos-go/mabel"
	_ "github.com/Reisentyann/Mabel-s-Tentacles/chaos-go/pseudorandom"
	_ "github.com/Reisentyann/Mabel-s-Tentacles/chaos-go/truerandom"
)
