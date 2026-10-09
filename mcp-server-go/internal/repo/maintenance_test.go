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
			stmt = strings.Replace(stmt, "CREATE TABLE IF NOT EXISTS", "CREATE TEMP TABLE", 1)
			if _, err := pool.Exec(ctx, stmt); err != nil {
				t.Fatal(err)
			}
		}
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
}
