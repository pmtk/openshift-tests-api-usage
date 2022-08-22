package main

import (
	"fmt"
	"log"
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/callgraph/rta"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

func TestPlay(t *testing.T) {
	g := NewGomegaWithT(t)

	// Load, parse, and type-check the whole program.
	cfg := packages.Config{
		Mode:  packages.LoadAllSyntax,
		Dir:   "/home/pm/dev/origin/",
		Tests: true,
	}
	log.Printf("packages.Load - before\n")
	initial, err := packages.Load(&cfg, "./test/extended/apiserver/")
	g.Expect(err).NotTo(HaveOccurred())
	log.Printf("packages.Load - after\n")

	log.Printf("ssautil.AllPackages - before\n")
	prog, pkgs := ssautil.AllPackages(initial, ssa.InstantiateGenerics) // ssa.PrintPackages
	log.Printf("ssautil.AllPackages - after\n")
	_ = pkgs

	log.Printf("prog.Build - before\n")
	prog.Build()
	log.Printf("prog.Build - after\n")

	fcm := map[string]*ssa.Function{}
	fcs := []*ssa.Function{}

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
						fcs = append(fcs, anonFun)
						fcm[cnst.Value.String()] = anonFun
					}
				}
			}
		}
	}

	log.Printf("rta.Analyze - before\n")
	rtaAnalysis := rta.Analyze(fcs, true)
	log.Printf("rta.Analyze - after\n")

	for desc, fun := range fcm {
		fmt.Printf("%s - %s\n", desc, fun.Name())

		node, found := rtaAnalysis.CallGraph.Nodes[fun]
		g.Expect(found).To(BeTrue())

		traverseNodes(rtaAnalysis.CallGraph.Nodes, node, desc, []string{}, 1)
	}
}

func traverseNodes(m map[*ssa.Function]*callgraph.Node, node *callgraph.Node, parentTestName string, parentAPIcalls []string, lvl int) {
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

		testName := parentTestName
		var childNode *callgraph.Node

		if calleePkg == "github.com/onsi/ginkgo" {
			if calleeFunc == "It" || calleeFunc == "Context" {
				call := edge.Site.Value()
				args := call.Call.Args
				ginkgoDesc := ""

				// arg0 can be const(string) or call(fmt.Sprintf)
				if cnst, ok := args[0].(*ssa.Const); ok {
					ginkgoDesc = cnst.Value.String()
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

				fmt.Printf("+++ %s.%s (%s, %s)\n", calleePkg, calleeFunc, ginkgoDesc, f3.Name())

				var found bool
				childNode, found = m[f3]
				if !found {
					panic(".")
				}
				testName += " " + ginkgoDesc
			}
		} else {
			fmt.Printf("%s: %s.%s", parentTestName, calleePkg, calleeFunc)

			if strings.Contains(calleePkg, "github.com/openshift/client-go") ||
				strings.Contains(calleePkg, "k8s.io/client-go") ||
				strings.Contains(calleePkg, "k8s.io/apimachinery") ||
				calleePkg == "github.com/openshift/origin/test/extended/util" {
				fmt.Printf("\t[API]")
			}
			if strings.Contains(calleePkg, "k8s.io/kubernetes/test/e2e") ||
				strings.Contains(calleePkg, "github.com/openshift/origin/test/") {
				childNode = callee
				fmt.Printf("\tGOING DOWN")
			}
			fmt.Printf("\n")
		}

		if childNode != nil {
			traverseNodes(m, childNode, testName, parentAPIcalls, lvl+1)
		}
	}
}
