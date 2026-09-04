// 文件：mcp-server-go/internal/repo/users.go —— users 表存取：用户模型 / 按用户名查询
// 修改：2026-09-03（日期由 fresh-header.ps1 刷新）

package repo

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

type User struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	Email        *string   `json:"email"`
	IsActive     bool      `json:"is_active"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func scanUser(row pgx.Row) (*User, error) {
	var u User
	if err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Email, &u.IsActive, &u.CreatedAt, &u.UpdatedAt); err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *pgxStore) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	return scanUser(s.pool.QueryRow(ctx,
		`SELECT id, username, password_hash, email, is_active, created_at, updated_at
		 FROM users WHERE username=$1`, username))
}

func (s *pgxStore) CreateUser(ctx context.Context, username, passwordHash, email string) (*User, error) {
	return scanUser(s.pool.QueryRow(ctx,
		`INSERT INTO users (username, password_hash, email) VALUES ($1, $2, $3)
		 RETURNING id, username, password_hash, email, is_active, created_at, updated_at`,
		username, passwordHash, email))
}
