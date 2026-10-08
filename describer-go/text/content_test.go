// 文件：describer-go/text/content_test.go —— 内容定位字段：围栏隔离、标题层级、表头验证、链接归一与上限
// 修改：2026-10-08（日期由 fresh-header.ps1 刷新）

package text

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/Reisentyann/Mabel-s-Tentacles/describer-go"
)

func TestContentFacts(t *testing.T) {
	src := "# 部署\n## 数据库\n### 备份 ###\n## 日志\n" +
		"[文档](https://EXAMPLE.org:443/a) https://example.org/b https://go.dev/doc。\n" +
		"````Go\n# 假标题\n| 假 | 表 |\n|---|---|\n```\n````\n~~~SQL\nselect 1;\n~~~\n" +
		"| 角色 | 技能\\|效果 |\n|:---|---:|\n| 女巫 | 解药 |\n" +
		"| 不是 | 表头 |\n|bad|---|\n"
	a, _ := (descriptor{}).Analyze(describer.Input{Size: int64(len(src))}, []byte(src))
	for key, want := range map[string][]string{
		"link-domains":   {"example.org", "go.dev"},
		"code-languages": {"go", "sql"},
		"section-paths":  {"部署", "部署 / 数据库", "部署 / 数据库 / 备份", "部署 / 日志"},
		"table-headers":  {"角色", "技能|效果"},
	} {
		if !reflect.DeepEqual(a["cod-text-"+key], want) {
			t.Errorf("%s = %#v, want %#v", key, a["cod-text-"+key], want)
		}
	}
	if a["cod-text-partial"] != false {
		t.Fatal("complete content marked partial")
	}
}

func TestContentLimitsAndAbsence(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 35; i++ {
		fmt.Fprintf(&b, "https://host%d.example/a\n", i)
	}
	a, _ := (descriptor{}).Analyze(describer.Input{Size: 9999}, []byte(b.String()))
	if len(a["cod-text-link-domains"].([]string)) != 30 || a["cod-text-link-domains-truncated"] != true || a["cod-text-partial"] != true {
		t.Fatalf("limits: %+v", a)
	}
	a, _ = (descriptor{}).Analyze(describer.Input{}, []byte("ordinary prose"))
	for _, key := range []string{"link-domains", "code-languages", "section-paths", "table-headers"} {
		if _, ok := a["cod-text-"+key]; ok {
			t.Errorf("unexpected %s", key)
		}
	}
}
