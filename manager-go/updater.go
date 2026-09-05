// 文件：manager-go/updater.go —— 更新回填域：T2 启动后台回填 + T3 手动重分析（字典第 10 节三触发器）
// 修改：2026-09-05（日期由 fresh-header.ps1 刷新）

// updater 域职责：让存量元数据跟上引擎演进。
// 执行器唯一路径：读文件 → describer.Analyze → MergeResults → Upsert → 喂食索引机。
// 幂等（家族整族替换保证），可中断续跑；循环由 timer/启动驱动，一轮结束即返回，
// 绝不自旋（铁律 3）。连续 3 轮盘上缺失 → SoftDeleteMeta（字典 10.4，
// 对接软删除待办）。
//
// 陈旧判定用 describer.IsStale（缺 ver / 版本落后 / checksum 漂 / mtime 新），
// execute_command 直改文件的绕口由此兜底。curVer 来源 describer.CurrentVersions()
// （版本表导出——交接文档缺口 1 的解法，不硬编码常量表）。

package manager

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/Reisentyann/Mabel-s-Tentacles/describer-go"
)

// backfillPageSize 游标分页每页行数（重分析上限 batch 由调用方传入）。
const backfillPageSize = 100

// ghostSoftDeleteRounds 盘上连续缺失多少轮后软删除元数据（字段字典 10.4）。
const ghostSoftDeleteRounds = 3

// AnalyzeReport T3 单文件重分析的产出报告（MCP analyze_file / HTTP analyze
// 返回给 agent 的事实视图）。
type AnalyzeReport struct {
	Path     string         `json:"path"`
	Families []string       `json:"families"` // 本次命中的插件家族（注册序）
	Attrs    map[string]any `json:"attrs"`    // 本次新产出的 cod- / sp-cod- 事实（展平）
}

// AnalyzeFile T3 手动入口：单文件重分析并落库，返回本次事实产出。
// 执行：resolve → stat（真实 mtime——IsStale 条件 4 口径）→ 读 head +
// 惰性全量 + 流式 checksum → Analyze → 读旧 attrs → MergeResults →
// Upsert → 喂索引。幂等：重复执行结果恒等。
func (m *Manager) AnalyzeFile(ctx context.Context, path string) (*AnalyzeReport, error) {
	start := time.Now()
	report, err := m.analyze(ctx, path)
	if err != nil {
		slog.Error("analyze file failed", "path", path, "error", err, "duration", time.Since(start).String())
		return nil, err
	}
	slog.Info("analyze file ok",
		"path", path, "families", report.Families, "cod_keys", len(report.Attrs),
		"duration", time.Since(start).String())
	return report, nil
}

// Backfill T2 一轮批量回填：游标分页扫描 → 陈旧才重分析（mtime 快路径先行，
// checksum 慢路径兜底）→ 盘上缺失计数（连续 3 轮软删除）。
// 返回本轮重分析的文件数。由装配层 goroutine 按 interval 反复调用
// （describe.backfill 配置，默认关闭）；一轮结束即返回，可中断续跑。
// batch 为本轮重分析上限（<=0 时取 backfillPageSize）；超限后余下文件
// 仍做缺失检查（stat 廉价），只跳过重分析。
func (m *Manager) Backfill(ctx context.Context, batch int) (int, error) {
	if batch <= 0 {
		batch = backfillPageSize
	}
	start := time.Now()
	analyzed, missing, softDeleted := 0, 0, 0
	since := ""
	for {
		rows, err := m.store.ListMetaPage(ctx, since, backfillPageSize)
		if err != nil {
			return analyzed, fmt.Errorf("backfill list page: %w", err)
		}
		if len(rows) == 0 {
			break // 扫到表尾
		}
		for _, row := range rows {
			since = row.Path
			if err := ctx.Err(); err != nil {
				return analyzed, err // 关停即断点（幂等可续跑）
			}

			// 盘上缺失：计数（3 轮 → 软删除）；存在：清零由 UpsertMeta 语义承担
			abs, serr := m.resolve(row.Path)
			var info os.FileInfo
			if serr == nil {
				info, serr = os.Stat(abs)
			}
			if serr != nil {
				if os.IsNotExist(serr) {
					missing++
					rounds, merr := m.store.MarkMissing(ctx, row.Path)
					if merr != nil {
						slog.Warn("backfill mark missing failed", "path", row.Path, "error", merr)
						continue
					}
					if rounds >= ghostSoftDeleteRounds {
						if derr := m.store.SoftDeleteMeta(ctx, row.Path); derr != nil {
							slog.Warn("backfill soft delete failed", "path", row.Path, "error", derr)
							continue
						}
						softDeleted++
						slog.Warn("ghost metadata soft-deleted", "path", row.Path, "missing_rounds", rounds)
					}
					continue
				}
				slog.Warn("backfill stat failed", "path", row.Path, "error", serr)
				continue
			}

			if analyzed >= batch {
				continue // 达本轮上限：只做上面的缺失检查，不重分析
			}
			attrs := describer.AttrsFromJSON(row.Attributes)
			if !m.stale(attrs, row.Checksum, abs, info) {
				continue
			}
			if _, aerr := m.analyze(ctx, row.Path); aerr != nil {
				slog.Warn("backfill analyze failed", "path", row.Path, "error", aerr)
				continue
			}
			analyzed++
		}
	}
	slog.Info("backfill round done",
		"analyzed", analyzed, "missing", missing, "soft_deleted", softDeleted,
		"duration", time.Since(start).String())
	return analyzed, nil
}

// analyze 执行器本体（AnalyzeFile 与 Backfill 逐文件复用）：
// resolve → stat → head 512B + 惰性全量（5MB 限读，引擎内部仍会再截——
// 双保险）+ 全文件流式 checksum → Analyze → 读旧 attrs → MergeResults →
// Upsert → 喂索引。T1 写路径（RecordFileMeta）内容在手不走这里。
func (m *Manager) analyze(ctx context.Context, path string) (*AnalyzeReport, error) {
	abs, err := m.resolve(path)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("stat: %w", err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("'%s' is a directory", path)
	}

	head, err := readHead(abs)
	if err != nil {
		return nil, fmt.Errorf("read head: %w", err)
	}
	cs, err := checksumFile(abs)
	if err != nil {
		return nil, fmt.Errorf("checksum: %w", err)
	}

	results := describer.Analyze(describer.Input{
		Path:    path,
		Head:    head,
		Size:    info.Size(),
		MTime:   info.ModTime(),
		ExtMime: m.extMimeOf(path),
	}, func() ([]byte, error) {
		return readLimited(abs, describer.MaxFullBytes)
	})

	// 读-改-写：整族合并 cod-*，保留 llm-* / sp-llm-*（无行 = 空开始）
	old := map[string]any{}
	row, gerr := m.store.GetMeta(ctx, path)
	if gerr != nil {
		return nil, fmt.Errorf("get meta: %w", gerr)
	}
	if row != nil {
		old = describer.AttrsFromJSON(row.Attributes)
	}
	merged := describer.MergeResults(old, results, time.Now())

	uuid, err := m.store.UpsertMeta(ctx, MetaRecord{
		Path:       path,
		SizeBytes:  info.Size(),
		Checksum:   cs,
		Attributes: describer.JSONFromAttrs(merged),
	})
	if err != nil {
		return nil, fmt.Errorf("upsert meta: %w", err)
	}
	if m.sink != nil {
		m.sink.Update(uuid, old, merged)
	}

	report := &AnalyzeReport{Path: path, Families: make([]string, 0, len(results)), Attrs: map[string]any{}}
	for _, r := range results {
		report.Families = append(report.Families, r.Family)
		for k, v := range r.Attrs {
			report.Attrs[k] = v
		}
	}
	return report, nil
}

// stale 陈旧判定（IsStale 四条件，快慢两段，字段字典 10.2）：
//   - 快路径（零 IO）：缺 ver / ver 落后 / mtime 新（条件 1/2/4）
//   - 慢路径（读全文件哈希）：checksum 漂移（条件 3——execute_command
//     直改文件的绕口对账锚点）
//
// 应适用家族：basic 恒查（从未分析的老数据由"缺 ver"兜住）；其余家族只在
// attrs 已有 cod-<family>-ver 时才查——从未适用（如图片无 text 家族）不算
// 陈旧；路由漏分析的场景由 checksum / mtime 漂移与 audit 对账兜底（已知取舍）。
func (m *Manager) stale(attrs map[string]any, storedChecksum, abs string, info os.FileInfo) bool {
	vers := describer.CurrentVersions()
	for family, cur := range vers {
		if !familyApplied(attrs, family) {
			continue
		}
		if describer.IsStale(attrs, family, cur, storedChecksum, "", info.ModTime()) {
			return true
		}
	}
	cs, err := checksumFile(abs)
	if err != nil {
		return false // 哈希失败不误伤（stat 已成功，读失败罕见；下一轮再试）
	}
	for family, cur := range vers {
		if !familyApplied(attrs, family) {
			continue
		}
		if describer.IsStale(attrs, family, cur, storedChecksum, cs, time.Time{}) {
			return true
		}
	}
	return false
}

// familyApplied 家族是否适用于该存量记录：basic 恒适用；其余以已产出
// cod-<family>-ver 为「曾适用」证据。
func familyApplied(attrs map[string]any, family string) bool {
	if family == "basic" {
		return true
	}
	_, ok := attrs["cod-"+family+"-ver"]
	return ok
}

// extMimeOf 扩展名 MIME（未注入推导函数时返回空——cod-basic-mime-match 不产出）。
func (m *Manager) extMimeOf(path string) string {
	if m.extMime == nil {
		return ""
	}
	return m.extMime(path)
}

// readHead 读文件前 512B（describer.MaxHeadBytes 口径；短文件按实际长度）。
func readHead(abs string) ([]byte, error) {
	f, err := os.Open(abs)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	buf := make([]byte, describer.MaxHeadBytes)
	n, err := io.ReadFull(f, buf)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return nil, err
	}
	return buf[:n], nil
}

// readLimited 读文件前 limit 字节（Loader 全量预算；引擎内部仍会再截，
// 双保险——execute_command 可产出超大文件，这里先挡住内存峰值）。
func readLimited(abs string, limit int64) ([]byte, error) {
	f, err := os.Open(abs)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, limit))
}

// checksumFile 全文件流式 SHA-256（与 T1 的全量 content 哈希同口径；
// 流式不受 5MB 分析预算影响——checksum 是完整性事实，必须覆盖全文件）。
func checksumFile(abs string) (string, error) {
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
