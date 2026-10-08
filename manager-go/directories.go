// 文件：manager-go/directories.go —— 逻辑目录、UUID 目录项与同名文件存取
// 修改：2026-10-08（日期由 fresh-header.ps1 刷新）

package manager

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"path"
)

// DirectoryRef 保留 Path 为旧键空间权限适配；外部交互以 UUID 为准。
type DirectoryRef struct {
	UUID string `json:"uuid"`
	Path string `json:"path"`
}

// DirectoryEntry 是目录中的文件或子目录，Kind 为 file 或 dir。
type DirectoryEntry struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}

// DirectoryStore 是可选能力，避免旧存储实现被迫实现目录上传。
type DirectoryStore interface {
	DirectoryByUUID(context.Context, string) (*DirectoryRef, error)
	DirectoryByPath(context.Context, string) (*DirectoryRef, error)
	LinkDirectoryFile(context.Context, string, string, string) error
	DirectoryEntries(context.Context, string) ([]DirectoryEntry, error)
}

// atomicDirectoryIntakeStore 标记 ReserveMeta 能在同一事务中保存目录关联
// 和复制来源。旧适配器仍可通过 LinkDirectoryFile 补充普通上传的关联。
type atomicDirectoryIntakeStore interface {
	AtomicDirectoryIntake()
}

func (m *Manager) directoryStore() (DirectoryStore, error) {
	s, ok := m.store.(DirectoryStore)
	if !ok {
		return nil, fmt.Errorf("manager: UUID directory storage unavailable")
	}
	return s, nil
}

// LocateDirectory 按 UUID 查询逻辑目录。
func (m *Manager) LocateDirectory(ctx context.Context, uuid string) (*DirectoryRef, error) {
	s, err := m.directoryStore()
	if err != nil {
		return nil, err
	}
	return s.DirectoryByUUID(ctx, uuid)
}

// DirectoryAt 按逻辑路径查询目录。
func (m *Manager) DirectoryAt(ctx context.Context, logicPath string) (*DirectoryRef, error) {
	s, err := m.directoryStore()
	if err != nil {
		return nil, err
	}
	return s.DirectoryByPath(ctx, logicPath)
}

// CreateDirectory 创建逻辑空目录及其父目录，目录只存于元数据。
func (m *Manager) CreateDirectory(ctx context.Context, logicPath string) error {
	if err := validLogicPath(logicPath); err != nil {
		return err
	}
	for _, prefix := range logicPrefixes(logicPath) {
		if row, err := m.store.GetMeta(ctx, prefix); err != nil {
			return fmt.Errorf("get meta: %w", err)
		} else if row != nil {
			return ErrKeyExists
		}
	}
	if exists, err := m.store.DirectoryExists(ctx, logicPath); err != nil {
		return fmt.Errorf("get directory: %w", err)
	} else if exists {
		return ErrDirectoryExists
	}
	for _, prefix := range logicPrefixes(logicPath) {
		err := m.store.CreateDirectory(ctx, prefix)
		if err != nil && !errors.Is(err, ErrDirectoryExists) {
			return err
		}
	}
	return nil
}

// CreateChildDirectory 在指定 UUID 目录下创建子目录。
func (m *Manager) CreateChildDirectory(ctx context.Context, parentUUID, name string) (*DirectoryRef, error) {
	if err := validEntryName(name); err != nil {
		return nil, err
	}
	parent, err := m.LocateDirectory(ctx, parentUUID)
	if err != nil {
		return nil, err
	}
	p := path.Join(parent.Path, name)
	if err := m.CreateDirectory(ctx, p); err != nil {
		return nil, err
	}
	return m.DirectoryAt(ctx, p)
}

// newDirectoryFilePath 用不透明唯一名称保持旧 file_path 唯一约束，显示名称单独记录。
func newDirectoryFilePath(dir *DirectoryRef, name string) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return path.Join(dir.Path, fmt.Sprintf("%x", b)+path.Ext(name)), nil
}

// WriteInDirectory 创建新文件，允许与已有文件显示名称相同。
func (m *Manager) WriteInDirectory(ctx context.Context, parentUUID, name, content string) (*WriteReceipt, error) {
	return m.WriteInDirectoryRequest(ctx, parentUUID, name, content, "")
}

// WriteInDirectoryRequest 对尚未移动、修改或删除的上传提供可选幂等重放。
func (m *Manager) WriteInDirectoryRequest(ctx context.Context, parentUUID, name, content, requestID string) (*WriteReceipt, error) {
	s, key, err := m.directoryIntakePath(ctx, parentUUID, name, requestID)
	if err != nil {
		return nil, err
	}
	ctx = WithDirectoryIntake(ctx, parentUUID, name)
	r, err := m.Write(ctx, key, content)
	if err != nil {
		if requestID != "" && errors.Is(err, ErrKeyExists) {
			return m.replayDirectoryWrite(ctx, s, parentUUID, key, name, content)
		}
		return nil, err
	}
	return r, linkDirectoryIntake(ctx, s, parentUUID, r.UUID, name)
}

// replayDirectoryWrite 仅返回仍属于原目录、名称与内容均一致的已发布对象。
func (m *Manager) replayDirectoryWrite(ctx context.Context, s DirectoryStore, parentUUID, key, name, content string) (*WriteReceipt, error) {
	row, err := m.store.GetMeta(ctx, key)
	if err != nil {
		return nil, err
	}
	if row == nil || row.IsDeleted {
		return nil, fmt.Errorf("request result unavailable")
	}
	items, err := s.DirectoryEntries(ctx, parentUUID)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if item.UUID != row.UUID || item.Name != name {
			continue
		}
		rf, err := m.ReadByLogic(ctx, key, 0)
		if err != nil {
			return nil, err
		}
		if string(rf.Content) != content {
			return nil, fmt.Errorf("request parameters differ")
		}
		rel, err := StoragePathOf(row.UUID, key)
		if err != nil {
			return nil, err
		}
		return &WriteReceipt{UUID: row.UUID, LogicPath: key, StorageRel: rel, SizeBytes: int64(len(content))}, nil
	}
	return nil, fmt.Errorf("request in progress or parameters differ")
}

// ImportInDirectory 将本地文件复制到指定目录并保留显示名称。
func (m *Manager) ImportInDirectory(ctx context.Context, parentUUID, name, source string) (*ImportReceipt, error) {
	s, key, err := m.directoryIntakePath(ctx, parentUUID, name, "")
	if err != nil {
		return nil, err
	}
	ctx = WithDirectoryIntake(ctx, parentUUID, name)
	r, err := m.ImportFile(ctx, key, source)
	if err != nil {
		return nil, err
	}
	return r, linkDirectoryIntake(ctx, s, parentUUID, r.UUID, name)
}

// directoryIntakePath 校验目录项并生成唯一键；请求重放使用确定性的键。
func (m *Manager) directoryIntakePath(ctx context.Context, parentUUID, name, requestID string) (DirectoryStore, string, error) {
	if err := validEntryName(name); err != nil {
		return nil, "", err
	}
	s, err := m.directoryStore()
	if err != nil {
		return nil, "", err
	}
	dir, err := s.DirectoryByUUID(ctx, parentUUID)
	if err != nil {
		return nil, "", err
	}
	if requestID != "" {
		sum := sha256.Sum256([]byte(parentUUID + "\x00" + requestID))
		return s, path.Join(dir.Path, fmt.Sprintf("request-%x", sum)), nil
	}
	key, err := newDirectoryFilePath(dir, name)
	return s, key, err
}

// linkDirectoryIntake 仅为旧存储补充关联；原子入库已在占位事务中完成。
func linkDirectoryIntake(ctx context.Context, s DirectoryStore, parentUUID, uuid, name string) error {
	if _, atomic := s.(atomicDirectoryIntakeStore); atomic {
		return nil
	}
	if err := s.LinkDirectoryFile(ctx, parentUUID, uuid, name); err != nil {
		return fmt.Errorf("file stored but directory link failed (uuid=%s): %w", uuid, err)
	}
	return nil
}

// CopyInDirectory 复制源文件内容，并由占位事务固定目录和复制谱系。
// 内容受写入预算限制，存储必须支持原子目录入库。
func (m *Manager) CopyInDirectory(ctx context.Context, sourceUUID, parentUUID, name string) (*WriteReceipt, error) {
	if _, ok := m.store.(atomicDirectoryIntakeStore); !ok {
		return nil, fmt.Errorf("manager: atomic directory copy unavailable")
	}
	if err := validEntryName(name); err != nil {
		return nil, err
	}
	ds, err := m.directoryStore()
	if err != nil {
		return nil, err
	}
	parent, err := ds.DirectoryByUUID(ctx, parentUUID)
	if err != nil {
		return nil, err
	}
	ref, err := m.Locate(ctx, sourceUUID)
	if err != nil {
		return nil, err
	}
	if ref.IsDeleted {
		return nil, ErrDeleted
	}
	src, err := m.Read(ctx, sourceUUID, maxIntakeBytes+1)
	if err != nil {
		return nil, err
	}
	if len(src.Content) > maxIntakeBytes {
		return nil, fmt.Errorf("directory copy exceeds content limit")
	}
	key, err := newDirectoryFilePath(parent, name)
	if err != nil {
		return nil, err
	}
	ctx = WithDirectoryIntake(ctx, parentUUID, name)
	ctx = WithDirectoryCopy(ctx, sourceUUID)
	return m.Write(ctx, key, string(src.Content))
}

// ListDirectory 列出目录的直接子项。
func (m *Manager) ListDirectory(ctx context.Context, uuid string) ([]DirectoryEntry, error) {
	s, err := m.directoryStore()
	if err != nil {
		return nil, err
	}
	if _, err := s.DirectoryByUUID(ctx, uuid); err != nil {
		return nil, err
	}
	return s.DirectoryEntries(ctx, uuid)
}
