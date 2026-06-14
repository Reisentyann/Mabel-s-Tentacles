# MCP 分段回复功能说明

## 功能目标

AstrBot 接入 QQ 官方 API 后，原版分段回复能力可能失效。本功能基于 MCP 工具调用会触发 AstrBot 再次回复的行为实现“分段回复”：模型调用 `segmented_reply` 工具后，服务器会把一段长回复拆成多段，先返回第一段；随后模型继续调用 `next_reply`，每次取出下一段并诱导 AstrBot 再发送一次。

工具仍会把所有分段写入 `data/segmented_replies/` 目录，便于调试和追踪；真正用于 QQ 多次发送的是 `segmented_reply` 与 `next_reply` 返回值中的 `message` 字段。

## 专用目录

分段回复相关代码和配置已经放在独立目录中：

```text
src/tools/segmented_reply/
├── __init__.py
├── config.py
├── config.json
└── core.py
```

其中 `src/tools/segmented_reply/config.json` 是本地配置文件，你可以直接手动修改。

## 本地配置文件

配置文件路径：`src/tools/segmented_reply/config.json`

默认配置：

```json
{
  "force_segmented_reply": true,
  "interval_seconds": 1.0,
  "segment_length_threshold": 500,
  "max_segments": 20,
  "output_dir": "segmented_replies",
  "split_words": ["---", "\n---\n", "【分段】", "[分段]"],
  "content_filter": {
    "blocked_words": [],
    "replace_rules": {
      "敏感词示例": "***"
    }
  }
}
```

配置项说明：

| 配置项                     | 类型      | 说明                                               |
| -------------------------- | --------- | -------------------------------------------------- |
| `force_segmented_reply`    | `boolean` | 是否强制机器人优先使用分段回复工具                 |
| `interval_seconds`         | `number`  | 每段写入之间的等待秒数，最大会被限制为 `10` 秒     |
| `segment_length_threshold` | `number`  | 单段字数阈值，超过后自动按标点或空格拆分           |
| `max_segments`             | `number`  | 单次最多生成的分段数量，避免误触发刷屏             |
| `output_dir`               | `string`  | 输出目录，实际路径位于 `data/` 下                  |
| `split_words`              | `array`   | 分段词列表，内容中出现这些词时会优先作为手动分段点 |
| `content_filter`           | `object`  | 内容过滤器，支持阻断词和替换规则                   |

内容过滤器说明：

- `blocked_words`：只要回复内容包含列表内任意词，就拒绝生成分段文件。
- `replace_rules`：按字符串直接替换，例如把敏感词替换成 `***`。

## 新增工具

工具名：`segmented_reply`

参数：

| 参数         | 类型  | 默认值    | 说明                                       |
| ------------ | ----- | --------- | ------------------------------------------ |
| `content`    | `str` | 必填      | 需要分段发送的完整回复内容                 |
| `session_id` | `str` | `default` | 会话标识，用于生成文件名，便于区分不同会话 |

分段词、字数阈值、间隔时间和内容过滤器都从本地配置文件读取，不再需要每次调用工具时传入。调用成功后，AstrBot 应把返回的 `message` 作为本次可见回复；如果 `has_more` 为 `true`，继续调用 `next_reply`。

返回字段：

| 字段                    | 说明                                    |
| ----------------------- | --------------------------------------- |
| `success`               | 是否成功生成分段文件                    |
| `message`               | 执行结果说明                            |
| `reply`                 | 与 `message` 相同，表示本次应发送的文本 |
| `segment_count`         | 分段数量                                |
| `remaining_count`       | 剩余未发送分段数量                      |
| `has_more`              | 是否还有后续分段                        |
| `next_tool`             | 有后续分段时为 `next_reply`             |
| `interval_seconds`      | 本次使用的分段间隔秒数                  |
| `force_segmented_reply` | 本次是否启用强制分段回复配置            |
| `config_path`           | 本次读取的配置文件路径                  |
| `queue_path`            | 后续分段队列文件路径                    |
| `files`                 | 生成的文件路径，路径相对于 `data/` 目录 |
| `segments`              | 实际拆分出的文本片段                    |

工具名：`next_reply`

参数：

| 参数         | 类型  | 默认值    | 说明                                            |
| ------------ | ----- | --------- | ----------------------------------------------- |
| `session_id` | `str` | `default` | 会话标识，必须与 `segmented_reply` 使用同一个值 |

返回字段：

| 字段              | 说明                          |
| ----------------- | ----------------------------- |
| `success`         | 是否成功取出下一段            |
| `message`         | 本次应发送的文本              |
| `reply`           | 与 `message` 相同             |
| `segment_index`   | 当前分段序号                  |
| `segment_count`   | 总分段数量                    |
| `remaining_count` | 剩余未发送分段数量            |
| `has_more`        | 是否还有后续分段              |
| `next_tool`       | 有后续分段时仍为 `next_reply` |

## 使用方式

### 手动分段

在回复内容中使用 `---` 分隔每一段：

```text
第一段内容
---
第二段内容
---
第三段内容
```

调用示例：

```json
{
  "content": "第一段内容\n---\n第二段内容\n---\n第三段内容",
  "session_id": "qq_123456"
}
```

调用后如果返回 `has_more: true`，继续调用：

```json
{
  "session_id": "qq_123456"
}
```

每调用一次 `next_reply`，AstrBot 就应发送返回的 `message`，直到 `has_more` 为 `false`。

### 自动分段

如果内容中没有分段词，工具会按 `segment_length_threshold` 自动拆分。自动拆分会优先尝试在换行、中文句号/问号/感叹号、英文标点或空格处断开，尽量避免从句子中间截断。

```json
{
  "content": "这里是一段很长的回复……",
  "session_id": "qq_123456"
}
```

## 文件输出规则

默认生成文件目录：

```text
data/segmented_replies/
```

文件名格式：

```text
YYYYMMDD_HHMMSS_microseconds_sessionid_序号.txt
```

例如：

```text
data/segmented_replies/20260614_225100_123456_qq_123456_01.txt
data/segmented_replies/20260614_225100_123456_qq_123456_02.txt
```

## 安全限制

- 单次 `content` 最大为 `5MB`。
- 单次最多生成段数由 `max_segments` 控制，默认 `20` 段。
- `segment_length_threshold` 必须在 `50` 到 `5000` 之间。
- `interval_seconds` 最大会被限制为 `10` 秒，避免工具长时间阻塞。
- `session_id` 只会保留字母、数字、下划线和短横线，其他字符会被替换为下划线。
- 所有输出都限制在 `data/` 目录下。

## 推荐提示词

可以在 AstrBot 或模型系统提示词中加入类似规则：

```text
当需要连续发送多条消息时，优先调用 MCP 工具 segmented_reply。
调用 segmented_reply 后，必须把返回的 message 作为当前回复发送。
如果返回 has_more=true，必须继续调用 next_reply，并把每次返回的 message 作为新回复发送。
重复调用 next_reply 直到 has_more=false。
如果用户明确要求“分段”“分多条”“别一次发完”，请使用配置文件 split_words 中的分段词分隔每一段。
每段应保持语义完整，不要把一句话硬拆开。
```

## 注意事项

这个功能会在 MCP 服务器侧拆分文本、写入分段文件，并维护 `next_reply` 队列。最终能否表现为 QQ 中的多次回复，取决于 AstrBot 或上层 MCP 调用链是否按 `has_more` 继续调用 `next_reply`，并把每次工具结果中的 `message` 作为可见消息发送。
