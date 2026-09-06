// 文件：mcp-server-go/internal/mcp/server.go —— MCP 服务器装配：SSE 端点 + 工具注册（tools/all）
// 修改：2026-09-06（日期由 fresh-header.ps1 刷新）

package mcp

import (
	"github.com/mark3labs/mcp-go/server"

	"github.com/Reisentyann/Mabel-s-Tentacles/manager-go"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/core"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/config"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/repo"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/tools"
	_ "github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/tools/all"
)

// New 装配 MCP 服务器。orch 为编排机（写路径事件 + Describe 入口；
// 三机串联的注入点，见 core 包注释）。
func New(cfg *config.Config, st repo.Store, mgr *manager.Manager, orch *core.Orchestrator) *server.MCPServer {
	s := server.NewMCPServer(
		"agent-mcp-server",
		"0.1.0",
		server.WithRecovery(),
	)

	tools.RegisterAll(s, tools.Deps{Cfg: cfg, Store: st, Manager: mgr, Orch: orch})

	return s
}
