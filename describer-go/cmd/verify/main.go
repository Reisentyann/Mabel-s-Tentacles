// 文件：describer-go/cmd/verify/main.go —— L2 夹具验证器：跑全插件流水线出 JSON 报告（无 DB）
// 修改：2026-09-03（日期由 fresh-header.ps1 刷新）

// verify 夹具验证器：对目录下每个文件跑完整 Analyze，输出 family→attrs 的 JSON 报告，
// 供 test/测试规则.md 的断言表核对。无 DB，纯 describer 引擎直跑。
//
// 用法：go run ./cmd/verify <fixtures目录> [输出.json]
//
// 注意：本工具不传 ExtMime，报告不含 cod-basic-mime-match（该字段只在 DB 写入链路产出）。
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/Reisentyann/Mabel-s-Tentacles/describer-go"
	_ "github.com/Reisentyann/Mabel-s-Tentacles/describer-go/all"
)

const maxFull = 5 << 20 // 与 maxFileSize 一致的 5MB 预算

func main() {
	flag.Parse()
	root := flag.Arg(0)
	if root == "" {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/verify <fixtures-dir> [out.json]")
		os.Exit(1)
	}
	report := map[string]map[string]any{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Name() == "清单.md" {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		full, err := readUpTo(path, maxFull)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		head := full
		if len(head) > 512 {
			head = head[:512]
		}
		results := describer.Analyze(describer.Input{
			Path:  filepath.ToSlash(mustRel(root, path)),
			Head:  head,
			Full:  full,
			Size:  info.Size(),
			MTime: info.ModTime(),
		}, nil)
		fam := map[string]any{}
		for _, r := range results {
			fam[r.Family] = r.Attrs
		}
		report[filepath.ToSlash(mustRel(root, path))] = fam
		return nil
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "walk:", err)
		os.Exit(1)
	}
	b, _ := json.MarshalIndent(report, "", "  ")
	if out := flag.Arg(1); out != "" {
		if err := os.WriteFile(out, b, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "write:", err)
			os.Exit(1)
		}
		fmt.Printf("report written: %s (%d files)\n", out, len(report))
	} else {
		fmt.Println(string(b))
	}
}

func readUpTo(path string, max int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, max))
}

func mustRel(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return rel
}
