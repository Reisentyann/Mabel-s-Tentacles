// 文件：mcp-server-go/internal/repo/maintenance_test.go —— PostgreSQL 维护任务认领、恢复、退避与完成隔离验证
// 修改：2026-10-09（日期由 fresh-header.ps1 刷新）

package repo

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestMaintenancePostgres(t *testing.T) {
	dsn := os.Getenv("MABEL_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set MABEL_TEST_DATABASE_URL for PostgreSQL integration")
	}
	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal("invalid test DB configuration")
	}
	cfg.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal("connect test DB failed")
	}
	defer pool.Close()
	for _, stmt := range migrations {
		if strings.HasPrefix(stmt, "CREATE TABLE IF NOT EXISTS metadata_maintenance_jobs") {
			_, err := pool.Exec(ctx, `CREATE TEMP TABLE metadata_maintenance_jobs (
				id BIGSERIAL PRIMARY KEY, uuid UUID NOT NULL, kind TEXT NOT NULL,
				session_id TEXT NOT NULL DEFAULT '', actor TEXT NOT NULL DEFAULT '',
				status TEXT NOT NULL CHECK(status IN ('prepared','ready','retry','done','cancelled')),
				attempts INT NOT NULL DEFAULT 0, last_error TEXT NOT NULL DEFAULT '',
				next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
			)`)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `INSERT INTO metadata_maintenance_jobs(uuid,kind,status) VALUES($1,'legacy','done')`, "00000000-0000-0000-0000-000000000099"); err != nil {
				t.Fatal(err)
			}
		}
		if strings.HasPrefix(stmt, "ALTER TABLE metadata_maintenance_jobs DROP CONSTRAINT") || strings.HasPrefix(stmt, "ALTER TABLE metadata_maintenance_jobs ADD CONSTRAINT") {
			if _, err := pool.Exec(ctx, stmt); err != nil {
				t.Fatalf("upgrade old maintenance status constraint: %v", err)
			}
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO metadata_maintenance_jobs(uuid,kind,status) VALUES($1,'processing-check','processing')`, "00000000-0000-0000-0000-000000000098"); err != nil {
		t.Fatalf("upgraded constraint rejects processing: %v", err)
	}
	st := &pgxStore{pool: pool}
	id, err := st.PrepareMaintenance(ctx, "00000000-0000-0000-0000-000000000001", "modify", "s", "alice")
	if err != nil {
		t.Fatal(err)
	}
	jobs, err := st.PendingMaintenance(ctx, 10)
	if err != nil || len(jobs) != 0 {
		t.Fatalf("prepared task claimed: %v %v", jobs, err)
	}
	if err := st.RecoverMaintenanceClaims(ctx); err != nil {
		t.Fatal(err)
	}
	jobs, err = st.PendingMaintenance(ctx, 10)
	if err != nil || len(jobs) != 1 || jobs[0].ID != id {
		t.Fatalf("recovery claim: %v %v", jobs, err)
	}
	jobs, err = st.PendingMaintenance(ctx, 10)
	if err != nil || len(jobs) != 0 {
		t.Fatalf("duplicate claim: %v %v", jobs, err)
	}
	if err := st.FinishMaintenance(ctx, id, "retry", "injected"); err != nil {
		t.Fatal(err)
	}
	jobs, err = st.PendingMaintenance(ctx, 10)
	if err != nil || len(jobs) != 0 {
		t.Fatalf("backoff ignored: %v %v", jobs, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE metadata_maintenance_jobs SET next_attempt_at=NOW()-INTERVAL '1 minute' WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	jobs, err = st.PendingMaintenance(ctx, 10)
	if err != nil || len(jobs) != 1 || jobs[0].Attempts != 1 {
		t.Fatalf("retry claim: %v %v", jobs, err)
	}
	if err := st.FinishMaintenance(ctx, id, "done", ""); err != nil {
		t.Fatal(err)
	}
	if err := st.RecoverMaintenanceClaims(ctx); err != nil {
		t.Fatal(err)
	}
	jobs, err = st.PendingMaintenance(ctx, 10)
	if err != nil || len(jobs) != 0 {
		t.Fatalf("completed task revived: %v %v", jobs, err)
	}

	// Multiple operations for a UUID keep independent completion state.
	id1, err := st.PrepareMaintenance(ctx, "00000000-0000-0000-0000-000000000002", "modify", "s1", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.FinishMaintenance(ctx, id1, "ready", ""); err != nil {
		t.Fatal(err)
	}
	id2, err := st.PrepareMaintenance(ctx, "00000000-0000-0000-0000-000000000002", "move", "s2", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.FinishMaintenance(ctx, id2, "ready", ""); err != nil {
		t.Fatal(err)
	}
	jobs, err = st.PendingMaintenance(ctx, 10)
	if err != nil || len(jobs) != 2 {
		t.Fatalf("same-UUID jobs=%v err=%v", jobs, err)
	}
	if err := st.FinishMaintenance(ctx, id1, "done", ""); err != nil {
		t.Fatal(err)
	}
	if err := st.FinishMaintenance(ctx, id2, "done", ""); err != nil {
		t.Fatal(err)
	}
}
