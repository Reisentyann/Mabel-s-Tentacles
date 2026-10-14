// 文件：manager-go/store.go —— 元数据存储契约与读写载荷
// 修改：2026-10-08（日期由 fresh-header.ps1 刷新）

package manager

import (
	"context"
	"encoding/json"
	"time"
)

// MetaRow 是存储提供的元数据读视图。
type MetaRow struct {
	Path          string // 唯一逻辑键。
	UUID          string // 文件身份及物理路径派生源。
	Scope         string // global / user / game。
	MimeType      string
	Checksum      string
	Attributes    json.RawMessage
	IsDeleted     bool
	MissingRounds int
	SizeBytes     int64
	UpdatedAt     time.Time
	MovedFrom     string // 最近一次移动的原逻辑键。
	CopiedFrom    string // 复制来源逻辑键。
}

// MetaRecord 是重分析后的落库载荷。
// 顶层类型字段由存储适配器根据 Name 推导，Path 为持久化键。
type MetaRecord struct {
	Path       string
	Name       string
	SizeBytes  int64
	Checksum   string
	Attributes json.RawMessage
}

// Store 是 Manager 所需的持久化契约，由调用方的存储适配器实现。
// 可选的 UUID 目录与移动能力见 DirectoryStore、UUIDMoveStore 和 MoveRecoveryStore。
type Store interface {
	// 查询：分页按 Path 升序，sincePath 不包含在结果中。
	// ListMetaPage 隐藏软删及未发布对象；All 包含软删，供对账使用。
	ListMetaPage(ctx context.Context, sincePath string, limit int) ([]MetaRow, error)
	ListMetaPageAll(ctx context.Context, sincePath string, limit int) ([]MetaRow, error)
	// 单对象查询无行返回 (nil, nil)，批量查询省略缺失 UUID。
	GetMeta(ctx context.Context, path string) (*MetaRow, error)
	GetMetaByUUID(ctx context.Context, uuid string) (*FileRef, error)
	GetMetaByUUIDs(ctx context.Context, uuids []string) (map[string]*FileRef, error)
	ReverseCopiedFrom(ctx context.Context, path string) ([]MetaRow, error)

	// 重分析与盘面巡检。
	UpsertMeta(ctx context.Context, rec MetaRecord) (string, error)
	MarkMissing(ctx context.Context, path string) (int, error)
	ResetMissing(ctx context.Context, uuid string) error
	SoftDeleteMeta(ctx context.Context, path string) error

	// 入库：原子占位产生 UUID，逻辑键已占用（含软删）返回 ErrKeyExists。
	ReserveMeta(ctx context.Context, logicPath string) (string, error)
	CompleteIntake(ctx context.Context, logicPath, uuid string) error
	// 仅归档本次 UUID 的占位，保留扩展名与物理位置并释放原名称。
	ArchiveFailedIntake(ctx context.Context, logicPath, uuid string) error
	ListPendingIntakes(ctx context.Context) ([]IntakeOperation, error)
	// IsIntakePending 同时覆盖未完成的入库和移动。
	IsIntakePending(ctx context.Context, uuid string) (bool, error)

	// 逻辑目录与移动。MoveMeta 同事务更新谱系，返回文件 UUID；
	// 扩展名改变时必须同时持久化待恢复记录。
	CreateDirectory(ctx context.Context, logicPath string) error
	ListDirectoryPaths(ctx context.Context) ([]string, error)
	DirectoryExists(ctx context.Context, logicPath string) (bool, error)
	MoveMeta(ctx context.Context, from, to string) (string, error)
}
