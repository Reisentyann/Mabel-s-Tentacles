# JM 下载维护方案

> 状态：后台任务部分仍为方案草案；同步下载完成后的归档入库接线已实现（2026-09-23）。
> 范围：`chaos-go/jmcomic`、MCP 混沌机工具面、Mabel 管理机入库流程。
> 目标：JM 下载超过聊天工具等待时限时仍可靠完成，并支持日志追踪、断点续传、归档复用。

## 1. 已确认的问题

本次 `288449` 下载暴露出当前链路的四个问题：

1. AstrBot 的单次工具等待上限为 120 秒，完整本子下载超过该时间后，AstrBot 返回超时。
2. Mabel 的 JM Python 子进程仍同步等待，当前 Go 侧下载等待上限为 15 分钟；等待结束后进程被杀，MCP 没有后台任务语义。
3. JMComic 默认日志写到 Python 子进程 stdout，Go 只收集内存中的 stdout/stderr，没有为每个任务保存独立日志。
4. JMComic 缓存目前主要按 `os.path.exists` 判断；中断留下的 0 字节图片可能被当作已完成文件，存在缺图后仍打包的风险。

本次现场证据：

- `288449` 在 `/opt/mabel/欧姆社学习漫画-物理·漫画相对论/` 产生了大量图片。
- 至少一个图片文件为 0 字节。
- AstrBot 在 120 秒时超时。
- Mabel 在 15 分钟时记录 `signal: killed`，没有产生 ZIP，也没有入库。

## 2. 目标行为

### 2.1 首次调用

MCP 调用 `jm_comic` 下载时：

1. 先按车号、目标类型、格式和配置计算稳定任务键。
2. 如果 Mabel 已有对应的完整归档，直接返回已有短链，不访问 JM。
3. 如果同一任务已有后台任务在运行，直接返回“下载中”、任务 ID、日志路径和当前进度信息，不重复启动。
4. 如果没有完成归档，启动后台下载任务。
5. MCP 在短等待窗口内等待任务完成；窗口内完成则直接返回短链。
6. 超过短等待窗口后，返回明确的后台状态，不返回模糊的成功或失败：

```json
{
  "success": true,
  "status": "downloading",
  "jm_id": "288449",
  "job_id": "jm-288449-...",
  "message": "下载已转入后台，完成后可再次调用查询或下载同一车号复用归档",
  "log_path": "logs/jmcomic/jm-288449-....log"
}
```

### 2.2 后续调用

再次调用同一车号时按以下顺序处理：

1. 已有完整归档并已进入 Mabel 元数据：返回原有下载短链。
2. 有运行中任务：返回运行状态、已耗时、日志路径和最近进度。
3. 有失败任务但存在可续传图片：复用原临时目录，从缺失或无效图片继续。
4. 没有可复用产物：创建新任务。

后续可增加显式动作：

- `action=view`：查询 JM 元数据。
- `action=download`：创建或复用下载任务。
- `action=status`：查询任务状态和最近日志摘要。
- `action=retry`：仅在任务失败时强制重新调度，不删除已有文件。

## 3. 任务状态模型

任务状态建议使用以下有限集合：

| 状态 | 含义 | 是否允许再次下载 |
|---|---|---|
| `queued` | 已创建，等待执行 | 否，复用任务 |
| `downloading` | Python 下载器运行中 | 否，复用任务 |
| `packaging` | 图片齐全，正在生成 ZIP | 否，复用任务 |
| `ingesting` | ZIP 已完成，正在移入 Mabel 并描述 | 否，复用任务 |
| `completed` | 已入库并有下载短链 | 否，直接复用 |
| `failed` | 本轮失败 | 有断点时续传，无断点时新任务 |
| `cancelled` | 明确停止或服务关闭导致的任务结束 | 有断点时续传 |
| `stale` | 进程已消失但任务仍未完成 | 复用断点重新调度 |

任务状态不能只存在 Go 内存中。至少要写入本地任务 JSON 或数据库，否则服务重启后无法判断上次是否已经完成。

## 4. 持久化目录布局

建议所有 JM 运行产物集中在 Mabel 数据目录外的专用工作目录，完成归档再进入 Mabel 管理机：

```text
/opt/mabel/jmcomic/
├── jobs/
│   └── <job-id>/
│       ├── job.json          # 任务状态、车号、目标类型、格式、时间、错误
│       ├── download.log      # JMComic stdout/stderr 与结构化阶段日志
│       ├── images/            # JMComic 断点续传目录
│       ├── archive.partial    # 正在生成时使用，避免被当作完成 ZIP
│       └── archive.zip        # 完整归档完成后才出现
└── index.json                 # 可选：任务索引，启动时也可扫描 jobs/*/job.json 重建
```

完成入库后，`archive.zip` 交给 `manager.Manager.ImportFile(logicPath, sourcePath)`。
管理机内部负责 Reserve、UUID 派生物理路径和文件转移；混沌机/MCP 装配层不拼接
Mabel `data/<uuid前2位>/...`，也不直接写元数据。整个过程不把整份归档读入内存。

禁止用“删除临时目录”作为成功流程的一部分。临时图片目录保留为断点与审计证据；后续清理只能作为独立的人工确认维护动作，并移动到项目根 `旧内容/` 或服务器专用归档目录。

## 5. 日志方案

### 5.1 每任务独立日志

每个任务创建独立日志文件：

```text
/opt/mabel/jmcomic/jobs/<job-id>/download.log
```

日志同时写入 Mabel 主日志，主日志只记录摘要，避免把每张图片的详细日志塞入服务主日志。

### 5.2 必须记录的字段

每条结构化摘要至少包含：

- `job_id`
- `jm_id`
- `target_type`
- `format`
- `status`
- `phase`
- `proxy_enabled`
- `started_at`
- `duration`
- `images_total`
- `images_reused`
- `images_downloaded`
- `images_invalid`
- `archive_path`
- `archive_bytes`
- `logic_path`
- `storage_uuid`
- `download_url`
- `error`

失败必须记录错误和当前阶段。超时必须明确区分：

- `client_timeout`: AstrBot 等待超时，但后台任务仍在运行；
- `download_timeout`: Python 下载任务达到后端任务时限并被停止；
- `request_timeout`: 某个 JM HTTP 请求超时；
- `archive_timeout`: 下载完成后打包超时。

### 5.3 Python stdout/stderr

Go 启动 Python 时：

1. stdout 和 stderr 直接追加到任务日志文件，不只放在内存 `bytes.Buffer`。
2. 保留 `##JMRESULT##` 结果标记，最后一行仍返回结构化结果。
3. Python 日志使用 UTF-8。
4. MCP 回执只返回日志路径和最近摘要，不返回整份日志。

## 6. 断点续传与文件有效性

JMComic 本身的 `download.cache=true` 能复用已存在图片，但当前存在“0 字节文件也算存在”的缺陷，必须在接线层补强：

1. 图片缓存命中条件从“路径存在”提升为“普通文件且大小大于 0”。
2. 对无法打开、大小为 0 的文件视为无效，允许 JMComic 重新下载。
3. 下载器成功写入后才计入 `images_downloaded`。
4. ZIP 只能从全部预期图片有效的目录生成。
5. ZIP 先写为 `archive.partial`，关闭且校验大小大于 0 后再改名为 `archive.zip`。
6. Mabel 只接收 `archive.zip`，不接收 `archive.partial`。

如果上游 JMComic 不能提供原子图片写入，则由维护封装层在再次运行前扫描并标记 0 字节文件为无效；不删除文件，必要时改名为 `.partial`，避免下次误命中缓存。

## 7. 归档复用规则

任务复用键建议为：

```text
jm_id + target_type + format + option_fingerprint
```

默认情况下，`option_fingerprint` 至少包含：

- 目标类型（album/photo）
- 输出格式（zip/raw）
- 图片后缀与解码选项
- 代理不纳入复用键，代理只是传输路径

完成归档的最终复用依据不是临时目录，而是 Mabel 元数据中的逻辑路径和有效物理文件：

```text
comics/[JM<id>]<标题>.zip
```

同车号标题发生变化时，不覆盖旧归档；应生成新逻辑路径或进入显式版本策略，避免误覆盖。

## 8. 超时策略

建议拆成三层：

1. **MCP 短等待窗口**：例如 20～30 秒。窗口内完成就直接返回，超过即返回后台状态。
2. **Python 单请求超时**：由 JMComic/curl-cffi 控制，避免某个图片请求无限等待。
3. **后台任务总时限**：例如 30 分钟或按配置设置。达到上限后记录 `download_timeout`，保留断点，不杀掉已完成图片。

AstrBot 的 `tool_call_timeout` 可以提高到 600 秒作为缓冲，但不能把它当作可靠下载机制。真正的可靠机制必须是 MCP 快速回执 + 后台任务。

## 9. Mabel 接线顺序

下载任务完成后的唯一顺序：

1. 校验所有预期图片有效。
2. 生成 `archive.partial`。
3. 校验并改名为 `archive.zip`。
4. 调用 `manager.Manager.ImportFile(logicPath, archivePath)`；该接口内部完成
   `ReserveMeta`、UUID 派生和同卷 rename/跨卷流式导入。
5. 导入成功后，文件位置由管理机接管，调用方不再接触物理路径。
6. 提交编排机 `KindWrite` 事件。
7. 等待或记录异步描述结果。
8. 签发短链。
9. 更新任务状态为 `completed`。

任何中间步骤失败，都不能把任务标记为完成，也不能把半成品暴露成下载链接。

## 10. 服务重启与恢复

Mabel 启动时扫描 `jobs/*/job.json`：

- `completed`：检查 Mabel 元数据和物理文件，存在则保留复用记录。
- `downloading`：检查 PID 是否仍存在；不存在则改为 `stale`，保留断点。
- `packaging`：若只有 `archive.partial`，重新校验图片后继续打包。
- `ingesting`：检查逻辑路径和 UUID；已入库则补发短链，未入库则继续接线。
- `failed` / `stale`：不自动无限重试，启动时只标记可恢复，下一次 MCP 调用再续传。

## 11. 实现批次

### P1：先解决本次故障

- 为每个任务保存 stdout/stderr 日志。
- 修复 0 字节缓存命中。
- Python 下载达到 MCP 等待窗口后转后台，不被 MCP 超时杀死。
- 增加 `action=status`。
- 保留已有图片，失败后允许下次复用。

### P2：归档与复用

- `archive.partial` 原子归档。
- 完成归档按逻辑路径复用短链。
- 同一任务并发去重。
- 服务重启扫描任务状态。

### P3：运维与体验

- AstrBot `tool_call_timeout` 调整为 600 秒作为辅助缓冲。
- 管理端显示 JM 任务、阶段、进度、日志路径和失败原因。
- 增加人工重试/继续按钮。
- 增加日志与任务目录容量告警。

## 12. 验收标准

1. 下载一个 30 秒内完成的章节，MCP 直接返回短链。
2. 下载一个超过 120 秒的本子，MCP 在短窗口内返回 `downloading`，AstrBot 不报失败。
3. 下载中结束 Mabel 进程，重启后任务标记为 `stale`，再次调用能从已有图片继续。
4. 制造 0 字节图片，下一次不会命中缓存，会重新下载该图片。
5. 下载完成后只生成一个有效 ZIP，Mabel 元数据只有一个有效归档记录。
6. 重复调用已完成车号，不产生第二次 JM 请求，直接返回已有下载链接。
7. Python 错误、单图请求超时、整体任务超时和 MCP 客户端超时在日志中可区分。

## 13. 推荐架构调整：本地下载工作机

结合当前网络条件，推荐把“访问 JM”和“管理最终文件”拆开：

```text
AstrBot
   │ MCP：创建/查询任务
   ▼
云服务器 Mabel
   │ 返回 job_id；不在 MCP 请求内等待整本下载
   │
   │ 本地工作机轮询任务
   ▼
本地 JMWorker
   ├── 本地 JMComic Python
   ├── 本地代理 127.0.0.1:7890
   ├── 本地断点目录与任务日志
   └── 本地生成完整 ZIP
             │ SSH/SFTP 上传 archive.zip.part
             ▼
云服务器 Mabel staging
   ├── 校验大小与 SHA-256
   ├── .part → 完整归档原子改名
   ├── Manager 零拷贝移入 data 派生路径
   ├── 编排机描述、落库、喂索引
   └── 生成对外下载短链
```

### 13.1 为什么比服务器直接下载更合适

1. JM 的所有外网请求都在本地代理环境完成，服务器不再承担 JM 域名访问、连接重试和 CDN 稳定性。
2. MCP 请求不再绑定整个下载生命周期，AstrBot 的 120 秒工具超时不会杀掉或掩盖下载任务。
3. 本地 JMComic 工作目录天然保留图片缓存，下次任务可从已有图片继续。
4. ZIP 只在本地完整生成后上传，服务器接收的是单个归档文件，Mabel 继续负责最终存储、元数据和下载链接。
5. 本地日志可以完整保留 JM 的每章节、每图片、重试和失败信息，不会因为 MCP 连接断开而丢失。

### 13.2 任务流程

1. `jm_comic(action=download)` 只在 Mabel 中创建或复用任务，快速返回 `job_id`。
2. 本地 `JMWorker` 使用受限凭据轮询待处理任务。
3. Worker 以 `jm_id + target_type + format + option_fingerprint` 作为本地任务键。
4. Worker 检查本地完整 ZIP；已有完整 ZIP 时跳过 JM 下载，直接进入上传/复用流程。
5. 没有完整 ZIP 时调用 JMComic；已有图片目录则启用缓存继续下载。
6. 下载阶段只认“普通文件且大小大于 0”的图片缓存；0 字节或无法打开的文件视为未完成。
7. 所有图片有效后，先生成 `archive.zip.partial`，校验成功后改为 `archive.zip`。
8. Worker 通过 SSH/SFTP 将 ZIP 上传为服务器上的 `.part` 文件；上传完成后校验 SHA-256 和大小。
9. Mabel 以原子改名接收文件，调用现有 Manager/Orchestrator 入库流程。
10. 任务变为 `completed` 后，后续 MCP 调用只返回现有短链，不重新请求 JM。

### 13.3 上传方式

不建议在 MCP handler 中直接执行普通 `scp`：

- `scp` 默认不支持断点续传；大文件中断后重传成本高。
- MCP handler 等待上传仍可能触发 AstrBot 超时。
- 上传过程中不应让半文件出现在 Mabel 的正式数据目录。

建议使用专用 SSH/SFTP 上传通道：

- 上传目标固定为 `/opt/mabel/jmcomic/incoming/`；
- 临时文件使用 `<job-id>.zip.part`；
- 只允许写入 staging，不允许覆盖任意服务器路径；
- 上传完成后由 Mabel 内部导入逻辑校验并移动；
- SSH 使用专用受限账号和专用密钥，不继续扩大 root 密钥用途；
- 隧道、上传和任务接口都不对公网开放。

第一版可以使用现有 SSH 密钥完成上传验证，稳定后再收紧为专用账号/受限命令。

### 13.4 断点与复用边界

必须区分三种缓存：

1. **本地图片缓存**：用于 JMComic 断点续传，只能跳过有效图片。
2. **本地完整 ZIP**：用于避免再次下载和再次压缩。
3. **Mabel 完整归档**：最终权威缓存；存在有效元数据、物理文件和下载短链时直接复用。

任一层发现文件损坏，都不能直接返回成功：

- 图片层：回到 JMComic 重新获取对应图片；
- ZIP 层：重新打包，不重新下载有效图片；
- Mabel 层：重新上传/导入，不重新访问 JM。

### 13.5 本地工作机离线时的行为

本地电脑关闭、代理退出或 SSH 不在线时：

- Mabel 仍正常运行；
- MCP 调用只创建任务并返回 `queued`；
- 不把任务误报为失败；
- Worker 恢复后继续领取任务；
- 任务在服务器端保留，不丢失车号、日志和目标路径。

### 13.6 对现有方案的调整

本地工作机方案确定后，实施优先级调整为：

1. P1：任务表/任务 JSON、状态查询、独立日志、本地 Worker、图片有效性判断。
2. P2：本地 ZIP 复用、SFTP `.part` 上传、服务器校验与 Manager 导入。
3. P3：MCP 完成通知、管理页任务状态、受限 SSH 账号、日志保留策略。

云服务器端不再运行 JMComic 下载 Python 任务；云端只负责任务、接收、校验、入库和对外服务。
