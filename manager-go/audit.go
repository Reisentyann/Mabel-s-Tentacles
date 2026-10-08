// 文件：manager-go/audit.go —— 对账域：盘 vs DB 巡检（孤儿/幽灵/重复），只报告不动手
// 修改：2026-10-08（日期由 fresh-header.ps1 刷新）

// audit 域职责：文件系统与数据库的偏差巡检。
// 发现三类问题并出报告给 agent（铁律 2：自动只巡检报告，动手必须显式）：
//   - 孤儿文件：盘上存在、DB 无元数据（execute_command 绕过写入路径的产物）
//   - 幽灵元数据：DB 有记录、盘上文件已消失（连续 3 轮缺失联动 updater → SoftDelete）
//   - 重复 checksum：内容相同的多份文件（磁盘浪费线索）
// 报告是 agent 决策的情报源，不是行动指令。

package manager

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// AuditReport 一轮对账结果（情报，不是行动指令）。
type AuditReport struct {
	Orphans      []string     `json:"orphans"`
	Ghosts       []AuditGhost `json:"ghosts"`
	DupChecksums []DupGroup   `json:"dup_checksums"`
}

// AuditGhost 元数据存在但派生物理文件缺失的记录。
type AuditGhost struct {
	Path          string `json:"path"`
	UUID          string `json:"uuid"`
	MissingRounds int    `json:"missing_rounds"`
	IsDeleted     bool   `json:"is_deleted"`
}

// DupGroup 同一 checksum 的多路径组。
type DupGroup struct {
	Checksum string   `json:"checksum"`
	Paths    []string `json:"paths"`
}

// Audit 执行一轮盘 vs DB 对账。
func (m *Manager) Audit(ctx context.Context) (*AuditReport, error) {
	rows := make([]MetaRow, 0)
	since := ""
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		page, err := m.store.ListMetaPageAll(ctx, since, 200)
		if err != nil {
			return nil, fmt.Errorf("audit metadata scan: %w", err)
		}
		if len(page) == 0 {
			break
		}
		if page[len(page)-1].Path <= since {
			return nil, fmt.Errorf("audit metadata cursor did not advance")
		}
		rows = append(rows, page...)
		since = page[len(page)-1].Path
	}

	known := make(map[string]struct{}, len(rows))
	duplicates := make(map[string][]string)
	for _, row := range rows {
		rel, err := StoragePathOf(row.UUID, row.Path)
		if err != nil {
			return nil, err
		}
		known[rel] = struct{}{}
		if strings.TrimSpace(row.Checksum) != "" && !row.IsDeleted {
			duplicates[row.Checksum] = append(duplicates[row.Checksum], row.Path)
		}
	}
	report := &AuditReport{Orphans: []string{}, Ghosts: []AuditGhost{}, DupChecksums: []DupGroup{}}
	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		abs, err := m.storageAbs(row.UUID, row.Path)
		if err != nil {
			return nil, fmt.Errorf("audit resolve %q: %w", row.Path, err)
		}
		if info, err := os.Lstat(abs); os.IsNotExist(err) {
			report.Ghosts = append(report.Ghosts, AuditGhost{Path: row.Path, UUID: row.UUID, MissingRounds: row.MissingRounds, IsDeleted: row.IsDeleted})
		} else if err != nil {
			return nil, fmt.Errorf("audit stat %q: %w", row.Path, err)
		} else if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("audit %q: expected regular file", row.Path)
		}
	}

	files, err := auditPhysicalFiles(ctx, m.dataDir)
	if err != nil {
		return nil, fmt.Errorf("audit physical scan: %w", err)
	}
	for _, rel := range files {
		if strings.HasSuffix(rel, ".partial") {
			if _, owned := known[strings.TrimSuffix(rel, ".partial")]; owned {
				continue
			}
		}
		if _, ok := known[rel]; !ok {
			report.Orphans = append(report.Orphans, rel)
		}
	}
	for checksum, paths := range duplicates {
		if len(paths) > 1 {
			sort.Strings(paths)
			report.DupChecksums = append(report.DupChecksums, DupGroup{Checksum: checksum, Paths: paths})
		}
	}
	sort.Strings(report.Orphans)
	sort.Slice(report.Ghosts, func(i, j int) bool { return report.Ghosts[i].Path < report.Ghosts[j].Path })
	sort.Slice(report.DupChecksums, func(i, j int) bool { return report.DupChecksums[i].Checksum < report.DupChecksums[j].Checksum })
	return report, nil
}

func auditPhysicalFiles(ctx context.Context, root string) ([]string, error) {
	files := []string{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	return files, err
}
