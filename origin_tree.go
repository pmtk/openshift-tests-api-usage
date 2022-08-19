package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/printer"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/ast/inspector"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/types/typeutil"
	klog "k8s.io/klog/v2"
)

// TODO: Handle InBareMetalIPv4ClusterContext from test/extended/networking/services.go
// TODO: Make sure table driven tests are handled correctly

type Origin struct {
	Tests   Node
	Helpers Node
}

func NewOrigin() *Origin {
	return &Origin{
		Tests:   NewRootNode(),
		Helpers: NewRootNode(),
	}
}

func ParseOrigin(originPath string, originPkgs []string) (*Origin, error) {
	klog.Infof("From %s loading paths: %s\n", originPath, originPkgs)

	cfg := &packages.Config{
		// TODO: Tweak Mode to see if speed can be improved
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax | packages.NeedCompiledGoFiles |
			packages.NeedDeps | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports |
			packages.NeedExportFile | packages.NeedModule,
		Dir: originPath,
	}

	pkgs, err := packages.Load(cfg, originPkgs...)
	if err != nil {
		return nil, fmt.Errorf("packages.Load failed: %w", err)
	}
	klog.Infof("Loaded %d packages\n", len(pkgs))

	for _, p := range pkgs {
		if len(p.Errors) != 0 {
			for _, e := range p.Errors {
				if !strings.Contains(e.Msg, "no Go files") {
			return nil, fmt.Errorf("internal package errror: %#v", p.Errors)
				}
			}
		}
	}

	o := NewOrigin()
	for _, p := range pkgs {
		ts, hs, err := handlePackage(p)
		if err != nil {
			return nil, err
		}
		o.Tests.AddChildren(ts.GetChildren())
		o.Helpers.AddChildren(hs.GetChildren())
	}

	return o, nil
}

func handlePackage(p *packages.Package) (Node, Node, error) {
	klog.Infof("Inspecting package %s\n", p.ID)

	if len(p.GoFiles) != len(p.Syntax) {
		return nil, nil, fmt.Errorf("len(p.GoFiles) != len(p.Syntax) -- INVESTIGATE")
	}

	tests := NewRootNode()
	helpers := NewRootNode()

	for idx, file := range p.Syntax {
		ts, hs, err := handleFile(p.GoFiles[idx], file, p)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to handle file %s: %w", p.GoFiles[idx], err)
		}
		if ts != nil {
			tests.AddChildren(ts.GetChildren())
		}
		if hs != nil {
			helpers.AddChildren(hs.GetChildren())
		}
	}

	return tests, helpers, nil
}

func handleFile(path string, f *ast.File, p *packages.Package) (Node, Node, error) {
	klog.Infof("Inspecting file %s\n", path)

	tests, err := buildTestsTree(path, f, p)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to build tests: %w", err)
	}

	helpers, err := buildHelpers(path, f, p)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to build helpers: %w", err)
	}

	return tests, helpers, nil
}

func buildHelpers(path string, f *ast.File, p *packages.Package) (Node, error) {
	rn := NewRootNode()
	currentNode := rn

	var topErr error

	inspector := inspector.New([]*ast.File{f})
	inspector.WithStack(
		[]ast.Node{
			&ast.CallExpr{},
			&ast.FuncDecl{},
			&ast.GenDecl{},
		},
		func(n ast.Node, push bool, stack []ast.Node) (proceed bool) {
			if topErr != nil {
				return false
			}
			proceed = true

			if gd, ok := n.(*ast.GenDecl); ok {
				if gd.Tok != token.VAR {
					return
				}
				if len(gd.Specs) == 1 {
					if vs, ok := gd.Specs[0].(*ast.ValueSpec); ok {
						varName := ""
						if len(vs.Names) == 1 {
							varName = vs.Names[0].Name
						}

						if len(vs.Values) == 1 {
							if ce, ok := vs.Values[0].(*ast.CallExpr); ok {
								if se, ok := ce.Fun.(*ast.SelectorExpr); ok {
									if varName == "_" && se.Sel.Name == "Describe" {
										// don't go into Ginkgo tests
										return false
									}
								} else if ident, ok := ce.Fun.(*ast.Ident); ok {
									if varName == "_" && ident.Name == "Describe" {
										return false
									}
								}
							}
						}
					}
				}
				return
			}

			if fn, ok := n.(*ast.FuncDecl); ok {
				if push {
					klog.V(2).Infof("Helper %s - adding new node", fn.Name.Name)
					recv := ""
					if fn.Recv != nil {
						if len(fn.Recv.List) != 1 {
							panic("investigate")
						}
						if star, ok := fn.Recv.List[0].Type.(*ast.StarExpr); ok {
							if ident, ok := star.X.(*ast.Ident); ok {
								recv = ident.Name
							} else {
							panic("investigate")
						}
						} else if ident, ok := fn.Recv.List[0].Type.(*ast.Ident); ok {
						recv = ident.Name
						} else {
							panic("investigate")
						}
					}
					n := NewHelperFunctionNode(p.PkgPath, recv, fn.Name.Name)
					currentNode.AddChild(n)
					currentNode = n
				} else {
					klog.V(2).Infof("Helper %s - going up", fn.Name.Name)
					currentNode = currentNode.GetParent()
				}
				return
			}

			if !push {
				return
			}

			if ce, ok := n.(*ast.CallExpr); ok {
				n, err := NewNodeFromCallExpr(ce, p, path)
				if err != nil {
					topErr = fmt.Errorf("buildHelpers > callExprIntoNode failed: %v", err)
					return
				}
				if n == nil {
					return
				}

				_, isGinkgo := n.(*GinkgoNode)
				if isGinkgo {
					return
				}
				currentNode.AddChild(n)
			}
			return
		},
	)

	return rn, topErr
}

func buildTestsTree(path string, f *ast.File, p *packages.Package) (Node, error) {
	rn := NewRootNode()
	currentNode := rn
	var topErr error

	inspector := inspector.New([]*ast.File{f})
	inspector.WithStack(
		[]ast.Node{
			&ast.CallExpr{},
			&ast.FuncDecl{},
		},
		func(n ast.Node, push bool, stack []ast.Node) (proceed bool) {
			if topErr != nil {
				return false
			}

			proceed = true

			if _, ok := n.(*ast.FuncDecl); ok {
				// don't go into pkg-scoped functions - they'll handled by another inspector
				proceed = false
				return
			}

			if ce, ok := n.(*ast.CallExpr); ok {
				n, err := NewNodeFromCallExpr(ce, p, path)
				if err != nil {
					topErr = fmt.Errorf("callExprIntoNode failed: %w", err)
					return
				}
				if n == nil {
					return
				}

				if _, ok := n.(*GinkgoNode); ok {
					if !push {
						currentNode = currentNode.GetParent()
						klog.V(2).Infof("GINKGO UP | CURRENT NODE: %v\n", currentNode)
						return
					}

					klog.V(2).Infof("GINKGO ADDING NEW TREE NODE: %v\n", n)
					currentNode.AddChild(n)
					currentNode = n
					return
				}

				if !push {
					return
				}

				currentNode.AddChild(n)
			}

			return
		})

	return rn, topErr
}

type FuncCall struct {
	Pkg string
	// Receiver is a name of the struct for the method. If empty, then FuncName is a function.
	Receiver string
	FuncName string
	Args     []string
}

func NewFuncCall(ce *ast.CallExpr, p *packages.Package) (*FuncCall, error) {
	funName := ""
	if ident, ok := ce.Fun.(*ast.Ident); ok {
		funName = ident.Name
	} else if se, ok := ce.Fun.(*ast.SelectorExpr); ok {
		funName = se.Sel.Name
	} else if _, ok := ce.Fun.(*ast.ArrayType); ok {
		return nil, nil
	} else if _, ok := ce.Fun.(*ast.FuncLit); ok {
		return nil, nil
	} else if _, ok := ce.Fun.(*ast.IndexExpr); ok {
		// test/e2e/upgrade/monitor.go - sequence()
		return nil, nil
	} else if _, ok := ce.Fun.(*ast.ParenExpr); ok {
		// test/extended/cluster/metrics/metrics.go - (*TestDuration)(nil)
		return nil, nil
	} else {
		return nil, fmt.Errorf("callExpr.Fun is %T, expected: *ast.SelectorExpr or *ast.Ident", ce.Fun)
	}

	args := func(c *ast.CallExpr) []string {
		args := []string{}
		if len(c.Args) == 0 {
			return nil
		}
		var b bytes.Buffer
		for _, arg := range c.Args {
			printer.Fprint(&b, token.NewFileSet(), arg)
			args = append(args, b.String())
			b.Reset()
		}
		return args
	}(ce)

	callee := typeutil.Callee(p.TypesInfo, ce)
	if callee == nil {
		return &FuncCall{FuncName: funName, Args: args}, nil
	}

	recv := func(o types.Object) string {
		if o == nil {
			return ""
		}
		typ := o.Type()
		sig, ok := typ.(*types.Signature)
		if !ok {
			return ""
		}
		recv := sig.Recv()
		if recv != nil {
			t := recv.Type()
			if pointer, ok := t.(*types.Pointer); ok {
				t = pointer.Elem()
			}

			if named, ok := t.(*types.Named); ok {
				return named.Obj().Name()
			}
		}

		// following returns "" following (executing run from struct{run: func}):
		// https://github.com/openshift/origin/blob/80e4580ea73536c8f9193c749cf5c9e14e70e1ab/test/extended/authorization/authorization_rbac_proxy.go#L860
		// no receiver since these are funcs, not methods, but looks weird in summary
		return ""
	}(callee)

	if callee.Pkg() == nil {
		return &FuncCall{
			Receiver: recv,
			FuncName: funName,
			Args:     args,
		}, nil
	}

	return &FuncCall{
		Pkg:      callee.Pkg().Path(),
		Receiver: recv,
		FuncName: funName,
		Args:     args,
	}, nil
}
