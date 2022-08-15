package main

import (
	"testing"

	. "github.com/onsi/gomega"
)

/*
root
  - helper  A.1

  - helper  B.2
    - API      Y.1
	- helper   B.1

  - helper  C.1
    - helper   B.2

  - helper  B.1
    - API      Z.1
*/
var resolvable = &RootNode{
	node: node{
		children: []Node{
			&HelperFunctionNode{Pkg: "D", Func: "1",
				node: node{
					children: []Node{
						&HelperFunctionNode{Pkg: "C", Func: "1"},
					},
				},
			},

			&HelperFunctionNode{Pkg: "A", Func: "1"},

			&HelperFunctionNode{Pkg: "B", Func: "2",
				node: node{
					children: []Node{
						&APIUsageNode{Pkg: "Y", Func: "1"},
						&APIUsageNode{Pkg: "Z", Func: "1"},
						&HelperFunctionNode{Pkg: "B", Func: "1"},
					},
				},
			},

			&HelperFunctionNode{Pkg: "C", Func: "1",
				node: node{
					children: []Node{
						&HelperFunctionNode{Pkg: "B", Func: "2"},
					},
				},
			},

			&HelperFunctionNode{Pkg: "B", Func: "1",
				node: node{
					children: []Node{
						&APIUsageNode{Pkg: "Z", Func: "1"},
					},
				},
			},
		},
	},
}

func TestHelperTreeResolver(t *testing.T) {
	g := NewGomegaWithT(t)
	expected := map[string]ResolvedHelperFunction{
		"A#1": {
			Pkg: "A", Func: "1",
		},
		"B#1": {
			Pkg: "B", Func: "1",
			APICalls: []string{"Z##1"},
		},
		"B#2": {
			Pkg: "B", Func: "2",
			APICalls: []string{"Y##1", "Z##1"},
		},
		"C#1": {
			Pkg: "C", Func: "1",
			APICalls: []string{"Y##1", "Z##1"},
		},
		"D#1": {
			Pkg: "D", Func: "1",
			APICalls: []string{"Y##1", "Z##1"},
		},
	}

	resolved, err := resolveHelperTree(resolvable)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(resolved).To(Equal(expected))
}
