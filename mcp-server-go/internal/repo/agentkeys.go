// 文件：mcp-server-go/internal/repo/agentkeys.go —— agent_keys 表存取：外部 agent 受限 key（raw 只见一次，库存 sha256）
// 修改：2026-09-06（日期由 fresh-header.ps1 刷新）

package repo

import (
	"context"
	"time"
)

// AgentKey 外部 agent 的受限 key（绑定一个用户身份，权限=该用户）。
// KeyHash 为 raw key 的 sha256 hex——raw 只在 admin 签发响应里出现一次，
// 任何查询/列表都不回传（json 标签 "-"）。
type AgentKey struct {
	ID           int64     `json:"id"`
	Name         string    `json:"name"`
	KeyHash      string    `json:"-"`
	PrincipalUID int64     `json:"principal_uid"`
	IsActive     bool      `json:"is_active"`
	CreatedAt    time.Time `json:"created_at"`
}

func (s *pgxStore) CreateAgentKey(ctx context.Context, name, keyHash string, principalUID int64) (*AgentKey, error) {
	k := &AgentKey{}
	err := s.pool.QueryRow(ctx,
		`INSERT INTO agent_keys (name, key_hash, principal_uid) VALUES ($1, $2, $3)
		 RETURNING id, name, key_hash, principal_uid, is_active, created_at`,
		name, keyHash, principalUID).
		Scan(&k.ID, &k.Name, &k.KeyHash, &k.PrincipalUID, &k.IsActive, &k.CreatedAt)
	if err != nil {
		return nil, err
	}
	return k, nil
}

// GetAgentKeyByHash MCP 通道验 key 用（hash 直查，unique 索引 O(1)）。
func (s *pgxStore) GetAgentKeyByHash(ctx context.Context, keyHash string) (*AgentKey, error) {
	k := &AgentKey{}
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, key_hash, principal_uid, is_active, created_at
		 FROM agent_keys WHERE key_hash=$1`, keyHash).
		Scan(&k.ID, &k.Name, &k.KeyHash, &k.PrincipalUID, &k.IsActive, &k.CreatedAt)
	if err != nil {
		return nil, err
	}
	return k, nil
}

func (s *pgxStore) ListAgentKeys(ctx context.Context) ([]AgentKey, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, key_hash, principal_uid, is_active, created_at FROM agent_keys ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AgentKey{}
	for rows.Next() {
		var k AgentKey
		if err := rows.Scan(&k.ID, &k.Name, &k.KeyHash, &k.PrincipalUID, &k.IsActive, &k.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// RevokeAgentKey 吊销（is_active=false；MCP 侧验 key 即时拒绝）。
func (s *pgxStore) RevokeAgentKey(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `UPDATE agent_keys SET is_active=FALSE WHERE id=$1`, id)
	return err
}
