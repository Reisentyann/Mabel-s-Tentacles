// 文件：describer-go/code/gofacts_test.go —— Go 内容定位测试：导入别名、遮蔽、方法、动态参数及解析失败
// 修改：2026-10-08（日期由 fresh-header.ps1 刷新）

package code

import (
	"reflect"
	"testing"

	"github.com/Reisentyann/Mabel-s-Tentacles/describer-go"
)

func TestGoContentFacts(t *testing.T) {
	src := `package sample
import environment "os"
type Worker[T any] struct{}
const Mode = "test"
var hidden = environment.Getenv("DATABASE_URL")
func (w *Worker[T]) Run() { environment.LookupEnv("HOME") }
func start() {
 environment.Getenv("DATABASE_URL")
 environment.Getenv("PREFIX" + Mode)
 environment := struct{ Getenv func(string) string }{}
 environment.Getenv("FALSE_MATCH")
}
// environment.Getenv("COMMENT")
`
	a, _ := (descriptor{}).Analyze(describer.Input{Path: "sample.go", Size: int64(len(src))}, []byte(src))
	if !reflect.DeepEqual(a["cod-code-env-vars"], []string{"DATABASE_URL", "HOME"}) {
		t.Fatalf("env: %#v", a["cod-code-env-vars"])
	}
	if !reflect.DeepEqual(a["cod-code-declared-symbols"], []string{"Worker", "Mode", "hidden", "Worker.Run", "start"}) {
		t.Fatalf("symbols: %#v", a["cod-code-declared-symbols"])
	}
}

func TestGoContentUnavailable(t *testing.T) {
	for _, in := range []describer.Input{{Path: "x.go"}, {Path: "x.go", Size: 1000}, {Path: "x.py"}} {
		src := []byte("package p\nfunc broken(")
		a, _ := (descriptor{}).Analyze(in, src)
		for _, key := range []string{"env-vars", "declared-symbols"} {
			if _, ok := a["cod-code-"+key]; ok {
				t.Errorf("unexpected %s", key)
			}
		}
	}
}

func TestGoPartialValidSource(t *testing.T) {
	src := []byte("package p\nimport \"os\"\nvar Name = os.Getenv(\"HOME\")\n")
	a, _ := (descriptor{}).Analyze(describer.Input{Path: "x.go", Size: int64(len(src)) + 1}, src)
	if _, ok := a["cod-code-env-vars"]; ok {
		t.Fatal("partial source produced environment facts")
	}
	if _, ok := a["cod-code-declared-symbols"]; ok {
		t.Fatal("partial source produced declarations")
	}
}
