package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/printer"
	"go/token"
	"go/types"
	"strings"

	"github.com/davecgh/go-spew/spew"
	klog "k8s.io/klog/v2"

	"golang.org/x/tools/go/ast/inspector"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/types/typeutil"
)

/*
TODO:
- Merge test and helper trees: any helper node should be changed to list of API calls
- Transform API nodes into specific k8s/ocp API packages
- Export a summary: Ginkgo nodes + API used
- Run for whole origin repo
*/

func main() {
	klog.InitFlags(nil)
	flag.Parse()
	defer klog.Flush()

	cfg := &packages.Config{
		// TODO: Tweak Mode to see if speed can be improved
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax | packages.NeedCompiledGoFiles |
			packages.NeedDeps | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports |
			packages.NeedExportFile | packages.NeedModule,
		Dir: "/home/pm/dev/origin/",
	}

	paths := []string{"./test/extended/apiserver"}
	klog.Infof("From %s loading paths: %s\n", cfg.Dir, paths)
	pkgs, err := packages.Load(cfg, paths...)
	if err != nil {
		klog.Fatalf("packages.Load failed: %#v\n", err)
	}
	klog.Infof("Loaded %d packages\n", len(pkgs))

	for _, p := range pkgs {
		if len(p.Errors) != 0 {
			klog.Fatalf("%#v\n", p.Errors)
		}
		handlePackage(p)
	}
}

func handlePackage(p *packages.Package) error {
	klog.Infof("Inspecting package %s\n", p.ID)

	if len(p.GoFiles) != len(p.Syntax) {
		klog.Fatal("len(p.GoFiles) != len(p.Syntax) -- INVESTIGATE\n")
	}

	for idx, file := range p.Syntax {
		handleFile(p.GoFiles[idx], file, p)
	}

	return nil
}

type FuncCall struct {
	Pkg string
	// Receiver is a name of the struct for the method. If empty, then FuncName is a function.
	Receiver string
	FuncName string
}

func getFuncCall(ce *ast.CallExpr, p *packages.Package) (*FuncCall, error) {
	getCallReceiver := func(o types.Object) string {
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
		return ""
	}

	funName := ""
	if ident, ok := ce.Fun.(*ast.Ident); ok {
		funName = ident.Name
	} else if se, ok := ce.Fun.(*ast.SelectorExpr); ok {
		funName = se.Sel.Name
	} else if _, ok := ce.Fun.(*ast.ArrayType); ok {
		return nil, nil
	} else {
		return nil, fmt.Errorf("callExpr.Fun is %T, expected: *ast.SelectorExpr or *ast.Ident", ce.Fun)
	}

	callee := typeutil.Callee(p.TypesInfo, ce)
	if callee == nil {
		return &FuncCall{FuncName: funName}, nil
	}
	if callee.Pkg() == nil {
		return &FuncCall{
			Receiver: getCallReceiver(callee),
			FuncName: funName,
		}, nil
	}

	return &FuncCall{
		Pkg:      callee.Pkg().Path(),
		Receiver: getCallReceiver(callee),
		FuncName: funName,
	}, nil
}

func getCallExprArgs(ce *ast.CallExpr, count int) string {
	if len(ce.Args) == 0 {
		return ""
	}
	if count == -1 || count > len(ce.Args) {
		count = len(ce.Args)
	}
	args := ""
	var b bytes.Buffer
	for i := 0; i < count; i++ {
		if argCallExpr, ok := ce.Args[i].(*ast.BasicLit); ok {
			printer.Fprint(&b, token.NewFileSet(), argCallExpr)
			args += b.String()
		} else {
			args += fmt.Sprintf("%T", ce.Args[i])
		}
		if i != len(ce.Args)-1 {
			args += ", "
		}
		b.Reset()
	}

	return args
}

func buildHelpers(path string, f *ast.File, p *packages.Package) Node {
	rn := NewRootNode()
	currentNode := rn

	inspector := inspector.New([]*ast.File{f})
	inspector.WithStack(
		[]ast.Node{
			&ast.CallExpr{},
			&ast.FuncDecl{},
			&ast.GenDecl{},
		},
		func(n ast.Node, push bool, stack []ast.Node) (proceed bool) {
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
								}
							}
						}
					}
				}
				klog.Infof("Unhandled *ast.GenDecl: %v", spew.Sdump(gd))
				return
			}

			if fn, ok := n.(*ast.FuncDecl); ok {
				if push {
					klog.V(2).Infof("Helper %s - adding new node", fn.Name.Name)
					n := NewHelperFunctionNode(p.PkgPath, fn.Name.Name)
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
				n, err := callExprIntoNode(ce, p, path)
				if err != nil {
					klog.Errorf("buildHelpers > callExprIntoNode failed: %v\n", err)
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

	return rn
}

func buildTestsTree(path string, f *ast.File, p *packages.Package) Node {
	rn := NewRootNode()
	currentNode := rn
	inspector := inspector.New([]*ast.File{f})
	inspector.WithStack(
		[]ast.Node{
			&ast.CallExpr{},
			&ast.FuncDecl{},
		},
		func(n ast.Node, push bool, stack []ast.Node) (proceed bool) {
			proceed = true

			if _, ok := n.(*ast.FuncDecl); ok {
				// don't go into pkg-scoped functions - they'll handled by another inspector
				proceed = false
				return
			}

			if ce, ok := n.(*ast.CallExpr); ok {
				n, err := callExprIntoNode(ce, p, path)
				if err != nil {
					klog.Errorf("callExprIntoNode failed: %v\n", err)
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

	return rn
}

func handleFile(path string, f *ast.File, p *packages.Package) error {
	klog.Infof("Inspecting file %s\n", path)

	tests := buildTestsTree(path, f, p)
	helpers := buildHelpers(path, f, p)

	if tests != nil {
		printTree(tests)
	}
	if helpers != nil {
		printTree(helpers)
	}

	return nil
}

func isAPICall(fc *FuncCall) bool {
	if fc.Pkg == "github.com/openshift/origin/pkg/test/ginkgo/result" {
		return false
	}

	return strings.Contains(fc.Pkg, "github.com/openshift/client-go") ||
		strings.Contains(fc.Pkg, "k8s.io/client-go") ||
		fc.Pkg == "github.com/openshift/origin/test/extended/util"
}

func callExprIntoNode(ce *ast.CallExpr, p *packages.Package, path string) (Node, error) {
	fc, err := getFuncCall(ce, p)
	if err != nil {
		return nil, fmt.Errorf("getFuncCall failed: %w", err)
	}
	if fc == nil {
		return nil, nil
	}

	klog.V(3).Infof("func call: %#v\n", fc)

	if fc.Pkg == "github.com/onsi/ginkgo" {
		if GinkgoNodeType(fc.FuncName) != GinkgoDescribe &&
			GinkgoNodeType(fc.FuncName) != GinkgoIt {
			return nil, nil
		}
		return NewGinkgoNode(GinkgoNodeType(fc.FuncName), path, getCallExprArgs(ce, 1)), nil
	}

	if fc.Pkg == "github.com/openshift/origin/test/extended/util" {
		if fc.FuncName == "Run" {
			return NewAPIUsageNodeWithArgs(fc.Pkg, fc.Receiver, fc.FuncName, getCallExprArgs(ce, -1)), nil
		}
		return nil, nil
	}

	if strings.Contains(fc.Pkg, "github.com/openshift/origin") {
		return NewHelperFunctionNode(fc.Pkg, fc.FuncName), nil
	}

	if isAPICall(fc) {
		return NewAPIUsageNode(fc.Pkg, fc.Receiver, fc.FuncName), nil
	}

	klog.V(2).Infof("WARNING: Ignored FuncCall: %v\n", fc)

	return nil, nil
}
