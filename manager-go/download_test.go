// 文件：manager-go/download_test.go —— 下载票据域 L1：签发/验签往返 / 单文件绑定 / 过期 / 篡改 / 转义 / 未启用
// 修改：2026-09-11（日期由 fresh-header.ps1 刷新）

package manager_test

import (
	"net/url"
	"strings"
	"testing"

	"github.com/Reisentyann/Mabel-s-Tentacles/manager-go"
)

// newDLManager 构造只关心下载域的 manager（store 用空替身；dataDir 空即可）。
func newDLManager(secret, base string) *manager.Manager {
	return manager.New(newFakeStore(), "", nil, nil, manager.DownloadConfig{Secret: secret, BaseURL: base})
}

func dlQuery(t *testing.T, u string) url.Values {
	t.Helper()
	i := strings.Index(u, "?")
	if i < 0 {
		t.Fatalf("下载地址缺 query: %q", u)
	}
	vals, err := url.ParseQuery(u[i+1:])
	if err != nil {
		t.Fatal(err)
	}
	return vals
}

// TestIssueAndVerifyDownloadURL 往返 + 单文件绑定 + 篡改 + 过期。
func TestIssueAndVerifyDownloadURL(t *testing.T) {
	m := newDLManager("test-secret", "http://localhost:8080")
	u := m.IssueDownloadURL("jokes.txt", "u-1", 0)
	if !strings.HasPrefix(u, "http://localhost:8080"+manager.DownloadEndpoint+"?path=jokes.txt&") {
		t.Fatalf("地址形态: %q", u)
	}
	q := dlQuery(t, u)
	if q.Get("ticket") == "" || q.Get("exp") == "" {
		t.Fatalf("缺 exp/ticket: %q", u)
	}
	if err := m.VerifyDownloadTicket("jokes.txt", "u-1", q.Get("exp"), q.Get("ticket")); err != nil {
		t.Fatalf("验签失败: %v", err)
	}

	// 单文件绑定：换文件 / 换 uuid 一律失效
	if err := m.VerifyDownloadTicket("other.txt", "u-1", q.Get("exp"), q.Get("ticket")); err == nil {
		t.Fatal("换文件不应通过")
	}
	if err := m.VerifyDownloadTicket("jokes.txt", "u-2", q.Get("exp"), q.Get("ticket")); err == nil {
		t.Fatal("换 uuid 不应通过")
	}
	// 篡改
	if err := m.VerifyDownloadTicket("jokes.txt", "u-1", q.Get("exp"), q.Get("ticket")+"x"); err == nil {
		t.Fatal("篡改票据不应通过")
	}
	// 过期（exp 在过去）
	if err := m.VerifyDownloadTicket("jokes.txt", "u-1", "1", q.Get("ticket")); err == nil {
		t.Fatal("过期票据不应通过")
	}
}

// TestDownloadURLEscape 斜杠保持裸形（QQ 二次转义治理），空格仍编码。
func TestDownloadURLEscape(t *testing.T) {
	m := newDLManager("s", "http://h")
	u := m.IssueDownloadURL("sub/a b.txt", "u-2", 0)
	if !strings.HasPrefix(u, "http://h"+manager.DownloadEndpoint+"?path=sub/a+b.txt&") {
		t.Fatalf("转义形态: %q", u)
	}
}

// TestDownloadDisabled Secret / BaseURL 缺失时地址不产出、验签直接拒。
func TestDownloadDisabled(t *testing.T) {
	if got := newDLManager("", "http://h").IssueDownloadURL("x", "u", 0); got != "" {
		t.Fatalf("无 secret 应返回空串, got %q", got)
	}
	if got := newDLManager("s", "").IssueDownloadURL("x", "u", 0); got != "" {
		t.Fatalf("无 base 应返回空串, got %q", got)
	}
	if err := newDLManager("", "http://h").VerifyDownloadTicket("x", "u", "99999999999", "t"); err == nil {
		t.Fatal("未启用时验签应报错")
	}
}
