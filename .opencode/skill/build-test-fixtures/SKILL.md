---
name: build-test-fixtures
description: 为 Mabel-s-Tentacles 文件描述系统构造测试夹具（fixtures）。当用户提到 构造测试文件、生成测试夹具、造测试数据、fixtures、测试素材、补充夹具 时触发。按 test/fixtures/清单.md 登记规格生成各类文本/图像/编码夹具，供 test/测试规则.md 的断言表核对。
---

# 构造测试夹具（文件描述系统）

目标：生成**真实形态**的测试文件，覆盖 describer-go 各插件的路由分支与字段产出。
一切以 `docs/元数据字段说明.md` 的字段定义为准；生成后必须登记清单并跑 L2 核对。

## 通用约定

- 输出位置：`test/fixtures/<类别>/<文件名>`，类别目录固定为：
  `小说/ 攻略/ 日志/ 数据/ 配置/ 代码/ 聊天/ 编码/ 图像/`
- 内容必须"像真的"：字段断言才有意义（例：日志必须行均匀、小说必须有对话与章节）
- 每个夹具 20-60 行为宜；不要塞入真实隐私信息
- 生成后：登记 `test/fixtures/清单.md`；在 `test/测试规则.md` 断言表追加行

## 文本类配方

| 类别 | 文件名 | 内容要点（决定断言） |
|---|---|---|
| 小说 | `深夜书架.txt` | ≥3 个中文章节标题（`第X章 `行）；对话行用中文引号包裹且占比明显；段落长（空行分块，块内>30字）；纯中文 |
| 攻略 | `狼人杀新手攻略.md` | markdown：1 个 `#` 大标题 + ≥2 个 `##` 小节；≥6 个 `-` 列表项；≥1 个 `\|a\|b\|` 表格（≥3 行）；无代码块 |
| 日志 | `server.log` | 每行 `YYYY-MM-DD HH:MM:SS LEVEL msg`；行长度均匀；含重复行（≥20%）；无空行；无中文标题 |
| 数据 | `players.csv` | 3 列逗号分隔、首行表头、≥8 数据行、**每行列数一致**；`players-broken.csv`：≥2 行列数错位 |
| 配置 | `app.json` | 合法 JSON 对象，顶层 ≥3 键；嵌套一层即可 |
| 代码 | `sample.go` | package + 2 个单行 import + 1 个 import 块（≥2 项）；**恰好** 1 个 TODO + 1 个 FIXME |
| 聊天 | `群聊记录.txt` | 每行 `时间 昵称: 消息`；含 `@某人`、≥1 个 emoji、短行（<40 字）、中英混 |

## 特殊编码配方（PowerShell 7）

GBK 夹具（.NET Core 需先注册 CodePages provider）：

```powershell
Add-Type -AssemblyName System.Text.Encoding.CodePages
[System.Text.Encoding]::RegisterProvider([System.Text.CodePagesEncodingProvider]::Instance)
$gbk = [System.Text.Encoding]::GetEncoding(936)
$text = "这是一份GBK编码的中文笔记。`n第二行内容。"
[IO.File]::WriteAllBytes("<repo>\test\fixtures\编码\gbk-note.txt", $gbk.GetBytes($text))
```

CRLF 夹具：

```powershell
"中文行一。`r`n中文行二。" | Set-Content -NoNewline -Encoding utf8NoBOM "<repo>\test\fixtures\编码\crlf-note.txt"
```

## 图像配方

- 纯色/分区图 + 带 tEXt 块的"AI 生成图"：仓库根执行 `go run ./test/tools/gen_png.go`
  （生成 `图像/warm.png` 与 `图像/ai-gen.png`，无需手动造）
- 新图像形态（像素画/线稿/灰度）暂缓：等 P1 `cod-image-pixel-art` 等字段落地后扩充配方

## 生成后自检（必做）

```powershell
Set-Location describer-go
go run ./cmd/verify ../test/fixtures
```

确认新夹具出现在报告里、family 路由符合预期（文本无 cod-image、图像无 cod-text），
然后按 `test/测试规则.md` 断言表核对字段值。

## 清单登记格式（test/fixtures/清单.md）

`| 路径 | 类型 | 覆盖断言 | 登记日期 |`，一行一夹具；修改夹具内容时更新登记日期。
