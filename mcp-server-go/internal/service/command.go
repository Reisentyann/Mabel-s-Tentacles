package service

import (
	"bytes"
	"context"
	"os/exec"
	"runtime"
	"time"
)

const defaultCommandTimeout = 60 * time.Second

func ExecuteCommand(ctx context.Context, command string, timeout time.Duration) (int, string, string) {
	if timeout <= 0 {
		timeout = defaultCommandTimeout
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := shellCommand(ctx, command)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

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
