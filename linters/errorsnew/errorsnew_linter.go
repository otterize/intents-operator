package errorsnew

import (
	"go/ast"
	"go/token"
	"golang.org/x/tools/go/analysis"
)

var Analyzer = &analysis.Analyzer{
	Name: "errorsnew",
	Doc:  "checks that there are no calls to errors.New outside of a function (we should use sentinel errors for that)",
	Run:  run,
}

func run(pass *analysis.Pass) (interface{}, error) {
	for _, file := range pass.Files {
		ast.Inspect(file, func(node ast.Node) bool {
			if decl, ok := node.(*ast.GenDecl); ok && decl.Tok == token.VAR {
				handleGenDecl(pass, decl)
			}
			return true
		})
	}
	return nil, nil
}

func handleGenDecl(pass *analysis.Pass, decl *ast.GenDecl) {
	for _, spec := range decl.Specs {
		if valueSpec, ok := spec.(*ast.ValueSpec); ok {
			handleValueSpec(pass, valueSpec)
		}
	}
}

func handleValueSpec(pass *analysis.Pass, valueSpec *ast.ValueSpec) {
	for _, value := range valueSpec.Values {
		if call, ok := value.(*ast.CallExpr); ok {
			handleCallExpr(pass, call)
		}
	}
}

func handleCallExpr(pass *analysis.Pass, call *ast.CallExpr) {
	if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
		handleSelectorExpr(pass, sel, call)
	}
}

func handleSelectorExpr(pass *analysis.Pass, sel *ast.SelectorExpr, call *ast.CallExpr) {
	if pkg, ok := sel.X.(*ast.Ident); ok {
		if pkg.Name == "errors" && (sel.Sel.Name == "New" || sel.Sel.Name == "Errorf") {
			pass.Reportf(call.Pos(), "Found \"errors.New\" in the global scope. Replace it with \"errors.NewSentinelError\"")
		}
	}
}
