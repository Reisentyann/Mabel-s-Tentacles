// 文件：mcp-server-go/internal/mcp/server.go —— MCP 服务器装配：SSE 端点 + 工具注册（tools/all）
// 修改：2026-09-03（日期由 fresh-header.ps1 刷新）

package mcp

import (
	"github.com/mark3labs/mcp-go/server"

	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/config"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/repo"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/tools"
	_ "github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/tools/all"
)

func New(cfg *config.Config, st repo.Store) *server.MCPServer {
	s := server.NewMCPServer(
		"agent-mcp-server",
		"0.1.0",
		server.WithRecovery(),
	)

	tools.RegisterAll(s, tools.Deps{Cfg: cfg, Store: st})

	return s
}
