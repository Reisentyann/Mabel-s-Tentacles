// 文件：manager-go/updater.go —— 更新回填域：T2 启动后台回填 + T3 手动重分析（字典第 10 节三触发器）
// 修改：2026-09-03（日期由 fresh-header.ps1 刷新）

// updater 域职责：让存量元数据跟上引擎演进。
// 执行器唯一路径：读文件 → describer.Analyze → MergeResults → Upsert → 喂食索引机。
// 幂等（家族整族替换保证），可中断续跑；循环由 timer/启动驱动，一轮结束即返回，
// 绝不自旋（铁律 3）。连续 3 轮盘上缺失 → 编排 repo.SoftDeleteMetadata
// （字典 10.4，对接软删除待办）。
//
// 陈旧判定用 describer.IsStale（缺 ver / 版本落后 / checksum 漂 / mtime 新），
// execute_command 直改文件的绕口由此兜底。

package manager

import (
	"context"
	"errors"
)

// errNoUpdater 域内通用未实现错误（骨架期）。
var errNoUpdater = errors.New("manager: updater not implemented")

// AnalyzeFile T3 手动入口：单文件重分析并落库。
// TODO 实现批次：读文件（placement 域要 dataDir）→ Analyze → Merge → Upsert → idx.Update。
// 对应 MCP analyze_file 工具与 HTTP POST /api/files/analyze（T3 待办）。
func (m *Manager) AnalyzeFile(ctx context.Context, path string) error {
	return errNoUpdater
}

// Backfill T2 一轮批量回填：分页扫描 file_metadata，陈旧才重分析，限速批处理。
// 返回本轮重分析的文件数。由服务启动后的后台 goroutine 按 interval 反复调用
// （describe.backfill 配置，默认关闭）。
// TODO 实现批次：分页读元数据 → IsStale 过滤 → AnalyzeFile → 计数。
func (m *Manager) Backfill(ctx context.Context) (int, error) {
	return 0, errNoUpdater
}
