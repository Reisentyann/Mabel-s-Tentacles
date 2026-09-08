// 文件：mcp-server-go/internal/service/archive.go —— zip 打包：多文件流式写入（物理路径由调用方 uuid 派生，归档名保留逻辑路径）
// 修改：2026-09-08（日期由 fresh-header.ps1 刷新）

package service

import (
	"archive/zip"
	"io"
	"os"
)

// ZipEntry 一个待打包条目：AbsPath 为 uuid 派生的物理绝对路径
// （调用方已过防穿越），ArchiveName 为逻辑路径（zip 内的用户可读名）。
type ZipEntry struct {
	AbsPath     string
	ArchiveName string
}

// ZipFiles 把多个文件流式打包写入 w（zip 格式）。只读源文件、不删除；
// 条目不存在/目录会被跳过。返回实际打包的文件数量。
func ZipFiles(entries []ZipEntry, w io.Writer) (int, error) {
	zw := zip.NewWriter(w)
	count := 0
	for _, e := range entries {
		info, err := os.Stat(e.AbsPath)
		if err != nil || info.IsDir() {
			continue
		}
		f, err := os.Open(e.AbsPath)
		if err != nil {
			continue
		}
		zf, err := zw.Create(e.ArchiveName)
		if err != nil {
			f.Close()
			continue
		}
		if _, err := io.Copy(zf, f); err != nil {
			f.Close()
			continue
		}
		f.Close()
		count++
	}
	if err := zw.Close(); err != nil {
		return count, err
	}
	return count, nil
}
