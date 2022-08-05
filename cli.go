package main

import (
	"go/ast"
	"log"
	"reflect"
	"runtime"
	"strings"
)

type Func struct {
	Name string // optional
	Type *ast.FuncType
	Body *ast.BlockStmt
}

func handleFunc(f Func) {
	log.Printf("handling function %s\n", f.Name)

	varOC := checkParamsForCLI(f.Type)
	if varOC != "" {
		log.Printf("function has CLI param named: %s\n", varOC)
	} else {
		for _, stmt := range f.Body.List {
			varOC = checkStmtForNewCLI(stmt)
			if varOC != "" {
				log.Printf("cliVar: %s\n", varOC)
			}
		}
	}

	if varOC == "" {
		log.Printf("function does not use CLI")
		return
	}

	// scan function body for usage of *CLI
	// now we can walk whole function and all nodes
}

// checkAssignStmtForNewCLI inspects given AssignStmt for *.NewCLI*() or *.NewHypershiftManagementCLI() function
// if found, then name of the variable storing *CLI is returned
func checkStmtForNewCLI(s ast.Stmt) string {
	/*
	   *ast.AssignStmt {
	   .  Lhs: []ast.Expr (len = 1) {
	   .  .  0: *ast.Ident {
	   .  .  .  Name: "oc"
	   .  .  .  Obj: *ast.Object {
	   .  .  .  .  Kind: var
	   .  .  .  .  Name: "oc"
	   .  .  .  }
	   .  .  }
	   .  }
	   .  Tok: :=
	   .  Rhs: []ast.Expr (len = 1) {
	   .  .  0: *ast.CallExpr {
	   .  .  .  Fun: *ast.SelectorExpr {
	   .  .  .  .  X: *ast.Ident { Name: "exutil" }
	   .  .  .  .  Sel: *ast.Ident { Name: "NewCLIWithPodSecurityLevel" }
	   .  .  .  }
	   .  .  .  Args: []ast.Expr (len = 2) {
	   .  .  .  .  0: *ast.BasicLit { Value: "\"test-cmd\"" }
	   .  .  .  .  1: *ast.SelectorExpr {
	   .  .  .  .  .  X: *ast.Ident { Name: "admissionapi" }
	   .  .  .  .  .  Sel: *ast.Ident { Name: "LevelBaseline" }
	   .  .  .  .  }
	   .  .  .  }
	   .  .  }
	   .  }
	   }

	*/

	a, ok := s.(*ast.AssignStmt)
	if !ok {
		return ""
	}

	if len(a.Lhs) != 1 || len(a.Rhs) != 1 {
		return ""
	}
	call, ok := a.Rhs[0].(*ast.CallExpr)
	if !ok {
		return ""
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return ""
	}

	if strings.HasPrefix(sel.Sel.Name, "NewCLI") ||
		sel.Sel.Name == runtime.FuncForPC(reflect.ValueOf(NewHypershiftManagementCLI).Pointer()).Name() {
		if variable, ok := a.Lhs[0].(*ast.Ident); ok {
			return variable.Name
		}
	}
	return ""
}

func checkParamsForCLI(t *ast.FuncType) string {
	if len(t.Params.List) == 0 {
		return ""
	}

	/*
		List: []*ast.Field (len = 2) {
		.  0: *ast.Field {
		.  .  Names: []*ast.Ident (len = 1) {
		.  .  .  0: *ast.Ident {
		.  .  .  .  NamePos: 3979
		.  .  .  .  Name: "oc"
		.  .  .  .  Obj: *ast.Object {
		.  .  .  .  .  Kind: var
		.  .  .  .  .  Name: "oc"
		.  .  .  .  .  Decl: *(obj @ 878)
		.  .  .  .  }
		.  .  .  }
		.  .  }
		.  .  Type: *ast.StarExpr {
		.  .  .  Star: 3982
		.  .  .  X: *ast.SelectorExpr {
		.  .  .  .  X: *ast.Ident {
		.  .  .  .  .  NamePos: 3983
		.  .  .  .  .  Name: "exutil"
		.  .  .  .  }
		.  .  .  .  Sel: *ast.Ident {
		.  .  .  .  .  NamePos: 3990
		.  .  .  .  .  Name: "CLI"
		.  .  .  .  }
		.  .  .  }
		.  .  }
		.  }
		.  1: *ast.Field { ... }
	*/

	for _, field := range t.Params.List {
		starExpr, ok := field.Type.(*ast.StarExpr)
		if !ok {
			continue
		}
		selExpr, ok := starExpr.X.(*ast.SelectorExpr)
		if !ok {
			continue
		}
		if selExpr.Sel.Name == "CLI" {
			// TODO: Check package-alias (Field.Type.X.X.Name - e.g. exutil)
			if len(field.Names) != 1 {
				panic("len(field.Names) != 1")
			}
			return field.Names[0].Name
		}
	}
	return ""
}
