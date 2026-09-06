// 文件：mcp-server-go/cmd/server/main.go —— 服务入口：装配 config/logging/repo/search/api/mcp + 优雅关停 + 不安全默认值告警
// 修改：2026-09-06（日期由 fresh-header.ps1 刷新）

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
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/mark3labs/mcp-go/server"

	"github.com/Reisentyann/Mabel-s-Tentacles/indexer-go"
	"github.com/Reisentyann/Mabel-s-Tentacles/manager-go"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/core"
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

	// 三机经编排机串联（2026-09-06 接线批次）：
	//   - indexer 实例同时注入编排机（T1/describe/copy 喂食 + 检索门面）与
	//     管理机（T2/T3 喂食）——喂食链全点亮
	//   - 启动全量 Rebuild（DB 是事实源，索引是派生缓存）；失败仅告警，
	//     检索自动降级 SQL，服务照常起
	//   - 写路径走编排机事件队列（异步）；describe 走同步入口
	idx := indexer.New()
	orch, err := core.New(core.Options{
		DataDir:  cfg.DataDir,
		Store:    st,
		Sink:     idx,
		Index:    idx,
		Fallback: search.NewSQLSearcher(st),
	})
	if err != nil {
		slog.Error("init orchestrator failed", "error", err)
		os.Exit(1)
	}
	if err := orch.RebuildIndex(ctx); err != nil {
		slog.Warn("index rebuild failed, search degrades to SQL until next restart", "error", err)
	}
	orch.Start(ctx)

	// 管理机（updater 域：T2/T3）：sink 同为 idx 实例，重分析喂食随之点亮
	mgr := manager.New(repo.NewManagerStore(st), cfg.DataDir, idx, func(p string) string {
		_, mt := service.InferFileMeta(p)
		return mt
	})

	// MCP server。通道鉴权双轨：master key（.env，管家）/ 外部 key
	// （agent_keys 表，绑定用户）；空 key = 开发放行（上方已大红 WARN）
	s := mcpserver.New(cfg, st, mgr, orch)
	sse := server.NewSSEServer(s, server.WithBaseURL(cfg.Server.BaseURL))
	mcpAuth := mcpserver.AuthMiddleware(sse, cfg.MCP.APIKey, st)

	// 组合 MCP + HTTP API 到同一个 mux。
	// orch 实现 search.Searcher（检索门面：索引优先 → SQL 降级，索引化检索待 uuid 取件批次）
	mux := http.NewServeMux()
	mux.Handle("/sse", mcpAuth)
	mux.Handle("/message", mcpAuth)
	api.Register(mux, cfg, st, orch, mgr)

	// T2 启动后台回填（describe.backfill，默认关闭）：先跑一轮再按 interval 轮询，
	// 一轮结束即返回（铁律 3：绝不自旋），关停即断点（幂等可续跑）
	if cfg.Describe.Backfill.Enabled {
		go backfillLoop(ctx, mgr, cfg.Describe.Backfill)
	}

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
	// 编排机排空在途事件（尽力而为：ctx 已取消的落库失败按容灾立场丢弃，
	// 盘上文件是事实源，T2 对账兜底重建）
	orch.Stop()
}

// backfillLoop T2 轮询驱动：启动先跑一轮，之后按 interval 反复调用
// manager.Backfill（一轮 = 查→处理→写→结束，返回后才等下一轮）。
func backfillLoop(ctx context.Context, mgr *manager.Manager, bc config.BackfillConfig) {
	interval := time.Duration(bc.Interval) * time.Second
	if interval <= 0 {
		interval = time.Minute
	}
	if n, err := mgr.Backfill(ctx, bc.Batch); err != nil {
		slog.Warn("backfill round failed", "error", err, "analyzed", n)
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if n, err := mgr.Backfill(ctx, bc.Batch); err != nil {
				slog.Warn("backfill round failed", "error", err, "analyzed", n)
			}
		}
	}
}

// warnInsecureDefaults 检测仍在使用示例默认值的安全配置，启动时大声告警（不阻断启动）。
func warnInsecureDefaults(cfg *config.Config) {
	if cfg.Security.SecretKey == "supersecretkey" {
		slog.Warn("insecure config: security.secret_key 还是示例默认值 supersecretkey，JWT 可被伪造，生产环境必须修改（config.yml 或 SECRET_KEY 环境变量）")
	}
	if cfg.Admin.Password == "admin123" {
		slog.Warn("insecure config: admin.password 还是示例默认值 admin123，任何人都能登录管理端，生产环境必须修改（config.yml 或 ADMIN_PASSWORD 环境变量）")
	}
	if cfg.MCP.APIKey == "" {
		slog.Warn("insecure config: mcp.api_key（master key）未配置——MCP 通道无鉴权放行（开发口径），生产环境必须配置强随机值（.env 的 MCP_API_KEY），否则任何能访问 /sse 的客户端都拥有管家全权（含 execute_command）")
	}
	if cfg.API.AccessToken == "" {
		slog.Info("config note: api.access_token 为空——/api/files/download 需 JWT（前端 blob 下载）；agent 直发链接场景需配置该静态 token（过渡口径，将来由管理机下载票据域取代）")
	}
}

// bootstrapAdmin 启动时确保默认管理员存在且持有 admin 角色（权限批次
// 2026-09-06）：迁移给存量行落的默认角色是 'user'，这里对配置的管理员
// 用户名做存在性 + 角色双保障——已有账号自动提升（密码不动）。
func bootstrapAdmin(ctx context.Context, st repo.Store, cfg *config.Config) error {
	u, err := st.GetUserByUsername(ctx, cfg.Admin.Username)
	if err == nil {
		if u.Role != "admin" {
			if err := st.SetUserRole(ctx, cfg.Admin.Username, "admin"); err != nil {
				return err
			}
			slog.Info("existing admin promoted to admin role", "username", cfg.Admin.Username)
		}
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}

	hash, err := service.HashPassword(cfg.Admin.Password)
	if err != nil {
		return err
	}
	if _, err := st.CreateUser(ctx, cfg.Admin.Username, hash, "admin@example.com", "admin"); err != nil {
		return err
	}
	slog.Info("default admin created", "username", cfg.Admin.Username, "role", "admin")
	return nil
}
