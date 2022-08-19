package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	. "github.com/onsi/gomega"
	"golang.org/x/tools/go/ast/inspector"
)

func TestIsCallExprGinkgo(t *testing.T) {
	g := NewGomegaWithT(t)

	testFiles := []string{
		`package a
import (
	g "github.com/onsi/ginkgo"
)
var _ = g.Describe("Desc", func(){})
`,
		`package a
import (
	"github.com/onsi/ginkgo"
)
var _ = ginkgo.Describe("Desc", func(){})
`,
		`package a
import (
	. "github.com/onsi/ginkgo"
)
var _ = Describe("Desc", func(){})
`,
	}

	for _, tf := range testFiles {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, "", tf, 0)
		g.Expect(err).NotTo(HaveOccurred())

		inspector := inspector.New([]*ast.File{f})
		inspector.WithStack(
			[]ast.Node{
				&ast.CallExpr{},
			},
			func(n ast.Node, push bool, stack []ast.Node) (proceed bool) {
				ce, ok := n.(*ast.CallExpr)
				g.Expect(ok).To(BeTrue())
				g.Expect(isCallExprGinkgo(ce, f)).To(BeTrue())
				return true
			},
		)
	}
}

func TestCheckStackIfInsideGinkgo(t *testing.T) {
	g := NewGomegaWithT(t)

	tf := `package a
import (
	g "github.com/onsi/ginkgo"
)

var _ = g.Describe("Desc", func(){
	someFuncCall()
})
`

	// just to make sure we actually inspect someFuncCall's stack
	// and avoid false positive in case of not getting to the assertions
	reached := false

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", tf, 0)
	g.Expect(err).NotTo(HaveOccurred())

	inspector := inspector.New([]*ast.File{f})
	inspector.WithStack(
		[]ast.Node{
			&ast.CallExpr{},
		},
		func(n ast.Node, push bool, stack []ast.Node) (proceed bool) {
			ce, ok := n.(*ast.CallExpr)
			g.Expect(ok).To(BeTrue())

			// skip ginkgo nodes
			if isCallExprGinkgo(ce, f) {
				return true
			}

			ident, ok := ce.Fun.(*ast.Ident)
			g.Expect(ok).To(BeTrue())
			g.Expect(ident.Name).To(Equal("someFuncCall"))
			g.Expect(checkStackIfInsideGinkgo(f, stack)).To(BeTrue())
			reached = true
			return true
		},
	)

	g.Expect(reached).To(BeTrue())
}
