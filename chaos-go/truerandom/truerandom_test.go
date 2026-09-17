// 文件：chaos-go/truerandom/truerandom_test.go —— 真随机机组件单测：未配置/成功/失败不回退/自注册
// 修改：2026-09-11（日期由 fresh-header.ps1 刷新）

package truerandom

import (
	"errors"
	"testing"

	"github.com/Reisentyann/Mabel-s-Tentacles/chaos-go"
)

// fakeSource 熵源替身。
type fakeSource struct {
	name string
	data []byte
	err  error
}

func (f fakeSource) Name() string { return f.name }
func (f fakeSource) Bytes(n int) ([]byte, error) {
	if f.err != nil {
		return nil, f.err
	}
	if len(f.data) < n {
		return nil, errors.New("short")
	}
	return f.data[:n], nil
}

func TestRunNoSource(t *testing.T) {
	if _, err := (trueRandom{}).Run(chaos.New(), nil); !errors.Is(err, chaos.ErrNoEntropy) {
		t.Fatalf("未配置应返回 ErrNoEntropy，得到 %v", err)
	}
}

func TestRunSuccess(t *testing.T) {
	c := chaos.New()
	c.SetEntropySource(fakeSource{name: "quantum:test", data: []byte{0xde, 0xad, 0xbe, 0xef, 1, 2, 3, 4}})
	out, err := (trueRandom{}).Run(c, chaos.Params{"count": 4})
	if err != nil {
		t.Fatal(err)
	}
	if out["hex"] != "deadbeef" || out["source"] != "quantum:test" {
		t.Fatalf("回执异常: %+v", out)
	}
}

// TestRunErrorNoFallback 取数失败必须报错——不回退伪随机。
func TestRunErrorNoFallback(t *testing.T) {
	c := chaos.New()
	c.SetEntropySource(fakeSource{name: "x", err: errors.New("boom")})
	if _, err := (trueRandom{}).Run(c, nil); err == nil {
		t.Fatal("取数失败应报错（不回退）")
	}
}

func TestRegistered(t *testing.T) {
	if _, ok := chaos.Lookup("true_random"); !ok {
		t.Fatal("true_random 未自注册")
	}
}
