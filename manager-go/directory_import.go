// 文件：manager-go/directory_import.go —— 本地目录递归导入与逐项结果
// 修改：2026-10-08（日期由 fresh-header.ps1 刷新）

package manager

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// ImportEntryResult 记录每个相对路径的导入结果。
type ImportEntryResult struct {
	Path  string `json:"path"`
	UUID  string `json:"uuid,omitempty"`
	Error string `json:"error,omitempty"`
}

// ImportDirectory 仅供受控本地来源使用。逐项回执，保留源文件。
// 调用方必须检查每项 Error，返回的 error 只表示目录遍历失败。
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
