package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Store 是数据访问层接口，便于 mock 测试与替换实现。
type Store interface {
	RecordOperation(ctx context.Context, sessionID, toolName, filePath, status, errMsg string, params map[string]any) error
	GetOperations(ctx context.Context, page, size int) ([]OperationResult, int, error)
	InsertCommand(ctx context.Context, userID int, command string) (int64, error)
	UpdateCommand(ctx context.Context, id int64, status, result, errMsg string, exitCode int) error
	GetCommands(ctx context.Context, userID, limit int) ([]CommandResult, error)
	Close()
}

type pgxStore struct {
	pool *pgxpool.Pool
}

var migrations = []string{
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

	// 命令记录表（Go 自包含，无 users 外键）
	`CREATE TABLE IF NOT EXISTS commands (
		id            BIGSERIAL PRIMARY KEY,
		user_id       INTEGER,
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
}

func New(ctx context.Context, dsn string, maxConns int32) (Store, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}
	if maxConns > 0 {
		cfg.MaxConns = maxConns
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
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
