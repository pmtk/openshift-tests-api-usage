// Following program performs a static analysis of tests in OpenShift's origin repository to get K8s/OpenShift APIs particular tests are using.
// It's currently limited to test/extended/ directory.
// Program leverages Go tools to build following code representations: Abstract Syntax Trees (AST), Single Static Assignment (SSA) form,
// and call graphs using Rapid-Type Analysis (RTA) algorithm.
//
// In its current state it's only concerned with typed client-go's from Kubernetes and Openshift.
// Dynamic client-go (untyped, using unstructured.Unstructured) and OpenShift's CLI (`oc`-like interface) are next to be implemented.
package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"golang.org/x/tools/go/ast/astutil"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/callgraph/rta"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"

	"k8s.io/apimachinery/pkg/util/sets"
	klog "k8s.io/klog/v2"
)

// TODO: Handle k8s.io/client-go/dynamic & github.com/openshift/origin/test/extended/util
// TODO: Confirm correctness of the report

func main() {
	var originPathArg = flag.String("origin", "", "path to origin repository")
	var testdirRegexp = flag.String("filter", "", "regexp to filter test dirs")

	flag.Parse()
	defer klog.Flush()

	if *originPathArg == "" {
		klog.Fatalf("Provide path to origin repository using -origin")
	}

	if exists, err := checkIfPathExists(*originPathArg); !exists {
		klog.Exitf("Path %s does not exist", *originPathArg)
	} else if err != nil {
		klog.Exitf("Error occurred when checking if path %s exists: %v", *originPathArg, err)
	}

	var rx *regexp.Regexp
	if *testdirRegexp != "" {
		rx = regexp.MustCompile(*testdirRegexp)
	}

	pkgs, err := getPackages(*originPathArg, rx)
	if err != nil {
		klog.Fatalf("Failed to build SSA.Program: %v", err)
	}

	// Build a Single Static Assignment (SSA) form of packages
	klog.Infof("Building Single Static Assignment (SSA) forms")
	prog, _ := ssautil.AllPackages(pkgs, ssa.InstantiateGenerics|ssa.GlobalDebug)
	prog.Build()

	if err := analyzeProgramUsingRTA(prog, pkgs); err != nil {
		klog.Fatalf("Failed to build report: %v", err)
	}
}

func getPackages(path string, rx *regexp.Regexp) ([]*packages.Package, error) {
	astCfg := packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedImports | packages.NeedTypes | packages.NeedTypesSizes |
			packages.NeedSyntax | packages.NeedTypesInfo | packages.NeedDeps,
		Dir:   path,
		Tests: true,
	}

	originPkgs := func() []string {
		res := []string{}
		_ = filepath.WalkDir(path+"/test/extended/", func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				klog.Fatalf("Error when walking origin's dir tree: %v", err)
			}
			if d.IsDir() &&
				!strings.Contains(path, "testdata") &&
				(rx == nil || (rx != nil && rx.MatchString(path))) {
				res = append(res, path)
			}
			return nil
		})
		return res
	}()
	klog.Infof("%d pkgs to scan: %#v", len(originPkgs), originPkgs)

	// Load packages into a memory with their Abstract Syntax Trees (stored in Syntax member)
	klog.Infof("Building Abstract Syntax Trees")
	ppkgs, err := packages.Load(&astCfg, originPkgs...)
	if err != nil {
		return nil, fmt.Errorf("packages.Load failed: %w", err)
	}
	return ppkgs, nil
}

// getGinkgoNodes traverses anonymous functions of given ssa.Function.
// It expects to get a function called 'init' (the one that bootstraps package and contains anonymous functions used in
// Ginkgo functions such as Describe, Context, and It.)
func getGinkgoNodes(af *ssa.Function, fset *token.FileSet, p *packages.Package, out map[token.Position]*ssa.Function) {
	for _, af2 := range af.AnonFuncs {
		getGinkgoNodes(af2, fset, p, out)
	}

	pos := fset.Position(af.Pos())
	idx := getIndex(p.GoFiles, func(s string) bool { return s == pos.Filename })
	if idx == -1 {
		return
	}

	path, _ := astutil.PathEnclosingInterval(p.Syntax[idx], af.Pos(), af.Pos())
	// Because af.Pos() points at the anonymous function following path is expected: *ast.FuncType, *ast.FuncLit, *ast.CallExpr, ...
	// That 3rd item is ginkgo.{Describe,Context,It}()
	node := path[2]
	callExpr, ok := node.(*ast.CallExpr)
	if !ok {
		return
	}
	fun, ok := callExpr.Fun.(*ast.SelectorExpr)
	if !ok {
		return
	}
	switch fun.Sel.Name {
	case "Describe", "Context", "It":
		out[pos] = af
	}
}

func analyzeProgramUsingRTA(prog *ssa.Program, pkgs []*packages.Package) error {
	// Go through packages from github.com/openshift/origin/test/extended/ and look for init.start block which contains
	// variables created during init such as `var _ = ginkgo.Describe("DESC", func() {...})`.
	// Store their description and pointer to the `func() {...}`.

	ginkgoNodes := map[token.Position]*ssa.Function{}
	for _, pkg := range prog.AllPackages() {
		if pkg != nil && strings.Contains(pkg.Pkg.Path(), "github.com/openshift/origin/test/extended/") {

			// ssa.Program contains more pkgs than we care about
			// intersect with AST []*packages.Package so only packages requested by -filter are handled
			ppkgs := filter(pkgs, func(p *packages.Package) bool {
				return pkg.Pkg.Path() == p.ID
			})
			if len(ppkgs) == 0 {
				continue
			}
			init := pkg.Members["init"].(*ssa.Function)
			getGinkgoNodes(init, prog.Fset, ppkgs[0], ginkgoNodes)
		}
	}

	// Perform a Rapid Type Analysis (RTA) for given functions. This will build a call graph as well.
	// This can even last about 5 minutes or more (depends on how many packages we want to analyze).
	klog.Infof("Building call graphs using Rapid Type Analysis (RTA) - it can take several minutes")
	rtaAnalysis := rta.Analyze(getValues(ginkgoNodes), true)

	w := New(prog.Fset, rtaAnalysis.CallGraph.Nodes)

	wg := &sync.WaitGroup{}
	for position, fun := range ginkgoNodes {
		position := position
		node, found := w.FuncToNode[fun]
		if !found {
			return fmt.Errorf("couldn't find %s (%s) in rtaAnalysis.CallGraph.Nodes[]", fun.Name(), position)
		}
		wg.Add(1)
		go func(node *callgraph.Node, pos token.Position) {
			klog.Infof("Inspecting '%s'", position)
			w.traverseNodes(node, pos, 1)
			wg.Done()
		}(node, position)
	}
	wg.Wait()

	klog.Infof("%+v\n", w.GetReport())

	return nil
}

func New(fset *token.FileSet, funcToNode map[*ssa.Function]*callgraph.Node) *worker {
	return &worker{
		Fset:       fset,
		FuncToNode: funcToNode,
		r:          make(map[string]struct{}),
	}
}

type worker struct {
	Fset       *token.FileSet
	FuncToNode map[*ssa.Function]*callgraph.Node

	m sync.Mutex
	r map[string]struct{}
}

func (w *worker) Report(s string) {
	w.m.Lock()
	defer w.m.Unlock()
	w.r[s] = struct{}{}
}

func (w *worker) GetReport() []string {
	res := []string{}
	for k := range w.r {
		res = append(res, k)
	}
	return res
}

// traverseNodes visits callee's executed by given node.Caller in depth first manner.
// Callees are inspected in terms of pkg and function name to determine if they're an API call.
// Each ginkgo node (e.g. Describe, Context, It) appends a new string to parentTestTree.
// If a function call is considered final (API or ignored), it's reported via either apiChan or ignoreChan.
func (w *worker) traverseNodes(node *callgraph.Node, pos token.Position, depth int) {
	// In some tests there appears to be a call loop going through couple of the same tests over and over.
	// Following block is just reports them.
	// TODO: Investigate and figure out how to break that loop
	if depth > 50 {
		return
	}

	// Node represents specific function within a call graph
	// This function makes a function calls represented as edges
	for _, edge := range node.Out {
		callee := edge.Callee
		funcName := callee.Func.Name()
		pkg := getCalleesPkgPath(callee)

		// Let's go into Defers only if they're not GinkgoRecover
		if _, ok := edge.Site.(*ssa.Defer); ok {
			if funcName != "GinkgoRecover" {
				w.traverseNodes(edge.Callee, pos, depth+1)
				return
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
				nodeToVisit, found := w.FuncToNode[f]
				if !found {
					panic(fmt.Sprintf("f3(%v) not found in m", f.Name()))
				}
				w.traverseNodes(nodeToVisit, w.Fset.Position(nodeToVisit.Func.Pos()), depth+1)
			}

		} else if strings.Contains(pkg, "github.com/openshift/client-go") {
			recv, _ := getRecvFromFunc(callee.Func)
			if recv != nil && checkIfClientGoInterface(recv) {
				for i := 0; i < recv.NumMethods(); i++ {
					if recv.Method(i).Name() == "Get" {
						obj := recv.Method(i).Type().(*types.Signature).Results().At(0).Type().(*types.Pointer).Elem().(*types.Named).Obj()
						w.Report(fmt.Sprintf("%v: %v.openshift.io\n", pos, strings.Split(obj.Pkg().Path(), "/")[3]))
					}
				}
			}

		} else if strings.Contains(pkg, "k8s.io/kubernetes/test/e2e") || strings.Contains(pkg, "github.com/openshift/origin/test/") {
			// Go into the helper functions but avoid recursion
			if edge.Callee != edge.Caller {
				w.traverseNodes(callee, pos, depth+1)
			}
		}
	}
}

// standardPackages is a naive list of Go's standard packages from which function calls
// are completely ignored (they don't even make it to the report's ignored calls)
var standardPackages = func() map[string]struct{} {
	pkgs, err := packages.Load(nil, "std")
	if err != nil {
		panic(err)
	}

	result := make(map[string]struct{})
	for _, p := range pkgs {
		result[p.PkgPath] = struct{}{}
	}

	return result
}()

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

func checkIfPathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}
