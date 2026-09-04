// 文件：describer-go/code/code_test.go —— cod-code 单元测试
// 修改：2026-09-03（日期由 fresh-header.ps1 刷新）

package code

import (
	"reflect"
	"testing"

	"github.com/Reisentyann/Mabel-s-Tentacles/describer-go"
)

const goSrc = `package main

import "fmt"
import (
	"os"
	"strings"
)

// TODO: fix me
func main() {
	// FIXME: later
	fmt.Println(strings.Join(os.Args, " "))
}
`

func TestGoAnalyze(t *testing.T) {
	d := descriptor{}
	attrs, _ := d.Analyze(describer.Input{Path: "main.go"}, []byte(goSrc))

	if attrs["cod-code-lang"] != "go" {
		t.Fatalf("lang = %v", attrs["cod-code-lang"])
	}
	if got := attrs["cod-code-imports"].([]string); !reflect.DeepEqual(got, []string{"fmt", "os", "strings"}) {
		t.Fatalf("imports = %#v", got)
	}
	if attrs["cod-code-todo-count"].(int) != 2 {
		t.Fatalf("todo-count = %v, want 2", attrs["cod-code-todo-count"])
	}
}

func TestSupports(t *testing.T) {
	d := descriptor{}
	if !d.Supports("x.go", nil, describer.Basic{Textish: true}) {
		t.Fatal("go file should be supported")
	}
	if d.Supports("x.md", nil, describer.Basic{Textish: true}) {
		t.Fatal("markdown should not be code")
	}
	if d.Supports("x.go", nil, describer.Basic{Textish: false}) {
		t.Fatal("non-textish must not be code")
	}
}
