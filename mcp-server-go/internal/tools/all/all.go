// Package all 集中 blank import 所有工具子包，触发各自的 init() 自注册。
// 新增工具：新建子包后，在这里加一行 import。
package all

import (
	_ "github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/tools/executecommand"
	_ "github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/tools/getresults"
	_ "github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/tools/listdatafiles"
	_ "github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/tools/modifydatafile"
	_ "github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/tools/segmentedreply"
	_ "github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/tools/writefile"
)
