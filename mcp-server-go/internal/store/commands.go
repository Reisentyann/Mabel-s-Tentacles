package store

import (
	"context"
	"time"
)

type CommandResult struct {
	ID           int64      `json:"id"`
	CommandText  string     `json:"command_text"`
	Status       string     `json:"status"`
	Result       *string    `json:"result"`
	ErrorMessage *string    `json:"error_message"`
	ExitCode     *int32     `json:"exit_code"`
	CreatedAt    time.Time  `json:"created_at"`
	FinishedAt   *time.Time `json:"finished_at"`
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
		`SELECT id, command_text, status, result, error_message, exit_code, created_at, finished_at
		 FROM commands WHERE user_id=$1 ORDER BY created_at DESC LIMIT $2`,
		userID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []CommandResult
	for rows.Next() {
		var r CommandResult
		if err := rows.Scan(&r.ID, &r.CommandText, &r.Status, &r.Result, &r.ErrorMessage, &r.ExitCode, &r.CreatedAt, &r.FinishedAt); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}
