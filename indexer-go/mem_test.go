// 文件：indexer-go/mem_test.go —— 索引机单元测试：三型桶查询 / And-Or 组合 / Update diff 幂等 / Rebuild / 脏值 / 并发 / Stats 自省
// 修改：2026-09-11（日期由 fresh-header.ps1 刷新）

package indexer

import (
	"fmt"
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

// scanAll 扫描型 op 样例库（2026-09-10 检索语言扩展）：键存在性差异
// （u3 无 upper-ratio——分母 0 不产键）、字符串字段（title-line）、
// 数组字段（headings，元素级 contains）、ne 的缺键语义。
func scanAll() map[string]map[string]any {
	return map[string]map[string]any{
		"u1": {
			"cod-text-language":  "zh",
			"cod-text-title-line": "# 归档计划",
			"cod-text-headings":  []any{"归档计划", "待办"},
			"cod-text-upper-ratio": 0.12,
		},
		"u2": {
			"cod-text-language":  "en",
			"cod-text-title-line": "# Chapter Two",
			"cod-text-headings":  []any{"Chapter Two"},
			"cod-text-upper-ratio": 0.56,
		},
		"u3": {
			"cod-text-language":  "zh",
			"cod-text-title-line": "第二章 深夜来电",
			// 无 upper-ratio（纯中文无拉丁字母——分母 0 不产键）
		},
	}
}

func TestQueryScanOps(t *testing.T) {
	ix := New()
	if err := ix.Rebuild(scanAll()); err != nil {
		t.Fatal(err)
	}

	// exists：键存在性（镜像支撑——缺键信息桶里不可见）
	if got := mustQuery(t, ix, []Condition{{Field: "cod-text-upper-ratio", Op: OpExists, Value: true}}, And); !reflect.DeepEqual(got, []string{"u1", "u2"}) {
		t.Fatalf("exists upper-ratio=true → %v, want [u1 u2]", got)
	}
	if got := mustQuery(t, ix, []Condition{{Field: "cod-text-upper-ratio", Op: OpExists, Value: false}}, And); !reflect.DeepEqual(got, []string{"u3"}) {
		t.Fatalf("exists upper-ratio=false → %v, want [u3]", got)
	}
	// exists 对从未出现的字段：false = 全集（镜像是唯一裁判）
	if got := mustQuery(t, ix, []Condition{{Field: "cod-image-taken-at", Op: OpExists, Value: false}}, And); !reflect.DeepEqual(got, []string{"u1", "u2", "u3"}) {
		t.Fatalf("exists 从未出现字段=false → %v, want 全集", got)
	}

	// ne：有键且值不同；缺键不算（语义上"无键"≠"值不等于"）
	if got := mustQuery(t, ix, []Condition{{Field: "cod-text-language", Op: OpNe, Value: "zh"}}, And); !reflect.DeepEqual(got, []string{"u2"}) {
		t.Fatalf("language ne zh → %v, want [u2]", got)
	}
	if got := mustQuery(t, ix, []Condition{{Field: "cod-text-upper-ratio", Op: OpNe, Value: 0.12}}, And); !reflect.DeepEqual(got, []string{"u2"}) {
		t.Fatalf("upper-ratio ne 0.12 → %v, want [u2]（u1 同值不算，u3 缺键不算）", got)
	}
	// ne 数值：给 u1 增补 lines=420（走 Update 增量——镜像同步受验）
	u1New := map[string]any{}
	for k, v := range scanAll()["u1"] {
		u1New[k] = v
	}
	u1New["cod-text-lines"] = 420
	ix.Update("u1", scanAll()["u1"], u1New)
	if got := mustQuery(t, ix, []Condition{{Field: "cod-text-lines", Op: OpNe, Value: 88}}, And); !reflect.DeepEqual(got, []string{"u1"}) {
		t.Fatalf("lines ne 88 → %v, want [u1]", got)
	}

	// contains：字符串字段子串
	if got := mustQuery(t, ix, []Condition{{Field: "cod-text-title-line", Op: OpContains, Value: "归档"}}, And); !reflect.DeepEqual(got, []string{"u1"}) {
		t.Fatalf("title-line contains 归档 → %v, want [u1]", got)
	}
	// contains：数组字段元素级（u2 的 headings 含 Chapter）
	if got := mustQuery(t, ix, []Condition{{Field: "cod-text-headings", Op: OpContains, Value: "Chapter"}}, And); !reflect.DeepEqual(got, []string{"u2"}) {
		t.Fatalf("headings contains Chapter → %v, want [u2]", got)
	}

	// 扫描型 op 与桶型 op 混合 And：zh 且 title 含 归档
	if got := mustQuery(t, ix, []Condition{
		{Field: "cod-text-language", Op: OpEq, Value: "zh"},
		{Field: "cod-text-title-line", Op: OpContains, Value: "归档"},
	}, And); !reflect.DeepEqual(got, []string{"u1"}) {
		t.Fatalf("zh ∧ contains 归档 → %v, want [u1]", got)
	}
}

// TestScanMirrorUpdateSync 镜像与桶在 Update 增量下不漂移：键消失 →
// exists 翻转；整体移除 → 出镜。
func TestScanMirrorUpdateSync(t *testing.T) {
	ix := New()
	all := scanAll()
	if err := ix.Rebuild(all); err != nil {
		t.Fatal(err)
	}
	// 增量：u1 去掉 upper-ratio（键消失）
	old := all["u1"]
	newAttrs := map[string]any{}
	for k, v := range old {
		if k != "cod-text-upper-ratio" {
			newAttrs[k] = v
		}
	}
	ix.Update("u1", old, newAttrs)
	if got := mustQuery(t, ix, []Condition{{Field: "cod-text-upper-ratio", Op: OpExists, Value: true}}, And); !reflect.DeepEqual(got, []string{"u2"}) {
		t.Fatalf("键消失后 exists=true → %v, want [u2]", got)
	}
	// 整体移除：u2 出镜
	ix.Update("u2", all["u2"], nil)
	if got := mustQuery(t, ix, []Condition{{Field: "cod-text-language", Op: OpExists, Value: true}}, And); !reflect.DeepEqual(got, []string{"u1", "u3"}) {
		t.Fatalf("整体移除后 language exists → %v, want [u1 u3]", got)
	}
	if got := mustQuery(t, ix, eq("cod-text-language", "en"), And); len(got) != 0 {
		t.Fatalf("移除后 language=en 应无命中 → %v", got)
	}
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

func TestStats(t *testing.T) {
	// 空索引：全零
	ix := New()
	if s := ix.Stats(); s.Fields != 0 || s.Keys != 0 || s.Mounts != 0 {
		t.Fatalf("空索引 Stats = %+v, want 全零", s)
	}

	// sampleAll：4 桶（language·enum / lines·num / textish·enum / tags·multi）
	// keys：language 2 + textish 2 + lines 3 + tags 3 = 10
	// mounts：u1 5 + u2 4 + u3 3 = 12（tags 多值字段一桶多挂）
	if err := ix.Rebuild(sampleAll()); err != nil {
		t.Fatal(err)
	}
	if s := ix.Stats(); s.Fields != 4 || s.Keys != 10 || s.Mounts != 12 {
		t.Fatalf("Rebuild 后 Stats = %+v, want {4 10 12}", s)
	}

	// 增量挂载：language 新值 ja → 键 +1、挂载 +1
	ix.Update("u4", nil, map[string]any{"cod-text-language": "ja"})
	if s := ix.Stats(); s.Keys != 11 || s.Mounts != 13 {
		t.Fatalf("增挂后 Stats = %+v, want Keys=11 Mounts=13", s)
	}

	// 值变：旧值移除新值挂入 → 挂载数不变（一移一挂）
	ix.Update("u4", map[string]any{"cod-text-language": "ja"}, map[string]any{"cod-text-language": "zh"})
	if s := ix.Stats(); s.Mounts != 13 {
		t.Fatalf("值变后 Stats = %+v, want Mounts=13", s)
	}

	// 删除路径：整体移除 → 挂载 -1
	ix.Update("u4", map[string]any{"cod-text-language": "zh"}, nil)
	if s := ix.Stats(); s.Mounts != 12 {
		t.Fatalf("移除后 Stats = %+v, want Mounts=12", s)
	}

	// 幂等：重复喂食同一状态不重复挂载
	ix.Update("u5", nil, map[string]any{"cod-text-language": "zh"})
	ix.Update("u5", nil, map[string]any{"cod-text-language": "zh"})
	if s := ix.Stats(); s.Mounts != 13 {
		t.Fatalf("幂等喂食后 Stats = %+v, want Mounts=13", s)
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

// findField 目录检索助手。
func findField(t *testing.T, cat []FieldInfo, field string) FieldInfo {
	t.Helper()
	for _, fi := range cat {
		if fi.Field == field {
			return fi
		}
	}
	t.Fatalf("目录缺字段 %s（共 %d 项）", field, len(cat))
	return FieldInfo{}
}

func TestCatalog(t *testing.T) {
	ix := New()
	if got := ix.Catalog(); len(got) != 0 {
		t.Fatalf("空索引目录应为空切片 → %v", got)
	}
	if err := ix.Rebuild(sampleAll()); err != nil {
		t.Fatal(err)
	}
	cat := ix.Catalog()
	if len(cat) != 4 {
		t.Fatalf("目录字段数 = %d, want 4", len(cat))
	}
	// 字段名升序（确定性）：cod-basic-textish < cod-text-language < cod-text-lines < tags
	for i := 1; i < len(cat); i++ {
		if cat[i-1].Field >= cat[i].Field {
			t.Fatalf("目录未按字段名升序：%s ≥ %s", cat[i-1].Field, cat[i].Field)
		}
	}

	lang := findField(t, cat, "cod-text-language")
	if lang.Kind != KindEnum || !reflect.DeepEqual(lang.Values, []string{"en", "zh"}) {
		t.Fatalf("language 目录项 = %+v, want enum{en,zh}", lang)
	}
	if lang.Keys != 2 || lang.Mounts != 3 {
		t.Fatalf("language Keys/Mounts = %d/%d, want 2/3", lang.Keys, lang.Mounts)
	}

	lines := findField(t, cat, "cod-text-lines")
	if lines.Kind != KindNum || lines.Min == nil || lines.Max == nil {
		t.Fatalf("lines 目录项 = %+v, want num{min,max}", lines)
	}
	if *lines.Min != 12 || *lines.Max != 420 {
		t.Fatalf("lines 值域 = [%v,%v], want [12,420]", *lines.Min, *lines.Max)
	}

	textish := findField(t, cat, "cod-basic-textish")
	if textish.Kind != KindEnum || !reflect.DeepEqual(textish.Values, []string{"false", "true"}) {
		t.Fatalf("textish 目录项 = %+v, want enum{false,true}", textish)
	}

	tags := findField(t, cat, "tags")
	if tags.Kind != KindMulti || !reflect.DeepEqual(tags.Values, []string{"梅贝尔", "触手", "铃仙"}) {
		t.Fatalf("tags 目录项 = %+v, want multi{码点升序}", tags)
	}

	// 目录随喂食动态更新：新值挂入后取值列表应包含它
	ix.Update("u9", nil, map[string]any{"cod-text-language": "ja"})
	lang2 := findField(t, ix.Catalog(), "cod-text-language")
	if !reflect.DeepEqual(lang2.Values, []string{"en", "ja", "zh"}) {
		t.Fatalf("喂食 ja 后 language 取值 = %v", lang2.Values)
	}
	// 整体移除后目录不再含该值
	ix.Update("u9", map[string]any{"cod-text-language": "ja"}, nil)
	if lang3 := findField(t, ix.Catalog(), "cod-text-language"); !reflect.DeepEqual(lang3.Values, []string{"en", "zh"}) {
		t.Fatalf("移除 ja 后 language 取值 = %v", lang3.Values)
	}
}

func TestCatalogTruncate(t *testing.T) {
	ix := New()
	all := map[string]map[string]any{}
	// 造超上限取值的字段：CatalogValueLimit+7 个互异值
	const extra = 7
	for i := 0; i < CatalogValueLimit+extra; i++ {
		all[fmt.Sprintf("u%03d", i)] = map[string]any{"f": fmt.Sprintf("v%03d", i)}
	}
	if err := ix.Rebuild(all); err != nil {
		t.Fatal(err)
	}
	fi := findField(t, ix.Catalog(), "f")
	if len(fi.Values) != CatalogValueLimit || !fi.Truncated {
		t.Fatalf("截断保护失效：len=%d truncated=%v", len(fi.Values), fi.Truncated)
	}
	if fi.Keys != CatalogValueLimit+extra {
		t.Fatalf("Keys 应为全量键数 %d, got %d", CatalogValueLimit+extra, fi.Keys)
	}
}
