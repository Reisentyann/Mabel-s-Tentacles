# MCP 服务规范与重构设计

> 本文档规范 Mabel-s-Tentacles 的 MCP（Model Context Protocol）服务面设计。
> 包含工具（Tools）命名体系、入参契约、Prompt 模版、Resource 资源、以及新增能力的演进路线。
> 状态：规范定稿，待按此推进 `mcp-server-go` 重构。

---

## 1. 现状痛点与重构目标

### 1.1 现状硬伤
1. **命名分裂**：`write_file` / `read_file` 采用 `[verb]_file`，但中间夹杂祖传的 `modify_data_file`、`list_data_files`（带有早期的 `_data_` 痕迹）；命令历史工具叫极其模糊的 `get_results`。
2. **入参 key 割裂**：`read_file` 入参叫 `path`，其他工具叫 `file_path`，移动/复制叫 `source` / `target`；模型频繁因参数名细微差异导致调用失败。
3. **抽象严重泄露**：`execute_command` 与 `get_results` 强制要求外部 Agent 传入数据库自增主键 `user_id`，而 Agent 本身并不知道自己的 DB 整数 ID。
4. **生命周期有头无尾**：缺少主动删除工具（`delete_file`），缺少单文件元数据直查（`get_file_info`），缺少代码/文本微创编辑（`patch_file`）。
5. **协议只用三分之一**：未利用 MCP 的 **Prompts** 与 **Resources** 特性，仅作为纯 RPC 工具箱使用。
6. **提示词打磨糙**：复杂工具（如 `search_files` 的布尔组合树、`describe_file` 的枚举标签）缺少 Few-Shot 示例与强 Schema 约束。

### 1.2 重构原则
- **正交一致**：所有文件操作一律采用 `[verb]_file` 动词+单数名词命名；统一路径入参为 `file_path`（代码层对旧参数名 `path`、`source` 做静默别名兼容，平滑过渡）。
- **零泄漏**：上下文相关的身份（Principal / User）一律从 MCP 会话与 Token 中隐式提取，严禁在工具入参暴露内部主键。
- **全生命周期闭环**：创（write）、查（read/stat/list/search）、改（modify/patch/move/copy/describe）、灭（delete）全链路打通。
- **协议完整实现**：补齐 Tools + Prompts + Resources 三支柱，充分利用客户端上下文装配能力。

---

## 2. 工具面（Tools）重构体系

### 2.1 核心工具清单与命名映射

| 新工具名（规范） | 旧工具名（兼容别名） | 类别 | 功能说明 | 核心变更点 |
|---|---|---|---|---|
| `read_file` | `read_file` | 读 | 读文件文本（1MB 截断防爆） | 入参统一为 `file_path`，兼容 `path` |
| `write_file` | `write_file` | 写 | 写入新文件/全量覆盖，带元数据 | 保持现有能力，细化提示词 |
| `modify_file` | `modify_data_file` | 写 | 追加 (`append`) 或整段覆写 (`overwrite`) | 剔除 `_data_` 冗余字样，入参统一 |
| `patch_file` | *(新增)* | 写 | 局部精准替换（基于唯一锚点文本替换） | 避免大文件全量覆写导致上下文污染与截断风险 |
| `delete_file` | *(新增)* | 灭 | 软删除文件（标记删除，进入回收期） | 补齐文件清理闭环，支持回收留档 |
| `get_file_info` | *(新增)* | 查 | 获取单文件完整详情（元数据+90+事实字段） | 终结“查已知文件还必须走搜索”的反直觉操作 |
| `list_files` | `list_data_files` | 查 | 目录/文件分页简览列表 | 剔除 `_data_` 冗余，返回字段与类型对齐 |
| `search_files` | `search_files` | 查 | 内存索引布尔条件查询 | 保持强大 And/Or/Not 树，补全 Few-Shot Description |
| `list_index_fields` | `list_index_fields` | 发现 | 发现全部可索引字段、分档基准、当前值域 | 增加缓存与 Resource 对应引导 |
| `move_file` | `move_file` | 管理 | 逻辑重命名/移动（保留 uuid、元数据、谱系） | 入参支持 `source_path` / `target_path`（兼容 `source`/`target`） |
| `copy_file` | `copy_file` | 管理 | 复制文件与元数据，记来源血缘 | 入参支持 `source_path` / `target_path` |
| `describe_file` | `describe_file` | 标注 | 给文件打标签、语义分类与自定义属性 | 明确区分只读 `cod-*` 引擎事实区与开放描述区 |
| `analyze_file` | `analyze_file` | 引擎 | 强制重新运行确定性描述机刷新事实字段 | 明确适用场景（绕过写入、引擎版本升级） |
| `execute_command` | `execute_command` | 系统 | 执行 Shell 命令（仅管家 Master Key 权限） | **彻底移除 `user_id` 入参**，由上下文自动注入 |
| `get_command_history`| `get_results` | 系统 | 查看历史命令执行记录与输出 | **改名**并**彻底移除 `user_id` 入参** |
| `mabel_quote` | `mabel_quote` | 混沌 | 梅贝尔随机台词（娱乐/陪聊） | 保持混沌机自注册 |
| `pseudo_random` | `pseudo_random` | 混沌 | 本地快速伪随机数生成 | 保持混沌机自注册 |
| `true_random` | `true_random` | 混沌 | 物理量子真随机数（需熵源接入） | 保持混沌机自注册 |
| `jm_comic` | `jm_comic` | 混沌/文件 | 查询或下载 JMComic；ZIP 下载自动交给管理机入库并生成短链 | 保持混沌机自注册，装配层调用 `Manager.ImportFile` |

---

### 2.2 新增核心工具详述

#### 1. `get_file_info`（单文件元数据直查）
- **背景**：此前 Agent 若想知道 `report.md` 的最长句、行数或标签，只能调用 `search_files` 查文件名或从 `list_data_files` 里人肉翻页，极不经济。
- **Schema**：
  ```json
  {
    "file_path": "string (必填，文件逻辑路径，如 'docs/readme.txt' 或 '~other_user/public.md')"
  }
  ```
- **返回结果**：
  ```json
  {
    "success": true,
    "data": {
      "uuid": "4a72d3e1-...",
      "logical_path": "docs/readme.txt",
      "size": 1024,
      "creator": "admin",
      "visibility": "private",
      "updated_at": "2026-09-17T12:00:00Z",
      "lineage": { "copied_from": "", "moved_from": "" },
      "description": {
        "title": "系统说明",
        "desc": "核心架构文档",
        "tags": ["guide", "system"]
      },
      "attributes": {
        "cod-text-language": "zh",
        "cod-text-lines": 85,
        "cod-text-longest-sentence": 120,
        "llm-semantic-type": "technical_doc"
      }
    }
  }
  ```

#### 2. `delete_file`（软删除/垃圾回收）
- **背景**：底层 `repo.SoftDeleteMetadata` 早已就绪，但从未暴露 MCP 入口。Agent 产生的临时测试数据、草稿无法销毁。
- **Schema**：
  ```json
  {
    "file_path": "string (必填，要删除的文件逻辑路径)",
    "reason": "string (可选，删除原因，记录于操作日志)"
  }
  ```
- **行为**：
  - 检查调用者是否有该文件的写权限。
  - 标记软删除（`deleted_at = now()`），管理机从逻辑活跃树中摘除，索引机触发 `Update(ref, old, nil)` 移除所有桶的索引映射。
  - 物理文件按安全策略移入暂存区或等待定期 GC，彻底防止误伤。

#### 3. `patch_file`（微创局部替换）
- **背景**：针对代码修改或配置调整，Agent 经常只需替换一个函数或修改一行配置。要求 Agent 全量 `write_file` 极易出现“截断失真”或丢失原文件其他段落。
- **Schema**：
  ```json
  {
    "file_path": "string (必填，文件逻辑路径)",
    "old_string": "string (必填，待替换的原始子串，必须在文件内唯一)",
    "new_string": "string (必填，用于替换的新子串)"
  }
  ```
- **行为**：
  - 读取目标文件内容，校验 `old_string` 在内容中出现且仅出现 1 次（若出现 0 次或多次则安全拒绝并报错，防止改错）。
  - 执行单次替换并原子写回，触发管理机与编排机异步重新分析。

---

### 2.3 入参别名与容错机制（代码层兼容规范）

为保证客户端生态（历史会话、旧提示词、第三方客户端）平滑过渡，在 `internal/tools/common.go` 提供统一的参数提取辅助函数：

```go
// GetFilePath 统一提取文件路径参数：优先 file_path，次选 path
func GetFilePath(req mcp.CallToolRequest) (string, error) {
    if p, ok := req.Params.Arguments["file_path"].(string); ok && p != "" {
        return p, nil
    }
    if p, ok := req.Params.Arguments["path"].(string); ok && p != "" {
        return p, nil
    }
    return "", errors.New("missing required parameter: file_path")
}

// GetSourceTarget 统一提取源与目标路径参数
func GetSourceTarget(req mcp.CallToolRequest) (source string, target string, err error) {
    // 兼容 source / source_path
    // 兼容 target / target_path
}
```

---

## 3. 上下文资源（MCP Resources）规范

MCP Resources 允许宿主客户端将静态/半静态的系统知识直接预加载到模型上下文，省去重复的 Tool Call。

### 3.1 资源挂载定义

| Resource URI | 名称 | MIME 类型 | 用途与内容 |
|---|---|---|---|
| `mabel://catalog/fields` | 索引字段目录 | `application/json` | 当前系统所有可检索字段（`cod-*`、`llm-*`）、桶类型、取值基准（bench）与字段说明。模型检索时可直接查阅。 |
| `mabel://guide/search-contract` | 检索契约速查 | `text/markdown` | `docs/检索契约.md` 的机器精炼版，包含布尔表达式（And/Or/Not 树）、8 种 op 操作符的合法组合。 |
| `mabel://persona/system` | 梅贝尔助手人设 | `text/markdown` | 梅贝尔的角色设定、服务原则、工作流规范与安全边界（严谨、纯粹理性、不擅自越权）。 |

---

## 4. 提示词模版（MCP Prompts）设计

针对多步复杂文件任务，服务端预置标准工作流 Prompts，客户端可通过 `GetPrompt` 一键拉起标准作业流。

### 4.1 预设 Prompt 清单

#### 1. `organize_workspace`（整理与归档工作区）
- **参数**：
  - `target_dir`（可选，默认根目录）：待整理的目标逻辑目录。
- **内置引导**：
  1. 调用 `list_files` 获取目录下所有文件的列表与基础信息。
  2. 调用 `get_file_info` 或结合元数据判断文件的分类（代码、日志、小说、笔记、临时垃圾）。
  3. 对无描述的文件调用 `describe_file` 补充语义信息。
  4. 对格式杂乱的文件使用 `move_file` 移动到规范的分组目录（如 `notes/`、`archive/`）。
  5. 对完全过期的临时文件列出清理建议，征询用户后调用 `delete_file`。

#### 2. `advanced_search_wizard`（结构化布尔检索向导）
- **参数**：
  - `user_query`（必填）：用户的自然语言查询意图（如“找一下上周写的大于100行的Go代码或技术文档”）。
- **内置引导**：
  1. 查阅 `mabel://catalog/fields` 资源或调用 `list_index_fields` 确定对应字段：
     - 文件行数：`cod-text-lines` (`gt`, 100)
     - 语言/类型：`cod-code-language` (`eq`, "go") 或 `llm-semantic-type` (`eq`, "technical_doc")
  2. 组装合法的 And/Or 嵌套布尔树。
  3. 执行 `search_files` 并对结果集进行精准筛选与摘要回传。

#### 3. `audit_undescribed_files`（未描述文件巡检与标注）
- **参数**：
  - `limit`（可选，默认 10）：单次巡检处理数量。
- **内置引导**：
  1. 调用 `list_files` 找出标记为 `has_description: false` 的文件。
  2. 针对目标文件读取头部内容（`read_file`）并提取核心语义。
  3. 调用 `describe_file` 规范填入 `title`、`description`、`tags` 以及 `llm-semantic-type`。

---

## 5. Tool Description 提示词工程规范（防御与 Few-Shot）

所有 Tool 的 Description 必须包含三要素：**能力陈述**、**参数/Schema 防御警告**、**标准 JSON 示例**。

### 5.1 示例：`search_files` Description 优化范式

```
Search files using the deterministic memory index with high precision.
Supports 8 operators (eq, in, gt, lt, range, ne, exists, contains) and nested boolean trees (and, or, not).

[CRITICAL RULES]
- conditions accepts an array of leaf conditions (defaults to AND), or a boolean combination tree.
- range requires a 2-element array: [min, max].
- NEVER hallucinate field names. Call list_index_fields first if unsure.

[EXAMPLES]
1. Simple AND query:
[
  {"field": "cod-text-language", "op": "eq", "value": "zh"},
  {"field": "cod-text-lines", "op": "gt", "value": 50}
]

2. Nested OR/AND query:
{
  "and": [
    {"field": "cod-text-language", "op": "eq", "value": "zh"},
    {
      "or": [
        {"field": "cod-text-has-table", "op": "eq", "value": true},
        {"field": "cod-text-headings-count", "op": "gt", "value": 3}
      ]
    }
  ]
}
```

### 5.2 示例：`describe_file` Description 优化范式

```
Annotate an existing file with human/agent readable metadata, tags, and semantic attributes.

[FORBIDDEN]
- DO NOT pass cod-* fields in attributes! cod-* fields (e.g. cod-text-lines) are read-only facts computed by the deterministic engine. Attempting to modify them will cause immediate rejection.

[ACCEPTED ATTRIBUTES]
- llm-semantic-type: Must be one of [novel, game_guide, technical_doc, note, log, meme, illustration, photo, screenshot, code_artifact, data, other]
- llm-tone, llm-characters, llm-action, llm-style, llm-summary
- sp-llm-* (free-form custom attributes)
- Pass null for any attribute key to remove that key.
```

---

## 6. 实施演进步骤

1. **第一阶段（接口规范化与别名兼容，无破坏升级）**：
   - 统一公共参数提取（`GetFilePath` 等）。
   - 为 `modify_data_file` 挂载 `modify_file` 规范名，保留旧名作为 alias。
   - 为 `list_data_files` 挂载 `list_files` 规范名，保留旧名作为 alias。
   - 彻底修复 `execute_command` 与 `get_results`（更名为 `get_command_history`），剔除 `user_id` 入参，改为纯上下文上下文注入。
2. **第二阶段（补齐核心工具动作）**：
   - 实现 `get_file_info`（依赖现有 Manager + Store 批查/单查接口）。
   - 实现 `delete_file`（软删除接入）。
   - 实现 `patch_file`（精准文本替换与幂等重分析）。
3. **第三阶段（接入 MCP Prompts & Resources）**：
   - 实现 MCP Server 的 `AddResource`（`fields` 目录、检索契约）。
   - 实现 MCP Server 的 `AddPrompt`（`organize_workspace`、`audit_undescribed_files`）。
4. **第四阶段（Description 强化与多模态扩展）**：
   - 全面更新各 Tool 的 Description，嵌入 Few-Shot 与严格边界防御。
   - 实现 `get_image`（支持 MCP image block 原生看图回显，为多模态 Agent 看图补标签做准备）。
