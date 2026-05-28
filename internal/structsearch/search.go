package structsearch

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
)

type Query struct {
	Language string
	Kind     string
	Name     string
}

type Result struct {
	Name string
	Kind string
	Path string
	Line int
}

func Search(root string, q Query) ([]Result, error) {
	var out []Result
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return nil
		}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			if q.Name != "" && fn.Name.Name != q.Name {
				continue
			}
			pos := fset.Position(fn.Pos())
			out = append(out, Result{Name: fn.Name.Name, Kind: "function", Path: path, Line: pos.Line})
		}
		return nil
	})
	return out, err
}
