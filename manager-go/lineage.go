// 文件：manager-go/lineage.go —— 谱系域：文件之间的关系（copied_from 现成边 + lineage 表将来）
// 修改：2026-09-03（日期由 fresh-header.ps1 刷新）

// lineage 域职责：文件之间的关系。现成边是 file_metadata.copied_from
// （copy_file 产生）与命令执行记录；将来补一张 lineage 表即可成 DAG
// （README 规划：文件谱系图）。其他组件问"这文件从哪来/有什么亲戚"，
// 只有这里回答。

package manager

import (
	"context"
	"errors"
)

// errNoLineage 域内通用未实现错误（骨架期）。
var errNoLineage = errors.New("manager: lineage not implemented")

// Relation 一条谱系边。
type Relation struct {
	UUID string // 相关文件
	Kind string // copied（复制来源）/ moved（移动前身份）/ derived（将来：命令产出）
}

// Related 查询文件的谱系邻居（来源 + 衍生）。
// TODO 实现批次：copied_from 列 + lineage 表（文件谱系图待办落地时建）。
func (m *Manager) Related(ctx context.Context, uuid string) ([]Relation, error) {
	return nil, errNoLineage
}
