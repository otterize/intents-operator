package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// Check if a call expression is errors.New
func isErrorsNewCall(expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	return pkg.Name == "errors" && sel.Sel.Name == "New"
}

// Visitor struct for AST traversal
type visitor struct {
	fset        *token.FileSet
	errorsFound bool
	filename    string
}

func (v *visitor) Visit(node ast.Node) ast.Visitor {
	if node == nil {
		return nil
	}

	// Check for calls to errors.New outside functions
	if decl, ok := node.(*ast.GenDecl); ok && decl.Tok == token.VAR {
		for _, spec := range decl.Specs {
			if valueSpec, ok := spec.(*ast.ValueSpec); ok {
				for _, value := range valueSpec.Values {
					if isErrorsNewCall(value) {
						pos := v.fset.Position(value.Pos())
						fmt.Printf("Found errors.New outside a function at %s:%d\n", pos.Filename, pos.Line)
						v.errorsFound = true
					}
				}
			}
		}
	}

	return v
}

func lintFile(filename string) bool {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filename, nil, parser.AllErrors)
	if err != nil {
		log.Printf("Failed to parse file %s: %v\n", filename, err)
		return false
	}

	v := &visitor{
		fset:     fset,
		filename: filename,
	}
	ast.Walk(v, node)
	return v.errorsFound
}

func lintDir(dirname string) bool {
	var foundErrors bool
	err := filepath.Walk(dirname, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".go") {
			if lintFile(path) {
				foundErrors = true
			}
		}
		return nil
	})
	if err != nil {
		log.Printf("Failed to walk directory %s: %v\n", dirname, err)
	}
	return foundErrors
}

func main() {
	flag.Parse()
	args := flag.Args()
	if len(args) == 0 {
		fmt.Println("Usage: go run errors_linter.go [file or directory]")
		os.Exit(1)
	}

	var foundErrors bool
	for _, arg := range args {
		info, err := os.Stat(arg)
		if err != nil {
			log.Printf("Failed to stat %s: %v\n", arg, err)
			continue
		}
		if info.IsDir() {
			if lintDir(arg) {
				foundErrors = true
			}
		} else {
			if lintFile(arg) {
				foundErrors = true
			}
		}
	}

	if foundErrors {
		os.Exit(1)
	}
}
