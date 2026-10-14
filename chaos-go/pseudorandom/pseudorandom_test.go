// 文件：chaos-go/pseudorandom/pseudorandom_test.go —— 伪随机机组件单测：数量/区间/默认值/自注册
// 修改：2026-09-13（日期由 fresh-header.ps1 刷新）

package pseudorandom

import (
	"testing"

	"github.com/Reisentyann/Mabel-s-Tentacles/chaos-go"
)

func run(t *testing.T, p chaos.Params) map[string]any {
	t.Helper()
	out, err := (pseudoRandom{}).Run(chaos.New(), p)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestDefaults(t *testing.T) {
	out := run(t, nil)
	vals, _ := out["values"].([]int)
	if len(vals) != 1 {
		t.Fatalf("默认应返回 1 个数，得到 %d", len(vals))
	}
	if v := vals[0]; v < 1 || v > 100 {
		t.Fatalf("默认区间 1-100，得到 %d", v)
	}
}

func TestCountClampAndRange(t *testing.T) {
	out := run(t, chaos.Params{"count": 1000, "min": 5, "max": 7})
	vals, _ := out["values"].([]int)
	if len(vals) != 100 {
		t.Fatalf("count 应夹到 100，得到 %d", len(vals))
	}
	for _, v := range vals {
		if v < 5 || v > 7 {
			t.Fatalf("值 %d 越界 [5,7]", v)
		}
	}
}

func TestRangeSwap(t *testing.T) {
	out := run(t, chaos.Params{"count": 20, "min": 9, "max": 2})
	if out["min"] != 2 || out["max"] != 9 {
		t.Fatalf("min>max 应交换，得到 min=%v max=%v", out["min"], out["max"])
	}
	for _, v := range out["values"].([]int) {
		if v < 2 || v > 9 {
			t.Fatalf("值 %d 越界", v)
		}
	}
}

func TestHugeRangeError(t *testing.T) {
	// int 溢出区间 → 报错而非 panic
	if _, err := (pseudoRandom{}).Run(chaos.New(), chaos.Params{"min": -1 << 62, "max": 1 << 62}); err == nil {
		t.Fatal("溢出区间应报错")
	}
}

func TestRegistered(t *testing.T) {
	if _, ok := chaos.Lookup("pseudo_random"); !ok {
		t.Fatal("pseudo_random 未自注册")
	}
}
