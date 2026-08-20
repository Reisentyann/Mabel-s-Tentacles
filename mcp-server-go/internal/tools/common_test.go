package tools

import (
	"testing"

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
