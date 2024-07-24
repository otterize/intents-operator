package errorsnew

import (
	"go/ast"
	"go/token"
	"golang.org/x/tools/go/analysis"
)

var Analyzer = &analysis.Analyzer{
	Name: "errorsnewlinter",
	Doc:  "checks that there are no calls to errors.New outside of a function (we should use sentinel errors for that)",
	Run:  run,
}

func run(pass *analysis.Pass) (interface{}, error) {
	for _, file := range pass.Files {
		ast.Inspect(file, func(node ast.Node) bool {
			// Check for calls to errors.New outside functions
			if decl, ok := node.(*ast.GenDecl); ok && decl.Tok == token.VAR {
				for _, spec := range decl.Specs {
					if valueSpec, ok := spec.(*ast.ValueSpec); ok {
						for _, value := range valueSpec.Values {
							if call, ok := value.(*ast.CallExpr); ok {
								if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
									if pkg, ok := sel.X.(*ast.Ident); ok {
										if pkg.Name == "errors" && (sel.Sel.Name == "New" || sel.Sel.Name == "Errorf") {
											pass.Reportf(call.Pos(), "found errors.New outside a function - replace with errors.NewSentinelError: %s", pass.Fset.Position(call.Pos()))
										}
									}
								}
							}
						}
					}
				}
			}
			return true
		})
	}
	return nil, nil
}
