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
	klog "k8s.io/klog/v2"
)

// TODO: Transform API calls based on (Pkg, Recv, Func, Args) to get actual API used
// TODO: Prepare final report: tests (It level) +APIs used

var ignoreDirs = []string{
	"test/extended/testdata", "test/extended/testdata/",
}

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
		if d.IsDir() {
			for _, ignore := range ignoreDirs {
				if path == originPath+ignore {
					return nil
				}
			}
			pkgs = append(pkgs, path)
		}
		return nil
	})

	klog.V(2).Infof("Pkgs to scan: %#v", pkgs)
	buildReportUsingRTA(originPath, pkgs)
}

// FunCallInTest is a function FunCallInTest in context of specific ginkgo node
type FunCallInTest struct {
	Test    []string // [Describe-Description, [Context-Description...], It-Description]
	Call    FuncCall
	Ignored bool // Ignored informs if function call is not API calls, but persisted for debugging purposes
}

type FuncCall struct {
	Pkg  string
	Recv string
	Func string
	Args []string
}

type report struct {
	// Ignored
}

func buildReportUsingRTA(originPath string, pkgs []string) error {
	cfg := packages.Config{
		Mode:  packages.LoadAllSyntax,
		Dir:   originPath,
		Tests: true,
	}
	klog.V(2).Infof("packages.Load - start\n")
	start := time.Now()
	initial, err := packages.Load(&cfg, "./test/extended/apiserver/") // TODO: pkgs
	if err != nil {
		return fmt.Errorf("packages.Load failed: %w", err)
	}
	klog.V(2).Infof("packages.Load - end after %s\n", time.Since(start))

	klog.V(2).Infof("ssautil.AllPackages - start\n")
	start = time.Now()
	prog, _ := ssautil.AllPackages(initial, ssa.InstantiateGenerics) // ssa.PrintPackages
	klog.V(2).Infof("ssautil.AllPackages - end after %s\n", time.Since(start))

	klog.V(2).Infof("prog.Build - start\n")
	start = time.Now()
	prog.Build()
	klog.V(2).Infof("prog.Build - end after %s\n", time.Since(start))

	fcm := map[string]*ssa.Function{}

	for _, p := range prog.AllPackages() {
		if p != nil && strings.Contains(p.Pkg.Path(), "github.com/openshift/origin/test/extended/apiserver") {
			a := p.Members["init"]
			a2 := a.(*ssa.Function)
			bb := a2.Blocks[1] // "init.start"

			for _, instr := range bb.Instrs {
				if c, ok := instr.(*ssa.Call); ok {
					f := c.Call.Value.(*ssa.Function)
					if f.Name() == "Describe" {
						cnst, ok := c.Call.Args[0].(*ssa.Const)
						if !ok {
							return fmt.Errorf("expected c.Call.Args[0] to be type *ssa.Const: %#v", c.Call.Args[0])
						}
						if cnst.Type().String() != "string" {
							return fmt.Errorf("expected cnst.Type() to be string: %#v", cnst.Type())
						}
						anonFun := c.Call.Args[1].(*ssa.Function)
						fcm[strings.ReplaceAll(cnst.Value.String(), "\"", "")] = anonFun
					}
				}
			}
		}
	}

	fcs := make([]*ssa.Function, 0, len(fcm))
	for _, f := range fcm {
		fcs = append(fcs, f)
	}

	klog.V(2).Infof("rta.Analyze - start\n")
	start = time.Now()
	rtaAnalysis := rta.Analyze(fcs, true)
	klog.V(2).Infof("rta.Analyze - end after %s\n", time.Since(start))

	// TODO: Parallelize traverseNodes + bufferedChannel? Don't think we'd have great speed up since building RTA is most time consuming
	callChan := make(chan FunCallInTest)
	calls := []FunCallInTest{}

	go func() {
		for c := range callChan {
			calls = append(calls, c)
		}
	}()

	klog.V(2).Infof("Traversing tests - start\n")
	start = time.Now()
	for desc, fun := range fcm {
		node, found := rtaAnalysis.CallGraph.Nodes[fun]
		if !found {
			return fmt.Errorf("couldn't find %s (%s) in rtaAnalysis.CallGraph.Nodes[]", fun.Name(), desc)
		}
		traverseNodes(rtaAnalysis.CallGraph.Nodes, node, []string{desc}, callChan)
	}
	klog.V(2).Infof("Traversing tests - end after %s\n", time.Since(start))

	ignored := map[string]struct{}{}
	mergedApiUsage := map[string]map[string]struct{}{}
	for _, c := range calls {
		if c.Ignored {
			ignored[c.Call.Pkg] = struct{}{}
		} else {
			key := strings.Join(c.Test, " ")
			if mergedApiUsage[key] == nil {
				mergedApiUsage[key] = map[string]struct{}{}
			}
			mergedApiUsage[key][c.Call.Pkg] = struct{}{}
		}
	}

	return nil
}

func traverseNodes(m map[*ssa.Function]*callgraph.Node, node *callgraph.Node, parentTestTree []string, callChan chan<- FunCallInTest) {
	for _, edge := range node.Out {
		callee := edge.Callee
		funcName := callee.Func.Name()
		pkg := getCalleesPkgPath(callee)

		testTree := make([]string, len(parentTestTree))
		copy(testTree, parentTestTree)

		if _, ok := edge.Site.(*ssa.Defer); ok {
			traverseNodes(m, edge.Callee, testTree, callChan)
			return
		}

		var nodeToVisit *callgraph.Node

		call := edge.Site.Value()
		args := call.Call.Args

		recv := ""
		if callee.Func.Signature.Recv() != nil {
			if ptr, ok := callee.Func.Params[0].Type().(*types.Pointer); ok {
				recv = ptr.Elem().(*types.Named).Obj().Name()
			} else if named, ok := callee.Func.Params[0].Type().(*types.Named); ok {
				recv = named.Obj().Name()
			} else if strct, ok := callee.Func.Params[0].Type().(*types.Struct); ok {
				// error.Error()
				recv = strct.Field(0).Name()
			} else {
				panic(".")
			}
		}

		fcit := FunCallInTest{
			Test: testTree,
			Call: FuncCall{
				Pkg:  pkg,
				Recv: recv,
				Func: funcName,
				Args: argsToStrings(edge.Site.Common().Args),
			},
		}

		if pkg == "github.com/onsi/ginkgo" {
			if funcName == "It" || funcName == "Context" {
				ginkgoDesc := ""

				// arg0 can be const(string) or call(fmt.Sprintf)
				if ssaConst, ok := args[0].(*ssa.Const); ok {
					// ginkgoDesc = cnst.Value.String()
					ginkgoDesc = ssaConst.Value.ExactString()
				} else if ssaCall, ok := args[0].(*ssa.Call); ok {
					if ssaCall.Call.Value.Name() == "Sprintf" {
						ginkgoDesc = ssaCall.Call.Args[0].(*ssa.Const).Value.String()
					} else {
						panic("unknown args[0] call")
					}
				} else {
					panic("unknown args[0] type")
				}

				f1, ok := args[1].(*ssa.MakeInterface)
				if !ok {
					panic(".")
				}
				f2, ok := f1.X.(*ssa.MakeClosure)
				if !ok {
					panic(".")
				}
				f3, ok := f2.Fn.(*ssa.Function)
				if !ok {
					panic(".")
				}

				var found bool
				// Context/It("desc", func(){))
				//              visit ^^^^^^^^ by setting childNode
				nodeToVisit, found = m[f3]
				if !found {
					panic(".")
				}
				testTree = append(testTree, strings.ReplaceAll(ginkgoDesc, "\"", ""))
			}
		} else {
			if strings.Contains(pkg, "github.com/openshift/client-go") ||
				strings.Contains(pkg, "k8s.io/client-go") ||
				strings.Contains(pkg, "k8s.io/apimachinery") ||
				pkg == "github.com/openshift/origin/test/extended/util" {
				// Handle untyped clients like: k8s.io/client-go/dynamic.Create("context.Background()", "new k8s.io/apimachinery/pkg/apis/meta/v1/unstructured.Unstructured (complit)", "*t27", "nil:[]string")
				// Handle typed clients as well

				// Store API call
				callChan <- fcit
				klog.V(3).Infof("%#v\n", fcit)

			} else if strings.Contains(pkg, "k8s.io/kubernetes/test/e2e") || strings.Contains(pkg, "github.com/openshift/origin/test/") {
				// go into the helper functions
				nodeToVisit = callee

			} else {
				// Store non-API call for debug purposes
				fcit.Ignored = true
				callChan <- fcit
			}
		}

		if nodeToVisit != nil {
			traverseNodes(m, nodeToVisit, testTree, callChan)
		}
	}
}

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
			// callee can have a receiver, but not be a part of a Pkg like error
			return callee.Func.Params[0].Object().Pkg().Path()
		}
	} else {
		panic("unknown calleePkg")
	}
	return ""
}
