// 文件：mcp-server-go/internal/repo/groups.go —— groups / group_members 表存取：group 可见性的成员面（权限批次）
// 修改：2026-09-06（日期由 fresh-header.ps1 刷新）

package repo

import (
	"context"
	"time"
)

// Group 组（group 可见性文件的共享圈）。
type Group struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *pgxStore) CreateGroup(ctx context.Context, name string) (*Group, error) {
	g := &Group{}
	err := s.pool.QueryRow(ctx,
		`INSERT INTO groups (name) VALUES ($1) RETURNING id, name, created_at`,
		name).Scan(&g.ID, &g.Name, &g.CreatedAt)
	if err != nil {
		return nil, err
	}
	return g, nil
}

func (s *pgxStore) ListGroups(ctx context.Context) ([]Group, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, name, created_at FROM groups ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Group{}
	for rows.Next() {
		var g Group
		if err := rows.Scan(&g.ID, &g.Name, &g.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *pgxStore) DeleteGroup(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM groups WHERE id=$1`, id)
	return err
}

// AddGroupMember 幂等入组（ON CONFLICT no-op）。
func (s *pgxStore) AddGroupMember(ctx context.Context, groupID, userID int64) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO group_members (group_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		groupID, userID)
	return err
}

func (s *pgxStore) RemoveGroupMember(ctx context.Context, groupID, userID int64) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM group_members WHERE group_id=$1 AND user_id=$2`, groupID, userID)
	return err
}

// UserGroupIDs 用户所属组的 id 列表（Principal.GroupIDs 的库侧来源，
// 认证中间件每请求调用——主键索引查询，个人库量级无压力）。
func (s *pgxStore) UserGroupIDs(ctx context.Context, userID int64) ([]int64, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT group_id FROM group_members WHERE user_id=$1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []int64{}
	for rows.Next() {
		var gid int64
		if err := rows.Scan(&gid); err != nil {
			return nil, err
		}
		out = append(out, gid)
	}
	return out, rows.Err()
}
