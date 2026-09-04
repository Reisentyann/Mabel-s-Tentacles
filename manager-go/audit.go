// 文件：manager-go/audit.go —— 对账域：盘 vs DB 巡检（孤儿/幽灵/重复），只报告不动手
// 修改：2026-09-03（日期由 fresh-header.ps1 刷新）

// audit 域职责：文件系统与数据库的偏差巡检。
// 发现三类问题并出报告给 agent（铁律 2：自动只巡检报告，动手必须显式）：
//   - 孤儿文件：盘上存在、DB 无元数据（execute_command 绕过写入路径的产物）
//   - 幽灵元数据：DB 有记录、盘上文件已消失（连续 3 轮缺失联动 updater → SoftDelete）
//   - 重复 checksum：内容相同的多份文件（磁盘浪费线索）
// 报告是 agent 决策的情报源，不是行动指令。

package manager

import (
	"context"
	"errors"
)

// errNoAudit 域内通用未实现错误（骨架期）。
var errNoAudit = errors.New("manager: audit not implemented")

// AuditReport 一轮对账结果（情报，不是行动指令）。
type AuditReport struct {
	Orphans      []string // 盘上有、DB 无的路径
	Ghosts       []string // DB 有、盘上无的路径（含连续缺失轮次计数）
	DupChecksums []DupGroup
}

// DupGroup 同一 checksum 的多路径组。
type DupGroup struct {
	Checksum string
	Paths    []string
}

// Audit 执行一轮盘 vs DB 对账。
// TODO 实现批次：service.SafeList 盘上全集 vs repo 元数据全集 → 三类差异归组
// → 幽灵的连续缺失计数（repo 侧补存续状态）。
func (m *Manager) Audit(ctx context.Context) (*AuditReport, error) {
	return nil, errNoAudit
}
