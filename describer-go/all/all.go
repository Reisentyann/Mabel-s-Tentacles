// 文件：describer-go/all/all.go —— describer 插件聚合 blank import（触发各子包 init 自注册）
// 修改：2026-09-03（日期由 fresh-header.ps1 刷新）

// Package all 集中 blank import 所有描述插件子包，触发各自的 init() 自注册。
// 新增插件：建子包后在这里加一行 import（与 mcp-server-go/tools/all 同一模式）。
package all

import (
	_ "github.com/Reisentyann/Mabel-s-Tentacles/describer-go/basic"
	_ "github.com/Reisentyann/Mabel-s-Tentacles/describer-go/code"
	_ "github.com/Reisentyann/Mabel-s-Tentacles/describer-go/image"
	_ "github.com/Reisentyann/Mabel-s-Tentacles/describer-go/text"
)
