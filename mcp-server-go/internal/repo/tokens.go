package repo

import (
	"context"
	"time"
)

func (s *pgxStore) InsertBlacklist(ctx context.Context, jti string, expiresAt time.Time) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO token_blacklist (token_jti, expires_at) VALUES ($1, $2)`,
		jti, expiresAt)
	return err
}

func (s *pgxStore) IsTokenBlacklisted(ctx context.Context, jti string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM token_blacklist WHERE token_jti=$1)`, jti).Scan(&exists)
	return exists, err
}
