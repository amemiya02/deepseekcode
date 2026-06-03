package codegraph

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
)

// ParseDir parses all .go files under dir, adding nodes and edges to store.
// pkgPath is the module-qualified import path for the package (used to build NodeIDs).
func ParseDir(dir, pkgPath string, store *Store) error {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, func(fi fs.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		return fmt.Errorf("parseDir %s: %w", dir, err)
	}
	for _, pkg := range pkgs {
		parsePackage(fset, pkg, pkgPath, store)
	}
	return nil
}

func parsePackage(fset *token.FileSet, pkg *ast.Package, pkgPath string, store *Store) {
	// First pass: register all top-level declarations as nodes.
	for _, file := range pkg.Files {
		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				pos := fset.Position(d.Pos())
				id := NodeID(pkgPath + "." + d.Name.Name)
				store.AddNode(&Node{
					ID:   id,
					Kind: KindFunc,
					Name: d.Name.Name,
					File: filepath.ToSlash(pos.Filename),
					Line: pos.Line,
				})
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					ts, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}
					pos := fset.Position(ts.Pos())
					kind := KindType
					if _, isIface := ts.Type.(*ast.InterfaceType); isIface {
						kind = KindInterface
					}
					id := NodeID(pkgPath + "." + ts.Name.Name)
					store.AddNode(&Node{
						ID:   id,
						Kind: kind,
						Name: ts.Name.Name,
						File: filepath.ToSlash(pos.Filename),
						Line: pos.Line,
					})
				}
			}
		}
	}

	// Second pass: walk function bodies for call expressions to build CALLS edges.
	for _, file := range pkg.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			callerID := NodeID(pkgPath + "." + fn.Name.Name)
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				calleeName := callExprName(call.Fun)
				if calleeName == "" {
					return true
				}
				calleeID := NodeID(pkgPath + "." + calleeName)
				if store.Node(calleeID) != nil {
					store.AddEdge(Edge{From: callerID, To: calleeID, Kind: EdgeCalls})
				}
				return true
			})
		}
	}
}

// callExprName extracts the simple identifier name from a call expression's Fun field.
// Returns "" for method calls, selector expressions on external packages, etc.
func callExprName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	default:
		return ""
	}
}
