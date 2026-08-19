package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var migrations = []string{
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
}

type Store struct {
	pool *pgxpool.Pool
}

func New(ctx context.Context, dsn string, maxConns int32) (*Store, error) {
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

	return &Store{pool: pool}, nil
}

func (s *Store) RecordOperation(ctx context.Context, sessionID, toolName, filePath, status, errMsg string, params map[string]any) error {
	p, _ := json.Marshal(params)
	_, err := s.pool.Exec(ctx,
		`INSERT INTO operations (session_id, tool_name, file_path, params, status, error, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		sessionID, toolName, filePath, p, status, errMsg, time.Now(),
	)
	return err
}

func (s *Store) Close() {
	s.pool.Close()
}
