package tools

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

func TestEverySuccessStepStartsAsASentence(t *testing.T) {
	files, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	checked := 0
	for _, f := range files {
		name := f.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 2 {
				return true
			}
			if fn, ok := call.Fun.(*ast.Ident); !ok || fn.Name != "success" {
				return true
			}
			lit, ok := leadingString(call.Args[0])
			if !ok {
				return true
			}
			checked++
			if lit == "" || !unicode.IsUpper([]rune(lit)[0]) {
				t.Errorf("%s: success step %q is not a sentence starting with a capital", fset.Position(call.Pos()), lit)
			}
			return true
		})
	}
	if checked < 90 {
		t.Fatalf("checked only %d success steps; the parse no longer finds the handlers", checked)
	}
}

func leadingString(e ast.Expr) (string, bool) {
	switch e := e.(type) {
	case *ast.BasicLit:
		if e.Kind != token.STRING {
			return "", false
		}
		s, err := strconv.Unquote(e.Value)
		return s, err == nil
	case *ast.BinaryExpr:
		return leadingString(e.X)
	case *ast.CallExpr:
		if sel, ok := e.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Sprintf" && len(e.Args) > 0 {
			format, ok := leadingString(e.Args[0])
			return format, ok && !strings.HasPrefix(format, "%")
		}
	}
	return "", false
}
