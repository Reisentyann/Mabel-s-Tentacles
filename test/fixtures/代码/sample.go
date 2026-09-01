package sample

import "fmt"
import (
	"os"
	"strings"
)

// Sample 生成一句问候。
func Sample(name string) string {
	// TODO: 支持多语言问候
	if name == "" {
		name = "world"
	}
	return fmt.Sprintf("hello, %s", strings.Join(os.Args, " "))
}

// farewell 告别语。
func farewell() string {
	// FIXME: 返回值硬编码
	return "bye"
}
