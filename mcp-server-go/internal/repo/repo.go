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

	// 文件元数据
	UpsertMetadata(ctx context.Context, m *FileMetadata) error
	GetMetadata(ctx context.Context, filePath string) (*FileMetadata, error)
	GetMetadataByPaths(ctx context.Context, paths []string) (map[string]*FileMetadata, error)
	SearchFiles(ctx context.Context, fs FileSearch) ([]FileMetadata, int, error)
	CopyMetadata(ctx context.Context, source, target, sessionID, userID string) error
	SoftDeleteMetadata(ctx context.Context, filePath string) error
	IncrementDownloadCount(ctx context.Context, filePath string) error

	Close()
}

type pgxStore struct {
	pool *pgxpool.Pool
}

var migrations = []string{
	// ---- users ----
	`CREATE TABLE IF NOT EXISTS users (
		id            BIGSERIAL PRIMARY KEY,
		uuid          UUID NOT NULL DEFAULT gen_random_uuid(),
		username      VARCHAR(50) UNIQUE NOT NULL,
		password_hash VARCHAR(255) NOT NULL,
		email         VARCHAR(100),
		is_active     BOOLEAN NOT NULL DEFAULT TRUE,
		created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`,
	`ALTER TABLE users ADD COLUMN IF NOT EXISTS uuid UUID NOT NULL DEFAULT gen_random_uuid()`,
	`UPDATE users SET is_active = COALESCE(is_active, TRUE), created_at = COALESCE(created_at, NOW()), updated_at = COALESCE(updated_at, NOW())`,
	`ALTER TABLE users ALTER COLUMN is_active SET NOT NULL`,
	`ALTER TABLE users ALTER COLUMN created_at SET NOT NULL`,
	`ALTER TABLE users ALTER COLUMN updated_at SET NOT NULL`,

	// ---- token_blacklist ----
	`CREATE TABLE IF NOT EXISTS token_blacklist (
		id            BIGSERIAL PRIMARY KEY,
		uuid          UUID NOT NULL DEFAULT gen_random_uuid(),
		token_jti     VARCHAR(255) UNIQUE NOT NULL,
		expires_at    TIMESTAMPTZ NOT NULL,
		created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`,
	`ALTER TABLE token_blacklist ADD COLUMN IF NOT EXISTS uuid UUID NOT NULL DEFAULT gen_random_uuid()`,
	`UPDATE token_blacklist SET created_at = COALESCE(created_at, NOW())`,
	`ALTER TABLE token_blacklist ALTER COLUMN created_at SET NOT NULL`,

	// ---- 操作存档表 ----
	`CREATE TABLE IF NOT EXISTS operations (
		id         BIGSERIAL PRIMARY KEY,
		uuid       UUID NOT NULL DEFAULT gen_random_uuid(),
		session_id TEXT,
		tool_name  TEXT NOT NULL,
		file_path  TEXT,
		params     JSONB NOT NULL DEFAULT '{}',
		status     TEXT NOT NULL,
		error      TEXT,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`,
	`ALTER TABLE operations ADD COLUMN IF NOT EXISTS uuid UUID NOT NULL DEFAULT gen_random_uuid()`,
	`UPDATE operations SET params = COALESCE(params, '{}'::jsonb), created_at = COALESCE(created_at, NOW())`,
	`ALTER TABLE operations ALTER COLUMN params SET NOT NULL`,
	`ALTER TABLE operations ALTER COLUMN created_at SET NOT NULL`,
	`CREATE INDEX IF NOT EXISTS idx_operations_created_at ON operations (created_at DESC)`,
	`CREATE INDEX IF NOT EXISTS idx_operations_session ON operations (session_id)`,

	// ---- 命令记录表（无 users 外键：MCP 写入的是 QQ 用户 ID，不在 users 表）----
	`CREATE TABLE IF NOT EXISTS commands (
		id            BIGSERIAL PRIMARY KEY,
		uuid          UUID NOT NULL DEFAULT gen_random_uuid(),
		user_id       INTEGER,
		source        VARCHAR(20) NOT NULL DEFAULT 'qq',
		command_text  TEXT NOT NULL,
		command_type  VARCHAR(30) NOT NULL DEFAULT 'shell',
		status        VARCHAR(20) NOT NULL DEFAULT 'pending',
		result        TEXT,
		error_message TEXT,
		exit_code     INTEGER,
		environment   JSONB NOT NULL DEFAULT '{}',
		created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		finished_at   TIMESTAMPTZ
	)`,
	// 兼容旧表：若 commands 已存在但缺 source 列则补齐
	`ALTER TABLE commands ADD COLUMN IF NOT EXISTS source VARCHAR(20) DEFAULT 'qq'`,
	// 兼容旧表：去掉 user_id 外键（MCP 写入的是 QQ 用户 ID，不在 users 表）
	`ALTER TABLE commands DROP CONSTRAINT IF EXISTS commands_user_id_fkey`,
	`ALTER TABLE commands ADD COLUMN IF NOT EXISTS uuid UUID NOT NULL DEFAULT gen_random_uuid()`,
	`UPDATE commands SET source = COALESCE(source, 'qq'), command_type = COALESCE(command_type, 'shell'), status = COALESCE(status, 'pending'), environment = COALESCE(environment, '{}'::jsonb), created_at = COALESCE(created_at, NOW())`,
	`ALTER TABLE commands ALTER COLUMN source SET NOT NULL`,
	`ALTER TABLE commands ALTER COLUMN command_type SET NOT NULL`,
	`ALTER TABLE commands ALTER COLUMN status SET NOT NULL`,
	`ALTER TABLE commands ALTER COLUMN environment SET NOT NULL`,
	`ALTER TABLE commands ALTER COLUMN created_at SET NOT NULL`,

	// ---- 文件元数据表（描述/标签/属性/软删除）----
	`CREATE TABLE IF NOT EXISTS file_metadata (
		id               BIGSERIAL PRIMARY KEY,
		uuid             UUID NOT NULL DEFAULT gen_random_uuid(),
		file_path        TEXT UNIQUE NOT NULL,
		scope            TEXT NOT NULL DEFAULT 'global',
		owner_id         TEXT,
		title            TEXT,
		description      TEXT,
		tags             TEXT[] DEFAULT '{}',
		file_type        TEXT,
		mime_type        TEXT,
		extension        TEXT,
		size_bytes       BIGINT,
		checksum         TEXT,
		session_id       TEXT,
		user_id          TEXT,
		attributes       JSONB NOT NULL DEFAULT '{}',
		copied_from      TEXT,
		download_count   BIGINT NOT NULL DEFAULT 0,
		last_accessed_at TIMESTAMPTZ,
		expires_at       TIMESTAMPTZ,
		is_deleted       BOOLEAN NOT NULL DEFAULT FALSE,
		deleted_at       TIMESTAMPTZ,
		created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`,
	// 兼容旧表：补齐新增字段
	`ALTER TABLE file_metadata ADD COLUMN IF NOT EXISTS scope TEXT NOT NULL DEFAULT 'global'`,
	`ALTER TABLE file_metadata ADD COLUMN IF NOT EXISTS owner_id TEXT`,
	`ALTER TABLE file_metadata ADD COLUMN IF NOT EXISTS title TEXT`,
	`ALTER TABLE file_metadata ADD COLUMN IF NOT EXISTS extension TEXT`,
	`ALTER TABLE file_metadata ADD COLUMN IF NOT EXISTS checksum TEXT`,
	`ALTER TABLE file_metadata ADD COLUMN IF NOT EXISTS download_count BIGINT DEFAULT 0`,
	`ALTER TABLE file_metadata ADD COLUMN IF NOT EXISTS last_accessed_at TIMESTAMPTZ`,
	`ALTER TABLE file_metadata ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ`,
	`ALTER TABLE file_metadata ADD COLUMN IF NOT EXISTS uuid UUID NOT NULL DEFAULT gen_random_uuid()`,
	`UPDATE file_metadata SET attributes = COALESCE(attributes, '{}'::jsonb), download_count = COALESCE(download_count, 0), is_deleted = COALESCE(is_deleted, FALSE), created_at = COALESCE(created_at, NOW()), updated_at = COALESCE(updated_at, NOW())`,
	`ALTER TABLE file_metadata ALTER COLUMN attributes SET NOT NULL`,
	`ALTER TABLE file_metadata ALTER COLUMN download_count SET NOT NULL`,
	`ALTER TABLE file_metadata ALTER COLUMN is_deleted SET NOT NULL`,
	`ALTER TABLE file_metadata ALTER COLUMN created_at SET NOT NULL`,
	`ALTER TABLE file_metadata ALTER COLUMN updated_at SET NOT NULL`,
	`CREATE INDEX IF NOT EXISTS idx_meta_tags  ON file_metadata USING GIN (tags)`,
	`CREATE INDEX IF NOT EXISTS idx_meta_attrs ON file_metadata USING GIN (attributes)`,
	`CREATE INDEX IF NOT EXISTS idx_meta_type  ON file_metadata (file_type)`,
	`CREATE INDEX IF NOT EXISTS idx_meta_by    ON file_metadata (user_id)`,
	`CREATE INDEX IF NOT EXISTS idx_meta_del   ON file_metadata (is_deleted)`,
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

	checkSchema(ctx, pool)

	return &pgxStore{pool: pool}, nil
}

func (s *pgxStore) Close() {
	s.pool.Close()
}
