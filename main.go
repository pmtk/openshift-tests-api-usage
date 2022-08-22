package main

import (
	"flag"
	"fmt"
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

// call is a function call in context of specific ginkgo node
type call struct {
	Test    []string // [Describe-Description, [Context-Description...], It-Description]
	Call    funcCall
	Ignored bool // Ignored informs if function call is not API calls, but persisted for debugging purposes
}

type funcCall struct {
	Pkg  string
	Func string
	Args []string
}

// type pkgCalls struct {
// 	Pkg  string
// 	Func []string
// }

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
	callChan := make(chan call)
	calls := []call{}

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

func traverseNodes(m map[*ssa.Function]*callgraph.Node, node *callgraph.Node, parentTestTree []string, callChan chan<- call) {
	for _, edge := range node.Out {
		callee := edge.Callee
		calleeFuncName := callee.Func.Name()
		calleePkg := ""
		if callee.Func.Pkg != nil {
			calleePkg = callee.Func.Pkg.Pkg.Path()
		} else if callee.Func.Signature.Recv() != nil {
			// if Recv != nil, then it's stored as a first parameter
			if callee.Func.Params[0].Object().Pkg() != nil {
				// callee can have a receiver, but not be a part of a Pkg like error
				calleePkg = callee.Func.Params[0].Object().Pkg().Path()
			}
		} else {
			panic("unknown calleePkg")
		}

		testTree := make([]string, len(parentTestTree))
		copy(testTree, parentTestTree)

		var childNode *callgraph.Node

		if calleePkg == "github.com/onsi/ginkgo" {
			if calleeFuncName == "It" || calleeFuncName == "Context" {
				call := edge.Site.Value()
				args := call.Call.Args
				ginkgoDesc := ""

				// arg0 can be const(string) or call(fmt.Sprintf)
				if cnst, ok := args[0].(*ssa.Const); ok {
					// ginkgoDesc = cnst.Value.String()
					ginkgoDesc = cnst.Value.ExactString()
				} else if c, ok := args[0].(*ssa.Call); ok {
					callName := c.Call.Value.Name()
					if callName == "Sprintf" {
						ginkgoDesc = c.Call.Args[0].(*ssa.Const).Value.String()
					} else {
						panic("unknown args[0] call")
					}
				} else {
					panic("unknown args[0] type")
				}

				f1, ok := call.Call.Args[1].(*ssa.MakeInterface)
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
				childNode, found = m[f3]
				if !found {
					panic(".")
				}
				testTree = append(testTree, strings.ReplaceAll(ginkgoDesc, "\"", ""))
			}
		} else {
			if strings.Contains(calleePkg, "github.com/openshift/client-go") ||
				strings.Contains(calleePkg, "k8s.io/client-go") ||
				strings.Contains(calleePkg, "k8s.io/apimachinery") ||
				calleePkg == "github.com/openshift/origin/test/extended/util" {
				// k8s.io/client-go/dynamic.Create("context.Background()", "new k8s.io/apimachinery/pkg/apis/meta/v1/unstructured.Unstructured (complit)", "*t27", "nil:[]string")

				// get recv
				c := call{Test: testTree, Call: funcCall{Pkg: calleePkg, Func: calleeFuncName}}
				for _, arg := range edge.Site.Common().Args {
					c.Call.Args = append(c.Call.Args, arg.String())
				}
				callChan <- c
				fmt.Printf("%#v\n", c)

			} else if strings.Contains(calleePkg, "k8s.io/kubernetes/test/e2e") ||
				strings.Contains(calleePkg, "github.com/openshift/origin/test/") {
				// go into the helper functions
				childNode = callee

			} else {
				c := call{Test: testTree, Call: funcCall{Pkg: calleePkg}, Ignored: true}
				for _, arg := range edge.Site.Common().Args {
					c.Call.Args = append(c.Call.Args, arg.String())
				}
				callChan <- c
			}
		}

		if childNode != nil {
			traverseNodes(m, childNode, testTree, callChan)
		}
	}
}
