# common 共享层（跨模块纯函数复用）

> 本文档是 `common/` 模块的使用说明书与准入规则：写代码前先来这里查，
> 能复用就不新写；想往里加东西先过准入判据。架构形状见 `架构设计.md`，
> 本文只管 common 这一层。
> 状态：已实现（2026-09-06，重构批次：三处逐字复制 + 六处同义助手收敛）。

## 1. 定位与铁律

**形态**：仓库根目录的独立 Go 模块 `common/`（单根包平铺，与 indexer-go 同款零依赖立场），
四个业务模块（describer-go / indexer-go / manager-go / mcp-server-go）都可 require 它。

**四条铁律**：

1. **零业务依赖**：common 不 import 业务模块（describer / indexer / manager / mcp-server-go）——
   它是最底层叶子，依赖方向永远朝它汇聚，不可能成环。
   （谁想反向让 common 调 repo/manager，就说明这个函数不该进 common。）
   允许成熟的纯工具三方库（如 filepath-securejoin——runc/containerd 同款路径安全库，
   "零业务依赖"指的是不绑业务，不是拒绝轮子）。
2. **只收纯函数**：无状态、无 IO 副作用（盘读/哈希这类"读而不改"的 IO 除外）、
   无配置、无初始化顺序要求。slog / config / DB 连接这类设施**永远不进**。
3. **不持有业务口径**：预算常量的正主在业务模块（如 `describer.MaxHeadBytes` /
   `MaxFullBytes`），common 通过参数接收口径（`ReadHead(abs, headBytes)`），
   自己不定义业务数字——口径单一来源，不重演"5MB 重定义 5 处"。
4. **错误文案是对外契约**：`ResolveWithin` 的 "security error: ..." 系列
   是两侧调用方原样透传的既有契约，改文案 = 改对外行为，需走评审。

## 2. 现有清单（写代码前先扫这张表）

### common/fsutil.go —— 文件安全层 + 盘读助手

| 函数 | 签名 | 用途 | 收编前重复 |
|---|---|---|---|
| `ResolveWithin` | `(baseDir, rel) → (abs, error)` | 防目录穿越 / 拒盘符 / **拒 symlink 逃逸与改写**（securejoin 逐段解析 + fail-closed 双检，2026-09-08） | manager-go/placement.go `resolve` 内化副本 + service/files.go `resolveWithin`（archive.go 同用） |
| `WithinDir` | `(dir, target) → bool` | target 是否仍在 dir 内（`..` 前缀 = 越界） | 两份同款 `withinDir` |
| `ReadHead` | `(abs, headBytes) → ([]byte, error)` | 读文件前 N 字节（短文件按实际长度） | updater.go / executor.go 两份 `readHead` |
| `ReadLimited` | `(abs, limit) → ([]byte, error)` | LimitReader 限读（防超大文件内存峰值） | 两份 `readLimited` + verify 的 `readUpTo` 变体 |
| `ChecksumFile` | `(abs) → (hex, error)` | 全文件流式 SHA-256（完整性事实，不受分析预算影响） | 两份 `checksumFile` |

### common/ptr.go —— 指针小件

| 函数 | 语义 | 收编前重复 |
|---|---|---|
| `StrPtr` | 空串→nil（COALESCE：nil 不覆盖既有值），非空→指针 | tools.StrPtr + core.strPtr（当年为防 import 成环而生的副本——common 顶层包无此约束） |
| `DerefStr` | nil→空串 | repo.derefStr + listdatafiles.ptrStr |
| `DerefInt64` | nil→0 | listdatafiles.ptrInt64 |

### common/httpx.go —— HTTP 小件

| 函数 | 语义 | 收编前重复 |
|---|---|---|
| `ClientIP` | 反代优先 X-Forwarded-For 首段，否则 RemoteAddr 去端口 | api 包 `clientIP`（middleware/admin/auth/files/ratelimit 六文件共用）+ mcp/auth.go 同款 |

## 3. 接线（新模块 require common）

```text
require github.com/Reisentyann/Mabel-s-Tentacles/common v0.0.0
replace github.com/Reisentyann/Mabel-s-Tentacles/common => ../common
```

Docker 构建上下文 = 仓库根，`mcp-server-go/Dockerfile` 已 COPY 全部五个模块的
go.mod 与源码；新加模块记得同步 Dockerfile 的两段 COPY。

## 4. 准入判据（想往 common 加东西时）

**进**（同时满足）：

- 纯函数、零业务依赖（判据 1/2 见上）
- 已在 **≥2 个模块**重复出现，或同模块 **≥2 个包**重复（如 clientIP 在 api 六个文件）
- 语义稳定（不是快照期的临时代码）

**不进**（现状清单，避免误收）：

- 单点存在的设施：slog 初始化（logging.go 唯一）、config 加载、JWT/bcrypt、
  pgxpool 连接——没有重复可消，收进来反而抬高依赖
- manager 的哨兵错误系列（`manager: xxx not implemented`）——刻意的域模式
- describer 插件 `init()` 自注册——插件架构本体
- 测试内联助手（如 authz_test.go 的 `strPtr`：纯取地址，语义与 `common.StrPtr`
  的空串→nil 不同）——测试就近可读性优先
- 口径常量（5MB/512B 等）——正主在 describer，引用正主而不是搬进 common

**待收编候选**（后续批次做了就划掉）：

| 候选 | 现状 | 备注 |
|---|---|---|
| MCP 工具三态编排（denied/failed/success + duration + slog 顺序） | 10 个工具各写一遍 | 模块内收口（tools.Wrap 包装器），不动模块边界 |
| `InferFileMeta`/`InferScope`/`InferExtension` + FileMetadata 组装 | core/executor.go 与 repo/manager_adapter.go 两份 | 收口到装配层一处 |
| since 游标分页扫描循环 | updater.Backfill 与 core.RebuildIndex 两份同款 | 可做 `ScrollPage[T]` 泛型助手 |
| `Sink` 接口 `Update(uuid, old, new)` | indexer / manager / core 三处同签名（结构同构，刻意零依赖） | 收编 = 获得编译期漂移保护，代价是 indexer/manager 多一条 replace 依赖，需权衡 |

## 5. 复用检查点（写代码时）

- 要写"路径校验/防穿越"→ **先查 `fsutil.ResolveWithin`**，别再手写 TrimLeft/盘符/Rel 判定
- 要读文件头/限读/算哈希 → **先查 `fsutil` 三件套**，口径参数从 describer 常量传入
- 要"空串→nil 指针"或解引用 → **先查 `ptr`**
- 要取客户端 IP → **先查 `httpx.ClientIP`**（HTTP 与 MCP 两侧同口径）
- 5MB/512B 数字 → **引用 `describer.MaxFullBytes`/`MaxHeadBytes` 正主**，不写字面量
