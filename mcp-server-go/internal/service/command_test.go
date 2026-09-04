// 文件：mcp-server-go/internal/service/command_test.go —— 命令执行单元测试：超时 / 截断 / 退出码
// 修改：2026-09-03（日期由 fresh-header.ps1 刷新）

package service

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestLimitWriterKeepsPrefix(t *testing.T) {
	w := &limitWriter{limit: 10}
	if _, err := w.Write([]byte("0123456789ABCDEF")); err != nil {
		t.Fatal(err)
	}
	got := w.String()
	if !strings.HasPrefix(got, "0123456789") {
		t.Fatalf("前缀丢失: %q", got)
	}
	if !strings.Contains(got, "output truncated") {
		t.Fatalf("缺少截断标记: %q", got)
	}
	if strings.Contains(got, "ABCDEF") {
		t.Fatalf("超限内容未丢弃: %q", got)
	}
}

func TestLimitWriterNoMarkerWhenUnderLimit(t *testing.T) {
	w := &limitWriter{limit: 100}
	if _, err := w.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	if got := w.String(); got != "hello" {
		t.Fatalf("未超限时不应有标记: %q", got)
	}
}

func TestLimitWriterMultipleWrites(t *testing.T) {
	w := &limitWriter{limit: 5}
	w.Write([]byte("abc"))
	w.Write([]byte("def"))
	w.Write([]byte("ghi"))
	got := w.String()
	if !strings.HasPrefix(got, "abcde") {
		t.Fatalf("应保留前 5 字节: %q", got)
	}
	if !strings.Contains(got, "4 bytes dropped") {
		t.Fatalf("应丢弃 4 字节并标记: %q", got)
	}
}

func TestExecuteCommandOutputTruncated(t *testing.T) {
	// Windows: cmd /c；Linux: sh -c。echo 都可用，输出远小于上限。
	code, stdout, _ := ExecuteCommand(context.Background(), "echo hello", 10*time.Second)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout, "hello") {
		t.Fatalf("stdout 缺少 hello: %q", stdout)
	}
}
