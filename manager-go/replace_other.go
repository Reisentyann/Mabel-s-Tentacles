//go:build !windows

// 文件：manager-go/replace_other.go —— 非 Windows 同目录文件替换：通过 rename 发布完整内容
// 修改：2026-10-08（日期由 fresh-header.ps1 刷新）

package manager

import "os"

func replaceFile(source, target string) error { return os.Rename(source, target) }
