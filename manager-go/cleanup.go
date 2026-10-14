// 文件：manager-go/cleanup.go —— 显式清理计划与按天归档已完成发布的临时目录项
// 修改：2026-10-08（日期由 fresh-header.ps1 刷新）

package manager

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Reisentyann/Mabel-s-Tentacles/common"
)

// CleanupOptions 显式指定归档根（项目的旧内容目录），必须在 dataDir 之外。
// MinAge 为候选最短存续时间，零值使用 24 小时；MaxFiles 零值使用 100。
type CleanupOptions struct {
	ArchiveRoot string
	MinAge      time.Duration
	MaxFiles    int
}

// CleanupCandidate 仅表示可归档的 .partial 目录项，不表示可释放的物理字节。
type CleanupCandidate struct {
	Path       string    `json:"path"`
	UUID       string    `json:"uuid"`
	SizeBytes  int64     `json:"size_bytes"`
	ModifiedAt time.Time `json:"modified_at"`
}

// CleanupPlan 包含候选和仅报告的文件；路径均相对于 dataDir。
type CleanupPlan struct {
	ID         string             `json:"id"`
	CreatedAt  time.Time          `json:"created_at"`
	Options    CleanupOptions     `json:"options"`
	Candidates []CleanupCandidate `json:"candidates"`
	Reported   []string           `json:"reported"`
}

// CleanupResult 是逐项归档结果，Status 为 archived、skipped 或 failed。
type CleanupResult struct {
	Path        string `json:"path"`
	Destination string `json:"destination"`
	Status      string `json:"status"`
	Reason      string `json:"reason,omitempty"`
}

// CleanupReport 包含批次目录及逐项结果，不承诺释放磁盘空间。
type CleanupReport struct {
	BatchDir string          `json:"batch_dir"`
	Results  []CleanupResult `json:"results"`
}

func (m *Manager) cleanupOptions(o CleanupOptions) (CleanupOptions, error) {
	if !filepath.IsAbs(o.ArchiveRoot) {
		return o, fmt.Errorf("cleanup: archive root must be absolute")
	}
	data, err := filepath.Abs(m.dataDir)
	if err != nil {
		return o, err
	}
	// 归档根必须已经存在，以便解析符号链接并核对实际位置。
	o.ArchiveRoot, err = filepath.EvalSymlinks(o.ArchiveRoot)
	if err != nil {
		return o, err
	}
	if real, err := filepath.EvalSymlinks(data); err == nil {
		data = real
	} else if !os.IsNotExist(err) {
		return o, err
	}
	if common.WithinDir(data, o.ArchiveRoot) || common.WithinDir(o.ArchiveRoot, data) {
		return o, fmt.Errorf("cleanup: archive and data roots must be separate")
	}
	if o.MinAge == 0 {
		o.MinAge = 24 * time.Hour
	}
	if o.MaxFiles == 0 {
		o.MaxFiles = 100
	}
	if o.MinAge < 0 || o.MaxFiles < 1 || o.MaxFiles > 1000 {
		return o, fmt.Errorf("cleanup: invalid age or batch limit")
	}
	return o, nil
}

// PlanCleanup 只读扫描；仅选择已发布且与正式文件同一身份的 .partial。
// 无归属、修改暂存及不满足条件的临时文件仅报告，不自动归档。
func (m *Manager) PlanCleanup(ctx context.Context, options CleanupOptions) (*CleanupPlan, error) {
	o, err := m.cleanupOptions(options)
	if err != nil {
		return nil, err
	}
	refs := map[string]string{}
	since := ""
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		rows, err := m.store.ListMetaPageAll(ctx, since, 200)
		if err != nil {
			return nil, fmt.Errorf("cleanup metadata: %w", err)
		}
		if len(rows) == 0 {
			break
		}
		if rows[len(rows)-1].Path <= since {
			return nil, fmt.Errorf("cleanup: metadata cursor did not advance")
		}
		for _, row := range rows {
			rel, err := StoragePathOf(row.UUID, row.Path)
			if err != nil {
				return nil, err
			}
			refs[rel] = row.UUID
		}
		since = rows[len(rows)-1].Path
	}
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return nil, err
	}
	p := &CleanupPlan{ID: fmt.Sprintf("%x", id), CreatedAt: time.Now(), Options: o, Candidates: []CleanupCandidate{}, Reported: []string{}}
	files, err := auditPhysicalFiles(ctx, m.dataDir)
	if err != nil {
		return nil, err
	}
	for _, rel := range files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if refs[rel] != "" {
			continue
		}
		uuid := refs[strings.TrimSuffix(rel, ".partial")]
		if !strings.HasSuffix(rel, ".partial") || uuid == "" || len(p.Candidates) >= o.MaxFiles {
			p.Reported = append(p.Reported, rel)
			continue
		}
		info, err := m.cleanupPartial(ctx, rel, uuid, o.MinAge)
		if errors.Is(err, errCleanupUnsafe) {
			p.Reported = append(p.Reported, rel)
			continue
		}
		if err != nil {
			return nil, err
		}
		p.Candidates = append(p.Candidates, CleanupCandidate{Path: rel, UUID: uuid, SizeBytes: info.Size(), ModifiedAt: info.ModTime()})
	}
	return p, nil
}

var errCleanupUnsafe = errors.New("cleanup: file no longer eligible")

func (m *Manager) cleanupPartial(ctx context.Context, rel, uuid string, age time.Duration) (os.FileInfo, error) {
	if err := validLogicPath(rel); err != nil {
		return nil, errCleanupUnsafe
	}
	if !strings.HasSuffix(rel, ".partial") {
		return nil, errCleanupUnsafe
	}
	ref, err := m.store.GetMetaByUUID(ctx, uuid)
	if err != nil {
		return nil, err
	}
	if ref == nil {
		return nil, errCleanupUnsafe
	}
	current, err := StoragePathOf(uuid, ref.Path)
	if err != nil {
		return nil, err
	}
	if current+".partial" != rel {
		return nil, errCleanupUnsafe
	}
	if err := m.requirePublished(ctx, uuid); err != nil {
		if errors.Is(err, ErrPending) {
			return nil, errCleanupUnsafe
		}
		return nil, err
	}
	abs, err := m.resolve(rel)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(abs)
	if os.IsNotExist(err) {
		return nil, errCleanupUnsafe
	}
	if err != nil {
		return nil, err
	}
	formal, err := os.Lstat(strings.TrimSuffix(abs, ".partial"))
	if os.IsNotExist(err) {
		return nil, errCleanupUnsafe
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || !formal.Mode().IsRegular() || !os.SameFile(info, formal) || time.Since(info.ModTime()) < age {
		return nil, errCleanupUnsafe
	}
	return info, nil
}

// ApplyCleanup 显式执行计划，按本地日期归档到 ArchiveRoot/管理机清理/日期/批次。
// 等待单实例在途入库结束，并与修改、移动串行。每次移动前重新验证。
// 跨卷 Rename 失败则保留源文件；清单先落盘，结果追加同步到 results.jsonl。
func (m *Manager) ApplyCleanup(ctx context.Context, p *CleanupPlan) (*CleanupReport, error) {
	if p == nil || len(p.ID) != 32 || p.CreatedAt.IsZero() {
		return nil, fmt.Errorf("cleanup: invalid plan")
	}
	for _, c := range p.ID {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return nil, fmt.Errorf("cleanup: invalid batch ID")
		}
	}
	o, err := m.cleanupOptions(p.Options)
	if err != nil {
		return nil, err
	}
	if len(p.Candidates) > o.MaxFiles {
		return nil, fmt.Errorf("cleanup: batch limit exceeded")
	}
	m.intake.Lock()
	defer m.intake.Unlock()
	m.mutation.Lock()
	defer m.mutation.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	batch, err := common.ResolveWithin(o.ArchiveRoot, filepath.ToSlash(filepath.Join("管理机清理", p.CreatedAt.Format("2006-01-02"), p.ID)))
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(batch, 0o755); err != nil {
		return nil, err
	}
	manifest, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := writeCleanupManifest(filepath.Join(batch, "manifest.json"), manifest); err != nil {
		return nil, err
	}
	log, err := os.OpenFile(filepath.Join(batch, "results.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	defer log.Close()
	report := &CleanupReport{BatchDir: batch, Results: []CleanupResult{}}
	for _, c := range p.Candidates {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		r := CleanupResult{Path: c.Path, Status: "skipped"}
		info, err := m.cleanupPartial(ctx, c.Path, c.UUID, o.MinAge)
		if errors.Is(err, errCleanupUnsafe) {
			r.Reason = err.Error()
		} else if err != nil {
			return report, err // 数据库或文件状态查询失败，不继续处置。
		} else if info.Size() != c.SizeBytes || !info.ModTime().Equal(c.ModifiedAt) {
			r.Reason = "file changed since planning"
		} else {
			src, err := m.resolve(c.Path)
			if err != nil {
				return report, err
			}
			dst, err := common.ResolveWithin(batch, "data/"+c.Path)
			if err != nil {
				return report, err
			}
			r.Destination = dst
			// 独占创建每项容器，避免 Rename 在 Unix 上覆盖已有目标。
			container := dst + ".archive"
			if err := os.MkdirAll(filepath.Dir(container), 0o755); err != nil {
				return report, err
			}
			if err := os.Mkdir(container, 0o755); err != nil {
				r.Reason = "archive destination already reserved or unavailable: " + err.Error()
			} else {
				r.Destination = filepath.Join(container, "content")
				if err := os.Rename(src, r.Destination); err != nil {
					r.Status = "failed"
					r.Reason = err.Error()
				} else {
					r.Status = "archived"
				}
			}
		}
		report.Results = append(report.Results, r)
		if err := json.NewEncoder(log).Encode(r); err != nil {
			return report, err
		}
		if err := log.Sync(); err != nil {
			return report, err
		}
	}
	return report, nil
}

func writeCleanupManifest(name string, content []byte) error {
	f, err := os.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if os.IsExist(err) {
		old, readErr := os.ReadFile(name)
		if readErr != nil {
			return readErr
		}
		if string(old) != string(content) {
			return fmt.Errorf("cleanup: batch manifest differs")
		}
		return nil
	}
	if err != nil {
		return err
	}
	_, writeErr := f.Write(content)
	return errors.Join(writeErr, f.Sync(), f.Close())
}
