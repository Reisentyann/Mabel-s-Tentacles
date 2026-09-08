// 文件：mcp-server-go/internal/service/files.go —— 路径归一薄层（ResolvePath）；盘 IO 已全部收归管理机（intake/fetch 域，2026-09-08）
// 修改：2026-09-08（日期由 fresh-header.ps1 刷新）

// 历史注记（2026-09-08 文件 IO 主权收归管理机）：本文件原承载
// SafeWrite / SafeModify / SafeRead / SafeList / ListTree 等盘操作——
// 架构第 1 节"组件不越过管理机摸文件系统"自此彻底贯彻：
//   - 写入/修改 → manager.Write / manager.Modify（intake 域：逻辑键 +
//     uuid 派生随机物理路径，agent 不接触物理布局）
//   - 读取 → manager.ReadByLogic / OpenByLogic（fetch 域 + buffer）
//   - 下载 → manager.StreamByLogic / StoragePathOf 派生（直流，不经缓存）
//   - 枚举/树 → manager.LogicPaths / LogicTree（逻辑视图，盘上无树可看）
//
// 退役实现可在 git 历史（本提交的父提交）找到。
package service

import (
	"github.com/Reisentyann/Mabel-s-Tentacles/common"
)

// ResolvePath 校验并解析 baseDir 内的相对路径为绝对路径（防穿越 /
// symlink 逃逸，common.ResolveWithin 承担）。现状唯一调用方：编排机
// 执行器把 uuid 派生的存储相对路径归一为绝对路径。
func ResolvePath(baseDir, filePath string) (string, error) {
	return common.ResolveWithin(baseDir, filePath)
}
