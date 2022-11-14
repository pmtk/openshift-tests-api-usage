package main

import (
	"flag"
	"fmt"
	"go/token"
	"go/types"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"

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
	prog, ssaPkgs := getSSA(astPkgs)
	pkgs := getPkgs(astPkgs, ssaPkgs, prog)
	for _, pkg := range pkgs {
		workOnPkg(pkg)
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
	}
	ppkgs, err := packages.Load(&astCfg, pkgs...)
	if err != nil {
		return nil, fmt.Errorf("packages.Load failed: %w", err)
	}
	return ppkgs, nil
}

func getSSA(pkgs []*packages.Package) (*ssa.Program, []*ssa.Package) {
	prog, ssapkgs := ssautil.AllPackages(pkgs, ssa.InstantiateGenerics|ssa.GlobalDebug)
	prog.Build()
	return prog, ssapkgs
}

type Package struct {
	AST  *packages.Package
	SSA  *ssa.Package
	Prog *ssa.Program
}

func getPkgs(astPkgs []*packages.Package, ssaPkgs []*ssa.Package, prog *ssa.Program) []Package {
	pkgs := []Package{}
	for _, ssapkg := range ssaPkgs {
		if ssapkg == nil {
			continue
		}
		foundASTPkgs := filter(astPkgs, func(p *packages.Package) bool {
			if p == nil {
				return false
			}
			return ssapkg.Pkg.Path() == p.ID
		})
		if len(foundASTPkgs) == 0 {
			continue
		}
		if len(foundASTPkgs) > 1 {
			panic("len(foundASTPkgs) > 1")
		}
		astPkg := foundASTPkgs[0]
		pkgs = append(pkgs, Package{
			AST:  astPkg,
			SSA:  ssapkg,
			Prog: prog,
		})
	}
	return pkgs
}

func workOnPkg(pkg Package) {
	for _, member := range pkg.SSA.Members {
		if member.Token() != token.VAR || member.Object() == nil {
			continue
		}

		// TODO: Get actual `group` value
		switch obj := member.Object().Type().(type) {
		case *types.Named:
			_ = isGVR_Named(obj)
			println("detected var GVR")
		case *types.Slice:
			isGVR := isGVR_Type(obj.Elem())
			_ = isGVR
			fmt.Printf("detected []GVR\n")
		case *types.Map:
			isKeyGVR := isGVR_Type(obj.Key())
			isElemGVR := isGVR_Type(obj.Elem())
			fmt.Printf("detected gvr map key:%v val:%v\n", isKeyGVR, isElemGVR)
		default:
			panic(nil)
		}
	}

	// detect creation of GVR by:
	// - GroupVersionResource{ }
	// - gvr() helper func like in test/extended/etcd/etcd_storage_path.go

	// to do:
	// 1 scan pkg ast/ssa (decide) for all variables of type GroupVersionResource
	// 2 find all var _ = g.Describe
	// 3 scan Describe's anon-func in context of 1
	// 4 func scan line/instr by line/instr
	// 5 descend into functions with context build in 4
}

func isGVR_Type(t types.Type) bool {
	named, ok := t.(*types.Named)
	if !ok {
		return false
	}
	return isGVR_Named(named)
}

func isGVR_Named(n *types.Named) bool {
	typeName := n.Obj()
	return typeName.Id() == "GroupVersionResource" &&
		typeName.Pkg().Path() == "k8s.io/apimachinery/pkg/runtime/schema"
}

func findGinkgoDescribes() {

}

func getPackageVars() {

}
