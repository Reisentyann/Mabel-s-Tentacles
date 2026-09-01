// Package all 集中 blank import 所有描述插件子包，触发各自的 init() 自注册。
// 新增插件：建子包后在这里加一行 import（与 mcp-server-go/tools/all 同一模式）。
package all

import (
	_ "github.com/Reisentyann/Mabel-s-Tentacles/describer-go/plugins/basic"
	_ "github.com/Reisentyann/Mabel-s-Tentacles/describer-go/plugins/code"
	_ "github.com/Reisentyann/Mabel-s-Tentacles/describer-go/plugins/image"
	_ "github.com/Reisentyann/Mabel-s-Tentacles/describer-go/plugins/text"
)
