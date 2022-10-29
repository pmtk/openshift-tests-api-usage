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
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/pmtk/openshift-tests-api-usage/pkg/traverser"

	"golang.org/x/tools/go/ast/astutil"
	"golang.org/x/tools/go/callgraph/rta"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"

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

	w := traverser.New(prog.Fset, rtaAnalysis.CallGraph.Nodes)
	if err := w.Analyze(ginkgoNodes); err != nil {
		panic(err)
	}
	fmt.Printf("%+v\n", w.GetReport())
}

func getPackages(path string, rx *regexp.Regexp) ([]*packages.Package, error) {
	astCfg := packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedImports | packages.NeedTypes | packages.NeedTypesSizes |
			packages.NeedSyntax | packages.NeedTypesInfo | packages.NeedDeps,
		Dir: path,
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
