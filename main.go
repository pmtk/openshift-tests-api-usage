/*
assumptions:
- many `var _ = g.Describe` possible in single file
- nested `Describe`s with 1 It are possible, but only top Describe is taken into consideration
- free functions can create oc.CLI (rather just being given via args)

structure:
- pkg
  - file
    - free functions
	  `var _ = g.Describe`s (can call free functions)

how to keep track of such?
  clusterAdminClientConfig := oc.AdminConfig()
  clusterAdminOAuthClient := oauthv1client.NewForConfigOrDie(clusterAdminClientConfig)

maybe AST needs to be transformed...?
g.Before() also can initiate CLI (but not create - needs to be in outer scope)
*/

package main

import (
	"bytes"
	"go/ast"
	"go/printer"
	"go/token"
	"log"
	"path"
	"reflect"
	"runtime"
	"strings"

	p "golang.org/x/tools/go/packages"
)

// TODO: test Dir: origin + patterns "."

func main() {
	cfg := &p.Config{
		Mode: p.NeedFiles | p.NeedSyntax | p.NeedCompiledGoFiles,
		Dir:  "/home/pm/dev/origin/test/extended/cmd",
	}
	pkgs, err := p.Load(cfg, "")
	if err != nil {
		log.Fatalf("error: %#v\n", err)
	}
	if len(pkgs) != 1 {
		log.Fatalf("unexpected len(pkgs): %d\n", len(pkgs))
	}

	imports := make(map[string]string, 0)
	for _, file := range pkgs[0].Syntax {
		// get map of pkg-alias to pkg-url
		for _, imp := range file.Imports {
			name, path := getImportNameAndPath(imp)
			imports[name] = path
		}

		ast.Inspect(file, func(n ast.Node) bool {
			switch v := n.(type) {
			case *ast.FuncDecl:
				handleFunc(Func{Name: v.Name.Name, Type: v.Type, Body: v.Body})
			case *ast.FuncLit:
				handleFunc(Func{Type: v.Type, Body: v.Body})
			}
			return true
		})

		// ginkgoAlias := getImportName(imports, "ginkgo")

		// for _, decl := range file.Decls {

		// 	switch d := decl.(type) {
		// 	case *ast.FuncDecl:
		// 		handleFunc(Func{d.Type, d.Body})
		// 	case *ast.GenDecl:
		// 		log.Printf("GenDecl: %#v\n", d)
		// 		handleGenDecl(d, ginkgoAlias)
		// 	}

		// }
	}
}

func getImportNameAndPath(i *ast.ImportSpec) (name string, p string) {
	p = i.Path.Value
	if i.Name != nil {
		name = i.Name.Name
	} else {
		_, name = path.Split(strings.Replace(i.Path.Value, "\"", "", -1))
	}
	return
}

func getImportName(imports map[string]string, x string) string {
	for k, v := range imports {
		if strings.Contains(v, "ginkgo") {
			return k
		}
	}
	return ""
}

// tryGetFuncBodyFromGenDecl
func handleGenDecl(decl *ast.GenDecl, ginkgoAlias string) {
	if decl.Tok != token.VAR {
		return
	}

	if len(decl.Specs) != 1 {
		log.Fatalf("unexpected len(decl.Specs): %d", len(decl.Specs))
	}

	values, ok := decl.Specs[0].(*ast.ValueSpec)
	if !ok {
		log.Fatalf("decl.Specs[0].(*ast.ValueSpec) failed")
	}
	if len(values.Values) != 1 {
		log.Fatalf("unexpected len(values): %d", len(values.Values))
	}

	call, ok := values.Values[0].(*ast.CallExpr)
	if !ok {
		return
		// log.Fatalf("values.Values[0].(*ast.CallExpr) failed")
	}

	name, body := ginkgoDescribeCallExprToNameAndBody(call)
	_ = name
	_ = body
	if body == nil {
		return
	}

	// look for `oc` creation

	findNewCLI := func(body *ast.BlockStmt) string {
		for _, stmt := range body.List {
			assign, ok := stmt.(*ast.AssignStmt)
			if !ok {
				continue
			}

			if len(assign.Rhs) != 1 {
				log.Printf("len(assign.Rhs) != 1")
				continue
			}
			call, ok := assign.Rhs[0].(*ast.CallExpr)
			if !ok {
				log.Printf("assign.Rhs[0].(*ast.CallExpr) not ok")
			}
			if strings.HasPrefix(call.Fun.(*ast.SelectorExpr).Sel.Name, "NewCLI") ||
				call.Fun.(*ast.SelectorExpr).Sel.Name == runtime.FuncForPC(reflect.ValueOf(NewHypershiftManagementCLI).Pointer()).Name() {
				log.Printf("found *CLI creation, var is: %s", assign.Lhs[0].(*ast.Ident).Name)

				return assign.Lhs[0].(*ast.Ident).Name
			}
		}
		return ""
	}

	cliVar := findNewCLI(body)
	if cliVar == "" {
		log.Printf("no CLI found")
		return
	}

	ast.Inspect(body, func(n ast.Node) bool {
		if _, ok := n.(*ast.CallExpr); ok {
			fset := token.NewFileSet()
			b := &bytes.Buffer{}
			printer.Fprint(b, fset, n)

			if strings.HasPrefix(b.String(), cliVar+".") {
				log.Println(b.String())
				return false
			}
		}

		return true
	})
}

func ginkgoDescribeCallExprToNameAndBody(callExpr *ast.CallExpr) (string, *ast.BlockStmt) {
	selector := callExpr.Fun.(*ast.SelectorExpr)

	// Check that function is calling a Describe from Ginkgo pkg
	if selector.X.(*ast.Ident).Name != "g" || selector.Sel.Name != "Describe" {
		return "", nil
	}

	return strings.Replace(callExpr.Args[0].(*ast.BasicLit).Value, "\"", "", -1),
		callExpr.Args[1].(*ast.FuncLit).Body
}

// dummy function for reflection
// TODO: use func from origin
func NewHypershiftManagementCLI() {}
