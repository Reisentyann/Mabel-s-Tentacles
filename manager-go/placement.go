// 文件：manager-go/placement.go —— UUID 位置派生与逻辑路径校验
// 修改：2026-10-08（日期由 fresh-header.ps1 刷新）

package manager

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/Reisentyann/Mabel-s-Tentacles/common"
)

// resolve 把 dataDir 内的相对路径解析为绝对路径（防目录穿越 / 盘符）。
// 判定本体收敛到 common.ResolveWithin（原内化自装配层 files.go 的
// 同源副本退役——manager 不再反向 import 装配层，语义与文案不变）。
func (m *Manager) resolve(rel string) (string, error) {
	return common.ResolveWithin(m.dataDir, rel)
}

// Resolve 凭 uuid 解析文件逻辑路径——唯一知情者的核心问答（fetch.Locate
// 的纯路径薄壳；需要位置 + 展示元数据的调用方直接用 Locate）。
// 哨兵语义与 Locate 一致：DB 无行 → ErrNotFound；软删行照报路径。
func (m *Manager) Resolve(ctx context.Context, uuid string) (string, error) {
	ref, err := m.Locate(ctx, uuid)
	if err != nil {
		return "", err
	}
	return ref.Path, nil
}

// StoragePathOf 派生物理相对路径 <uuid前2位>/<uuid><ext>，返回正斜杠形式。
func StoragePathOf(uuid, logicPath string) (string, error) {
	if len(uuid) < 2 {
		return "", fmt.Errorf("storage path: bad uuid %q", uuid)
	}
	return path.Join(uuid[:2], uuid+path.Ext(logicPath)), nil
}

// StorageAbs 派生并校验 dataDir 内的文件绝对路径。
func (m *Manager) StorageAbs(uuid, logicPath string) (string, error) {
	rel, err := StoragePathOf(uuid, logicPath)
	if err != nil {
		return "", err
	}
	return m.resolve(rel)
}

func (m *Manager) storageAbs(uuid, logicPath string) (string, error) {
	return m.StorageAbs(uuid, logicPath)
}

// validLogicPath 拒绝空路径、绝对路径、盘符和 . / .. / 空段。
func validLogicPath(p string) error {
	if p == "" || strings.HasPrefix(p, "/") || strings.HasPrefix(p, `\`) {
		return ErrInvalidPath
	}
	if strings.Contains(p, ":") {
		return fmt.Errorf("security error: path cannot contain drive letters")
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == "" || seg == "." || seg == ".." || strings.Contains(seg, `\`) {
			return ErrInvalidPath
		}
	}
	return nil
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

func logicPrefixes(p string) []string {
	parts := strings.Split(p, "/")
	prefixes := make([]string, len(parts))
	for i := range parts {
		prefixes[i] = strings.Join(parts[:i+1], "/")
	}
	return prefixes
}
