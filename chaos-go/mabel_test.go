// 文件：chaos-go/mabel_test.go —— 梅贝尔台词功能单测：解析分组 / 随机段数 / 去重 / 并发
// 修改：2026-09-11（日期由 fresh-header.ps1 刷新）

package chaos

import (
	"strings"
	"sync"
	"testing"
)

func TestMabelLibraryParsed(t *testing.T) {
	if n := MabelCount(); n < 100 {
		t.Fatalf("台词库组数异常偏少: %d", n)
	}
	for i, g := range mabelLibrary() {
		if strings.TrimSpace(g) == "" {
			t.Errorf("第 %d 组为空", i)
		}
		if strings.Contains(g, `\c[`) {
			t.Errorf("第 %d 组残留控制码: %q", i, g)
		}
		if strings.Contains(g, `\!`) {
			t.Errorf("第 %d 组残留转义叹号: %q", i, g)
		}
	}
}

func TestMabelLinesCountAndDistinct(t *testing.T) {
	c := New()
	for _, n := range []int{1, 3, 5} {
		got := c.MabelLines(n)
		if len(got) != n {
			t.Fatalf("MabelLines(%d) 返回 %d 段", n, len(got))
		}
		seen := map[string]bool{}
		for _, g := range got {
			if seen[g] {
				t.Fatalf("MabelLines(%d) 出现重复台词", n)
			}
			seen[g] = true
		}
	}
}

func TestMabelLinesDefaultRandomRange(t *testing.T) {
	c := New()
	for i := 0; i < 50; i++ {
		if got := c.MabelLines(0); len(got) < 1 || len(got) > 5 {
			t.Fatalf("默认随机段数越界: %d", len(got))
		}
	}
}

func TestMabelLinesClampHigh(t *testing.T) {
	if got := New().MabelLines(100); len(got) != 5 {
		t.Fatalf("超过上限应夹到 5，得到 %d", len(got))
	}
}

func TestMabelLinesConcurrent(t *testing.T) {
	c := New()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if len(c.MabelLines(5)) != 5 {
				t.Error("并发取词长度异常")
			}
		}()
	}
	wg.Wait()
}
