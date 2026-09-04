// 文件：indexer-go/mem_test.go —— 索引机单元测试：三型桶查询 / And-Or 组合 / Update diff 幂等 / Rebuild / 脏值 / 并发
// 修改：2026-09-03（日期由 fresh-header.ps1 刷新）

package indexer

import (
	"reflect"
	"sync"
	"testing"
)

// sampleAll 三文件样例库，覆盖三型桶（enum / num / multi）与 bool 字段。
func sampleAll() map[string]map[string]any {
	return map[string]map[string]any{
		"u1": {
			"cod-text-language": "zh",
			"cod-text-lines":    420,
			"cod-basic-textish": true,
			"tags":              []any{"梅贝尔", "触手"},
		},
		"u2": {
			"cod-text-language": "en",
			"cod-text-lines":    12,
			"cod-basic-textish": true,
			"tags":              []any{"铃仙"},
		},
		"u3": {
			"cod-text-language": "zh",
			"cod-text-lines":    88,
			"cod-basic-textish": false,
		},
	}
}

func mustQuery(t *testing.T, ix Indexer, conds []Condition, mode Combine) []string {
	t.Helper()
	out, err := ix.Query(conds, mode)
	if err != nil {
		t.Fatalf("Query(%#v, %v) 出错: %v", conds, mode, err)
	}
	return out
}

func eq(field string, v any) []Condition {
	return []Condition{{Field: field, Op: OpEq, Value: v}}
}

func TestQueryEnum(t *testing.T) {
	ix := New()
	if err := ix.Rebuild(sampleAll()); err != nil {
		t.Fatal(err)
	}
	if got := mustQuery(t, ix, eq("cod-text-language", "zh"), And); !reflect.DeepEqual(got, []string{"u1", "u3"}) {
		t.Fatalf("language=zh → %v", got)
	}
	if got := mustQuery(t, ix, eq("cod-basic-textish", true), And); !reflect.DeepEqual(got, []string{"u1", "u2"}) {
		t.Fatalf("textish=true → %v", got)
	}
	if got := mustQuery(t, ix, eq("cod-text-language", "ja"), And); len(got) != 0 {
		t.Fatalf("language=ja 应无命中 → %v", got)
	}
	if got := mustQuery(t, ix,
		[]Condition{{Field: "cod-text-language", Op: OpIn, Value: []any{"en", "ja"}}}, And); !reflect.DeepEqual(got, []string{"u2"}) {
		t.Fatalf("language in [en,ja] → %v", got)
	}
}

func TestQueryNum(t *testing.T) {
	ix := New()
	if err := ix.Rebuild(sampleAll()); err != nil {
		t.Fatal(err)
	}
	if got := mustQuery(t, ix, []Condition{{Field: "cod-text-lines", Op: OpGt, Value: 100}}, And); !reflect.DeepEqual(got, []string{"u1"}) {
		t.Fatalf("lines>100 → %v", got)
	}
	if got := mustQuery(t, ix, []Condition{{Field: "cod-text-lines", Op: OpLt, Value: 100}}, And); !reflect.DeepEqual(got, []string{"u2", "u3"}) {
		t.Fatalf("lines<100 → %v", got)
	}
	// range 闭区间：88 与 420 两端都命中
	if got := mustQuery(t, ix, []Condition{{Field: "cod-text-lines", Op: OpRange, Value: [2]any{88, 420}}}, And); !reflect.DeepEqual(got, []string{"u1", "u3"}) {
		t.Fatalf("range [88,420] → %v", got)
	}
	if got := mustQuery(t, ix, eq("cod-text-lines", 12), And); !reflect.DeepEqual(got, []string{"u2"}) {
		t.Fatalf("lines=12 → %v", got)
	}
	// []any 形态的 range；lo>hi 空区间无命中不报错
	if got := mustQuery(t, ix, []Condition{{Field: "cod-text-lines", Op: OpRange, Value: []any{100, 88}}}, And); len(got) != 0 {
		t.Fatalf("空区间 → %v", got)
	}
	// in
	if got := mustQuery(t, ix, []Condition{{Field: "cod-text-lines", Op: OpIn, Value: []any{12, 88}}}, And); !reflect.DeepEqual(got, []string{"u2", "u3"}) {
		t.Fatalf("lines in [12,88] → %v", got)
	}
}

func TestQueryMulti(t *testing.T) {
	ix := New()
	if err := ix.Rebuild(sampleAll()); err != nil {
		t.Fatal(err)
	}
	if got := mustQuery(t, ix, eq("tags", "梅贝尔"), And); !reflect.DeepEqual(got, []string{"u1"}) {
		t.Fatalf("tags 含 梅贝尔 → %v", got)
	}
	// in 命中任一元素即整文件命中
	if got := mustQuery(t, ix, []Condition{{Field: "tags", Op: OpIn, Value: []any{"梅贝尔", "铃仙"}}}, And); !reflect.DeepEqual(got, []string{"u1", "u2"}) {
		t.Fatalf("tags in [梅贝尔,铃仙] → %v", got)
	}
	if got := mustQuery(t, ix, eq("tags", "无此标签"), And); len(got) != 0 {
		t.Fatalf("tags 无命中 → %v", got)
	}
}

func TestQueryCombine(t *testing.T) {
	ix := New()
	if err := ix.Rebuild(sampleAll()); err != nil {
		t.Fatal(err)
	}
	conds := []Condition{
		{Field: "cod-text-language", Op: OpEq, Value: "zh"},
		{Field: "cod-text-lines", Op: OpGt, Value: 100},
	}
	if got := mustQuery(t, ix, conds, And); !reflect.DeepEqual(got, []string{"u1"}) {
		t.Fatalf("And zh且lines>100 → %v", got)
	}
	if got := mustQuery(t, ix, conds, Or); !reflect.DeepEqual(got, []string{"u1", "u3"}) {
		t.Fatalf("Or zh或lines>100 → %v", got)
	}
	// 未知字段：And 归零 / Or 不受影响
	withUK := append(conds, Condition{Field: "cod-no-such-field", Op: OpEq, Value: "x"})
	if got := mustQuery(t, ix, withUK, And); len(got) != 0 {
		t.Fatalf("And 含未知字段 → %v", got)
	}
	if got := mustQuery(t, ix, withUK, Or); !reflect.DeepEqual(got, []string{"u1", "u3"}) {
		t.Fatalf("Or 含未知字段 → %v", got)
	}
}

func TestQueryEmptyConds(t *testing.T) {
	ix := New()
	if err := ix.Rebuild(sampleAll()); err != nil {
		t.Fatal(err)
	}
	got, err := ix.Query(nil, And)
	if err != nil || len(got) != 0 {
		t.Fatalf("空条件应返回空集不报错，got %v err %v", got, err)
	}
}

func TestQueryOpMismatch(t *testing.T) {
	ix := New()
	if err := ix.Rebuild(sampleAll()); err != nil {
		t.Fatal(err)
	}
	// 枚举桶收 range → 报错（上层降级 SQL 兜底）
	if _, err := ix.Query([]Condition{{Field: "cod-text-language", Op: OpRange, Value: [2]any{1, 2}}}, And); err == nil {
		t.Fatal("枚举桶收 range 应报错")
	}
	// 数值桶收非数值 → 报错
	if _, err := ix.Query([]Condition{{Field: "cod-text-lines", Op: OpGt, Value: "很多"}}, And); err == nil {
		t.Fatal("数值桶收字符串值应报错")
	}
}

func TestUpdateDiff(t *testing.T) {
	ix := New()
	first := map[string]any{"cod-text-language": "zh", "tags": []any{"a", "b"}, "cod-text-lines": 10}

	// 初挂
	ix.Update("u1", nil, first)
	if got := mustQuery(t, ix, eq("cod-text-language", "zh"), And); !reflect.DeepEqual(got, []string{"u1"}) {
		t.Fatalf("初挂后 zh → %v", got)
	}

	// 值变：language zh→en；tags a 移除 c 新挂 b 保留；lines 没动不碰桶
	ix.Update("u1", first, map[string]any{
		"cod-text-language": "en",
		"tags":              []any{"b", "c"},
		"cod-text-lines":    10,
	})
	if got := mustQuery(t, ix, eq("cod-text-language", "zh"), And); len(got) != 0 {
		t.Fatalf("旧值 zh 应已移除 → %v", got)
	}
	if got := mustQuery(t, ix, eq("cod-text-language", "en"), And); !reflect.DeepEqual(got, []string{"u1"}) {
		t.Fatalf("新值 en → %v", got)
	}
	if got := mustQuery(t, ix, eq("tags", "a"), And); len(got) != 0 {
		t.Fatalf("tags 旧元素 a 应已移除 → %v", got)
	}
	if got := mustQuery(t, ix, eq("tags", "c"), And); len(got) != 1 {
		t.Fatalf("tags 新元素 c 应在桶 → %v", got)
	}
	if got := mustQuery(t, ix, eq("tags", "b"), And); len(got) != 1 {
		t.Fatalf("tags 未变元素 b 应保留 → %v", got)
	}
	if got := mustQuery(t, ix, eq("cod-text-lines", 10), And); len(got) != 1 {
		t.Fatalf("lines 值没动应保留 → %v", got)
	}

	// 键消失：language 从新值中删掉
	ix.Update("u1",
		map[string]any{"cod-text-language": "en", "tags": []any{"b", "c"}, "cod-text-lines": 10},
		map[string]any{"tags": []any{"b", "c"}, "cod-text-lines": 10})
	if got := mustQuery(t, ix, eq("cod-text-language", "en"), And); len(got) != 0 {
		t.Fatalf("键消失应移除 → %v", got)
	}

	// new=nil 整体移除（文件删除路径）
	ix.Update("u1", map[string]any{"tags": []any{"b", "c"}, "cod-text-lines": 10}, nil)
	if got := mustQuery(t, ix, eq("cod-text-lines", 10), And); len(got) != 0 {
		t.Fatalf("删除路径 lines 应移除 → %v", got)
	}
	if got := mustQuery(t, ix, eq("tags", "b"), And); len(got) != 0 {
		t.Fatalf("删除路径 tags 应移除 → %v", got)
	}

	// 幂等：重复喂食同一状态，结果恒等
	ix.Update("u2", nil, map[string]any{"cod-text-language": "zh"})
	ix.Update("u2", nil, map[string]any{"cod-text-language": "zh"})
	if got := mustQuery(t, ix, eq("cod-text-language", "zh"), And); !reflect.DeepEqual(got, []string{"u2"}) {
		t.Fatalf("重复喂食后 zh → %v", got)
	}
}

func TestRebuild(t *testing.T) {
	ix := New()
	if err := ix.Rebuild(sampleAll()); err != nil {
		t.Fatal(err)
	}
	// 换数据全量重建：旧数据彻底消失
	if err := ix.Rebuild(map[string]map[string]any{
		"u9": {"cod-text-language": "ja"},
	}); err != nil {
		t.Fatal(err)
	}
	if got := mustQuery(t, ix, eq("cod-text-language", "zh"), And); len(got) != 0 {
		t.Fatalf("rebuild 后旧数据应清空 → %v", got)
	}
	if got := mustQuery(t, ix, eq("cod-text-language", "ja"), And); !reflect.DeepEqual(got, []string{"u9"}) {
		t.Fatalf("rebuild 后新数据 → %v", got)
	}
}

func TestDirtyValues(t *testing.T) {
	ix := New()
	// 对象值与 nil 不入索引；纯对象数组（cod-image-palette 形态）不建桶
	ix.Update("u1", nil, map[string]any{
		"cod-image-palette": []any{map[string]any{"hex": "#fff", "ratio": 0.5}},
		"cod-image-note":    nil,
		"cod-text-lines":    3,
	})
	if got := mustQuery(t, ix, eq("cod-text-lines", 3), And); !reflect.DeepEqual(got, []string{"u1"}) {
		t.Fatalf("标量应正常挂载 → %v", got)
	}
	if _, err := ix.Query(eq("cod-image-palette", "#fff"), And); err != nil {
		t.Fatalf("纯对象数组字段查询应为空集不报错: %v", err)
	}
	// 数字字符串入数值桶可查（宽松归一）
	if got := mustQuery(t, ix, eq("cod-text-lines", "3"), And); !reflect.DeepEqual(got, []string{"u1"}) {
		t.Fatalf("数字字符串查询 → %v", got)
	}
	// []string（describer 原生）与 []any（DB JSON）同形
	ix.Update("u2", nil, map[string]any{"tags": []string{"x"}})
	if got := mustQuery(t, ix, eq("tags", "x"), And); !reflect.DeepEqual(got, []string{"u2"}) {
		t.Fatalf("[]string 挂载 → %v", got)
	}
	// 同值异型 diff 为不变：[]string → []any 不重挂不出错
	ix.Update("u2", map[string]any{"tags": []string{"x"}}, map[string]any{"tags": []any{"x"}})
	if got := mustQuery(t, ix, eq("tags", "x"), And); !reflect.DeepEqual(got, []string{"u2"}) {
		t.Fatalf("同值异型 → %v", got)
	}
	// int 与 float64 同值：diff 为不变
	ix.Update("u3", nil, map[string]any{"cod-text-lines": 7})
	ix.Update("u3", map[string]any{"cod-text-lines": 7}, map[string]any{"cod-text-lines": float64(7)})
	if got := mustQuery(t, ix, eq("cod-text-lines", 7), And); !reflect.DeepEqual(got, []string{"u3"}) {
		t.Fatalf("int/float64 同值 → %v", got)
	}
}

func TestConcurrentQueryUpdate(t *testing.T) {
	// go test -race 下验证 RWMutex：单写者链条自洽变值，多读者并发查询。
	ix := New()
	if err := ix.Rebuild(sampleAll()); err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	var writers sync.WaitGroup
	writers.Add(1)
	go func() {
		defer writers.Done()
		old := map[string]any{"cod-text-lines": 420}
		for j := 1; ; j++ {
			select {
			case <-stop:
				return
			default:
			}
			nv := map[string]any{"cod-text-lines": float64(j%50 + 1)}
			ix.Update("u1", old, nv)
			old = nv
		}
	}()

	var readers sync.WaitGroup
	for i := 0; i < 4; i++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for j := 0; j < 300; j++ {
				got, err := ix.Query([]Condition{{Field: "cod-text-lines", Op: OpGt, Value: 0}}, And)
				if err != nil {
					t.Errorf("concurrent query: %v", err)
					return
				}
				for _, u := range got {
					if u != "u1" && u != "u2" && u != "u3" {
						t.Errorf("unexpected uuid %q", u)
						return
					}
				}
			}
		}()
	}
	readers.Wait()
	close(stop)
	writers.Wait()

	// 终态：u1 仍可查（挂在某一轮的值上）
	if got := mustQuery(t, ix, []Condition{{Field: "cod-text-lines", Op: OpGt, Value: -1}}, And); len(got) != 3 {
		t.Fatalf("终态三文件应全部可查 → %v", got)
	}
}
