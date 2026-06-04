# Agent MCP Server Service

## 1. 简介 (Overview)
MCP (Model Context Protocol) Server 是一个独立的代理服务进程，通常被大语言模型 (LLM) 或是上层 Agent 调度使用。通过向模型提供标准化的上下文与工具接口（Tools），使得 AI 能够安全、规范地执行操作系统命令、读写文件或进行数据库查询。

## 2. 核心技术栈
- **语言**: Python 3.11+
- **依赖管理**: `pyproject.toml`
- **协议实现**: MCP 规范
- **功能模块**: 系统命令执行引擎、文件 I/O 处理器、数据库访问层

## 3. 目录结构
- `src/server.py`: MCP 服务器的核心入口点。
- `src/config.py`: 服务及环境变量的加载。
- `src/services/db.py`: 与主数据库交互的服务层。
- `src/services/command_executor.py`: 处理命令行及 Shell 脚本的执行逻辑层。
- `src/tools/`: 暴露给大模型调用的具体技能工具，例如：
  - `execute_command.py`: 运行终端命令。
  - `write_file.py`: 写入或修改文件。
  - `get_results.py`: 获取工具执行的结果等。
- `mcp-config.json`: MCP 服务的元数据与启动配置声明。
- `Dockerfile`: 构建 MCP 服务镜像。

## 4. 架构优化建议 (Optimization Analysis)
MCP 服务具有最高级别的系统访问权限，面临极高的安全和并发压力。建议在以下方面进行加固和优化：
1. **指令安全与沙箱隔离 (Crucial)**: 提供 `execute_command` 功能时，绝不能原样将 LLM 输出的字符串直接交给系统 `shell=True` 运行。必须引入严格的白名单机制、危险字符过滤，甚至在容器/沙箱 (Sandbox) 级别限制进程的网络与挂载权限，防范 `rm -rf /` 或反弹 Shell 攻击。
2. **操作审计日志 (Audit Logging)**: LLM 通过 MCP 执行的每一条指令、每一个文件的修改操作，都应当具有防篡改的完整日志。建议使用专门的 `audit.log` 记录操作时间、请求负载与最终输出。
3. **超时与资源限制 (Resource Quotas)**: 模型可能会生成一个死循环命令或大文件读取操作。在 `command_executor.py` 层面，需要增加严格的超时时间控制 (Timeout) 以及内存/CPU 使用限制 (cgroups)，防止服务器资源被耗尽。
4. **并发支持与状态隔离**: 如果 MCP 服务需要同时处理多个客户端/模型的请求，确保不同 Session 的执行上下文（如当前工作目录、临时环境变量）相互隔离，不会发生状态污染。
