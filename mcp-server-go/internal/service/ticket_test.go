// 文件：mcp-server-go/internal/service/ticket_test.go —— 限时下载票据 L1：签发/验证/过期/单文件绑定/篡改拒
// 修改：2026-09-08（日期由 fresh-header.ps1 刷新）

package service

import (
	"strconv"
	"testing"
	"time"
)

func TestTicketRoundTrip(t *testing.T) {
	exp := time.Now().Add(DownloadTicketTTL)
	ticket := SignDownloadTicket("secret", "笔记.txt", "u-123", exp)
	if err := VerifyDownloadTicket("secret", "笔记.txt", "u-123",
		strconv.FormatInt(exp.Unix(), 10), ticket); err != nil {
		t.Fatalf("valid ticket rejected: %v", err)
	}
}

func TestTicketExpired(t *testing.T) {
	exp := time.Now().Add(-time.Minute) // 已过期
	ticket := SignDownloadTicket("secret", "x.txt", "u-1", exp)
	if err := VerifyDownloadTicket("secret", "x.txt", "u-1",
		strconv.FormatInt(exp.Unix(), 10), ticket); err == nil {
		t.Fatal("expired ticket must be rejected")
	}
}

func TestTicketFileBound(t *testing.T) {
	exp := time.Now().Add(DownloadTicketTTL)
	ticket := SignDownloadTicket("secret", "a.txt", "u-1", exp)
	// 换文件（同一票据下别的路径）必须失效——单文件绑定
	if err := VerifyDownloadTicket("secret", "b.txt", "u-1",
		strconv.FormatInt(exp.Unix(), 10), ticket); err == nil {
		t.Fatal("ticket must be bound to its file")
	}
	// 换 uuid（同路径别的文件）同样失效
	if err := VerifyDownloadTicket("secret", "a.txt", "u-2",
		strconv.FormatInt(exp.Unix(), 10), ticket); err == nil {
		t.Fatal("ticket must be bound to its uuid")
	}
}

func TestTicketTamper(t *testing.T) {
	exp := time.Now().Add(DownloadTicketTTL)
	ticket := SignDownloadTicket("secret", "x.txt", "u-1", exp)
	if err := VerifyDownloadTicket("secret", "x.txt", "u-1",
		strconv.FormatInt(exp.Unix(), 10), ticket+"x"); err == nil {
		t.Fatal("tampered ticket must be rejected")
	}
	if err := VerifyDownloadTicket("other-secret", "x.txt", "u-1",
		strconv.FormatInt(exp.Unix(), 10), ticket); err == nil {
		t.Fatal("ticket from another secret must be rejected")
	}
	if err := VerifyDownloadTicket("secret", "x.txt", "u-1", "abc", ticket); err == nil {
		t.Fatal("bad exp must be rejected")
	}
}
