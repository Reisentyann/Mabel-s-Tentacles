// 文件：mcp-server-go/internal/service/command.go —— 命令执行：60s 超时 + 256KB 输出截断（limitWriter）
// 修改：2026-09-03（日期由 fresh-header.ps1 刷新）

package service

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"time"
)

const defaultCommandTimeout = 60 * time.Second

// maxOutputBytes 单个流（stdout/stderr）保留的最大字节数；超出部分丢弃并追加截断标记。
// 没有它，一条高频输出的命令在 60s 超时内能把内存和 commands.result 打爆。
const maxOutputBytes = 256 * 1024

// limitWriter 只保留写入的前 limit 字节，其余丢弃；total 记录总写入量用于截断标记。
type limitWriter struct {
	buf   bytes.Buffer
	limit int
	total int64
}

func (w *limitWriter) Write(p []byte) (int, error) {
	if room := w.limit - w.buf.Len(); room > 0 {
		n := min(room, len(p))
		w.buf.Write(p[:n])
	}
	w.total += int64(len(p))
	return len(p), nil
}

func (w *limitWriter) String() string {
	if dropped := w.total - int64(w.buf.Len()); dropped > 0 {
		return w.buf.String() + fmt.Sprintf("\n...[output truncated, %d bytes dropped]", dropped)
	}
	return w.buf.String()
}

func ExecuteCommand(ctx context.Context, command string, timeout time.Duration) (int, string, string) {
	if timeout <= 0 {
		timeout = defaultCommandTimeout
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := shellCommand(ctx, command)
	stdout := &limitWriter{limit: maxOutputBytes}
	stderr := &limitWriter{limit: maxOutputBytes}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	err := cmd.Run()
	if err == nil {
		return 0, stdout.String(), stderr.String()
	}
	if ctx.Err() == context.DeadlineExceeded {
		return -1, "", "Command timed out"
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode(), stdout.String(), stderr.String()
	}
	return -1, "", err.Error()
}

func shellCommand(ctx context.Context, command string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.CommandContext(ctx, "cmd", "/c", command)
	}
	return exec.CommandContext(ctx, "sh", "-c", command)
}
