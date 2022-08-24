package main

import (
	"flag"
	"fmt"
	"go/token"
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

	klog.V(2).Infof("Pkgs (%v) to scan: %#v", len(pkgs), pkgs)
	buildReportUsingRTA(originPath, pkgs)
}

// FunCallInTest is a function FunCallInTest in context of specific ginkgo node
type FunCallInTest struct {
	Test    []string // [Describe-Description, [Context-Description...], It-Description]
	Call    FuncCall
	Ignored bool // Ignored informs if function call is not API calls, but persisted for debugging purposes
}

type FuncCall struct {
	Pkg        string
	Recv       string
	Func       string
	Args       []string
	FuncDefPos token.Position
}

// TODO
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
	initial, err := packages.Load(&cfg, "./test/extended/apiserver/") // pkgs...
	if err != nil {
		return fmt.Errorf("packages.Load failed: %w", err)
	}
	klog.V(2).Infof("packages.Load - end after %s\n", time.Since(start))

	klog.V(2).Infof("ssautil.AllPackages - start\n")
	start = time.Now()
	prog, _ := ssautil.AllPackages(initial, ssa.InstantiateGenerics)
	klog.V(2).Infof("ssautil.AllPackages - end after %s\n", time.Since(start))

	klog.V(2).Infof("prog.Build - start\n")
	start = time.Now()
	prog.Build()
	klog.V(2).Infof("prog.Build - end after %s\n", time.Since(start))

	fcm := map[string]*ssa.Function{}

	for _, p := range prog.AllPackages() {
		if p != nil && strings.Contains(p.Pkg.Path(), "github.com/openshift/origin/test/extended/") {
			init := p.Members["init"].(*ssa.Function)
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
							fcm[strings.ReplaceAll(cnst.Value.String(), "\"", "")] = anonFun
						}
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

	callChan := make(chan FunCallInTest)
	calls := []FunCallInTest{}

	go func() {
		for c := range callChan {
			calls = append(calls, c)
		}
	}()

	klog.V(2).Infof("Traversing %d tests - start\n", len(fcm))
	start = time.Now()
	for desc, fun := range fcm {
		node, found := rtaAnalysis.CallGraph.Nodes[fun]
		if !found {
			return fmt.Errorf("couldn't find %s (%s) in rtaAnalysis.CallGraph.Nodes[]", fun.Name(), desc)
		}
		klog.V(2).Infof("Inspecting test: %s", desc)
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
			if funcName != "GinkgoRecover" {
				traverseNodes(m, edge.Callee, testTree, callChan)
			}
			continue
		}

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
				panic("callee.Func.Params[0].Type() - unknown type")
			}
		}

		fcit := FunCallInTest{
			Test: testTree,
			Call: FuncCall{
				Pkg:        pkg,
				Recv:       recv,
				Func:       funcName,
				Args:       argsToStrings(edge.Site.Common().Args),
				FuncDefPos: callee.Func.Prog.Fset.Position(callee.Func.Pos()),
			},
		}

		if pkg == "github.com/onsi/ginkgo" {
			if funcName == "It" || funcName == "Context" || funcName == "Describe" {
				args := edge.Site.Value().Call.Args

				// Context/It("desc", func(){))
				//              visit ^^^^^^^^ by setting childNode
				f := getFuncFromValue(args[1])
				nodeToVisit, found := m[f]
				if !found {
					panic(fmt.Sprintf("f3(%v) not found in m", f.Name()))
				}

				ginkgoDesc := getStringFromValue(args[0])
				testTree = append(testTree, strings.ReplaceAll(ginkgoDesc, "\"", ""))
				traverseNodes(m, nodeToVisit, testTree, callChan)
			}
		} else {
			if strings.Contains(pkg, "github.com/openshift/client-go") ||
				strings.Contains(pkg, "k8s.io/client-go/dynamic") ||
				//strings.Contains(pkg, "k8s.io/apimachinery") ||
				pkg == "github.com/openshift/origin/test/extended/util" {
				// TODO Handle untyped clients like: k8s.io/client-go/dynamic.Create("context.Background()", "new k8s.io/apimachinery/pkg/apis/meta/v1/unstructured.Unstructured (complit)", "*t27", "nil:[]string")
				// TODO Handle typed clients as well

				// Store API call
				callChan <- fcit
				// klog.V(3).Infof("%#v\n", fcit)

			} else if strings.Contains(pkg, "k8s.io/kubernetes/test/e2e") || strings.Contains(pkg, "github.com/openshift/origin/test/") {
				// go into the helper functions
				if edge.Callee != edge.Caller {
					traverseNodes(m, callee, testTree, callChan)
				}

			} else {
				// Store non-API call for debug purposes
				fcit.Ignored = true
				callChan <- fcit
			}
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
		stop := 0
		_ = stop
	}

	panic(fmt.Sprintf("getStringFromValue: v's type in unexpected: %T", v))
}
