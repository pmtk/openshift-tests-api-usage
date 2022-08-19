package main

import (
	"fmt"
	"sort"
)

const maxPasses = 10

func checkIfAllResolved(m map[string]*HelperFunctionNode) bool {
	for _, v := range m {
		for _, c := range v.children {
			if _, ok := c.(*HelperFunctionNode); ok {
				return false
			}
		}
	}

	return true
}

type ResolvedHelperFunction struct {
	Pkg, Recv, Func string
	APICalls        []string
}

func resolveHelperTree(rn Node) (map[string]ResolvedHelperFunction, error) {
	// at this point helper tree should be at most 2 levels deep:
	// (0) root node
	// (1) - helper func
	// (2)   - api call
	// (2)   - helper func call

	if _, ok := rn.(*RootNode); !ok {
		return nil, fmt.Errorf("expected RootNode as an argument, but it's %T", rn)
	}

	hs := make(map[string]*HelperFunctionNode, len(rn.GetChildren()))
	for _, helper := range rn.GetChildren() {
		if helper == nil {
			panic("rn.GetChildren() returned a nil node - shouldn't happen")
		}
		hn, ok := helper.(*HelperFunctionNode)
		if !ok {
			panic("helper is not a helper node - shouldn't happen")
		}
		hs[hn.Hash()] = hn
	}

	pass := 0

	// go through hs' nodes and change HelperFuctionNodes
	// Helper nodes into API calls these helpers are calling
	//
	// - helper func 1
	//   - api func 1
	//   - helper func 2
	// - helper func 2
	//   - api func 2
	//
	// will change into
	//
	// - helper func 1
	//   - api func 1
	//   - api func 2
	// - helper func 2
	//   - api func 2
	for {
		for k, v := range hs {
			apiCalls := []Node{}

			for i := 0; i < len(v.children); i++ {
				if len(v.children[i].GetChildren()) != 0 {
					// helper.helper.helper should not have children,
					// it should just be a "keyed ref" to another top level helper
					panic(fmt.Sprintf("v.children[i].GetChildren() is %d", v.children[i].GetChildren()))
				}

				c := v.children[i]
				helperCall, ok := c.(*HelperFunctionNode)
				if ok {
					if v.Pkg == helperCall.Pkg &&
						v.Func == helperCall.Func &&
						v.Recv == helperCall.Recv {
						// avoid recursive calls
						continue
					}

					if dependency, found := hs[helperCall.Hash()]; found {
						apiCalls = append(apiCalls, dependency.children...)
					}
				} else {
					apiCalls = append(apiCalls, c)
				}
			}

			hs[k].children = apiCalls
		}

		if checkIfAllResolved(hs) {
			break
		}
		if pass >= maxPasses {
			return nil, fmt.Errorf("deps unresolved after %d passes", maxPasses)
		}
		pass += 1
	}

	// transform hs into
	// - key: helper function hash
	//   api calls: []string with hashed API calls
	res := make(map[string]ResolvedHelperFunction, len(hs))
	for k, v := range hs {
		rhf := ResolvedHelperFunction{Pkg: v.Pkg, Recv: v.Recv, Func: v.Func}

		dedupSet := map[string]struct{}{}
		for _, a := range v.children {
			if n, ok := a.(*APIUsageNode); ok {
				dedupSet[n.Hash()] = struct{}{}
			} else {
				panic("after resolving: child of a helper func is not an APIUsageNode\n")
			}
		}

		for k := range dedupSet {
			rhf.APICalls = append(rhf.APICalls, k)
		}

		sort.Strings(rhf.APICalls)
		res[k] = rhf
	}

	return res, nil
}
