// 文件：test/tools/mcpclient/main.go —— L4 端到端 MCP 客户端：write_file 造 3 文件 → analyze_file 核对事实产出（测试规则.md 第 6 节）
// 修改：2026-09-05（日期由 fresh-header.ps1 刷新）

// L4 部署链路测试（test/测试规则.md 第 6 节）：真服务 + 真 DB + 真 MCP 协议。
// 流程：SSE 握手 → write_file ×3（覆盖 text / text+code / markdown 结构三种路由）
// → analyze_file ×3 → 程序内断言 cod-* 事实符合字段字典口径。
// 退出码 0 = 全部通过；非 0 = 有断言失败（逐条打印 FAIL 行）。
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

var failures int

func fail(format string, args ...any) {
	failures++
	fmt.Printf("FAIL: "+format+"\n", args...)
}

func pass(format string, args ...any) {
	fmt.Printf("PASS: "+format+"\n", args...)
}

// callTool 调 MCP 工具，把 CallToolResult 的首个文本内容解析为 JSON map。
func callTool(ctx context.Context, c *client.Client, name string, args map[string]any) map[string]any {
	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args
	res, err := c.CallTool(ctx, req)
	if err != nil {
		fail("%s transport error: %v", name, err)
		return nil
	}
	if res.IsError {
		fail("%s returned tool-level error", name)
		return nil
	}
	if len(res.Content) == 0 {
		fail("%s returned no content", name)
		return nil
	}
	text, ok := res.Content[0].(mcp.TextContent)
	if !ok {
		fail("%s content[0] is %T, want TextContent", name, res.Content[0])
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(text.Text), &out); err != nil {
		fail("%s content is not JSON: %.120s", name, text.Text)
		return nil
	}
	if v, _ := out["success"].(bool); !v {
		fail("%s success=false: %.200s", name, text.Text)
		return nil
	}
	return out
}

func write(ctx context.Context, c *client.Client, path, content string) {
	callTool(ctx, c, "write_file", map[string]any{
		"file_path":   path,
		"content":     content,
		"title":       "e2e " + path,
		"description": "L4 端到端测试产物（mcpclient）",
		"tags":        "e2e,测试批次",
	})
}

// analyze 调 analyze_file 返回（families, attrs）。
func analyze(ctx context.Context, c *client.Client, path string) ([]string, map[string]any) {
	out := callTool(ctx, c, "analyze_file", map[string]any{"file_path": path})
	if out == nil {
		return nil, nil
	}
	fams, _ := out["families"].([]any)
	families := make([]string, 0, len(fams))
	for _, f := range fams {
		families = append(families, fmt.Sprint(f))
	}
	attrs, _ := out["attrs"].(map[string]any)
	return families, attrs
}

func has(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

const novelTxt = `# 第二章 深夜来电

深夜的书房里，梅贝尔放下了手中的茶杯。

「铃仙，那批文件整理得怎么样了？」她轻声问道。

「快好了，还剩最后三个箱子。」铃仙头也不抬地回答。

梅贝尔微微一笑。触手从她的背后展开，轻巧地卷起一摞摞文件，按封面的颜色分门别类地码放整齐。

「这样下去，天亮之前就能全部归档完毕。」

窗外，雨还在下。
`

const scriptPy = `#!/usr/bin/env python3
"""整理目录下的小文件。"""
import os
import sys

def scan_dir(root):
    # TODO: 过滤临时文件
    count = 0
    for name in os.listdir(root):
        if name.endswith(".tmp"):
            continue
        count += 1
    return count

def main():
    root = sys.argv[1] if len(sys.argv) > 1 else "."
    print(scan_dir(root))

if __name__ == "__main__":
    main()
`

const noteMd = `# 归档计划

## 待办

- [ ] 扫描旧目录
- [ ] 生成清单
- [x] 建立索引

## 文件类型统计

| 类型 | 数量 |
|---|---|
| 文本 | 42 |
| 图片 | 17 |

## 说明

按分区归档，游戏文件走 game/。
`

func main() {
	url := flag.String("url", "http://127.0.0.1:8080/sse", "MCP SSE 端点")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	c, err := client.NewSSEMCPClient(*url)
	if err != nil {
		fail("new sse client: %v", err)
		os.Exit(1)
	}
	if err := c.Start(ctx); err != nil {
		fail("sse start: %v", err)
		os.Exit(1)
	}
	defer c.Close()

	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{Name: "mabel-e2e", Version: "1.0"}
	if _, err := c.Initialize(ctx, initReq); err != nil {
		fail("initialize: %v", err)
		os.Exit(1)
	}
	pass("MCP 握手成功（%s）", *url)

	// ---- 第一步：write_file 造 3 个文件（T1 写路径自动跑全插件）----
	const novelPath = "测试批次/小说片段.txt"
	const scriptPath = "测试批次/工具脚本.py"
	const notePath = "测试批次/临时笔记.md"

	write(ctx, c, novelPath, novelTxt)
	write(ctx, c, scriptPath, scriptPy)
	write(ctx, c, notePath, noteMd)
	pass("write_file ×3 完成（text / text+code / markdown 三种路由）")

	// ---- 第二步：analyze_file 核对 T3 事实产出 ----
	fams, attrs := analyze(ctx, c, novelPath)
	if fams == nil {
		// 失败已记录
	} else {
		if !has(fams, "basic") || !has(fams, "text") {
			fail("%s families = %v, want basic+text", novelPath, fams)
		} else if has(fams, "image") || has(fams, "code") {
			fail("%s families = %v, text 不得命中 image/code", novelPath, fams)
		} else {
			pass("%s families = %v", novelPath, fams)
		}
		if attrs["cod-text-language"] != "zh" {
			fail("cod-text-language = %v, want zh", attrs["cod-text-language"])
		} else {
			pass("cod-text-language = zh")
		}
		if cjk, ok := attrs["cod-text-cjk-ratio"].(float64); !ok || cjk < 0.5 {
			fail("cod-text-cjk-ratio = %v, want >= 0.5", attrs["cod-text-cjk-ratio"])
		} else {
			pass("cod-text-cjk-ratio = %.2f", cjk)
		}
		if dr, ok := attrs["cod-text-dialog-ratio"].(float64); !ok || dr <= 0 {
			fail("cod-text-dialog-ratio = %v, want > 0（含「」对话）", attrs["cod-text-dialog-ratio"])
		} else {
			pass("cod-text-dialog-ratio = %.2f", dr)
		}
	}

	fams, attrs = analyze(ctx, c, scriptPath)
	if fams == nil {
		// 失败已记录
	} else {
		if !has(fams, "code") {
			fail("%s families = %v, want 含 code", scriptPath, fams)
		} else {
			pass("%s families = %v", scriptPath, fams)
		}
		if attrs["cod-code-lang"] != "python" {
			fail("cod-code-lang = %v, want python", attrs["cod-code-lang"])
		} else {
			pass("cod-code-lang = python")
		}
		if sh, _ := attrs["cod-text-shebang"].(string); sh != "#!/usr/bin/env python3" {
			fail("cod-text-shebang = %q, want #!/usr/bin/env python3", sh)
		} else {
			pass("cod-text-shebang 命中")
		}
		var imports []any
		if raw, ok := attrs["cod-code-imports"].([]any); ok {
			imports = raw
		}
		foundOS, foundSys := false, false
		for _, im := range imports {
			switch fmt.Sprint(im) {
			case "os":
				foundOS = true
			case "sys":
				foundSys = true
			}
		}
		if !foundOS || !foundSys {
			fail("cod-code-imports = %v, want 含 os 与 sys", imports)
		} else {
			pass("cod-code-imports 含 os / sys")
		}
		if td, ok := attrs["cod-code-todo-count"].(float64); !ok || td != 1 {
			fail("cod-code-todo-count = %v, want 1", attrs["cod-code-todo-count"])
		} else {
			pass("cod-code-todo-count = 1")
		}
		if fc, ok := attrs["cod-code-func-count"].(float64); !ok || fc < 2 {
			fail("cod-code-func-count = %v, want >= 2", attrs["cod-code-func-count"])
		} else {
			pass("cod-code-func-count = %.0f", fc)
		}
	}

	fams, attrs = analyze(ctx, c, notePath)
	if fams == nil {
		// 失败已记录
	} else {
		if !has(fams, "text") {
			fail("%s families = %v, want 含 text", notePath, fams)
		} else {
			pass("%s families = %v", notePath, fams)
		}
		if tr, ok := attrs["cod-text-table-rows"].(float64); !ok || tr < 3 {
			fail("cod-text-table-rows = %v, want >= 3", attrs["cod-text-table-rows"])
		} else {
			pass("cod-text-table-rows = %.0f", tr)
		}
		if li, ok := attrs["cod-text-list-items"].(float64); !ok || li != 3 {
			fail("cod-text-list-items = %v, want 3", attrs["cod-text-list-items"])
		} else {
			pass("cod-text-list-items = 3")
		}
		if cb, ok := attrs["cod-text-checkboxes"].(float64); !ok || cb != 3 {
			fail("cod-text-checkboxes = %v, want 3", attrs["cod-text-checkboxes"])
		} else {
			pass("cod-text-checkboxes = 3")
		}
	}

	fmt.Printf("\n==== MCP 端到端：%d 项失败 ====\n", failures)
	if failures > 0 {
		os.Exit(1)
	}
}
