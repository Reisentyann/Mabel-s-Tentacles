// 文件：manager-go/lineage.go —— 谱系域：文件之间的关系（copied_from 现成边 + lineage 表将来）
// 修改：2026-09-23（日期由 fresh-header.ps1 刷新）

// lineage 域职责：文件之间的关系。现成边是 file_metadata.copied_from
// （copy_file 产生）与命令执行记录；将来补一张 lineage 表即可成 DAG
// （README 规划：文件谱系图）。其他组件问"这文件从哪来/有什么亲戚"，
// 只有这里回答。

package manager

import (
	"context"
)

// Relation 一条谱系边。
type Relation struct {
	UUID string // 相关文件
	Kind string // copied（复制来源）/ moved（移动前身份）/ derived（将来：命令产出）
}

// Related 查询文件的谱系邻居（复制来源、复制衍生与移动身份）。
// 当前复用 file_metadata 的 copied_from / moved_from 列；将来若需要
// 命令产出等多跳关系，再补 lineage 表扩展关系种类。
func (m *Manager) Related(ctx context.Context, uuid string) ([]Relation, error) {
	ref, err := m.store.GetMetaByUUID(ctx, uuid)
	if err != nil {
		return nil, err
	}
	if ref == nil {
		return nil, ErrNotFound
	}
	row, err := m.store.GetMeta(ctx, ref.Path)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, ErrNotFound
	}

	relations := make([]Relation, 0, 3)
	if row.CopiedFrom != "" {
		source, err := m.store.GetMeta(ctx, row.CopiedFrom)
		if err != nil {
			return nil, err
		}
		if source != nil {
			relations = append(relations, Relation{UUID: source.UUID, Kind: "copied"})
		}
	}
	if row.MovedFrom != "" {
		relations = append(relations, Relation{UUID: uuid, Kind: "moved"})
	}
	children, err := m.store.ReverseCopiedFrom(ctx, row.Path)
	if err != nil {
		return nil, err
	}
	for _, child := range children {
		if child.UUID != "" && child.UUID != uuid {
			relations = append(relations, Relation{UUID: child.UUID, Kind: "copied"})
		}
	}
	return relations, nil
}
