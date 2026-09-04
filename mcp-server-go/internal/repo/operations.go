// 文件：mcp-server-go/internal/repo/operations.go —— operations 表：工具调用审计记录
// 修改：2026-09-03（日期由 fresh-header.ps1 刷新）

package repo

import (
	"context"
	"encoding/json"
	"time"
)

func (s *pgxStore) RecordOperation(ctx context.Context, sessionID, toolName, filePath, status, errMsg string, params map[string]any) error {
	p, _ := json.Marshal(params)
	_, err := s.pool.Exec(ctx,
		`INSERT INTO operations (session_id, tool_name, file_path, params, status, error, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		sessionID, toolName, filePath, p, status, errMsg, time.Now(),
	)
	return err
}

type OperationResult struct {
	ID        int64           `json:"id"`
	SessionID *string         `json:"session_id"`
	ToolName  string          `json:"tool_name"`
	FilePath  *string         `json:"file_path"`
	Params    json.RawMessage `json:"params"`
	Status    string          `json:"status"`
	Error     *string         `json:"error"`
	CreatedAt time.Time       `json:"created_at"`
}

func (s *pgxStore) GetOperations(ctx context.Context, page, size int) ([]OperationResult, int, error) {
	var total int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM operations`).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * size
	rows, err := s.pool.Query(ctx,
		`SELECT id, session_id, tool_name, file_path, params, status, error, created_at
		 FROM operations ORDER BY created_at DESC, id DESC LIMIT $1 OFFSET $2`,
		size, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []OperationResult
	for rows.Next() {
		var r OperationResult
		if err := rows.Scan(&r.ID, &r.SessionID, &r.ToolName, &r.FilePath, &r.Params, &r.Status, &r.Error, &r.CreatedAt); err != nil {
			return nil, 0, err
		}
		items = append(items, r)
	}
	return items, total, rows.Err()
}
