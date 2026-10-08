// 文件：manager-go/directory_export.go —— 带逐项授权及 UUID 清单的 ZIP 目录导出
// 修改：2026-10-08（日期由 fresh-header.ps1 刷新）

package manager

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"strings"
)

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
