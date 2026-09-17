// 文件：mcp-server-go/internal/mcp/prompts.go —— MCP Prompts 模版注册：工作区整理、未描述文件巡检与布尔检索向导
// 修改：2026-09-17（日期由 fresh-header.ps1 刷新）

package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/Reisentyann/Mabel-s-Tentacles/chaos-go/mabel"
)

func registerPrompts(s *server.MCPServer) {
	// 1. organize_workspace（工作区巡检与整理）
	s.AddPrompt(
		mcp.NewPrompt("organize_workspace",
			mcp.WithPromptDescription("引导智能体巡检工作区文件、分类归档、补充元数据并清理过期临时文件"),
			mcp.WithArgument("target_dir",
				mcp.ArgumentDescription("待整理的目标逻辑目录（默认根目录）"),
			),
		),
		func(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
			targetDir := req.Params.Arguments["target_dir"]
			if targetDir == "" {
				targetDir = "根目录"
			}
			text := fmt.Sprintf(`请执行工作区整理任务，目标目录为：%s。

建议执行步骤：
1. 调用 list_files（可使用关键词 q 过滤）获取当前目录下的文件列表与元数据概况；
2. 针对类型不清或未分类的文件，调用 get_file_info 获取其 90+ 确定性量化属性；
3. 对于无描述的文件，阅读其开头部分并调用 describe_file 填入清晰的 title、description 与 tags；
4. 针对散乱文件，使用 move_file 规划并归入语义目录（如 notes/、src/、docs/、archive/）；
5. 针对过期草稿或无用临时文件，经确认后使用 delete_file 软删除归档。`, targetDir)

			msg := mcp.NewPromptMessage(mcp.RoleUser, mcp.NewTextContent(text))
			return mcp.NewGetPromptResult("工作区文件整理与归档向导", []mcp.PromptMessage{msg}), nil
		},
	)

	// 2. audit_undescribed_files（未描述文件巡检与标注）
	s.AddPrompt(
		mcp.NewPrompt("audit_undescribed_files",
			mcp.WithPromptDescription("批量找出尚未添加描述和标签的文件，引导模型阅读其内容并调用 describe_file 补齐"),
			mcp.WithArgument("limit",
				mcp.ArgumentDescription("单次巡检处理上限（默认 10）"),
			),
		),
		func(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
			limit := req.Params.Arguments["limit"]
			if limit == "" {
				limit = "10"
			}
			text := fmt.Sprintf(`请巡检未描述文件并补充元数据，处理数量上限：%s。

标准执行节奏：
1. 调用 list_files 获取文件列表，筛选出 has_description=false 的文件；
2. 针对目标文件，调用 read_file 查看前部内容；
3. 提炼文件要点后，调用 describe_file 补齐以下字段：
   - title: 简明标题
   - description: 核心内容摘要
   - tags: 标签（逗号分隔）
   - attributes: 可选填入 llm-semantic-type（例如 technical_doc / note / log / code_artifact 等）
[注意] 严禁在 attributes 中传入 cod-* 字段，引擎只读区会直接拒绝。`, limit)

			msg := mcp.NewPromptMessage(mcp.RoleUser, mcp.NewTextContent(text))
			return mcp.NewGetPromptResult("未描述文件巡检与语义标注", []mcp.PromptMessage{msg}), nil
		},
	)

	// 3. advanced_search_wizard（结构化布尔检索向导）
	s.AddPrompt(
		mcp.NewPrompt("advanced_search_wizard",
			mcp.WithPromptDescription("根据用户的自然语言查询需求，指导生成结构化布尔条件表达式并执行精确检索"),
			mcp.WithArgument("user_query",
				mcp.RequiredArgument(),
				mcp.ArgumentDescription("用户的自然语言查询意图（如：找一下大于100行的Go代码或技术文档）"),
			),
		),
		func(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
			userQuery := req.Params.Arguments["user_query"]
			text := fmt.Sprintf(`用户意图检索文件：%s。

请按以下向导将用户意图转化为 search_files 条件树：
1. 查阅 mabel://catalog/fields 资源或调用 list_index_fields 确定对应字段名与分档（例如行数 cod-text-lines、代码语言 cod-code-lang、文档类型 llm-semantic-type）；
2. 构造 conditions 表达式：
   - 简单与：[{"field": "...", "op": "...", "value": ...}]
   - 组合树：{"and": [...], "or": [...]}
3. 调用 search_files 执行检索；
4. 若需要，对命中结果调用 read_file 验证内容，并将结果精炼汇总呈现给用户。`, userQuery)

			msg := mcp.NewPromptMessage(mcp.RoleUser, mcp.NewTextContent(text))
			return mcp.NewGetPromptResult("布尔语法检索生成向导", []mcp.PromptMessage{msg}), nil
		},
	)

	// 4. mabel_chat（梅贝尔人设与原作对话风格引导）
	s.AddPrompt(
		mcp.NewPrompt("mabel_chat",
			mcp.WithPromptDescription("引导模型以梅贝尔（Mabel）的原作对话风格与用户交流，自动抽取 3 段真实台词作为 Few-Shot 风格参考"),
			mcp.WithArgument("user_message",
				mcp.ArgumentDescription("用户发起的话题或聊天内容（可选）"),
			),
		),
		func(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
			userMsg := req.Params.Arguments["user_message"]
			if userMsg == "" {
				userMsg = "（用户进入了书房，向你打招呼）"
			}
			quotes := mabel.Lines(3)
			var sb strings.Builder
			for i, q := range quotes {
				sb.WriteString(fmt.Sprintf("\n【风格示例 %d】\n%s\n", i+1, q))
			}

			promptContent := fmt.Sprintf(`你现在扮演梅贝尔（梅酱）与用户交流。
梅贝尔是触手书房的守护者与文件管家（原型来自《Black Souls II》）。你的性格特征是：带着微醺的醉意，说话软糯娇憨、亲昵自然，偶尔会带有「♪」或「♡」等娇俏音符；在温柔黏人的外表下，隐藏着对克苏鲁深渊原罪的看透与彻底包容。

以下是刚刚从原作台词库中随机抽取的 3 段真实对白，请将其作为你的语气、用词习惯与神态的 Few-Shot 风格参考：%s
---
【当前用户话题】
%s

请完全沉浸在梅贝尔的口吻与性格中，参考上述台词的语气风格，给用户做出自然、鲜活且符合人设的回复。`, sb.String(), userMsg)

			msg := mcp.NewPromptMessage(mcp.RoleUser, mcp.NewTextContent(promptContent))
			return mcp.NewGetPromptResult("梅贝尔角色扮演与原作对话风格向导", []mcp.PromptMessage{msg}), nil
		},
	)
}
