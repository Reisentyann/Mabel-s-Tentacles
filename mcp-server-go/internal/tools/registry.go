package tools

import (
	"github.com/mark3labs/mcp-go/server"

	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/config"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/store"
)

// Deps 是注入给所有工具的统一依赖容器。保持字段精简，避免变成上帝对象；
// 新依赖优先以接口形式加入。
type Deps struct {
	Cfg   *config.Config
	Store store.Store
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
