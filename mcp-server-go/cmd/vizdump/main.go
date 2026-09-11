// 文件：mcp-server-go/cmd/vizdump/main.go —— 开发者工具 vizdump：DB 元数据 + 盘面全量导出 → 单文件 HTML 可视化报告（逻辑键/描述/物理位对照）
// 修改：2026-09-11（日期由 fresh-header.ps1 刷新）

// vizdump 是观察者工具（开发者/运维用），不是服务链路的一环：
// 三机架构把信息分层藏了起来（逻辑键是 agent 寻址语言、物理位是 uuid
// 派生、描述事实在 DB JSONB）——人要直接观察时什么都看不见。本工具
// 一次性把三层摊平成一份可在浏览器打开的自包含 HTML：
//
//	文件卡（逻辑路径 → uuid → 物理绝对路径 → 盘上实况 → 描述全属性）
//	＋ 盘面核对（存在性 / 大小 / sha-256 对账，直改文件当场现形）
//	＋ 字段目录统计（描述机产出键的覆盖面）
//
// 数据源直读 PostgreSQL（含软删行，报告里打标）——观察工具读事实源
// 本体，不走 repo 接口面（那是服务链路的边界，不是查询语言）。
// 用法（仓库根）：
//
//	pwsh -NoProfile -File .\test\tools\run-vizdump.ps1     # 装配 .env 后跑
//	或直接：vizdump -env .env -data data -o fileviz.html
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Reisentyann/Mabel-s-Tentacles/manager-go"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/config"
)

// fileRow 报告的单文件视图：DB 行（含软删）+ 盘面核对 + 物理位推导。
type fileRow struct {
	LogicPath   string
	UUID        string
	Title       string
	Description string
	Tags        []string
	FileType    string
	MimeType    string
	Extension   string
	Scope       string
	OwnerID     string
	UserID      string
	GroupID     string
	Visibility  string
	SizeBytes   int64
	Checksum    string
	MissRounds  int
	IsDeleted   bool
	DeletedAt   *time.Time
	CopiedFrom  string
	DLCnt       int64
	UpdatedAt   time.Time
	CreatedAt   time.Time
	Attrs       map[string]any

	// 盘面核对（派生）
	StorageAbs string
	DiskOK     bool
	DiskSize   int64
	DiskMtime  time.Time
	HashMatch  string // ""=未核对(DB无checksum/盘缺) / "ok" / "drift"
}

func main() {
	start := time.Now()
	envPath := flag.String("env", ".env", ".env 路径（存在才解析；进程环境变量优先）")
	dataDir := flag.String("data", "", "数据目录（默认取 DATA_DIR / config 推导）")
	out := flag.String("o", "fileviz.html", "输出 HTML 路径")
	jsonOut := flag.String("json", "", "附带导出原始 JSON（空 = 不导）")
	flag.Parse()

	loadDotEnv(*envPath)
	cfg := config.Load()
	dir := *dataDir
	if dir == "" {
		dir = cfg.DataDir
	}
	if dir == "" {
		dir = "data"
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		slog.Error("resolve data dir failed", "dir", dir, "error", err)
		os.Exit(1)
	}

	ctx := context.Background()
	rows, err := dumpRows(ctx, cfg.DatabaseDSN(), absDir)
	if err != nil {
		slog.Error("dump failed", "error", err, "duration", time.Since(start).String())
		os.Exit(1)
	}

	if *jsonOut != "" {
		if err := writeJSON(*jsonOut, rows); err != nil {
			slog.Error("write json failed", "path", *jsonOut, "error", err)
			os.Exit(1)
		}
	}

	html, err := renderReport(absDir, rows)
	if err != nil {
		slog.Error("render report failed", "error", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*out, []byte(html), 0o644); err != nil {
		slog.Error("write report failed", "path", *out, "error", err)
		os.Exit(1)
	}

	active, deleted, ghost, drift := 0, 0, 0, 0
	for _, r := range rows {
		switch {
		case r.IsDeleted:
			deleted++
		default:
			active++
		}
		if !r.DiskOK {
			ghost++
		}
		if r.HashMatch == "drift" {
			drift++
		}
	}
	slog.Info("vizdump exported",
		"files", len(rows), "active", active, "soft_deleted", deleted,
		"missing_on_disk", ghost, "checksum_drift", drift,
		"report", *out, "bytes_out", len(html), "duration", time.Since(start).String())
	fmt.Printf("报告已生成：%s（%d 文件，软删 %d，盘缺 %d，校验漂移 %d）\n",
		absOf(*out), len(rows), deleted, ghost, drift)
}

// loadDotEnv 极简 .env 解析（与 run-idx-e2e.ps1 同口径）：KEY=VALUE，
// 注释/空行跳过；已存在的进程环境变量不覆盖。
func loadDotEnv(path string) {
	b, err := os.ReadFile(path)
	if err != nil {
		return // 无 .env 不是错误：纯 env 口径也能跑
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if k != "" && os.Getenv(k) == "" {
			os.Setenv(k, v)
		}
	}
}

// dumpRows 全量读表（含软删）+ 逐行盘面核对 + sha-256 对账。
func dumpRows(ctx context.Context, dsn, dataDir string) ([]fileRow, error) {
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	defer conn.Close(ctx)

	const q = `SELECT file_path, uuid::text, title, description, tags, file_type,
		mime_type, extension, scope, owner_id, user_id, group_id::text,
		visibility, COALESCE(size_bytes,0), COALESCE(checksum,''), missing_rounds,
		is_deleted, deleted_at, COALESCE(copied_from,''), download_count,
		updated_at, created_at, attributes
		FROM file_metadata ORDER BY file_path`
	rs, err := conn.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer rs.Close()

	var out []fileRow
	for rs.Next() {
		var r fileRow
		var title, desc, ftype, mime, ext, owner, uid, copied *string
		var tags []string
		var deletedAt *time.Time
		var attrsRaw []byte
		var gidPtr *string
		if err := rs.Scan(&r.LogicPath, &r.UUID, &title, &desc, &tags, &ftype,
			&mime, &ext, &r.Scope, &owner, &uid, &gidPtr,
			&r.Visibility, &r.SizeBytes, &r.Checksum, &r.MissRounds,
			&r.IsDeleted, &deletedAt, &copied, &r.DLCnt,
			&r.UpdatedAt, &r.CreatedAt, &attrsRaw); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		derefInto(&r.Title, title)
		derefInto(&r.Description, desc)
		derefInto(&r.FileType, ftype)
		derefInto(&r.MimeType, mime)
		derefInto(&r.Extension, ext)
		derefInto(&r.OwnerID, owner)
		derefInto(&r.UserID, uid)
		derefInto(&r.CopiedFrom, copied)
		if gidPtr != nil {
			r.GroupID = *gidPtr
		}
		r.Tags = tags
		if r.Tags == nil {
			r.Tags = []string{}
		}
		r.DeletedAt = deletedAt
		r.Attrs = decodeAttrs(attrsRaw)
		checkDisk(&r, dataDir)
		out = append(out, r)
	}
	return out, rs.Err()
}

// decodeAttrs JSONB → map（UseNumber 保留数字原貌，报告展示不失真）。
func decodeAttrs(b []byte) map[string]any {
	m := map[string]any{}
	if len(b) == 0 {
		return m
	}
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.UseNumber()
	if err := dec.Decode(&m); err != nil {
		return map[string]any{"_解析失败": err.Error()}
	}
	return m
}

// checkDisk 盘面核对：uuid 派生物理位 → 存在性 / 大小 / mtime / sha-256 对账。
func checkDisk(r *fileRow, dataDir string) {
	rel, err := manager.StoragePathOf(r.UUID, r.LogicPath)
	if err != nil {
		return
	}
	r.StorageAbs = filepath.Join(dataDir, filepath.FromSlash(rel))
	info, err := os.Stat(r.StorageAbs)
	if err != nil || info.IsDir() {
		r.DiskOK = false
		return
	}
	r.DiskOK, r.DiskSize, r.DiskMtime = true, info.Size(), info.ModTime()
	if r.Checksum == "" {
		return // DB 无 checksum（未分析/极旧行）：无从对账
	}
	sum, err := fileSHA256(r.StorageAbs)
	if err != nil {
		return
	}
	if sum == r.Checksum {
		r.HashMatch = "ok"
	} else {
		r.HashMatch = "drift" // 内容被直改（execute_command 绕口）——T2 对账的现场证据
	}
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func derefInto(dst *string, p *string) {
	if p != nil {
		*dst = *p
	}
}

func writeJSON(path string, rows []fileRow) error {
	b, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func absOf(p string) string {
	a, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return a
}

// fieldStats 字段目录统计：描述机产出键的覆盖面（索引机目录的人类视角）。
func fieldStats(rows []fileRow) []fieldStat {
	type agg struct {
		files   int
		samples []string
		kinds   map[string]bool
	}
	m := map[string]*agg{}
	for _, r := range rows {
		if r.IsDeleted {
			continue // 目录口径与索引机一致：软删不挂载
		}
		for k, v := range r.Attrs {
			a := m[k]
			if a == nil {
				a = &agg{kinds: map[string]bool{}}
				m[k] = a
			}
			a.files++
			a.kinds[stampOf(v)] = true
			if len(a.samples) < 3 {
				s := fmt.Sprint(v)
				if len(s) > 24 {
					s = s[:24] + "…"
				}
				a.samples = append(a.samples, s)
			}
		}
	}
	out := make([]fieldStat, 0, len(m))
	for k, a := range m {
		st := fieldStat{Field: k, Files: a.files, Samples: strings.Join(a.samples, " / ")}
		switch {
		case len(a.kinds) == 1 && a.kinds["bool"]:
			st.Kind = "enum"
		case a.kinds["num"]:
			st.Kind = "num"
		case a.kinds["array"]:
			st.Kind = "multi"
		default:
			st.Kind = "enum"
		}
		out = append(out, st)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Files != out[j].Files {
			return out[i].Files > out[j].Files
		}
		return out[i].Field < out[j].Field
	})
	return out
}

// stampOf 值形状归类（统计口径，非精确类型学）。
func stampOf(v any) string {
	switch v.(type) {
	case bool:
		return "bool"
	case json.Number:
		return "num"
	case string:
		return "str"
	case []any:
		return "array"
	default:
		return "other"
	}
}
