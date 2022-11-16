package main

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/tools/go/packages"
	"k8s.io/klog/v2"
)

func main() {
	defer klog.Flush()

	var originPathArg = flag.String("origin", "", "path to origin repository")
	var testdirFilterArg = flag.String("filter", "", "regexp to filter test dirs")
	flag.Parse()
	var rx *regexp.Regexp
	if *testdirFilterArg != "" {
		rx = regexp.MustCompile(*testdirFilterArg)
	}

	if *originPathArg == "" {
		klog.Fatalf("Provide path to origin repository using -origin")
	}
	if exists, err := checkIfPathExists(*originPathArg); !exists {
		klog.Exitf("Path %s does not exist", *originPathArg)
	} else if err != nil {
		klog.Exitf("Error occurred when checking if path %s exists: %v", *originPathArg, err)
	}

	astPkgs, err := getASTpackages(*originPathArg, getDirsToScan(*originPathArg, rx))
	if err != nil {
		panic(err)
	}
	for _, astPkg := range astPkgs {
		for _, e := range astPkg.Errors {
			if !strings.Contains(e.Msg, "no Go files") {
				panic(e)
			}
		}
	}

	for _, astPkg := range astPkgs {
		if len(astPkg.Errors) == 0 {
			workOnAstPkg(astPkg)
		}
	}
}

func checkIfPathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func getDirsToScan(originPath string, filter *regexp.Regexp) []string {
	originPkgs := func() []string {
		res := []string{}
		_ = filepath.WalkDir(originPath+"/test/extended/", func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				klog.Fatalf("Error when walking origin's dir tree: %v", err)
			}
			if d.IsDir() &&
				!strings.Contains(path, "testdata") &&
				(filter == nil || (filter != nil && filter.MatchString(path))) {
				res = append(res, path)
			}
			return nil
		})
		return res
	}()
	return originPkgs
}

func getASTpackages(originPath string, pkgs []string) ([]*packages.Package, error) {
	astCfg := packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedImports | packages.NeedTypes | packages.NeedTypesSizes |
			packages.NeedSyntax | packages.NeedTypesInfo | packages.NeedDeps,
		Dir: originPath,
		//BuildFlags: []string{"-N", "-l"},
	}
	ppkgs, err := packages.Load(&astCfg, pkgs...)
	if err != nil {
		return nil, fmt.Errorf("packages.Load failed: %w", err)
	}
	return ppkgs, nil
}
