# AstrBot 接入 MCP 服务端指南

AstrBot 是一款功能强大的聊天机器人框架，它支持通过 Model Context Protocol (MCP) 接入外部工具，从而赋予机器人操作服务器、写入文件等“特权”能力。

我们的 `agent-mcp-server` 是一个基于 stdio (标准输入输出) 运行的标准 MCP 服务器，可以直接被 AstrBot 调用。

## 1. 了解连接方式

我们的 MCP 服务器使用了 `mcp.server.stdio.stdio_server()`。这意味着它不是通过 HTTP 端口暴露的（虽然我们在 `docker-compose.yml` 中映射了 18001，但那是 Docker 容器的配置边界，代码实际上跑的是 stdin/stdout），而是通过执行命令行脚本来启动并进行进程间通信。

因此，为了让 AstrBot 连接，AstrBot 所在的机器/容器需要能够直接调用 python 命令启动我们的 MCP 脚本。

## 2. AstrBot 接入配置 (SSE 模式)

考虑到大多数机器人框架和 Docker 跨容器通信的易用性，本项目已经将 `agent_mcp_server` 的通信模式由 stdio 升级为了基于网络的 **SSE (Server-Sent Events)** 模式。

这意味着，只要您的 AstrBot 能够 ping 通本项目部署机器的 IP 或通过 Docker 网络相互访问，就能直接通过 HTTP 协议挂载。

在 AstrBot 的 MCP 配置文件或后台界面中，添加如下 **SSE 类型**的服务器节点：

```json
{
  "mcpServers": {
    "agent-mcp-server": {
      "type": "sse",
      "url": "http://127.0.0.1:18001/sse"
    }
  }
}
```

### 💡 注意事项：
* 如果您的 AstrBot 和本项目是在**同一台物理机**的不同 Docker 环境运行的，且没有配置特定的 Docker 网络，`127.0.0.1` 可能会指向 AstrBot 容器自身。
* 此时您可以将 `127.0.0.1` 替换为您物理机的 **局域网 IP** (例如 `192.168.1.100`)。或者是把 AstrBot 容器加入到 `agent_default` 网络中，并将 URL 改为容器名 `http://agent_mcp_server:8001/sse`。

## 4. 可用能力列表

成功接入后，AstrBot 的大模型将自动获知并能使用以下三个工具：

1. **`execute_command`**: 能够执行宿主机容器内的 Shell 命令（例如 `ping`, `ls` 等）。需要传入 `command` 与 `user_id`。
2. **`get_results`**: 获取该 `user_id` 之前执行命令的历史记录和结果。
3. **`write_file`**: 可以将大模型生成的长文本、代码，直接保存为服务器 `/app/data/` (映射到宿主机 `data/`) 文件夹下的物理文件。

## 5. 测试接入

配置完成后重启 AstrBot，您可以在聊天窗口中输入：
> "帮我执行一个 ls -la 命令，我的 user_id 是 1"

如果 AstrBot 成功回复了包含目录文件的结果，或者说 "我已经帮你执行完毕"，说明 MCP 服务接入已经圆满成功！您在网页端的 Dashboard 里也能同步看到这一条命令记录。