package service

import (
	"archive/zip"
	"io"
	"os"
)

// ZipFiles 将 dataDir 下的多个文件打包写入 w（zip 格式）。
// 只读源文件、不删除；paths 是相对 dataDir 的路径，非法/不存在/目录会被跳过。
// 返回实际打包的文件数量。
func ZipFiles(dataDir string, paths []string, w io.Writer) (int, error) {
	zw := zip.NewWriter(w)
	count := 0
	for _, p := range paths {
		target, err := resolveWithin(dataDir, p)
		if err != nil {
			continue
		}
		info, err := os.Stat(target)
		if err != nil || info.IsDir() {
			continue
		}
		f, err := os.Open(target)
		if err != nil {
			continue
		}
		zf, err := zw.Create(p)
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
