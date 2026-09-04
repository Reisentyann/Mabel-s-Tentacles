// 文件：mcp-server-go/cmd/server/main.go —— 服务入口：装配 config/logging/repo/search/api/mcp + 优雅关停 + 不安全默认值告警
// 修改：2026-09-03（日期由 fresh-header.ps1 刷新）

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
	warnInsecureDefaults(cfg)

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

// warnInsecureDefaults 检测仍在使用示例默认值的安全配置，启动时大声告警（不阻断启动）。
func warnInsecureDefaults(cfg *config.Config) {
	if cfg.Security.SecretKey == "supersecretkey" {
		slog.Warn("insecure config: security.secret_key 还是示例默认值 supersecretkey，JWT 可被伪造，生产环境必须修改（config.yml 或 SECRET_KEY 环境变量）")
	}
	if cfg.Admin.Password == "admin123" {
		slog.Warn("insecure config: admin.password 还是示例默认值 admin123，任何人都能登录管理端，生产环境必须修改（config.yml 或 ADMIN_PASSWORD 环境变量）")
	}
	if cfg.API.RequireAuth {
		// 开着鉴权却没配下载 token 时，/api/files/download 会裸奔，这里只提示一句
		if cfg.API.AccessToken == "" {
			slog.Warn("insecure config: require_auth 已开启但 api.access_token 为空，/api/files/download 下载端点将不做鉴权")
		}
	}
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
