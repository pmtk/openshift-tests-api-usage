package main

import (
	"flag"
	"io/fs"
	"path/filepath"

	"github.com/davecgh/go-spew/spew"
	klog "k8s.io/klog/v2"
)

/*
TODO:
- Transform API nodes into specific k8s/ocp API packages
- Export a summary: Ginkgo nodes + API used
- Run for whole origin repo
- Handle: `authorizationv1.GroupVersion.WithResource(tt.resource).GroupResource()`
*/

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

	origin, err := ParseOrigin(originPath, pkgs)
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
