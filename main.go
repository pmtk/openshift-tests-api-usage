package main

import (
	"flag"

	"github.com/davecgh/go-spew/spew"
	klog "k8s.io/klog/v2"
)

/*
TODO:
- Transform API nodes into specific k8s/ocp API packages
- Export a summary: Ginkgo nodes + API used
- Run for whole origin repo
*/

func main() {
	klog.InitFlags(nil)
	flag.Parse()
	defer klog.Flush()

	origin, err := ParseOrigin("/home/pm/dev/origin/", []string{"./test/extended/apiserver"})
	if err != nil {
		klog.Fatalf("Failed to build test tree: %#v\n", err)
	}

	printTree(origin.Tests)
	printTree(origin.Helpers)

	hs, err := resolveHelperTree(origin.Helpers)
	if err != nil {
		klog.Fatalf("Failed to resolve helpers: %#v\n", err)
	}

	spew.Dump(hs)

	// TODO: Go over origin.Tests and transform Helper (flatten its API calls) and API nodes
	// TODO: Process "transformed API calls" - this should be close to final report of Tests+API their using
}
