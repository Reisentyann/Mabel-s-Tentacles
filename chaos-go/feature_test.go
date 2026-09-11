// 文件：chaos-go/feature_test.go —— 功能注册表单测：自注册 / 派发 / 参数归一
// 修改：2026-09-11（日期由 fresh-header.ps1 刷新）

package chaos

import "testing"

func TestMabelFeatureRegistered(t *testing.T) {
	f, ok := Lookup("mabel_quote")
	if !ok {
		t.Fatal("mabel_quote 未自注册")
	}
	if f.Name() != "mabel_quote" {
		t.Fatalf("功能名不符: %q", f.Name())
	}
	if len(f.Params()) == 0 {
		t.Fatal("mabel_quote 应声明 count 参数")
	}
}

func TestRunFeature(t *testing.T) {
	out, err := Run("mabel_quote", Params{"count": 2})
	if err != nil {
		t.Fatal(err)
	}
	lines, ok := out["lines"].([]string)
	if !ok || len(lines) != 2 {
		t.Fatalf("lines 形态异常: %#v", out["lines"])
	}
}

func TestRunFeatureJSONFloat(t *testing.T) {
	out, err := Run("mabel_quote", Params{"count": float64(3)})
	if err != nil {
		t.Fatal(err)
	}
	if lines, _ := out["lines"].([]string); len(lines) != 3 {
		t.Fatalf("float64 形参未归一，得到 %d 段", len(lines))
	}
}

func TestRunUnknownFeature(t *testing.T) {
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
