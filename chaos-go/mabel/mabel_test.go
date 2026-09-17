// 文件：chaos-go/mabel/mabel_test.go —— 梅贝尔台词组件单测：解析分组 / 随机段数 / 去重 / 并发 / 自注册
// 修改：2026-09-11（日期由 fresh-header.ps1 刷新）

package mabel_test

import (
	"strings"
	"sync"
	"testing"

	"github.com/Reisentyann/Mabel-s-Tentacles/chaos-go"
	"github.com/Reisentyann/Mabel-s-Tentacles/chaos-go/mabel"
)

func TestLibraryParsed(t *testing.T) {
	if n := mabel.Count(); n < 100 {
		t.Fatalf("台词库组数异常偏少: %d", n)
	}
	for _, g := range mabel.Lines(5) {
		if strings.TrimSpace(g) == "" {
			t.Fatal("空台词")
		}
		if strings.Contains(g, `\c[`) || strings.Contains(g, `\!`) {
			t.Fatalf("残留转录控制码: %q", g)
		}
	}
}

func TestLinesCountAndDistinct(t *testing.T) {
	for _, n := range []int{1, 3, 5} {
		got := mabel.Lines(n)
		if len(got) != n {
			t.Fatalf("Lines(%d) 返回 %d 段", n, len(got))
		}
		seen := map[string]bool{}
		for _, g := range got {
			if seen[g] {
				t.Fatalf("Lines(%d) 出现重复", n)
			}
			seen[g] = true
		}
	}
}

func TestLinesDefaultRandomRange(t *testing.T) {
	for i := 0; i < 50; i++ {
		if got := mabel.Lines(0); len(got) < 1 || len(got) > 5 {
			t.Fatalf("默认随机段数越界: %d", len(got))
		}
	}
}

func TestLinesClampHigh(t *testing.T) {
	if got := mabel.Lines(100); len(got) != 5 {
		t.Fatalf("超过上限应夹到 5，得到 %d", len(got))
	}
}

func TestLinesConcurrent(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if len(mabel.Lines(5)) != 5 {
				t.Error("并发取词长度异常")
			}
		}()
	}
	wg.Wait()
}

// TestRegistered 组件自注册（init 触发 chaos.Register）。
func TestRegistered(t *testing.T) {
	f, ok := chaos.Lookup("mabel_quote")
	if !ok {
		t.Fatal("mabel_quote 未自注册")
	}
	if f.Name() != "mabel_quote" || len(f.Params()) == 0 {
		t.Fatalf("组件注册异常: %+v", f)
	}
}
