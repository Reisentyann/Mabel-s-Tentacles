package service

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const maxFileSize = 5 * 1024 * 1024

func SafeWrite(baseDir, filePath, content string) error {
	if len(content) > maxFileSize {
		return fmt.Errorf("security error: file content exceeds 5MB limit")
	}

	clean := strings.TrimLeft(filePath, `/\`)
	if strings.Contains(clean, ":") {
		return fmt.Errorf("security error: path cannot contain drive letters")
	}

	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return fmt.Errorf("resolve base dir: %w", err)
	}

	target := filepath.Clean(filepath.Join(absBase, clean))
	if !withinDir(absBase, target) {
		return fmt.Errorf("security error: directory traversal detected and blocked")
	}

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write file: %w", err)
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
