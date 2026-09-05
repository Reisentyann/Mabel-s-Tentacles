# Mabel-s-Tentacles

> 梅贝尔之触 —— 她会帮你管理文件。
>
> A lightweight MCP file manager for AI agents, written in Go — deterministic file describer, field indexer and lifecycle manager.

<p align="center">
  <img src="docs/images/mabel.png" alt="梅贝尔" width="410">
</p>
<p align="center">—— 这是梅贝尔</p>

## 简介

这是一个用 Go 开发的 MCP 服务器项目，用于管理、创造、分类文件。聊天机器人风靡全球，铃仙意识到让机器人来管理文件是非常有必要的。但传统 agent 直接管理文件，面临着易出错、没有固定流程、文件一多就找不到的风险。

基于此，我制作了一个面向 agent 的 MCP 文件管理器：agent 只管好好聊天，文件方面的事情就交给纯粹的理性分析吧。

至于为什么选 Go——因为 Go 的体感相当优雅。

技术栈：Go（单体后端，同进程提供 MCP 与 HTTP API）· Vue 3 · PostgreSQL · Docker Compose。

## 功能

**MCP 工具**（agent 调用）：

| 工具 | 功能 |
|---|---|
| `write_file` / `read_file` | 创造文件（写入自动产出确定性描述事实）/ 读取文件 |
| `modify_data_file` | 追加或覆写已有文件，元数据自动刷新 |
| `list_data_files` | 分页列出文件与简略元数据，可按路径关键词过滤，标出缺描述的文件 |
| `describe_file` | 写描述、标签与语义字段（支持追加模式；llm 字段受前缀闸门保护） |
| `analyze_file` | 手动重分析（T3）：重新跑描述引擎并返回本次事实产出，绕口直改后的对账入口 |
| `copy_file` | 复制文件（内容 + 元数据一起搬） |
| `execute_command` / `get_results` | 执行 Shell 命令（60s 超时）/ 查询历史执行结果 |

**Web 前端**（人类使用）：

| 功能 | 说明 |
|---|---|
| 文件区浏览 | 目录树与分页列表 |
| 搜索 | 关键词 / 标签 / 文件类型 / 分区过滤（含游戏室 `game/` 分区） |
| 描述管理 | 查看、编辑描述与标签 |
| 下载 | 网页下载单文件与打包下载 |
| 仪表盘 | 命令执行记录 |

**开发中与规划**：

| 功能 | 状态 |
|---|---|
| 文件内容解析并分类 | 描述引擎 v2 已上线（cod-text 41 事实字段 + 家族版本号）；语义分类待 llm 轨 |
| 字段索引机 | 已实现模块级（`indexer-go/` 纯库：三型桶 + `Query`/`Update`/`Rebuild` + 并发安全；条件 → 文件 uuid 集合，每字段独立索引桶，uuid 是组件间货币）。接线（启动 Rebuild + 检索接入）随索引机批次落地；**喂食钩子已预埋**（T1/T3 写路径 Upsert 后 diff 喂食的 sink 接口，装配层注入即点亮）；SQL 检索保留兜底 |
| 文件管理机 | 规划中（文件生命周期编排层：显式移动 + 谱系边、短期下载票据、盘库对账；DB 交互归 repo 层；自动只巡检报告，动手走 `move_file`）。**updater 域已实现**（T2/T3，见下） |
| 存量文件回填 | **已实现**：T3 手动入口（MCP `analyze_file` / HTTP `POST /api/files/analyze`）+ T2 一轮批量回填（`POST /api/files/backfill`；启动后台轮询 `describe.backfill` 配置默认关）+ IsStale 四条件（缺 ver / 版本落后 / checksum 漂 / mtime 新）+ 幽灵 3 轮软删 |
| 文件谱系图 | 规划中（`copied_from` 列与命令执行记录已是现成的边，补一张 lineage 表即可成 DAG） |
| 文件删除入口（软删除） | 部分实现（T2 幽灵 3 轮自动软删已接 `SoftDeleteMetadata`；agent 显式删除入口待文件管理机批次） |
| 游戏室分区 | 命名空间已预留（`game/` 路径自动归档） |
| 真随机机 | 规划中（未来功能，职责形态待定） |
| 机器人分段回复 | 已禁用退役 |

还有更多的功能。

## 快速开始

### 生产部署（Docker Compose）

```bash
git clone git@github.com:Reisentyann/Mabel-s-Tentacles.git
cd Mabel-s-Tentacles
docker compose up -d --build
```

- Web 前端：`http://<服务器IP>:18080/`（数据库等其余端口均只绑定本机，不出公网）
- MCP 端点：`http://127.0.0.1:18001/sse`
- 默认管理员：`admin / admin123`（生产环境务必修改，方法见部署指南）

### 本地开发（Windows / PowerShell 7）

```powershell
# 1. 先把数据库拉起来（只跑 postgres 一个容器即可）
docker compose up -d postgres

# 2. 一键编译后端(8080) + 启动前端 dev(5173)
./start-dev.ps1
```

AstrBot 等 MCP 客户端的接入配置，见[部署指南](部署指南.md)第 5 节。

## 文档

- [部署指南](部署指南.md) —— 环境准备、Docker Compose 部署、安全组配置、MCP 客户端（AstrBot 等）接入方法、1Panel 反代挂载子域名
- [架构设计](docs/架构设计.md) —— 组件职责与约定：管理机（位置与谱系唯一知情者）、索引机（条件→uuid 纯查询）、describer（字节进事实出）、组件间货币与依赖方向铁律

## 许可证

[PolyForm Noncommercial 1.0.0](https://polyformproject.org/licenses/noncommercial/1.0.0)，全文见 [LICENSE](LICENSE)。

- ✅ 允许：个人学习、研究、爱好项目、教育与公益等**非商业**用途下使用、修改、再分发（须保留 LICENSE 与版权声明）
- ❌ 禁止：任何**商业用途**（销售、商业服务、公司经营性使用等）
- ❌ 禁止：去除版权声明后的搬运分发

商用授权及其他许可事宜请联系作者。
