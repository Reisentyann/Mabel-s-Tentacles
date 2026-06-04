# Agent Backend Service

## 1. 简介 (Overview)
本项目是一个基于 FastAPI 构建的异步后端服务，主要负责处理前端的业务请求、用户认证鉴权（JWT），以及与 PostgreSQL 数据库进行数据交互。

## 2. 核心技术栈
- **框架**: FastAPI
- **语言**: Python 3.11+
- **数据库 ORM**: SQLAlchemy 2.0 (异步模式) + asyncpg
- **数据验证**: Pydantic v2
- **认证与加密**: passlib + bcrypt + PyJWT

## 3. 目录结构
- `src/main.py`: 应用入口，注册路由、配置 CORS 及中间件。
- `src/config.py`: 环境及配置加载 (基于 pydantic-settings)。
- `src/database.py`: 异步数据库引擎与会话管理。
- `src/routes/`: 接口路由定义 (如 `auth.py`, `commands.py`)。
- `src/models/`: SQLAlchemy 数据库模型。
- `src/schemas/`: Pydantic 数据序列化/校验模型。
- `src/services/`: 核心业务逻辑（如密码哈希验证等）。
- `src/middleware/`: 自定义请求拦截及中间件。
- `Dockerfile`: 用于构建后端镜像。
- `requirements.txt`: 依赖清单。

## 4. 环境变量
- `DATABASE_URL`: 数据库连接字符串
- `SECRET_KEY`: JWT 签名密钥
- `POSTGRES_USER` / `POSTGRES_PASSWORD` / `POSTGRES_DB`: 默认数据库参数

## 5. 架构优化建议 (Optimization Analysis)
为了进一步提升代码的可维护性、安全性和性能，建议进行以下优化：
1. **依赖管理优化**: 建议从 `requirements.txt` 迁移到 `Poetry` 或 `Pipenv`。目前直接手写包版本容易引发隐式子依赖冲突（例如此前遇到的 `passlib` 和最新 `bcrypt` 不兼容问题），使用现代包管理工具可以锁定依赖树 (`.lock`)。
2. **连接池与超时配置**: 在 `src/database.py` 中，目前仅使用了默认的 `create_async_engine`。建议显式配置 `pool_size`（连接池大小）、`max_overflow`（最大溢出连接）以及 `pool_recycle`（连接回收时间），以应对高并发情况下的数据库连接枯竭问题。
3. **全局异常捕获**: 目前代码中部分异常会直接导致 HTTP 500 甚至暴露堆栈信息。建议在 `src/main.py` 增加全局异常处理器（Exception Handler），拦截 SQLAlchemy 异常或业务逻辑异常，统一返回标准格式的 JSON 错误响应。
4. **日志系统规范化**: 目前大多通过 `print()` 或基础日志进行输出。建议引入 `Loguru` 替代标准库日志，规范日志级别、格式及文件分割策略，方便后期结合 ELK 等日志系统使用。
