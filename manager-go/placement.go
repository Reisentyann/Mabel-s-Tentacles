// 文件：manager-go/placement.go —— 位置域：路径解析（uuid → 逻辑路径；移动归 intake）
// 修改：2026-09-11（日期由 fresh-header.ps1 刷新）

// placement 域职责：文件在哪。
// 其他组件（索引机消费者、HTTP、MCP 工具）凭 uuid 问路径——只有这里回答。
// 移动是显式操作，正主在 intake 域（MCP move_file）：物理位随派生规则 +
// 元数据唯一键迁移 + 谱系边，绝无后台自动归档（agent 的路径预期不容破坏）。

package manager

import (
	"context"

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

// Move 显式移动：正主落地 intake.go 的 Manager.Move——物理随机化后
// "移动 = 逻辑键改"（uuid 不变则物理位由派生规则决定，ext 不变连盘都
// 不用碰）。本域职责收窄为路径解析（resolve / Resolve），移动/入库归 intake。
