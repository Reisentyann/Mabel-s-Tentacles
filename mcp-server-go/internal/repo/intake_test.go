// 文件：mcp-server-go/internal/repo/intake_test.go —— PostgreSQL 占位重名保护与管理机适配验证（临时表隔离）
// 修改：2026-10-08（日期由 fresh-header.ps1 刷新）

package repo

import (
	"context"
	"errors"
	"os"
	"testing"

	manager "github.com/Reisentyann/Mabel-s-Tentacles/manager-go"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestReserveMetaPostgres(t *testing.T) {
	dsn := os.Getenv("MABEL_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set MABEL_TEST_DATABASE_URL for PostgreSQL integration")
	}
	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal("invalid test database configuration")
	}
	cfg.MaxConns = 1 // 同一会话使用临时表，绝不接触正式 file_metadata。
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal("connect test database failed")
	}
	defer pool.Close()
	_, err = pool.Exec(ctx, `CREATE TEMP TABLE file_metadata (
		file_path text UNIQUE NOT NULL, uuid uuid DEFAULT gen_random_uuid(),
		missing_rounds integer DEFAULT 0, updated_at timestamptz DEFAULT now(),
		is_deleted boolean DEFAULT false, checksum text, title text, attributes jsonb DEFAULT '{}')`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `CREATE TEMP TABLE logical_directories(file_path text primary key,uuid uuid unique default gen_random_uuid(),is_deleted boolean default false);
	CREATE TEMP TABLE directory_files(file_uuid uuid primary key,parent_uuid uuid,name text)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `CREATE TEMP TABLE intake_operations (
		uuid uuid primary key, file_path text unique not null, status text not null,
		created_at timestamptz default now(), updated_at timestamptz default now())`)
	if err != nil {
		t.Fatal(err)
	}
	st := &pgxStore{pool: pool}
	a := NewManagerStore(st)
	uuid, err := a.ReserveMeta(ctx, "~alice/note.txt")
	if err != nil || uuid == "" {
		t.Fatalf("reserve = %q, %v", uuid, err)
	}
	if err := a.CompleteIntake(ctx, "~alice/note.txt", uuid); err != nil {
		t.Fatal(err)
	}
	for _, deleted := range []bool{false, true} {
		_, err = pool.Exec(ctx, `UPDATE file_metadata SET missing_rounds=2,
		 updated_at='2020-01-01', attributes='{"keep":true}', is_deleted=$1`, deleted)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := a.ReserveMeta(ctx, "~alice/note.txt"); !errors.Is(err, manager.ErrKeyExists) {
			t.Fatalf("conflict = %v", err)
		}
		var intact bool
		err = pool.QueryRow(ctx, `SELECT uuid=$1 AND missing_rounds=2 AND updated_at='2020-01-01'
		 AND attributes='{"keep":true}' AND is_deleted=$2 FROM file_metadata`, uuid, deleted).Scan(&intact)
		if err != nil || !intact {
			t.Fatalf("old metadata changed: %v", err)
		}
	}
	if _, err := a.ReserveMeta(ctx, "~bob/note.txt"); err != nil {
		t.Fatal("separate owner path rejected", err)
	}
	results := make(chan error, 12)
	for i := 0; i < 12; i++ {
		go func() {
			_, err := a.ReserveMeta(ctx, "~alice/concurrent.txt")
			results <- err
		}()
	}
	winners := 0
	for i := 0; i < 12; i++ {
		if err := <-results; err == nil {
			winners++
		} else if !errors.Is(err, manager.ErrKeyExists) {
			t.Fatal(err)
		}
	}
	if winners != 1 {
		t.Fatalf("reservation winners = %d", winners)
	}
	failedUUID, err := a.ReserveMeta(ctx, "retry.txt")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.ArchiveFailedIntake(ctx, "retry.txt", failedUUID); err != nil {
		t.Fatal(err)
	}
	nextUUID, err := a.ReserveMeta(ctx, "retry.txt")
	if err != nil || nextUUID == failedUUID {
		t.Fatalf("retry uuid=%s err=%v", nextUUID, err)
	}
	if err := a.ArchiveFailedIntake(ctx, "retry.txt", failedUUID); err == nil {
		t.Fatal("stale recovery must not move new reservation")
	}
	if err := a.CreateDirectory(ctx, "~alice"); err != nil {
		t.Fatal(err)
	}
	d, err := a.DirectoryByPath(ctx, "~alice")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.LinkDirectoryFile(ctx, d.UUID, uuid, "note.txt"); err != nil {
		t.Fatal(err)
	}
	items, err := a.DirectoryEntries(ctx, d.UUID)
	if err != nil || len(items) < 1 {
		t.Fatalf("entries=%v err=%v", items, err)
	}
}
