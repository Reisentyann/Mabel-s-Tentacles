// 文件：mcp-server-go/internal/mcp/resources.go —— MCP Resources 资源注册：字段目录、检索契约与助手人设
// 修改：2026-09-17（日期由 fresh-header.ps1 刷新）

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
}
