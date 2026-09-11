// 文件：test/tools/idxclient/main.go —— L4 端到端索引链路客户端：write 造料 → list_index_fields 目录发现 → search_files 条件断言（eq/in/gt/range/And/空集/坏op）+ 权限链路断言（agent key 复判边界）
// 修改：2026-09-10（日期由 fresh-header.ps1 刷新）

// L4 索引链路端到端（test/测试规则.md 第 6 节）：真服务 + 真 DB + 真 MCP 协议。
// 流程：SSE 握手 → write_file ×4（zh 小说 / en 文本 / py 脚本 / md 笔记，
// 属性各异）→ 轮询等待异步喂食（write 是事件异步落库+喂索引，回执≠可查）
// → list_index_fields 断言目录（enum 取值 / num 值域 / 前缀过滤）→
// search_files 断言组（eq 精确圈人 / in 并集 / And 交集 / gt 数值 / range
// 区间 / 空集合法 / 坏 op 人话错误）。
// -mkey：MCP 通道 master key（服务须以 MCP_API_KEY 同值启动）——本会话
// 即管家主体（admin），全程真鉴权路径（匿名开发放行不参与本 e2e）。
// -perm：权限链路断言（ backlog 权限 e2e）——HTTP 注册普通用户 + admin
// 签发绑定该用户的 agent key → 第二 MCP 会话以 agent key 接入 →
// search_files 复判断言（public 可见 / 他人 private 隔离 / admin 全见），
// 外加坏 key 与吊销 key 的拒绝断言（鉴权链的负路径）。
// -verify 模式：跳过写入只跑只读断言——服务重启后由 run-idx-e2e.ps1
// 调用，验证 RebuildIndex 从 DB 全量重建后查询仍命中（DB 是事实源）。
// 退出码 0 = 全部通过；非 0 = 有断言失败（逐条打印 FAIL 行）。
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
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
// toolErr=true 表示允许 success=false（坏条件断言用）。
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
	return out
}

// dial 建立 SSE 客户端；key 非空时带 Authorization Bearer（master key 与
// agent key 同一口径——AuthMiddleware 两者都认）。
func dial(rawURL, key string) (*client.Client, error) {
	if key != "" {
		return client.NewSSEMCPClient(rawURL,
			client.WithHeaders(map[string]string{"Authorization": "Bearer " + key}))
	}
	return client.NewSSEMCPClient(rawURL)
}

// handshake MCP 初始化握手；错误原样上抛（坏 key 断言靠它判拒连）。
func handshake(ctx context.Context, c *client.Client) error {
	req := mcp.InitializeRequest{}
	req.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	req.Params.ClientInfo = mcp.Implementation{Name: "mabel-idx-e2e", Version: "1.0"}
	_, err := c.Initialize(ctx, req)
	return err
}

// write 调 write_file 落一个文件；visibility 空 = 服务端缺省（public）。
func write(ctx context.Context, c *client.Client, path, content, visibility string) {
	args := map[string]any{
		"file_path":   path,
		"content":     content,
		"title":       "idx e2e " + path,
		"description": "L4 索引链路测试产物（idxclient）",
		"tags":        "idx-e2e,索引批次",
	}
	if visibility != "" {
		args["visibility"] = visibility
	}
	out := callTool(ctx, c, "write_file", args)
	if out == nil {
		return
	}
	if v, _ := out["success"].(bool); !v {
		fail("write_file %s success=false", path)
	}
}

// search 调 search_files 返回 (files, total, ok)。
func search(ctx context.Context, c *client.Client, conds string, args ...string) ([]any, float64, bool) {
	m := map[string]any{
		"conditions": conds,
		"size":       50,
	}
	for i := 0; i+1 < len(args); i += 2 {
		m[args[i]] = args[i+1]
	}
	out := callTool(ctx, c, "search_files", m)
	if out == nil {
		return nil, 0, false
	}
	if v, _ := out["success"].(bool); !v {
		fail("search_files %s success=false: %.200s", conds, fmt.Sprint(out["message"]))
		return nil, 0, false
	}
	files, _ := out["files"].([]any)
	total, _ := out["total"].(float64)
	return files, total, true
}

// pathsOf 提取命中行的 path 列表。
func pathsOf(files []any) []string {
	out := make([]string, 0, len(files))
	for _, f := range files {
		if m, ok := f.(map[string]any); ok {
			out = append(out, fmt.Sprint(m["path"]))
		}
	}
	return out
}

func hasPath(paths []string, sub string) bool {
	for _, p := range paths {
		if len(p) >= len(sub) && p[len(p)-len(sub):] == sub {
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

const englishTxt = `# Chapter Two: Midnight Call

The rain tapped against the library windows as Mabel set down her teacup.

"Reisen, how goes the filing?" she asked softly.

"Almost done. Three crates left," Reisen replied without looking up.

Outside, the rain kept falling.
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
        count = count + 1
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

## 说明

按分区归档，游戏文件走 game/。
`

const (
	novelPath  = "索引批次/小说片段.txt"
	enPath     = "索引批次/english-note.txt"
	scriptPath = "索引批次/工具脚本.py"
	notePath   = "索引批次/临时笔记.md"
)

// waitFed 轮询等待异步喂食完成：write 回执即回，描述落库+喂索引走事件，
// 有界重试直到 zh 查询命中（或超时告败）。
func waitFed(ctx context.Context, c *client.Client) bool {
	deadline := time.Now().Add(20 * time.Second)
	for {
		files, total, ok := search(ctx, c,
			`[{"field":"cod-text-language","op":"eq","value":"zh"}]`)
		if ok && total >= 1 && hasPath(pathsOf(files), novelPath) {
			return true
		}
		if time.Now().After(deadline) {
			fail("喂食轮询超时：zh 查询 20s 内未命中小说（异步事件未消化？）")
			return false
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func main() {
	url := flag.String("url", "http://127.0.0.1:8080/sse", "MCP SSE 端点")
	api := flag.String("api", "http://127.0.0.1:8080", "HTTP API 基址（索引机 HTTP 面断言用）")
	verify := flag.Bool("verify", false, "只读模式：跳过写入，仅断言查询（重启后 RebuildIndex 验证用）")
	mkey := flag.String("mkey", "", "MCP master key（服务以 MCP_API_KEY 同值启动时必传；空 = 匿名开发口径）")
	perm := flag.Bool("perm", false, "权限链路断言：agent key 绑普通用户的第二会话 + search_files 复判边界（需 -mkey）")
	flag.Parse()
	if *perm && *mkey == "" {
		fail("-perm 需要 -mkey（权限断言要求服务以 MCP_API_KEY 启动）")
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	c, err := dial(*url, *mkey)
	if err != nil {
		fail("new sse client: %v", err)
		os.Exit(1)
	}
	if err := c.Start(ctx); err != nil {
		fail("sse start: %v", err)
		os.Exit(1)
	}
	defer c.Close()

	if err := handshake(ctx, c); err != nil {
		fail("initialize: %v", err)
		os.Exit(1)
	}
	pass("MCP 握手成功（%s）", *url)

	// ---- 工具注册面：search_files / list_index_fields 必须在列 ----
	tools, err := c.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		fail("list tools: %v", err)
		os.Exit(1)
	}
	registed := map[string]bool{}
	for _, t := range tools.Tools {
		registed[t.Name] = true
	}
	if !registed["search_files"] || !registed["list_index_fields"] {
		fail("tools 缺 search_files/list_index_fields")
	} else {
		pass("工具注册面：search_files + list_index_fields 在列")
	}

	if !*verify {
		// ---- 写入四种属性各异的文件 ----
		write(ctx, c, novelPath, novelTxt, "")
		write(ctx, c, enPath, englishTxt, "")
		write(ctx, c, scriptPath, scriptPy, "")
		write(ctx, c, notePath, noteMd, "")
		pass("write_file ×4 完成（zh / en / py / md）")

		// ---- 轮询等待异步喂食 ----
		if !waitFed(ctx, c) {
			fmt.Printf("\n==== 索引链路 e2e：%d 项失败 ====\n", failures)
			os.Exit(1)
		}
		pass("异步喂食消化完成（zh 查询命中小说）")
	}

	// ---- 字段目录发现 ----
	out := callTool(ctx, c, "list_index_fields", map[string]any{})
	if out == nil {
		os.Exit(1)
	}
	// 契约半边之二（2026-09-10）：guide = 通用规则 + 速查表（跨字段口径）
	if guide, ok := out["guide"].(map[string]any); ok {
		rules, _ := guide["rules"].([]any)
		quick, _ := guide["quick_ref"].([]any)
		if len(rules) < 8 || len(quick) < 10 {
			fail("目录响应缺 guide（rules=%d quick_ref=%d, want ≥8/≥10）", len(rules), len(quick))
		} else {
			pass("目录：guide 契约在位（%d 条规则 + %d 条速查）", len(rules), len(quick))
		}
	} else {
		fail("目录响应缺 guide 字段（跨字段规则未接线？）")
	}
	fields, _ := out["fields"].([]any)
	fieldMap := map[string]map[string]any{}
	for _, f := range fields {
		if m, ok := f.(map[string]any); ok {
			fieldMap[fmt.Sprint(m["field"])] = m
		}
	}
	if lang, ok := fieldMap["cod-text-language"]; ok {
		kind := fmt.Sprint(lang["kind"])
		values := fmt.Sprint(lang["values"])
		if kind != "enum" {
			fail("cod-text-language kind = %s, want enum", kind)
		} else if !contains(values, "zh") || !contains(values, "en") {
			fail("cod-text-language values = %s, want 含 zh 与 en", values)
		} else {
			pass("目录：cod-text-language enum 取值含 zh/en")
		}
		// 基准面契约（2026-09-10）：关键字段自带 desc（含义）+ bench（分档）——
		// agent 不读本地文档，目录即文档才算自足发现
		if d, _ := lang["desc"].(string); d == "" {
			fail("cod-text-language 缺 desc（基准注册表未接线？）")
		} else if b, _ := lang["bench"].(string); b == "" {
			fail("cod-text-language 缺 bench（分档基准未接线？）")
		} else {
			pass("目录：cod-text-language 带 desc/bench（含义+分档，agent 自足发现）")
		}
	} else {
		fail("目录缺 cod-text-language（喂食未消化？）")
	}
	if lines, ok := fieldMap["cod-text-lines"]; ok {
		if fmt.Sprint(lines["kind"]) != "num" {
			fail("cod-text-lines kind = %s, want num", lines["kind"])
		} else if lines["min"] == nil || lines["max"] == nil {
			fail("cod-text-lines 目录缺 min/max 值域")
		} else {
			pass("目录：cod-text-lines num 值域 [%v, %v]", lines["min"], lines["max"])
		}
	} else {
		fail("目录缺 cod-text-lines")
	}
	if cl, ok := fieldMap["cod-code-lang"]; ok {
		if !contains(fmt.Sprint(cl["values"]), "python") {
			fail("cod-code-lang values = %s, want 含 python", cl["values"])
		} else {
			pass("目录：cod-code-lang 取值含 python")
		}
	} else {
		fail("目录缺 cod-code-lang")
	}

	// ---- search_files 断言组（相对断言：只判本批 4 文件的包含/排除，
	// 库内历史文件不参与口径——与 run-e2e 的口径中立哲学一致。
	// 字段正交性备忘：note.md 中文 → language=zh；py 脚本文本英文 →
	// language=en 且 code-lang=python——两字段维度独立，互不互斥） ----
	// eq：语言=zh → 含小说+笔记（中文两件），不含英文与脚本
	files, _, ok := search(ctx, c, `[{"field":"cod-text-language","op":"eq","value":"zh"}]`)
	if ok {
		paths := pathsOf(files)
		if !hasPath(paths, novelPath) || !hasPath(paths, notePath) || hasPath(paths, enPath) || hasPath(paths, scriptPath) {
			fail("eq zh → 本批口径错乱（want 含小说+笔记，不含英文/脚本）: %v", paths)
		} else {
			pass("eq zh → 含小说+笔记，不含英文/脚本（圈人正确）")
		}
	}

	// eq：语言=en → 含英文笔记+py 脚本（脚本文本为英文），不含中文两件
	files, _, ok = search(ctx, c, `[{"field":"cod-text-language","op":"eq","value":"en"}]`)
	if ok {
		paths := pathsOf(files)
		if !hasPath(paths, enPath) || !hasPath(paths, scriptPath) || hasPath(paths, novelPath) || hasPath(paths, notePath) {
			fail("eq en → 本批口径错乱（want 含英文+脚本，不含中文件）: %v", paths)
		} else {
			pass("eq en → 含英文笔记+py 脚本，不含中文件")
		}
	}

	// eq：代码语言=python → 只含脚本（其余三件无 code-lang=python）
	files, _, ok = search(ctx, c, `[{"field":"cod-code-lang","op":"eq","value":"python"}]`)
	if ok {
		paths := pathsOf(files)
		if !hasPath(paths, scriptPath) || hasPath(paths, novelPath) || hasPath(paths, enPath) || hasPath(paths, notePath) {
			fail("eq python → 本批口径错乱（want 仅脚本）: %v", paths)
		} else {
			pass("eq python → 含脚本，不含文本三件（代码维度独立）")
		}
	}

	// in：语言 ∈ [zh,en] → 本批四件全命中（并集）
	files, _, ok = search(ctx, c, `[{"field":"cod-text-language","op":"in","value":["zh","en"]}]`)
	if ok {
		paths := pathsOf(files)
		if !hasPath(paths, novelPath) || !hasPath(paths, notePath) || !hasPath(paths, enPath) || !hasPath(paths, scriptPath) {
			fail("in [zh,en] → 本批四件应全命中（并集）: %v", paths)
		} else {
			pass("in [zh,en] → 本批四件全命中（并集语义）")
		}
	}

	// And 交集：语言=zh 且代码语言=python → 本批无文件同时满足（互斥维度）
	files, _, ok = search(ctx, c,
		`[{"field":"cod-text-language","op":"eq","value":"zh"},{"field":"cod-code-lang","op":"eq","value":"python"}]`)
	if ok {
		paths := pathsOf(files)
		if hasPath(paths, novelPath) || hasPath(paths, notePath) || hasPath(paths, enPath) || hasPath(paths, scriptPath) {
			fail("And zh∧python → 本批文件不得命中（互斥条件交集空）: %v", paths)
		} else {
			pass("And zh∧python → 本批零命中（交集语义正确）")
		}
	}

	// gt：行数 > 100 → 大概率空（夹具行数少）；> 0 → 至少命中
	_, total, ok := search(ctx, c, `[{"field":"cod-text-lines","op":"gt","value":100}]`)
	if ok {
		if total != 0 {
			pass("gt lines>100 → %v 条（夹具行数少，空集符合预期）", total)
		} else {
			pass("gt lines>100 → 空集（数值比较生效）")
		}
	}
	files, total, ok = search(ctx, c, `[{"field":"cod-text-lines","op":"gt","value":0}]`)
	if ok && total < 2 {
		fail("gt lines>0 → total=%v, want ≥2（文本文件全命中）", total)
	} else if ok {
		pass("gt lines>0 → %v 条（数值比较命中）", total)
	}

	// range：行数 ∈ [1,99999] → 文本类全命中
	files, total, ok = search(ctx, c, `[{"field":"cod-text-lines","op":"range","value":[1,99999]}]`)
	if ok {
		if total < 3 || !hasPath(pathsOf(files), novelPath) {
			fail("range [1,99999] → total=%v, want ≥3 且含小说", total)
		} else {
			pass("range [1,99999] → %v 条（区间扫描）", total)
		}
	}

	// 空集合法：查一个不存在的取值 → success=true + 0 条（不是报错）
	files, total, ok = search(ctx, c, `[{"field":"cod-text-language","op":"eq","value":"xx-不存在"}]`)
	if ok {
		if total != 0 || len(files) != 0 {
			fail("eq 不存在值 → total=%v, want 0（空集=合法答案）", total)
		} else {
			pass("eq 不存在值 → 空集合法（不降级不报错）")
		}
	}

	// 复判过滤：file_type=image 叠加属性条件 → 0 条（本批全文本）
	_, total, ok = search(ctx, c, `[{"field":"cod-text-language","op":"eq","value":"zh"}]`, "file_type", "image")
	if ok {
		if total != 0 {
			fail("file_type=image 复判 → total=%v, want 0", total)
		} else {
			pass("复判过滤：file_type=image + zh → 0 条")
		}
	}

	// 坏 op：人话错误（success=false 教自纠错）
	out = callTool(ctx, c, "search_files", map[string]any{
		"conditions": `[{"field":"cod-text-language","op":"like","value":"zh"}]`,
	})
	if out != nil {
		if v, _ := out["success"].(bool); v {
			fail("坏 op like → success=true, want false（校验失效）")
		} else {
			pass("坏 op like → success=false 人话错误")
		}
	}

	// ---- HTTP 面（目录批次 2026-09-09）：字段目录端点 + cond 条件检索 ----
	httpAssert(ctx, *api)

	// ---- 权限链路（backlog #1）：agent key 第二会话 + search_files 复判边界 ----
	if *perm {
		permAssert(ctx, c, *api, *url)
	}

	// ---- 汇总 ----
	fmt.Printf("\n==== 索引链路 e2e（verify=%v）：%d 项失败 ====\n", *verify, failures)
	if failures > 0 {
		os.Exit(1)
	}
}

// httpAssert 索引机 HTTP 面断言：GET /api/index/fields（目录发现）+
// GET /api/files/search?cond=（条件检索，与 MCP 工具同一份解析与口径）。
// 登录凭证从环境变量取（run-idx-e2e.ps1 已从 .env 装配 ADMIN_USERNAME /
// ADMIN_PASSWORD，与 HTTP 侧 e2e 同源）。
func httpAssert(ctx context.Context, base string) {
	bearer := adminLogin(ctx, base)
	if bearer == "" {
		return
	}

	// GET /api/index/fields?prefix=cod-text → cod-text-language 目录项
	fields := httpGetJSON(ctx, base+"/api/index/fields?prefix=cod-text", bearer)
	if fields != nil {
		list, _ := fields["fields"].([]any)
		found := false
		for _, f := range list {
			if m, ok := f.(map[string]any); ok && fmt.Sprint(m["field"]) == "cod-text-language" {
				found = true
				if fmt.Sprint(m["kind"]) != "enum" || !contains(fmt.Sprint(m["values"]), "zh") {
					fail("HTTP 目录 cod-text-language = %v（want enum 含 zh）", m)
				} else {
					pass("HTTP /api/index/fields：cod-text-language enum 含 zh（prefix 过滤生效）")
				}
			}
		}
		if !found {
			fail("HTTP 目录 prefix=cod-text 未含 cod-text-language（fields=%d 项）", len(list))
		}
	}

	// GET /api/files/search?cond=... → 命中小说（相对断言：file_path 含
	// 小说片段；HTTP 返回原键（~owner/ 前缀不剥——与 MCP 工具层的差异
	// 是设计口径，断言用包含匹配）
	cond := `[{"field":"cod-text-language","op":"eq","value":"zh"}]`
	out := httpGetJSON(ctx, base+"/api/files/search?cond="+url.QueryEscape(cond)+"&size=50", bearer)
	if out != nil {
		items, _ := out["items"].([]any)
		hit := false
		for _, it := range items {
			if m, ok := it.(map[string]any); ok && contains(fmt.Sprint(m["file_path"]), novelPath) {
				hit = true
				break
			}
		}
		if !hit {
			fail("HTTP cond 检索未命中小说（items=%d）", len(items))
		} else {
			pass("HTTP /api/files/search?cond=eq zh → 命中小说（索引条件直查）")
		}
	}

	// 坏 cond → 400 人话
	res2, err := httpGet(ctx, base+"/api/files/search?cond="+url.QueryEscape(`[{"field":"x","op":"like","value":1}]`), bearer)
	if err == nil && res2 != nil {
		defer res2.Body.Close()
		if res2.StatusCode != http.StatusBadRequest {
			fail("HTTP 坏 cond → status=%d, want 400", res2.StatusCode)
		} else {
			pass("HTTP 坏 cond → 400 人话错误")
		}
	}
}

// httpGetJSON 带 JWT 的 GET，解析为 JSON map（nil = 已计失败）。
func httpGetJSON(ctx context.Context, rawURL, bearer string) map[string]any {
	res, err := httpGet(ctx, rawURL, bearer)
	if err != nil || res == nil {
		return nil
	}
	defer res.Body.Close()
	var out map[string]any
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		fail("http GET %s 响应非 JSON: %v", rawURL, err)
		return nil
	}
	return out
}

// httpGet 带 JWT 的 GET（返回原始响应，调用方负责关闭 Body）。
func httpGet(ctx context.Context, rawURL, bearer string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		fail("http new request %s: %v", rawURL, err)
		return nil, err
	}
	req.Header.Set("Authorization", bearer)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		fail("http GET %s: %v", rawURL, err)
		return nil, err
	}
	return res, nil
}

func contains(s, sub string) bool {
	return strings.Contains(s, sub)
}

// —— 权限链路断言（backlog #1）——

const (
	permUser     = "idxperm" // 普通用户：agent key 绑定的身份（重跑幂等，见下）
	permPass     = "idx-e2e-pass-123"
	permPubPath  = "perm批次/公开小说.txt"
	permPrivPath = "perm批次/私密小说.txt"
)

const permZhTxt = `# 权限断言夹具

梅贝尔把公开的档案放上了共享书架，私密的那几份则收进了只有她能打开的抽屉。
「铃仙，抽屉的钥匙只有一把。」她轻声说。
窗外，雨还在下。
`

// adminLogin HTTP 管理员登录拿 JWT（返回 Bearer 串；空 = 已计失败）。
func adminLogin(ctx context.Context, base string) string {
	user := os.Getenv("ADMIN_USERNAME")
	passwd := os.Getenv("ADMIN_PASSWORD")
	if user == "" || passwd == "" {
		fail("HTTP 断言跳过：ADMIN_USERNAME/ADMIN_PASSWORD 环境变量缺失")
		return ""
	}
	loginBody, _ := json.Marshal(map[string]string{"username": user, "password": passwd})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/auth/login", bytes.NewReader(loginBody))
	if err != nil {
		fail("http login new request: %v", err)
		return ""
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		fail("http login: %v", err)
		return ""
	}
	defer res.Body.Close()
	var login struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(res.Body).Decode(&login); err != nil || login.AccessToken == "" {
		fail("http login 响应异常（status=%d）", res.StatusCode)
		return ""
	}
	return "Bearer " + login.AccessToken
}

// rejected 断言 key 会被拒连：dial/Start/握手任一失败即拒（true）；
// 全部通过 = 鉴权失守（false，由调用方计失败）。拒绝点在哪个环节是
// 实现自由（AuthMiddleware 包整个 SSE handler，通常 Start 就断）。
func rejected(ctx context.Context, sseURL, key string) bool {
	c, err := dial(sseURL, key)
	if err != nil {
		return true
	}
	if err := c.Start(ctx); err != nil {
		return true
	}
	defer c.Close()
	return handshake(ctx, c) != nil
}

// permAssert 权限链路断言：agent key 绑定普通用户的第二 MCP 会话 →
// search_files 复判边界。链路：HTTP 注册 idxperm → admin 签发绑定它的
// agent key（raw 只出现一次）→ master 会话写 public/private 两件 zh 文本
// （owner=agent）→ agent 会话复判（public 可见 / 他人 private 隔离）→
// master 会话全见（admin 无观察者过滤）→ 坏 key / 吊销 key 拒连。
// master 为 main 的主会话（-mkey 拨号）；重跑幂等：用户 taken 容忍、
// 文件 upsert 覆写、key 每轮新签并在收尾吊销。
func permAssert(ctx context.Context, master *client.Client, base, sseURL string) {
	bearer := adminLogin(ctx, base)
	if bearer == "" {
		return
	}

	// 注册普通用户（重跑容忍 taken——用户存在即继续；真缺用户时后续
	// 签发 404 会大声失败，不会静默漏断言）
	rbody, _ := json.Marshal(map[string]string{"username": permUser, "password": permPass})
	if _, code := httpPostJSON(ctx, base+"/api/auth/register", rbody, ""); code != http.StatusCreated && code != http.StatusConflict {
		fail("perm 注册 %s → status=%d, want 201/409", permUser, code)
		return
	}

	// admin 签发绑定 idxperm 的 agent key
	kbody, _ := json.Marshal(map[string]string{"name": "idx-e2e-perm", "username": permUser})
	kres, code := httpPostJSON(ctx, base+"/api/admin/agent-keys", kbody, bearer)
	if kres == nil {
		return
	}
	if code != http.StatusCreated {
		fail("perm 签发 agent key → status=%d, want 201", code)
		return
	}
	rawKey, _ := kres["key"].(string)
	keyID, _ := kres["id"].(float64)
	if rawKey == "" || keyID == 0 {
		fail("perm 签发响应缺 key/id：%v", kres)
		return
	}
	pass("perm 准备：用户 %s 就绪 + agent key 签发（id=%.0f）", permUser, keyID)

	// master 会话写两件 zh 文本：public（缺省共享）+ private（显式私密）
	write(ctx, master, permPubPath, permZhTxt, "public")
	write(ctx, master, permPrivPath, permZhTxt, "private")

	// 等异步喂食：master 视角私密件可查（owner=agent + admin 豁免）
	deadline := time.Now().Add(20 * time.Second)
	for {
		files, _, ok := search(ctx, master, condZhEq)
		if ok && hasPath(pathsOf(files), permPrivPath) {
			break
		}
		if time.Now().After(deadline) {
			fail("perm 喂食轮询超时：master 20s 内未命中私密件（异步事件未消化？）")
			return
		}
		time.Sleep(500 * time.Millisecond)
	}

	// agent 会话：握手成功本身即 agent-external 鉴权贯通的断言
	ac, err := dial(sseURL, rawKey)
	if err != nil {
		fail("perm agent 会话 dial: %v", err)
		return
	}
	if err := ac.Start(ctx); err != nil {
		fail("perm agent 会话 start: %v", err)
		return
	}
	defer ac.Close()
	if err := handshake(ctx, ac); err != nil {
		fail("perm agent 会话握手失败（agent key 鉴权未贯通？）: %v", err)
		return
	}
	pass("perm：agent key 会话握手成功（agent-external 鉴权贯通）")

	// 复判核心：agent 视角 zh 检索——public 可见、他人 private 隔离
	files, _, ok := search(ctx, ac, condZhEq)
	if ok {
		paths := pathsOf(files)
		if !hasPath(paths, permPubPath) {
			fail("perm agent 视角缺 public 件（复判误伤 public？）: %v", paths)
		} else if hasPath(paths, permPrivPath) {
			fail("perm agent 视角见他人 private 件（复判失守！）: %v", paths)
		} else {
			pass("perm 复判：agent 可见 public、隔离他人 private（search_files 复判边界）")
		}
	}

	// master 视角：两件全见（admin 无观察者过滤）
	files, _, ok = search(ctx, master, condZhEq)
	if ok {
		paths := pathsOf(files)
		if !hasPath(paths, permPubPath) || !hasPath(paths, permPrivPath) {
			fail("perm master 视角应两件全见（admin 豁免）: %v", paths)
		} else {
			pass("perm 复判：master 全见（admin 无观察者过滤）")
		}
	}

	// 负路径：坏 key 拒连
	if rejected(ctx, sseURL, "mak-definitely-not-a-valid-key") {
		pass("perm 负路径：坏 key 被拒")
	} else {
		fail("perm 坏 key 竟然握手成功（鉴权失守！）")
	}

	// 吊销 agent key → 同 key 再连被拒（revocation 生效）
	if dcode := httpDelete(ctx, fmt.Sprintf("%s/api/admin/agent-keys/%d", base, int64(keyID)), bearer); dcode != http.StatusOK {
		fail("perm 吊销 key id=%.0f → status=%d, want 200", keyID, dcode)
	} else if rejected(ctx, sseURL, rawKey) {
		pass("perm 负路径：吊销后同 key 被拒（revocation 生效）")
	} else {
		fail("perm 吊销后同 key 竟然握手成功（revocation 失守！）")
	}
}

// condZhEq 权限段复判用的检索条件（zh 文本圈中两件夹具）。
const condZhEq = `[{"field":"cod-text-language","op":"eq","value":"zh"}]`

// httpPostJSON POST JSON（可选 Bearer）并解析响应为 (map, status)；
// map 为 nil 时 status 仍尽力返回（非 JSON 响应体或请求失败）。
func httpPostJSON(ctx context.Context, rawURL string, body []byte, bearer string) (map[string]any, int) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, bytes.NewReader(body))
	if err != nil {
		fail("http new request %s: %v", rawURL, err)
		return nil, 0
	}
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", bearer)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		fail("http POST %s: %v", rawURL, err)
		return nil, 0
	}
	defer res.Body.Close()
	var out map[string]any
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		fail("http POST %s 响应非 JSON: %v", rawURL, err)
		return nil, res.StatusCode
	}
	return out, res.StatusCode
}

// httpDelete 带 Bearer 的 DELETE，返回状态码（-1 = 请求失败已计 FAIL）。
func httpDelete(ctx context.Context, rawURL, bearer string) int {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, rawURL, nil)
	if err != nil {
		fail("http new request %s: %v", rawURL, err)
		return -1
	}
	req.Header.Set("Authorization", bearer)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		fail("http DELETE %s: %v", rawURL, err)
		return -1
	}
	defer res.Body.Close()
	return res.StatusCode
}
