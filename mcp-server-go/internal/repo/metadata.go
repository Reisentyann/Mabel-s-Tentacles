// 文件：mcp-server-go/internal/repo/metadata.go —— file_metadata 表存取：模型 / Upsert(COALESCE 返回 uuid) / 搜索 / 分页扫描 / 缺失计数 / 软删
// 修改：2026-09-08（日期由 fresh-header.ps1 刷新）

package repo

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// FileMetadata 文件的描述/标签/属性等元数据。指针字段为 NULL 时表示「未提供」。
type FileMetadata struct {
	ID             int64           `json:"id"`
	UUID           string          `json:"uuid"` // 组件间货币（索引机挂载键 / manager 凭它取件），DB 默认生成
	FilePath       string          `json:"file_path"`
	Scope          string          `json:"scope"`      // global | user | game（默认 global；game 为游戏室预留分区，按 game/ 路径前缀自动推导）
	OwnerID        *string         `json:"owner_id"`   // 归属（authz 矩阵的 owner 匹配口径；NULL = 无主存量，写权归 admin）
	Visibility     string          `json:"visibility"` // public | group | private（authz 资源轴；空串 = COALESCE 保留既有值）
	GroupID        *int64          `json:"group_id"`   // group 可见性的组 id
	Title          *string         `json:"title"`
	Description    *string         `json:"description"`
	Tags           []string        `json:"tags"`
	FileType       *string         `json:"file_type"`
	MimeType       *string         `json:"mime_type"`
	Extension      *string         `json:"extension"`
	SizeBytes      *int64          `json:"size_bytes"`
	Checksum       *string         `json:"checksum"`
	SessionID      *string         `json:"session_id"`
	UserID         *string         `json:"user_id"`
	Attributes     json.RawMessage `json:"attributes"`
	CopiedFrom     *string         `json:"copied_from"`
	MovedFrom      *string         `json:"moved_from"` // 谱系：最近一次移动的原键（文件管理域）
	DownloadCount  int64           `json:"download_count"`
	LastAccessedAt *time.Time      `json:"last_accessed_at"`
	ExpiresAt      *time.Time      `json:"expires_at"`
	IsDeleted      bool            `json:"is_deleted"`
	DeletedAt      *time.Time      `json:"deleted_at"`
	MissingRounds  int             `json:"missing_rounds"` // T2 回填中盘上连续缺失轮次（3 轮软删除）
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

// FileSearch 检索条件，零值字段表示不筛选。
type FileSearch struct {
	Query          string
	Tags           []string
	FileType       string
	Creator        string
	Scope          string // 分区过滤：global / user / game（空 = 不过滤）
	Attributes     map[string]any
	IncludeDeleted bool
	Page           int
	Size           int
	// 观察者过滤（authz：非 admin 只见 public / 自己的 / 本组 group 文件）
	ViewerName   string  // 观察者用户名；空 = 不过滤（admin / 匿名开发直通）
	ViewerAdmin  bool    // admin 不做可见性过滤
	ViewerGroups []int64 // 观察者所在组
}

const metaColumns = `id, uuid, file_path, scope, owner_id, visibility, group_id, title, description, tags, file_type, mime_type, extension, size_bytes, checksum, session_id, user_id, attributes, copied_from, moved_from, download_count, last_accessed_at, expires_at, is_deleted, deleted_at, missing_rounds, created_at, updated_at`

const metaWhere = `is_deleted = $1
 AND ($2::text   IS NULL OR description ILIKE '%'||$2||'%' OR file_path ILIKE '%'||$2||'%')
 AND ($3::text[] IS NULL OR tags @> $3)
 AND ($4::text   IS NULL OR file_type = $4)
 AND ($5::text   IS NULL OR user_id = $5)
 AND ($6::jsonb  IS NULL OR attributes @> $6)
 AND ($7::text   IS NULL OR scope    = $7)
 AND ($8::text   IS NULL OR visibility = 'public' OR owner_id = $8 OR (visibility = 'group' AND group_id = ANY($9)))`

func scanMeta(row pgx.Row) (*FileMetadata, error) {
	var m FileMetadata
	if err := row.Scan(&m.ID, &m.UUID, &m.FilePath, &m.Scope, &m.OwnerID, &m.Visibility, &m.GroupID,
		&m.Title, &m.Description, &m.Tags,
		&m.FileType, &m.MimeType, &m.Extension, &m.SizeBytes, &m.Checksum, &m.SessionID, &m.UserID,
		&m.Attributes, &m.CopiedFrom, &m.MovedFrom, &m.DownloadCount, &m.LastAccessedAt, &m.ExpiresAt,
		&m.IsDeleted, &m.DeletedAt, &m.MissingRounds, &m.CreatedAt, &m.UpdatedAt); err != nil {
		return nil, err
	}
	if m.Tags == nil {
		m.Tags = []string{}
	}
	if len(m.Attributes) == 0 {
		m.Attributes = json.RawMessage("{}")
	}
	return &m, nil
}

// UpsertMetadata 写入/更新文件元数据，返回该行的 uuid（组件间货币）。
// 指针字段为 NULL 时保留原值，非 NULL 才覆盖；Visibility 空串保留原值
// （新行空串 = 存量口径，authz 按 public 对待）。
// download_count / last_accessed_at / is_deleted 由各自的专用方法维护，这里不动。
// 写入即文件存在的证据：missing_rounds 清零（幽灵计数只在 MarkMissingRound 递增）。
func (s *pgxStore) UpsertMetadata(ctx context.Context, m *FileMetadata) (string, error) {
	scope := m.Scope
	if scope == "" {
		scope = "global"
	}
	attrs := m.Attributes
	if len(attrs) == 0 {
		attrs = json.RawMessage("{}")
	}
	var uuid string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO file_metadata (file_path, scope, owner_id, visibility, group_id, title, description, tags, file_type, mime_type, extension, size_bytes, checksum, session_id, user_id, attributes, copied_from, expires_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
		 ON CONFLICT (file_path) DO UPDATE SET
		   scope        = COALESCE(NULLIF(EXCLUDED.scope,''), file_metadata.scope),
		   owner_id     = COALESCE(EXCLUDED.owner_id,     file_metadata.owner_id),
		   visibility   = COALESCE(NULLIF(EXCLUDED.visibility,''), file_metadata.visibility),
		   group_id     = COALESCE(EXCLUDED.group_id,     file_metadata.group_id),
		   title        = COALESCE(EXCLUDED.title,        file_metadata.title),
		   description  = COALESCE(EXCLUDED.description,  file_metadata.description),
		   tags         = COALESCE(EXCLUDED.tags,         file_metadata.tags),
		   file_type    = COALESCE(EXCLUDED.file_type,    file_metadata.file_type),
		   mime_type    = COALESCE(EXCLUDED.mime_type,    file_metadata.mime_type),
		   extension    = COALESCE(EXCLUDED.extension,    file_metadata.extension),
		   size_bytes   = COALESCE(EXCLUDED.size_bytes,   file_metadata.size_bytes),
		   checksum     = COALESCE(EXCLUDED.checksum,     file_metadata.checksum),
		   session_id   = COALESCE(EXCLUDED.session_id,   file_metadata.session_id),
		   user_id      = COALESCE(EXCLUDED.user_id,      file_metadata.user_id),
		   attributes   = COALESCE(EXCLUDED.attributes,   file_metadata.attributes),
		   copied_from  = COALESCE(EXCLUDED.copied_from,  file_metadata.copied_from),
		   expires_at   = COALESCE(EXCLUDED.expires_at,   file_metadata.expires_at),
		   missing_rounds = 0,
		   updated_at   = NOW()
		 RETURNING uuid`,
		m.FilePath, scope, m.OwnerID, m.Visibility, m.GroupID, m.Title, m.Description, m.Tags, m.FileType, m.MimeType, m.Extension,
		m.SizeBytes, m.Checksum, m.SessionID, m.UserID, attrs, m.CopiedFrom, m.ExpiresAt,
	).Scan(&uuid)
	if err != nil {
		return "", err
	}
	return uuid, nil
}

func (s *pgxStore) GetMetadata(ctx context.Context, filePath string) (*FileMetadata, error) {
	return scanMeta(s.pool.QueryRow(ctx,
		`SELECT `+metaColumns+` FROM file_metadata WHERE file_path=$1`, filePath))
}

// GetMetadataByPaths 批量取多个文件的元数据，按 file_path 映射返回，供分页列表联表用。
// 不存在的路径不出现在结果里，调用方按 nil 处理为「无元数据」。
func (s *pgxStore) GetMetadataByPaths(ctx context.Context, paths []string) (map[string]*FileMetadata, error) {
	out := make(map[string]*FileMetadata, len(paths))
	if len(paths) == 0 {
		return out, nil
	}
	rows, err := s.pool.Query(ctx,
		`SELECT `+metaColumns+` FROM file_metadata WHERE file_path = ANY($1)`, paths)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		m, err := scanMeta(rows)
		if err != nil {
			return nil, err
		}
		out[m.FilePath] = m
	}
	return out, rows.Err()
}

// GetMetadataByUUID 凭 uuid 读单行（含软删行——manager fetch 的 Locate
// 语义"软删照报"；无行返回 pgx.ErrNoRows，由调用方映射）。
func (s *pgxStore) GetMetadataByUUID(ctx context.Context, uuid string) (*FileMetadata, error) {
	return scanMeta(s.pool.QueryRow(ctx,
		`SELECT `+metaColumns+` FROM file_metadata WHERE uuid=$1`, uuid))
}

// GetMetadataByUUIDs 批量凭 uuid 取行（含软删行）：uuid → 行映射，
// 缺失的 uuid 不入 map（调用方对照入参找缺）。搜索索引路径一次取齐
// （Index.Query → uuids → 本方法，免 N+1）。
func (s *pgxStore) GetMetadataByUUIDs(ctx context.Context, uuids []string) (map[string]*FileMetadata, error) {
	out := make(map[string]*FileMetadata, len(uuids))
	if len(uuids) == 0 {
		return out, nil
	}
	rows, err := s.pool.Query(ctx,
		`SELECT `+metaColumns+` FROM file_metadata WHERE uuid = ANY($1)`, uuids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		m, err := scanMeta(rows)
		if err != nil {
			return nil, err
		}
		out[m.UUID] = m
	}
	return out, rows.Err()
}

// SearchFiles 按条件检索元数据，返回分页结果与总数。
func (s *pgxStore) SearchFiles(ctx context.Context, fs FileSearch) ([]FileMetadata, int, error) {
	var q *string
	if fs.Query != "" {
		q = &fs.Query
	}
	var tags []string
	if len(fs.Tags) > 0 {
		tags = fs.Tags
	}
	var ft *string
	if fs.FileType != "" {
		ft = &fs.FileType
	}
	var creator *string
	if fs.Creator != "" {
		creator = &fs.Creator
	}
	var scope *string
	if fs.Scope != "" {
		scope = &fs.Scope
	}
	var attrs json.RawMessage
	if len(fs.Attributes) > 0 {
		attrs, _ = json.Marshal(fs.Attributes)
	}

	// 观察者过滤：admin / 未指定观察者时不过滤（$8 NULL 短路整条）；
	// 普通用户只见 public / 自己的 / 本组 group 文件（authz.CanRead 的 SQL 投影）
	var viewer *string
	var vgroups []int64
	if !fs.ViewerAdmin && fs.ViewerName != "" {
		viewer = &fs.ViewerName
		vgroups = fs.ViewerGroups
	}

	var total int
	if err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM file_metadata WHERE `+metaWhere,
		fs.IncludeDeleted, q, tags, ft, creator, attrs, scope, viewer, vgroups,
	).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (fs.Page - 1) * fs.Size
	rows, err := s.pool.Query(ctx,
		`SELECT `+metaColumns+` FROM file_metadata WHERE `+metaWhere+` ORDER BY updated_at DESC LIMIT $10 OFFSET $11`,
		fs.IncludeDeleted, q, tags, ft, creator, attrs, scope, viewer, vgroups, fs.Size, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []FileMetadata
	for rows.Next() {
		m, err := scanMeta(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, *m)
	}
	return items, total, rows.Err()
}

// CopyMetadata 复制源文件元数据到目标（内容复制由 service 完成）。
// 权限批次语义（2026-09-06）：副本归操作者（拿走即拥有，copied_from 保留
// 谱系溯源），可见性与组继承源文件——授权判定（CanRead 源）归调用方。
func (s *pgxStore) CopyMetadata(ctx context.Context, source, target, owner, sessionID string) error {
	src, err := s.GetMetadata(ctx, source)
	if err != nil {
		return err
	}
	var sid *string
	if sessionID != "" {
		sid = &sessionID
	}
	var ownerPtr *string
	if owner != "" {
		ownerPtr = &owner
	}
	cp := &FileMetadata{
		FilePath:    target,
		Scope:       src.Scope,
		OwnerID:     ownerPtr,
		Visibility:  src.Visibility,
		GroupID:     src.GroupID,
		Title:       src.Title,
		Description: src.Description,
		Tags:        src.Tags,
		FileType:    src.FileType,
		MimeType:    src.MimeType,
		Extension:   src.Extension,
		SizeBytes:   src.SizeBytes,
		Checksum:    src.Checksum,
		SessionID:   sid,
		Attributes:  src.Attributes,
		CopiedFrom:  &source,
	}
	_, err = s.UpsertMetadata(ctx, cp)
	return err
}

// SoftDeleteMetadata 软删除：只打标记，不物理删除，元数据可追溯。
func (s *pgxStore) SoftDeleteMetadata(ctx context.Context, filePath string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE file_metadata SET is_deleted=TRUE, deleted_at=NOW(), updated_at=NOW() WHERE file_path=$1`, filePath)
	return err
}

// ErrKeyExists 目标逻辑键已被占用（Move 拒绝覆盖；file_path UNIQUE 兜底
// 并发竞态——两个 Move 同时抢同一目标键时后者在 UNIQUE 上撞出本哨兵）。
var ErrKeyExists = errors.New("repo: logic key already exists")

// MoveMetadata 逻辑键改（文件管理域 2026-09-08；manager.Move 的支撑）：
// from 行键改 to + moved_from 记谱系，返回行 uuid。无行 / 软删行 →
// pgx.ErrNoRows（调用方翻译哨兵）；to 已占用 → ErrKeyExists。
// uuid / 归属 / 描述 / 索引键全不动——键改即完成"移动"。
func (s *pgxStore) MoveMetadata(ctx context.Context, from, to string) (string, error) {
	// 预查目标占用（人话错误）；并发竞态由 UNIQUE 约束兜底（下方 23505 捕获）
	var exists bool
	if err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM file_metadata WHERE file_path=$1)`, to).Scan(&exists); err != nil {
		return "", err
	}
	if exists {
		return "", ErrKeyExists
	}
	var uuid string
	err := s.pool.QueryRow(ctx,
		`UPDATE file_metadata SET file_path=$2, moved_from=$1, updated_at=NOW()
		 WHERE file_path=$1 AND is_deleted=FALSE
		 RETURNING uuid`, from, to).Scan(&uuid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", pgx.ErrNoRows // 无行 / 软删行
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return "", ErrKeyExists // UNIQUE 兜底：并发抢键的后到者
		}
		return "", err
	}
	return uuid, nil
}

// IncrementDownloadCount 递增下载计数并刷新最后访问时间。
// 已软删除的文件不计入，便于通过下载入口阻断被回收的文件。
func (s *pgxStore) IncrementDownloadCount(ctx context.Context, filePath string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE file_metadata SET download_count = download_count + 1, last_accessed_at = NOW(), updated_at = NOW()
		 WHERE file_path=$1 AND is_deleted=FALSE`, filePath)
	return err
}

// ListMetadataPage 按 file_path 升序的游标分页：返回 sincePath 之后（不含）的
// limit 条未软删元数据。sincePath 传空串从头开始。T2 回填的扫描入口——
// 游标分页可中断续跑，深分页无 OFFSET 性能悬崖。
func (s *pgxStore) ListMetadataPage(ctx context.Context, sincePath string, limit int) ([]FileMetadata, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx,
		`SELECT `+metaColumns+` FROM file_metadata
		 WHERE is_deleted = FALSE AND file_path > $1
		 ORDER BY file_path ASC LIMIT $2`, sincePath, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]FileMetadata, 0, limit)
	for rows.Next() {
		m, err := scanMeta(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *m)
	}
	return items, rows.Err()
}

// ReserveMeta 入库占位行：按逻辑键幂等拿 uuid（manager intake 域——
// uuid 先于盘写存在，物理路径由它派生）。最小 INSERT（scope/attributes/
// visibility 等走 DDL 默认值），冲突即复用既有 uuid；写入即存在证据，
// missing_rounds 清零。
func (s *pgxStore) ReserveMeta(ctx context.Context, logicPath string) (string, error) {
	var uuid string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO file_metadata (file_path) VALUES ($1)
		 ON CONFLICT (file_path) DO UPDATE SET missing_rounds = 0, updated_at = NOW()
		 RETURNING uuid`, logicPath).Scan(&uuid)
	return uuid, err
}

// MarkMissingRound 盘上缺失计数 +1，返回累计轮次（manager updater 的幽灵存续：
// 连续 3 轮缺失触发软删除）。文件重新出现走 UpsertMetadata 时清零。
func (s *pgxStore) MarkMissingRound(ctx context.Context, filePath string) (int, error) {
	var rounds int
	err := s.pool.QueryRow(ctx,
		`UPDATE file_metadata SET missing_rounds = missing_rounds + 1, updated_at = NOW()
		 WHERE file_path = $1 AND is_deleted = FALSE
		 RETURNING missing_rounds`, filePath).Scan(&rounds)
	return rounds, err
}
