package main

import (
	"flag"
	"fmt"
	"go/types"
	"io/fs"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/callgraph/rta"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"

	"k8s.io/apimachinery/pkg/util/sets"
	klog "k8s.io/klog/v2"
)

// TODO: Report as a file

func main() {
	klog.InitFlags(nil)
	flag.Parse()
	defer klog.Flush()

	originPath := "/home/pm/dev/origin/" // TODO: Program arg

	pkgs := []string{}
	filepath.WalkDir(originPath+"test/extended/", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			klog.Fatalf("Error when walking origin's dir tree: %v", err)
		}
		if d.IsDir() && !strings.Contains(path, "testdata") {
			pkgs = append(pkgs, path)
		}
		return nil
	})

	// TESTING
	pkgs = []string{
		"/home/pm/dev/origin/test/extended/adminack",
		// "/home/pm/dev/origin/test/extended/apiserver",
		// "/home/pm/dev/origin/test/extended/authentication",
		// "/home/pm/dev/origin/test/extended/authorization",
		// "/home/pm/dev/origin/test/extended/authorization/rbac",
		// "/home/pm/dev/origin/test/extended/baremetal",
		// "/home/pm/dev/origin/test/extended/bootstrap_user",
		// "/home/pm/dev/origin/test/extended/builds",
		// "/home/pm/dev/origin/test/extended/ci",
		// "/home/pm/dev/origin/test/extended/cli",
		// "/home/pm/dev/origin/test/extended/cluster",
		// "/home/pm/dev/origin/test/extended/cluster/metrics",
		// "/home/pm/dev/origin/test/extended/cmd",
		// "/home/pm/dev/origin/test/extended/controller_manager",
		// "/home/pm/dev/origin/test/extended/coreos",
		// "/home/pm/dev/origin/test/extended/crdvalidation",
		// "/home/pm/dev/origin/test/extended/csrapprover",
		// "/home/pm/dev/origin/test/extended/deployments",
		// "/home/pm/dev/origin/test/extended/dns",
		// "/home/pm/dev/origin/test/extended/dr",
		// "/home/pm/dev/origin/test/extended/etcd",
		// "/home/pm/dev/origin/test/extended/etcd/helpers",
		// "/home/pm/dev/origin/test/extended/idling",
		// "/home/pm/dev/origin/test/extended/image_ecosystem",
		// "/home/pm/dev/origin/test/extended/imageapis",
		// "/home/pm/dev/origin/test/extended/images",
		// "/home/pm/dev/origin/test/extended/images/trigger",
		// "/home/pm/dev/origin/test/extended/jobs",
		// "/home/pm/dev/origin/test/extended/machines",
		// "/home/pm/dev/origin/test/extended/networking",
		// "/home/pm/dev/origin/test/extended/oauth",
		// "/home/pm/dev/origin/test/extended/olm",
		// "/home/pm/dev/origin/test/extended/operators",
		// "/home/pm/dev/origin/test/extended/pods",
		// "/home/pm/dev/origin/test/extended/project",
		// "/home/pm/dev/origin/test/extended/prometheus",
		// "/home/pm/dev/origin/test/extended/prometheus/client",
		// "/home/pm/dev/origin/test/extended/quota",
		// "/home/pm/dev/origin/test/extended/router",
		// "/home/pm/dev/origin/test/extended/router/certgen",
		// "/home/pm/dev/origin/test/extended/router/grpc-interop",
		// "/home/pm/dev/origin/test/extended/router/h2spec",
		// "/home/pm/dev/origin/test/extended/router/shard",
		// "/home/pm/dev/origin/test/extended/scheduling",
		// "/home/pm/dev/origin/test/extended/scheme",
		// "/home/pm/dev/origin/test/extended/security",
		// "/home/pm/dev/origin/test/extended/single_node",
		// "/home/pm/dev/origin/test/extended/tbr_health",
		// "/home/pm/dev/origin/test/extended/templates",
		// "/home/pm/dev/origin/test/extended/templates/openservicebroker",
		// "/home/pm/dev/origin/test/extended/templates/openservicebroker/api",
		// "/home/pm/dev/origin/test/extended/templates/openservicebroker/client",
		// "/home/pm/dev/origin/test/extended/user",
		// "/home/pm/dev/origin/test/extended/util",
		// "/home/pm/dev/origin/test/extended/util/alibabacloud",
		// "/home/pm/dev/origin/test/extended/util/annotate",
		// "/home/pm/dev/origin/test/extended/util/annotate/generated",
		// "/home/pm/dev/origin/test/extended/util/azure",
		// "/home/pm/dev/origin/test/extended/util/baremetal",
		// "/home/pm/dev/origin/test/extended/util/cluster",
		// "/home/pm/dev/origin/test/extended/util/db",
		// "/home/pm/dev/origin/test/extended/util/disruption",
		// "/home/pm/dev/origin/test/extended/util/disruption/controlplane",
		// "/home/pm/dev/origin/test/extended/util/disruption/frontends",
		// "/home/pm/dev/origin/test/extended/util/disruption/imageregistry",
		// "/home/pm/dev/origin/test/extended/util/ibmcloud",
		// "/home/pm/dev/origin/test/extended/util/image",
		// "/home/pm/dev/origin/test/extended/util/imageregistryutil",
		// "/home/pm/dev/origin/test/extended/util/jenkins",
		// "/home/pm/dev/origin/test/extended/util/kubevirt",
		// "/home/pm/dev/origin/test/extended/util/nutanix",
		// "/home/pm/dev/origin/test/extended/util/oauthserver",
		// "/home/pm/dev/origin/test/extended/util/oauthserver/tokencmd",
		// "/home/pm/dev/origin/test/extended/util/openshift",
		// "/home/pm/dev/origin/test/extended/util/openshift/clusterversionoperator",
		// "/home/pm/dev/origin/test/extended/util/operator",
		// "/home/pm/dev/origin/test/extended/util/ovirt",
		// "/home/pm/dev/origin/test/extended/util/prometheus",
		// "/home/pm/dev/origin/test/extended/util/url",
	}

	buildReportUsingRTA(originPath, pkgs)
}

type APICallInTest struct {
	Test []string // [Describe-Description, [Describe/Context-Description...], It-Description]
	GVK  string
}

type IgnoredCallInTest struct {
	Test []string
	Pkg  string
	Recv string
	Func string
	Args []string
}

func buildReportUsingRTA(originPath string, pkgs []string) error {
	cfg := packages.Config{
		Mode:  packages.LoadAllSyntax,
		Dir:   originPath,
		Tests: true,
	}

	klog.V(2).Infof("Pkgs (%v) to scan: %#v", len(pkgs), pkgs)

	start := time.Now()
	// Load packages into a memory with their Abstract Syntax Trees (stored in Syntax member)
	initial, err := packages.Load(&cfg, pkgs...)
	if err != nil {
		return fmt.Errorf("packages.Load failed: %w", err)
	}
	klog.V(2).Infof("packages.Load done in %s\n", time.Since(start))

	start = time.Now()
	// Build a Single Static Assignment (SSA) form of packages
	prog, _ := ssautil.AllPackages(initial, ssa.InstantiateGenerics)
	klog.V(2).Infof("ssautil.AllPackages done in %s\n", time.Since(start))

	start = time.Now()
	prog.Build()
	klog.V(2).Infof("prog.Build done in %s\n", time.Since(start))

	// Go through packages from github.com/openshift/origin/test/extended/ and look for init.start block which contains
	// variables created during init such as `var _ = ginkgo.Describe("DESC", func() {...})`.
	// Store their description and pointer to the `func() {...}`.
	describeFuncs := map[string]*ssa.Function{}
	for _, pkg := range prog.AllPackages() {
		if pkg != nil && strings.Contains(pkg.Pkg.Path(), "github.com/openshift/origin/test/extended/") {
			init := pkg.Members["init"].(*ssa.Function)
			initStart := init.Blocks[1] // "init.start"

			for _, instr := range initStart.Instrs {
				if c, ok := instr.(*ssa.Call); ok {
					if f, ok := c.Call.Value.(*ssa.Function); ok {
						if f.Name() == "Describe" {
							cnst, ok := c.Call.Args[0].(*ssa.Const)
							if !ok {
								return fmt.Errorf("expected c.Call.Args[0] to be type *ssa.Const: %#v", c.Call.Args[0])
							}
							if cnst.Type().String() != "string" {
								return fmt.Errorf("expected cnst.Type() to be string: %#v", cnst.Type())
							}
							anonFun := c.Call.Args[1].(*ssa.Function)
							describeFuncs[strings.ReplaceAll(cnst.Value.String(), "\"", "")] = anonFun
						}
					}
				}
			}
		}
	}

	describeFuncsSlice := make([]*ssa.Function, 0, len(describeFuncs))
	for _, fun := range describeFuncs {
		describeFuncsSlice = append(describeFuncsSlice, fun)
	}

	start = time.Now()
	// Perform a Rapid Type Analysis (RTA) for given functions. This will build a call graph as well.
	// This can even lasts about 5 minutes or more (depends on how many packages we want to analyze).
	rtaAnalysis := rta.Analyze(describeFuncsSlice, true)
	klog.V(2).Infof("rta.Analyze done in %s\n", time.Since(start))

	// Go routine for simple function calls within tests receival instead of returning the results via `return` and merging them in a recursive function.
	apiChan := make(chan APICallInTest)
	apiCalls := []APICallInTest{}
	ignoredChan := make(chan IgnoredCallInTest) // Ignored calls are persisted for debugging
	ignoredCalls := []IgnoredCallInTest{}
	go func() {
		for {
			select {
			case c, ok := <-apiChan:
				if !ok {
					return
				}
				apiCalls = append(apiCalls, c)
			case c, ok := <-ignoredChan:
				if !ok {
					return
				}
				ignoredCalls = append(ignoredCalls, c)
			}
		}
	}()

	// Go through previously stored list of functions given to ginkgo.Describe and traverse their call graph.
	klog.V(2).Infof("Traversing %d tests - start\n", len(describeFuncs))
	start = time.Now()
	for desc, fun := range describeFuncs {
		node, found := rtaAnalysis.CallGraph.Nodes[fun]
		if !found {
			return fmt.Errorf("couldn't find %s (%s) in rtaAnalysis.CallGraph.Nodes[]", fun.Name(), desc)
		}
		klog.V(2).Infof("Inspecting test: %s", desc)
		traverseNodes(rtaAnalysis.CallGraph.Nodes, node, []string{desc}, apiChan, ignoredChan)
	}
	klog.V(2).Infof("Traversing tests - done in %s\n", time.Since(start))
	close(apiChan)
	close(ignoredChan)

	// Just some testing purposes print out
	for _, c := range apiCalls {
		fmt.Printf("API: %s -> %s\n", strings.Join(c.Test, " "), c.GVK)
	}

	fmt.Printf("\n\n\n")

	for _, c := range ignoredCalls {
		fmt.Printf("IGNORED: (%s.%s).%s()\n", c.Pkg, c.Recv, c.Func)
	}

	return nil
}

func traverseNodes(m map[*ssa.Function]*callgraph.Node, node *callgraph.Node, parentTestTree []string,
	apiChan chan<- APICallInTest, ignoreChan chan<- IgnoredCallInTest) {

	// node represents specific function within a call graph
	// this function makes a function calls represented as edges
	for _, edge := range node.Out {
		callee := edge.Callee
		funcName := callee.Func.Name()
		pkg := getCalleesPkgPath(callee)

		testTree := make([]string, len(parentTestTree))
		copy(testTree, parentTestTree)

		// let's go into Defers only if they're not GinkgoRecover
		if _, ok := edge.Site.(*ssa.Defer); ok {
			if funcName != "GinkgoRecover" {
				traverseNodes(m, edge.Callee, testTree, apiChan, ignoreChan)
			}
			continue
		}

		// if function call is ginkgo.Context/Describe/It then
		// we want to get its description
		// Context/It("desc", func(){))
		//          and visit ^^^^^^^^
		if pkg == "github.com/onsi/ginkgo" {
			if funcName == "Context" || funcName == "Describe" || funcName == "It" {
				args := edge.Site.Value().Call.Args

				f := getFuncFromValue(args[1])
				nodeToVisit, found := m[f]
				if !found {
					panic(fmt.Sprintf("f3(%v) not found in m", f.Name()))
				}

				ginkgoDesc := getStringFromValue(args[0])
				testTree = append(testTree, strings.ReplaceAll(ginkgoDesc, "\"", ""))
				traverseNodes(m, nodeToVisit, testTree, apiChan, ignoreChan)
			}

		} else if strings.Contains(pkg, "k8s.io/client-go/dynamic") {
			// TODO Handle untyped clients like: k8s.io/client-go/dynamic.Create("context.Background()", "new k8s.io/apimachinery/pkg/apis/meta/v1/unstructured.Unstructured (complit)", "*t27", "nil:[]string")

		} else if strings.Contains(pkg, "github.com/openshift/client-go") || strings.Contains(pkg, "k8s.io/client-go") {
			recv, _ := getRecvFromFunc(callee.Func)
			if recv == nil {
				klog.Infof("recv for openshift-clientgo is nil")
				continue
			}

			if checkIfClientGoInterface(recv) {
				for i := 0; i < recv.NumMethods(); i++ {
					if recv.Method(i).Name() == "Get" {
						obj := recv.Method(i).Type().(*types.Signature).Results().At(0).Type().(*types.Pointer).Elem().(*types.Named).Obj()
						gvk := fmt.Sprintf("%s.%s", obj.Pkg().Path(), obj.Name())
						apiChan <- APICallInTest{
							Test: testTree,
							GVK:  gvk,
						}
					}
				}
			}

		} else if pkg == "github.com/openshift/origin/test/extended/util" {
			// TODO Handle only Run()...
			recv, recvName := getRecvFromFunc(callee.Func)
			stop := 0
			_ = stop
			_ = recv
			_ = recvName

		} else if strings.Contains(pkg, "k8s.io/kubernetes/test/e2e") || strings.Contains(pkg, "github.com/openshift/origin/test/") {
			// go into the helper functions but avoid recursion
			if edge.Callee != edge.Caller {
				traverseNodes(m, callee, testTree, apiChan, ignoreChan)
			}

		} else {
			if _, found := standardPackages[pkg]; !found {
				// store ignored calls for debug purposes
				_, recvName := getRecvFromFunc(callee.Func)
				ignoreChan <- IgnoredCallInTest{
					Test: testTree,
					Pkg:  pkg,
					Recv: recvName,
					Func: funcName,
					Args: argsToStrings(edge.Site.Common().Args),
				}
			}
		}
	}
}

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

func argsToStrings(args []ssa.Value) []string {
	as := []string{}
	for _, arg := range args {
		as = append(as, arg.String())
	}
	return as
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
		default:
			klog.Fatalf("getFuncFromValue: mi.X.(type) not handled: %T", x)
		}

	case *ssa.MakeClosure:
		return getFuncFromClosure(v)
	case *ssa.Function:
		return v
	}

	klog.Fatalf("getFuncFromValue: v's type is unexpected: %T", v)
	return nil
}

// getStringFromValue converts ssa.Value to string, panics if fails
// It expects to receive an arg0 of ginkgo's Describe/Context/It
func getStringFromValue(v ssa.Value) string {
	switch v := v.(type) {
	case *ssa.Const:
		// Describe(STRING, ...)
		return v.Value.ExactString()

	case *ssa.Call:
		if v.Call.Value.Name() == "Sprintf" {
			// Describe(fmt.Sprintf(STRING, ...), ...)
			return v.Call.Args[0].(*ssa.Const).Value.String()
		} else {
			klog.Fatalf("getStringFromValue: unexpected call: %s", v.Call.Value.Name())
		}

	case *ssa.BinOp:
		// It("test/cmd/"+currFilename, ...)
		panic("getStringFromValue: *ssa.BinOp")
	}

	panic(fmt.Sprintf("getStringFromValue: v's type in unexpected: %T", v))
}

var expectedMethods = []string{
	"Create",
	"Update",
	// "UpdateStatus",
	"Delete",
	"DeleteCollection",
	"Get",
	"List",
	"Watch",
	"Patch",
	// "Apply",
	// "ApplyStatus",
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
		// some structure embedded some `error` and Error() was called
		return nil,
			strct.Field(0).Name()

	} else {
		panic("investigate arg0type type")
	}
}
