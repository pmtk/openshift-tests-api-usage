package main

import (
	"fmt"
	"go/ast"
	"golang.org/x/tools/go/ast/inspector"
	"golang.org/x/tools/go/packages"
)

func assert(b bool) {
	if !b {
		panic("assertion failed")
	}
}

func checkIfResourceInterfaceCreation(ce *ast.CallExpr) bool {
	selExpr, ok := ce.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	// TODO: Make check more comprehensive
	// in OBJ.Resource() check if OBJ is created using dynamic.NewForConfig*
	// selExpr.X.(*ast.Ident).Obj.Decl.Rhs[0].Fun
	// 	.X == "dynamic" -> verify with imports if k8s.io/client-go/dynamic
	//  .Sel == "NewForConfigOrDie" | "NewForConfigAndClient" | "NewForConfig"

	if selExpr.Sel.Name != "Resource" {
		return false
	}
	return true
}

func analyzeResourceParam(e ast.Expr) {
	// TODO: return group value

	switch v := e.(type) {
	case *ast.CompositeLit:
		// 1) directly in call: Resource(GroupVersionResource{...})
		typ, ok := v.Type.(*ast.SelectorExpr)
		assert(ok && typ.Sel.Name == "GroupVersionResource")

		switch elt0 := v.Elts[0].(type) {
		case *ast.BasicLit:
			fmt.Printf("\tapigroup:%s\n", elt0.Value)
		case *ast.KeyValueExpr:
			switch v := elt0.Value.(type) {
			case *ast.BasicLit:
				fmt.Printf("\tapigroup:%s\n", v.Value)
			case *ast.Ident:
				assignStmt, ok := v.Obj.Decl.(*ast.AssignStmt)
				if ok {
					group := goDownAssignStmtsForGVR(assignStmt)
					fmt.Printf("\tapigroup:%s\n", group)
				}
			}
		}
	case *ast.Ident:
		// TODO: 2) passed as a var: Resource(gvr)
		assignStmt, ok := v.Obj.Decl.(*ast.AssignStmt)
		if ok {
			group := goDownAssignStmtsForGVR(assignStmt)
			fmt.Printf("\tapigroup:%s\n", group)
		}

	default:
		panic("TODO")
	}
}

func goDownAssignStmtsForGVR(a *ast.AssignStmt) string {
	// travel down to declaration
	switch rhs := a.Rhs[0].(type) {
	case *ast.Ident:
		a2, ok := rhs.Obj.Decl.(*ast.AssignStmt)
		assert(ok)
		return goDownAssignStmtsForGVR(a2)
	case *ast.BasicLit:
		return rhs.Value
	case *ast.CompositeLit:
		typ, ok := rhs.Type.(*ast.SelectorExpr)
		assert(ok && typ.Sel.Name == "GroupVersionResource")

		switch elt0 := rhs.Elts[0].(type) {
		case *ast.BasicLit:
			return elt0.Value
		case *ast.KeyValueExpr:
			switch v := elt0.Value.(type) {
			case *ast.BasicLit:
				return v.Value
			case *ast.Ident:
				assignStmt, ok := v.Obj.Decl.(*ast.AssignStmt)
				if ok {
					return goDownAssignStmtsForGVR(assignStmt)
				}
			}
		}
	case *ast.UnaryExpr:
		// for _, gvr := range gvrs
		// switch to looking for gvrs
		switch x := rhs.X.(type) {
		case *ast.Ident:
			switch decl := x.Obj.Decl.(type) {
			case *ast.ValueSpec:
				for _, elt := range decl.Values[0].(*ast.CompositeLit).Elts {
					switch elt := elt.(type) {
					case *ast.CompositeLit:
						fmt.Printf("++ apigroup: %s\n", elt.Elts[0].(*ast.KeyValueExpr).Value.(*ast.BasicLit).Value)
					case *ast.CallExpr:
						analyzeCallExprReturningGVR(elt)
					}
				}
			case *ast.AssignStmt:
				if ce, ok := decl.Rhs[0].(*ast.CallExpr); ok {
					analyzeCallExprReturningGVR(ce)
				} else {
					panic("TODO")
				}
			default:
				panic("TODO")
			}
		default:
			panic("TODO")
		}
	default:
		panic("TODO")
	}
	return ""
}

func analyzeCallExprReturningGVR(ce *ast.CallExpr) {
	// TODO: check if fun returns GVR, map[GVR], []GVR, or something else

	if len(ce.Args) == 3 {
		arg0 := ce.Args[0].(*ast.BasicLit)
		fmt.Printf("++ %s\n", arg0.Value)
		return
	}

	se := ce.Fun.(*ast.SelectorExpr)
	pkg := se.X.(*ast.Ident).Name
	funName := se.Sel.Name
	println(pkg, "  ", funName)
}

type investigator struct {
	pkg *packages.Package
}

func (i *investigator) getGroupsFromCall(call *ast.CallExpr) []string {
	assert(len(call.Args) == 1)
	analyzeResourceParam(call.Args[0])

	return nil
}

func workOnAstPkg(pkg *packages.Package) {
	inv := investigator{pkg}

	i := inspector.New(pkg.Syntax)
	i.WithStack(
		[]ast.Node{&ast.CallExpr{}},
		func(n ast.Node, push bool, stack []ast.Node) (proceed bool) {
			proceed = true
			if !push {
				return
			}

			callExpr := n.(*ast.CallExpr)
			if checkIfResourceInterfaceCreation(callExpr) {
				groups := inv.getGroupsFromCall(callExpr)
				fmt.Printf("ResourceInterface creation: %v\n\tAPI Groups:%v\n", pkg.Fset.Position(n.Pos()), groups)
			}
			return
		},
	)
}
