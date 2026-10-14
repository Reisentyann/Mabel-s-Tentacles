# Mabel-s-Tentacles

> 梅贝尔之触 —— 她会帮你管理文件。
>
> A lightweight MCP file manager for AI agents, written in Go — deterministic file describer, field indexer, and lifecycle manager.

<p align="center">
  <img src="docs/images/mabel.png" alt="梅贝尔" width="410">
</p>
<p align="center">—— 这是梅贝尔</p>

## 简介

这是一个用 Go 开发的 **MCP（Model Context Protocol）服务器与文件资产管理平台**，专为聊天机器人（如 AstrBot 等 AI Agent）设计，用于安全、有序地创造、分类、检索与分享文件。

传统 Agent 直接操作宿主机文件系统时，往往面临路径混乱、易误覆写、缺乏结构化元数据、文件稍多便无法精准找回的困境。基于此，本项目构建了一套面向 Agent 的理性文件底座：**Agent 只管专心对话与创作，文件的存储隔离、特征提取、多维检索与生命周期维护全部交由系统完成。**

**核心设计（四机协同 + 编排中心）：**
- **描述机（`describer-go`）**：纯确定性特征提取引擎，无需 AI 参与即可从文本、代码、图像中提取 90+ 维度的量化字段（字符数、行数、标题结构、表格数量、中英占比、最长句、图片尺寸与色调等），确保结果毫秒级产出且绝对一致。
- **索引机（`indexer-go`）**：内存多维倒排与分桶索引引擎，支持布尔逻辑组合（AND / OR / NOT）、区间比较与分档模糊查询，并提供字段字典自省。
- **管理机（`manager-go`）**：执掌文件物理生命周期。采用逻辑路径与物理存储解耦设计（UUID 哈希分片落盘）、LRU 内存热缓存、重名防覆盖保护、目录树原子操作，以及 T1/T2/T3 持久维护任务与盘库对账恢复。
- **混沌机（`chaos-go`）**：独立娱乐与扩展模块，提供梅贝尔原作随机台词（带人设语气指导）、伪随机/量子真随机数生成，以及 JMComic 漫画查询下载并自动归档入库。
- **MCP 编排服务（`mcp-server-go`）**：同进程提供 MCP SSE 通道、RESTful 管理 API 与 `/d/{code}` 极简限时短链直下服务。

技术栈：**Go 1.26**（多模块工作区，单体进程交付）· **Vue 3 + Vite** · **PostgreSQL 16** · **Docker Compose**。

---

## 核心特性

- **逻辑与物理彻底解耦**：Agent 只需使用直观的逻辑路径（如 `小说/第一章.txt`），底层自动按 UUID 派生随机物理路径（`<uuid前2>/<uuid><ext>`）。即使直接查看服务器磁盘，也无法从盘面猜出文件真实名称与目录结构。
- **多用户键空间隔离**：每个用户拥有独立命名空间（`~用户名/路径`），同名文件互不冲突；支持 `public` / `group` / `private` 三级可见性矩阵与外部 Agent Key 绑定授权。
- **90+ 维度量化描述与两步精确检索**：文件写入或改动后自动触发分析，生成统一特征字段。Agent 可先通过 `list_index_fields` 获取字段字典与分档标准，再通过 `search_files` 组合复杂条件精准定位文件。
- **持久维护任务与单文件串行锁**：文件写入、修改、移动与后台 T2/T3 巡检共享逐文件串行锁与数据库持久维护队列（`metadata_maintenance_jobs`），崩溃重启后自动恢复未完任务，杜绝并发脏写与幽灵复活。
- **24h 极简下载短链**：写文件或归档完成后自动签发 6 位 Base62 限时短链（如 `https://your-domain/d/Fne07G`），支持中文文件名无乱码直下，方便机器人在群聊或私聊中直接把文件递给人类。
- **Web 管理台与实时审计**：配套 Vue 3 管理后台，支持目录树浏览、图片/代码在线预览、元数据与标签就地编辑、批量打包下载，以及 Agent MCP 工具调用流水实时看板。

---

## MCP 能力一览

### 1. MCP 工具（Tools）

| 类别 | 工具名 | 功能说明 |
|---|---|---|
| **文件读写** | `write_file` / `upload_file` | 新建文本文件或上传二进制/图片文件（重名拒绝覆盖，自动触发 T1 特征提取，返回 UUID 与 24h 下载短链） |
| | `read_file` | 按逻辑路径或 UUID 读取文件内容（命中热缓存免磨盘） |
| | `modify_file` / `modify_data_file` | 修改已有文件（支持末尾追加或整段覆写） |
| | `patch_file` | 局部精准替换：基于唯一上下文锚点替换指定片段（单次或全量），避免大文件全量重写 |
| **目录与生命周期** | `create_directory` / `create_child_directory` | 创建持久化逻辑目录（支持按路径或父目录 UUID 创建） |
| | `list_directory` / `write_directory_file` | 按目录 UUID 列出子项或在指定目录下新建文件 |
| | `move_file` / `copy_file` | 移动/重命名文件（纯元数据键迁移或同卷改后缀，UUID 不变）与复制文件（自动记录 `moved_from` / `copied_from` 溯源谱系） |
| | `delete_file` | 软删除文件至回收站，保留元数据与历史血统记录 |
| **元数据与检索** | `stat_file` / `get_file_info` | 查询单文件状态或全量 90+ 维描述字段与溯源信息 |
| | `list_files` / `list_data_files` | 分页列出文件列表，支持关键词过滤并标识未描述文件 |
| | `list_index_fields` | 获取实时可检索字段字典（含义、取值分布、数值范围与分档建议） |
| | `search_files` | 多维条件组合检索（支持 `eq` / `in` / `gt` / `lt` / `between` / `contains` / `exists` 及 `AND` / `OR` / `NOT` 嵌套） |
| | `describe_file` / `analyze_file` | 人工/Agent 补充语义描述与标签（`cod-*` 机器字段只读保护）；或强制重新扫描刷新特征 |
| | `audit_files` | 管理员只读盘库对账：检测孤儿文件、幽灵记录与重复 Checksum |
| **系统与混沌机** | `execute_command` / `get_command_history` | 管家特权：执行受控 Shell 命令（60s 超时截断）及查询历史执行记录 |
| | `mabel_quote` | 随机抽取 1–5 段梅贝尔原作台词，附带 `style_guidance` 人设语气指导 |
| | `pseudo_random` / `true_random` | 本地高速伪随机数生成 / 物理世界量子源真随机数生成（无降级凑数） |
| | `jm_comic` | 查询或下载 JMComic 本子，自动打包 ZIP 导入 Mabel 文件库并签发下载短链 |

### 2. MCP 资源（Resources）与提示词模板（Prompts）

- **Resources**：
  - `mabel://catalog/fields`：实时索引字段字典元数据，供模型自主规划高级查询。
  - `mabel://guide/search-contract`：精准/模糊/范围搜索规范契约与 JSON 构造指南。
  - `mabel://persona/system`：梅贝尔系统人设指引，动态注入 3 段原作台词作为语气基准。
- **Prompts**：
  - `organize_workspace`：工作区深度梳理与分类归档流程引导。
  - `audit_undescribed_files`：未描述/未分类文件专项审计与批量打标。
  - `advanced_search_wizard`：多维度复合条件组合搜索向导。
  - `mabel_chat`：梅贝尔角色对话模版（自动提取真实台词作为 Few-Shot 样本）。

---

## 仓库结构

```text
Mabel-s-Tentacles/
├── common/           # 跨模块零业务依赖的公共工具层
├── describer-go/     # 描述机：纯字节/文件确定性特征提取（90+ cod-* 字段）
├── indexer-go/       # 索引机：内存倒排与分桶索引、布尔表达式求值器
├── manager-go/       # 管理机：UUID 派生落盘、LRU 缓存、目录树与盘库同步
├── chaos-go/         # 混沌机：梅贝尔台词库、真/伪随机、JMComic 扩展
├── mcp-server-go/    # 编排中心与服务入口：MCP SSE Server + HTTP API + PostgreSQL 存储
├── frontend/         # Web 管理后台（Vue 3 + Element Plus + Vite）
├── database/         # PostgreSQL 初始化 Schema（init.sql）
├── test/             # 跨模块集成测试夹具与端到端场景验收工具
└── docs/             # 架构设计、检索契约、权限与维护机制文档
```

---

## 快速开始

### 1. Docker Compose 部署（推荐）

项目通过 `docker-compose.yml` 编排为三个容器：`agent_postgres`（127.0.0.1:54322）、`agent_backend`（127.0.0.1:18001）与 `agent_frontend`（0.0.0.0:18080）。

```bash
# 1. 准备环境变量（生产环境请务必修改默认密码与密钥）
cp .env.production.example .env

# 关键配置项说明：
# POSTGRES_PASSWORD / JWT_SECRET / ADMIN_PASSWORD：必填强随机值
# MCP_API_KEY：MCP 通道管家密钥（留空则为开发模式免鉴权放行）
# DOWNLOAD_BASE_URL：对外公网域名（如 https://your-domain.com），用于生成可直下的短链

# 2. 启动全部服务
docker compose up -d --build
```

> **低配服务器提示**：若云服务器内存较小（如 2GB），建议在本地开发机构建 `mabel-s-tentacles-backend:latest` 与 `mabel-s-tentacles-frontend:latest` 镜像后通过 `docker save` / `docker load` 传输至服务器，再执行 `docker compose up -d` 免编译直接启动。

### 2. 接入 MCP 客户端（以 AstrBot 为例）

- **同机宿主机访问**：`http://127.0.0.1:18001/sse`
- **同机 Docker 容器互通**（将客户端容器接入 `mabel-s-tentacles_default` 网络）：`http://agent_backend:8080/sse`

```json
{
  "mcpServers": {
    "Mabel-s-Tentacles": {
      "active": true,
      "transport": "sse",
      "url": "http://agent_backend:8080/sse",
      "headers": {
        "Authorization": "Bearer <你的_MCP_API_KEY>"
      },
      "timeout": 5,
      "sse_read_timeout": 300
    }
  }
}
```

### 3. 本地开发（Windows / PowerShell 7）

```powershell
# 1. 启动 PostgreSQL 容器
docker compose up -d postgres

# 2. 一键编译 Go 后端（:8080）并启动前端开发服务器（:5173）
pwsh -NoProfile -File .\start-dev.ps1
```

---

## 文档导航

详细设计与维护规范均位于 [`docs/`](docs/README.md) 目录：

- [部署指南](部署指南.md) —— 环境变量、Docker 编排、AstrBot 接入与 1Panel 反向代理配置（含 `/api/` 与 `/d/` 短链路由）
- [架构设计](docs/架构设计.md) —— 描述机、索引机、管理机与编排中心的职责边界与依赖铁律
- [MCP 服务规范](docs/MCP服务规范.md) —— 工具边界矩阵、上传规范、Resources 与 Prompts 设计
- [检索契约](docs/检索契约.md) —— `list_index_fields` → `search_files` 两步精确查询契约
- [元数据字段说明](docs/元数据字段说明.md) —— 90+ 确定性字段（`cod-*`）与语义字段字典
- [权限设计](docs/权限设计.md) —— 用户键空间（`~user/`）、主体模型与可见性矩阵
- [管理机与索引机更新机制](docs/管理机与索引机更新机制.md) —— 持久维护任务状态机与 T1/T2/T3 同步机制
- [测试规则](test/测试规则.md) —— L1–L4 分层测试规范与端到端回归方案

---

## 许可证

本项目采用 [PolyForm Noncommercial 1.0.0](https://polyformproject.org/licenses/noncommercial/1.0.0) 许可证，完整条款见 [LICENSE](LICENSE)。

- ✅ **允许**：个人学习、学术研究、爱好者项目、教育与公益等**非商业用途**下的使用、修改与再分发（须完整保留版权声明与许可证）。
- ❌ **禁止**：任何形式的**商业用途**（包括商业售卖、付费托管服务、企业经营性内部使用等）及抹除版权声明的搬运分发。

如需商用授权或其他许可事宜，请联系作者。
