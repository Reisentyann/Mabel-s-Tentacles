// 文件：mcp-server-go/internal/mcp/resources.go —— MCP Resources 资源注册：字段目录、检索契约与助手人设
// 修改：2026-10-14（日期由 fresh-header.ps1 刷新）

package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/Reisentyann/Mabel-s-Tentacles/chaos-go/mabel"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/core"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/search"
)

func registerResources(s *server.MCPServer, orch *core.Orchestrator) {
	// 1. 字段目录资源：mabel://catalog/fields
	s.AddResource(
		mcp.NewResource(
			"mabel://catalog/fields",
			"索引字段目录",
			mcp.WithResourceDescription("当前系统所有可检索字段（cod-*、llm-*）、桶类型、取值基准（bench）与字段说明"),
			mcp.WithMIMEType("application/json"),
		),
		func(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
			var fields []map[string]any
			if orch != nil {
				cat := orch.IndexCatalog()
				for _, fi := range cat {
					item := map[string]any{
						"field":  fi.Field,
						"kind":   string(fi.Kind),
						"keys":   fi.Keys,
						"mounts": fi.Mounts,
					}
					if b, ok := search.Bench(fi.Field); ok {
						item["desc"] = b.Desc
						item["bench"] = b.Bench
					}
					if len(fi.Values) > 0 {
						item["values"] = fi.Values
					}
					if fi.Truncated {
						item["truncated"] = true
					}
					if fi.Min != nil {
						item["min"] = *fi.Min
					}
					if fi.Max != nil {
						item["max"] = *fi.Max
					}
					fields = append(fields, item)
				}
			}
			data, _ := json.MarshalIndent(fields, "", "  ")
			return []mcp.ResourceContents{
				mcp.TextResourceContents{
					URI:      "mabel://catalog/fields",
					MIMEType: "application/json",
					Text:     string(data),
				},
			}, nil
		},
	)

	// 2. 检索契约资源：mabel://guide/search-contract
	const searchGuide = `# Mabel-s-Tentacles 检索契约速查

## 查询标准节奏
1. 发现：使用 mabel://catalog/fields 资源或 list_index_fields 获知可用字段与分档
2. 圈定：调用 search_files 执行精确条件过滤（返回文件简表）
3. 取件：调用 read_file 读取正文

## conditions 表达式结构
- 扁平数组 = 全部 And（默认）：
  [{"field":"cod-text-language","op":"eq","value":"zh"}, {"field":"cod-text-lines","op":"gt","value":50}]

- 嵌套树 = and / or / not 自由组合：
  {
    "and": [
      {"field":"cod-text-language","op":"eq","value":"zh"},
      {
        "or": [
          {"field":"cod-text-has-table","op":"eq","value":true},
          {"field":"cod-text-headings-count","op":"gt","value":3}
        ]
      }
    ]
  }

## 8 种 op 操作符
- eq: 等值（enum / num / multi 桶含该值）
- in: 集合包含任一（value 为数组）
- gt / lt: 数值严格大于 / 小于
- range: 闭区间（value 为 [min, max] 双元数组）
- ne: 不等于
- exists: 键存在性（value 为 true 或 false）
- contains: 子串匹配
`

	s.AddResource(
		mcp.NewResource(
			"mabel://guide/search-contract",
			"检索契约速查",
			mcp.WithResourceDescription("检索语法速查与布尔组合树（And/Or/Not）使用说明"),
			mcp.WithMIMEType("text/markdown"),
		),
		func(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
			return []mcp.ResourceContents{
				mcp.TextResourceContents{
					URI:      "mabel://guide/search-contract",
					MIMEType: "text/markdown",
					Text:     searchGuide,
				},
			}, nil
		},
	)

	// 3. 梅贝尔人设与准则：mabel://persona/system（动态注入 3 段原作真实台词作为会话参考）
	s.AddResource(
		mcp.NewResource(
			"mabel://persona/system",
			"梅贝尔系统人设与准则",
			mcp.WithResourceDescription("梅贝尔的服务原则、工作流规范、安全边界以及动态抽取的原作对话风格样例"),
			mcp.WithMIMEType("text/markdown"),
		),
		func(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
			quotes := mabel.Lines(3)
			var qb strings.Builder
			for i, q := range quotes {
				qb.WriteString(fmt.Sprintf("\n【原作台词样例 %d】\n%s\n", i+1, q))
			}

			text := fmt.Sprintf(`# 梅贝尔（Mabel）系统人设与协作准则

梅贝尔是理性的智能文件管家助手（角色原型为《Black Souls II》中的梅贝尔/梅酱）。
核心定位：Agent 负责自然语言交互与发散创作，可参考梅贝尔原作的语言风格（软糯、娇憨、微醺、亲昵、带有「♪」和「♡」符号）；而在底层文件管理与量化分析时，则完全依赖严谨确定的理性系统。

## 原作会话风格参考（实时从台词库动态抽取）
以下 3 段抽取自原作真实台词，供你校准会话语气与人设表达：%s
## 协作铁律
1. **纯粹理性与确定性**：依赖确定性算法算出的近百个量化事实字段（行数、语言、行文指纹、色调等），不要凭空捏造文件位置与属性。
2. **安全第一**：严禁随意覆写未确认的文件。对于微创修改优先使用 patch_file，创建新文件使用 write_file，软删除使用 delete_file。
3. **结构化检索**：查询文件时遵循两步节奏：先查字段目录（或使用已缓存目录），再用 search_files 构造布尔表达式精准圈定文件，最后 read_file。
4. **元数据维护**：创建重要文件时带上 title、description 和 tags，保持工作区井井有条。
`, qb.String())

			return []mcp.ResourceContents{
				mcp.TextResourceContents{
					URI:      "mabel://persona/system",
					MIMEType: "text/markdown",
					Text:     text,
				},
			}, nil
		},
	)

	// 4. 操作与工具边界总览：mabel://guide/workflow
	const workflowGuide = `# Mabel-s-Tentacles 操作指南与工具边界矩阵

## 一、 我该调用哪个工具？（边界速查表）

| 你的意图 | 必须使用的工具 | 严禁混用的工具与边界说明 |
|---|---|---|
| **新建纯文本/代码文件** | ` + "`write_file`" + ` | 仅限新路径；已存在路径会报错拒绝。支持内联传 title/description/tags/attributes |
| **上传图片/二进制/压缩包** | ` + "`upload_file`" + ` | 传 ` + "`content_base64`" + `（支持 data:image/...;base64,...）或宿主机 ` + "`source_path`" + `（限管理员）。上传后自动提取图片宽高/主色调/EXIF 等 ` + "`cod-image-*`" + ` 属性并入索引 |
| **局部修改已有文本/代码** | ` + "`patch_file`" + ` | 基于唯一 ` + "`old_string -> new_string`" + ` 锚点替换，最安全、不丢上下文 |
| **向已有文件追加或整篇重写** | ` + "`modify_file`" + ` | ` + "`mode='append'`" + `（追加）或 ` + "`mode='overwrite'`" + `（覆盖）；不要用 ` + "`write_file`" + ` |
| **补充标题/标签/LLM语义字段** | ` + "`describe_file`" + ` | 写 ` + "`title/description/tags`" + ` 及 ` + "`llm-*`" + ` / ` + "`sp-llm-*`" + ` 属性；**严禁传 ` + "`cod-*`" + ` 只读属性** |
| **重新运行引擎提取物理特征** | ` + "`analyze_file`" + ` | 仅在文件被外部命令修改或引擎升级后使用；不会修改 ` + "`llm-*`" + ` 人工标注 |
| **按属性/条件精确查找文件** | ` + "`list_index_fields`" + ` → ` + "`search_files`" + ` | 先查一次目录获知字段名与 ` + "`bench`" + ` 分档，再用布尔树查询 |
| **浏览文件列表 / 查缺描述文件** | ` + "`list_files`" + ` | 分页浏览文件简表，通过 ` + "`has_description: false`" + ` 找出未标注文件 |
| **查看单文件完整 90+ 属性** | ` + "`get_file_info`" + ` | 已知文件路径时直查其全部 ` + "`cod-*`" + ` 与 ` + "`llm-*`" + ` 详情，无需走 ` + "`search_files`" + ` |
| **读取文件正文内容** | ` + "`read_file`" + ` | 仅用于读文本内容（1MB 截断）；支持传 ` + "`uuid`" + ` 或 ` + "`file_path`" + ` |
| **重命名 / 移动 / 复制 / 软删除** | ` + "`move_file`" + ` / ` + "`copy_file`" + ` / ` + "`delete_file`" + ` | 自动同步数据库、谱系（lineage）与内存索引 |

## 二、 图片上传与管理标准工作流（Image Upload & Management SOP）

1. **上传并内联标注**：
   调用 ` + "`upload_file`" + `：
   - ` + "`file_path`" + `: 例如 ` + "`\"illustrations/mabel-argon.png\"`" + `
   - ` + "`content_base64`" + `: 图片的 Base64 编码或 Data URL
   - ` + "`title`" + ` / ` + "`description`" + ` / ` + "`tags`" + `: 随上传一并填入
   - ` + "`attributes`" + `: 例如 ` + "`{\"llm-semantic-type\":\"illustration\",\"llm-characters\":[\"梅贝尔\"],\"llm-style\":\"赛博机能\"}`" + `
2. **自动量化分析（零干预）**：
   管理机落盘后，编排机自动调用描述机算出 ` + "`cod-image-width`" + `、` + "`cod-image-height`" + `、` + "`cod-image-aspect-ratio`" + `、` + "`cod-image-dominant-color`" + `、` + "`cod-image-brightness`" + ` 等事实并挂载到内存索引。
3. **验证与检索**：
   - 调用 ` + "`get_file_info`" + ` 查看刚上传图片的完整 ` + "`cod-image-*`" + ` 与 ` + "`llm-*`" + ` 属性及下载短链。
   - 调用 ` + "`search_files`" + ` 按色调、宽高比、像素量或角色标签随时检索图片。
`

	s.AddResource(
		mcp.NewResource(
			"mabel://guide/workflow",
			"操作指南与工具边界矩阵",
			mcp.WithResourceDescription("文件与图片上传、编辑、语义描述、确定性分析、布尔检索的标准工作流与工具边界表"),
			mcp.WithMIMEType("text/markdown"),
		),
		func(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
			return []mcp.ResourceContents{
				mcp.TextResourceContents{
					URI:      "mabel://guide/workflow",
					MIMEType: "text/markdown",
					Text:     workflowGuide,
				},
			}, nil
		},
	)
}
