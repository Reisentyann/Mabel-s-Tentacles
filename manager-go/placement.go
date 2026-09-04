// 文件：manager-go/placement.go —— 位置域：路径解析与显式移动（uuid→path 的唯一入口）
// 修改：2026-09-03（日期由 fresh-header.ps1 刷新）

// placement 域职责：文件在哪。
// 其他组件（索引机消费者、HTTP、MCP 工具）凭 uuid 问路径——只有这里回答。
// 移动是显式操作（MCP move_file）：物理改名 + 元数据唯一键迁移 + 谱系边，
// 绝无后台自动归档（agent 的路径预期不容破坏）。

package manager

import (
	"context"
	"errors"
)

// errNotPlaced 域内通用未实现错误（骨架期）。
var errNotPlaced = errors.New("manager: placement not implemented")

// Resolve 凭 uuid 解析文件路径——唯一知情者的核心问答。
// 其他组件不查 repo 不摸文件系统，路径问题只有这个入口。
func (m *Manager) Resolve(ctx context.Context, uuid string) (string, error) {
	return "", errNotPlaced // TODO：repo 按 uuid 取 file_path（repo 侧补 GetMetadataByUUID）
}

// Move 显式移动：物理改名 + file_metadata 唯一键迁移 + 谱系边（lineage 域联动）。
// TODO 实现批次：防穿越校验（service.ResolvePath）→ rename → 元数据迁移
// （含 copied_from/lineage 补边）→ 喂食索引机 Update。
func (m *Manager) Move(ctx context.Context, source, target string) error {
	return errNotPlaced
}
