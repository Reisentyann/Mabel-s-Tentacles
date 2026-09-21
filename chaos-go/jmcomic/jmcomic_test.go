// 文件：chaos-go/jmcomic/jmcomic_test.go —— 混沌机组件·JMComic 单元测试：参数定义与自注册核验
// 修改：2026-09-21（日期由 fresh-header.ps1 刷新）

package jmcomic_test

import (
	"testing"

	"github.com/Reisentyann/Mabel-s-Tentacles/chaos-go"
	_ "github.com/Reisentyann/Mabel-s-Tentacles/chaos-go/jmcomic"
)

func TestJMComicRegistration(t *testing.T) {
	f, ok := chaos.Lookup("jm_comic")
	if !ok {
		t.Fatalf("expected jm_comic to be registered, but not found")
	}

	if f.Name() != "jm_comic" {
		t.Errorf("expected name 'jm_comic', got '%s'", f.Name())
	}

	params := f.Params()
	if len(params) == 0 {
		t.Errorf("expected params to be defined")
	}

	hasJmID := false
	for _, p := range params {
		if p.Name == "jm_id" && p.Required {
			hasJmID = true
			break
		}
	}
	if !hasJmID {
		t.Errorf("expected required param 'jm_id' in Params()")
	}
}

func TestJMComicParamValidation(t *testing.T) {
	f, ok := chaos.Lookup("jm_comic")
	if !ok {
		t.Fatalf("feature jm_comic not registered")
	}

	c := chaos.New()
	_, err := f.Run(c, chaos.Params{})
	if err == nil {
		t.Errorf("expected error when running with empty params, got nil")
	}

	_, err = f.Run(c, chaos.Params{
		"jm_id":  "123",
		"action": "invalid_action",
	})
	if err == nil {
		t.Errorf("expected error for invalid action, got nil")
	}
}
