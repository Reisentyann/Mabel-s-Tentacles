package repo

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Store 是数据访问层接口，便于 mock 测试与替换实现。
type Store interface {
	// 用户
	GetUserByUsername(ctx context.Context, username string) (*User, error)
	CreateUser(ctx context.Context, username, passwordHash, email string) (*User, error)

	// token 黑名单
	InsertBlacklist(ctx context.Context, jti string, expiresAt time.Time) error
	IsTokenBlacklisted(ctx context.Context, jti string) (bool, error)

	// 命令
	InsertCommand(ctx context.Context, userID int, command string) (int64, error)
	UpdateCommand(ctx context.Context, id int64, status, result, errMsg string, exitCode int) error
	GetCommands(ctx context.Context, userID, limit int) ([]CommandResult, error)
	ListCommands(ctx context.Context, page, size int) ([]CommandResult, int, error)
	GetCommand(ctx context.Context, id int64) (*CommandResult, error)

	// 操作存档
	RecordOperation(ctx context.Context, sessionID, toolName, filePath, status, errMsg string, params map[string]any) error
	GetOperations(ctx context.Context, page, size int) ([]OperationResult, int, error)

	Close()
}

type pgxStore struct {
	pool *pgxpool.Pool
}

var migrations = []string{
	`CREATE TABLE IF NOT EXISTS users (
		id            BIGSERIAL PRIMARY KEY,
		username      VARCHAR(50) UNIQUE NOT NULL,
		password_hash VARCHAR(255) NOT NULL,
		email         VARCHAR(100),
		is_active     BOOLEAN DEFAULT TRUE,
		created_at    TIMESTAMPTZ DEFAULT NOW(),
		updated_at    TIMESTAMPTZ DEFAULT NOW()
	)`,
	`CREATE TABLE IF NOT EXISTS token_blacklist (
		id            BIGSERIAL PRIMARY KEY,
		token_jti     VARCHAR(255) UNIQUE NOT NULL,
		expires_at    TIMESTAMPTZ NOT NULL,
		created_at    TIMESTAMPTZ DEFAULT NOW()
	)`,

	// 操作存档表
	`CREATE TABLE IF NOT EXISTS operations (
		id         BIGSERIAL PRIMARY KEY,
		session_id TEXT,
		tool_name  TEXT NOT NULL,
		file_path  TEXT,
		params     JSONB DEFAULT '{}',
		status     TEXT NOT NULL,
		error      TEXT,
		created_at TIMESTAMPTZ DEFAULT NOW()
	)`,
	`CREATE INDEX IF NOT EXISTS idx_operations_created_at ON operations (created_at DESC)`,
	`CREATE INDEX IF NOT EXISTS idx_operations_session ON operations (session_id)`,

	// 命令记录表（无 users 外键：MCP 写入的是 QQ 用户 ID，不在 users 表）
	`CREATE TABLE IF NOT EXISTS commands (
		id            BIGSERIAL PRIMARY KEY,
		user_id       INTEGER,
		source        VARCHAR(20) DEFAULT 'qq',
		command_text  TEXT NOT NULL,
		command_type  VARCHAR(30) DEFAULT 'shell',
		status        VARCHAR(20) DEFAULT 'pending',
		result        TEXT,
		error_message TEXT,
		exit_code     INTEGER,
		environment   JSONB DEFAULT '{}',
		created_at    TIMESTAMPTZ DEFAULT NOW(),
		finished_at   TIMESTAMPTZ
	)`,
	// 兼容旧表：若 commands 已存在但缺 source 列则补齐
	`ALTER TABLE commands ADD COLUMN IF NOT EXISTS source VARCHAR(20) DEFAULT 'qq'`,
	// 兼容旧表：去掉 user_id 外键（MCP 写入的是 QQ 用户 ID，不在 users 表）
	`ALTER TABLE commands DROP CONSTRAINT IF EXISTS commands_user_id_fkey`,
}

func New(ctx context.Context, dsn string, maxConns int32) (Store, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}
	if maxConns > 0 {
		cfg.MaxConns = maxConns
	}
	// 连接超时：连不上库时快速失败，避免无限挂起（例如默认 host "postgres" 在宿主机上无法解析）
	if cfg.ConnConfig.ConnectTimeout == 0 {
		cfg.ConnConfig.ConnectTimeout = 5 * time.Second
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}

	for _, stmt := range migrations {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			pool.Close()
			return nil, fmt.Errorf("migrate: %w", err)
		}
	}

	return &pgxStore{pool: pool}, nil
}

func (s *pgxStore) Close() {
	s.pool.Close()
}
