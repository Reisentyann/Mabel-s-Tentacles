// 文件：mcp-server-go/internal/tools/common_test.go —— 工具共享层单元测试
// 修改：2026-09-08（日期由 fresh-header.ps1 刷新）

package tools

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/authz"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/config"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/service"
)

// TicketMatches 断言 URL 里的 exp/ticket 能通过验票（单文件绑定口径）。
func TicketMatches(t *testing.T, cfg *config.Config, u, path, uuid string) {
	t.Helper()
	// u 形如 <base>/api/files/download?path=…&exp=…&ticket=…
	q := u[strings.Index(u, "?")+1:]
	vals, _ := url.ParseQuery(q)
	ticket, exp := vals.Get("ticket"), vals.Get("exp")
	if ticket == "" || exp == "" {
		t.Fatalf("url missing exp/ticket: %q", u)
	}
	if err := service.VerifyDownloadTicket(cfg.Security.SecretKey, path, uuid, exp, ticket); err != nil {
		t.Fatalf("ticket verify: %v (url=%s)", err, u)
	}
}

func TestDownloadURL(t *testing.T) {
	cfg := &config.Config{}
	cfg.API.DownloadBaseURL = "http://localhost:8080"
	cfg.Security.SecretKey = "test-secret"

	u := DownloadURL(cfg, "jokes.txt", "u-1")
	if !strings.HasPrefix(u, "http://localhost:8080/api/files/download?path=jokes.txt&") {
		t.Errorf("basic: got %q", u)
	}
	TicketMatches(t, cfg, u, "jokes.txt", "u-1")

	// 子目录：斜杠保持裸形（QQ 转义靶子治理），空格仍编码
	u = DownloadURL(cfg, "sub/a b.txt", "u-2")
	if !strings.HasPrefix(u, "http://localhost:8080/api/files/download?path=sub/a+b.txt&") {
		t.Errorf("escape: got %q", u)
	}
	TicketMatches(t, cfg, u, "sub/a b.txt", "u-2")

	// 票据按文件绑定：拿 a 文件的票去下 b 文件必须失效
	u = DownloadURL(cfg, "a.txt", "u-a")
	if err := service.VerifyDownloadTicket(cfg.Security.SecretKey, "b.txt", "u-a",
		parseExp(t, u), ticketOf(t, u)); err == nil {
		t.Error("ticket must not verify for another file")
	}
}

func TestDownloadURLEmpty(t *testing.T) {
	cfg := &config.Config{}
	if got := DownloadURL(cfg, "x.txt", "u-1"); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

// TestDownloadURLFallbackBaseURL download_base_url 空时回退 server.base_url
// （同源部署：SSE/API/下载同一入口——部署批次 2026-09-08）。
func TestDownloadURLFallbackBaseURL(t *testing.T) {
	cfg := &config.Config{}
	cfg.Security.SecretKey = "test-secret"
	cfg.Server.BaseURL = "https://tentacles.example.cn/"
	u := DownloadURL(cfg, "笔记.txt", "u-9")
	if !strings.HasPrefix(u, "https://tentacles.example.cn/api/files/download?path=%E7%AC%94%E8%AE%B0.txt&") {
		t.Errorf("fallback: got %q", u)
	}
	TicketMatches(t, cfg, u, "笔记.txt", "u-9")

	// 专门前缀仍然优先
	cfg.API.DownloadBaseURL = "http://dl.internal"
	u = DownloadURL(cfg, "x.txt", "u-10")
	if !strings.HasPrefix(u, "http://dl.internal/api/files/download?path=x.txt&") {
		t.Errorf("priority: got %q", u)
	}
}

func parseExp(t *testing.T, u string) string {
	t.Helper()
	vals, _ := url.ParseQuery(u[strings.Index(u, "?")+1:])
	return vals.Get("exp")
}

func ticketOf(t *testing.T, u string) string {
	t.Helper()
	vals, _ := url.ParseQuery(u[strings.Index(u, "?")+1:])
	return vals.Get("ticket")
}

// scopedCtx 造一个已认证主体的 context（键空间测试用）。
func scopedCtx(name string) context.Context {
	return authz.WithPrincipal(context.Background(), &authz.Principal{
		Kind: authz.KindUser, Name: name, Role: "user",
	})
}

// TestScopePath 键空间解析（owner 隔离批次）：无前缀 = 自己空间自动拼接；
// ~/ 展开；~A/ 跨用户寻址；坏段 / 未认证 fail-closed。
func TestScopePath(t *testing.T) {
	ctx := scopedCtx("mabel")

	// 无前缀 → 自己空间
	sc, err := ScopePath(ctx, "笔记.txt")
	if err != nil || sc.Key != "~mabel/笔记.txt" || !sc.Self || sc.Owner != "mabel" {
		t.Fatalf("plain: %+v err=%v", sc, err)
	}
	// ~/ → 自己空间展开
	sc, err = ScopePath(ctx, "~/小说/第一章.txt")
	if err != nil || sc.Key != "~mabel/小说/第一章.txt" || !sc.Self {
		t.Fatalf("tilde-self: %+v err=%v", sc, err)
	}
	// ~A/ → 跨用户寻址（只读语义）
	sc, err = ScopePath(ctx, "~ringo/清单.md")
	if err != nil || sc.Key != "~ringo/清单.md" || sc.Self || sc.Owner != "ringo" {
		t.Fatalf("cross: %+v err=%v", sc, err)
	}
	// 显式寻址自己 = self
	sc, err = ScopePath(ctx, "~mabel/x.txt")
	if err != nil || !sc.Self {
		t.Fatalf("explicit-self: %+v err=%v", sc, err)
	}

	// 坏段：空路径 / 裸 ~ / ~ 后无段 / 未认证
	for _, bad := range []string{"", "~", "~/", "~A/"} {
		if _, err := ScopePath(ctx, bad); err == nil {
			t.Errorf("ScopePath(%q) should fail", bad)
		}
	}
	if _, err := ScopePath(context.Background(), "x.txt"); err == nil {
		t.Error("unauthenticated context must fail (fail-closed)")
	}
}

// TestScopeWrite 写侧拒绝：跨用户写入禁止（MCP 工具不代人写）。
func TestScopeWrite(t *testing.T) {
	ctx := scopedCtx("mabel")

	if sc, err := ScopeWrite(ctx, "笔记.txt"); err != nil || sc.Key != "~mabel/笔记.txt" {
		t.Fatalf("self write: %+v err=%v", sc, err)
	}
	if _, err := ScopeWrite(ctx, "~ringo/清单.md"); err == nil {
		t.Fatal("cross-user write must be rejected")
	}
	if _, err := ScopeWrite(context.Background(), "x.txt"); err == nil {
		t.Fatal("unauthenticated write must fail")
	}
}
