// 文件：chaos-go/feature_test.go —— 核心单测：注册表机制 / 参数归一 / 熵源注入
// 修改：2026-09-11（日期由 fresh-header.ps1 刷新）
//
// 组件的功能测试在各组件文件夹内（mabel/mabel_test.go、truerandom/…）。

package chaos

import (
	"errors"
	"testing"
)

// dummyFeature 注册表机制测试用（不依赖任何组件子包）。
type dummyFeature struct{}

func (dummyFeature) Name() string        { return "dummy_test_feature" }
func (dummyFeature) Description() string { return "test" }
func (dummyFeature) Params() []Param     { return nil }
func (dummyFeature) Run(c *Chaos, p Params) (map[string]any, error) {
	return map[string]any{"ok": true}, nil
}

func TestRegistry(t *testing.T) {
	if _, ok := Lookup("dummy_test_feature"); !ok {
		Register(dummyFeature{})
	}
	f, ok := Lookup("dummy_test_feature")
	if !ok || f.Name() != "dummy_test_feature" {
		t.Fatalf("注册表查找失败: %+v", f)
	}
	out, err := Run("dummy_test_feature", nil)
	if err != nil || out["ok"] != true {
		t.Fatalf("Run = (%v, %v)", out, err)
	}
	if _, err := Run("no_such_feature", nil); err == nil {
		t.Fatal("未知功能应返回错误")
	}
}

func TestIntParam(t *testing.T) {
	cases := []struct {
		in   Params
		key  string
		def  int
		want int
	}{
		{Params{"n": 7}, "n", 0, 7},
		{Params{"n": int64(8)}, "n", 0, 8},
		{Params{"n": float64(9)}, "n", 0, 9},
		{Params{"n": "10"}, "n", 0, 10},
		{Params{"n": "bad"}, "n", 5, 5},
		{Params{}, "n", 6, 6},
		{Params{"n": nil}, "n", 4, 4},
	}
	for _, c := range cases {
		if got := IntParam(c.in, c.key, c.def); got != c.want {
			t.Errorf("IntParam(%v) = %d, want %d", c.in, got, c.want)
		}
	}
}

// fakeEntropy 熵源替身。
type fakeEntropy struct {
	name string
	data []byte
	err  error
}

func (f fakeEntropy) Name() string              { return f.name }
func (f fakeEntropy) Bytes(int) ([]byte, error) { return f.data, f.err }

func TestEntropySource(t *testing.T) {
	c := New()
	if _, err := c.Entropy(); !errors.Is(err, ErrNoEntropy) {
		t.Fatalf("未注入应返回 ErrNoEntropy，得到 %v", err)
	}
	c.SetEntropySource(fakeEntropy{name: "fake", data: []byte{1, 2, 3}})
	src, err := c.Entropy()
	if err != nil || src.Name() != "fake" {
		t.Fatalf("注入后取源失败: %v %v", src, err)
	}
	c.SetEntropySource(nil)
	if _, err := c.Entropy(); !errors.Is(err, ErrNoEntropy) {
		t.Fatal("清除后应回到未配置")
	}
}
