package repo

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
)

type CommandResult struct {
	ID           int64           `json:"id"`
	UserID       *int32          `json:"user_id"`
	Source       string          `json:"source"`
	CommandText  string          `json:"command_text"`
	CommandType  string          `json:"command_type"`
	Status       string          `json:"status"`
	Result       *string         `json:"result"`
	ErrorMessage *string         `json:"error_message"`
	ExitCode     *int32          `json:"exit_code"`
	Environment  json.RawMessage `json:"environment"`
	CreatedAt    time.Time       `json:"created_at"`
	FinishedAt   *time.Time      `json:"finished_at"`
}

const commandColumns = `id, user_id, source, command_text, command_type, status, result, error_message, exit_code, environment, created_at, finished_at`

func scanCommand(row pgx.Row) (*CommandResult, error) {
	var c CommandResult
	if err := row.Scan(&c.ID, &c.UserID, &c.Source, &c.CommandText, &c.CommandType, &c.Status,
		&c.Result, &c.ErrorMessage, &c.ExitCode, &c.Environment, &c.CreatedAt, &c.FinishedAt); err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *pgxStore) InsertCommand(ctx context.Context, userID int, command string) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx,
		`INSERT INTO commands (user_id, command_text, status) VALUES ($1, $2, 'running') RETURNING id`,
		userID, command,
	).Scan(&id)
	return id, err
}

func (s *pgxStore) UpdateCommand(ctx context.Context, id int64, status, result, errMsg string, exitCode int) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE commands SET status=$1, result=$2, error_message=$3, exit_code=$4, finished_at=NOW() WHERE id=$5`,
		status, result, errMsg, exitCode, id,
	)
	return err
}

func (s *pgxStore) GetCommands(ctx context.Context, userID, limit int) ([]CommandResult, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+commandColumns+` FROM commands WHERE user_id=$1 ORDER BY created_at DESC LIMIT $2`,
		userID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []CommandResult
	for rows.Next() {
		c, err := scanCommand(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, *c)
	}
	return results, rows.Err()
}

func (s *pgxStore) ListCommands(ctx context.Context, page, size int) ([]CommandResult, int, error) {
	var total int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM commands`).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * size
	rows, err := s.pool.Query(ctx,
		`SELECT `+commandColumns+` FROM commands ORDER BY created_at DESC, id DESC LIMIT $1 OFFSET $2`,
		size, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []CommandResult
	for rows.Next() {
		c, err := scanCommand(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, *c)
	}
	return items, total, rows.Err()
}

func (s *pgxStore) GetCommand(ctx context.Context, id int64) (*CommandResult, error) {
	return scanCommand(s.pool.QueryRow(ctx,
		`SELECT `+commandColumns+` FROM commands WHERE id=$1`, id))
}
