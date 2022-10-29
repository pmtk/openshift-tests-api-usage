package traverser

import (
	"fmt"
	"go/token"
	"go/types"
	"strings"
	"sync"

	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/ssa"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
)

func New(fset *token.FileSet, funcToNode map[*ssa.Function]*callgraph.Node) *Traverser {
	return &Traverser{
		Fset:       fset,
		FuncToNode: funcToNode,
		r:          make(map[string]struct{}),
	}
}

type Traverser struct {
	Fset       *token.FileSet
	FuncToNode map[*ssa.Function]*callgraph.Node

	m sync.Mutex
	r map[string]struct{}
}

func (t *Traverser) GetReport() []string {
	res := []string{}
	for k := range t.r {
		res = append(res, k)
	}
	return res
}

func (t *Traverser) Analyze(roots map[token.Position]*ssa.Function) error {
	wg := &sync.WaitGroup{}
	for position, fun := range roots {
		position := position
		node, found := t.FuncToNode[fun]
		if !found {
			return fmt.Errorf("couldn't find %s (%s) in rtaAnalysis.CallGraph.Nodes[]", fun.Name(), position)
		}
		wg.Add(1)
		go func(node *callgraph.Node, pos token.Position) {
			klog.Infof("Inspecting '%s'", position)
			t.traverseNodes(node, pos, 1)
			wg.Done()
		}(node, position)
	}
	wg.Wait()
	return nil
}

func (t *Traverser) report(s string) {
	t.m.Lock()
	defer t.m.Unlock()
	t.r[s] = struct{}{}
}

// traverseNodes visits callee's executed by given node.Caller in depth first manner.
// Callees are inspected in terms of pkg and function name to determine if they're an API call.
// Each ginkgo node (e.g. Describe, Context, It) appends a new string to parentTestTree.
// If a function call is considered final (API or ignored), it's reported via either apiChan or ignoreChan.
func (t *Traverser) traverseNodes(node *callgraph.Node, pos token.Position, depth int) bool {
	if depth > 50 {
		return false
	}

	// Node represents specific function within a call graph
	// This function makes a function calls represented as edges
	for _, edge := range node.Out {
		callee := edge.Callee
		funcName := callee.Func.Name()
		pkg := getCalleesPkgPath(callee)
		funcPos := t.Fset.Position(callee.Func.Pos())
		_ = funcPos

		// Let's go into Defers only if they're not GinkgoRecover
		if _, ok := edge.Site.(*ssa.Defer); ok {
			if funcName != "GinkgoRecover" {
				if !t.traverseNodes(edge.Callee, pos, depth+1) {
					return false
				}
			}
			continue
		}

		// If function call is ginkgo.Context/Describe/It then
		// we want to get its description
		// Context/It("desc", func(){))
		//          and visit ^^^^^^^^
		if pkg == "github.com/onsi/ginkgo" {
			if funcName == "Context" || funcName == "Describe" || funcName == "It" {
				args := edge.Site.Value().Call.Args

				f := getFuncFromValue(args[1])
				nodeToVisit, found := t.FuncToNode[f]
				if !found {
					panic(fmt.Sprintf("f3(%v) not found in m", f.Name()))
				}
				if !t.traverseNodes(nodeToVisit, t.Fset.Position(nodeToVisit.Func.Pos()), depth+1) {
					return false
				}
			}

		} else if strings.Contains(pkg, "github.com/openshift/client-go") {
			recv, _ := getRecvFromFunc(callee.Func)
			if recv != nil && checkIfClientGoInterface(recv) {
				for i := 0; i < recv.NumMethods(); i++ {
					if recv.Method(i).Name() == "Get" {
						obj := recv.Method(i).Type().(*types.Signature).Results().At(0).Type().(*types.Pointer).Elem().(*types.Named).Obj()
						t.report(fmt.Sprintf("%v: %v.openshift.io\n", pos, strings.Split(obj.Pkg().Path(), "/")[3]))
					}
				}
			}

		} else if strings.Contains(pkg, "k8s.io/kubernetes/test/e2e") || strings.Contains(pkg, "github.com/openshift/origin/test/") {
			// Go into the helper functions but avoid recursion
			if edge.Callee != edge.Caller {
				if !t.traverseNodes(callee, pos, depth+1) {
					return false
				}
			}
		}
	}
	return true
}

func getCalleesPkgPath(callee *callgraph.Node) string {
	if callee.Func.Pkg != nil {
		return callee.Func.Pkg.Pkg.Path()
	} else if callee.Func.Signature.Recv() != nil {
		// if Recv != nil, then it's stored as a first parameter
		if callee.Func.Params[0].Object().Pkg() != nil {
			return callee.Func.Params[0].Object().Pkg().Path()
		} else {
			// callee can have a receiver, but not be a part of a Pkg like error
			return ""
		}
	} else if callee.Func.Object() != nil && callee.Func.Object().Pkg() != nil {
		return callee.Func.Object().Pkg().Path()
	}

	panic("unknown calleePkg")
}

func getFuncFromValue(v ssa.Value) *ssa.Function {
	getFuncFromClosure := func(mc *ssa.MakeClosure) *ssa.Function {
		if ssaFunc, ok := mc.Fn.(*ssa.Function); ok {
			return ssaFunc
		}
		klog.Fatalf("getFuncFromClosure: mc.Fn is not *ssa.Function")
		return nil
	}

	switch v := v.(type) {
	case *ssa.MakeInterface:
		switch x := v.X.(type) {
		case *ssa.MakeClosure:
			return getFuncFromClosure(x)
		case *ssa.MakeInterface:
			if c, ok := x.X.(*ssa.Call); ok {
				if mc, ok := c.Call.Value.(*ssa.MakeClosure); ok {
					return getFuncFromClosure(mc)
				}
			}
		case *ssa.Function:
			return x
		case *ssa.Call:
			if mc, ok := x.Call.Value.(*ssa.MakeClosure); ok {
				return getFuncFromClosure(mc)
			}
		default:
			klog.Fatalf("getFuncFromValue: mi.X.(type) not handled: %T", x)
		}

	case *ssa.MakeClosure:
		return getFuncFromClosure(v)
	case *ssa.Function:
		return v
	case *ssa.Call:
		if f, ok := v.Call.Value.(*ssa.Function); ok {
			return f
		} else {
			klog.Fatalf("getFuncFromValue: v.Call.Value's type is unexpected: %T", v.Call.Value)
		}
	}

	klog.Fatalf("getFuncFromValue: v's type is unexpected: %T", v)
	return nil
}

var expectedMethods = []string{
	"Create",
	"Update",
	"Delete",
	"DeleteCollection",
	"Get",
	"List",
	"Watch",
	"Patch",
	// Not every interface we're interested in implements "UpdateStatus", "Apply", "ApplyStatus"
}

// checkIfClientGoInterface checks given types.Named if its methods are intersecting with a set of methods that client-go interface should have
func checkIfClientGoInterface(n *types.Named) bool {
	methods := sets.String{}
	for i := 0; i < n.NumMethods(); i++ {
		methods.Insert(n.Method(i).Name())
	}
	return methods.HasAll(expectedMethods...)
}

// getRecvFromFunc tries to get the receiver object and/or its name from given ssa.Function
func getRecvFromFunc(f *ssa.Function) (*types.Named, string) {
	if f.Signature.Recv() == nil {
		return nil, ""
	}

	arg0type := f.Params[0].Type()

	if ptr, ok := arg0type.(*types.Pointer); ok {
		return ptr.Elem().(*types.Named),
			ptr.Elem().(*types.Named).Obj().Name()

	} else if named, ok := arg0type.(*types.Named); ok {
		return named,
			named.Obj().Name()

	} else if strct, ok := arg0type.(*types.Struct); ok {
		// Some structure embedded some `error` and Error() was called
		return nil,
			strct.Field(0).Name()

	} else {
		panic("investigate arg0type type")
	}
}
