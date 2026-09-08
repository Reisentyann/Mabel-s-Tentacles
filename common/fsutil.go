// 文件：common/fsutil.go —— 跨模块文件安全层：防目录穿越解析（ResolveWithin/WithinDir）+ 盘读助手（ReadHead/ReadLimited/ChecksumFile）
// 修改：2026-09-06（日期由 fresh-header.ps1 刷新）

// 本文件是三处逐字复制（manager-go/updater.go、mcp-server-go/core/executor.go、
// mcp-server-go/internal/service/files.go、describer-go/cmd/verify）的收敛点。
// 预算口径常量不在此持有——正主在 describer（MaxHeadBytes/MaxFullBytes），
// 调用方传参进来，common 保持零业务依赖。
package common

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	securejoin "github.com/cyphar/filepath-securejoin"
)

// ResolveWithin 把 baseDir 内的相对路径解析为绝对路径（防目录穿越 / 盘符 /
// symlink 逃逸）。跨平台路径安全由 cyphar/filepath-securejoin 承担
// （runc / containerd 同款）——手写字符串判定在 Linux（symlink 指向界外）
// 与 Windows（盘符/junction）各有暗坑，成熟库逐段解析兜底。
// 错误文案是对外契约（盘符/`..` 穿越两条沿用既有文案），勿改写。
//
// 防线三段：① 词法预检管 `..` 越界与盘符；② securejoin 逐段解析 symlink
// （尾段不存在也安全——写场景）；③ fail-closed 双检——解析结果越界
// （symlink 逃逸）或与词法形态不一致（symlink 改写路径，含指向界内的
// 改写）一律拒绝。data 内快捷方式被一并拒掉是已知取舍：静默跟随改写
// 路径比拒绝危险（打开的不是调用方要的文件）。
func ResolveWithin(baseDir, rel string) (string, error) {
	clean := strings.TrimLeft(rel, `/\`)
	if strings.Contains(clean, ":") {
		return "", fmt.Errorf("security error: path cannot contain drive letters")
	}
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return "", fmt.Errorf("resolve base dir: %w", err)
	}
	// ① 词法预检：`..` 越界（既有契约文案）
	target := filepath.Clean(filepath.Join(absBase, clean))
	if !WithinDir(absBase, target) {
		return "", fmt.Errorf("security error: directory traversal detected and blocked")
	}
	// base 自身为 symlink 时先解析，与 securejoin 的解析基准对齐；
	// base 尚不存在（首次写场景）时用词法 base——首段解析自然短路
	realBase := absBase
	if rb, rerr := filepath.EvalSymlinks(absBase); rerr == nil {
		realBase = rb
	}
	// ② 逐段解析 symlink（尾段不存在安全）
	resolved, err := securejoin.SecureJoin(realBase, clean)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}
	// ③ fail-closed 双检：仍在界内 + 形态与词法一致（symlink 改写即拒）
	if !WithinDir(realBase, resolved) {
		return "", fmt.Errorf("security error: symlink escape detected and blocked")
	}
	got, rerr := filepath.Rel(realBase, resolved)
	if rerr != nil || filepath.ToSlash(got) != filepath.ToSlash(filepath.Clean(clean)) {
		return "", fmt.Errorf("security error: symlink path rewriting detected and blocked")
	}
	return resolved, nil
}

// WithinDir target 是否仍在 dir 内（.. 前缀 = 越界）。
func WithinDir(dir, target string) bool {
	rel, err := filepath.Rel(dir, target)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// ReadHead 读文件前 headBytes 字节（短文件按实际长度）。
// headBytes 口径由调用方传入（引擎嗅探预算正主 = describer.MaxHeadBytes）。
func ReadHead(abs string, headBytes int) ([]byte, error) {
	f, err := os.Open(abs)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	buf := make([]byte, headBytes)
	n, err := io.ReadFull(f, buf)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return nil, err
	}
	return buf[:n], nil
}

// ReadLimited 读文件前 limit 字节（Loader 全量预算；引擎内部仍会再截，
// 双保险——execute_command 可产出超大文件，这里先挡住内存峰值）。
func ReadLimited(abs string, limit int64) ([]byte, error) {
	f, err := os.Open(abs)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, limit))
}

// ChecksumFile 全文件流式 SHA-256（checksum 是完整性事实，必须覆盖全文件，
// 不受 5MB 分析预算影响）。
func ChecksumFile(abs string) (string, error) {
	f, err := os.Open(abs)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
