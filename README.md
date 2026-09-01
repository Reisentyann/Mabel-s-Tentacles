# Mabel-s-Tentacles

> 梅贝尔之触 —— 她会帮你管理文件。

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

- `write_file` / `read_file` —— 创造文件 / 读取文件
- `modify_data_file` —— 追加写文件
- `list_data_files` / `describe_file` —— 获取文件列表 / 给文件写描述与标签（供检索）
- `copy_file` —— 复制文件
- `execute_command` / `get_results` —— 执行 Shell 命令 / 查询历史执行结果

**Web 前端**（人类使用）：

- 文件区浏览、搜索、描述管理、网页下载（含打包下载）
- 命令执行记录仪表盘

**开发中**：文件内容解析并分类（deving）

~~机器人分段回复功能~~（已禁用退役）

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
