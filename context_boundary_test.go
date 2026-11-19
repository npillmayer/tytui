package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// Contexts belong only to streaming and font discovery/loading. Background
// contexts at entry points must not grow back into typesetting API parameters.
func TestContextBoundary(t *testing.T) {
	imports := map[string]bool{"main.go": true, "fonts.go": true, "font_requests.go": true, "fontselection/discovery.go": true, "fontselection/providers.go": true, "fontselection/selection.go": true, "replay/replay.go": true, "tycli/main.go": true, "document_workbench.go": true}
	contextTypes := map[string]bool{"fonts.go": true, "font_requests.go": true, "fontselection/discovery.go": true, "fontselection/providers.go": true, "fontselection/selection.go": true, "tycli/main.go": true}
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != "." && (strings.HasPrefix(d.Name(), ".") || d.Name() == "testdata" || d.Name() == "vendor") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		for _, imp := range f.Imports {
			if imp.Path.Value != `"context"` {
				continue
			}
			if !imports[path] {
				t.Errorf("context import outside streaming/loading boundary: %s", path)
			}
			qualifier := "context"
			if imp.Name != nil {
				qualifier = imp.Name.Name
			}
			if qualifier == "." {
				t.Errorf("dot context import obscures boundary: %s", path)
			}
			ast.Inspect(f, func(n ast.Node) bool {
				s, ok := n.(*ast.SelectorExpr)
				if !ok || s.Sel.Name != "Context" {
					return true
				}
				q, ok := s.X.(*ast.Ident)
				if ok && q.Name == qualifier && !contextTypes[path] {
					t.Errorf("context type outside streaming/loading API: %s", path)
				}
				return true
			})
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
