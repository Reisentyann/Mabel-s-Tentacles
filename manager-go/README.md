# manager

文件生命周期库，由后端装配层注入元数据存储、索引更新钩子、MIME 推导和下载配置。
导入路径为 `github.com/Reisentyann/Mabel-s-Tentacles/manager-go`，包名为 `manager`。

## 组织原则

采用单包、按职责分文件的 Go 库布局。文件操作共享 `Manager` 的存储、缓存和互斥锁，
内部辅助函数保持未导出。新增子包应具备独立职责和单向依赖；文件数量本身不构成拆包理由。
这是可导入的库，因此入口使用 `New`，无需 `cmd/`、`main.go` 或额外的 `pkg/` 层。

| 文件 | 职责 |
| --- | --- |
| `doc.go`、`manager.go` | 包约定、Manager 状态、依赖注入与构造 |
| `store.go` | 元数据读写载荷和必需存储契约 |
| `errors.go` | 公共哨兵错误，通过 `errors.Is` 判定 |
| `placement.go` | UUID 物理位置派生、逻辑路径和名称校验 |
| `intake.go`、`intake_recovery.go` | 新文件写入、源文件导入、发布及中断恢复 |
| `modify.go`、`replace_*.go` | 追加与覆盖、平台相关的原子替换 |
| `move.go`、`move_recovery.go` | 路径/UUID 移动、物理发布与恢复 |
| `fetch.go`、`buffer.go` | 定位、限读、流式读取与 LRU 内容缓存 |
| `directories.go` | 目录查询和创建、同名文件写入/导入/复制 |
| `directory_context.go` | 向占位事务传递目录关联和复制来源 |
| `directory_import.go`、`directory_export.go` | 本地目录递归导入、授权 ZIP 导出 |
| `tree.go` | 由元数据构建逻辑目录树和路径列表 |
| `updater.go` | 手动分析、批量回填和缺失状态检查 |
| `audit.go`、`lineage.go`、`download.go` | 盘库对账、谱系查询、下载票据 |

可选存储接口放在使用它的职责文件中，例如 `DirectoryStore`、`UUIDMoveStore`、
`MoveRecoveryStore`。SQL 和事务实现由后端 repo 提供，授权和事件投递由调用方负责。
`DirectoryIntake` 和复制来源上下文供 repo 的占位事务读取，不作为权限凭证。

## 运行约定

- 启动依次执行 `RecoverPendingMoves`、`RecoverPendingIntakes`，再接收请求与开始扫描。
- UUID 是对象身份；逻辑键用于存储索引；目录显示名称允许重复。
- `New` 的 `dataDir` 决定物理存储根，代码文件位置不参与数据路径解析。
- 导入源路径由调用方传入；相对路径仍相对于进程工作目录。需要固定位置时传绝对路径。
- 单实例的修改、移动和打开阶段由同一互斥锁协调。使用后不可复制 Manager。
- `Open` 返回的流由调用方关闭。导出失败时调用方应丢弃不完整 ZIP。

## 验证

在 `manager-go` 目录运行：

```powershell
go test ./... -count=1
go vet ./...
```

测试与实现同包目录存放。公共行为主要使用外部测试包 `manager_test`，缓存细节使用
包内测试。既有 `intake_test.go` 覆盖写入、修改、移动和逻辑树的联动；
`directories_test.go` 覆盖目录导入/导出、同名 UUID 操作和请求重放；恢复测试另见
`move_recovery_test.go`。重构涉及接口时还应运行后端与独立 `core` 模块的回归。

本次结构调整均在原模块目录内完成，导入路径、模块替换路径和启动工作目录保持一致。
物理数据继续由 `dataDir` 解析，导入来源与 ZIP 输出仍由调用方提供。

## 显式归档 MVP

`PlanCleanup` 只读扫描，`ApplyCleanup` 执行前重新核对状态。调用方传入已经存在的
绝对归档根，例如项目的 `旧内容` 目录；归档根与数据根不得互相包含。

```go
plan, err := mgr.PlanCleanup(ctx, manager.CleanupOptions{
    ArchiveRoot: archiveRoot,
})
// 检查 err 和 plan.Candidates；执行由调用方显式触发。
report, err := mgr.ApplyCleanup(ctx, plan)
```

默认保留期为 24 小时，每批最多 100 项（可设为 1–1000）。当前仅归档已完成发布、
与当前正式文件为同一文件身份的 `.partial`。未知孤儿、修改暂存、旧移动路径、
失败入库内容仅报告，保留可追溯元数据与恢复证据。硬链接归档不代表释放内容空间。

归档位置为 `旧内容/管理机清理/YYYY-MM-DD/<批次ID>/`，日期取计划创建时的本地日期。
批次保存 `manifest.json` 和同步追加的 `results.jsonl`；源相对路径保留在
`data/<源相对路径>.archive/content` 中。每项使用独占容器防止覆盖已有归档。
同一计划可以重复执行；已归档源会跳过，批次清单不一致会拒绝。
若进程在移动后、结果日志写入前中断，可用清单及目标内容核对实际结果。
若容器创建后移动失败，源仍保留，重新生成计划取得新批次后重试。

执行会等待单实例在途入库结束，并与修改和移动串行；等待锁期间不能即时响应取消。
数据库或状态查询失败立即停止。跨卷移动不回退复制，失败保留源文件。
这一机制不支持多实例并发维护或外部程序同时改盘，尚未接入 HTTP/MCP 或定时任务。
