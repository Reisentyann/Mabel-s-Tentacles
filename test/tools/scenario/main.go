// 文件：test/tools/scenario/main.go —— L4 全流程场景测试（构思自驱）：触手书房档案 + 多用户同名碰撞 + 移动谱系
// 修改：2026-09-08（日期由 fresh-header.ps1 刷新）

// 场景构思（2026-09-08 owner 键空间 + 文件管理域批次的全流程验证）：
//
//	Phase A 管家通道（master key）写书房档案 ×4（文本表格 / shell / markdown / 短文）
//	        → analyze 逐件断言 cod 事实
//	Phase B 移动两式：ext 不变（纯 DB 键改零盘操作）+ ext 变化（派生位 rename）
//	        → 新键可读、旧键失联、uuid 不变
//	Phase C 修改重析：append → analyze → 行数增长
//	Phase D 多用户同名碰撞：e2e_user（external key 通道）与管家各写"笔记.txt"
//	        → 键空间隔离（~e2e_user/ vs ~agent/）同名不碰撞、uuid 不同
//	        → private 越权读拒 / public 跨用户读通 / 谱系 moved_from 随元数据带出
//
// 退出码 0 = 全部通过；非 0 = 有 FAIL 行。
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
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

// ============ MCP 通道（鉴权头：master / external key 双轨） ============

type channel struct {
	c   *client.Client
	ctx context.Context
}

// dial 建立 MCP 通道（Authorization: Bearer <key>）。
func dial(url, key string) (*channel, error) {
	c, err := client.NewSSEMCPClient(url, client.WithHeaders(map[string]string{
		"Authorization": "Bearer " + key,
	}))
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	if err := c.Start(ctx); err != nil {
		cancel()
		return nil, err
	}
	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{Name: "mabel-scenario", Version: "1.0"}
	if _, err := c.Initialize(ctx, initReq); err != nil {
		cancel()
		return nil, err
	}
	// 把 cancel 挂进 ctx 生命周期之外不调（进程即回收），保 ctx 存活
	_ = cancel
	return &channel{c: c, ctx: ctx}, nil
}

// call 调工具，解析 JSON 回执（success=false 返回 nil 由调用方断言）。
func (ch *channel) call(name string, args map[string]any) map[string]any {
	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args
	res, err := ch.c.CallTool(ch.ctx, req)
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
		fail("%s content[0] is %T", name, res.Content[0])
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(text.Text), &out); err != nil {
		fail("%s content is not JSON: %.120s", name, text.Text)
		return nil
	}
	return out
}

// okCall 调工具并强制 success=true（返回 nil 时已记 FAIL）。
func (ch *channel) okCall(name string, args map[string]any) map[string]any {
	out := ch.call(name, args)
	if out == nil {
		return nil
	}
	if v, _ := out["success"].(bool); !v {
		fail("%s success=false: %.200s", name, out)
		return nil
	}
	return out
}

// denyCall 期待拒绝（success=false 且 message 含权限字样）——越权是正确答案。
func (ch *channel) denyCall(name string, args map[string]any) bool {
	out := ch.call(name, args)
	if out == nil {
		return false
	}
	if v, _ := out["success"].(bool); v {
		fail("%s 应被拒绝却成功了（越权未拦截！）args=%v", name, args)
		return false
	}
	msg, _ := out["message"].(string)
	if !strings.Contains(msg, "权限") {
		fail("%s 拒绝文案异常: %q", name, msg)
		return false
	}
	return true
}

func str(m map[string]any, k string) string {
	if m == nil {
		return ""
	}
	s, _ := m[k].(string)
	return s
}

func num(m map[string]any, k string) (float64, bool) {
	if m == nil {
		return 0, false
	}
	v, ok := m[k].(float64)
	return v, ok
}

// ============ HTTP（admin 登录 / 签发 external key / 谱系核对） ============

func httpJSON(method, url, token string, body any) (int, map[string]any) {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, url, rd)
	if body != nil {
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, nil
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(b, &out)
	return resp.StatusCode, out
}

// ============ 场景内容（构思：触手书房的日常档案） ============

const bookList = `梅贝尔书房 · 借阅清单（9月）

| 书名 | 借阅人 | 状态 |
|---|---|---|
| 触手生态图鉴 | 铃仙 | 已还 |
| 深海植物志 | 帕秋莉 | 在借 |
| 旧书修复手册 | 梅贝尔 | 在借 |

备注：逾期未还将被收取一枚书签作为罚金。
`

const tidyScript = `#!/bin/bash
# 书房整理脚本：按封面颜色归档
set -euo pipefail

ARCHIVE_DIR="档案/按颜色"

sort_by_color() {
  local dir="$1"
  # TODO: 识别封面主色再分箱
  find "$dir" -type f | wc -l
}

main() {
  mkdir -p "$ARCHIVE_DIR"
  sort_by_color "新到箱"
  echo "整理完毕。"
}

main "$@"
`

const nightLog = `# 深夜日志

## 今夜待办

- [x] 归档旧箱三只
- [x] 给帕秋莉回信
- [ ] 修补《深海植物志》书脊
- [ ] 清点触手饲料库存

## 备注

雨一直下，书房的灯留到天明。
`

const visitorLog = `访客登记

铃仙，午后，还书一册。
帕秋莉，黄昏，借书一册。
`

const noteContent = `私人笔记：触手饲料配方第三稿。

原料配比待定，先记在这里。
`

func main() {
	sseURL := flag.String("url", "http://127.0.0.1:8080/sse", "MCP SSE 端点")
	master := flag.String("master", "", "master key（管家通道）")
	apiBase := flag.String("api", "http://127.0.0.1:8080", "HTTP API 基址")
	adminUser := flag.String("admin-user", "", "admin 用户名")
	adminPass := flag.String("admin-pass", "", "admin 密码")
	flag.Parse()

	if *master == "" || *adminUser == "" || *adminPass == "" {
		fmt.Println("用法: scenario -url <sse> -master <key> -api <base> -admin-user <u> -admin-pass <p>")
		os.Exit(2)
	}

	// ---- Phase A：管家通道写书房档案 ×4 ----
	masterCh, err := dial(*sseURL, *master)
	if err != nil {
		fail("管家通道握手失败: %v", err)
		os.Exit(1)
	}
	pass("管家通道握手成功（master key 鉴权路径）")

	for _, w := range []struct{ path, content string }{
		{"书房档案/借阅清单.txt", bookList},
		{"书房档案/整理脚本.sh", tidyScript},
		{"书房档案/深夜日志.md", nightLog},
		{"书房档案/访客登记.txt", visitorLog},
	} {
		if masterCh.okCall("write_file", map[string]any{
			"file_path": w.path, "content": w.content,
			"description": "场景测试：触手书房档案", "tags": "场景,书房档案",
		}) == nil {
			os.Exit(1)
		}
	}
	pass("write_file ×4 完成（借阅清单/整理脚本/深夜日志/访客登记）")

	// analyze 逐件断言（描述事实随事件异步落库——轮询等执行器收尾）
	waitAttrs := func(path string, probe func(attrs map[string]any) string) (map[string]any, string) {
		for i := 0; i < 20; i++ {
			out := masterCh.okCall("analyze_file", map[string]any{"file_path": path})
			attrs, _ := out["attrs"].(map[string]any)
			if msg := probe(attrs); msg == "" {
				return attrs, ""
			} else if i == 19 {
				return attrs, msg
			}
			time.Sleep(300 * time.Millisecond)
		}
		return nil, "unreachable"
	}

	if _, msg := waitAttrs("书房档案/借阅清单.txt", func(a map[string]any) string {
		if a["cod-text-language"] != "zh" {
			return "cod-text-language != zh"
		}
		if tr, _ := a["cod-text-table-rows"].(float64); tr < 4 {
			return "table-rows < 4"
		}
		return ""
	}); msg != "" {
		fail("借阅清单 analyze: %s", msg)
	} else {
		pass("借阅清单: language=zh, table-rows>=4（表格文本事实）")
	}

	if _, msg := waitAttrs("书房档案/整理脚本.sh", func(a map[string]any) string {
		if sh, _ := a["cod-text-shebang"].(string); sh != "#!/bin/bash" {
			return "shebang != #!/bin/bash"
		}
		// funcCount 按语言正则（go/py/js/rust/c 系），shell 不产——用
		// todo-count 钉 code 侧事实（脚本内恰有一个 TODO）
		if td, _ := a["cod-code-todo-count"].(float64); td != 1 {
			return "todo-count != 1"
		}
		return ""
	}); msg != "" {
		fail("整理脚本 analyze: %s", msg)
	} else {
		pass("整理脚本: shebang 命中, todo-count=1（shell 事实）")
	}

	if _, msg := waitAttrs("书房档案/深夜日志.md", func(a map[string]any) string {
		if li, _ := a["cod-text-list-items"].(float64); li < 4 {
			return "list-items < 4"
		}
		if cb, _ := a["cod-text-checkboxes"].(float64); cb < 4 {
			return "checkboxes < 4"
		}
		return ""
	}); msg != "" {
		fail("深夜日志 analyze: %s", msg)
	} else {
		pass("深夜日志: list-items>=4, checkboxes>=4（markdown 事实）")
	}

	// ---- Phase B：移动两式（runID 后缀：场景可重复跑，目标键不撞库） ----
	runID := time.Now().Format("0102-150405")
	mv1 := masterCh.okCall("move_file", map[string]any{
		"source": "书房档案/访客登记.txt", "target": "书房档案/旧档/访客登记-" + runID + ".txt",
	})
	if mv1 == nil {
		os.Exit(1)
	}
	if sm, _ := mv1["storage_move"].(bool); sm {
		fail("ext 不变的移动应为纯 DB 键改（storage_move=false）")
	} else {
		pass("移动①纯键改: 访客登记.txt → 旧档/访客登记-0910.txt（零盘操作）")
	}

	mv2 := masterCh.okCall("move_file", map[string]any{
		"source": "书房档案/深夜日志.md", "target": "书房档案/日志归档-" + runID + ".txt",
	})
	if mv2 == nil {
		os.Exit(1)
	}
	if sm, _ := mv2["storage_move"].(bool); !sm {
		fail("ext 变化的移动应触发物理位 rename（storage_move=true）")
	} else {
		pass("移动②ext 变化: 深夜日志.md → 日志归档.txt（派生位 rename）")
	}

	// 新键可读 + 内容不变；旧键失联（期待失败——不记 FAIL，失联才是正确答案）
	if out := masterCh.okCall("read_file", map[string]any{"path": "书房档案/旧档/访客登记-" + runID + ".txt"}); out != nil {
		if !strings.Contains(str(out, "content"), "帕秋莉") {
			fail("移动后新键内容异常")
		} else {
			pass("新键可读且内容完好（uuid 不变 → 内容随键可达）")
		}
	} else {
		os.Exit(1)
	}
	if out := masterCh.call("read_file", map[string]any{"path": "书房档案/访客登记.txt"}); out != nil {
		if v, _ := out["success"].(bool); v {
			fail("旧键在移动后仍可读（键空间未迁移！）")
		} else {
			pass("旧键失联（键空间已迁移）")
		}
	} else {
		fail("旧键 read 异常（工具级错误而非业务拒绝）")
	}

	// ---- Phase C：修改重析（相对断言：append 后行数较 append 前增长） ----
	var linesBefore float64
	if out := masterCh.okCall("analyze_file", map[string]any{"file_path": "书房档案/借阅清单.txt"}); out != nil {
		attrs, _ := out["attrs"].(map[string]any)
		linesBefore, _ = attrs["cod-text-lines"].(float64)
	}
	if masterCh.okCall("modify_data_file", map[string]any{
		"file_path": "书房档案/借阅清单.txt", "mode": "append",
		"content": "\n追加：触手生态图鉴第二卷已到馆。\n",
	}) == nil {
		os.Exit(1)
	}
	if _, msg := waitAttrs("书房档案/借阅清单.txt", func(a map[string]any) string {
		if ln, _ := a["cod-text-lines"].(float64); ln <= linesBefore {
			return fmt.Sprintf("text-lines=%.0f <= before=%.0f（append 未入账）", ln, linesBefore)
		}
		return ""
	}); msg != "" {
		fail("修改重析: %s", msg)
	} else {
		pass("append 修改 → 重分析入账（cod-text-lines 增长）")
	}

	// ---- Phase D：多用户同名碰撞 ----
	// admin 登录 → 签发 external key（绑定 e2e_user）
	code, out := httpJSON("POST", *apiBase+"/api/auth/login", "", map[string]any{
		"username": *adminUser, "password": *adminPass,
	})
	if code != 200 || out == nil || str(out, "access_token") == "" {
		fail("admin 登录失败 code=%d", code)
		os.Exit(1)
	}
	adminTok := str(out, "access_token")

	// e2e_user 不存在则注册（重复跑兼容）
	code, out = httpJSON("POST", *apiBase+"/api/auth/login", "", map[string]any{
		"username": "e2e_user", "password": "e2e-password-123",
	})
	if code != 200 {
		if code, out = httpJSON("POST", *apiBase+"/api/auth/register", "", map[string]any{
			"username": "e2e_user", "password": "e2e-password-123",
		}); code != 201 && code != 200 {
			fail("e2e_user 注册/登录失败 code=%d", code)
			os.Exit(1)
		}
	}

	code, out = httpJSON("POST", *apiBase+"/api/admin/agent-keys", adminTok, map[string]any{
		"name": "scenario-key", "username": "e2e_user",
	})
	if code != 201 || str(out, "key") == "" {
		fail("签发 external key 失败 code=%d out=%v", code, out)
		os.Exit(1)
	}
	extKey := str(out, "key")
	pass("admin 签发 external key（绑定 e2e_user，raw 仅此一次）")

	userCh, err := dial(*sseURL, extKey)
	if err != nil {
		fail("e2e_user 通道握手失败: %v", err)
		os.Exit(1)
	}
	pass("e2e_user 通道握手成功（external key 鉴权路径）")

	// 双方各写"笔记.txt"（同名）——键空间隔离，不碰撞
	u1 := userCh.okCall("write_file", map[string]any{"file_path": "笔记.txt", "content": noteContent})
	a1 := masterCh.okCall("write_file", map[string]any{"file_path": "笔记.txt", "content": "管家的同名笔记。\n"})
	if u1 == nil || a1 == nil {
		os.Exit(1)
	}
	uuidU, uuidA := str(u1, "uuid"), str(a1, "uuid")
	if uuidU == "" || uuidA == "" || uuidU == uuidA {
		fail("同名笔记 uuid 相同或为空（键空间隔离失效！）user=%q agent=%q", uuidU, uuidA)
	} else {
		pass("同名不碰撞: e2e_user 与管家各写「笔记.txt」，uuid=%s… / %s…（键空间 ~e2e_user/ vs ~agent/）",
			uuidU[:8], uuidA[:8])
	}

	// e2e_user 读自己的笔记 → 通
	if userCh.okCall("read_file", map[string]any{"path": "笔记.txt"}) == nil {
		os.Exit(1)
	}
	pass("e2e_user 读自己的「笔记.txt」（无前缀 = 自己空间）")

	// 两级可见性模型（2026-09-08）：管家写一份显式 private 的私日记——
	// private 仅主人与管家；public（新默认）协作共享
	if masterCh.okCall("write_file", map[string]any{
		"file_path": "书房档案/管家私日记.txt", "content": "触手饲料的进货渠道，保密。\n",
		"visibility": "private",
	}) == nil {
		os.Exit(1)
	}

	// e2e_user 读管家的 public 档案 → 通（协作共享：接入即信任）
	if userCh.okCall("read_file", map[string]any{"path": "~agent/书房档案/借阅清单.txt"}) == nil {
		os.Exit(1)
	}
	pass("e2e_user 跨用户读管家 public 档案成功（两级模型：public 协作共享）")

	// e2e_user 读管家的 private 私日记 → 拒（私密 = 仅主人与管家）
	if userCh.denyCall("read_file", map[string]any{"path": "~agent/书房档案/管家私日记.txt"}) {
		pass("e2e_user 跨用户读 private 私日记被拒（~agent/ 显式寻址 + 两级可见性）")
	}

	// e2e_user 把自己笔记转 public → 管家跨用户读到
	if userCh.okCall("describe_file", map[string]any{
		"file_path": "笔记.txt", "visibility": "public", "description": "公开的饲料配方笔记",
	}) == nil {
		os.Exit(1)
	}
	if out := masterCh.okCall("read_file", map[string]any{"path": "~e2e_user/笔记.txt"}); out != nil {
		if !strings.Contains(str(out, "content"), "饲料配方") {
			fail("跨用户读到的内容异常（拿错文件？）")
		} else {
			pass("public 跨用户读通: 管家经 ~e2e_user/笔记.txt 读到正确内容")
		}
	}

	// HTTP 谱系核对：moved_from 随元数据自然带出
	code, out = httpJSON("GET", *apiBase+"/api/files/metadata?path="+url.QueryEscape("~agent/书房档案/旧档/访客登记-"+runID+".txt"), adminTok, nil)
	if code != 200 {
		fail("谱系元数据查询失败 code=%d", code)
	} else if mf, _ := out["moved_from"].(string); mf != "~agent/书房档案/访客登记.txt" {
		fail("moved_from = %q, want 原键", mf)
	} else {
		pass("谱系核对: moved_from 随元数据带出（%s ← %s）",
			"访客登记-0910.txt", "访客登记.txt")
	}

	fmt.Printf("\n==== 场景全流程：%d 项失败 ====\n", failures)
	if failures > 0 {
		os.Exit(1)
	}
}
