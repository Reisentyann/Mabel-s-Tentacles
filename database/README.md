# Database Initialization

## 1. 简介 (Overview)
本目录用于存放 PostgreSQL 数据库的初始化脚本与配置。依托于官方 Postgres Docker 镜像的特性，这里的脚本会在数据库容器挂载的卷首次初始化（数据卷为空）时被自动执行。

## 2. 目录结构
- `init.sql`: 包含建表语句、索引创建、初始角色或触发器定义等原生 SQL 脚本。

## 3. 初始化与 schema 演进

- **新环境**：PostgreSQL 容器首次初始化时执行 `init.sql`，建立基础表与索引。
- **已有环境**：Go 后端启动时由 `mcp-server-go/internal/repo` 执行幂等迁移，包括 `CREATE TABLE IF NOT EXISTS` 和 `ALTER TABLE ... ADD COLUMN IF NOT EXISTS`，不再使用 Python/SQLAlchemy/Alembic。
- **新增结构**：同步维护 `init.sql` 的全新环境基线和 repo 中已有环境的升级语句，并补充对应迁移测试。
- **初始账号**：由后端启动流程按配置创建管理员，不使用 Python Seed 脚本。
- **权限最小化**：生产环境应使用仅具备业务所需 DML/DDL 权限的数据库账号，避免应用使用超级用户。
