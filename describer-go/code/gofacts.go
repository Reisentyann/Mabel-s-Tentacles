// 文件：describer-go/code/gofacts.go —— Go AST 内容定位：顶层声明及 os 环境变量字面量读取
// 修改：2026-10-08（日期由 fresh-header.ps1 刷新）

package code

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"unicode/utf8"
)

func goContentFacts(src []byte, attrs map[string]any) {
	f, err := parser.ParseFile(token.NewFileSet(), "", src, 0)
	if err != nil {
		return
	}
	values := map[string][]string{"env-vars": {}, "declared-symbols": {}}
	seen := map[string]map[string]bool{"env-vars": {}, "declared-symbols": {}}
	truncated := map[string]bool{}
	add := func(k, v string) {
		if v == "" || v == "_" || seen[k][v] {
			return
		}
		if utf8.RuneCountInString(v) > 200 || len(values[k]) >= 30 {
			truncated[k] = true
			return
		}
		seen[k][v] = true
		values[k] = append(values[k], v)
	}
	aliases := map[string]bool{}
	for _, imp := range f.Imports {
		p, _ := strconv.Unquote(imp.Path.Value)
		if p != "os" {
			continue
		}
		name := "os"
		if imp.Name != nil {
			name = imp.Name.Name
		}
		if name != "_" && name != "." {
			aliases[name] = true
		}
	}
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			name := d.Name.Name
			if d.Recv != nil && len(d.Recv.List) > 0 {
				if receiver := receiverName(d.Recv.List[0].Type); receiver != "" {
					name = receiver + "." + name
				}
			}
			add("declared-symbols", name)
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					add("declared-symbols", s.Name.Name)
				case *ast.ValueSpec:
					for _, name := range s.Names {
						add("declared-symbols", name.Name)
					}
				}
			}
		}
	}
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) != 1 {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || (sel.Sel.Name != "Getenv" && sel.Sel.Name != "LookupEnv") {
			return true
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok || !aliases[pkg.Name] || pkg.Obj != nil {
			return true
		}
		lit, ok := call.Args[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		v, err := strconv.Unquote(lit.Value)
		if err == nil {
			add("env-vars", v)
		}
		return true
	})
	for _, key := range []string{"env-vars", "declared-symbols"} {
		if len(values[key]) > 0 {
			attrs["cod-code-"+key] = values[key]
		}
		if len(values[key]) > 0 || truncated[key] {
			attrs["cod-code-"+key+"-truncated"] = truncated[key]
		}
	}
}

func receiverName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.StarExpr:
		return receiverName(e.X)
	case *ast.IndexExpr:
		return receiverName(e.X)
	case *ast.IndexListExpr:
		return receiverName(e.X)
	case *ast.ParenExpr:
		return receiverName(e.X)
	}
	return ""
}
