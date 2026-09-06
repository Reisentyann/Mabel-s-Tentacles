// 文件：mcp-server-go/internal/repo/users.go —— users 表存取：角色模型 / 按名与按 ID 查询 / 封停 / 列表
// 修改：2026-09-06（日期由 fresh-header.ps1 刷新）

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
	Role         string    `json:"role"` // admin / user（authz 矩阵的输入）
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

const userColumns = `id, username, password_hash, email, is_active, role, created_at, updated_at`

func scanUser(row pgx.Row) (*User, error) {
	var u User
	if err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Email, &u.IsActive, &u.Role, &u.CreatedAt, &u.UpdatedAt); err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *pgxStore) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	return scanUser(s.pool.QueryRow(ctx,
		`SELECT `+userColumns+` FROM users WHERE username=$1`, username))
}

func (s *pgxStore) GetUserByID(ctx context.Context, id int64) (*User, error) {
	return scanUser(s.pool.QueryRow(ctx,
		`SELECT `+userColumns+` FROM users WHERE id=$1`, id))
}

// CreateUser 建号（role 由调用方给：register 一律 "user"，bootstrap admin 给 "admin"）。
func (s *pgxStore) CreateUser(ctx context.Context, username, passwordHash, email, role string) (*User, error) {
	if role == "" {
		role = "user"
	}
	var emailPtr *string
	if email != "" {
		emailPtr = &email
	}
	return scanUser(s.pool.QueryRow(ctx,
		`INSERT INTO users (username, password_hash, email, role) VALUES ($1, $2, $3, $4)
		 RETURNING `+userColumns,
		username, passwordHash, emailPtr, role))
}

func (s *pgxStore) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+userColumns+` FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *u)
	}
	return out, rows.Err()
}

// SetUserActive 封停/解封（requireAuth 每请求查库，实时生效）。
func (s *pgxStore) SetUserActive(ctx context.Context, username string, active bool) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE users SET is_active=$1, updated_at=NOW() WHERE username=$2`, active, username)
	return err
}

// SetUserRole 角色调整（bootstrap admin 启动时自提升用）。
func (s *pgxStore) SetUserRole(ctx context.Context, username, role string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE users SET role=$1, updated_at=NOW() WHERE username=$2`, role, username)
	return err
}
