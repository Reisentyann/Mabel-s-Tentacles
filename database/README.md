# Database Initialization

## 1. 简介 (Overview)
本目录用于存放 PostgreSQL 数据库的初始化脚本与配置。依托于官方 Postgres Docker 镜像的特性，这里的脚本会在数据库容器挂载的卷首次初始化（数据卷为空）时被自动执行。

## 2. 目录结构
- `init.sql`: 包含建表语句、索引创建、初始角色或触发器定义等原生 SQL 脚本。

## 3. 架构优化建议 (Optimization Analysis)
对于现代的复杂应用来说，完全依赖 `init.sql` 并非长久之计。建议在数据库管理流程上进行如下优化：
1. **采用 Alembic 进行版本迁移**: 后端采用了 SQLAlchemy，强烈建议摒弃 `init.sql` 这种一次性的建表方式，转而使用 Alembic 工具（已在后端 requirements.txt 中列出）。将所有的表结构定义转化为可追溯的迁移脚本 (`migration versions`)。这样可以在保留现有数据的情况下安全地跟踪和部署表结构变更。
2. **种子数据自动化 (Seed Scripts)**: 目前的 SQL 初始化不利于开发环境和生产环境的数据隔离。建议引入基于 Python 的 Seed 脚本（例如在后端项目中编写 `scripts/seed_data.py`），用于在开发环境中一键生成测试用户或默认业务配置项，而不去污染纯粹的结构 SQL。
3. **权限最小化原则**: 在生产环境中，当前脚本一般以 `postgres` 超级用户权限运行并使用默认库。建议增加更为细粒度的角色授权逻辑，为业务微服务（如 Backend、MCP-Server）单独分配只具备 DML 和特定 DDL 权限的服务专用账号。
