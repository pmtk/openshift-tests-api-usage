package main

import (
	"fmt"
	"go/ast"
	"go/types"
	"golang.org/x/tools/go/ast/inspector"
	"golang.org/x/tools/go/packages"
	"strings"
)

func assert(b bool) {
	if !b {
		panic("assertion failed")
	}
}

func sanitize(s string) string {
	return strings.ReplaceAll(s, "\"", "")
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

func (i *investigator) astPrint(x any) {
	ast.Print(i.pkg.Fset, x)
}

func (i *investigator) assignStmt(a *ast.AssignStmt) []string {
	// travel down to declaration
	switch rhs := a.Rhs[0].(type) {
	case *ast.Ident:
		// g2 := g1
		a2, ok := rhs.Obj.Decl.(*ast.AssignStmt)
		assert(ok)
		return i.assignStmt(a2)
	case *ast.BasicLit:
		// gr := "g"
		return []string{sanitize(rhs.Value)}
	case *ast.CompositeLit:
		switch typ := rhs.Type.(type) {
		case *ast.MapType:
			isKeyGVR := isExprGVR(typ.Key)
			groups := []string{}
			for _, elt := range rhs.Elts {
				elt := elt.(*ast.KeyValueExpr)
				if isKeyGVR {
					switch key := elt.Key.(type) {
					case *ast.Ident:
						groups = append(groups, i.assignStmt(key.Obj.Decl.(*ast.AssignStmt))...)
					case *ast.CallExpr:
						groups = append(groups, i.analyzeCallExprReturningGVR(key)...)
					default:
						panic("TODO")
					}
				} else {
					switch key := elt.Value.(type) {
					case *ast.Ident:
						groups = append(groups, i.assignStmt(key.Obj.Decl.(*ast.AssignStmt))...)
					case *ast.CallExpr:
						groups = append(groups, i.analyzeCallExprReturningGVR(key)...)
					default:
						panic("TODO")
					}
				}
			}
			return groups
		case *ast.SelectorExpr:
			// gvr := schema.GroupVersionResource{Group: "g", Version: "v", Resource: "r"}
			assert(typ.Sel.Name == "GroupVersionResource")
			switch elt0 := rhs.Elts[0].(type) { // assuming 0 - group, 1 - version, 2 - kind
			case *ast.BasicLit:
				// GVR{ "g" }
				//      ^^^
				return []string{sanitize(elt0.Value)}
			case *ast.KeyValueExpr:
				// GVR{ Group: "g" }
				//      ^^^^^^^^^^
				switch v := elt0.Value.(type) {
				case *ast.BasicLit:
					// GVR{ Group: "g" }
					//             ^^^ - plain string
					return []string{sanitize(v.Value)}
				case *ast.Ident:
					// ???
					assignStmt, ok := v.Obj.Decl.(*ast.AssignStmt)
					if ok {
						return i.assignStmt(assignStmt)
					}
				}
			}

		}

	case *ast.UnaryExpr:
		// for _, gvr := range gvrs -- operator: RANGE, operand: gvrs
		// switch to looking for gvrs
		operand, ok := rhs.X.(*ast.Ident)
		assert(ok)
		switch decl := operand.Obj.Decl.(type) {
		case *ast.ValueSpec:
			// grvs := []GVR{ GVR, GVR }
			//         ^^^^^^          ^
			groups := []string{}
			assert(len(decl.Values) == 1)
			switch value := decl.Values[0].(type) {
			case *ast.CompositeLit:
				for _, elt := range value.Elts {
					switch elt := elt.(type) {
					case *ast.CompositeLit:
						// grvs := []GVR{ GVR{}, GVR{} }
						//                ^^^^^ - created as literal struct
						groups = append(groups, sanitize(elt.Elts[0].(*ast.KeyValueExpr).Value.(*ast.BasicLit).Value))
					case *ast.CallExpr:
						// grvs := []GVR{ funcReturningGVR(), funcReturningGVR() }
						//                ^^^^^^^^^^^^^^^^^^ - GVR from function
						groups = append(groups, i.analyzeCallExprReturningGVR(elt)...)
					}
				}
			case *ast.CallExpr:
				return i.analyzeCallExprReturningGVR(value)
			}
			return groups
		case *ast.AssignStmt:
			// gvrs := FuncReturningOperand()
			if ce, ok := decl.Rhs[0].(*ast.CallExpr); ok {
				return i.analyzeCallExprReturningGVR(ce)
			} else {
				panic("TODO")
			}
		default:
			panic("TODO")
		}
	case *ast.CallExpr:
		return i.analyzeCallExprReturningGVR(rhs)
	default:
		panic("TODO")
	}
	return nil
}

// isFunctionGVRHelper checks for "GVR Helper" which is defined as a function that takes 3 string params
// and return GVR object: func F(g, v, r) GVR
func isFunctionGVRHelper(signature *types.Signature) bool {
	params := signature.Params()
	funcTakes3Strings := params.Len() == 3 &&
		params.At(0).Type().String() == "string" &&
		params.At(1).Type().String() == "string" &&
		params.At(2).Type().String() == "string"

	results := signature.Results()
	funcReturnsGVR := results.Len() == 1 &&
		results.At(0).Type().String() == "k8s.io/apimachinery/pkg/runtime/schema.GroupVersionResource"

	return funcTakes3Strings && funcReturnsGVR
}

func isTypeGVR(t types.Type) bool {
	named, ok := t.(*types.Named)
	if !ok {
		panic("TODO")
		return false
	}
	return named.Obj().Type().String() == "k8s.io/apimachinery/pkg/runtime/schema.GroupVersionResource"
}

func isExprGVR(e ast.Expr) bool {
	switch e := e.(type) {
	case *ast.SelectorExpr:
		return sanitize(e.Sel.Name) == "GroupVersionResource"
	case *ast.Ident:
		if sanitize(e.Name) == "GroupVersionResource" {
			return true
		}
		if as, ok := e.Obj.Decl.(*ast.AssignStmt); ok {
			if len(as.Rhs) == 1 {
				if cl, ok := as.Rhs[0].(*ast.CompositeLit); ok {
					return isExprGVR(cl.Type)
				} else {
					panic("TODO")
				}
			} else {
				panic("TODO")
			}
		}
		return false
	case *ast.CompositeLit:
		return isExprGVR(e.Type)
	}

	panic("TODO")
	return false
}

func (i *investigator) getFunctionFromImportedPackage(f *ast.SelectorExpr) (*packages.Package, *ast.FuncDecl) {
	pkg, ok := f.X.(*ast.Ident)
	assert(ok)

	findImportSpec := func(pred func(is *ast.ImportSpec) bool) *ast.ImportSpec {
		for _, astFile := range i.pkg.Syntax {
			for _, imp := range astFile.Imports {
				if pred(imp) {
					return imp
				}
			}
		}
		return nil
	}

	// first try to match package by "import name" (like "g" for ginkgo)
	importSpec := findImportSpec(func(is *ast.ImportSpec) bool {
		return is.Name != nil && is.Name == pkg
	})
	if importSpec == nil {
		// try match pkg by last part of url
		importSpec = findImportSpec(func(is *ast.ImportSpec) bool {
			lastPart := sanitize(is.Path.Value[strings.LastIndex(is.Path.Value, "/")+1:])
			return lastPart == pkg.Name
		})
	}
	assert(importSpec != nil)

	otherPkg, ok := i.pkg.Imports[sanitize(importSpec.Path.Value)]
	assert(ok)

	for _, file := range otherPkg.Syntax {
		for _, decl := range file.Decls {
			if fd, ok := decl.(*ast.FuncDecl); ok {
				if fd.Name.Name == f.Sel.Name {
					return otherPkg, fd
				}
			}
		}
	}
	return nil, nil
}

func (i *investigator) analyzeCompositeLit(m *ast.CompositeLit) []string {
	groups := []string{}

	for _, elt := range m.Elts {
		switch elt := elt.(type) {
		case *ast.KeyValueExpr:
			isKeyGVR := isExprGVR(elt.Key)
			if isKeyGVR {
				switch key := elt.Key.(type) {
				case *ast.Ident:
					groups = append(groups, i.assignStmt(key.Obj.Decl.(*ast.AssignStmt))...)
				case *ast.CallExpr:
					groups = append(groups, i.analyzeCallExprReturningGVR(key)...)
				case *ast.CompositeLit:
					if id, ok := key.Elts[0].(*ast.KeyValueExpr).Key.(*ast.Ident); ok {
						if sanitize(id.Name) == "Group" {
							groups = append(groups, sanitize(key.Elts[0].(*ast.KeyValueExpr).Value.(*ast.BasicLit).Value))
						} else {
							panic("TODO")
						}
					} else {
						groups = append(groups, i.analyzeCompositeLit(key)...)
					}
				default:
					panic("TODO")
				}
			} else {
				switch val := elt.Value.(type) {
				case *ast.Ident:
					groups = append(groups, i.assignStmt(val.Obj.Decl.(*ast.AssignStmt))...)
				case *ast.CallExpr:
					groups = append(groups, i.analyzeCallExprReturningGVR(val)...)
				default:
					panic("TODO")
				}
			}
		}
	}
	return groups
}

func (i *investigator) analyzeFunction(fun *ast.FuncDecl) []string {
	// last Stmt should be ReturnStmt
	// TODO: named return var - low prio

	lastStmt := fun.Body.List[len(fun.Body.List)-1]
	returnStmt, ok := lastStmt.(*ast.ReturnStmt)
	assert(ok)
	assert(returnStmt != nil)

	groups := []string{}

	// find which returned value contains GVR (hopefully just one...)
	for idx, res := range fun.Type.Results.List {
		switch typ := res.Type.(type) {
		case *ast.MapType:
			switch returnedMap := returnStmt.Results[idx].(type) {
			case *ast.CallExpr: // func returns func returning map
				return i.analyzeFunction(returnedMap.Fun.(*ast.Ident).Obj.Decl.(*ast.FuncDecl))
			case *ast.Ident:
				return i.assignStmt(returnedMap.Obj.Decl.(*ast.AssignStmt))
			case *ast.CompositeLit:
				groups = append(groups, i.analyzeCompositeLit(returnedMap)...)
			default:
				panic("todo")
			}
		case *ast.ArrayType:
			isGVRArray := isExprGVR(typ.Elt)
			if isGVRArray {
				returnedGVRArr := returnStmt.Results[idx]
				switch returnedGVRArr := returnedGVRArr.(type) {
				case *ast.CompositeLit:
					for _, elt := range returnedGVRArr.Elts {
						switch elt := elt.(type) {
						case *ast.Ident:
							assignStmt, ok := elt.Obj.Decl.(*ast.AssignStmt)
							if ok {
								groups = append(groups, i.assignStmt(assignStmt)...)
							}
						case *ast.CompositeLit:
							assert(elt.Elts[0].(*ast.KeyValueExpr).Key.(*ast.Ident).Name == "Group")
							groups = append(groups, sanitize(elt.Elts[0].(*ast.KeyValueExpr).Value.(*ast.BasicLit).Value))
						default:
							panic("todo")
						}
					}
				}
			}
		case *ast.CompositeLit:
			panic("TODO")
		case *ast.Ident:
			continue
		default:
			panic("TODO")
		}
	}

	return groups
}

func (i *investigator) analyzeCallExprReturningGVR(ce *ast.CallExpr) []string {
	switch fun := ce.Fun.(type) {
	case *ast.SelectorExpr:
		// func is from another pkg
		if typ, ok := i.pkg.TypesInfo.Types[fun]; ok {
			if signature, ok := typ.Type.(*types.Signature); ok {
				if isFunctionGVRHelper(signature) {
					// just take first arg which is assumed to be a group
					switch arg := ce.Args[0].(type) {
					case *ast.BasicLit:
						// F( "g", ... )
						return []string{sanitize(arg.Value)}
					case *ast.Ident:
						// F( var, ... )
						if as, ok := arg.Obj.Decl.(*ast.AssignStmt); ok {
							return i.assignStmt(as)
						} else {
							panic(nil)
						}
					}
				} else {
					// not a "helper function F(g,v,r) GVR" but a function that returns GVR in some form like []GVR, map[GVR]* or map[*]GVR

					funPkg, fun := i.getFunctionFromImportedPackage(fun)
					assert(fun != nil)
					if funPkg != i.pkg {
						// if function resides in another package, we need metadata from that different pkg
						i2 := investigator{pkg: funPkg}
						return i2.analyzeFunction(fun)
					}
					return i.analyzeFunction(fun)
				}
			}
		}

	case *ast.Ident:
		// function resides in current pkg
		if f, ok := i.pkg.TypesInfo.Uses[fun]; ok {
			if signature, ok := f.Type().(*types.Signature); ok {
				if isFunctionGVRHelper(signature) {
					// just take first arg which is assumed to be api group

					switch arg := ce.Args[0].(type) {
					case *ast.BasicLit:
						// F( "g", ... )
						return []string{sanitize(arg.Value)}
					case *ast.Ident:
						// F( var, ... )
						if as, ok := arg.Obj.Decl.(*ast.AssignStmt); ok {
							return i.assignStmt(as)
						} else {
							panic(nil)
						}
					}

				} else {
					panic("TODO")
				}
			}
		} else {
			panic("TODO")
		}

		panic("TODO")

	}

	return nil
}

type investigator struct {
	pkg *packages.Package
}

// analyzeInterfaceResourceCall expects an *ast.CallExpr that is confirmed to be k8s.io/client-go/dynamic.Interface.Resource() call
// it returns all API Groups used in that function call
func (i *investigator) analyzeInterfaceResourceCall(call *ast.CallExpr) []string {
	assert(len(call.Args) == 1)
	switch v := call.Args[0].(type) {
	case *ast.CompositeLit:
		// 1) directly in call: Resource(GroupVersionResource{...})
		typ, ok := v.Type.(*ast.SelectorExpr)
		assert(ok && typ.Sel.Name == "GroupVersionResource")

		switch elt0 := v.Elts[0].(type) {
		case *ast.BasicLit:
			// Resource(GroupVersionResource{"g", "v", "r"}) -- no keys (Group, Version, Resource)
			return []string{sanitize(elt0.Value)}
		case *ast.KeyValueExpr:
			switch v := elt0.Value.(type) {
			case *ast.BasicLit:
				// Resource(GroupVersionResource{Group: "g", Version: "v", Resource: "r"}) -- keys are used
				return []string{sanitize(v.Value)}
			case *ast.Ident:
				// Resource(GroupVersionResource{Group: gr, Version: v, Resource: r}) -- string var are used
				assignStmt, ok := v.Obj.Decl.(*ast.AssignStmt)
				if ok {
					return i.assignStmt(assignStmt)
				}
			}
		}
	case *ast.Ident:
		// Resource(gvr) -- gvr has type GroupVersionResource
		assignStmt, ok := v.Obj.Decl.(*ast.AssignStmt)
		if ok {
			return i.assignStmt(assignStmt)
		}

	default:
		panic("TODO")
	}

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
				fmt.Printf("ResourceInterface creation: %v\n", pkg.Fset.Position(n.Pos()))
				fmt.Printf("\tAPI Groups:%v\n", inv.analyzeInterfaceResourceCall(callExpr))
			}
			return
		},
	)
}
