// 文件：describer-go/code/code.go —— cod-code 插件：源码语言 / import 提取 / TODO 计数
// 修改：2026-09-03（日期由 fresh-header.ps1 刷新）

// Package code cod-code 插件：源码文件的确定性事实。
// 字段字典见 docs/元数据字段说明.md 第 4.4 节。
package code

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Reisentyann/Mabel-s-Tentacles/describer-go"
)

func init() { describer.Register(descriptor{}) }

type descriptor struct{}

var codeLangs = map[string]string{
	".go": "go", ".py": "python", ".js": "javascript", ".ts": "typescript",
	".jsx": "javascript", ".tsx": "typescript", ".java": "java",
	".c": "c", ".h": "c", ".cpp": "cpp", ".hpp": "cpp", ".cc": "cpp",
	".rs": "rust", ".rb": "ruby", ".php": "php", ".sql": "sql",
	".sh": "shell", ".vue": "vue", ".css": "css", ".scss": "scss",
	".html": "html", ".swift": "swift", ".kt": "kotlin",
}

func (descriptor) Family() string         { return "code" }
func (descriptor) FamilyVersion() int     { return 1 }
func (descriptor) SPNamespaces() []string { return nil }
func (descriptor) Supports(path string, _ []byte, b describer.Basic) bool {
	if !b.Textish {
		return false
	}
	_, ok := codeLangs[strings.ToLower(filepath.Ext(path))]
	return ok
}

var (
	reGoImport    = regexp.MustCompile(`(?m)^\s*import\s+(?:_\s+|\w+\s+)?"([^"]+)"`)
	reGoBlock     = regexp.MustCompile(`(?m)^\s*"([^"]+)"\s*$`)
	reESFrom      = regexp.MustCompile(`(?m)^\s*(?:import|export)\s+[^\n]*?from\s+["']([^"']+)["']`)
	reESImport    = regexp.MustCompile(`(?m)^\s*import\s+["']([^"']+)["']`)
	reRequire     = regexp.MustCompile(`(?m)^\s*(?:const|let|var)\s+\w+(?:\s*=\s*)?\w*\s*=\s*require\(\s*["']([^"']+)["']`)
	rePyFrom      = regexp.MustCompile(`(?m)^\s*from\s+([\w.]+)\s+import\b`)
	rePyImport    = regexp.MustCompile(`(?m)^\s*import\s+([\w.]+)`)
	reCInclude    = regexp.MustCompile(`(?m)^\s*#\s*include\s+[<"]([^>"]+)[>"]`)
	reTODO        = regexp.MustCompile(`(?i)\b(TODO|FIXME)\b`)
	reGoImportBlk = regexp.MustCompile(`(?s)import\s*\(([^)]*)\)`)
)

func (descriptor) Analyze(in describer.Input, full []byte) (map[string]any, map[string]string) {
	src := string(full)
	lang := codeLangs[strings.ToLower(filepath.Ext(in.Path))]
	a := map[string]any{
		"cod-code-lang":       lang,
		"cod-code-todo-count": len(reTODO.FindAllString(src, -1)),
	}
	if imports := extractImports(lang, src); len(imports) > 0 {
		a["cod-code-imports"] = imports
	}
	return a, nil
}

// extractImports 提取前 10 条 import/require/include（去重保序）。
func extractImports(lang, src string) []string {
	var raw [][]string
	switch lang {
	case "go":
		raw = append(raw, reGoImport.FindAllStringSubmatch(src, -1)...)
		if blk := reGoImportBlk.FindStringSubmatch(src); blk != nil {
			raw = append(raw, reGoBlock.FindAllStringSubmatch(blk[1], -1)...)
		}
	case "python":
		raw = append(raw, rePyFrom.FindAllStringSubmatch(src, -1)...)
		raw = append(raw, rePyImport.FindAllStringSubmatch(src, -1)...)
	case "c", "cpp":
		raw = append(raw, reCInclude.FindAllStringSubmatch(src, -1)...)
	default: // js 系 / php / ruby 等
		raw = append(raw, reESFrom.FindAllStringSubmatch(src, -1)...)
		raw = append(raw, reESImport.FindAllStringSubmatch(src, -1)...)
		raw = append(raw, reRequire.FindAllStringSubmatch(src, -1)...)
	}

	const maxN = 10
	seen := map[string]bool{}
	var out []string
	for _, m := range raw {
		if len(m) < 2 {
			continue
		}
		mod := strings.TrimSpace(m[1])
		if mod == "" || seen[mod] {
			continue
		}
		seen[mod] = true
		out = append(out, mod)
		if len(out) >= maxN {
			break
		}
	}
	return out
}
