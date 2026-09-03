// 文件：mcp-server-go/internal/repo/schema.go —— 启动前 schema 校验：数据模型字段与数据库对账
// 修改：2026-09-03（日期由 fresh-header.ps1 刷新）

package repo

import (
	"context"
	"log/slog"
	"sort"

	"github.com/jackc/pgx/v5/pgxpool"
)

// expectedSchema 定义数据模型期望的字段（启动前用于校验数据库是否对得上）。
var expectedSchema = map[string][]string{
	"users":           {"id", "uuid", "username", "password_hash", "email", "is_active", "created_at", "updated_at"},
	"token_blacklist": {"id", "uuid", "token_jti", "expires_at", "created_at"},
	"operations":      {"id", "uuid", "session_id", "tool_name", "file_path", "params", "status", "error", "created_at"},
	"commands":        {"id", "uuid", "user_id", "source", "command_text", "command_type", "status", "result", "error_message", "exit_code", "environment", "created_at", "finished_at"},
	"file_metadata":   {"id", "uuid", "file_path", "scope", "owner_id", "title", "description", "tags", "file_type", "mime_type", "extension", "size_bytes", "checksum", "session_id", "user_id", "attributes", "copied_from", "download_count", "last_accessed_at", "expires_at", "is_deleted", "deleted_at", "created_at", "updated_at"},
}

// checkSchema 启动前校验数据库字段与数据模型是否一致，不一致打 WARN 日志（不阻断启动）。
func checkSchema(ctx context.Context, pool *pgxpool.Pool) {
	totalMissing, totalExtra := 0, 0
	for table, want := range expectedSchema {
		missing, extra, err := diffColumns(ctx, pool, table, want)
		if err != nil {
			slog.Warn("schema check failed", "table", table, "error", err)
			continue
		}
		if len(missing) > 0 {
			totalMissing += len(missing)
			slog.Warn("schema mismatch: 数据库缺少字段", "table", table, "missing", missing)
		}
		if len(extra) > 0 {
			totalExtra += len(extra)
			slog.Warn("schema mismatch: 数据库多余字段", "table", table, "extra", extra)
		}
	}
	if totalMissing == 0 && totalExtra == 0 {
		slog.Info("schema check passed: 数据库字段与数据模型一致")
	} else {
		slog.Warn("schema check found mismatches", "missing", totalMissing, "extra", totalExtra)
	}
}

func diffColumns(ctx context.Context, pool *pgxpool.Pool, table string, want []string) (missing, extra []string, err error) {
	rows, err := pool.Query(ctx,
		`SELECT column_name FROM information_schema.columns WHERE table_schema='public' AND table_name=$1`, table)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	actual := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, nil, err
		}
		actual[name] = true
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	wantSet := make(map[string]bool, len(want))
	for _, c := range want {
		wantSet[c] = true
		if !actual[c] {
			missing = append(missing, c)
		}
	}
	for c := range actual {
		if !wantSet[c] {
			extra = append(extra, c)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	return missing, extra, nil
}
