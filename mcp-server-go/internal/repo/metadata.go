package repo

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
)

// FileMetadata 文件的描述/标签/属性等元数据。指针字段为 NULL 时表示「未提供」。
type FileMetadata struct {
	ID             int64           `json:"id"`
	FilePath       string          `json:"file_path"`
	Scope          string          `json:"scope"` // global | user（默认 global）
	OwnerID        *string         `json:"owner_id"`
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
	DownloadCount  int64           `json:"download_count"`
	LastAccessedAt *time.Time      `json:"last_accessed_at"`
	ExpiresAt      *time.Time      `json:"expires_at"`
	IsDeleted      bool            `json:"is_deleted"`
	DeletedAt      *time.Time      `json:"deleted_at"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

// FileSearch 检索条件，零值字段表示不筛选。
type FileSearch struct {
	Query          string
	Tags           []string
	FileType       string
	Creator        string
	Attributes     map[string]any
	IncludeDeleted bool
	Page           int
	Size           int
}

const metaColumns = `id, file_path, scope, owner_id, title, description, tags, file_type, mime_type, extension, size_bytes, checksum, session_id, user_id, attributes, copied_from, download_count, last_accessed_at, expires_at, is_deleted, deleted_at, created_at, updated_at`

const metaWhere = `is_deleted = $1
 AND ($2::text   IS NULL OR description ILIKE '%'||$2||'%' OR file_path ILIKE '%'||$2||'%')
 AND ($3::text[] IS NULL OR tags @> $3)
 AND ($4::text   IS NULL OR file_type = $4)
 AND ($5::text   IS NULL OR user_id = $5)
 AND ($6::jsonb  IS NULL OR attributes @> $6)`

func scanMeta(row pgx.Row) (*FileMetadata, error) {
	var m FileMetadata
	if err := row.Scan(&m.ID, &m.FilePath, &m.Scope, &m.OwnerID, &m.Title, &m.Description, &m.Tags,
		&m.FileType, &m.MimeType, &m.Extension, &m.SizeBytes, &m.Checksum, &m.SessionID, &m.UserID,
		&m.Attributes, &m.CopiedFrom, &m.DownloadCount, &m.LastAccessedAt, &m.ExpiresAt,
		&m.IsDeleted, &m.DeletedAt, &m.CreatedAt, &m.UpdatedAt); err != nil {
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

// UpsertMetadata 写入/更新文件元数据。指针字段为 NULL 时保留原值，非 NULL 才覆盖。
// download_count / last_accessed_at / is_deleted 由各自的专用方法维护，这里不动。
func (s *pgxStore) UpsertMetadata(ctx context.Context, m *FileMetadata) error {
	scope := m.Scope
	if scope == "" {
		scope = "global"
	}
	attrs := m.Attributes
	if len(attrs) == 0 {
		attrs = json.RawMessage("{}")
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO file_metadata (file_path, scope, owner_id, title, description, tags, file_type, mime_type, extension, size_bytes, checksum, session_id, user_id, attributes, copied_from, expires_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
		 ON CONFLICT (file_path) DO UPDATE SET
		   scope        = COALESCE(NULLIF(EXCLUDED.scope,''), file_metadata.scope),
		   owner_id     = COALESCE(EXCLUDED.owner_id,     file_metadata.owner_id),
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
		   updated_at   = NOW()`,
		m.FilePath, scope, m.OwnerID, m.Title, m.Description, m.Tags, m.FileType, m.MimeType, m.Extension,
		m.SizeBytes, m.Checksum, m.SessionID, m.UserID, attrs, m.CopiedFrom, m.ExpiresAt,
	)
	return err
}

func (s *pgxStore) GetMetadata(ctx context.Context, filePath string) (*FileMetadata, error) {
	return scanMeta(s.pool.QueryRow(ctx,
		`SELECT `+metaColumns+` FROM file_metadata WHERE file_path=$1`, filePath))
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
	var attrs json.RawMessage
	if len(fs.Attributes) > 0 {
		attrs, _ = json.Marshal(fs.Attributes)
	}

	var total int
	if err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM file_metadata WHERE `+metaWhere,
		fs.IncludeDeleted, q, tags, ft, creator, attrs,
	).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (fs.Page - 1) * fs.Size
	rows, err := s.pool.Query(ctx,
		`SELECT `+metaColumns+` FROM file_metadata WHERE `+metaWhere+` ORDER BY updated_at DESC LIMIT $7 OFFSET $8`,
		fs.IncludeDeleted, q, tags, ft, creator, attrs, fs.Size, offset,
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
func (s *pgxStore) CopyMetadata(ctx context.Context, source, target, sessionID, userID string) error {
	src, err := s.GetMetadata(ctx, source)
	if err != nil {
		return err
	}
	var sid, uid *string
	if sessionID != "" {
		sid = &sessionID
	}
	if userID != "" {
		uid = &userID
	}
	cp := &FileMetadata{
		FilePath:    target,
		Scope:       src.Scope,
		OwnerID:     src.OwnerID,
		Title:       src.Title,
		Description: src.Description,
		Tags:        src.Tags,
		FileType:    src.FileType,
		MimeType:    src.MimeType,
		Extension:   src.Extension,
		SizeBytes:   src.SizeBytes,
		Checksum:    src.Checksum,
		SessionID:   sid,
		UserID:      uid,
		Attributes:  src.Attributes,
		CopiedFrom:  &source,
	}
	return s.UpsertMetadata(ctx, cp)
}

// SoftDeleteMetadata 软删除：只打标记，不物理删除，元数据可追溯。
func (s *pgxStore) SoftDeleteMetadata(ctx context.Context, filePath string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE file_metadata SET is_deleted=TRUE, deleted_at=NOW(), updated_at=NOW() WHERE file_path=$1`, filePath)
	return err
}
