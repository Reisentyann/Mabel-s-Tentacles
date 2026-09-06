// 文件：mcp-server-go/internal/tools/registry.go —— 工具注册表：Deps 依赖集 + Register / RegisterAll
// 修改：2026-09-06（日期由 fresh-header.ps1 刷新）

package tools

import (
	"github.com/mark3labs/mcp-go/server"

	"github.com/Reisentyann/Mabel-s-Tentacles/manager-go"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/core"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/config"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/repo"
)

// Deps 是注入给所有工具的统一依赖容器。保持字段精简，避免变成上帝对象；
// 新依赖优先以接口形式加入。
type Deps struct {
	Cfg     *config.Config
	Store   repo.Store         // 操作存档 / 元数据直读（list_data_files 等）
	Manager *manager.Manager   // 管理机（updater 域：analyze_file 工具走它；nil = 未装配）
	Orch    *core.Orchestrator // 编排机：写路径事件 Submit（T1 异步落库喂索引）+ Describe 同步入口；nil = 未装配
}

type Registrar func(s *server.MCPServer, deps Deps)

var registrars []Registrar

// Register 由各工具子包的 init() 调用，完成自注册。
func Register(r Registrar) {
	registrars = append(registrars, r)
}

// RegisterAll 遍历所有已注册的工具，统一挂载到 MCP server。
func RegisterAll(s *server.MCPServer, deps Deps) {
	for _, r := range registrars {
		r(s, deps)
	}
}
