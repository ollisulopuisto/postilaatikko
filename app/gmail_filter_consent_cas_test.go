package app

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// The stale-snapshot behavior is exercised by credentials' cross-Store CAS
// tests. This wiring guard prevents standalone consent from bypassing it.
func TestStandaloneGmailConsentUsesValidatedSnapshotCAS(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "gmail_filter_oauth.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, declaration := range file.Decls {
		fn, ok := declaration.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "persistGmailFilterConsent" {
			continue
		}
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if selector.Sel.Name == "SetOAuthTokens" {
				t.Error("consent bypasses snapshot CAS")
			}
			if selector.Sel.Name == "CompareAndSwapOAuthTokens" {
				found = true
				if len(call.Args) != 3 {
					t.Error("CAS must receive account, validated snapshot and replacement")
					return true
				}
				snapshot, ok := call.Args[1].(*ast.Ident)
				if !ok || snapshot.Name != "current" {
					t.Error("CAS does not use validated current snapshot")
				}
			}
			return true
		})
	}
	if !found {
		t.Error("standalone consent must CAS its validated snapshot")
	}
}
