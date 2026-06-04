# AstrBot 接入 MCP 服务端指南

AstrBot 是一款功能强大的聊天机器人框架，它支持通过 Model Context Protocol (MCP) 接入外部工具，从而赋予机器人操作服务器、写入文件等“特权”能力。

我们的 `agent-mcp-server` 是一个基于 stdio (标准输入输出) 运行的标准 MCP 服务器，可以直接被 AstrBot 调用。

## 1. 了解连接方式

我们的 MCP 服务器使用了 `mcp.server.stdio.stdio_server()`。这意味着它不是通过 HTTP 端口暴露的（虽然我们在 `docker-compose.yml` 中映射了 18001，但那是 Docker 容器的配置边界，代码实际上跑的是 stdin/stdout），而是通过执行命令行脚本来启动并进行进程间通信。

因此，为了让 AstrBot 连接，AstrBot 所在的机器/容器需要能够直接调用 python 命令启动我们的 MCP 脚本。

## 2. AstrBot 端 MCP 配置

大多数支持 MCP 的客户端（如 AstrBot, Claude Desktop 等）使用的是类似如下的 JSON 配置来挂载基于 stdio 的 MCP 服务。

在 AstrBot 的 MCP 配置项中（或对应的插件配置），添加以下节点：

```json
{
  "mcpServers": {
    "agent-mcp-server": {
      "command": "docker",
      "args": [
        "exec",
        "-i",
        "agent_mcp_server",
        "python",
        "-m",
        "src.server"
      ]
    }
  }
}
```

### 💡 配置原理解释
因为我们的 MCP Server 是运行在 Docker 容器 `agent_mcp_server` 内部的，而 AstrBot 如果运行在宿主机上，可以通过 `docker exec -i` 的方式将容器内的标准输入输出与宿主机的进程打通。

**重要参数说明：**
- `command`: 基础命令。
- `args`: 命令参数。这里我们让它去执行运行中的 `agent_mcp_server` 容器内部的启动命令 `python -m src.server`。
- `-i`: Interactive 参数，保证 stdin 流畅通，这对 stdio 模式的 MCP 至关重要。不要加 `-t`，因为 tty 终端控制字符会污染 MCP 的 JSON-RPC 消息流。

## 3. 本地直接调用 (不通过 Docker)

如果您的 AstrBot 与本项目代码放置在同一台机器的同一级目录，且环境安装了 Python 依赖，您也可以不借助 Docker，直接通过 Python 调用：

```json
{
  "mcpServers": {
    "agent-mcp-server": {
      "command": "python",
      "args": [
        "-m",
        "src.server"
      ],
      "env": {
        "DATABASE_URL": "postgresql+asyncpg://agent_user:your_secure_db_password@localhost:54322/agent_db"
      }
    }
  }
}
```
*注意：本地调用需要根据您的 `.env` 手动传入 `DATABASE_URL`，并且数据库主机地址应指向映射的 `54322` 端口。*

## 4. 可用能力列表

成功接入后，AstrBot 的大模型将自动获知并能使用以下三个工具：

1. **`execute_command`**: 能够执行宿主机容器内的 Shell 命令（例如 `ping`, `ls` 等）。需要传入 `command` 与 `user_id`。
2. **`get_results`**: 获取该 `user_id` 之前执行命令的历史记录和结果。
3. **`write_file`**: 可以将大模型生成的长文本、代码，直接保存为服务器 `/app/data/` (映射到宿主机 `data/`) 文件夹下的物理文件。

## 5. 测试接入

配置完成后重启 AstrBot，您可以在聊天窗口中输入：
> "帮我执行一个 ls -la 命令，我的 user_id 是 1"

如果 AstrBot 成功回复了包含目录文件的结果，或者说 "我已经帮你执行完毕"，说明 MCP 服务接入已经圆满成功！您在网页端的 Dashboard 里也能同步看到这一条命令记录。