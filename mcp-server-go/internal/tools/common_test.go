// 文件：mcp-server-go/internal/tools/common_test.go —— 工具共享层单元测试
// 修改：2026-09-08（日期由 fresh-header.ps1 刷新）

package tools

import (
	"context"
	"testing"

	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/authz"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/config"
)

func TestDownloadURL(t *testing.T) {
	cfg := &config.Config{}
	cfg.API.DownloadBaseURL = "http://localhost:8080"

	if got := DownloadURL(cfg, "jokes.txt"); got != "http://localhost:8080/api/files/download?path=jokes.txt" {
		t.Errorf("basic: got %q", got)
	}

	// 子目录/特殊字符需 URL 编码
	if got := DownloadURL(cfg, "sub/a b.txt"); got != "http://localhost:8080/api/files/download?path=sub%2Fa+b.txt" {
		t.Errorf("escape: got %q", got)
	}

	// 带 access_token
	cfg.API.AccessToken = "abc"
	if got := DownloadURL(cfg, "x.txt"); got != "http://localhost:8080/api/files/download?path=x.txt&token=abc" {
		t.Errorf("token: got %q", got)
	}
}

func TestDownloadURLEmpty(t *testing.T) {
	cfg := &config.Config{}
	if got := DownloadURL(cfg, "x.txt"); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
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
