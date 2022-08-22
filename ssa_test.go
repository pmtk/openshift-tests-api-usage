package main

import (
	"log"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/callgraph/rta"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

// TODO: Handle cases like metav1, apimachinery, *CLI

type APIUsage struct {
	Test    []string
	API     string
	Ignored string // for reporting which pkgs were not considered  API
}

func TestPlay(t *testing.T) {
	g := NewGomegaWithT(t)

	// Load, parse, and type-check the whole program.
	cfg := packages.Config{
		Mode:  packages.LoadAllSyntax,
		Dir:   "/home/pm/dev/origin/",
		Tests: true,
	}
	log.Printf("packages.Load - start\n")
	start := time.Now()
	initial, err := packages.Load(&cfg, "./test/extended/apiserver/")
	g.Expect(err).NotTo(HaveOccurred())
	log.Printf("packages.Load - end after %s\n", time.Since(start))

	log.Printf("ssautil.AllPackages - start\n")
	start = time.Now()
	prog, _ := ssautil.AllPackages(initial, ssa.InstantiateGenerics) // ssa.PrintPackages
	log.Printf("ssautil.AllPackages - end after %s\n", time.Since(start))

	log.Printf("prog.Build - start\n")
	start = time.Now()
	prog.Build()
	log.Printf("prog.Build - end after %s\n", time.Since(start))

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
						g.Expect(ok).To(BeTrue())
						g.Expect(cnst.Type().String()).To(Equal("string"))

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

	log.Printf("rta.Analyze - start\n")
	start = time.Now()
	rtaAnalysis := rta.Analyze(fcs, true)
	log.Printf("rta.Analyze - end after %s\n", time.Since(start))

	// TODO: Parallelize traverseNodes + bufferedChannel? Don't think we'd have great speed up since building RTA is most time consuming
	apiChan := make(chan APIUsage)
	apiUsages := []APIUsage{}

	go func() {
		for api := range apiChan {
			apiUsages = append(apiUsages, api)
		}
	}()

	log.Printf("Traversing tests - start\n")
	start = time.Now()
	for desc, fun := range fcm {
		node, found := rtaAnalysis.CallGraph.Nodes[fun]
		g.Expect(found).To(BeTrue())
		traverseNodes(rtaAnalysis.CallGraph.Nodes, node, []string{desc}, apiChan)
	}
	log.Printf("Traversing tests - end after %s\n", time.Since(start))

	ignored := map[string]struct{}{}
	mergedApiUsage := map[string]map[string]struct{}{}
	for _, au := range apiUsages {
		if au.Ignored != "" {
			ignored[au.Ignored] = struct{}{}
		} else {
			key := strings.Join(au.Test, " ")
			if mergedApiUsage[key] == nil {
				mergedApiUsage[key] = map[string]struct{}{}
			}
			mergedApiUsage[key][au.API] = struct{}{}
		}
	}

	stop := 0
	_ = stop

	// apiUsages := map[string][]string
}

func traverseNodes(m map[*ssa.Function]*callgraph.Node, node *callgraph.Node, parentTestTree []string, apiChan chan<- APIUsage) {
	for _, edge := range node.Out {
		callee := edge.Callee
		calleeFunc := callee.Func.Name()
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

		// TODO: Check if can be improved (like slicing)
		testTree := make([]string, len(parentTestTree))
		copy(testTree, parentTestTree)

		var childNode *callgraph.Node

		if calleePkg == "github.com/onsi/ginkgo" {
			if calleeFunc == "It" || calleeFunc == "Context" {
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
				apiChan <- APIUsage{Test: testTree, API: calleePkg}
			} else if strings.Contains(calleePkg, "k8s.io/kubernetes/test/e2e") ||
				strings.Contains(calleePkg, "github.com/openshift/origin/test/") {
				childNode = callee
			} else {
				apiChan <- APIUsage{Test: testTree, Ignored: calleePkg}
			}
		}

		if childNode != nil {
			traverseNodes(m, childNode, testTree, apiChan)
		}
	}
}
