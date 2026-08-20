package mcp

import (
	"github.com/mark3labs/mcp-go/server"

	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/config"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/store"
	_ "github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/tools/all"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/tools"
)

func New(cfg *config.Config, st store.Store) *server.MCPServer {
	s := server.NewMCPServer(
		"agent-mcp-server",
		"0.1.0",
		server.WithRecovery(),
	)

	tools.RegisterAll(s, tools.Deps{Cfg: cfg, Store: st})

	return s
}
