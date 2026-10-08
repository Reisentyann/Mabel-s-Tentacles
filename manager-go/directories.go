// 文件：manager-go/directories.go —— UUID 目录存取：目录项、同名文件、递归导入和 ZIP 导出
// 修改：2026-10-08（日期由 fresh-header.ps1 刷新）

package manager

import (
	"archive/zip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// DirectoryRef 保留 Path 为旧键空间权限适配；外部交互以 UUID 为准。
type DirectoryRef struct {
	UUID string `json:"uuid"`
	Path string `json:"path"`
}

type DirectoryEntry struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}

type directoryIntakeKey struct{}

// DirectoryIntake 在数据库占位事务中固定目录及展示名称。
type DirectoryIntake struct{ ParentUUID, Name string }

func DirectoryIntakeFrom(ctx context.Context) (DirectoryIntake, bool) {
	v, ok := ctx.Value(directoryIntakeKey{}).(DirectoryIntake)
	return v, ok
}

func WithDirectoryIntake(ctx context.Context, parentUUID, name string) context.Context {
	return context.WithValue(ctx, directoryIntakeKey{}, DirectoryIntake{parentUUID, name})
}

// DirectoryStore 是可选能力，避免旧存储实现被迫实现目录上传。
type DirectoryStore interface {
	DirectoryByUUID(context.Context, string) (*DirectoryRef, error)
	DirectoryByPath(context.Context, string) (*DirectoryRef, error)
	LinkDirectoryFile(context.Context, string, string, string) error
	DirectoryEntries(context.Context, string) ([]DirectoryEntry, error)
}

func (m *Manager) directoryStore() (DirectoryStore, error) {
	s, ok := m.store.(DirectoryStore)
	if !ok {
		return nil, fmt.Errorf("manager: UUID directory storage unavailable")
	}
	return s, nil
}

func (m *Manager) LocateDirectory(ctx context.Context, uuid string) (*DirectoryRef, error) {
	s, err := m.directoryStore()
	if err != nil {
		return nil, err
	}
	return s.DirectoryByUUID(ctx, uuid)
}

func (m *Manager) DirectoryAt(ctx context.Context, logicPath string) (*DirectoryRef, error) {
	s, err := m.directoryStore()
	if err != nil {
		return nil, err
	}
	return s.DirectoryByPath(ctx, logicPath)
}

func validEntryName(name string) error {
	if err := validLogicPath(name); err != nil {
		return err
	}
	if strings.Contains(name, "/") {
		return ErrInvalidPath
	}
	return nil
}

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

func (m *Manager) WriteInDirectory(ctx context.Context, parentUUID, name, content string) (*WriteReceipt, error) {
	return m.WriteInDirectoryRequest(ctx, parentUUID, name, content, "")
}

// WriteInDirectoryRequest 对尚未移动、修改或删除的上传提供可选幂等重放。
func (m *Manager) WriteInDirectoryRequest(ctx context.Context, parentUUID, name, content, requestID string) (*WriteReceipt, error) {
	if err := validEntryName(name); err != nil {
		return nil, err
	}
	dir, err := m.LocateDirectory(ctx, parentUUID)
	if err != nil {
		return nil, err
	}
	s, err := m.directoryStore()
	if err != nil {
		return nil, err
	}
	key, err := newDirectoryFilePath(dir, name)
	if err != nil {
		return nil, err
	}
	if requestID != "" {
		sum := sha256.Sum256([]byte(parentUUID + "\x00" + requestID))
		key = path.Join(dir.Path, fmt.Sprintf("request-%x", sum))
	}
	ctx = context.WithValue(ctx, directoryIntakeKey{}, DirectoryIntake{parentUUID, name})
	r, err := m.Write(ctx, key, content)
	if err != nil {
		if requestID != "" && errors.Is(err, ErrKeyExists) {
			row, e := m.store.GetMeta(ctx, key)
			if e != nil {
				return nil, e
			}
			if row == nil || row.IsDeleted {
				return nil, fmt.Errorf("request result unavailable")
			}
			items, e := s.DirectoryEntries(ctx, parentUUID)
			if e != nil {
				return nil, e
			}
			matched := false
			for _, item := range items {
				if item.UUID == row.UUID && item.Name == name {
					matched = true
				}
			}
			if !matched {
				return nil, fmt.Errorf("request in progress or parameters differ")
			}
			rf, e := m.ReadByLogic(ctx, key, 0)
			if e != nil {
				return nil, e
			}
			if string(rf.Content) != content {
				return nil, fmt.Errorf("request parameters differ")
			}
			rel, e := StoragePathOf(row.UUID, key)
			if e != nil {
				return nil, e
			}
			return &WriteReceipt{UUID: row.UUID, LogicPath: key, StorageRel: rel, SizeBytes: int64(len(content))}, nil
		}
		return nil, err
	}
	if _, atomic := s.(interface{ AtomicDirectoryIntake() }); !atomic {
		if err := s.LinkDirectoryFile(ctx, parentUUID, r.UUID, name); err != nil {
			return r, fmt.Errorf("file stored but directory link failed (uuid=%s): %w", r.UUID, err)
		}
	}
	return r, nil
}

func (m *Manager) ImportInDirectory(ctx context.Context, parentUUID, name, source string) (*ImportReceipt, error) {
	if err := validEntryName(name); err != nil {
		return nil, err
	}
	dir, err := m.LocateDirectory(ctx, parentUUID)
	if err != nil {
		return nil, err
	}
	s, err := m.directoryStore()
	if err != nil {
		return nil, err
	}
	key, err := newDirectoryFilePath(dir, name)
	if err != nil {
		return nil, err
	}
	ctx = context.WithValue(ctx, directoryIntakeKey{}, DirectoryIntake{parentUUID, name})
	r, err := m.ImportFile(ctx, key, source)
	if err != nil {
		return nil, err
	}
	if _, atomic := s.(interface{ AtomicDirectoryIntake() }); !atomic {
		if err := s.LinkDirectoryFile(ctx, parentUUID, r.UUID, name); err != nil {
			return r, fmt.Errorf("file stored but directory link failed (uuid=%s): %w", r.UUID, err)
		}
	}
	return r, nil
}

func (m *Manager) ListDirectory(ctx context.Context, uuid string) ([]DirectoryEntry, error) {
	if _, err := m.LocateDirectory(ctx, uuid); err != nil {
		return nil, err
	}
	s, err := m.directoryStore()
	if err != nil {
		return nil, err
	}
	return s.DirectoryEntries(ctx, uuid)
}

func (m *Manager) ModifyByUUID(ctx context.Context, uuid, content, mode string) error {
	m.mutation.Lock()
	defer m.mutation.Unlock()
	r, err := m.Locate(ctx, uuid)
	if err != nil {
		return err
	}
	if r.IsDeleted {
		return ErrDeleted
	}
	return m.modifyRef(ctx, r, content, mode)
}

type ImportEntryResult struct {
	Path  string `json:"path"`
	UUID  string `json:"uuid,omitempty"`
	Error string `json:"error,omitempty"`
}

// ImportDirectory 仅供受控本地来源使用。逐项回执，不把部分失败报告成整体成功；保留源文件。
func (m *Manager) ImportDirectory(ctx context.Context, parentUUID, source string) ([]ImportEntryResult, error) {
	if _, err := m.LocateDirectory(ctx, parentUUID); err != nil {
		return nil, err
	}
	info, err := os.Lstat(source)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("source must be a directory")
	}
	parents := map[string]string{".": parentUUID}
	results := []ImportEntryResult{}
	err = filepath.WalkDir(source, func(p string, entry os.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		rel, err := filepath.Rel(source, p)
		if err != nil {
			return err
		}
		if rel == "." {
			return walkErr
		}
		result := ImportEntryResult{Path: filepath.ToSlash(rel)}
		parent, ok := parents[filepath.Dir(rel)]
		if walkErr != nil {
			result.Error = walkErr.Error()
		} else if !ok {
			result.Error = "parent directory import failed"
		} else if entry.Type()&os.ModeSymlink != 0 {
			result.Error = "symbolic link not accepted"
		} else if entry.IsDir() {
			r, err := m.CreateChildDirectory(ctx, parent, entry.Name())
			if err != nil {
				result.Error = err.Error()
			} else {
				result.UUID = r.UUID
				parents[rel] = r.UUID
			}
		} else {
			r, err := m.ImportInDirectory(ctx, parent, entry.Name(), p)
			if r != nil {
				result.UUID = r.UUID
			}
			if err != nil {
				result.Error = err.Error()
			}
		}
		results = append(results, result)
		return nil
	})
	return results, err
}

// ExportDirectory 通过 UUID 逐项取件。调用方提供逐项权限检查；失败 ZIP 必须丢弃。
// 同名条目在包中加 UUID 后缀，并写映射清单。空目录有显式 ZIP 条目。
func (m *Manager) ExportDirectory(ctx context.Context, uuid string, dst io.Writer, authorize func(DirectoryEntry) error) error {
	if authorize == nil {
		return fmt.Errorf("directory export requires authorization")
	}
	w := zip.NewWriter(dst)
	seen := map[string]bool{}
	mapping := map[string]string{}
	var walk func(string, string) error
	walk = func(id, prefix string) error {
		if seen[id] {
			return fmt.Errorf("directory cycle detected")
		}
		seen[id] = true
		if err := ctx.Err(); err != nil {
			return err
		}
		items, err := m.ListDirectory(ctx, id)
		if err != nil {
			return err
		}
		used := map[string]bool{}
		for _, item := range items {
			if err := authorize(item); err != nil {
				return err
			}
			if err := validEntryName(item.Name); err != nil {
				return err
			}
			name := item.Name
			if used[name] {
				name = strings.TrimSuffix(name, path.Ext(name)) + " (" + item.UUID + ")" + path.Ext(name)
			}
			if used[name] {
				return fmt.Errorf("export name collision")
			}
			used[name] = true
			p := path.Join(prefix, name)
			mapping[p] = item.UUID
			if item.Kind == "dir" {
				if _, err := w.Create(p + "/"); err != nil {
					return err
				}
				if err := walk(item.UUID, p); err != nil {
					return err
				}
			} else {
				f, err := m.Open(ctx, item.UUID)
				if err != nil {
					return err
				}
				out, err := w.Create(p)
				if err == nil {
					_, err = io.Copy(out, f.Content)
				}
				closeErr := f.Content.Close()
				if err != nil {
					return err
				}
				if closeErr != nil {
					return closeErr
				}
			}
		}
		return nil
	}
	if err := walk(uuid, ""); err != nil {
		_ = w.Close()
		return err
	}
	manifest := "mabel-uuid-manifest.json"
	for mapping[manifest] != "" {
		manifest = "_" + manifest
	}
	out, err := w.Create(manifest)
	if err != nil {
		_ = w.Close()
		return err
	}
	if err := json.NewEncoder(out).Encode(mapping); err != nil {
		_ = w.Close()
		return err
	}
	return w.Close()
}
