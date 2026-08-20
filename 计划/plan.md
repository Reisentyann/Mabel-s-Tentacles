# 合并 backend 与 mcp-server-go 为单一 Go 服务 —— 实施计划

## 一、目标

把 Python `backend/`（FastAPI）的功能并入 `mcp-server-go/`，形成**单一 Go 服务**，同时提供两个入口：

- **MCP（SSE）**：`/sse`、`/message`，给 AstrBot 调用工具。
- **HTTP API**：`/api/*`，给前端调用。

Python `backend/` 与 `mcp-server/` 在 Go 全部接住后退役（移入 `旧内容/`，不删除）。

## 二、已锁定决策

| 项 | 决策 |
|---|---|
| 服务形态 | 单服务单文件夹：`mcp-server-go/`，入口 `cmd/server/main.go` |
| 分层 | `api/` + `tools/`（两个薄接入层）→ `service/`（业务）→ `repo/`（数据，由 `store` 改名） |
| 鉴权 | `/api/*` 加 JWT 中间件；`/auth/*`、`/files/download`、`/health` 豁免 |
| 下载鉴权 | `/files/download` 用 query `token`（`api.access_token`），不用 JWT（前端 `window.location` 带不了 header） |
| 前端 | 独立服务，nginx 反代 `/api` → Go 内网端口 |
| 数据库 | 单一 schema：`users`、`token_blacklist`、`commands`、`operations` |

## 三、目标目录结构

```
mcp-server-go/                     # 一个服务一个文件夹
├── go.mod
├── config.yml(.example)
├── cmd/server/main.go             # 组装 + 启动 + 优雅关闭 + 管理员 bootstrap
└── internal/
    ├── config/                    # 配置（含 security/admin 块）
    ├── logging/                   # slog 初始化
    ├── repo/                      # 数据层（原 store）：接口 + pgx 实现 + 迁移
    │   ├── repo.go                # Store 接口定义
    │   ├── users.go               # users 表读写
    │   ├── tokens.go              # token_blacklist 表读写
    │   ├── commands.go            # commands 表读写（分页/详情）
    │   └── operations.go          # operations 表读写
    ├── service/                   # 业务层（与协议无关）
    │   ├── auth.go                # bcrypt 校验 + JWT 签发/校验
    │   ├── command.go             # ExecuteCommand（shell）
    │   ├── files.go               # SafeWrite/SafeModify/SafeList/ListTree/ResolvePath
    │   └── archive.go             # ZipFiles（只读不删）
    ├── api/                       # 接入层①：HTTP REST（薄 handler）
    │   ├── router.go              # 集中注册路由
    │   ├── middleware.go          # JWT 中间件 + 请求日志 + 尾斜杠归一化
    │   ├── auth.go                # login/refresh/logout（register 返回 403）
    │   ├── commands.go            # GET /api/commands、/api/commands/{id}
    │   ├── files.go               # GET /api/files、download、download-zip
    │   └── operations.go          # GET /api/operations
    ├── tools/                     # 接入层②：MCP 工具（每工具一子包，薄 handler）
    │   ├── registry.go / all/all.go
    │   ├── writefile/ listdatafiles/ modifydatafile/
    │   ├── executecommand/ getresults/ segmentedreply/
    └── mcp/
        ├── server.go              # MCP 组装
        └── auth.go                # MCP Bearer 鉴权
```

## 四、分层依赖方向

```
api/（HTTP handler）──┐
                      ├──► service/ ──► repo/ ──► pgx
tools/（MCP handler）─┘
```

- 接入层只做「解参数 → 调 service → 包返回值」，不做业务。
- `service` 与协议无关，可被 `api` 与 `tools` 复用（files/commands/operations 两头共用）。
- `repo.Store` 保持接口，便于 mock 测试与替换实现。

## 五、改动清单

### 1. repo 层（`internal/store` → `internal/repo`）
- 重命名包与 `Store` 接口、`Deps.Store` 引用。
- 迁移补充表：`users`、`token_blacklist`（现只有 `operations`、`commands`）。
- `commands` 表：**去掉 `user_id` 外键**（MCP 写入的是 QQ 用户 ID，不在 users 表，保留外键会插入失败）、补 `source` 列。
- 接口新增方法：
  - `GetUserByUsername(ctx, username)` → 用户
  - `InsertBlacklist(ctx, jti, expiresAt)` / `IsTokenBlacklisted(ctx, jti)`
  - `ListCommands(ctx, page, size)` / `GetCommand(ctx, id)`
- `CommandResult` 补字段：`user_id`、`source`、`command_type`、`environment`（对齐前端 `CommandResponse`）。

### 2. service 层
- 新增 `service/auth.go`：
  - bcrypt 哈希/校验（`golang.org/x/crypto/bcrypt`，兼容 Python 已存的 `$2b$` 哈希，无需迁移数据）。
  - JWT 签发/校验（`github.com/golang-jwt/jwt/v5`），claims 含 `sub`(username)、`exp`、`user_id`、`jti`。

### 3. api 层
- `auth.go`：
  - `POST /api/auth/login` → 成功 `{access_token, refresh_token, token_type:"bearer"}`；失败 401 `{"detail":"Incorrect username or password"}`。
  - `POST /api/auth/refresh` → 校验 jti 未拉黑 → 拉黑旧 jti → 发新 token。
  - `POST /api/auth/logout` → 拉黑 jti。
  - `POST /api/auth/register` → 403（注册已禁用，与 Python 一致）。
- `commands.go`：`GET /api/commands`（分页）、`GET /api/commands/{id}`（ServeMux `{id}` 路径参数）。
- `middleware.go`：
  - JWT 中间件（豁免 `/auth/*`、`/files/download`、`/health`）。
  - 尾斜杠归一化（前端调 `/files/`、`/operations/`、`/commands/`，否则 404）。
  - 请求日志（迁移现有 `RequestLog`）。
- 错误响应统一 `{"detail":"..."}`（与 FastAPI 一致，前端 `LoginView` 依赖）。

### 4. config / 入口
- `config.yml` 新增：
  ```yaml
  security:
    secret_key: "..."
    algorithm: "HS256"
    access_token_expire_minutes: 30
    refresh_token_expire_days: 7
  admin:
    username: "admin"
    password: "admin123"
  ```
- `main.go` 启动时 bootstrap 管理员（查重，不重复建，等价 Python `lifespan`）。

### 5. 日志（重点，全链路）
- 统一 `slog` 结构化日志（已有 `logging.Setup`，`debug|info|warn|error` + `json|text`）。
- HTTP 层：`RequestLog` 中间件输出 `method path status duration`。
- MCP 层：每次工具调用 `RecordOperation` 写审计存档（已有）。
- **新增**：
  - 认证事件打日志：登录成功/失败、refresh、logout、JWT 校验失败（不记录密码）。
  - 下载/打包：记录 `path`、文件数、字节数、耗时。
  - 命令执行：`command`、`exit_code`（已有，保留）。
- 所有错误路径 `slog.Error`，正常路径 `slog.Info`，避免裸 `print`。

### 6. 部署
- `frontend/nginx.conf`：`proxy_pass` 从 `backend:8000` 改为 `backend:8080`。
- `docker-compose.yml`：删除 Python `backend`、`mcp-server` 两个 service，改为单个 Go `backend`（build `./mcp-server-go`，**不映射公网端口**）；保留 `postgres`、`nginx`、`frontend`。
- `database/init.sql`：`commands.user_id` 去外键、补 `source`，与 Go 迁移对齐。

## 六、实施顺序

1. `store` → `repo` 重命名 + 补 `users`/`tokens` 表与方法 + `CommandResult` 补字段
2. 新建 `service/auth.go`（bcrypt + JWT）
3. api 层：`auth.go` 路由（login/refresh/logout/register）
4. api 层：`commands.go` 路由
5. api 层：`middleware.go`（JWT 中间件 + 尾斜杠归一化）+ `router.go` 重构
6. config 加 security/admin 块 + `main.go` 管理员 bootstrap
7. 部署切换（nginx.conf、docker-compose、init.sql）
8. 联调：MCP 工具、文件下载/批量下载、命令查询、操作存档、JWT 全流程
9. Python `backend/`、`mcp-server/` 退役（移入 `旧内容/`）

## 七、风险与注意

- **密码兼容**：Go bcrypt 可验证 Python 已存 `$2b$` 哈希，bootstrap 时查重避免重复建管理员。
- **JWT claims**：必须含 `sub`、`exp`（前端 `decodeJwt` 只读这两个）。
- **下载不套 JWT**：`/files/download` 保持 query token，否则前端下载 401。
- **尾斜杠**：Go ServeMux 精确匹配，`/api/files` 不匹配 `/api/files/`，必须归一化。
- **不删除任何文件**：退役的 Python 代码与旧文档一律移入 `旧内容/`，由人类决定去留。
