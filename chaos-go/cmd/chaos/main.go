// 文件：chaos-go/cmd/chaos/main.go —— 混沌机命令行：按功能名运行（默认梅贝尔台词）
// 修改：2026-09-11（日期由 fresh-header.ps1 刷新）

// 用法：
//
//	go run ./cmd/chaos                          # 默认 mabel_quote：随机 1-5 段台词
//	go run ./cmd/chaos mabel_quote count=2      # 指定段数
//	go run ./cmd/chaos -source lfdr true_random count=16
//	go run ./cmd/chaos -source jinan true_random
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Reisentyann/Mabel-s-Tentacles/chaos-go"
	_ "github.com/Reisentyann/Mabel-s-Tentacles/chaos-go/all"
	"github.com/Reisentyann/Mabel-s-Tentacles/chaos-go/truerandom"
)

func main() {
	source := flag.String("source", "", "真随机熵源：lfdr / jinan / anu（不指定则不注入真随机源）")
	flag.Parse()

	switch *source {
	case "lfdr":
		chaos.Default().SetEntropySource(truerandom.NewLFDR("", 15*time.Second))
	case "jinan":
		chaos.Default().SetEntropySource(truerandom.NewJinan("", 15*time.Second))
	case "anu":
		chaos.Default().SetEntropySource(truerandom.NewANU("", 15*time.Second))
	}

	rest := flag.Args()
	name := "mabel_quote"
	if len(rest) > 0 {
		name = rest[0]
		rest = rest[1:]
	}
	params := chaos.Params{}
	for _, a := range rest {
		if k, v, ok := strings.Cut(a, "="); ok {
			params[k] = v
		}
	}

	out, err := chaos.Default().Run(name, params)
	if err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		os.Exit(1)
	}

	if lines, ok := out["lines"].([]string); ok {
		fmt.Println(strings.Join(lines, "\n\n"))
		return
	}
	if hexv, ok := out["hex"].(string); ok {
		fmt.Printf("%s（来源 %v）\n", hexv, out["source"])
		return
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	fmt.Println(string(b))
}
