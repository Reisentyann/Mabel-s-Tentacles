package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/jackc/pgx/v5"
	"github.com/mark3labs/mcp-go/server"

	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/api"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/config"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/logging"
	mcpserver "github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/mcp"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/repo"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/search"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/service"
)

func main() {
	cfg := config.Load()
	logging.Setup(cfg.Log.Level, cfg.Log.Format, cfg.Log.File)

	slog.Info("config loaded",
		"port", cfg.Server.Port,
		"base_url", cfg.Server.BaseURL,
		"data_dir", cfg.DataDir,
		"log_level", cfg.Log.Level,
		"log_format", cfg.Log.Format,
		"require_auth", cfg.API.RequireAuth,
		"mcp_api_key_set", cfg.MCP.APIKey != "",
		"access_token_set", cfg.API.AccessToken != "",
		"download_base_url", cfg.API.DownloadBaseURL,
		"admin_username", cfg.Admin.Username,
	)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if u, err := url.Parse(cfg.DatabaseDSN()); err == nil {
		slog.Info("connecting database", "host", u.Host, "name", strings.TrimPrefix(u.Path, "/"))
	}

	st, err := repo.New(ctx, cfg.DatabaseDSN(), cfg.Database.MaxConns)
	if err != nil {
		slog.Error("connect database failed", "error", err)
		slog.Error("hint: set DATABASE_URL env to a reachable PostgreSQL (e.g. postgres://user:pass@localhost:5432/agent_db), or fix config.yml database.host/port")
		os.Exit(1)
	}
	defer st.Close()
	slog.Info("database connected")

	if err := bootstrapAdmin(ctx, st, cfg); err != nil {
		slog.Warn("bootstrap admin failed", "error", err)
	}

	// MCP server
	s := mcpserver.New(cfg, st)
	sse := server.NewSSEServer(s, server.WithBaseURL(cfg.Server.BaseURL))
	mcpAuth := mcpserver.AuthMiddleware(sse, cfg.MCP.APIKey)

	// 组合 MCP + HTTP API 到同一个 mux
	mux := http.NewServeMux()
	mux.Handle("/sse", mcpAuth)
	mux.Handle("/message", mcpAuth)
	api.Register(mux, cfg, st, search.NewSQLSearcher(st))

	httpServer := &http.Server{
		Addr:    ":" + cfg.Server.Port,
		Handler: api.RequestLog(api.TrailingSlash(mux)),
	}

	go func() {
		slog.Info("server listening", "port", cfg.Server.Port, "mcp", "/sse", "api", "/api")
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down")
	sse.CloseSessions()
	_ = httpServer.Shutdown(context.Background())
}

// bootstrapAdmin 启动时确保默认管理员存在（等价 Python 的 lifespan）。
func bootstrapAdmin(ctx context.Context, st repo.Store, cfg *config.Config) error {
	_, err := st.GetUserByUsername(ctx, cfg.Admin.Username)
	if err == nil {
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}

	hash, err := service.HashPassword(cfg.Admin.Password)
	if err != nil {
		return err
	}
	if _, err := st.CreateUser(ctx, cfg.Admin.Username, hash, "admin@example.com"); err != nil {
		return err
	}
	slog.Info("default admin created", "username", cfg.Admin.Username)
	return nil
}
