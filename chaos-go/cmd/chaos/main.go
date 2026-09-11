// 文件：chaos-go/cmd/chaos/main.go —— 混沌机命令行：运行即打印随机梅贝尔台词
// 修改：2026-09-11（日期由 fresh-header.ps1 刷新）

package main

import (
	"flag"
	"fmt"
	"strings"

	"github.com/Reisentyann/Mabel-s-Tentacles/chaos-go"
)

func main() {
	n := flag.Int("n", 0, "台词段数（1-5；0 或缺省 = 随机 1-5 段）")
	flag.Parse()

	lines := chaos.MabelLines(*n)
	fmt.Println(strings.Join(lines, "\n\n"))
}
