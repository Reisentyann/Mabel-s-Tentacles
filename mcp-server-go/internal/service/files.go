// 文件：mcp-server-go/internal/service/files.go —— 文件安全操作：SafeWrite/SafeRead/SafeModify/SafeList + 防目录穿越 + 5MB 上限
// 修改：2026-09-03（日期由 fresh-header.ps1 刷新）

package service

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const maxFileSize = 5 * 1024 * 1024

func resolveWithin(baseDir, filePath string) (string, error) {
	clean := strings.TrimLeft(filePath, `/\`)
	if strings.Contains(clean, ":") {
		return "", fmt.Errorf("security error: path cannot contain drive letters")
	}

	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return "", fmt.Errorf("resolve base dir: %w", err)
	}

	target := filepath.Clean(filepath.Join(absBase, clean))
	if !withinDir(absBase, target) {
		return "", fmt.Errorf("security error: directory traversal detected and blocked")
	}
	return target, nil
}

func SafeWrite(baseDir, filePath, content string) error {
	if len(content) > maxFileSize {
		return fmt.Errorf("security error: file content exceeds 5MB limit")
	}

	target, err := resolveWithin(baseDir, filePath)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}
	if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	return nil
}

func SafeModify(baseDir, filePath, content, mode string) error {
	if len(content) > maxFileSize {
		return fmt.Errorf("security error: file content exceeds 5MB limit")
	}

	target, err := resolveWithin(baseDir, filePath)
	if err != nil {
		return err
	}

	info, err := os.Stat(target)
	if err != nil {
		return fmt.Errorf("error: file '%s' does not exist", filePath)
	}
	if info.IsDir() {
		return fmt.Errorf("error: path '%s' is not a file", filePath)
	}

	switch mode {
	case "append":
		f, err := os.OpenFile(target, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return fmt.Errorf("open file: %w", err)
		}
		defer f.Close()
		if _, err := f.WriteString(content); err != nil {
			return fmt.Errorf("append file: %w", err)
		}
	case "overwrite":
		if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
			return fmt.Errorf("overwrite file: %w", err)
		}
	default:
		return fmt.Errorf("error: invalid mode '%s', must be 'append' or 'overwrite'", mode)
	}
	return nil
}

func SafeList(baseDir string) ([]string, error) {
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return nil, fmt.Errorf("resolve base dir: %w", err)
	}
	if err := os.MkdirAll(absBase, 0o755); err != nil {
		return nil, fmt.Errorf("create base dir: %w", err)
	}

	var files []string
	err = filepath.WalkDir(absBase, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, rerr := filepath.Rel(absBase, path)
		if rerr != nil {
			return rerr
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk dir: %w", err)
	}
	return files, nil
}

func withinDir(dir, target string) bool {
	rel, err := filepath.Rel(dir, target)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// ResolvePath 校验并解析 baseDir 内的相对路径为绝对路径，防目录穿越。
func ResolvePath(baseDir, filePath string) (string, error) {
	return resolveWithin(baseDir, filePath)
}

// SafeRead 读取 baseDir 内的文件内容，防目录穿越。
func SafeRead(baseDir, filePath string) ([]byte, error) {
	target, err := resolveWithin(baseDir, filePath)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(target)
	if err != nil {
		return nil, fmt.Errorf("error: file '%s' does not exist", filePath)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("error: path '%s' is not a file", filePath)
	}
	return os.ReadFile(target)
}

type FileNode struct {
	Name       string      `json:"name"`
	Path       string      `json:"path"`
	Type       string      `json:"type"`
	Size       int64       `json:"size,omitempty"`
	ModifiedAt float64     `json:"modified_at,omitempty"`
	Children   []*FileNode `json:"children,omitempty"`
}

// ListTree 返回 dataDir 的递归目录树。
func ListTree(baseDir string) ([]*FileNode, error) {
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return nil, fmt.Errorf("resolve base dir: %w", err)
	}
	return buildTree(absBase, ""), nil
}

func buildTree(base, rel string) []*FileNode {
	dir := base
	if rel != "" {
		dir = filepath.Join(base, rel)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var nodes []*FileNode
	for _, e := range entries {
		name := e.Name()
		relPath := name
		if rel != "" {
			relPath = filepath.ToSlash(filepath.Join(rel, name))
		}

		if e.IsDir() {
			nodes = append(nodes, &FileNode{Name: name, Path: relPath, Type: "dir", Children: buildTree(base, relPath)})
			continue
		}

		info, err := e.Info()
		var size int64
		var mod float64
		if err == nil {
			size = info.Size()
			mod = float64(info.ModTime().UnixNano()) / 1e9
		}
		nodes = append(nodes, &FileNode{Name: name, Path: relPath, Type: "file", Size: size, ModifiedAt: mod})
	}
	return nodes
}
