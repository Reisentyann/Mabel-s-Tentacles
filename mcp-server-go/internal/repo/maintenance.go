// 文件：mcp-server-go/internal/repo/maintenance.go —— UUID 文件维护意图的持久记录与重试状态
// 修改：2026-10-09（日期由 fresh-header.ps1 刷新）

package repo

import (
	"context"
)

// MaintenanceJob 不保存文件内容，重试仅分析 UUID 当前的已发布文件。
type MaintenanceJob struct {
	ID       int64
	UUID     string
	Kind     string
	Session  string
	User     string
	Attempts int
}

// MaintenanceStore 单进程维护任务契约；prepared 在文件操作前落库。
type MaintenanceStore interface {
	PrepareMaintenance(context.Context, string, string, string, string) (int64, error)
	FinishMaintenance(context.Context, int64, string, string) error
	PendingMaintenance(context.Context, int) ([]MaintenanceJob, error)
	RecoverMaintenanceClaims(context.Context) error
}

func (s *pgxStore) PrepareMaintenance(ctx context.Context, uuid, kind, session, user string) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `INSERT INTO metadata_maintenance_jobs(uuid,kind,session_id,actor,status)
		VALUES($1,$2,$3,$4,'prepared') RETURNING id`, uuid, kind, session, user).Scan(&id)
	return id, err
}

func (s *pgxStore) FinishMaintenance(ctx context.Context, id int64, status, message string) error {
	_, err := s.pool.Exec(ctx, `UPDATE metadata_maintenance_jobs SET status=$2,last_error=$3,
		attempts=attempts+CASE WHEN $2='retry' THEN 1 ELSE 0 END,
		next_attempt_at=CASE WHEN $2='retry' THEN NOW()+LEAST(POWER(2,attempts)::INT,60)*INTERVAL '1 minute' ELSE NOW() END,
		updated_at=NOW() WHERE id=$1 AND status NOT IN ('done','cancelled')`, id, status, message)
	return err
}

func (s *pgxStore) PendingMaintenance(ctx context.Context, limit int) ([]MaintenanceJob, error) {
	rows, err := s.pool.Query(ctx, `WITH candidates AS (
		SELECT id FROM metadata_maintenance_jobs
		WHERE status='ready' OR (status='retry' AND attempts<3 AND next_attempt_at<=NOW())
		ORDER BY id LIMIT $1 FOR UPDATE SKIP LOCKED
	), claimed AS (
		UPDATE metadata_maintenance_jobs j SET status='processing', updated_at=NOW()
		FROM candidates c WHERE j.id=c.id
		RETURNING j.id,j.uuid,j.kind,j.session_id,j.actor,j.attempts
	) SELECT id,uuid,kind,session_id,actor,attempts FROM claimed ORDER BY id`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := []MaintenanceJob{}
	for rows.Next() {
		var j MaintenanceJob
		if err := rows.Scan(&j.ID, &j.UUID, &j.Kind, &j.Session, &j.User, &j.Attempts); err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

// RecoverMaintenanceClaims makes interrupted single-process claims retryable at startup.
func (s *pgxStore) RecoverMaintenanceClaims(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `UPDATE metadata_maintenance_jobs SET status='ready',updated_at=NOW() WHERE status IN ('processing','prepared')`)
	return err
}
