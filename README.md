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
| `write_file` / `read_file` | 创造文件（写入自动产出确定性描述事实；回执含 uuid 与下载地址）/ 读取文件 |
| `modify_data_file` | 追加或覆写已有文件，元数据自动刷新 |
| `move_file` | 逻辑键改（重命名/移动）：uuid 与描述不变，ext 不变 = 纯数据库键改零盘操作 |
| `list_data_files` | 分页列出文件与简略元数据，可按路径关键词过滤，标出缺描述的文件 |
| `describe_file` | 写描述、标签与语义字段（支持追加模式；llm 字段受前缀闸门保护） |
| `analyze_file` | 手动重分析（T3）：重新跑描述引擎并返回本次事实产出，绕口直改后的对账入口 |
| `copy_file` | 复制文件（内容 + 元数据一起搬，副本归操作者并记 `copied_from` 谱系） |
| `execute_command` / `get_results` | 执行 Shell 命令（60s 超时）/ 查询历史执行结果 |

**寻址与隔离**（agent 视角）：agent 只见逻辑键（组织语言），物理存储位由 uuid 纯派生（`<uuid前2位>/<uuid><ext>`）——盘面只有随机名，存储位置不可猜。多用户 owner 键空间：无前缀 = 自己的空间（工具层自动拼接 `~<主体名>/`），显式 `~用户名/` = 跨用户只读寻址；两个用户写同一个逻辑地址永不碰撞。

**Web 前端**（人类使用，单页管理台）：

| 功能 | 说明 |
|---|---|
| 逻辑树 | 左侧目录树（owner 键空间根），点击目录过滤文件表 |
| 文件表与全局搜索 | 路径 / 类型 / 大小 / 更新时间；关键词 / 标签 / 描述搜索 |
| 文件详情抽屉 | 基本信息 + 描述标签 + **cod/llm 事实字段按家族分组**（basic/text/code/image/llm）+ 谱系（复制自/移动自） |
| 文件操作 | 下载、移动（与 MCP 工具同一正主）、复制、重分析 |
| 统计条 | 文件数 / 总量 / 目录数 / 类型分布 |

暗色书房主题（深夜海面底 + 触手洋红品牌色），Element Plus 按需引入（不整库打包）。管理页仅限管理员账号登录（.env 种子账号），不开放注册——本系统对接自有 agent，管理面是自用工具。

**已实现**：

| 功能 | 说明 |
|---|---|
| 描述引擎 v2 | 四插件（basic/text/code/image）90+ 事实字段，字节进事实出；cod-text 41 字段 + 家族版本号机制 |
| 字段索引机 | 三型桶纯库 + 装配完成：启动 Rebuild、写路径/描述/复制/T2/T3 全喂食、检索门面（索引优先 → SQL 降级）、按 uuid 批量取件 |
| 管理机 fetch 域 | 取件三哨兵（NotFound/Deleted/Ghost）+ buffer LRU 快照缓存 + 大文件旁路直流 |
| 管理机 updater 域 | T2 一轮批量回填 + T3 手动重分析 + IsStale 四条件（缺 ver / 版本落后 / checksum 漂 / mtime 新）+ 幽灵 3 轮软删 |
| 管理机 intake 域 | 入库唯一口：Reserve 占位行幂等拿 uuid → uuid 派生位落盘（物理随机化，盘面不可猜）；Move 纯键改 + ext 变化随派生规则 rename（失败回滚） |
| 多用户 owner 键空间 | `~<主体名>/` 前缀隔离（保留段不可伪装）；无前缀 = 自己空间自动拼接，`~用户名/` = 跨用户只读寻址；同名逻辑地址永不碰撞 |
| 两级可见性模型 | public（默认，协作共享）/ private（仅主人与管家）；防线在门口（master key / external key 接入校验），接入后不做授权矩阵 |
| 谱系基础 | `copied_from` / `moved_from` 随元数据自然带出，查询零新端点 |
| 幽灵软删除 | T2 连续 3 轮盘上缺失自动软删（打标记不物理删，可追溯） |
| 单页管理台前端 | 暗色书房主题 + Element Plus 按需；逻辑树 / 文件表 / 详情抽屉（cod·llm 分组）/ 谱系 / 搜索 / 统计 / 移动复制下载重分析 |
| L1-L4 测试体系 | 单元 / 断言表 / 集成 / 端到端（e2e + 可编程场景客户端：书房档案全流程含多用户同名碰撞） |

**规划中**：

| 功能 | 说明 |
|---|---|
| 语义分类 | llm 轨字段（cod 事实已就位，语义层待模型轨接入） |
| 对话标识 | MCP 工具可选 `conversation` 参数（如 `qq:123456`）：带它写文件归 `~qq:123456/` 键空间——master key 一把接入 + 归属打标，零注册零密码 |
| 谱系图 | `copied_from` / `moved_from` / 执行记录已成边，DAG 图谱化展示 |
| agent 显式删除入口 | 软删除的 agent 侧入口（自动软删已实现，显式入口待做） |
| 短期下载票据 | 限时失效的下载凭证（当前为 ACCESS_TOKEN 静态 token 口径） |
| 盘库对账巡检 | 后台自动巡检报告（当前为 T2 主动触发 + 启动轮询） |
| 游戏室分区落地 | 命名空间已预留（`game/` 路径自动归档），功能待做 |
| 真随机机 | 未来功能，职责形态待定 |

还有更多的功能。

## 快速开始

### 生产部署（单二进制 + systemd）

```bash
# 服务器上（linux/amd64，纯 Go 无 CGO——本地交叉编译即可）
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o mabel-server ./cmd/server

mkdir -p /opt/mabel/{data,logs,web}
# 前端：pnpm build 后把 frontend/dist/* 放入 /opt/mabel/web（服务端自带 SPA 托管）
# 配置 /opt/mabel/.env（管理员账号、强随机的 SECRET_KEY 与 MCP_API_KEY、
# DATABASE_URL 指向本机 postgres、可选 ACCESS_TOKEN 供 agent 分享直下链接），
# 填法与 systemd 单元模板见部署指南第 5 节
```

- MCP 端点：`http://<内网>:18001/sse`（公网经反代/域名，SSE 需 `proxy_buffering off`）
- 管理页：同端口 `/`（仅管理员账号，建议只在内网或隧道访问）
- 本地开发（Windows / PowerShell 7）：

```powershell
# 1. 先把数据库拉起来（只跑 postgres 一个容器即可）
docker compose up -d postgres

# 2. 一键编译后端(8080) + 启动前端 dev(5173)
./start-dev.ps1
```

AstrBot 等 MCP 客户端的接入配置，见[部署指南](部署指南.md)第 5 节。

## 文档

- [部署指南](部署指南.md) —— 环境准备、部署形态、安全组配置、MCP 客户端（AstrBot 等）接入方法、1Panel 反代挂载子域名
- [架构设计](docs/架构设计.md) —— 组件职责与约定：管理机（位置与谱系唯一知情者）、索引机（条件→uuid 纯查询）、describer（字节进事实出）、组件间货币与依赖方向铁律
- [common 共享层](docs/common共享层.md) —— 跨模块纯函数复用清单与准入规则：防穿越 / 盘读三件套 / 指针转换 / ClientIP；写纯函数前先查它，避免重复造轮子
- [测试规则](test/测试规则.md) —— L1 单元 / L2 断言表 / L3 集成 / L4 端到端分层与回归清单；场景测试客户端见 `test/tools/scenario/`

## 许可证

[PolyForm Noncommercial 1.0.0](https://polyformproject.org/licenses/noncommercial/1.0.0)，全文见 [LICENSE](LICENSE)。

- ✅ 允许：个人学习、研究、爱好项目、教育与公益等**非商业**用途下使用、修改、再分发（须保留 LICENSE 与版权声明）
- ❌ 禁止：任何**商业用途**（销售、商业服务、公司经营性使用等）
- ❌ 禁止：去除版权声明后的搬运分发

商用授权及其他许可事宜请联系作者。
